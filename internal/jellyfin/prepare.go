package jellyfin

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// With playback prepared ahead in the settings, Polyfin does before a play
// what its first play would wait for: it analyzes the version a title
// plays first as soon as the title's details open, and readies the next
// episode near the end of the one playing.

const (
	// nextEpisodeLead is how long before an episode ends the next one is
	// prepared: less than the ten minutes stream lists are cached, so they
	// still are when it starts.
	nextEpisodeLead = 9 * time.Minute
	// maxPreparing, preparationsPerMinute and preparedFor bound the load
	// preparations put on sources, some of which limit their rate: the
	// preparations running at once over the server, those starting per user
	// in a minute, and how long a title prepared for a user is not prepared
	// again.
	maxPreparing          = 2
	preparationsPerMinute = 6
	preparedFor           = 10 * time.Minute
	// preparationTimeout bounds a preparation: listing a title's versions
	// and subtitles, then analyzing one, which ffprobe's timeout bounds too.
	preparationTimeout = 2 * time.Minute
)

var (
	errPrepared = errors.New("prepared recently")
	errTooMany  = errors.New("too many preparations")
)

// preparationKey names what a preparation prepares for a user: a title, or
// with next the episode after it.
type preparationKey struct {
	user, title accounts.ID
	next        bool
}

// preparations admits preparations within their bounds. One beyond them is
// dropped, not queued: a later trigger may start it.
type preparations struct {
	now func() time.Time

	mu      sync.Mutex
	running int
	// starts are the times each user's preparations started in the last
	// minute, oldest first, and prepared when each key was last prepared.
	starts   map[accounts.ID][]time.Time
	prepared map[preparationKey]time.Time
}

func newPreparations() *preparations {
	return &preparations{now: time.Now, starts: map[accounts.ID][]time.Time{}, prepared: map[preparationKey]time.Time{}}
}

// start admits a preparation of key, which the caller ends with finish.
func (p *preparations) start(key preparationKey) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.recentlyPrepared(key, now) {
		return errPrepared
	}
	starts := p.recentStarts(key.user, now)
	if p.running >= maxPreparing || len(starts) >= preparationsPerMinute {
		return errTooMany
	}
	p.running++
	p.starts[key.user] = append(starts, now)
	p.prepared[key] = now
	p.forget(now)
	return nil
}

// claim records that a running preparation also prepares key, unless key
// was prepared recently.
func (p *preparations) claim(key preparationKey) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.recentlyPrepared(key, now) {
		return false
	}
	p.prepared[key] = now
	return true
}

// finish ends a preparation start admitted.
func (p *preparations) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running--
}

func (p *preparations) recentlyPrepared(key preparationKey, now time.Time) bool {
	at, ok := p.prepared[key]
	return ok && now.Sub(at) < preparedFor
}

func (p *preparations) recentStarts(user accounts.ID, now time.Time) []time.Time {
	starts := p.starts[user]
	for len(starts) > 0 && now.Sub(starts[0]) >= time.Minute {
		starts = starts[1:]
	}
	return starts
}

// forget drops what no longer bounds anything once there is much of it.
func (p *preparations) forget(now time.Time) {
	if len(p.prepared) < 1000 && len(p.starts) < 1000 {
		return
	}
	for key, at := range p.prepared {
		if now.Sub(at) >= preparedFor {
			delete(p.prepared, key)
		}
	}
	for user := range p.starts {
		if len(p.recentStarts(user, now)) == 0 {
			delete(p.starts, user)
		}
	}
}

// prepare runs work in the background, detached from the request that
// triggered it, once the bounds admit a preparation of key.
func (h *Handler) prepare(key preparationKey, work func(ctx context.Context)) {
	if err := h.preparations.start(key); err != nil {
		if errors.Is(err, errTooMany) {
			h.Logger.Debug("A preparation was dropped", "error", err)
		}
		return
	}
	go func() {
		defer h.preparations.finish()
		ctx, cancel := context.WithTimeout(context.Background(), preparationTimeout)
		defer cancel()
		work(ctx)
	}()
}

