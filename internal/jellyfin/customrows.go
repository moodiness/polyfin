package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/trackers"
)

// Custom home rows.
//
// The administrator adds rows to the web player's home page (Settings ›
// Web player › Home rows, see package homerows), each shown to every user
// or to chosen ones, who may turn it off (My account › Home rows). Polyfin's
// web player script asks GET /Polyfin/HomeRows once the home page is
// drawn, and draws each row at its place. A row lists, for the user asking:
//
//   - a catalog of one of the server's addons: its first page (see
//     library.CatalogTitles);
//   - a collection: one made by users, by premiere date as Jellyfin orders
//     one, else a smart list seen as a collection or an addon's collection,
//     as their own listings order them;
//   - Because you watched: one row for each of the last movies and series
//     the user played, up to the row's count, of the titles similar to it
//     as movie recommendations give them, without those the user played;
//     none while Similar titles is off;
//   - New in your libraries: the titles new in the catalogs of the user's
//     libraries (see library.Arrivals);
//   - Now available and Still waiting: the titles of the user's watchlist
//     they have not started that versions were found for, and those still
//     without (see library.WatchlistAvailable);
//   - Top picks for you: the titles of the user's libraries their taste
//     profile ranks first (see taste.go);
//   - the user's Watchlist, named as the user sees it (see playlistName);
//   - My playlists: one row for each playlist the user may see and made a
//     favorite, up to the row's count, by name as the user sees them,
//     accents aside, each named after its playlist; without the user's
//     Watchlist while a Watchlist row shows to them too;
//   - a playlist the administrator chose, to the users who may see it.
//
// A row of a playlist lists its entries as /Playlists/{id}/Items lists
// them to the user, in the playlist's order (see shownTitles), but for a
// series several of whose episodes it lists, shown once, as the series,
// at its first episode's place (see groupedSeries).
//
// Every row leaves out what the user's listings leave out: the titles
// parental control or blocked genres hide, the titles they hid, and the
// libraries of the server hidden from them, whose catalogs and collections
// make no row.
//
// A Recommended on Trakt row lists the movies and series Trakt recommends
// to the user, as last read (see trackers.Service.Recommended), without
// those they played, as Because you watched leaves them out. It is shown
// only to users with a Trakt connection that works: for the others it is
// no row, no library and no folder.
//
// A row shows only inside its schedule (homerows.Row.ShowsAt), at the time
// h.now tells: outside it, it is no row, no library and no folder. A row
// that is also a library follows its dates only, so that apps, which keep
// the libraries they listed for a while, do not lose it by the hour.
//
// Each row is a folder of its own, a library without type, by an
// identifier of its own (customRowID): its name, in the web player,
// opens its list, /Items with the row as parentId. A row made a library
// in other apps (homerows.Row.Library) is also listed by /UserViews, as a
// library without type: apps give it a home row of latest items, its
// first titles. A row's folder shows each user their own image of it (see
// customRowFolder). A Because you watched row is a folder for each title it
// follows, whose identifier holds the title's. The rows' folders are found
// only after everything else, as an identifier nothing else knows. The
// titles a user played are read for /UserViews only when a Because you
// watched row is a library, and for an identifier only when it names, as
// such a row's folder, a title they played.

const (
	// customRowItems is how many titles a row shows unless asked.
	customRowItems = 20
	// maxCustomRowItems bounds the titles a row shows.
	maxCustomRowItems = 100
	// customRowListing bounds the titles of a row's list.
	customRowListing = 500
	// customRowLookups bounds the rows read at once.
	customRowLookups = 4
)

// customRowPlaces name the places of rows in GET /Polyfin/HomeRows.
var customRowPlaces = map[homerows.Place]string{
	homerows.PlaceTop:    "Top",
	homerows.PlaceResume: "AfterResume",
	homerows.PlaceNextUp: "AfterNextUp",
	homerows.PlaceBottom: "Bottom",
}

// customRowKinds name the kinds of rows in GET /Polyfin/HomeRows.
var customRowKinds = map[homerows.Kind]string{
	homerows.KindCatalog:    "Catalog",
	homerows.KindCollection: "Collection",
	homerows.KindBecause:    "BecauseYouWatched",
	homerows.KindNew:        "New",
	homerows.KindAvailable:  "NowAvailable",
	homerows.KindWaiting:    "StillWaiting",
	homerows.KindTrakt:      "Trakt",
	homerows.KindPicks:      "TopPicks",
	homerows.KindWatchlist:  "Watchlist",
	homerows.KindPlaylists:  "FavoritePlaylists",
	homerows.KindPlaylist:   "Playlist",
}

