package jellyfin

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

const defaultPageSize = 100

func (h *Handler) browseRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/UserViews", h.views)
	signedIn(http.MethodGet, "/Users/{userId}/Views", h.views)
	signedIn(http.MethodGet, "/Items", h.items)
	signedIn(http.MethodGet, "/Users/{userId}/Items", h.items)
	signedIn(http.MethodGet, "/Items/Latest", h.latest)
	signedIn(http.MethodGet, "/Users/{userId}/Items/Latest", h.latest)
	signedIn(http.MethodGet, "/Items/{itemId}", h.item)
	signedIn(http.MethodGet, "/Users/{userId}/Items/{itemId}", h.item)
	signedIn(http.MethodGet, "/Items/{itemId}/Ancestors", h.ancestors)
	signedIn(http.MethodGet, "/Shows/{seriesId}/Seasons", h.seasons)
	signedIn(http.MethodGet, "/Shows/{seriesId}/Episodes", h.episodes)
	signedIn(http.MethodGet, "/Items/Filters", h.filters)
	signedIn(http.MethodGet, "/Items/Filters2", h.filters2)
	signedIn(http.MethodGet, "/Genres", h.genres)
	// Jellyfin apps load images without credentials.
	rt.handle(http.MethodGet, "/Items/{itemId}/Images/{imageType}", http.HandlerFunc(h.image))
	rt.handle(http.MethodGet, "/Items/{itemId}/Images/{imageType}/{imageIndex}", http.HandlerFunc(h.image))
}

// viewer is the user whose libraries a request browses: the caller, or the
// user an administrator names by userId. It first answers the binding
// errors b collected, so that they are reported together as Jellyfin does;
// unknown answers a user that does not exist.
func (h *Handler) viewer(w http.ResponseWriter, r *http.Request, b bindErrors, unknown func(http.ResponseWriter)) (accounts.User, bool) {
	id, set := b.userID(r)
	if len(b) > 0 {
		validationProblem(w, b)
		return accounts.User{}, false
	}
	return h.targetUser(w, r, id, set, unknown)
}

// unknownListingUser answers a listing for a user that does not exist.
func unknownListingUser(w http.ResponseWriter) {
	processingError(w, http.StatusNotFound)
}

// pathID binds an identifier taken from the route.
func (b bindErrors) pathID(r *http.Request, name string) accounts.ID {
	raw := r.PathValue(name)
	id, ok := parseGUID(raw)
	if !ok {
		b.add(name, notValid(raw))
	}
	return id
}

// userID binds the user a request acts for: the route's {userId}, or the
// userId parameter. The zero identifier counts as unset.
func (b bindErrors) userID(r *http.Request) (accounts.ID, bool) {
	if r.PathValue("userId") == "" {
		return b.guid(r, "userId")
	}
	id := b.pathID(r, "userId")
	return id, id != accounts.ID{}
}

// browseError answers a library failure: a missing item, or an addon that
// could not answer.
func (h *Handler) browseError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, library.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	h.Logger.Warn("Browsing failed", "method", r.Method, "path", r.URL.Path, "error", err)
	processingError(w, http.StatusBadGateway)
}

func (h *Handler) dtos(items []library.Item, fields fieldSet, keep func(library.Item) bool) []BaseItemDto {
	result := make([]BaseItemDto, 0, len(items))
	for _, item := range items {
		if keep == nil || keep(item) {
			result = append(result, h.newItemDto(item, fields, false))
		}
	}
	return result
}

// folderDtos describes libraries and ancestors, which Jellyfin always
// describes in full.
func (h *Handler) folderDtos(items []library.Item) []BaseItemDto {
	result := make([]BaseItemDto, 0, len(items))
	for _, item := range items {
		result = append(result, h.newItemDto(item, nil, true))
	}
	return result
}

// paging binds startIndex and limit; a missing or negative limit is
// fallback. Jellyfin echoes startIndex as sent, even when negative.
func (b bindErrors) paging(r *http.Request, fallback int) (start, limit int) {
	start, _ = b.int32(r, "startIndex")
	limit, set := b.int32(r, "limit")
	if !set || limit < 0 {
		limit = fallback
	}
	return start, limit
}

