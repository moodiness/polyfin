package jellyfin

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// With playback prepared ahead in the settings, Polyfin does before a play
// what its first play would wait for: it reads the versions a title plays
// first as soon as the title's details open, again when an addon answering
// later changes which version comes first, the first version of the first
// titles of Continue Watching and Next Up (see listAhead), and readies the
// next episode near the end of the one playing.

const (
	// maxNextEpisodeLead is how long before an episode ends the next one is
	// prepared with version lists kept ten minutes, their default life (see
	// nextEpisodeLead).
	maxNextEpisodeLead = 9 * time.Minute
	// maxPreparing and maxQueued bound the load preparations put on
	// sources, some of which limit their rate: the preparations running at
	// once over the server, and those waiting for a place, the newest
	// first. Beyond maxQueued the oldest waiting is dropped: the title the
	// user opened last is the one they are about to play.
	maxPreparing = 2
	maxQueued    = 4
	// preparedFor is how long a preparation done for a user is not done
	// again.
	preparedFor = 10 * time.Minute
	// preparationTimeout bounds a preparation: listing a title's versions
	// and subtitles, then analyzing some, which ffprobe's timeout bounds
	// too.
	preparationTimeout = 2 * time.Minute
	// preparedVersions is how many of a title's versions opening it
	// prepares: the first ones a play reads at once (see
	// parallelAttempts).
	preparedVersions = 2
)

// preparationKey names what a preparation prepares for a user: a title
// whose first version is version, or with next the episode after a title.
type preparationKey struct {
	user, title accounts.ID
	next        bool
	version     accounts.ID
}

// preparation is a preparation waiting for a place: work reports whether
// it prepared what it had to.
type preparation struct {
	key  preparationKey
	work func(ctx context.Context) bool
}

// preparations runs preparations, maxPreparing at a time, the newest
// first. A preparation done is not done again for preparedFor; one that
// failed may be, by a later trigger.
type preparations struct {
	now    func() time.Time
	logger *slog.Logger

	mu      sync.Mutex
	workers int
	// queue holds the preparations waiting, the newest first; running
	// those under way, and prepared when each one done was last done.
	queue    []preparation
	running  map[preparationKey]bool
	prepared map[preparationKey]time.Time
}

func newPreparations(logger *slog.Logger) *preparations {
	return &preparations{now: time.Now, logger: logger, running: map[preparationKey]bool{}, prepared: map[preparationKey]time.Time{}}
}

// add queues a preparation of key, first, unless it was done recently or
// is under way. The same preparation waiting already moves to the front.
func (p *preparations) add(key preparationKey, work func(ctx context.Context) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.recentlyPrepared(key, p.now()) || p.running[key] {
		return
	}
	p.queue = slices.Insert(slices.DeleteFunc(p.queue, func(q preparation) bool { return q.key == key }), 0, preparation{key: key, work: work})
	if len(p.queue) > maxQueued {
		if p.logger != nil {
			p.logger.Debug("A preparation waiting was dropped for a newer one", "dropped", len(p.queue)-maxQueued)
		}
		clear(p.queue[maxQueued:])
		p.queue = p.queue[:maxQueued]
	}
	if p.workers < maxPreparing {
		p.workers++
		go p.work()
	}
}

// work runs the preparations waiting, the newest first, until none is.
// Each runs detached from the request that triggered it.
func (p *preparations) work() {
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.workers--
			p.queue = nil
			p.mu.Unlock()
			return
		}
		next := p.queue[0]
		p.queue[0] = preparation{}
		p.queue = p.queue[1:]
		p.running[next.key] = true
		p.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), preparationTimeout)
		done := next.work(ctx)
		cancel()
		p.mu.Lock()
		delete(p.running, next.key)
		if done {
			now := p.now()
			p.prepared[next.key] = now
			p.forget(now)
		}
		p.mu.Unlock()
	}
}

// idle reports whether no preparation runs or waits.
func (p *preparations) idle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workers == 0
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

func (p *preparations) recentlyPrepared(key preparationKey, now time.Time) bool {
	at, ok := p.prepared[key]
	return ok && now.Sub(at) < preparedFor
}

// forget drops what no longer bounds anything once there is much of it.
func (p *preparations) forget(now time.Time) {
	if len(p.prepared) < 1000 {
		return
	}
	for key, at := range p.prepared {
		if now.Sub(at) >= preparedFor {
			delete(p.prepared, key)
		}
	}
}

// prepare queues work, a preparation of key, which runs in the background.
func (h *Handler) prepare(key preparationKey, work func(ctx context.Context) bool) {
	h.preparations.add(key, work)
}

// prepareOpened analyzes, in the background, the versions a play of a
// title whose details opened would read first: the version it was opened
// as (see openedIndex), which PlaybackInfo tries first, then the next, of
// versions, without those that recently failed, as many as a play reads at
// once, preparedVersions at most. A play then finds them analyzed, and the
// keyframe index of an HLS play read.
func (h *Handler) prepareOpened(ctx context.Context, user accounts.User, item library.Item, versions []library.Version, opened accounts.ID) {
	settings := h.Accounts.Settings()
	if !settings.PrepareAhead || len(versions) == 0 {
		return
	}
	at := openedIndex(opened, versions)
	order := append([]library.Version{versions[at]}, slices.Delete(slices.Clone(versions), at, at+1)...)
	order = order[:min(len(order), preparedVersions, max(settings.VersionAttempts, 1))]
	h.prepareVersions(ctx, preparationKey{user: user.ID, title: item.ID, version: order[0].ID}, order)
}

