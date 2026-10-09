package playback

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// hold is the connection to its source a version that holds one (see
// library.Version.HoldsConnection) takes while it plays: a replay
// programme, read as a title's file is, but counted among its source's
// connections as a channel's feed is (see makeRoom). Every request of the
// play holds it; it lingers for liveTimes.grace after the last, so that
// a player between two requests keeps its place.
type hold struct {
	st      *liveState
	version accounts.ID
	source  accounts.ID
	opened  time.Time
	// ready is closed once room was made for the hold, or not: failed
	// then tells why, and the requests that joined it meanwhile fail too.
	ready  chan struct{}
	failed error
	// users counts the requests holding it, and grace ends it once none
	// has for a while; both are guarded by st.mu.
	users int
	grace *time.Timer
}

func (h *hold) since() time.Time { return h.opened }

func (h *hold) unread() bool {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	return h.users == 0
}

// onlyFor is false: a replay under way is not ended for another stream.
func (h *hold) onlyFor(accounts.ID) bool { return false }

func (h *hold) evict() {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	h.dropLocked()
}

// dropLocked forgets the hold. h.st.mu is held.
func (h *hold) dropLocked() {
	if h.grace != nil {
		h.grace.Stop()
		h.grace = nil
	}
	if h.st.holds[h.version] == h {
		delete(h.st.holds, h.version)
	}
}

// Hold takes one of its source's connections for a play of version until
// release, when the version holds one (see library.Version.HoldsConnection);
// else it does nothing. The requests of the same version share one: those
// that come while room is made for it wait for it, and fail with it. When
// the source's connections are all in use, the feeds and holds no one
// uses are ended, then the oldest channel the same user alone watches;
// else it fails with ErrSlotsInUse.
func (s *Service) Hold(ctx context.Context, version library.Version) (release func(), err error) {
	if !version.HoldsConnection || version.Origin.Addon == (accounts.ID{}) {
		return func() {}, nil
	}
	st := &s.feeds
	st.mu.Lock()
	if st.holds == nil {
		st.holds = map[accounts.ID]*hold{}
	}
	h := st.holds[version.ID]
	fresh := h == nil
	if fresh {
		h = &hold{st: st, version: version.ID, source: version.Origin.Addon, opened: time.Now(), ready: make(chan struct{})}
		st.holds[version.ID] = h
	}
	h.users++
	if h.grace != nil {
		h.grace.Stop()
		h.grace = nil
	}
	grace := st.timingLocked().grace
	st.mu.Unlock()
	if fresh {
		_, err = s.makeRoom(ctx, h.source, h, userOf(ctx))
		st.mu.Lock()
		if err != nil {
			h.failed = err
			h.dropLocked()
		}
		close(h.ready)
		st.mu.Unlock()
	} else {
		select {
		case <-h.ready:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err == nil {
			st.mu.Lock()
			err = h.failed
			st.mu.Unlock()
		}
	}
	if err != nil {
		st.mu.Lock()
		h.leaveLocked(grace)
		st.mu.Unlock()
		return nil, err
	}
	released := false
	return func() {
		st.mu.Lock()
		defer st.mu.Unlock()
		if !released {
			released = true
			h.leaveLocked(grace)
		}
	}, nil
}

// leaveLocked ends a request's use of the hold, which is then ended once
// no request used it for grace. h.st.mu is held.
func (h *hold) leaveLocked(grace time.Duration) {
	st := h.st
	if h.users--; h.users == 0 && st.holds[h.version] == h {
		h.grace = time.AfterFunc(grace, func() {
			st.mu.Lock()
			defer st.mu.Unlock()
			if h.users == 0 {
				h.dropLocked()
			}
		})
	}
}