func (h *Handler) views(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, unknownListingUser)
	if !ok {
		return
	}
	libraries, err := h.Library.Libraries(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items := h.folderDtos(libraries)
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
}

// items lists a folder's children, the results of a search, or items by
// identifier.
func (h *Handler) items(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, defaultPageSize)
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	fields := requestedFields(r)
	keep := itemTypeFilter(r)

	if ids := listQuery(r, "ids"); len(ids) > 0 {
		var items []BaseItemDto
		for _, raw := range ids {
			id, ok := parseGUID(raw)
			if !ok {
				continue
			}
			if item, err := h.Library.Item(r.Context(), user, id); err == nil && keep(item) {
				items = append(items, h.newItemDto(item, fields, false))
			}
		}
		items = nonNilItems(items)
		writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
		return
	}
	if term := strings.TrimSpace(query(r, "searchTerm")); term != "" {
		found, err := h.Library.Search(r.Context(), user, term, searchKinds(keep), max(start, 0)+limit)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		items := h.dtos(found, fields, keep)
		writeJSON(w, http.StatusOK, pageOf(items, start, limit, len(items)))
		return
	}
	if !hasParent || isFiltered(r) {
		// Without a folder, Jellyfin apps ask for favorites, recently played or
		// whole-server listings; Polyfin has no such state yet.
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return
	}
	genre, ok := h.genreFilter(r, user, parent)
	if !ok {
		// The library cannot be narrowed to the requested genre.
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return
	}
	page, err := h.Library.Children(r.Context(), user, parent, max(start, 0), limit, genre)
	if errors.Is(err, library.ErrNotFound) {
		// Jellyfin refuses to list a folder it does not know.
		processingError(w, http.StatusBadRequest)
		return
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: h.dtos(page.Items, fields, keep), TotalRecordCount: page.Total, StartIndex: start})
}

// genreFilter returns the genre a listing is narrowed to, given by name
// (genres, pipe-separated as Jellyfin apps send it) or identifier
// (genreIds). ok is false when a genre is requested that the library's
// catalog does not offer.
func (h *Handler) genreFilter(r *http.Request, user accounts.User, parent accounts.ID) (string, bool) {
	var names []string
	for name := range strings.SplitSeq(query(r, "genres"), "|") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	ids := listQuery(r, "genreIds")
	if len(names) == 0 && len(ids) == 0 {
		return "", true
	}
	genres, err := h.Library.Genres(r.Context(), user, parent)
	if err != nil {
		return "", false
	}
	for _, genre := range genres {
		if slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, genre) }) ||
			slices.Contains(ids, nameID("genre", genre)) {
			return genre, true
		}
	}
	return "", false
}

// isFiltered reports filters on user state (favorites, played, resumable):
// no item matches them until Polyfin records it.
func isFiltered(r *http.Request) bool {
	for _, filter := range listQuery(r, "filters") {
		switch strings.ToLower(filter) {
		case "isfavorite", "isplayed", "isresumable", "likes", "isfavoriteorlikes":
			return true
		}
	}
	for _, name := range []string{"isFavorite", "isPlayed"} {
		if value, set := boolQuery(r, name); set && value {
			return true
		}
	}
	return false
}

func nonNilItems(items []BaseItemDto) []BaseItemDto {
	if items == nil {
		return []BaseItemDto{}
	}
	return items
}

func pageOf(items []BaseItemDto, start, limit, total int) QueryResult {
	from := max(start, 0)
	if from >= len(items) {
		return QueryResult{Items: []BaseItemDto{}, TotalRecordCount: total, StartIndex: start}
	}
	return QueryResult{Items: items[from:min(from+limit, len(items))], TotalRecordCount: total, StartIndex: start}
}

