package jellyfin

import (
	"context"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// With playback prepared ahead in the settings, the titles of Continue
// Watching and Next Up have their versions listed in the background once
// an app's request for those rows is answered, so that a title opened
// from them lists its versions at once rather than a placeholder until
// the addons answer.
// The first version of the first homeRowPrepared titles is analyzed too,
// those a user is most likely to play next, and the bytes of the segment
// its play starts at read within the user's budget (see prepareRowTitle);
// item details analyze the version a title opens with (see
// prepareOpened).

const (
	// homeRowTitles is how many titles of a row's answer are listed ahead:
	// those an app shows first.
	homeRowTitles = 10
	// homeRowPrepared is how many of them are prepared once listed.
	homeRowPrepared = 2
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
	list func(row context.Context, user accounts.User, title accounts.ID, prepare bool)

	mu      sync.Mutex
	running int
	queue   []listing
	// waiting holds the titles queued or being listed, for a row asked
	// again meanwhile to queue none of them twice.
	waiting map[preparationKey]bool
}

// listing is a user's title to list, and with prepare to prepare once
// listed. row holds the views of the row's request, which the titles it
// queued share (see library.SharedViews).
type listing struct {
	row     context.Context
	user    accounts.User
	title   accounts.ID
	prepare bool
}

func newListings(list func(row context.Context, user accounts.User, title accounts.ID, prepare bool)) *listings {
	return &listings{list: list, waiting: map[preparationKey]bool{}}
}

// add queues a user's title, unless it waits already, and starts a worker
// while fewer than maxListing run. A title queued again to be prepared is
// prepared, if it is not being listed yet.
func (l *listings) add(row context.Context, user accounts.User, title accounts.ID, prepare bool) {
	key := preparationKey{user: user.ID, title: title}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.waiting[key] {
		if prepare {
			for i := range l.queue {
				if l.queue[i].user.ID == user.ID && l.queue[i].title == title {
					l.queue[i].prepare = true
				}
			}
		}
		return
	}
	l.waiting[key] = true
	l.queue = append(l.queue, listing{row: row, user: user, title: title, prepare: prepare})
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
		l.list(next.row, next.user, next.title, next.prepare)
		l.mu.Lock()
		delete(l.waiting, preparationKey{user: next.user.ID, title: next.title})
		l.mu.Unlock()
	}
}

// listAhead lists, in the background, the versions of the movies and
// episodes among the first homeRowTitles items of a row's answer, with
// playback prepared ahead, and prepares the first homeRowPrepared of them.
// It is called once the answer is written, which never waits for them,
// and the queued titles do not hold the database while the home screen's
// other rows are asked; they share the view the row's request, ctx's,
// built (see library.SharedViews). Channels and music are left out: these
// rows list neither, and their sources are not the version lists a
// title's page waits for.
func (h *Handler) listAhead(ctx context.Context, user accounts.User, items []library.Item) {
	if !h.Accounts.Settings().PrepareAhead {
		return
	}
	var row context.Context
	titles := 0
	for _, item := range items[:min(len(items), homeRowTitles)] {
		if versioned(item.Kind) {
			if row == nil {
				row = library.SharedViews(ctx)
			}
			h.listings.add(row, user, item.ID, titles < homeRowPrepared)
			titles++
		}
	}
}

// listVersions lists a title's versions as PlaybackInfo does, waiting for
// every addon, under row, the context its row's titles share: lists still
// kept cost nothing, and an addon asked for the title already, by details
// opened meanwhile, is joined, not asked again. With prepare, the first
// version a play would read is then prepared (see prepareRowTitle). It is
// background work, whose requests to addons are paced (see
// library.Background). An IPTV source is checked once for the listing,
// which describes its title more than once (see iptv.CheckOnce).
func (h *Handler) listVersions(row context.Context, user accounts.User, title accounts.ID, prepare bool) {
	ctx, cancel := context.WithTimeout(iptv.CheckOnce(library.Background(stremio.WithOrigin(row, stremio.OriginCatalogs))), preparationTimeout)
	defer cancel()
	versions, err := h.Library.Versions(ctx, user, title)
	if err != nil {
		h.Logger.Debug("The versions of a title in a home row could not be listed ahead", "error", err)
		return
	}
	if versions = h.offered(ctx, user, library.Item{ID: title}, versions); prepare && len(versions) > 0 {
		h.prepareRowTitle(user, title, versions[0])
	}
}

// prepareRowTitle queues a preparation of a home row's title whose first
// version is version: its analysis, unless it was analyzed, then the
// bytes of the segment its play starts at, where the user stopped, within
// what the user's first segments may read ahead (see warmAhead). A
// version that recently failed, or a torrent's that no playback runs, is
// left as prepareVersions leaves it.
func (h *Handler) prepareRowTitle(user accounts.User, title accounts.ID, version library.Version) {
	if h.Playback.Failed(version.ID) || h.idleTorrent(version) {
		return
	}
	h.prepare(preparationKey{user: user.ID, title: title, version: version.ID}, func(ctx context.Context) bool {
		if _, analyzed := h.Playback.Analyzed(ctx, version.ID); !analyzed && !h.analyzeInTurn(ctx, []library.Version{version}) {
			return false
		}
		var start time.Duration
		if data, err := h.UserData.Get(ctx, user.ID, []accounts.ID{title}); err == nil {
			start = data[title].Position
		}
		h.warmAhead(ctx, user, version, start)
		return true
	})
}
