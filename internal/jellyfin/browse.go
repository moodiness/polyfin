package jellyfin

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

const defaultPageSize = 100

func (h *Handler) browseRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/UserViews", h.views)
	signedIn(http.MethodGet, "/Users/{userId}/Views", h.views)
	signedIn(http.MethodGet, "/Library/VirtualFolders", h.virtualFolders)
	signedIn(http.MethodGet, "/Items", h.items)
	signedIn(http.MethodGet, "/Users/{userId}/Items", h.items)
	signedIn(http.MethodGet, "/Items/Latest", h.latest)
	signedIn(http.MethodGet, "/Users/{userId}/Items/Latest", h.latest)
	signedIn(http.MethodGet, "/Items/{itemId}", h.item)
	signedIn(http.MethodGet, "/Users/{userId}/Items/{itemId}", h.item)
	signedIn(http.MethodGet, "/Items/{itemId}/Ancestors", h.ancestors)
	signedIn(http.MethodGet, "/Polyfin/Items/{itemId}/Versions", h.versionProgress)
	signedIn(http.MethodGet, "/Shows/{seriesId}/Seasons", h.seasons)
	signedIn(http.MethodGet, "/Shows/{seriesId}/Episodes", h.episodes)
	signedIn(http.MethodGet, "/Items/Filters", h.filters)
	signedIn(http.MethodGet, "/Items/Filters2", h.filters2)
	signedIn(http.MethodGet, "/Genres", h.genres)
	signedIn(http.MethodGet, "/Genres/{genreName}", h.genrePage)
	signedIn(http.MethodGet, "/Studios/{name}", h.studioPage)
	signedIn(http.MethodGet, "/Years/{year}", h.yearPage)
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
// could not answer. A request the app abandoned failed for no one.
func (h *Handler) browseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, library.ErrNotFound):
		notFoundProblem(w)
	case r.Context().Err() != nil:
	default:
		h.Logger.Warn("Browsing failed", "method", r.Method, "path", r.URL.Path, "error", err)
		processingError(w, http.StatusBadGateway)
	}
}

// dtos describes listed items, with what the user did with them. When the
// request asks for MediaSources or MediaStreams, movies and episodes carry
// what is known of their versions.
func (h *Handler) dtos(r *http.Request, user accounts.User, items []library.Item, fields fieldSet, keep func(library.Item) bool) ([]BaseItemDto, error) {
	if keep != nil {
		items = slices.DeleteFunc(slices.Clone(items), func(item library.Item) bool { return !keep(item) })
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		return nil, err
	}
	return h.listDtos(r, user, items, fields, state), nil
}

// listDtos describes listed items, with what the user did with them in
// state.
func (h *Handler) listDtos(r *http.Request, user accounts.User, items []library.Item, fields fieldSet, state userState) []BaseItemDto {
	result := make([]BaseItemDto, 0, len(items))
	for _, item := range items {
		result = append(result, h.listDto(r, user, item, fields, state))
	}
	return result
}

func (h *Handler) listDto(r *http.Request, user accounts.User, item library.Item, fields fieldSet, state userState) BaseItemDto {
	dto := h.newItemDto(item, fields, false, state)
	sources, streams := fields.has("MediaSources"), fields.has("MediaStreams")
	if sources || streams {
		h.addMediaSources(r, user, &dto, item, item.ID, false)
		if !sources {
			dto.MediaSources = nil
		}
		if !streams {
			dto.MediaStreams = nil
		}
	} else if dto.Chapters != nil && (item.Kind == library.KindMovie || item.Kind == library.KindEpisode) {
		// Only what is known: a listing never asks addons for streams.
		h.setChapters(r.Context(), &dto, h.cachedPlayable(r.Context(), user, item).versions, item.ID)
	}
	if path := fields.has("Path"); (path || fields.has("CanDownload")) &&
		(item.Kind == library.KindMovie || item.Kind == library.KindEpisode) {
		// Only what is known, as MediaSources in the same listing: a listing
		// never asks addons for streams.
		h.setDownload(r, user, &dto, item, h.cachedPlayable(r.Context(), user, item).versions, item.ID, path)
	}
	if fields.has("Trickplay") && (item.Kind == library.KindMovie || item.Kind == library.KindEpisode) {
		dto.Trickplay = h.trickplayManifest(r.Context(), user, item, h.cachedPlayable(r.Context(), user, item).versions, item.ID)
	}
	return dto
}

