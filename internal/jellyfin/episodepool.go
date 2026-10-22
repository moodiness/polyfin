package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Library-wide episode listings, for live-TV style apps such as
// NostalgiaTV. These apps build "channels" by asking for every episode of
// the TV libraries at once (/Items?IncludeItemTypes=Episode&Recursive=true,
// with or without a ParentId), often narrowed to a genre, then for the
// genres themselves (/Genres without a ParentId). Stremio catalogs have no
// such query, so the first series of each TV library are expanded into
// their aired episodes, the series' genres filled in from their details.
// The pool is built once, kept poolFresh, and served stale while it is
// built again in the background, so a channel sync answers from memory.

const (
	// poolSeries is how many series of each TV library a pool expands.
	poolSeries = 40
	// poolFresh is how long a pool is served before it is built again.
	poolFresh = 30 * time.Minute
	// poolWorkers bounds the details and episode lists read at once.
	poolWorkers = 6
)

type poolEntry struct {
	at       time.Time
	series   []library.Item
	episodes []library.Item
	// building is closed when the first build ends; refreshing is true
	// while a later one runs.
	building   chan struct{}
	refreshing bool
}

// episodePools holds the pools by user and libraries: a user's libraries,
// parental control and hidden genres decide what their pool holds.
type episodePools struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
}

var pools episodePools

// episodeListing answers a library-wide episode listing (see above). It
// reports whether it answered: it leaves other listings alone, those of a
// series, a season, a collection or a movie library included.
func (h *Handler) episodeListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool,
	names nameFilter, fields fieldSet, start, limit int) bool {
	if recursive, _ := boolQuery(r, "recursive"); !recursive {
		return false
	}
	include := listQuery(r, "includeItemTypes")
	has := func(t string) bool {
		return slices.ContainsFunc(include, func(i string) bool { return strings.EqualFold(i, t) })
	}
	if !has("Episode") || has("Movie") {
		return false
	}
	libraries, ok := h.tvLibraries(r.Context(), user, parent, hasParent)
	if !ok {
		return false
	}
	entry := h.episodePool(r.Context(), user, libraries)
	var listed []library.Item
	seriesKept := map[accounts.ID]bool{}
	for _, series := range entry.series {
		if !names.requested() || slices.ContainsFunc(series.Genres, names.matches) {
			seriesKept[series.ID] = true
			if has("Series") {
				listed = append(listed, series)
			}
		}
	}
	for _, episode := range entry.episodes {
		if seriesKept[episode.SeriesID] {
			listed = append(listed, episode)
		}
	}
	from := min(max(start, 0), len(listed))
	to := min(from+max(limit, 0), len(listed))
	if wholeListing(r) {
		to = len(listed)
	}
	items, err := h.dtos(r, user, listed[from:to], fields, nil)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(listed), StartIndex: start})
	return true
}

// poolGenres lists the genres of the user's TV libraries' pools, for
// /Genres asked without a ParentId. ok is false when the user has no TV
// library.
func (h *Handler) poolGenres(ctx context.Context, user accounts.User) ([]string, bool) {
	libraries, ok := h.tvLibraries(ctx, user, accounts.ID{}, false)
	if !ok {
		return nil, false
	}
	entry := h.episodePool(ctx, user, libraries)
	var genres []string
	for _, series := range entry.series {
		for _, genre := range series.Genres {
			if !slices.ContainsFunc(genres, func(g string) bool { return strings.EqualFold(g, genre) }) {
				genres = append(genres, genre)
			}
		}
	}
	slices.SortFunc(genres, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return genres, true
}

// tvLibraries returns the TV libraries a listing reads: parent when it is
// one, else every TV library of the user. ok is false for a parent that is
// not a TV library, or a user without one.
func (h *Handler) tvLibraries(ctx context.Context, user accounts.User, parent accounts.ID, hasParent bool) ([]library.Item, bool) {
	all, err := h.Library.Libraries(ctx, user)
	if err != nil {
		return nil, false
	}
	var tv []library.Item
	for _, l := range all {
		if l.CollectionType == "tvshows" && (!hasParent || l.ID == parent) {
			tv = append(tv, l)
		}
	}
	return tv, len(tv) > 0
}

// episodePool returns the pool of libraries for user, building it on the
// first call and again in the background once it is older than poolFresh.
func (h *Handler) episodePool(ctx context.Context, user accounts.User, libraries []library.Item) *poolEntry {
	ids := make([]string, len(libraries))
	for i, l := range libraries {
		ids[i] = l.ID.String()
	}
	key := user.ID.String() + "|" + strings.Join(ids, ",")
	detached := context.WithoutCancel(ctx)

	pools.mu.Lock()
	if pools.entries == nil {
		pools.entries = map[string]*poolEntry{}
	}
	entry, found := pools.entries[key]
	if !found {
		entry = &poolEntry{building: make(chan struct{})}
		pools.entries[key] = entry
		pools.mu.Unlock()
		series, episodes := h.buildPool(detached, user, libraries)
		pools.mu.Lock()
		entry.at, entry.series, entry.episodes = time.Now(), series, episodes
		close(entry.building)
		pools.mu.Unlock()
		return entry
	}
	stale := !entry.at.IsZero() && time.Since(entry.at) > poolFresh && !entry.refreshing
	if stale {
		entry.refreshing = true
	}
	pools.mu.Unlock()

	<-entry.building
	if stale {
		go func() {
			series, episodes := h.buildPool(detached, user, libraries)
			pools.mu.Lock()
			entry.at, entry.series, entry.episodes, entry.refreshing = time.Now(), series, episodes, false
			pools.mu.Unlock()
		}()
	}
	pools.mu.Lock()
	defer pools.mu.Unlock()
	return &poolEntry{at: entry.at, series: entry.series, episodes: entry.episodes}
}

// buildPool lists the first poolSeries series of each library, with their
// genres, and their aired episodes, the series' genres given to episodes
// that have none.
func (h *Handler) buildPool(ctx context.Context, user accounts.User, libraries []library.Item) ([]library.Item, []library.Item) {
	var series []library.Item
	seen := map[accounts.ID]bool{}
	for _, l := range libraries {
		page, err := h.Library.Children(ctx, user, l.ID, 0, poolSeries, "")
		if err != nil {
			h.Logger.Warn("A TV library could not be listed for library-wide episodes", "library", l.Name, "error", err)
			continue
		}
		for _, item := range page.Items {
			if item.Kind == library.KindSeries && !seen[item.ID] {
				seen[item.ID] = true
				series = append(series, item)
			}
		}
	}
	lists := make([][]library.Item, len(series))
	slots := make(chan struct{}, poolWorkers)
	var wg sync.WaitGroup
	for i := range series {
		wg.Add(1)
		slots <- struct{}{}
		go func(i int) {
			defer func() { <-slots; wg.Done() }()
			if len(series[i].Genres) == 0 {
				if full, err := h.Library.Item(ctx, user, series[i].ID); err == nil {
					series[i].Genres = full.Genres
				}
			}
			episodes, err := h.Library.Episodes(ctx, user, series[i].ID, nil)
			if err != nil {
				return
			}
			aired := episodes[:0:0]
			for _, episode := range episodes {
				if !episode.Available {
					continue
				}
				if len(episode.Genres) == 0 {
					episode.Genres = series[i].Genres
				}
				aired = append(aired, episode)
			}
			lists[i] = aired
		}(i)
	}
	wg.Wait()
	var episodes []library.Item
	for _, list := range lists {
		episodes = append(episodes, list...)
	}
	h.Logger.Info("Library-wide episodes ready", "series", len(series), "episodes", len(episodes))
	return series, episodes
}
