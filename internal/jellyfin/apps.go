package jellyfin

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// The smaller routes Jellyfin apps call on a server of movies, series and
// Live TV: search hints, years, library grouping, plugin channels, what a
// session views, live streams to open, an item's file, theme media and
// images, and the refresh of a title's metadata.

func (h *Handler) appRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Search/Hints", h.searchHints)
	signedIn(http.MethodGet, "/Movies/Recommendations", h.movieRecommendations)
	signedIn(http.MethodGet, "/Trailers", h.trailers)
	signedIn(http.MethodGet, "/Years", h.years)
	signedIn(http.MethodGet, "/UserViews/GroupingOptions", h.groupingOptions)
	signedIn(http.MethodGet, "/Users/{userId}/GroupingOptions", h.groupingOptions)
	signedIn(http.MethodGet, "/Channels", h.channels)
	signedIn(http.MethodGet, "/Channels/Features", h.allChannelFeatures)
	signedIn(http.MethodGet, "/Channels/{channelId}/Features", h.unknownChannel)
	signedIn(http.MethodGet, "/Channels/{channelId}/Items", h.unknownChannel)
	signedIn(http.MethodGet, "/Channels/Items/Latest", h.latestChannelItems)
	signedIn(http.MethodPost, "/Sessions/Viewing", h.reportViewing)
	signedIn(http.MethodPost, "/LiveStreams/Open", h.openLiveStream)
	signedIn(http.MethodPost, "/LiveStreams/Close", h.closeLiveStream)
	// Like the download route, which it serves as: see streamAccess.
	rt.handle(http.MethodGet, "/Items/{itemId}/File", http.HandlerFunc(h.itemFile))
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeSongs", h.themeItems)
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeVideos", h.themeItems)
	signedIn(http.MethodGet, "/Items/{itemId}/Images", h.itemImages)
	signedIn(http.MethodGet, "/FallbackFont/Fonts", h.fallbackFonts)
	signedIn(http.MethodGet, "/FallbackFont/Fonts/{name}", h.fallbackFont)
	signedIn(http.MethodPost, "/Items/{itemId}/Refresh", h.refreshItem)
}

// SearchHint is Jellyfin's SearchHint for a movie, series, episode,
// person, or music; the fields of programmes are left out, as Jellyfin
// leaves out unset values.
type SearchHint struct {
	// ItemId repeats Id, for older apps.
	ItemId                  string
	Id                      string
	Name                    string
	IndexNumber             *int   `json:",omitempty"`
	ProductionYear          *int   `json:",omitempty"`
	ParentIndexNumber       *int   `json:",omitempty"`
	PrimaryImageTag         string `json:",omitempty"`
	ThumbImageTag           string `json:",omitempty"`
	ThumbImageItemId        string `json:",omitempty"`
	BackdropImageTag        string `json:",omitempty"`
	BackdropImageItemId     string `json:",omitempty"`
	Type                    string
	IsFolder                *bool  `json:",omitempty"`
	RunTimeTicks            *int64 `json:",omitempty"`
	MediaType               string
	Series                  string `json:",omitempty"`
	Status                  string `json:",omitempty"`
	Album                   string `json:",omitempty"`
	AlbumId                 string `json:",omitempty"`
	AlbumArtist             string `json:",omitempty"`
	Artists                 []string
	ChannelId               *string  // null, as for every item outside a channel
	PrimaryImageAspectRatio *float64 `json:",omitempty"`
}

// SearchHintResult is Jellyfin's answer to a search as the user types.
type SearchHintResult struct {
	SearchHints      []SearchHint
	TotalRecordCount int
}

// hintLimit bounds the hints of each kind looked up when the app sets no
// limit: a search as the user types shows a few.
const hintLimit = 50