// folderDtos describes libraries and ancestors, which Jellyfin always
// describes in full.
func (h *Handler) folderDtos(r *http.Request, user accounts.User, items []library.Item) ([]BaseItemDto, error) {
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		return nil, err
	}
	result := make([]BaseItemDto, 0, len(items))
	for _, item := range items {
		result = append(result, h.newItemDto(item, nil, true, state))
	}
	return result, nil
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
	b := bindErrors{}
	includeHidden, _ := b.bool(r, "includeHidden")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	items, err := h.userViews(r, user, includeHidden)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
}

// userViews describes the views user sees, in the order apps list them:
// their libraries and Polyfin's Live TV, Playlists and Collections views,
// those they hide included when includeHidden is set.
func (h *Handler) userViews(r *http.Request, user accounts.User, includeHidden bool) ([]BaseItemDto, error) {
	items, err := h.libraryViews(r, user)
	if err == nil {
		items, err = h.addLiveTvView(r, user, items)
	}
	if err == nil {
		items, err = h.addPlaylistsView(r, user, items)
	}
	if err == nil {
		items, err = h.addCollectionsView(r, user, items)
	}
	if err != nil {
		return nil, err
	}
	configuration, err := h.userConfiguration(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	return arrangeViews(items, configuration, includeHidden), nil
}

// libraryViews describes the libraries user sees, in the order apps list
// them.
func (h *Handler) libraryViews(r *http.Request, user accounts.User) ([]BaseItemDto, error) {
	libraries, err := h.Library.Libraries(r.Context(), user)
	if err != nil {
		return nil, err
	}
	return h.folderDtos(r, user, libraries)
}

// virtualFolders describes the caller's libraries as Jellyfin's library
// settings do. Jellyfin keeps them to administrators and refuses anyone
// else with an empty 403, as it does for every endpoint it restricts by
// policy. Libraries are addon catalogs: they have no paths, and nothing
// scans them, so they are always idle.
func (h *Handler) virtualFolders(w http.ResponseWriter, r *http.Request) {
	user := callerFrom(r.Context()).User
	if !user.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	views, err := h.libraryViews(r, user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	folders := make([]VirtualFolderInfo, 0, len(views))
	for _, view := range views {
		folder := VirtualFolderInfo{
			Name:           view.Name,
			Locations:      []string{},
			CollectionType: view.CollectionType,
			LibraryOptions: newLibraryOptions(),
			ItemId:         view.Id,
			RefreshStatus:  "Idle",
		}
		// Jellyfin names the library itself as the holder of its image,
		// and nothing when it has none.
		if view.ImageTags["Primary"] != "" {
			folder.PrimaryImageItemId = view.Id
		}
		folders = append(folders, folder)
	}
	writeJSON(w, http.StatusOK, folders)
}

// items lists a folder's children, the titles of a genre, studio or year,
// the results of a search, or items by identifier.
func (h *Handler) items(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, defaultPageSize)
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	if hasParent && h.hiddenLibrary(w, r, user, parent) {
		return
	}
	names, valid := nameFilterOf(r)
	if !valid {
		processingError(w, http.StatusBadRequest)
		return
	}
	fields := requestedFields(r)
	keep := itemTypeFilter(r)

	if ids := listQuery(r, "ids"); len(ids) > 0 {
		var wanted []accounts.ID
		for _, raw := range ids {
			if id, ok := parseGUID(raw); ok {
				wanted = append(wanted, id)
			}
		}
		found, err := h.Library.Items(r.Context(), user, wanted)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		items, err := h.dtos(r, user, found, fields, keep)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
		return
	}
	if term := strings.TrimSpace(query(r, "searchTerm")); term != "" {
		found, err := h.Library.Search(r.Context(), user, term, searchKinds(keep), max(start, 0)+limit)
		// Then what the user's music addons find.
		if kinds := musicSearchKinds(keep); err == nil && len(kinds) > 0 {
			var music []library.Item
			music, err = h.Library.SearchMusic(r.Context(), user, term, kinds, max(start, 0)+limit)
			found = append(found, music...)
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		items, err := h.dtos(r, user, found, fields, keep)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, pageOf(items, start, limit, len(items)))
		return
	}
	if people := listQuery(r, "personIds"); len(people) > 0 {
		var ids []accounts.ID
		for _, raw := range people {
			if id, ok := parseGUID(raw); ok {
				ids = append(ids, id)
			}
		}
		h.personListing(w, r, user, ids, start, limit)
		return
	}
	if hasParent && parent == rootFolderID {
		h.rootChildren(w, r, user, start, limit)
		return
	}
	if h.playlistListing(w, r, user, parent, hasParent, start, limit) {
		return
	}
	if h.musicListing(w, r, user, parent, hasParent, start, limit) {
		return
	}
	if h.collectionListing(w, r, user, parent, hasParent, start, limit) {
		return
	}
	if h.channelListing(w, r, user, parent, hasParent, start, limit) {
		return
	}
	if h.recordingListing(w, r, user, parent, hasParent, start, limit) {
		return
	}
	if filter, ok := stateFilterOf(r); ok {
		h.stateListing(w, r, user, filter, parent, hasParent, start, limit)
		return
	}
	if !hasParent {
		if names.requested() {
			h.narrowedListing(w, r, user, names, start, limit)
			return
		}
		// Without a folder, Jellyfin apps ask for whole-server listings,
		// which remote catalogs cannot answer.
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return
	}
	genre, ok := h.genreFilter(r, user, parent)
	if !ok {
		// The library cannot be narrowed to the requested genre, studio or
		// year.
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
	items, err := h.dtos(r, user, page.Items, fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: page.Total, StartIndex: start})
}

// hiddenLibrary answers a listing of one of the server's libraries the
// user does not see as Jellyfin answers one of a folder the user may not
// access: 401, naming them both. It reports whether it answered.
func (h *Handler) hiddenLibrary(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID) bool {
	if !slices.Contains(user.HiddenLibraries, parent) {
		return false
	}
	libraries, err := h.Library.ServerLibraries(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	i := slices.IndexFunc(libraries, func(l library.ServerLibrary) bool { return l.ID == parent })
	if i < 0 {
		return false
	}
	writeJSON(w, http.StatusUnauthorized, user.Name+" is not permitted to access Library "+libraries[i].Name+".")
	return true
}

// genreFilter returns the genre option of a library's catalog a listing is
// narrowed to, given by genre, studio or year (see nameFilter). ok is false
// when the listing asks for one the catalog does not offer.
func (h *Handler) genreFilter(r *http.Request, user accounts.User, parent accounts.ID) (string, bool) {
	names, valid := nameFilterOf(r)
	if !valid {
		return "", false
	}
	if !names.requested() {
		return "", true
	}
	options, err := h.Library.Genres(r.Context(), user, parent)
	if err != nil {
		return "", false
	}
	i := slices.IndexFunc(options, names.matches)
	if i < 0 {
		return "", false
	}
	return options[i], true
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
	var items []library.Item
	if folder, err := h.Library.MusicFolder(r.Context(), user, parent); err != nil {
		h.internalError(w, r, err)
		return
	} else if folder {
		// Jellyfin groups the latest songs by album, whatever the types
		// asked for: a music library's latest are its albums, in its row's
		// order, else its row's items.
		items, err = h.Library.Music(r.Context(), user, library.MusicQuery{Parent: parent, Kinds: []library.Kind{library.KindAlbum}, Recursive: true, Shallow: true})
		if err == nil && len(items) == 0 {
			items, err = h.Library.Music(r.Context(), user, library.MusicQuery{Parent: parent})
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		items = items[:min(len(items), limit)]
	} else {
		page, err := h.Library.Children(r.Context(), user, parent, 0, limit, "")
		if errors.Is(err, library.ErrNotFound) {
			writeJSON(w, http.StatusOK, []BaseItemDto{})
			return
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		keep := itemTypeFilter(r)
		items = slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) })
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	configuration, err := h.userConfiguration(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	// Like Jellyfin, a user who hides played titles among the latest (a new
	// user does) only sees the unplayed ones, unless isPlayed asks.
	played, explicit := boolQuery(r, "isPlayed")
	if !explicit && configuration.HidePlayedInLatest {
		played, explicit = false, true
	}
	if explicit {
		items = slices.DeleteFunc(items, func(item library.Item) bool { return state.of(item).Played != played })
	}
	writeJSON(w, http.StatusOK, h.listDtos(r, user, items, requestedFields(r), state))
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	// An app opening an item asks an IPTV title's details (see
	// iptv.Opening): listings never do.
	item, err := h.Library.Item(iptv.Opening(r.Context()), user, id)
	if errors.Is(err, library.ErrNotFound) {
		// Apps open a version as an item by its media source id.
		item, err = h.title(r.Context(), user, id)
	}
	if errors.Is(err, library.ErrNotFound) && h.describePlaylist(w, r, user, id) {
		return
	}
	if errors.Is(err, library.ErrNotFound) && h.describeCollection(w, r, user, id) {
		return
	}
	if errors.Is(err, library.ErrNotFound) && id == liveTvViewID && h.describeLiveTvView(w, r, user) {
		return
	}
	if errors.Is(err, library.ErrNotFound) && id == rootFolderID {
		h.writeRootFolder(w, r, user)
		return
	}
	if errors.Is(err, library.ErrNotFound) && h.describeRecording(w, r, user, id) {
		return
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	if item.Kind == library.KindPerson {
		h.writePerson(w, r, user, item)
		return
	}
	if item.Kind == library.KindRecording && h.describeRecording(w, r, user, item.ID) {
		return
	}
	state, err := h.userState(r.Context(), user, []library.Item{item})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dto := h.newItemDto(item, requestedFields(r), true, state)
	h.addMediaSources(r, user, &dto, item, id, true)
	if item.Kind == library.KindChannel {
		dtos := []BaseItemDto{dto}
		h.addCurrentPrograms(r, user, dtos, nil)
		dto = dtos[0]
	}
	if item.Kind == library.KindProgram {
		dtos := []BaseItemDto{dto}
		h.addTimers(r.Context(), dtos)
		dto = dtos[0]
	}
	writeJSON(w, http.StatusOK, dto)
}

func (h *Handler) ancestors(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if owner, isVersion := h.Library.VersionOwner(id); isVersion {
		// A version opened as an item sits where its title does.
		id = owner
	}
	folders, err := h.Library.Ancestors(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) && (h.collectionAncestors(w, r, user, id) || h.playlistAncestors(w, r, user, id)) {
		return
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	items, err := h.folderDtos(r, user, folders)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
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
	items, err := h.dtos(r, user, seasons, requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(items)})
}

func (h *Handler) episodes(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "seriesId")
	seasonID, hasSeasonID := b.guid(r, "seasonId")
	number, hasNumber := b.int32(r, "season")
	startItem, hasStartItem := b.guid(r, "startItemId")
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
	// jellyfin-web plays an episode with the episodes from it on, which it
	// asks starting at the episode, or at the version the user picked: a
	// version stands for its episode, as in Jellyfin. The list is empty
	// when the start item is not in it.
	if hasStartItem {
		if owner, ok := h.Library.VersionOwner(startItem); ok {
			startItem = owner
		}
		at := slices.IndexFunc(episodes, func(item library.Item) bool { return item.ID == startItem })
		if at < 0 {
			at = len(episodes)
		}
		episodes = episodes[at:]
	}
	items, err := h.dtos(r, user, episodes, requestedFields(r), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if limit < 0 {
		limit = len(items)
	}
	writeJSON(w, http.StatusOK, pageOf(items, start, limit, len(items)))
}

// libraryGenres returns the genres a library's catalog offers. Other
// folders have none, Polyfin's own among them; a folder that does not
// exist is refused, as Jellyfin does.
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
		exists, err := h.ownItemExists(r.Context(), user, id)
		if err != nil {
			h.internalError(w, r, err)
			return nil, false
		}
		if exists {
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
