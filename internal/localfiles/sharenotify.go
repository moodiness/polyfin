package localfiles

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// How a network share is watched, its Folder.ShareWatch: its server tells
// of its changes, or its folders are listed again every few minutes.
const (
	ShareNotified = "notified"
	SharePolled   = "polled"
)

// Why a network share is polled, its Folder.Polled: WebDAV servers do not
// tell of changes; the SMB server refuses to; it did not tell of a change
// a poll found; its watch failed, and is set up again shortly. Empty while
// the watch is being set up. A notified share whose server missed a
// change in one of its folders is also polled every few minutes:
// notifiedPartial.
const (
	polledWebDAV      = "webdav"
	polledUnsupported = "unsupported"
	polledSilent      = "silent"
	polledFailed      = "failed"
	notifiedPartial   = "partial"
)

// shareWatchState is how a share is watched, and why it is polled.
type shareWatchState struct {
	how, why string
}

// shareNotify is how the watch of a share by its server's notifications
// goes, shared by its poll and its watch: whether the server holds it, how
// many times it was set up, and how many answers it gave; whether it
// missed a change in a folder of the share; whether a poll found it
// silent, and the end of the watch under way. wake tells the poll the
// watch went on or off, or missed a change.
type shareNotify struct {
	mu       sync.Mutex
	on       bool
	partial  bool
	arms     int
	answers  int
	silenced bool
	cancel   context.CancelFunc
	wake     chan struct{}
}

// notifyMark is how a share's watch went at a time: two equal marks, on,
// tell of a watch held all along that gave no answer.
type notifyMark struct {
	on, partial   bool
	arms, answers int
}

func (n *shareNotify) mark() notifyMark {
	n.mu.Lock()
	defer n.mu.Unlock()
	return notifyMark{on: n.on, partial: n.partial, arms: n.arms, answers: n.answers}
}

// missedSome records that the server missed a change in a folder of the
// share, and reports whether that is news; the poll is woken, to list the
// share more often.
func (n *shareNotify) missedSome() bool {
	n.mu.Lock()
	news := !n.partial
	n.partial = true
	n.mu.Unlock()
	if news {
		select {
		case n.wake <- struct{}{}:
		default:
		}
	}
	return news
}

// set records the watch on or off, and wakes the poll when that changed.
func (n *shareNotify) set(on bool) {
	n.mu.Lock()
	changed := n.on != on
	n.on = on
	if on {
		n.arms++
	}
	n.mu.Unlock()
	if changed {
		select {
		case n.wake <- struct{}{}:
		default:
		}
	}
}

func (n *shareNotify) answered() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.answers++
}

// start records the end of the watch about to be set up, and reports
// whether a poll found the watch silent meanwhile.
func (n *shareNotify) start(cancel context.CancelFunc) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cancel = cancel
	return n.silenced
}

// silence ends the watch under way, found silent.
func (n *shareNotify) silence() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.silenced = true
	if n.cancel != nil {
		n.cancel()
	}
}

// takeSilenced reports whether a poll found the watch silent, and forgets
// it.
func (n *shareNotify) takeSilenced() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	silenced := n.silenced
	n.silenced = false
	return silenced
}

