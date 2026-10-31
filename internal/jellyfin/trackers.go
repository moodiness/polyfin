package jellyfin

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/trackers"
)

// trackedEvents names playback events for the tracking services.
var trackedEvents = map[playbackEvent]trackers.Event{
	playbackStarted:    trackers.Started,
	playbackProgressed: trackers.Progressed,
	playbackStopped:    trackers.Stopped,
}

// markScope is what a played mark on an item of kind covers.
func markScope(kind library.Kind) trackers.Scope {
	switch kind {
	case library.KindSeries:
		return trackers.ScopeSeries
	case library.KindSeason:
		return trackers.ScopeSeason
	}
	return trackers.ScopeTitle
}

// trackedTitles identifies to the tracking services the movies and
// episodes among items that they can know: episodes by their series'
// identifiers, loaded once per series. Those an anime addon names by
// their Kitsu, MyAnimeList or AniDB identifier go by those the anime
// mapping gives, when it knows them, and keep the entry for AniList and
// MyAnimeList. An episode of a series the mapping numbers in part as
// TVDB's absolute numbering gets its absolute number, from the episodes
// Polyfin lists of its series. It is called in the background.
func (h *Handler) trackedTitles(user accounts.User, items []library.Item) func(context.Context) []trackers.Title {
	return func(ctx context.Context) []trackers.Title {
		series := map[accounts.ID]map[string]string{}
		episodes := map[accounts.ID][]library.Item{}
		var titles []trackers.Title
		for _, item := range items {
			if item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
				continue
			}
			var (
				title trackers.Title
				ok    bool
			)
			anime, isAnime := h.Trackers.Anime(ctx, item.ID, item.StremioID, item.Kind == library.KindMovie)
			switch {
			case isAnime && anime.Identified():
				title, ok = anime, true
			case item.Kind == library.KindMovie:
				title, ok = trackers.Movie(item.ID, item.ProviderIDs)
			default:
				ids, loaded := series[item.SeriesID]
				if !loaded {
					if show, err := h.Library.Item(ctx, user, item.SeriesID); err == nil {
						ids = show.ProviderIDs
					}
					series[item.SeriesID] = ids
				}
				title, ok = trackers.Episode(item.ID, ids, item.ParentIndexNumber, item.IndexNumber)
			}
			switch {
			case ok && isAnime:
				// Found by its own identifiers, it keeps the entry.
				title.Anime = anime.Anime
			case isAnime:
				title, ok = anime, true
			}
			if ok && h.Trackers.NeedsAbsolute(title) {
				listed, loaded := episodes[item.SeriesID]
				if !loaded {
					// Numbered as aired, whatever order the series shows in.
					listed, _ = h.Library.Episodes(library.InAiredOrder(ctx), user, item.SeriesID, nil)
					episodes[item.SeriesID] = listed
				}
				title.Absolute = absoluteNumber(listed, title.Season, title.Number)
			}
			if ok {
				titles = append(titles, title)
			}
		}
		return titles
	}
}

// absoluteNumber is the place of episode number of season among the
// regular episodes of episodes, by season then number, from 1: TVDB's
// absolute numbering. It is 0 for a special or an episode not listed.
func absoluteNumber(episodes []library.Item, season, number int) int {
	if season <= 0 || number <= 0 {
		return 0
	}
	type place struct{ season, number int }
	var regular []place
	for _, e := range episodes {
		if e.ParentIndexNumber > 0 && e.IndexNumber > 0 {
			regular = append(regular, place{e.ParentIndexNumber, e.IndexNumber})
		}
	}
	compare := func(a, b place) int { return cmp.Or(cmp.Compare(a.season, b.season), cmp.Compare(a.number, b.number)) }
	slices.SortFunc(regular, compare)
	regular = slices.Compact(regular)
	i, found := slices.BinarySearchFunc(regular, place{season, number}, compare)
	if !found {
		return 0
	}
	return i + 1
}

// ratedTitle identifies to AniList and MyAnimeList the movie or series
// item a rating is for: by the entry an anime addon names it by, or by its
// own identifiers. It is called in the background.
func (h *Handler) ratedTitle(item library.Item) func(context.Context) (trackers.Title, bool) {
	return func(ctx context.Context) (trackers.Title, bool) {
		if item.Kind == library.KindMovie {
			titles := h.trackedTitles(accounts.User{}, []library.Item{item})(ctx)
			if len(titles) == 0 {
				return trackers.Title{}, false
			}
			return titles[0], true
		}
		if title, ok := trackers.AnimeSeries(item.ID, item.StremioID); ok {
			return title, true
		}
		return trackers.Series(item.ID, item.ProviderIDs)
	}
}

// rated sends the rating item now has to the user's tracking services that
// keep ratings, when it changed: only movies and series are rated there.
func (h *Handler) rated(user accounts.User, item library.Item, was, now *float64) {
	if h.Trackers == nil || item.Kind != library.KindMovie && item.Kind != library.KindSeries {
		return
	}
	if was == nil && now == nil || was != nil && now != nil && *was == *now {
		return
	}
	h.Trackers.Rate(user.ID, trackers.Rating{Score: now, Title: h.ratedTitle(item)})
}
