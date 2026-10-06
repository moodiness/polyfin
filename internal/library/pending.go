package library

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Item details answer with the versions Polyfin knows, without waiting for
// the addons whose streams it does not remember: it asks them in the
// background (see VersionsNow), and Pending tells how many it still asks,
// for a page to add their versions as they come.

const (
	// askedFor is how long the addons asked for a user's title are counted;
	// their requests end well before, within the client's timeout.
	askedFor = 5 * time.Minute
	// maxAsked bounds the titles counted at once.
	maxAsked = 2000
)

type askedKey struct {
	user, item accounts.ID
}

// asked counts the addons still asked for a user's title.
type asked struct {
	pending atomic.Int32
}

// VersionsNow lists, as Versions does, the versions of a movie or an
// episode known now, without waiting for addons: those of the addons whose
// streams are remembered, and of IPTV sources. It asks the other addons in
// the background, for later requests, which share their answers (see
// Pending). complete reports whether every addon's streams were known.
func (s *Service) VersionsNow(ctx context.Context, user accounts.User, id accounts.ID) (versions []Version, complete bool, err error) {
	return s.versionsOf(ctx, user, id, askLater)
}

// KnownVersions lists, as Versions does, the versions of a movie or an
// episode known now, asking no addon.
func (s *Service) KnownVersions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, error) {
	versions, _, err := s.versionsOf(ctx, user, id, knownOnly)
	return versions, err
}

// Pending is how many addons are still asked, in the background, for the
// streams of a user's title (see VersionsNow). An addon that fails is no
// longer asked: its versions are left out, as they are when a request
// waits for it.
func (s *Service) Pending(user accounts.User, id accounts.ID) int {
	if a, ok := s.asked.Get(askedKey{user.ID, id}); ok {
		return int(a.pending.Load())
	}
	return 0
}

// askStreams asks addons for a user's title's streams in the background,
// detached from the request, unless they are already asked for it. Each
// addon is asked once at a time whoever asks (see streams), so a request
// that waits for every version, such as PlaybackInfo, joins them.
func (s *Service) askStreams(ctx context.Context, user accounts.User, t target, unknown []installed) {
	if len(unknown) == 0 {
		return
	}
	key := askedKey{user.ID, t.item}
	if a, ok := s.asked.Get(key); ok && a.pending.Load() > 0 {
		return
	}
	a := &asked{}
	a.pending.Store(int32(len(unknown)))
	s.asked.Put(key, a)
	detached := context.WithoutCancel(ctx)
	for _, entry := range unknown {
		go func() {
			defer a.pending.Add(-1)
			if _, err := s.streams(detached, entry, t.metaType, t.id); err != nil {
				s.logger.Warn("An addon could not list streams", "addon", entry.addon.Manifest.Name, "error", err)
			}
		}()
	}
}