// prepareOpened analyzes, in the background, the version a play of a title
// whose details opened would start with: the first of versions, which come
// in PlaybackInfo's order, without those that recently failed.
func (h *Handler) prepareOpened(ctx context.Context, user accounts.User, item library.Item, versions []library.Version) {
	if !h.Accounts.Settings().PrepareAhead || len(versions) == 0 || !h.unanalyzed(ctx, versions[0]) {
		return
	}
	version := versions[0]
	h.prepare(preparationKey{user: user.ID, title: item.ID}, func(ctx context.Context) {
		h.analyzeAhead(ctx, version)
	})
}

// prepareNearTheEnd prepares the episode after the one a user plays once
// at most nextEpisodeLead of it is left. runtime is that of the version
// playing; an unknown runtime prepares nothing.
func (h *Handler) prepareNearTheEnd(user accounts.User, item library.Item, runtime, position time.Duration) {
	if item.Kind != library.KindEpisode || runtime <= 0 || runtime-position > nextEpisodeLead || !h.Accounts.Settings().PrepareAhead {
		return
	}
	h.prepare(preparationKey{user: user.ID, title: item.ID, next: true}, func(ctx context.Context) {
		episodes, err := h.Library.Episodes(ctx, user, item.SeriesID, nil)
		if err != nil {
			h.Logger.Debug("The episodes of a series could not be listed to prepare the next", "error", err)
			return
		}
		next, ok := episodeAfter(episodes, item.ID)
		if !ok || !h.preparations.claim(preparationKey{user: user.ID, title: next.ID}) {
			return
		}
		// Through Library.Item, as details, for parental control.
		if next, err = h.Library.Item(ctx, user, next.ID); err != nil {
			h.Logger.Debug("The next episode could not be prepared", "error", err)
			return
		}
		// Listed as PlaybackInfo lists them, so that both lists are cached
		// when it asks.
		p, err := h.playable(ctx, user, next)
		if err != nil {
			h.Logger.Debug("The versions of the next episode could not be listed", "error", err)
		}
		if versions := p.ordered(next.ID); len(versions) > 0 {
			h.analyzeAhead(ctx, versions[0])
		}
	})
}

// episodeAfter is the episode following current among a series' episodes
// in their order, the next of its season, else the first of the next one,
// if it is out. Specials are left out, as Next Up leaves them out.
func episodeAfter(episodes []library.Item, current accounts.ID) (library.Item, bool) {
	regular := regularEpisodes(episodes)
	i := slices.IndexFunc(regular, func(e library.Item) bool { return e.ID == current })
	if i < 0 || i+1 == len(regular) || !regular[i+1].Available {
		return library.Item{}, false
	}
	return regular[i+1], true
}

// unanalyzed reports whether a version is neither analyzed nor recently
// failed, which a preparation would only repeat.
func (h *Handler) unanalyzed(ctx context.Context, version library.Version) bool {
	if h.Playback.Failed(version.ID) {
		return false
	}
	_, analyzed := h.Playback.Analyzed(ctx, version.ID)
	return !analyzed
}

// analyzeAhead analyzes a version before a play asks. A PlaybackInfo
// arriving meanwhile waits for this analysis rather than starting another.
func (h *Handler) analyzeAhead(ctx context.Context, version library.Version) {
	if !h.unanalyzed(ctx, version) {
		return
	}
	started := time.Now()
	if _, err := h.Playback.Analyze(ctx, version); err != nil {
		h.Logger.Debug("A version could not be analyzed ahead of playback", "addon", version.Addon, "error", err)
		return
	}
	h.Logger.Debug("Analyzed a version ahead of playback", "addon", version.Addon, "duration", time.Since(started))
}
