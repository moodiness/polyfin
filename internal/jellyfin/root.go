package jellyfin

import (
	"context"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/taste"
)

// The user's root folder, item counts and suggestions: what Jellyfin
// derives from the whole of a user's libraries.

// rootFolderID identifies the root folder above every user's views, the
// same on every server, as Jellyfin's is the same for all its users.
var rootFolderID, _ = accounts.ParseID(nameID("folder", "root"))

// rootFolderItem is the root folder, Jellyfin's UserRootFolder.
func rootFolderItem() library.Item {
	return library.Item{ID: rootFolderID, Kind: library.KindLibrary, Name: "Media Folders"}
}

func (h *Handler) rootRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Items/Root", h.rootFolder)
	signedIn(http.MethodGet, "/Users/{userId}/Items/Root", h.rootFolder)
	signedIn(http.MethodGet, "/Items/Counts", h.itemCounts)
	signedIn(http.MethodGet, "/Items/Suggestions", h.suggestions)
	signedIn(http.MethodGet, "/Users/{userId}/Suggestions", h.suggestions)
}

func (h *Handler) rootFolder(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, notFoundProblem)
	if ok {
		h.writeRootFolder(w, r, user)
	}
}

// writeRootFolder describes the user's root folder, which holds their
// views, as Jellyfin does.
func (h *Handler) writeRootFolder(w http.ResponseWriter, r *http.Request, user accounts.User) {
	views, err := h.userViews(r, user, false)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	item := rootFolderItem()
	state, err := h.userState(r.Context(), user, []library.Item{item})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dto := h.newItemDto(item, nil, true, state)
	dto.Type, dto.CollectionType, dto.DateLastMediaAdded = "UserRootFolder", "", nil
	dto.ChildCount = new(len(views))
	writeJSON(w, http.StatusOK, dto)
}

// rootChildren lists the root folder's children: the user's views.
func (h *Handler) rootChildren(w http.ResponseWriter, r *http.Request, user accounts.User, start, limit int) {
	views, err := h.userViews(r, user, false)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pageOf(views, start, limit, len(views)))
}

// firstPages reads the first page of each of the user's libraries, as
// their home screen does. A library whose addon does not answer is left
// out.
func (h *Handler) firstPages(ctx context.Context, user accounts.User) ([]library.Page, error) {
	libraries, err := h.Library.Libraries(ctx, user)
	if err != nil {
		return nil, err
	}
	pages := make([]library.Page, len(libraries))
	var g errgroup.Group
	g.SetLimit(4)
	for i, l := range libraries {
		g.Go(func() error {
			page, err := h.Library.Children(ctx, user, l.ID, 0, defaultPageSize, "")
			if err != nil {
				h.Logger.Debug("A library could not be read", "library", l.ID.String(), "error", err)
				return nil
			}
			pages[i] = page
			return nil
		})
	}
	_ = g.Wait()
	return pages, ctx.Err()
}

// ItemCounts is Jellyfin's count of the items of each type in a user's
// libraries.
type ItemCounts struct {
	MovieCount      int
	SeriesCount     int
	EpisodeCount    int
	ArtistCount     int
	ProgramCount    int
	TrailerCount    int
	SongCount       int
	AlbumCount      int
	MusicVideoCount int
	BoxSetCount     int
	BookCount       int
	ItemCount       int
}

// add counts an item of kind.
func (c *ItemCounts) add(kind library.Kind) {
	switch kind {
	case library.KindMovie:
		c.MovieCount++
	case library.KindSeries:
		c.SeriesCount++
	case library.KindEpisode:
		c.EpisodeCount++
	case library.KindArtist:
		c.ArtistCount++
	case library.KindProgram:
		c.ProgramCount++
	case library.KindTrack:
		c.SongCount++
	case library.KindAlbum:
		c.AlbumCount++
	case library.KindCollection:
		c.BoxSetCount++
	case library.KindAudiobook:
		c.BookCount++
	}
	c.ItemCount++
}

// itemCounts counts the items of the user's libraries by type, the user's
// favorites with isFavorite. A catalog's size is unknown until it is read
// through, so libraries count as the pages of their named pages do (see
// writeNamedPage): their first page of items, plus one when more follow,
// as far as Polyfin keeps them: counts are informational, and asking the
// addons would make the screens that show them wait (see
// library.KnownPages). Favorites are counted exactly, but the titles the
// user hid, which their listing leaves out.
func (h *Handler) itemCounts(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	favorite, favoriteSet := b.bool(r, "isFavorite")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	var counts ItemCounts
	if favoriteSet && favorite {
		entries, err := h.UserData.Favorites(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		ids := make([]accounts.ID, 0, len(entries))
		for _, entry := range entries {
			ids = append(ids, entry.Item)
		}
		items, err := h.Library.Items(r.Context(), user, ids)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		hidden, err := h.Library.HiddenTitles(r.Context(), user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, item := range hidden.Without(items) {
			counts.add(item.Kind)
		}
		writeJSON(w, http.StatusOK, counts)
		return
	}
	pages, err := h.Library.KnownPages(r.Context(), user, defaultPageSize)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	var favorites map[accounts.ID]bool
	if favoriteSet {
		entries, err := h.UserData.Favorites(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		favorites = map[accounts.ID]bool{}
		for _, entry := range entries {
			favorites[entry.Item] = true
		}
	}
	for _, page := range pages {
		for _, item := range page.Items {
			if !favorites[item.ID] {
				counts.add(item.Kind)
			}
		}
		if page.More && len(page.Items) > 0 {
			counts.add(page.Items[len(page.Items)-1].Kind)
		}
	}
	writeJSON(w, http.StatusOK, counts)
}

// suggestions are items picked at random among the user's libraries, of
// the types and media types asked, as Jellyfin suggests them: here among
// the first page of each library. Above the confidence gate of the user's
// taste profile, the titles it ranks first come first (see taste.go).
func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	countAll, _ := b.bool(r, "enableTotalRecordCount")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	types, media := listQuery(r, "type"), listQuery(r, "mediaType")
	matches := func(list []string, value string) bool {
		return len(list) == 0 || slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, value) })
	}
	libraries, err := h.suggestionCandidates(r.Context(), user, func(item library.Item) bool {
		return matches(types, itemTypes[item.Kind]) && matches(media, mediaType(item.Kind))
	})
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	pool := slices.Concat(libraries...)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	// Above the confidence gate, the user's taste orders them, the titles
	// it ranks the same in a random order.
	pool = taste.Order(h.tasteProfile(r.Context(), user), pool, taste.TitleOf)
	from := min(max(start, 0), len(pool))
	to := len(pool)
	if limit >= 0 {
		to = min(from+limit, len(pool))
	}
	items, err := h.dtos(r, user, pool[from:to], requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	total := len(items)
	if countAll {
		total = len(pool)
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: total, StartIndex: max(start, 0)})
}

