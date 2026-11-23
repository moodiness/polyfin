package jellyfin

import (
	"context"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// With playback prepared ahead in the settings, the first rowAheadTitles
// titles of every other home row an app asks for, the latest of a library
// and the rows the administrator added, have their stream lists asked of
// the addons in the background, within each addon's bounds (see
// library.Service.ListRowAhead): a title opened from the row then lists
// its versions at once. For a series, it is the episode its Play starts
// with, as Next Up picks it. Continue Watching and Next Up list more of
// theirs, and prepare some (see listAhead).

const (
	// rowAheadTitles is how many titles of a row's answer are listed
	// ahead: those a screen shows first.
	rowAheadTitles = 3
	// maxRowsAhead bounds the rows whose titles are read at once to be
	// listed ahead: a home page's; those of more are not.
	maxRowsAhead = 16
)

// listRowAhead lists ahead, in the background, the stream lists of the
// first rowAheadTitles items of a home row's answer, with playback
// prepared ahead: its movies and episodes, and the episode each of its
// series plays first (see seriesStarts). It is called once the answer is
// written, which never waits for them, and shares the view the row's
// request, ctx's, built (see library.SharedViews).
func (h *Handler) listRowAhead(ctx context.Context, user accounts.User, items []library.Item) {
	if !h.Accounts.Settings().PrepareAhead {
		return
	}
	var titles, series []library.RowTitle
	for place, item := range items[:min(len(items), rowAheadTitles)] {
		switch item.Kind {
		case library.KindMovie, library.KindEpisode:
			titles = append(titles, library.RowTitle{ID: item.ID, Place: place})
		case library.KindSeries:
			series = append(series, library.RowTitle{ID: item.ID, Place: place})
		}
	}
	if len(titles) == 0 && len(series) == 0 {
		return
	}
	if !h.rowsAhead.start() {
		return
	}
	row := library.SharedViews(ctx)
	go func() {
		defer h.rowsAhead.done()
		defer logPanic(h.Logger, "Listing a home row's titles ahead ended on a panic")
		ctx, cancel := context.WithTimeout(library.Background(stremio.WithOrigin(row, stremio.OriginCatalogs)), preparationTimeout)
		defer cancel()
		h.Library.ListRowAhead(ctx, user, append(titles, h.seriesStarts(ctx, user, series)...))
	}()
}

// rowsAhead counts the home rows whose first titles are read to be listed
// ahead, maxRowsAhead at most.
type rowsAhead struct {
	mu      sync.Mutex
	running int
}

// start counts a row more, unless maxRowsAhead are read.
func (r *rowsAhead) start() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running >= maxRowsAhead {
		return false
	}
	r.running++
	return true
}

// done counts a row read.
func (r *rowsAhead) done() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running--
}

// idle reports whether no row is read.
func (r *rowsAhead) idle() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running == 0
}

// seriesStarts returns the episode each of series plays first, at the
// series' place: the one Next Up offers, the first episode when the user
// played none, a started one included (see nextEpisode). A series all
// played, or whose episodes cannot be listed, has none.
func (h *Handler) seriesStarts(ctx context.Context, user accounts.User, series []library.RowTitle) []library.RowTitle {
	if len(series) == 0 {
		return nil
	}
	ids := make([]accounts.ID, len(series))
	for i, s := range series {
		ids[i] = s.ID
	}
	entries, err := h.UserData.SeriesEpisodes(ctx, user.ID, ids)
	if err != nil {
		h.Logger.Debug("The episodes of a home row's series could not be read to list them ahead", "error", err)
		return nil
	}
	data := make(map[accounts.ID]userdata.Data, len(entries))
	for _, e := range entries {
		data[e.Item] = e.Data
	}
	listed, err := h.Library.PickedEpisodes(ctx, user, ids, func(_ accounts.ID, episodes []library.EpisodeOutline) []int {
		if place, _, ok := nextEpisode(episodes, data, true); ok {
			return []int{place}
		}
		return nil
	})
	if err != nil {
		h.Logger.Debug("The episodes of a home row's series could not be listed to list them ahead", "error", err)
		return nil
	}
	var starts []library.RowTitle
	for _, s := range series {
		if episodes := listed[s.ID]; len(episodes) > 0 {
			starts = append(starts, library.RowTitle{ID: episodes[0].ID, Place: s.Place})
		}
	}
	return starts
}
