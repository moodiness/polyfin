package jellyfin

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// The admin app builds the channels Polyfin airs (see package virtualtv)
// from what Jellyfin apps list: a collection, as a collection row lists
// it, a smart list, a series' episodes, or titles picked by hand.

const (
	// movieRuntime and episodeRuntime are what the schedule gives a movie
	// and an episode whose description gives no runtime.
	movieRuntime   = 90 * time.Minute
	episodeRuntime = 30 * time.Minute
)

// ChannelEntries freezes what a source of a channel holds, as user sees
// it: its movies, and the episodes of its series, seasons and episodes,
// in its order, a series' by season and number. Specials, and episodes
// that have not aired yet, are left out. A collection lists its titles as
// a collection row does; a smart list as its library or collection does.
// It lists at most virtualtv.MaxEntries.
func (h *Handler) ChannelEntries(ctx context.Context, user accounts.User, source virtualtv.Source) ([]virtualtv.Entry, error) {
	if err := source.Validate(); err != nil {
		return nil, err
	}
	var titles []library.Item
	switch source.Kind {
	case virtualtv.SourceCollection:
		page, err := h.collectionRowTitles(ctx, user, source.Items[0].ID, 0, virtualtv.MaxEntries, false)
		if err != nil {
			return nil, err
		}
		titles = page.Items
	case virtualtv.SourceSmartList:
		page, err := h.Library.Whole(ctx, user, source.Items[0].ID, 0, virtualtv.MaxEntries, "")
		if err != nil {
			return nil, err
		}
		titles = page.Items
	default:
		ids := make([]accounts.ID, 0, len(source.Items))
		for _, item := range source.Items {
			ids = append(ids, item.ID)
		}
		items, err := h.Library.Items(ctx, user, ids)
		if err != nil {
			return nil, err
		}
		titles = items
	}
	var entries []virtualtv.Entry
	series := map[accounts.ID]library.Item{}
	for _, title := range titles {
		if len(entries) >= virtualtv.MaxEntries {
			break
		}
		switch title.Kind {
		case library.KindMovie, library.KindVideo:
			entries = append(entries, movieEntry(title))
		case library.KindSeries, library.KindSeason, library.KindEpisode:
			more, err := h.episodeEntries(ctx, user, title, series)
			if err != nil && !errors.Is(err, library.ErrNotFound) {
				return nil, err
			}
			entries = append(entries, more...)
		}
	}
	if len(entries) > virtualtv.MaxEntries {
		entries = entries[:virtualtv.MaxEntries]
	}
	return entries, nil
}

// movieEntry is the entry of a movie, or of a video.
func movieEntry(movie library.Item) virtualtv.Entry {
	return virtualtv.Entry{Item: movie.ID, Title: movie.ID, Kind: virtualtv.EntryMovie, Name: movie.Name, Runtime: cmpDuration(movie.Runtime, movieRuntime),
		Overview: movie.Overview, Image: movie.Images.Primary, Year: movie.ProductionYear, Rating: movie.OfficialRating, Genres: slices.Clip(movie.Genres)}
}

// episodeEntries are the entries of a series, a season or an episode, by
// season and number, those its series kept describes: the series' rating
// and genres are what judge them.
func (h *Handler) episodeEntries(ctx context.Context, user accounts.User, title library.Item, kept map[accounts.ID]library.Item) ([]virtualtv.Entry, error) {
	seriesID := title.SeriesID
	if title.Kind == library.KindSeries {
		seriesID = title.ID
	}
	series, ok := kept[seriesID]
	if !ok {
		var err error
		if series, err = h.Library.Item(ctx, user, seriesID); err != nil {
			return nil, err
		}
		kept[seriesID] = series
	}
	episodes := []library.Item{title}
	if title.Kind != library.KindEpisode {
		var season *accounts.ID
		if title.Kind == library.KindSeason {
			season = &title.ID
		}
		var err error
		if episodes, err = h.Library.Episodes(ctx, user, seriesID, season); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	var entries []virtualtv.Entry
	for _, e := range episodes {
		if title.Kind != library.KindEpisode && (e.ParentIndexNumber == 0 || e.PremiereDate != nil && e.PremiereDate.After(now)) {
			continue
		}
		image := e.Images.Primary
		if image == "" {
			image = series.Images.Primary
		}
		entries = append(entries, virtualtv.Entry{Item: e.ID, Title: series.ID, Kind: virtualtv.EntryEpisode, Name: series.Name, Episode: e.Name,
			Season: e.ParentIndexNumber, Number: e.IndexNumber, Runtime: cmpDuration(e.Runtime, cmpDuration(series.Runtime, episodeRuntime)),
			Overview: e.Overview, Image: image, Year: series.ProductionYear, Rating: series.OfficialRating, Genres: slices.Clip(series.Genres)})
	}
	return entries, nil
}

// ChannelTitles finds the movies and series whose name holds term, for a
// channel built from titles picked by hand or from a series.
func (h *Handler) ChannelTitles(ctx context.Context, user accounts.User, term string, limit int) ([]library.Item, error) {
	return h.Library.Search(ctx, user, term, []library.Kind{library.KindMovie, library.KindSeries}, limit)
}