// suggestionCandidates is, for each of the user's libraries, the titles
// keep keeps on its first page, in the catalog's order, each title in the
// first library that has it.
func (h *Handler) suggestionCandidates(ctx context.Context, user accounts.User, keep func(library.Item) bool) ([][]library.Item, error) {
	pages, err := h.firstPages(ctx, user)
	if err != nil {
		return nil, err
	}
	seen := map[accounts.ID]bool{}
	libraries := make([][]library.Item, 0, len(pages))
	for _, page := range pages {
		var titles []library.Item
		for _, item := range page.Items {
			if !seen[item.ID] && keep(item) {
				seen[item.ID] = true
				titles = append(titles, item)
			}
		}
		if len(titles) > 0 {
			libraries = append(libraries, titles)
		}
	}
	return libraries, nil
}

// topPickWindow is how many of a library's next titles each round of
// topPicks picks among.
const topPickWindow = 2

// topPicks orders the libraries' titles, each in its catalog's order, round
// by round: each round, the libraries in a new random order each give one of
// their next topPickWindow titles at random. A library's first titles, its
// most popular in a popular catalog, come first, the libraries mixed, and
// the picks vary between calls. It takes the libraries' slices over.
func topPicks(libraries [][]library.Item) []library.Item {
	var picks []library.Item
	for {
		libraries = slices.DeleteFunc(libraries, func(titles []library.Item) bool { return len(titles) == 0 })
		if len(libraries) == 0 {
			return picks
		}
		rand.Shuffle(len(libraries), func(i, j int) { libraries[i], libraries[j] = libraries[j], libraries[i] })
		for i, titles := range libraries {
			n := rand.IntN(min(topPickWindow, len(titles)))
			picks = append(picks, titles[n])
			libraries[i] = slices.Delete(titles, n, n+1)
		}
	}
}

// randomTitles reports whether a listing without a folder asks for titles
// at random, as jellyfin-web's search page asks for its Suggestions before
// anything is typed: through the folders, sorted at random, favorite and
// liked titles first or not, of movies, series or music artists only.
func randomTitles(r *http.Request) bool {
	oneOf := func(names ...string) func(string) bool {
		return func(value string) bool {
			return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, value) })
		}
	}
	isNot := func(names ...string) func(string) bool {
		in := oneOf(names...)
		return func(value string) bool { return !in(value) }
	}
	recursive, _ := boolQuery(r, "recursive")
	sorts, include := listQuery(r, "sortBy"), listQuery(r, "includeItemTypes")
	return recursive && slices.ContainsFunc(sorts, oneOf("Random")) && !slices.ContainsFunc(sorts, isNot("Random", "IsFavoriteOrLiked")) &&
		len(include) > 0 && !slices.ContainsFunc(include, isNot("Movie", "Series", "MusicArtist"))
}

// randomListing answers a listing of titles at random (see randomTitles)
// with the top titles of the user's libraries (see topPicks), the user's
// favorite and liked ones first when it sorts by them. Jellyfin picks at
// random among everything, which a remote catalog cannot list: its first
// titles are those it puts forward.
func (h *Handler) randomListing(w http.ResponseWriter, r *http.Request, user accounts.User, start, limit int) {
	libraries, err := h.suggestionCandidates(r.Context(), user, itemTypeFilter(r))
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	pool := topPicks(libraries)
	from := min(max(start, 0), len(pool))
	to := min(from+limit, len(pool))
	// Sorting by favorites needs the state of every title, a page only
	// that of its own.
	liked := slices.ContainsFunc(listQuery(r, "sortBy"), func(key string) bool { return strings.EqualFold(key, "IsFavoriteOrLiked") })
	described := pool[from:to]
	if liked {
		described = pool
	}
	state, err := h.userState(r.Context(), user, described)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if liked {
		slices.SortStableFunc(pool, func(a, b library.Item) int { return compareBy("IsFavoriteOrLiked", b, a, state) })
	}
	items := h.listDtos(r, user, pool[from:to], requestedFields(r), state)
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(pool), StartIndex: start})
}
