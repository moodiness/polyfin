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

// Network shares are watched by listing their folders again, as neither
// WebDAV nor the SMB library tells of changes: a round every shareEvery,
// of at most shareLists listings shareGap apart, the share's root first,
// then its other folders in turn from where the last round stopped. A
// share holding more folders is listed whole over several rounds; its
// root, where a new movie's folder lands, every round. A folder whose
// listing differs from the one its last scan read has the share scanned
// again quiet later.
const (
	shareEvery = 2 * time.Minute
	shareLists = 100
	shareGap   = 500 * time.Millisecond
)

// shareSource reads what the polls of the shares compare: Service.
type shareSource interface {
	// shareReader returns the reader of a share's folder and what the
	// folder holds.
	shareReader(ctx context.Context, id accounts.ID, address string) (remote, string, error)
	// shareSeen returns the prints of the folders the last scan of a
	// share's folder listed, by path, nil until a scan of it succeeds;
	// scanned tells whether a scan of it ended since the service started.
	shareSeen(id accounts.ID) (prints map[string]uint64, scanned bool)
}

// polling is a share polled: its address, and the end of its poll.
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
			// Once the poll ended: it sets no reason after this.
			w.setReason(id, "")
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
// listed, past its root; whether the share failed the last round; and
// whether a scan was asked for because the last one failed.
type sharePoll struct {
	id      accounts.ID
	address string
	after   string
	failing bool
	retried bool
}

// poll lists a share's folders every round until ctx ends. The first
// round waits a whole interval: the share was just scanned, or is about to
// be.
func (w *watcher) poll(ctx context.Context, id accounts.ID, address string) {
	p := &sharePoll{id: id, address: address}
	ticker := time.NewTicker(w.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.round(ctx, p)
		}
	}
}

// round compares the listings of a share's folders with those its last
// scan read, and has it scanned again once one differs, or once it
// answers again after it failed. A folder gone, or refused now, differs;
// a share that cannot be listed is unwatched until it answers again.
func (w *watcher) round(ctx context.Context, p *sharePoll) {
	r, kind, err := w.shares.shareReader(ctx, p.id, p.address)
	if err != nil {
		if errors.Is(err, errShareRefused) {
			w.unreachable(ctx, p, err)
		} else if ctx.Err() == nil {
			w.logger.Warn("A network share to watch could not be read", "address", p.address, "error", err)
		}
		return
	}
	prints, scanned := w.shares.shareSeen(p.id)
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
		w.changedShare(ctx, p.id)
		return
	case prints == nil:
		// Its last scan failed, and it answers now: one more scan, not
		// one a round.
		if scanned && !p.retried {
			p.retried = true
			w.changedShare(ctx, p.id)
		}
		return
	}
	p.retried = false
	if folderPrint(kind, root) != prints[""] {
		w.changedShare(ctx, p.id)
		return
	}
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
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission), errors.Is(err, errIsFile):
			w.changedShare(ctx, p.id)
			return
		case err != nil:
			w.unreachable(ctx, p, err)
			return
		case folderPrint(kind, entries) != prints[rel]:
			w.changedShare(ctx, p.id)
			return
		}
	}
}

// listFolder lists a share's folder, within listTimeout.
func listFolder(ctx context.Context, r remote, rel string) ([]remoteEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	return r.list(ctx, rel)
}

// changedShare has a share scanned again quiet later.
func (w *watcher) changedShare(ctx context.Context, id accounts.ID) {
	select {
	case w.polled <- id:
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
