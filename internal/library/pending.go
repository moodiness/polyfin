package library

import (
	"context"
	"sync"
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

// asked holds the addons still asked for a user's title.
type asked struct {
	mu     sync.Mutex
	addons map[accounts.ID]bool
}

// asking returns a copy of the addons still asked.
func (a *asked) asking() map[accounts.ID]bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	asking := make(map[accounts.ID]bool, len(a.addons))
	for addon := range a.addons {
		asking[addon] = true
	}
	return asking
}

// done records that an addon is no longer asked.
func (a *asked) done(addon accounts.ID) {
	a.mu.Lock()
	delete(a.addons, addon)
	a.mu.Unlock()
}

// add records the addons of entries not asked yet as asked, and returns
// them.
func (a *asked) add(entries []installed) []installed {
	a.mu.Lock()
	defer a.mu.Unlock()
	var added []installed
	for _, entry := range entries {
		if !a.addons[entry.addon.ID] {
			a.addons[entry.addon.ID] = true
			added = append(added, entry)
		}
	}
	return added
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
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return 0, err
	}
	if AudioKind(t.kind) {
		return 0, nil
	}
	serving := streamServing(v, t)
	s.tracked().pages.Put(askedKey{user.ID, t.item}, titlePage{t: t, serving: serving})
	return s.pendingOf(user.ID, t, serving), nil
}

// pendingOf counts the addons among serving still asked for a user's
// title: for the first time, since their list expired, or again, their
// follow-ups scheduled or running. Each counts once, whichever way it is
// asked. Those asked are read first: one that answers meanwhile has its
// follow-up scheduled before it stops being asked, so it is never missed.
func (s *Service) pendingOf(user accounts.ID, t target, serving []installed) int {
	var asking map[accounts.ID]bool
	if a, ok := s.asked.Get(askedKey{user, t.item}); ok {
		asking = a.asking()
	}
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	n := 0
	for _, entry := range serving {
		_, following := s.followUps.running[followKey{listStreams, streamKey{entry.addon.ID, t.metaType, t.id}}]
		if asking[entry.addon.ID] || following {
			n++
		}
	}
	return n
}

// AskAgain asks the user's addons again for the streams of a movie or an
// episode, as a refresh of the title does for its streams only: each
// addon's stream list for it is forgotten, the one saved included, its
// follow-ups stop, and the addons are asked in the background, as item
// details ask them (see VersionsNow). Addons still asked for the title are
// left to answer. ErrNotFound is returned for an item the user cannot
// reach, or that is neither a movie nor an episode.
func (s *Service) AskAgain(ctx context.Context, user accounts.User, id accounts.ID) error {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return err
	}
	if t.kind != KindMovie && t.kind != KindEpisode {
		return ErrNotFound
	}
	serving := streamServing(v, t)
	known, _ := s.listVersions(ctx, t, serving, knownOnly)
	for _, version := range known {
		s.versions.Delete(version.ID)
	}
	for _, entry := range serving {
		// An IPTV source's streams are Polyfin's own, never kept.
		if !entry.addon.IPTV() {
			s.forgetStreams(ctx, streamKey{entry.addon.ID, t.metaType, t.id})
		}
	}
	if _, _, err := s.VersionsNow(ctx, user, id); err != nil {
		return err
	}
	// The title's other pages hear that its addons are asked again.
	s.versionsChanged(t.item)
	return nil
}

// askStreams asks addons for a user's title's streams in the background,
// detached from the request, but for those already asked for it. Each
// addon is asked once at a time whoever asks (see streams), so a request
// that waits for every version, such as PlaybackInfo, joins them. Each
// addon done, answering or not, is told to whoever follows the title's
// versions (see OnVersionsChanged).
func (s *Service) askStreams(ctx context.Context, user accounts.User, t target, unknown []installed) {
	if len(unknown) == 0 {
		return
	}
	key := askedKey{user.ID, t.item}
	a, ok := s.asked.Get(key)
	if !ok {
		a = &asked{addons: make(map[accounts.ID]bool, len(unknown))}
	}
	// Put again, so that it is kept as long as its latest addons are asked.
	s.asked.Put(key, a)
	detached := context.WithoutCancel(ctx)
	for _, entry := range a.add(unknown) {
		go func() {
			defer s.versionsChanged(t.item)
			defer a.done(entry.addon.ID)
			if _, err := s.streams(detached, entry, t.metaType, t.id); err != nil {
				s.logger.Warn("An addon could not list streams", "addon", entry.addon.Manifest.Name, "error", err)
			}
		}()
	}
}
