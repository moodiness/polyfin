package localfiles

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Network shares are watched by listing their folders again: a round
// every shareEvery, of at most shareLists listings shareGap apart, the
// share's root first, then its other folders in turn from where the last
// round stopped. A share holding more folders is listed whole over several
// rounds; its root, where a new movie's folder lands, every round. A
// folder whose listing differs from the one its last scan read has the
// share scanned again quiet later.
//
// An SMB server also tells of changes (see notify): the share is then
// scanned again quiet after the last change it told of, and the rounds,
// every shareNotifiedEvery, only make up for a watch that stopped
// answering. A watch that failed is set up again shareRearm later.
const (
	shareEvery         = 2 * time.Minute
	shareNotifiedEvery = 10 * time.Minute
	shareRearm         = 30 * time.Second
	shareLists         = 100
	shareGap           = 500 * time.Millisecond
)

// shareSource reads what the polls of the shares compare: Service.
type shareSource interface {
	// shareReader returns the reader of a share's folder and what the
	// folder holds.
	shareReader(ctx context.Context, id accounts.ID, address string) (remote, string, error)
	// shareSeen returns what the scans of a share's folder saw.
	shareSeen(id accounts.ID) seenShare
}

// seenShare is what the scans of a share's folder saw since the service
// started: the prints of the folders the last scan that succeeded listed,
// by path, nil until one does; how many scans kept prints, which tells a
// poll whether those it compared are still the last; and whether a scan
// of it ended.
type seenShare struct {
	prints  map[string]uint64
	kept    int
	scanned bool
}

// sharePolled is a share a poll found changed, against the prints the
// kept-th scan of it kept (see seenShare).
type sharePolled struct {
	id   accounts.ID
	kept int
}

// polling is a share polled: its address, and the end of its poll and of
// its watch.
type polling struct {
	address string
	cancel  context.CancelFunc
}

// pollShares polls the shares to watch not polled yet, those whose
// address changed again, and stops polling the others.
func (w *watcher) pollShares(shares map[accounts.ID]string) {
	for id, p := range w.polls {
		if address, ok := shares[id]; !ok || address != p.address {
			p.cancel()
			delete(w.polls, id)
			delete(w.changed, id)
			// Once the poll ended: it sets no reason, nor watch, after
			// this.
			w.setReason(id, "")
			w.mu.Lock()
			delete(w.shareWatches, id)
			w.mu.Unlock()
		}
	}
	for id, address := range shares {
		if w.polls[id] != nil {
			continue
		}
		ctx, cancel := context.WithCancel(w.ctx)
		w.polls[id] = &polling{address: address, cancel: cancel}
		w.spawn(func() { w.poll(ctx, id, address) })
	}
}

// sharePoll is how the poll of a share goes: the last folder its rounds
// listed, past its root; whether the share failed the last round; whether
// a scan was asked for because the last one failed; the folders its
// rounds found as the last scan left them; and how the share's watch by
// its server's notifications goes.
type sharePoll struct {
	id      accounts.ID
	address string
	after   string
	failing bool
	retried bool
	calm    *calmPrints
	notify  *shareNotify
}

func newSharePoll(id accounts.ID, address string) *sharePoll {
	return &sharePoll{id: id, address: address, notify: &shareNotify{wake: make(chan struct{}, 1)}}
}

// calmPrints are the folders of a share the rounds found as the kept-th
// scan of it left them, by path, with how the share's watch went before
// the round that listed each last.
type calmPrints struct {
	kept  int
	marks map[string]notifyMark
}

// poll lists a share's folders every round until ctx ends, and has an SMB
// share watched by its server's notifications meanwhile: a round every
// interval. The first round waits a whole interval: the share was just
// scanned, or is about to be.
func (w *watcher) poll(ctx context.Context, id accounts.ID, address string) {
	p := newSharePoll(id, address)
	if shareKind(address) == ShareSMB {
		w.setShareWatch(ctx, id, SharePolled, "")
		w.spawn(func() { w.notify(ctx, p) })
	} else {
		w.setShareWatch(ctx, id, SharePolled, polledWebDAV)
	}
	last := time.Now()
	timer := time.NewTimer(w.every)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.notify.wake:
			// A round past due comes at once.
			timer.Reset(time.Until(last.Add(w.interval(p))))
		case <-timer.C:
			w.round(ctx, p)
			last = time.Now()
			timer.Reset(w.interval(p))
		}
	}
}

