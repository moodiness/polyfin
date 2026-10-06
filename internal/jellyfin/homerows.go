package jellyfin

import (
	"context"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// With playback prepared ahead in the settings, the titles of Continue
// Watching and Next Up have their versions listed in the background as
// soon as an app asks for those rows, so that a title opened from them
// lists its versions at once rather than a placeholder until the addons
// answer. Only the lists are asked for: item details analyze the version a
// title opens with (see prepareOpened).

const (
	// homeRowTitles is how many titles of a row's answer are listed ahead:
	// those an app shows first.
	homeRowTitles = 10
	// maxListing is how many titles are listed at once over the server,
	// as maxPreparing bounds preparations, for sources that limit their
	// rate.
	maxListing = 2
)

// listings lists titles' versions in the background, maxListing at a
// time, in the order they were asked for. Unlike preparations, it queues
// what it cannot start yet: a row's titles come together, and dropping all
// but two would leave most of them unlisted. Each title waits once per
// user, which bounds the queue by the titles of the rows asked for.
type listings struct {
	list func(user accounts.User, title accounts.ID)

	mu      sync.Mutex
	running int
	queue   []listing
	// waiting holds the titles queued or being listed, for a row asked
	// again meanwhile to queue none of them twice.
	waiting map[preparationKey]bool
}

type listing struct {
	user  accounts.User
	title accounts.ID
}

func newListings(list func(user accounts.User, title accounts.ID)) *listings {
	return &listings{list: list, waiting: map[preparationKey]bool{}}
}

// add queues a user's title, unless it waits already, and starts a worker
// while fewer than maxListing run.
func (l *listings) add(user accounts.User, title accounts.ID) {
	key := preparationKey{user: user.ID, title: title}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.waiting[key] {
		return
	}
	l.waiting[key] = true
	l.queue = append(l.queue, listing{user: user, title: title})
	if l.running < maxListing {
		l.running++
		go l.work()
	}
}

// work lists the titles queued until none is left.
func (l *listings) work() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			l.running--
			l.queue = nil
			l.mu.Unlock()
			return
		}
		next := l.queue[0]
		l.queue[0] = listing{}
		l.queue = l.queue[1:]
		l.mu.Unlock()
		l.list(next.user, next.title)
		l.mu.Lock()
		delete(l.waiting, preparationKey{user: next.user.ID, title: next.title})
		l.mu.Unlock()
	}
}

// listAhead lists, in the background, the versions of the movies and
// episodes among the first homeRowTitles items of a row's answer, with
// playback prepared ahead. It returns at once: the answer never waits.
// Channels and music are left out: these rows list neither, and their
// sources are not the version lists a title's page waits for.
func (h *Handler) listAhead(user accounts.User, items []library.Item) {
	if !h.Accounts.Settings().PrepareAhead {
		return
	}
	for _, item := range items[:min(len(items), homeRowTitles)] {
		if item.Kind == library.KindMovie || item.Kind == library.KindEpisode {
			h.listings.add(user, item.ID)
		}
	}
}

// listVersions lists a title's versions as PlaybackInfo does, waiting for
// every addon: lists still kept cost nothing, and an addon asked for the
// title already, by details opened meanwhile, is joined, not asked again.
func (h *Handler) listVersions(user accounts.User, title accounts.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), preparationTimeout)
	defer cancel()
	if _, err := h.Library.Versions(ctx, user, title); err != nil {
		h.Logger.Debug("The versions of a title in a home row could not be listed ahead", "error", err)
	}
}
