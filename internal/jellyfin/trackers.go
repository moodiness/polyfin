package jellyfin

import (
	"context"

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
// mapping gives, when it knows them. It is called in the background.
func (h *Handler) trackedTitles(user accounts.User, items []library.Item) func(context.Context) []trackers.Title {
	return func(ctx context.Context) []trackers.Title {
		series := map[accounts.ID]map[string]string{}
		var titles []trackers.Title
		for _, item := range items {
			var (
				title trackers.Title
				ok    bool
			)
			switch item.Kind {
			case library.KindMovie:
				if title, ok = h.Trackers.Anime(ctx, item.ID, item.StremioID, true); !ok {
					title, ok = trackers.Movie(item.ID, item.ProviderIDs)
				}
			case library.KindEpisode:
				if title, ok = h.Trackers.Anime(ctx, item.ID, item.StremioID, false); ok {
					break
				}
				ids, loaded := series[item.SeriesID]
				if !loaded {
					if show, err := h.Library.Item(ctx, user, item.SeriesID); err == nil {
						ids = show.ProviderIDs
					}
					series[item.SeriesID] = ids
				}
				title, ok = trackers.Episode(item.ID, ids, item.ParentIndexNumber, item.IndexNumber)
			}
			if ok {
				titles = append(titles, title)
			}
		}
		return titles
	}
}
