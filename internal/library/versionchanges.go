package library

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title's page adds the versions of the addons that answer after it
// opened (see Pending). It learns how far the listing has come without
// building the user's view again for every question: item details remember
// what the page follows, the title and the addons asked for it, and the
// answers are counted from the lists kept (see FollowedProgress). Each
// change is also told as it happens (see OnVersionsChanged), for the page
// to hear of it without asking.

const (
	// maxTitles bounds the titles whose addon lists are tied to their
	// item, and maxPages the title pages followed.
	maxTitles = 20000
	maxPages  = 2000
)

// listedTitle is what addons are asked for: a title or an episode.
type listedTitle struct {
	contentType, id string
}

// titlePage is what a user's title page follows: the title, and the addons
// that list its streams in the user's view.
type titlePage struct {
	t       target
	serving []installed
}

// versionTracking ties addon lists to the items they list versions of,
// remembers the title pages followed, and the stream lists not saved (see
// savedStreams). Its zero value is ready: its caches are made on first use.
type versionTracking struct {
	once    sync.Once
	titles  *cache.Cache[listedTitle, accounts.ID]
	pages   *cache.Cache[askedKey, titlePage]
	unsaved *cache.Cache[streamKey, bool]
	// changed is told the item whose versions changed (see
	// OnVersionsChanged).
	changed atomic.Pointer[func(item accounts.ID)]
	// swept is when saved lists past keeping were last deleted, in Unix
	// nanoseconds.
	swept atomic.Int64
}

// tracked returns s's version tracking, ready.
func (s *Service) tracked() *versionTracking {
	t := &s.tracking
	t.once.Do(func() {
		t.titles = cache.New[listedTitle, accounts.ID](maxTitles, versionsTTL)
		t.pages = cache.New[askedKey, titlePage](maxPages, askedFor)
		t.unsaved = cache.New[streamKey, bool](maxTitles, staleLists)
	})
	return t
}

// OnVersionsChanged has changed told the item whose versions may have
// changed: an addon's list for it was stored or replaced, an addon asked
// for it is done, or its follow-ups ended. changed runs on the goroutine
// that made the change, so it must return quickly.
func (s *Service) OnVersionsChanged(changed func(item accounts.ID)) {
	s.tracked().changed.Store(&changed)
}

// versionsChanged tells the item whose versions may have changed.
func (s *Service) versionsChanged(item accounts.ID) {
	if changed := s.tracked().changed.Load(); changed != nil {
		(*changed)(item)
	}
}

// streamsChanged saves an addon's stream list for a title as it is kept
// now, and tells the item it lists versions of.
func (s *Service) streamsChanged(key streamKey) {
	s.saveStreams(key)
	if item, ok := s.tracked().titles.Get(listedTitle{key.contentType, key.id}); ok {
		s.versionsChanged(item)
	}
}

// FollowedProgress answers, for a title's page that item details or
// Pending answered lately, what Pending and KnownVersions would, from what
// is kept in memory: the user's view is not built again. id is the title's
// identifier or one of its versions'. ok is false when the page is not
// followed, for the caller to ask Pending and KnownVersions.
func (s *Service) FollowedProgress(ctx context.Context, user accounts.User, id accounts.ID) (pending int, versions []Version, ok bool) {
	item := id
	if version, known := s.versions.Get(id); known {
		item = version.Item
	}
	page, ok := s.tracked().pages.Get(askedKey{user.ID, item})
	if !ok {
		return 0, nil, false
	}
	pending = s.pendingOf(user.ID, page.t, page.serving)
	versions, _ = s.listVersions(ctx, page.t, page.serving, knownOnly)
	return pending, versions, true
}

// streamListOf returns, as listOf does, the stream list an addon gave for
// a title, restoring the list saved before a restart when none is kept in
// memory (see savedStreams).
func (s *Service) streamListOf(ctx context.Context, key streamKey) (l list[stremio.Stream], fresh, ok bool) {
	if l, fresh, ok = listOf(s, s.streamLists, key); ok {
		return l, fresh, ok
	}
	saved, ok := s.savedStreams(ctx, key)
	if !ok {
		return list[stremio.Stream]{}, false, false
	}
	// An answer stored meanwhile wins.
	l = saved
	s.streamLists.Update(key, func(kept list[stremio.Stream], had bool) (list[stremio.Stream], bool) {
		if had {
			l = kept
			return kept, false
		}
		return saved, true
	})
	fresh, ok = l.state(s.now(), s.listLife())
	return l, fresh, ok
}