// prepareVersions queues a preparation of key that analyzes those of
// versions not analyzed yet, at once, unless all are.
func (h *Handler) prepareVersions(ctx context.Context, key preparationKey, versions []library.Version) {
	versions = slices.DeleteFunc(slices.Clone(versions), func(v library.Version) bool { return !h.unanalyzed(ctx, v) })
	if len(versions) == 0 {
		return
	}
	h.prepare(key, func(ctx context.Context) bool {
		var wg sync.WaitGroup
		for _, version := range versions {
			wg.Go(func() { h.analyzeAhead(ctx, version) })
		}
		wg.Wait()
		for _, version := range versions {
			if _, ok := h.Playback.Analyzed(ctx, version.ID); !ok {
				return false
			}
		}
		return true
	})
}

// prepareListed prepares as prepareOpened does once the title's versions
// are all in, for details that answered before some addons did (see
// knownPlayable): waiting for them joins the requests the details started,
// asking nothing more, within preparationTimeout. An addon answering later
// still may change the version that comes first, which prepares it in turn
// (see versionsChanged).
func (h *Handler) prepareListed(ctx context.Context, user accounts.User, item library.Item, opened accounts.ID) {
	if !h.Accounts.Settings().PrepareAhead {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), preparationTimeout)
		defer cancel()
		if p, err := h.playable(ctx, user, item); err == nil {
			h.prepareOpened(ctx, user, item, p.versions, opened)
		}
	}()
}

// nextEpisodeLead is how long before an episode ends the next one is
// prepared: a minute less than version lists are kept (the settings'
// VersionListMinutes), so they still are when it starts, and at most
// maxNextEpisodeLead; at least a minute all the same.
func nextEpisodeLead(settings accounts.Settings) time.Duration {
	life := time.Duration(settings.VersionListMinutes) * time.Minute
	return max(min(maxNextEpisodeLead, life-time.Minute), time.Minute)
}

// prepareNearTheEnd prepares the episode after the one a user plays once
// at most nextEpisodeLead of it is left. runtime is that of the version
// playing; an unknown runtime prepares nothing.
func (h *Handler) prepareNearTheEnd(user accounts.User, device accounts.ID, item library.Item, runtime, position time.Duration) {
	settings := h.Accounts.Settings()
	if item.Kind != library.KindEpisode || runtime <= 0 || runtime-position > nextEpisodeLead(settings) || !settings.PrepareAhead {
		return
	}
	h.prepare(preparationKey{user: user.ID, title: item.ID, next: true}, func(ctx context.Context) bool {
		episodes, err := h.Library.Episodes(ctx, user, item.SeriesID, nil)
		if err != nil {
			h.Logger.Debug("The episodes of a series could not be listed to prepare the next", "error", err)
			return false
		}
		next, ok := episodeAfter(episodes, item.ID)
		if !ok {
			return true
		}
		// Through Library.Item, as details, for parental control.
		if next, err = h.Library.Item(ctx, user, next.ID); err != nil {
			h.Logger.Debug("The next episode could not be prepared", "error", err)
			return true
		}
		// Listed as PlaybackInfo lists them, so that both lists are cached
		// when it asks.
		p, err := h.playable(ctx, user, next)
		if err != nil {
			h.Logger.Debug("The versions of the next episode could not be listed", "error", err)
		}
		versions := p.versions
		if len(versions) == 0 {
			return false
		}
		h.analyzeAhead(ctx, versions[0])
		// Its images too, in the background, as its play would ask,
		// once the playback of this episode stopped.
		if h.Thumbnails != nil {
			h.Thumbnails.Queue(versions[0], device)
		}
		_, analyzed := h.Playback.Analyzed(ctx, versions[0].ID)
		return analyzed
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

// analyzeAhead does before a play asks what the first play of a version
// waits for: its analysis, then what an HLS play reads besides, its
// keyframe index, read while it is analyzed, the bytes of its first
// segment, which Plan warms into the source cache, and where its subtitle
// tracks sit in the index. Each is kept like the analysis, and a
// PlaybackInfo arriving meanwhile waits for the read under way rather than
// starting another. All of it is background work, which gives way to the
// playbacks reading the same hosts (see playback.Background).
func (h *Handler) analyzeAhead(ctx context.Context, version library.Version) {
	if h.Playback.Failed(version.ID) {
		return
	}
	ctx = playback.Background(ctx)
	started := time.Now()
	analysis, err := h.Playback.Analyze(ctx, version)
	if err != nil {
		h.Logger.Debug("A version could not be analyzed ahead of playback", "addon", version.Addon, "error", err)
		return
	}
	// A version without an index still plays as it is; a first play would
	// find that out the same way. One with an index has the bytes of its
	// first segment warmed in the background (see playback.Service.Warm).
	if _, err := h.Playback.Plan(ctx, version); err != nil {
		h.Logger.Debug("A version's keyframe index could not be read ahead of playback", "addon", version.Addon, "error", err)
	}
	h.Playback.SubtitlesLocated(ctx, version, analysis)
	h.Logger.Debug("Prepared a version ahead of playback", "addon", version.Addon, "duration", time.Since(started))
}