// CustomRowDto is a row of GET /Polyfin/HomeRows: its folder's Id, its
// Name, its Place (Top, AfterResume, AfterNextUp or Bottom), its Kind
// (Catalog, Collection, BecauseYouWatched, New, NowAvailable, StillWaiting
// or TopPicks) and its first Items.
// A Recommended on Trakt row's Kind is Trakt. A row of a playlist is of
// Kind Watchlist, FavoritePlaylists or Playlist, and PlaylistId names its
// playlist, whose page its name opens.
type CustomRowDto struct {
	Id         string
	Name       string
	Place      string
	Kind       string
	PlaylistId string `json:",omitempty"`
	Items      []BaseItemDto
}

// customRow is a row as a user sees it: one of a Because you watched row
// for each title it follows (baseline), named after it; one of a My
// playlists row for each playlist, named after it. playlist is the
// playlist a row of playlists lists.
type customRow struct {
	row      homerows.Row
	id       accounts.ID
	name     string
	baseline accounts.ID
	playlist accounts.ID
}

// customRowID identifies the folder of a row, or of the row of a title a
// Because you watched row follows: the row's own with the title's mixed
// in, so that the title is found again (see customRowBaseline).
func customRowID(row, baseline accounts.ID) accounts.ID {
	id, _ := accounts.ParseID(nameID("view", "homerow|"+row.String()))
	for i := range id {
		id[i] ^= baseline[i]
	}
	return id
}

// customRowBaseline is the title a Because you watched row's folder id
// follows, if id is one of row's.
func customRowBaseline(row, id accounts.ID) accounts.ID {
	return customRowID(row, id)
}

// customRows lists the rows user sees that keep accepts, every one when
// keep is nil, in order, a Because you watched row as one for each title
// it follows and a My playlists row as one for each playlist; none for a
// request made with an API key, none of them while Similar titles is off,
// and no Recommended on Trakt row without a Trakt connection.
func (h *Handler) customRows(ctx context.Context, user accounts.User, keep func(homerows.Row) bool) ([]customRow, error) {
	if h.HomeRows == nil || user.ID == (accounts.ID{}) {
		return nil, nil
	}
	rows, err := h.HomeRows.Shown(ctx, user.ID, h.now())
	if err != nil {
		return nil, err
	}
	watchlistShown := slices.ContainsFunc(rows, isWatchlistRow)
	if keep != nil {
		rows = slices.DeleteFunc(rows, func(row homerows.Row) bool { return !keep(row) })
	}
	isTrakt := func(row homerows.Row) bool { return row.Kind == homerows.KindTrakt }
	if slices.ContainsFunc(rows, isTrakt) {
		connected, err := h.traktRowShown(ctx, user)
		if err != nil {
			return nil, err
		}
		if !connected {
			rows = slices.DeleteFunc(rows, isTrakt)
		}
	}
	return h.userCustomRows(ctx, user, rows, watchlistShown)
}

func isWatchlistRow(row homerows.Row) bool { return row.Kind == homerows.KindWatchlist }

// traktRowShown reports whether user sees the Recommended on Trakt rows
// shown to them: whether they have a Trakt connection that works.
func (h *Handler) traktRowShown(ctx context.Context, user accounts.User) (bool, error) {
	if h.Trackers == nil {
		return false, nil
	}
	return h.Trackers.Recommends(ctx, user.ID)
}