// searchKinds lists the title kinds a search can return given the request's
// type filters.
func searchKinds(keep func(library.Item) bool) []library.Kind {
	var kinds []library.Kind
	for _, kind := range []library.Kind{library.KindMovie, library.KindSeries} {
		if keep(library.Item{Kind: kind}) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	limit, limited := b.int32(r, "limit")
	if !limited || limit < 0 {
		limit = 20
	}
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	if !hasParent || limit == 0 {
		writeJSON(w, http.StatusOK, []BaseItemDto{})
		return
	}
	page, err := h.Library.Children(r.Context(), user, parent, 0, limit, "")
	if errors.Is(err, library.ErrNotFound) {
		writeJSON(w, http.StatusOK, []BaseItemDto{})
		return
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.dtos(page.Items, requestedFields(r), itemTypeFilter(r)))
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.newItemDto(item, requestedFields(r), true))
}

func (h *Handler) ancestors(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	folders, err := h.Library.Ancestors(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.folderDtos(folders))
}

func (h *Handler) seasons(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "seriesId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	seasons, err := h.Library.Seasons(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	items := h.dtos(seasons, requestedFields(r), nil)
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
}

func (h *Handler) episodes(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "seriesId")
	seasonID, hasSeasonID := b.guid(r, "seasonId")
	number, hasNumber := b.int32(r, "season")
	start, limit := b.paging(r, -1)
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	var season *accounts.ID
	if hasSeasonID {
		season = &seasonID
	}
	episodes, err := h.Library.Episodes(r.Context(), user, id, season)
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeJSON(w, http.StatusNotFound, "Series not found")
		return
	case errors.Is(err, library.ErrSeasonNotFound):
		writeJSON(w, http.StatusNotFound, "No season exists with Id "+hyphenated(seasonID))
		return
	case err != nil:
		h.browseError(w, r, err)
		return
	}
	if hasNumber && !hasSeasonID {
		episodes = slices.DeleteFunc(episodes, func(item library.Item) bool { return item.ParentIndexNumber != number })
	}
	items := h.dtos(episodes, requestedFields(r), nil)
	if limit < 0 {
		limit = len(items)
	}
	writeJSON(w, http.StatusOK, pageOf(items, start, limit, len(items)))
}

// libraryGenres returns the genres a library's catalog offers. Other
// folders have none; a folder that does not exist is refused, as Jellyfin
// does.
func (h *Handler) libraryGenres(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	b := bindErrors{}
	id, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok || !hasParent {
		return nil, ok
	}
	genres, err := h.Library.Genres(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		if _, err := h.Library.Item(r.Context(), user, id); err == nil {
			return nil, true
		}
		processingError(w, http.StatusBadRequest)
		return nil, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return nil, false
	}
	return genres, true
}

func (h *Handler) filters(w http.ResponseWriter, r *http.Request) {
	genres, ok := h.libraryGenres(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]any{
		"Genres":          toAny(nonNil(genres)),
		"Tags":            {},
		"OfficialRatings": {},
		"Years":           {},
	})
}

func toAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func (h *Handler) filters2(w http.ResponseWriter, r *http.Request) {
	genres, ok := h.libraryGenres(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Genres":            genreItems(genres),
		"Tags":              []string{},
		"AudioLanguages":    []NameValuePair{},
		"SubtitleLanguages": []NameValuePair{},
	})
}

func (h *Handler) genres(w http.ResponseWriter, r *http.Request) {
	genres, ok := h.libraryGenres(w, r)
	if !ok {
		return
	}
	items := make([]BaseItemDto, 0, len(genres))
	for _, genre := range genres {
		items = append(items, BaseItemDto{
			Name:              genre,
			ServerId:          h.ServerID,
			Id:                nameID("genre", genre),
			Type:              "Genre",
			IsFolder:          false,
			ImageTags:         map[string]string{},
			BackdropImageTags: []string{},
			ImageBlurHashes:   map[string]map[string]string{},
			LocationType:      "FileSystem",
			MediaType:         "Unknown",
			UserData:          UserItemData{Key: genre, ItemId: nameID("genre", genre)},
		})
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items), StartIndex: 0})
}