// searchHints answers Jellyfin's search hints from the search /Items
// answers: the movies and series the user's addons find, then the
// episodes Polyfin knows by name (see library.Service.SearchEpisodes),
// then the people credited in titles the user reaches, each within the
// user's view and restrictions. Jellyfin ranks every kind together by how
// well it matches; addons rank their own results. parentId, and the
// isMovie, isSeries, isNews, isKids and isSports filters, which are about
// programmes, are read and left unused.
func (h *Handler) searchHints(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, _ := b.int32(r, "startIndex")
	limit, limited := b.int32(r, "limit")
	b.guid(r, "parentId")
	for _, name := range []string{"isMovie", "isSeries", "isNews", "isKids", "isSports", "includeGenres", "includeStudios", "includeArtists"} {
		b.bool(r, name)
	}
	people, peopleSet := b.bool(r, "includePeople")
	media, mediaSet := b.bool(r, "includeMedia")
	term := strings.TrimSpace(query(r, "searchTerm"))
	if term == "" {
		b.add("searchTerm", "The searchTerm field is required.")
	}
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	people, media = people || !peopleSet, media || !mediaSet
	// Enough beyond the page for the total to count the hints that follow.
	count := max(start, 0) + hintLimit
	if limited && limit > hintLimit {
		count = max(start, 0) + limit
	}
	keep := itemTypeFilter(r)
	var found []library.Item
	if media {
		if kinds := searchKinds(keep); len(kinds) > 0 {
			titles, err := h.Library.Search(r.Context(), user, term, kinds, count)
			if err != nil {
				h.browseError(w, r, err)
				return
			}
			found = append(found, titles...)
		}
		if keep(library.Item{Kind: library.KindEpisode}) {
			episodes, err := h.Library.SearchEpisodes(r.Context(), user, term, count)
			if err != nil {
				h.internalError(w, r, err)
				return
			}
			found = append(found, episodes...)
		}
		if kinds := musicSearchKinds(keep); len(kinds) > 0 {
			music, err := h.Library.SearchMusic(r.Context(), user, term, kinds, count)
			if err != nil {
				h.browseError(w, r, err)
				return
			}
			found = append(found, music...)
		}
	}
	if people && keep(library.Item{Kind: library.KindPerson}) {
		credited, _, err := h.Library.People(r.Context(), user, library.PeopleQuery{NameContains: term, Limit: count})
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		found = append(found, credited...)
	}
	found = slices.DeleteFunc(found, func(item library.Item) bool { return !keep(item) })
	hints := make([]SearchHint, 0, len(found))
	for _, item := range found {
		hints = append(hints, h.searchHint(item))
	}
	total := len(hints)
	hints = hints[min(max(start, 0), len(hints)):]
	if limited && limit >= 0 {
		hints = hints[:min(limit, len(hints))]
	}
	writeJSON(w, http.StatusOK, SearchHintResult{SearchHints: hints, TotalRecordCount: total})
}

// searchHint describes an item as a search hint, with the artwork its
// page shows: a season's or an episode's backdrop and thumbnail are their
// series'.
func (h *Handler) searchHint(item library.Item) SearchHint {
	var images BaseItemDto
	images.ImageTags = map[string]string{}
	h.setImages(&images, item)
	hint := SearchHint{
		ItemId:                  item.ID.String(),
		Id:                      item.ID.String(),
		Name:                    item.Name,
		PrimaryImageTag:         images.ImageTags["Primary"],
		Type:                    itemTypes[item.Kind],
		MediaType:               mediaType(item.Kind),
		Artists:                 []string{},
		PrimaryImageAspectRatio: images.PrimaryImageAspectRatio,
	}
	if item.ProductionYear > 0 {
		hint.ProductionYear = new(item.ProductionYear)
	}
	if item.Runtime > 0 {
		hint.RunTimeTicks = new(int64(item.Runtime / 100))
	}
	if isFolder(item.Kind) {
		hint.IsFolder = new(true)
	}
	parent := item.ID
	switch item.Kind {
	case library.KindEpisode:
		hint.IndexNumber, hint.ParentIndexNumber = new(item.IndexNumber), new(item.ParentIndexNumber)
		hint.Series = item.SeriesName
		parent = item.SeriesID
	case library.KindSeries:
		hint.Status = item.Status
	case library.KindTrack, library.KindAudiobook, library.KindAlbum:
		hint.Album = item.Album
		if item.AlbumID != (accounts.ID{}) {
			hint.AlbumId = item.AlbumID.String()
		}
		if item.IndexNumber > 0 {
			hint.IndexNumber = new(item.IndexNumber)
		}
		if item.AlbumArtist != nil {
			hint.AlbumArtist = item.AlbumArtist.Name
		}
		hint.Artists = *creditNames(item.Artists)
	}
	if tag := library.ImageTag(item.Images.Thumb); tag != "" {
		hint.ThumbImageTag, hint.ThumbImageItemId = tag, parent.String()
		if item.Kind != library.KindEpisode {
			hint.ThumbImageItemId = item.ID.String()
		}
	}
	if tag := library.ImageTag(item.Images.Backdrop); tag != "" {
		hint.BackdropImageTag, hint.BackdropImageItemId = tag, parent.String()
	}
	return hint
}

// trailers lists trailer items. Jellyfin lists those of its trailer
// libraries and channels; addons give trailers only with their titles,
// which apps get as RemoteTrailers, so Polyfin has no trailer item.
func (h *Handler) trailers(w http.ResponseWriter, r *http.Request) {
	emptyPage(w, r, bindErrors{})
}