// userCustomRows is customRows for rows, the user's rows; watchlistShown
// tells whether a Watchlist row shows to them, which keeps their
// Watchlist out of the My playlists rows.
func (h *Handler) userCustomRows(ctx context.Context, user accounts.User, rows []homerows.Row, watchlistShown bool) ([]customRow, error) {
	most, mostLists := 0, 0
	for _, row := range rows {
		switch row.Kind {
		case homerows.KindBecause:
			most = max(most, row.Count)
		case homerows.KindPlaylists:
			mostLists = max(mostLists, row.Count)
		}
	}
	var baselines []library.Item
	if most > 0 && h.Accounts.Settings().SimilarTitles {
		var err error
		if baselines, err = h.recentlyPlayedTitles(ctx, user, most); err != nil {
			return nil, err
		}
	}
	var favorites []playlists.Playlist
	if mostLists > 0 {
		var err error
		if favorites, err = h.favoritePlaylists(ctx, user, watchlistShown); err != nil {
			return nil, err
		}
	}
	var result []customRow
	for _, row := range rows {
		switch row.Kind {
		case homerows.KindBecause:
			for _, title := range baselines[:min(row.Count, len(baselines))] {
				result = append(result, customRow{row: row, id: customRowID(row.ID, title.ID), name: row.Name + " " + title.Name, baseline: title.ID})
			}
		case homerows.KindPlaylists:
			for _, p := range favorites[:min(row.Count, len(favorites))] {
				result = append(result, customRow{row: row, id: customRowID(row.ID, p.ID), name: p.Name, playlist: p.ID})
			}
		case homerows.KindWatchlist:
			p, err := h.Playlists.Watchlist(ctx, user.ID)
			if errors.Is(err, playlists.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			name, err := h.playlistName(ctx, user, p)
			if err != nil {
				return nil, err
			}
			result = append(result, customRow{row: row, id: customRowID(row.ID, accounts.ID{}), name: name, playlist: p.ID})
		case homerows.KindPlaylist:
			p, err := h.Playlists.Get(ctx, row.Playlist)
			if errors.Is(err, playlists.ErrNotFound) || err == nil && !p.Visible(user.ID) {
				continue
			}
			if err != nil {
				return nil, err
			}
			result = append(result, customRow{row: row, id: customRowID(row.ID, accounts.ID{}), name: row.Name, playlist: p.ID})
		default:
			result = append(result, customRow{row: row, id: customRowID(row.ID, accounts.ID{}), name: row.Name})
		}
	}
	return result, nil
}

// favoritePlaylists lists the playlists user may see and made a favorite,
// named as they see them and in that order, accents aside; without those
// that have no entry, which would make no row, and without their own
// Watchlist when withoutWatchlist is set.
func (h *Handler) favoritePlaylists(ctx context.Context, user accounts.User, withoutWatchlist bool) ([]playlists.Playlist, error) {
	visible, err := h.Playlists.Visible(ctx, user.ID)
	if err != nil || len(visible) == 0 {
		return nil, err
	}
	ids := make([]accounts.ID, len(visible))
	for i, p := range visible {
		ids[i] = p.ID
	}
	data, err := h.UserData.Get(ctx, user.ID, ids)
	if err != nil {
		return nil, err
	}
	visible = slices.DeleteFunc(visible, func(p playlists.Playlist) bool {
		return !data[p.ID].Favorite || len(p.Entries) == 0 || withoutWatchlist && p.Watchlist && p.Owner == user.ID
	})
	if err := h.namePlaylists(ctx, user, visible); err != nil {
		return nil, err
	}
	return visible, nil
}

// customRowOf returns the row of user whose folder is id. The titles the
// user played are read only when id names one of them for a Because you
// watched row, and their favorite playlists only when it names a playlist
// they may see for a My playlists row.
func (h *Handler) customRowOf(ctx context.Context, user accounts.User, id accounts.ID) (customRow, bool, error) {
	if h.HomeRows == nil || user.ID == (accounts.ID{}) {
		return customRow{}, false, nil
	}
	rows, err := h.HomeRows.Shown(ctx, user.ID, h.now())
	if err != nil {
		return customRow{}, false, err
	}
	watchlistShown := slices.ContainsFunc(rows, isWatchlistRow)
	var because, lists []homerows.Row
	var baselines []accounts.ID
	similar := h.Accounts.Settings().SimilarTitles
	for _, row := range rows {
		switch {
		case row.Kind == homerows.KindPlaylists:
			p, err := h.Playlists.Get(ctx, customRowBaseline(row.ID, id))
			if err != nil && !errors.Is(err, playlists.ErrNotFound) {
				return customRow{}, false, err
			}
			if err == nil && p.Visible(user.ID) {
				lists = append(lists, row)
			}
		case row.Kind != homerows.KindBecause:
			if customRowID(row.ID, accounts.ID{}) != id {
				continue
			}
			shown := true
			switch row.Kind {
			case homerows.KindTrakt:
				if shown, err = h.traktRowShown(ctx, user); err != nil {
					return customRow{}, false, err
				}
			case homerows.KindWatchlist, homerows.KindPlaylist:
				result, err := h.userCustomRows(ctx, user, []homerows.Row{row}, watchlistShown)
				if err != nil || len(result) == 0 {
					return customRow{}, false, err
				}
				return result[0], true, nil
			}
			return customRow{row: row, id: id, name: row.Name}, shown, nil
		case similar:
			because, baselines = append(because, row), append(baselines, customRowBaseline(row.ID, id))
		}
	}
	if len(because) > 0 {
		played, err := h.playedTitles(ctx, user, baselines)
		if err != nil {
			return customRow{}, false, err
		}
		because = slices.DeleteFunc(because, func(row homerows.Row) bool { return !played[customRowBaseline(row.ID, id)] })
	}
	if len(because)+len(lists) == 0 {
		return customRow{}, false, nil
	}
	result, err := h.userCustomRows(ctx, user, append(lists, because...), watchlistShown)
	if err != nil {
		return customRow{}, false, err
	}
	i := slices.IndexFunc(result, func(row customRow) bool { return row.id == id })
	if i < 0 {
		return customRow{}, false, nil
	}
	return result[i], true, nil
}

// playedTitles reports which of ids the user played, as an item or as a
// series one of whose episodes they played.
func (h *Handler) playedTitles(ctx context.Context, user accounts.User, ids []accounts.ID) (map[accounts.ID]bool, error) {
	data, err := h.UserData.Get(ctx, user.ID, ids)
	if err != nil {
		return nil, err
	}
	counts, err := h.UserData.EpisodeCounts(ctx, user.ID, ids)
	if err != nil {
		return nil, err
	}
	played := make(map[accounts.ID]bool, len(ids))
	for _, id := range ids {
		played[id] = data[id].Played || counts[id].Played > 0
	}
	return played, nil
}

// recentlyPlayedTitles lists up to count of the movies and series the user
// played last, most recent first, a series counting from its last episode
// played, among those they still reach and did not hide.
func (h *Handler) recentlyPlayedTitles(ctx context.Context, user accounts.User, count int) ([]library.Item, error) {
	played, err := h.UserData.Played(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	hidden, err := h.Library.HiddenTitles(ctx, user)
	if err != nil {
		return nil, err
	}
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, entry := range played {
		id := entry.Item
		if entry.Series != (accounts.ID{}) {
			id = entry.Series
		}
		if !seen[id] && !hidden[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	var titles []library.Item
	// Items of other kinds, or no longer reached, are skipped: a few more
	// than needed are described at a time.
	for len(ids) > 0 && len(titles) < count {
		batch := ids[:min(len(ids), 2*count)]
		ids = ids[len(batch):]
		items, err := h.Library.Items(ctx, user, batch)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if (item.Kind == library.KindMovie || item.Kind == library.KindSeries) && len(titles) < count {
				titles = append(titles, item)
			}
		}
	}
	return titles, nil
}

// customRowTitles lists a row's titles from start, for user: its first
// ones as a home row shows them when first is set, else as its list does.
// A row whose catalog or collection the user does not reach lists none.
func (h *Handler) customRowTitles(ctx context.Context, user accounts.User, row customRow, start, count int, first bool) (library.Page, error) {
	page, err := h.readCustomRow(ctx, user, row, start, count, first)
	if errors.Is(err, library.ErrNotFound) {
		return library.Page{}, nil
	}
	return page, err
}

func (h *Handler) readCustomRow(ctx context.Context, user accounts.User, row customRow, start, count int, first bool) (library.Page, error) {
	switch row.row.Kind {
	case homerows.KindTrakt:
		return h.traktRowTitles(ctx, user, start, count, first)
	case homerows.KindCatalog:
		return h.Library.CatalogTitles(ctx, user, row.row.Addon, row.row.CatalogType, row.row.CatalogID, start, count, first)
	case homerows.KindCollection:
		return h.collectionRowTitles(ctx, user, row.row.Collection, start, count, first)
	case homerows.KindBecause:
		// Twice as many leaves room for the played titles left out.
		similar, err := h.similarTitles(ctx, user, row.baseline, 2*(start+count))
		if err != nil {
			return library.Page{}, err
		}
		state, err := h.userState(ctx, user, similar)
		if err != nil {
			return library.Page{}, err
		}
		similar = slices.DeleteFunc(similar, func(item library.Item) bool { return state.of(item).Played })
		return pageOfItems(similar, start, count), nil
	case homerows.KindNew:
		return h.Library.Arrivals(ctx, user, start, count, first)
	case homerows.KindAvailable:
		return h.Library.WatchlistAvailable(ctx, user, start, count, first)
	case homerows.KindWaiting:
		return h.Library.WatchlistWaiting(ctx, user, start, count, first)
	case homerows.KindPicks:
		return h.topPicksRow(ctx, user, start, count)
	case homerows.KindWatchlist, homerows.KindPlaylists, homerows.KindPlaylist:
		_, shown, ok, err := h.shownTitles(ctx, user, row.playlist)
		if err != nil || !ok {
			return library.Page{}, cmp.Or(err, library.ErrNotFound)
		}
		grouped, err := h.groupedSeries(ctx, user, shown)
		if err != nil {
			return library.Page{}, err
		}
		return pageOfItems(grouped, start, count), nil
	}
	return library.Page{}, library.ErrNotFound
}

// groupedSeries is the titles of a row of a playlist, shown, with each
// series several of whose episodes it lists shown once, as the series, at
// its first episode's place; an episode alone of its series stays. A
// series user no longer reaches leaves its episodes as they are.
func (h *Handler) groupedSeries(ctx context.Context, user accounts.User, shown []library.Item) ([]library.Item, error) {
	episodes := map[accounts.ID]int{}
	var series []accounts.ID
	for _, title := range shown {
		if title.Kind != library.KindEpisode || title.SeriesID == (accounts.ID{}) {
			continue
		}
		if episodes[title.SeriesID]++; episodes[title.SeriesID] == 2 {
			series = append(series, title.SeriesID)
		}
	}
	if len(series) == 0 {
		return shown, nil
	}
	items, err := h.Library.Items(ctx, user, series)
	if err != nil {
		return nil, err
	}
	described := make(map[accounts.ID]library.Item, len(items))
	for _, item := range items {
		if item.Kind == library.KindSeries {
			described[item.ID] = item
		}
	}
	result := make([]library.Item, 0, len(shown))
	placed := map[accounts.ID]bool{}
	for _, title := range shown {
		s, ok := described[title.SeriesID]
		if title.Kind != library.KindEpisode || !ok || episodes[title.SeriesID] < 2 {
			result = append(result, title)
		} else if !placed[s.ID] {
			placed[s.ID] = true
			result = append(result, s)
		}
	}
	return result, nil
}

// traktRowTitles lists the titles of a Recommended on Trakt row: those
// Trakt recommends to user, as last read, without those they played;
// ErrNotFound without a Trakt connection that works.
func (h *Handler) traktRowTitles(ctx context.Context, user accounts.User, start, count int, first bool) (library.Page, error) {
	if h.Trackers == nil {
		return library.Page{}, library.ErrNotFound
	}
	ids, err := h.Trackers.Recommended(ctx, user.ID)
	if errors.Is(err, trackers.ErrNotConnected) {
		return library.Page{}, library.ErrNotFound
	}
	if err != nil || len(ids) == 0 {
		return library.Page{}, err
	}
	played, err := h.playedTitles(ctx, user, ids)
	if err != nil {
		return library.Page{}, err
	}
	ids = slices.DeleteFunc(ids, func(id accounts.ID) bool { return played[id] })
	return h.Library.Recommended(ctx, user, ids, start, count, first)
}

// collectionRowTitles lists the titles of a collection row: those of a
// collection made by users that user may see, by premiere date; else
// those of a smart list or an addon's collection, as their listings give
// them. An addon's collection listed by a library of the server hidden
// from the user is ErrNotFound.
func (h *Handler) collectionRowTitles(ctx context.Context, user accounts.User, id accounts.ID, start, count int, first bool) (library.Page, error) {
	c, err := h.Collections.Get(ctx, id)
	if err == nil {
		titles, err := h.collectionTitles(ctx, user, c)
		if err != nil {
			return library.Page{}, err
		}
		listed := titles[0]
		slices.SortStableFunc(listed, func(a, b library.Item) int { return premiere(a).Compare(premiere(b)) })
		return pageOfItems(listed, start, count), nil
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return library.Page{}, err
	}
	collection, err := h.Library.Item(ctx, user, id)
	if err != nil {
		return library.Page{}, err
	}
	if collection.Kind != library.KindCollection || slices.Contains(user.HiddenLibraries, collection.ParentID) {
		return library.Page{}, library.ErrNotFound
	}
	if first {
		return h.Library.Latest(ctx, user, id, start+count)
	}
	return h.Library.Children(ctx, user, id, start, count, "")
}

// pageOfItems is the window [start, start+count) of items.
func pageOfItems(items []library.Item, start, count int) library.Page {
	from, to := bounds(len(items), start, count)
	return library.Page{Items: items[from:to], Total: len(items), More: to < len(items)}
}

// customRowsAnswer answers GET /Polyfin/HomeRows: the rows the user sees,
// in order, each with its first titles, as many as limit asks (20 unless
// asked, 100 at most); the rows without any are left out. Fields asks for
// the titles' fields, as listings do.
func (h *Handler) customRowsAnswer(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	limit, limited := b.int32(r, "limit")
	if !limited || limit <= 0 {
		limit = customRowItems
	}
	limit = min(limit, maxCustomRowItems)
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	rows, titles, err := h.firstCustomRowTitles(r.Context(), user, limit)
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var all []library.Item
	for _, items := range titles {
		all = append(all, items...)
	}
	state, err := h.userState(r.Context(), user, all)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	fields := requestedFields(r)
	result := []CustomRowDto{}
	for i, row := range rows {
		dto := CustomRowDto{Id: row.id.String(), Name: row.name, Place: customRowPlaces[row.row.Place],
			Kind: customRowKinds[row.row.Kind], Items: h.listDtos(r, user, titles[i], fields, state)}
		if row.playlist != (accounts.ID{}) {
			dto.PlaylistId = row.playlist.String()
		}
		result = append(result, dto)
	}
	writeJSON(w, http.StatusOK, struct{ Items []CustomRowDto }{result})
}

// firstCustomRowTitles lists the rows user sees, in order, each with its
// first titles, up to limit. The rows without any are left out, and so
// are those that cannot be read, which are logged.
func (h *Handler) firstCustomRowTitles(ctx context.Context, user accounts.User, limit int) ([]customRow, [][]library.Item, error) {
	rows, err := h.customRows(ctx, user, nil)
	if err != nil {
		return nil, nil, err
	}
	pages := make([]library.Page, len(rows))
	var group errgroup.Group
	group.SetLimit(customRowLookups)
	for i, row := range rows {
		group.Go(func() error {
			var err error
			pages[i], err = h.customRowTitles(ctx, user, row, 0, limit, true)
			// A row that cannot be read is left out; the others show.
			if err != nil && ctx.Err() == nil {
				h.Logger.Warn("A home row could not be read", "row", row.name, "error", err)
			}
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var shown []customRow
	var titles [][]library.Item
	for i, row := range rows {
		if items := pages[i].Items[:min(limit, len(pages[i].Items))]; len(items) > 0 {
			shown, titles = append(shown, row), append(titles, items)
		}
	}
	return shown, titles, nil
}

// HomeRowPreview is a row of a user's home page in the web player, as GET
// /Polyfin/HomeRows answers it for them: its folder's ID, its Name, the
// Row it comes from, and its first Titles.
type HomeRowPreview struct {
	ID     accounts.ID
	Name   string
	Row    homerows.Row
	Titles []library.Item
}

// PreviewHomeRows lists the rows user sees now on the web player's home
// page, in order, each with its first titles, as GET /Polyfin/HomeRows
// answers them to the web player signed in as user (Settings › Web player
// › Home rows › Preview as).
func (h *Handler) PreviewHomeRows(ctx context.Context, user accounts.User) ([]HomeRowPreview, error) {
	rows, titles, err := h.firstCustomRowTitles(ctx, user, customRowItems)
	if err != nil {
		return nil, err
	}
	result := make([]HomeRowPreview, 0, len(rows))
	for i, row := range rows {
		result = append(result, HomeRowPreview{ID: row.id, Name: row.name, Row: row.row, Titles: titles[i]})
	}
	return result, nil
}

// customRowFolder is the folder of a row, with its image as user sees it,
// wide as a library's: for a row of a playlist the playlist's, as its page
// shows it (see playlistDtos), the Playlists emblem when it has nothing to
// draw; for another row a mosaic of its first titles, a series standing
// for its episodes (see library.Service.HomeRowImage).
func (h *Handler) customRowFolder(ctx context.Context, user accounts.User, row customRow) (library.Item, error) {
	item := library.Item{ID: row.id, Kind: library.KindLibrary, Name: row.name, WidePrimary: true}
	if row.playlist == (accounts.ID{}) {
		item.Images.Primary = h.Library.HomeRowImage(ctx, user, row.id, row.row.ID, accounts.ID{}, row.name, func() []accounts.ID {
			page, err := h.customRowTitles(ctx, user, row, 0, customRowItems, true)
			if err != nil {
				return nil
			}
			return seriesFirst(page.Items)
		})
		return item, nil
	}
	p, err := h.Playlists.Get(ctx, row.playlist)
	if errors.Is(err, playlists.ErrNotFound) {
		return item, nil
	}
	if err != nil {
		return library.Item{}, err
	}
	name, err := h.playlistName(ctx, user, p)
	if err != nil {
		return library.Item{}, err
	}
	shown := h.Library.Overridden(library.Item{ID: p.ID, Kind: library.KindMusicPlaylist, Name: name})[0]
	// An image uploaded for the playlist is served for the folder too (see
	// customRowUpload), but for a Watchlist row's, whose playlist differs
	// with whoever sees it.
	if shown.Images.Primary != "" && row.row.Kind != homerows.KindWatchlist {
		item.Images.Primary, item.WidePrimary = shown.Images.Primary, shown.WidePrimary
		return item, nil
	}
	item.Images.Primary = h.Library.HomeRowImage(ctx, user, row.id, row.row.ID, p.ID, shown.Name, func() []accounts.ID {
		titles, err := h.playlistContents(ctx, user, p)
		if err != nil {
			return nil
		}
		entries, err := h.shownEntries(ctx, user, p, titles)
		if err != nil {
			return nil
		}
		return posterTitles(entries, titles)
	})
	if item.Images.Primary == "" {
		item.Images.Primary = library.EmblemImage(viewEmblems[playlistsViewID]).URL()
	}
	return item, nil
}

// seriesFirst lists the IDs of titles, in order, each once, an episode
// giving its series'.
func seriesFirst(titles []library.Item) []accounts.ID {
	ids := make([]accounts.ID, 0, len(titles))
	for _, title := range titles {
		id := title.ID
		if title.Kind == library.KindEpisode && title.SeriesID != (accounts.ID{}) {
			id = title.SeriesID
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// customRowUpload returns the URL of the image of type imageType uploaded
// for the playlist whose row's folder is id: a Playlist row's, or one of
// a My playlists row's. Image requests come without a user: the rows are
// read only for an image nothing else answers.
func (h *Handler) customRowUpload(ctx context.Context, id accounts.ID, imageType string) (string, bool) {
	if h.HomeRows == nil {
		return "", false
	}
	rows, err := h.HomeRows.Rows(ctx)
	if err != nil {
		return "", false
	}
	for _, row := range rows {
		switch {
		case row.Kind == homerows.KindPlaylist && customRowID(row.ID, accounts.ID{}) == id:
			return h.Library.UploadedArtwork(row.Playlist, imageType)
		case row.Kind == homerows.KindPlaylists:
			if url, ok := h.Library.UploadedArtwork(customRowBaseline(row.ID, id), imageType); ok {
				return url, true
			}
		}
	}
	return "", false
}

// addCustomRowViews adds to views the folders of the rows user sees that
// are libraries in other apps too: only these rows are read.
func (h *Handler) addCustomRowViews(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	rows, err := h.customRows(r.Context(), user, func(row homerows.Row) bool { return row.Library })
	if err != nil {
		return nil, err
	}
	folders := make([]library.Item, 0, len(rows))
	for _, row := range rows {
		folder, err := h.customRowFolder(r.Context(), user, row)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	if len(folders) == 0 {
		return views, nil
	}
	dtos, err := h.folderDtos(r, user, folders)
	if err != nil {
		return nil, err
	}
	return append(views, dtos...), nil
}

// describeCustomRow answers the description of a row's folder. It reports
// whether id named one.
func (h *Handler) describeCustomRow(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	row, found, err := h.customRowOf(r.Context(), user, id)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if !found {
		return false
	}
	folder, err := h.customRowFolder(r.Context(), user, row)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	dtos, err := h.folderDtos(r, user, []library.Item{folder})
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, dtos[0])
	return true
}

// customRowAncestors answers the folders above a row's folder: none, as
// above a library. It reports whether id named one.
func (h *Handler) customRowAncestors(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	_, found, err := h.customRowOf(r.Context(), user, id)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if found {
		writeJSON(w, http.StatusOK, []BaseItemDto{})
	}
	return found
}

// customRowListing answers a listing of a row's folder: its list, in its
// own order, whatever sort the app asks, the request's type filters
// applying. It reports whether parent named one.
func (h *Handler) customRowListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, start, limit int) bool {
	row, found, err := h.customRowOf(r.Context(), user, parent)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if !found {
		return false
	}
	start = max(start, 0)
	if limit < 0 || start+limit > customRowListing {
		limit = max(0, customRowListing-start)
	}
	page, err := h.customRowTitles(r.Context(), user, row, start, limit, false)
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	keep := itemTypeFilter(r)
	items := slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) })
	h.writeItems(w, r, user, items, start, min(page.Total, customRowListing)-(len(page.Items)-len(items)))
	return true
}

// customRowLatest answers the latest items of a row's folder, as apps ask
// for a library's home row: its first titles. It reports whether parent
// named one.
func (h *Handler) customRowLatest(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, limit int) bool {
	row, found, err := h.customRowOf(r.Context(), user, parent)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if !found {
		return false
	}
	page, err := h.customRowTitles(r.Context(), user, row, 0, min(limit, maxCustomRowItems), true)
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	keep := itemTypeFilter(r)
	items := slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) })
	dtos, err := h.dtos(r, user, items, requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, dtos)
	return true
}

// HomeRowCollection is a collection a collection row may list: one made
// by users (Source "users"), a smart list seen as a collection ("smart")
// or an addon's collection ("addon").
type HomeRowCollection struct {
	ID     accounts.ID
	Name   string
	Source string
}

// HomeRowCollections lists the collections the admin app offers for a
// collection row, as user sees them: those made by users, by name, the
// smart lists seen as collections, then the collections of the collection
// libraries, each once.
func (h *Handler) HomeRowCollections(ctx context.Context, user accounts.User) ([]HomeRowCollection, error) {
	made, err := h.Collections.All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]HomeRowCollection, 0, len(made))
	for _, c := range made {
		result = append(result, HomeRowCollection{ID: c.ID, Name: c.Name, Source: "users"})
	}
	smart, err := h.Library.SmartCollections(ctx, user)
	if err != nil {
		return nil, err
	}
	for _, item := range smart {
		result = append(result, HomeRowCollection{ID: item.ID, Name: item.Name, Source: "smart"})
	}
	fromAddons, err := h.addonCollections(ctx, user)
	if err != nil {
		return nil, err
	}
	for _, item := range fromAddons {
		result = append(result, HomeRowCollection{ID: item.ID, Name: item.Name, Source: "addon"})
	}
	return result, nil
}