// interval is the time between two rounds of a share's poll: longer while
// its server tells of its changes, the poll then only making up for a
// watch that stopped answering; unless the server missed a change in a
// folder of the share.
func (w *watcher) interval(p *sharePoll) time.Duration {
	if mark := p.notify.mark(); mark.on && !mark.partial {
		return w.notifiedEvery
	}
	return w.every
}

// round compares the listings of a share's folders with those its last
// scan read, and has it scanned again once one differs, or once it
// answers again after it failed. A folder gone, or refused now, differs;
// a share that cannot be listed is unwatched until it answers again.
func (w *watcher) round(ctx context.Context, p *sharePoll) {
	// The watch is looked at before the folders are listed: on then, it
	// was on before the listings.
	mark := p.notify.mark()
	r, kind, err := w.shares.shareReader(ctx, p.id, p.address)
	if err != nil {
		if errors.Is(err, errShareRefused) {
			w.unreachable(ctx, p, err)
		} else if ctx.Err() == nil {
			w.logger.Warn("A network share to watch could not be read", "address", p.address, "error", err)
		}
		return
	}
	seen := w.shares.shareSeen(p.id)
	prints := seen.prints
	root, err := listFolder(ctx, r, "")
	if err != nil {
		w.unreachable(ctx, p, err)
		return
	}
	back := p.failing
	p.failing = false
	w.setPollReason(ctx, p.id, "")
	switch {
	case back:
		w.changedShare(ctx, p.id, seen.kept)
		return
	case prints == nil:
		// Its last scan failed, and it answers now: one more scan, not
		// one a round.
		if seen.scanned && !p.retried {
			p.retried = true
			w.changedShare(ctx, p.id, seen.kept)
		}
		return
	}
	p.retried = false
	if p.calm == nil || p.calm.kept != seen.kept {
		p.calm = &calmPrints{kept: seen.kept, marks: map[string]notifyMark{}}
	}
	if folderPrint(kind, "", root) != prints[""] {
		w.missed(ctx, p, mark, "")
		w.changedShare(ctx, p.id, seen.kept)
		return
	}
	p.calm.marks[""] = mark
	// The root comes first: "" sorts before any other path.
	folders := slices.Sorted(maps.Keys(prints))
	if len(folders) > 0 && folders[0] == "" {
		folders = folders[1:]
	}
	next, found := slices.BinarySearch(folders, p.after)
	if found {
		next++
	}
	for range min(w.lists-1, len(folders)) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.gap):
		}
		if next >= len(folders) {
			next = 0
		}
		rel := folders[next]
		next++
		p.after = rel
		entries, err := listFolder(ctx, r, rel)
		switch {
		case errors.Is(err, fs.ErrPermission):
			w.changedShare(ctx, p.id, seen.kept)
			return
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, errIsFile):
			w.missed(ctx, p, mark, rel)
			w.changedShare(ctx, p.id, seen.kept)
			return
		case err != nil:
			w.unreachable(ctx, p, err)
			return
		case folderPrint(kind, rel, entries) != prints[rel]:
			w.missed(ctx, p, mark, rel)
			w.changedShare(ctx, p.id, seen.kept)
			return
		}
		p.calm.marks[rel] = mark
	}
}

// listFolder lists a share's folder, within listTimeout.
func listFolder(ctx context.Context, r remote, rel string) ([]remoteEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	return r.list(ctx, rel)
}

// changedShare has a share scanned again quiet later, found changed
// against the prints the kept-th scan of it kept.
func (w *watcher) changedShare(ctx context.Context, id accounts.ID, kept int) {
	select {
	case w.polled <- sharePolled{id, kept}:
	case <-ctx.Done():
	}
}

// unreachable leaves a share that failed a round unwatched, with why: it
// keeps its schedule, and is scanned again once it answers.
func (w *watcher) unreachable(ctx context.Context, p *sharePoll, err error) {
	if ctx.Err() != nil {
		return
	}
	reason := unwatchedFailed
	switch failure := shareFailure(err); failure {
	case errUnreachable, errRefused:
		reason = failure
	}
	if !p.failing {
		w.logger.Warn("A network share cannot be watched for changes; it is scanned on its schedule", "address", p.address,
			"reason", reason, "error", err)
	}
	p.failing = true
	w.setPollReason(ctx, p.id, reason)
}

// setPollReason sets why a share is not watched, unless its poll ended:
// the reason of a share no longer polled stays cleared.
func (w *watcher) setPollReason(ctx context.Context, id accounts.ID, reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if reason == "" {
		delete(w.unwatched, id)
	} else {
		w.unwatched[id] = reason
	}
}