// years lists the years the user's year pages offer titles for (see
// yearPage), as Jellyfin lists the years of the titles under a folder,
// ascending unless sortOrder asks otherwise. With parentId, only those of
// that library; another folder or item has none.
func (h *Handler) years(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	var libraryID *accounts.ID
	if hasParent {
		if h.hiddenLibrary(w, r, user, parent) {
			return
		}
		libraryID = &parent
	}
	years, err := h.Library.Years(r.Context(), user, libraryID, searchKinds(itemTypeFilter(r)))
	if err != nil && !errors.Is(err, library.ErrNotFound) {
		h.browseError(w, r, err)
		return
	}
	if slices.ContainsFunc(listQuery(r, "sortOrder"), func(order string) bool { return strings.EqualFold(order, "Descending") }) {
		slices.Reverse(years)
	}
	items := make([]BaseItemDto, 0, len(years))
	for _, year := range years {
		name := strconv.Itoa(year)
		id := nameID("year", name)
		items = append(items, BaseItemDto{
			Name:              name,
			ServerId:          h.ServerID,
			Id:                id,
			Type:              "Year",
			ImageTags:         map[string]string{},
			BackdropImageTags: []string{},
			ImageBlurHashes:   map[string]map[string]string{},
			LocationType:      "FileSystem",
			MediaType:         "Unknown",
			UserData:          UserItemData{Key: name, ItemId: id},
		})
	}
	if limit < 0 {
		limit = len(items)
	}
	writeJSON(w, http.StatusOK, pageOf(items, start, limit, len(items)))
}

// SpecialViewOptionDto is a library apps may group the user's views by.
type SpecialViewOptionDto struct {
	Name string
	Id   string
}

// groupingOptions lists the user's libraries of movies, of series or of
// both, by name, as Jellyfin lists those it can group.
func (h *Handler) groupingOptions(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	libraries, err := h.Library.Libraries(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	options := []SpecialViewOptionDto{}
	for _, l := range libraries {
		if l.CollectionType == "movies" || l.CollectionType == "tvshows" || l.CollectionType == "" {
			options = append(options, SpecialViewOptionDto{Name: l.Name, Id: l.ID.String()})
		}
	}
	slices.SortStableFunc(options, func(a, b SpecialViewOptionDto) int { return strings.Compare(a.Name, b.Name) })
	writeJSON(w, http.StatusOK, options)
}

// Plugin channels. Jellyfin's channels are plugins that list videos from a
// site; Polyfin has none (Live TV channels are not these), so the lists
// are empty and any channel is unknown.

func (h *Handler) channels(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	for _, name := range []string{"supportsLatestItems", "supportsMediaDeletion", "isFavorite"} {
		b.bool(r, name)
	}
	emptyPage(w, r, b)
}

func (h *Handler) allChannelFeatures(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []struct{}{})
}

// unknownChannel answers the features or items of a channel, which
// Jellyfin cannot find a plugin for: 400.
func (h *Handler) unknownChannel(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.pathID(r, "channelId")
	b.guid(r, "folderId")
	b.guid(r, "userId")
	b.int32(r, "startIndex")
	b.int32(r, "limit")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	processingError(w, http.StatusBadRequest)
}

func (h *Handler) latestChannelItems(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	for _, raw := range listQuery(r, "channelIds") {
		if _, ok := parseGUID(raw); !ok {
			b.add("channelIds", notValid(raw))
		}
	}
	emptyPage(w, r, b)
}

