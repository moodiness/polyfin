package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
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
//     libraries (see library.Arrivals).
//
// Every row leaves out what the user's listings leave out: the titles
// parental control or blocked genres hide, the titles they hid, and the
// libraries of the server hidden from them, whose catalogs and collections
// make no row.
//
// Each row is a folder of its own, a library without type, by an
// identifier of its own (customRowID): its name, in the web player,
// opens its list, /Items with the row as parentId. A row made a library
// in other apps (homerows.Row.Library) is also listed by /UserViews, as
// the Watchlist view is: apps give it a home row of latest items, its
// first titles. A Because you watched row is a folder for each title it
// follows. The rows' folders are found only after everything else, as an
// identifier nothing else knows.

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

// CustomRowDto is a row of GET /Polyfin/HomeRows: its folder's Id, its
// Name, its Place (Top, AfterResume, AfterNextUp or Bottom) and its first
// Items.
type CustomRowDto struct {
	Id    string
	Name  string
	Place string
	Items []BaseItemDto
}

// customRow is a row as a user sees it: one of a Because you watched row
// for each title it follows (baseline), named after it.
type customRow struct {
	row      homerows.Row
	id       accounts.ID
	name     string
	baseline accounts.ID
}

// customRowID identifies the folder of a row, or of the row of a title a
// Because you watched row follows.
func customRowID(row, baseline accounts.ID) accounts.ID {
	name := "homerow|" + row.String()
	if baseline != (accounts.ID{}) {
		name += "|" + baseline.String()
	}
	id, _ := accounts.ParseID(nameID("view", name))
	return id
}

// customRows lists the rows user sees, in order, a Because you watched row
// as one for each title it follows; none for a request made with an API
// key, and none of them while Similar titles is off.
func (h *Handler) customRows(ctx context.Context, user accounts.User) ([]customRow, error) {
	if h.HomeRows == nil || user.ID == (accounts.ID{}) {
		return nil, nil
	}
	rows, err := h.HomeRows.Shown(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	most := 0
	for _, row := range rows {
		if row.Kind == homerows.KindBecause {
			most = max(most, row.Count)
		}
	}
	var baselines []library.Item
	if most > 0 && h.Accounts.Settings().SimilarTitles {
		if baselines, err = h.recentlyPlayedTitles(ctx, user, most); err != nil {
			return nil, err
		}
	}
	var result []customRow
	for _, row := range rows {
		if row.Kind != homerows.KindBecause {
			result = append(result, customRow{row: row, id: customRowID(row.ID, accounts.ID{}), name: row.Name})
			continue
		}
		for _, title := range baselines[:min(row.Count, len(baselines))] {
			result = append(result, customRow{row: row, id: customRowID(row.ID, title.ID), name: row.Name + " " + title.Name, baseline: title.ID})
		}
	}
	return result, nil
}

// customRowOf returns the row of user whose folder is id.
func (h *Handler) customRowOf(ctx context.Context, user accounts.User, id accounts.ID) (customRow, bool, error) {
	rows, err := h.customRows(ctx, user)
	if err != nil {
		return customRow{}, false, err
	}
	i := slices.IndexFunc(rows, func(row customRow) bool { return row.id == id })
	if i < 0 {
		return customRow{}, false, nil
	}
	return rows[i], true, nil
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
	case homerows.KindCatalog:
		return h.Library.CatalogTitles(ctx, user, row.row.Addon, row.row.CatalogType, row.row.CatalogID, start, count, first)
	case homerows.KindCollection:
		return h.collectionRowTitles(ctx, user, row.row.Collection, start, count, first)
	case homerows.KindBecause:
		// Twice as many leaves room for the played titles left out.
		similar, err := h.Library.Similar(ctx, user, row.baseline, 2*(start+count))
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
	}
	return library.Page{}, library.ErrNotFound
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
	rows, err := h.customRows(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	pages := make([]library.Page, len(rows))
	var group errgroup.Group
	group.SetLimit(customRowLookups)
	for i, row := range rows {
		group.Go(func() error {
			var err error
			pages[i], err = h.customRowTitles(r.Context(), user, row, 0, limit, true)
			// A row that cannot be read is left out; the others show.
			if err != nil && r.Context().Err() == nil {
				h.Logger.Warn("A home row could not be read", "row", row.name, "error", err)
			}
			return nil
		})
	}
	_ = group.Wait()
	if r.Context().Err() != nil {
		return
	}
	var all []library.Item
	for _, page := range pages {
		all = append(all, page.Items...)
	}
	state, err := h.userState(r.Context(), user, all)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	fields := requestedFields(r)
	result := []CustomRowDto{}
	for i, row := range rows {
		items := pages[i].Items[:min(limit, len(pages[i].Items))]
		if len(items) == 0 {
			continue
		}
		result = append(result, CustomRowDto{Id: row.id.String(), Name: row.name, Place: customRowPlaces[row.row.Place],
			Items: h.listDtos(r, user, items, fields, state)})
	}
	writeJSON(w, http.StatusOK, struct{ Items []CustomRowDto }{result})
}

// customRowFolder is the folder of a row.
func customRowFolder(row customRow) library.Item {
	return library.Item{ID: row.id, Kind: library.KindLibrary, Name: row.name}
}

// addCustomRowViews adds to views the folders of the rows user sees that
// are libraries in other apps too.
func (h *Handler) addCustomRowViews(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	rows, err := h.customRows(r.Context(), user)
	if err != nil {
		return nil, err
	}
	var folders []library.Item
	for _, row := range rows {
		if row.row.Library {
			folders = append(folders, customRowFolder(row))
		}
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
	dtos, err := h.folderDtos(r, user, []library.Item{customRowFolder(row)})
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
