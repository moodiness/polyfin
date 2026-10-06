package library

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Item details answer with the versions Polyfin knows, without waiting for
// the addons whose streams it does not remember, or remembers stale: it
// asks them in the background (see VersionsNow), then asks them again a
// little later (see followUpList), and Pending tells how many it still
// asks, for a page to add their versions as they come.

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
// streams are remembered, stale ones included (see staleLists), and of
// IPTV sources. It asks the other addons, and those whose list is stale,
// in the background, for later requests, which share their answers (see
// Pending). complete reports whether every addon's streams were known, and
// none asked again.
func (s *Service) VersionsNow(ctx context.Context, user accounts.User, id accounts.ID) (versions []Version, complete bool, err error) {
	return s.versionsOf(ctx, user, id, askLater)
}

// KnownVersions lists, as Versions does, the versions of a movie or an
// episode known now, stale ones included, asking no addon.
func (s *Service) KnownVersions(ctx context.Context, user accounts.User, id accounts.ID) ([]Version, error) {
	versions, _, err := s.versionsOf(ctx, user, id, knownOnly)
	return versions, err
}

// Pending is how many addons are still asked, in the background, for the
// streams of a user's title: for the first time, or since their list
// expired (see VersionsNow), or again, their follow-ups scheduled or
// running (see followUpList). An addon that fails is no longer asked: its
// versions are left out, as they are when a request waits for it, but for
// those of its stale list, which item details keep listing. The title's
// page follows its versions from then on (see FollowedProgress).
func (s *Service) Pending(ctx context.Context, user accounts.User, id accounts.ID) (int, error) {
	// The addons asked for the first time are counted first: one that
	// answers meanwhile has its follow-up scheduled before it stops being
	// asked, so it is counted at least once.
	pending := s.askedFor(user.ID, id)
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return 0, err
	}
	if AudioKind(t.kind) {
		return pending, nil
	}
	serving := streamServing(v, t)
	s.tracked().pages.Put(askedKey{user.ID, t.item}, titlePage{t: t, serving: serving})
	return pending + s.streamFollowUps(serving, t), nil
}

// askedFor counts the addons still asked for a user's title for the first
// time, or since their list expired.
func (s *Service) askedFor(user, item accounts.ID) int {
	if a, ok := s.asked.Get(askedKey{user, item}); ok {
		return int(a.pending.Load())
	}
	return 0
}

// askStreams asks addons for a user's title's streams in the background,
// detached from the request, unless they are already asked for it. Each
// addon is asked once at a time whoever asks (see streams), so a request
// that waits for every version, such as PlaybackInfo, joins them. Each
// addon done, answering or not, is told to whoever follows the title's
// versions (see OnVersionsChanged).
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
			defer s.versionsChanged(t.item)
			defer a.pending.Add(-1)
			if _, err := s.streams(detached, entry, t.metaType, t.id); err != nil {
				s.logger.Warn("An addon could not list streams", "addon", entry.addon.Manifest.Name, "error", err)
			}
		}()
	}
}