// reportViewing records the item a session shows, which session lists give
// as its NowViewingItem: the caller's session, or another the caller may
// control. Jellyfin fails on an item it does not know; Polyfin answers 404.
func (h *Handler) reportViewing(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	raw := strings.TrimSpace(query(r, "itemId"))
	item, valid := parseGUID(raw)
	switch {
	case raw == "":
		b.add("itemId", "The itemId field is required.")
	case !valid:
		b.add("itemId", notValid(raw))
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	caller := callerFrom(r.Context())
	session := caller.Device
	// An API key has no session of its own: it names one.
	if caller.APIKey != nil && query(r, "sessionId") == "" {
		validationProblem(w, map[string][]string{"sessionId": {"The sessionId field is required."}})
		return
	}
	if raw := query(r, "sessionId"); raw != "" {
		id, ok := parseGUID(raw)
		if !ok {
			processingError(w, http.StatusNotFound)
			return
		}
		target, err := h.Accounts.Device(r.Context(), id)
		switch {
		case errors.Is(err, accounts.ErrNotFound):
			processingError(w, http.StatusNotFound)
			return
		case err != nil:
			h.internalError(w, r, err)
			return
		case !mayControl(caller.User, target.UserID):
			processingError(w, http.StatusForbidden)
			return
		}
		session = target
	}
	owner, err := h.Accounts.User(r.Context(), session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if _, err := h.Library.Item(r.Context(), owner, item); err != nil {
		h.browseError(w, r, err)
		return
	}
	h.viewing.Put(session.ID, item)
	w.WriteHeader(http.StatusNoContent)
}

// describeViewing adds to a session the item its app last reported
// showing, if its user still reaches it.
func (h *Handler) describeViewing(r *http.Request, user accounts.User, info *SessionInfo, device accounts.ID) {
	id, ok := h.viewing.Get(device)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if err != nil {
		return
	}
	dtos, err := h.dtos(r, user, []library.Item{item}, nil, nil)
	if err == nil && len(dtos) == 1 {
		info.NowViewingItem = &dtos[0]
	}
}

// openLiveStream opens a source that must be opened before it plays.
// Polyfin's sources need no opening (RequiresOpening is false), and so
// have no open token: Jellyfin answers a request without one with 400.
func (h *Handler) openLiveStream(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.guid(r, "userId")
	b.guid(r, "itemId")
	for _, name := range []string{"maxStreamingBitrate", "audioStreamIndex", "subtitleStreamIndex", "maxAudioChannels"} {
		b.int32(r, name)
	}
	b.ticks(r, "startTimeTicks")
	for _, name := range []string{"enableDirectPlay", "enableDirectStream", "alwaysBurnInSubtitleWhenTranscoding"} {
		b.bool(r, name)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	processingError(w, http.StatusBadRequest)
}

// closeLiveStream closes a stream opened ahead: none is, so there is
// nothing to close, as Jellyfin answers for a stream it does not know.
func (h *Handler) closeLiveStream(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(query(r, "liveStreamId")) == "" {
		validationProblem(w, map[string][]string{"liveStreamId": {"The liveStreamId field is required."}})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// themeItems lists an item's theme songs or videos: addons give none. The
// result names the item as their owner, as Jellyfin's does.
func (h *Handler) themeItems(w http.ResponseWriter, r *http.Request) {
	id, ok := h.itemRequest(w, r, func(errs bindErrors) { errs.bool(r, "inheritFromParent") })
	if ok {
		writeJSON(w, http.StatusOK, themeMediaResult{OwnerId: id.String(), Items: []struct{}{}})
	}
}

// ImageInfo is Jellyfin's description of one of an item's images. Path,
// which Jellyfin gives as the file or address the image comes from, is
// left out: Polyfin's artwork is the addons'. Their size is unknown until
// downloaded, as Jellyfin reports for remote images.
type ImageInfo struct {
	ImageType  string
	ImageIndex *int `json:",omitempty"`
	ImageTag   string
	Size       int64
}

// itemImages lists the images an item's page shows, with the tags its
// details give: its own, without its parents'. Like Jellyfin, backdrops,
// of which an item may have several, come last and are numbered.
func (h *Handler) itemImages(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item, err := h.Library.Item(r.Context(), callerFrom(r.Context()).User, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	var dto BaseItemDto
	dto.ImageTags = map[string]string{}
	h.setImages(&dto, item)
	images := []ImageInfo{}
	for _, imageType := range []string{"Primary", "Logo", "Thumb"} {
		if tag := dto.ImageTags[imageType]; tag != "" {
			images = append(images, ImageInfo{ImageType: imageType, ImageTag: tag})
		}
	}
	for i, tag := range dto.BackdropImageTags {
		images = append(images, ImageInfo{ImageType: "Backdrop", ImageIndex: new(i), ImageTag: tag})
	}
	writeJSON(w, http.StatusOK, images)
}

// metadataRefreshModes are Jellyfin's MetadataRefreshMode names.
var metadataRefreshModes = []string{"None", "ValidationOnly", "Default", "FullRefresh"}

// refreshItem is Jellyfin's metadata refresh, for administrators: Polyfin
// forgets what it keeps of the title and asks the addons to describe it
// again (see library.Service.Refresh). The modes and replace options,
// which choose what Jellyfin reads again from disk and providers, change
// nothing: addons are always asked for everything.
func (h *Handler) refreshItem(w http.ResponseWriter, r *http.Request) {
	user := callerFrom(r.Context()).User
	if !user.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	for _, name := range []string{"metadataRefreshMode", "imageRefreshMode"} {
		if raw, _ := queryParam(r, name); strings.TrimSpace(raw) != "" {
			b.enum(name, raw, metadataRefreshModes)
		}
	}
	for _, name := range []string{"replaceAllMetadata", "replaceAllImages", "regenerateTrickplay"} {
		b.bool(r, name)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	if err := h.Library.Refresh(r.Context(), user, id); err != nil {
		h.browseError(w, r, err)
		return
	}
	if files, ok := h.subtitleFiles.Get(id); ok {
		for _, file := range files {
			h.subtitleCache.Delete(file.ID)
		}
		h.subtitleFiles.Delete(id)
	}
	w.WriteHeader(http.StatusNoContent)
}