// notify watches an SMB share through its server's notifications until
// ctx ends, and has it scanned again quiet after the last change it tells
// of, but those of folders a scan skips. A share whose reader is no
// notifier is polled only, as is one whose server refuses to tell of
// changes. A watch that fails is set up again: at once after it worked,
// as when the server ended the session, which the share's reader opens
// again; else shareRearm later. Once set up again, the share is scanned:
// what changed meanwhile went untold.
//
// The watch is of the folder's tree. Windows servers are known to accept
// that watch at the root of a share and never answer it: once a poll finds
// it silent (see missed), a watch of a share's root is split, the root's
// own entries and the tree of each of its folders watched apart, as
// Windows answers both; another watch found silent leaves the share
// polled.
func (w *watcher) notify(ctx context.Context, p *sharePoll) {
	n := p.notify
	split, lost, quick, warned := false, false, false, false
	for ctx.Err() == nil {
		r, kind, err := w.shares.shareReader(ctx, p.id, p.address)
		armed, armedAt := false, time.Time{}
		if err == nil {
			watcher, ok := r.(notifier)
			if !ok {
				w.setShareWatch(ctx, p.id, SharePolled, polledWebDAV)
				return
			}
			watchCtx, cancel := context.WithCancel(ctx)
			if !n.start(cancel) {
				err = watcher.watch(watchCtx, notifyHow{split: split, skip: func(name string) bool { return skippedIn(kind, name) },
					armed: func() {
						armed, armedAt, warned = true, time.Now(), false
						n.set(true)
						why := ""
						if n.mark().partial {
							why = notifiedPartial
						}
						w.setShareWatch(ctx, p.id, ShareNotified, why)
						// The share answers again: it is no longer reported
						// unreachable until its next round.
						w.setPollReason(ctx, p.id, "")
						if lost {
							lost = false
							w.tell(ctx, p.id)
						}
					},
					changed: func(paths []string) {
						n.answered()
						if relevantChange(kind, paths) {
							w.tell(ctx, p.id)
						}
					}})
			}
			cancel()
			n.set(false)
			if ctx.Err() != nil {
				return
			}
			if n.takeSilenced() {
				if !split && watcher.splittable() {
					w.logger.Warn("A network share's server did not tell of a change; its folders are now watched apart", "address", p.address)
					split, lost = true, true
					continue
				}
				w.logger.Warn("A network share's server did not tell of a change; it is listed again every few minutes", "address", p.address)
				w.setShareWatch(ctx, p.id, SharePolled, polledSilent)
				return
			}
			switch {
			case errors.Is(err, errNotifyUnsupported):
				w.logger.Info("A network share's server does not tell of changes; it is listed again every few minutes", "address", p.address,
					"error", err)
				w.setShareWatch(ctx, p.id, SharePolled, polledUnsupported)
				return
			case errors.Is(err, errTooManyFolders):
				w.logger.Warn("A network share's server did not tell of a change, and its root holds too many folders to watch each; it is listed again every few minutes",
					"address", p.address)
				w.setShareWatch(ctx, p.id, SharePolled, polledSilent)
				return
			}
		}
		lost = true
		if !warned && ctx.Err() == nil {
			w.logger.Warn("A network share's server cannot tell of changes for now; it is listed again every few minutes", "address", p.address,
				"error", err)
			warned = true
		}
		w.setShareWatch(ctx, p.id, SharePolled, polledFailed)
		// A watch that worked is set up again at once; not twice in a row
		// unless it lasted rearm, so that a server failing each watch is
		// not asked again and again.
		if armed && (!quick || time.Since(armedAt) >= w.rearm) {
			quick = true
			continue
		}
		quick = false
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.rearm):
		}
	}
}

// tell has a share scanned again quiet after its last change.
func (w *watcher) tell(ctx context.Context, id accounts.ID) {
	select {
	case w.notified <- id:
	case <-ctx.Done():
	}
}

// relevantChange reports whether one of the paths a share's server told
// of may change what a scan of a folder of kind reads: none in a folder it
// skips, nor named as one. All may when the server did not tell which.
func relevantChange(kind string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		if !slices.ContainsFunc(strings.Split(p, "/"), func(name string) bool { return skippedIn(kind, name) }) {
			return true
		}
	}
	return false
}

// missed handles a change of a share's folder at rel a round found that
// the share's watch did not tell of: a round found the folder as the same
// scan left it while the watch was held, and the server told of nothing
// since, nor quiet later, a change told on its way. A watch that missed a
// change of the share's own folder is let go (see notify); one that missed
// a change deeper, as Samba misses those made on its own disk below the
// watched folder, is kept, the share listed again every few minutes too.
func (w *watcher) missed(ctx context.Context, p *sharePoll, mark notifyMark, rel string) {
	before, ok := p.calm.marks[rel]
	if !ok || !before.on || mark != before {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(w.quiet):
	}
	if p.notify.mark() != before {
		return
	}
	if rel == "" {
		p.notify.silence()
		return
	}
	if p.notify.missedSome() {
		w.logger.Warn("A network share's server did not tell of a change in one of its folders; it is also listed again every few minutes",
			"address", p.address)
		w.setShareWatch(ctx, p.id, ShareNotified, notifiedPartial)
	}
}

// shareWatch tells how a share is watched, and why it is polled; empty
// for a folder that is no share, or not watched.
func (w *watcher) shareWatch(id accounts.ID) (how, why string) {
	if w == nil {
		return "", ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	state := w.shareWatches[id]
	return state.how, state.why
}

// setShareWatch sets how a share is watched, unless its poll ended: the
// state of a share no longer polled stays cleared.
func (w *watcher) setShareWatch(ctx context.Context, id accounts.ID, how, why string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	w.shareWatches[id] = shareWatchState{how: how, why: why}
}