// HomeRowPlaylist is a playlist a playlist row may list: its owner's
// name, and who may see it: everyone (Access "public"), the users it is
// shared with too ("shared"), or its owner only ("private").
type HomeRowPlaylist struct {
	ID        accounts.ID
	Name      string
	OwnerName string
	Access    string
}

// HomeRowPlaylists lists the playlists the admin app offers for a
// playlist row: every playlist of the server but the Watchlists, by name.
func (h *Handler) HomeRowPlaylists(ctx context.Context) ([]HomeRowPlaylist, error) {
	all, err := h.Playlists.All(ctx)
	if err != nil {
		return nil, err
	}
	owners := map[accounts.ID]string{}
	result := make([]HomeRowPlaylist, 0, len(all))
	for _, p := range all {
		if p.Watchlist {
			continue
		}
		owner, known := owners[p.Owner]
		if !known {
			found, err := h.Accounts.User(ctx, p.Owner)
			if err != nil {
				return nil, err
			}
			owner, owners[p.Owner] = found.Name, found.Name
		}
		access := "private"
		switch {
		case p.OpenAccess:
			access = "public"
		case len(p.Shares) > 0:
			access = "shared"
		}
		result = append(result, HomeRowPlaylist{ID: p.ID, Name: p.Name, OwnerName: owner, Access: access})
	}
	return result, nil
}

// WatchlistRowName is the name of user's Watchlist as they see it, which
// names a Watchlist row for them; empty once they deleted it.
func (h *Handler) WatchlistRowName(ctx context.Context, user accounts.User) (string, error) {
	p, err := h.Playlists.Watchlist(ctx, user.ID)
	if errors.Is(err, playlists.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h.playlistName(ctx, user, p)
}

// SeesPlaylist reports whether user may see playlist id, without which a
// Playlist row of it is no row of theirs.
func (h *Handler) SeesPlaylist(ctx context.Context, user accounts.User, id accounts.ID) (bool, error) {
	p, err := h.Playlists.Get(ctx, id)
	if errors.Is(err, playlists.ErrNotFound) {
		return false, nil
	}
	return err == nil && p.Visible(user.ID), err
}
