package jellyfin

import (
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
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
func (h *Handler) firstPages(r *http.Request, user accounts.User) ([]library.Page, error) {
	libraries, err := h.Library.Libraries(r.Context(), user)
	if err != nil {
		return nil, err
	}
	pages := make([]library.Page, len(libraries))
	var g errgroup.Group
	g.SetLimit(4)
	for i, l := range libraries {
		g.Go(func() error {
			page, err := h.Library.Children(r.Context(), user, l.ID, 0, defaultPageSize, "")
			if err != nil {
				h.Logger.Debug("A library could not be read", "library", l.ID.String(), "error", err)
				return nil
			}
			pages[i] = page
			return nil
		})
	}
	_ = g.Wait()
	return pages, r.Context().Err()
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
// writeNamedPage): their first page of items, plus one when more follow.
// Favorites are counted exactly.
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
		for _, item := range items {
			counts.add(item.Kind)
		}
		writeJSON(w, http.StatusOK, counts)
		return
	}
	pages, err := h.firstPages(r, user)
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
// the first page of each library.
func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	countAll, _ := b.bool(r, "enableTotalRecordCount")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	pages, err := h.firstPages(r, user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	types, media := listQuery(r, "type"), listQuery(r, "mediaType")
	matches := func(list []string, value string) bool {
		return len(list) == 0 || slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, value) })
	}
	seen := map[accounts.ID]bool{}
	var pool []library.Item
	for _, page := range pages {
		for _, item := range page.Items {
			if !seen[item.ID] && matches(types, itemTypes[item.Kind]) && matches(media, mediaType(item.Kind)) {
				seen[item.ID] = true
				pool = append(pool, item)
			}
		}
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
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
