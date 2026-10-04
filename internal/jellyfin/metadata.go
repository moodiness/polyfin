package jellyfin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
)

// Administrators' edits.
//
// jellyfin-web's Edit metadata, Edit images and Identify dialogs, for
// administrators. Edits are kept apart from what addons say and win over
// it for every user (see library.Overrides); people and provider
// identifiers stay the addons'. Polyfin has no image provider, and its
// metadata providers are the server's addons: Identify searches them, but
// cannot bind an item to another title, since an item is the addon's title.

// remoteSearchTypes are the item types Jellyfin's Identify searches for,
// with the kind of title the server's addons are searched for, if any.
var remoteSearchTypes = map[string]library.Kind{
	"Movie": library.KindMovie, "Series": library.KindSeries, "Trailer": "", "MusicVideo": "", "BoxSet": "",
	"MusicArtist": "", "MusicAlbum": "", "Person": "", "Book": "",
}

func (h *Handler) metadataRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	administrator := func(method, pattern string, handler http.HandlerFunc) {
		signedIn(method, pattern, func(w http.ResponseWriter, r *http.Request) {
			if !callerFrom(r.Context()).User.IsAdministrator {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		})
	}
	administrator(http.MethodGet, "/Items/{itemId}/MetadataEditor", h.metadataEditor)
	administrator(http.MethodPost, "/Items/{itemId}", h.updateItem)
	administrator(http.MethodGet, "/Items/{itemId}/ExternalIdInfos", h.externalIDInfos)
	signedIn(http.MethodGet, "/Items/{itemId}/RemoteImages", h.remoteImages)
	signedIn(http.MethodGet, "/Items/{itemId}/RemoteImages/Providers", h.remoteImageProviders)
	administrator(http.MethodPost, "/Items/{itemId}/Images/{imageType}", h.uploadItemImage)
	administrator(http.MethodPost, "/Items/{itemId}/Images/{imageType}/{imageIndex}", h.uploadItemImage)
	administrator(http.MethodDelete, "/Items/{itemId}/Images/{imageType}", h.deleteItemImage)
	administrator(http.MethodDelete, "/Items/{itemId}/Images/{imageType}/{imageIndex}", h.deleteItemImage)
	for itemType, kind := range remoteSearchTypes {
		if itemType == "Person" {
			administrator(http.MethodPost, "/Items/RemoteSearch/"+itemType, h.remoteSearch(kind))
		} else {
			signedIn(http.MethodPost, "/Items/RemoteSearch/"+itemType, h.remoteSearch(kind))
		}
	}
	administrator(http.MethodPost, "/Items/RemoteSearch/Apply/{itemId}", h.applyRemoteSearch)
}

// originalItem resolves an item the caller reaches as Polyfin describes it
// apart from administrators' edits: an item of the libraries, one of
// Polyfin's collections or playlists, its views or recordings.
func (h *Handler) originalItem(r *http.Request, user accounts.User, id accounts.ID) (library.Item, error) {
	ctx := r.Context()
	item, err := h.Library.Original(ctx, user, id)
	if !errors.Is(err, library.ErrNotFound) {
		return item, err
	}
	switch id {
	case rootFolderID:
		return rootFolderItem(), nil
	case collectionsViewID:
		return collectionsViewItem(), nil
	case playlistsViewID:
		return playlistsViewItem(), nil
	case liveTvViewID:
		if has, err := h.Library.HasChannels(ctx, user); err != nil || !has {
			return library.Item{}, cmpErr(err, library.ErrNotFound)
		}
		return library.Item{ID: liveTvViewID, Kind: library.KindLibrary, Name: "Live TV", CollectionType: "livetv"}, nil
	}
	if c, err := h.Collections.Get(ctx, id); err == nil {
		titles, err := h.collectionTitles(ctx, user, c)
		if err != nil {
			return library.Item{}, err
		}
		item, _ := collectionItem(c, titles[0], nil, nil)
		return item, nil
	} else if !errors.Is(err, collections.ErrNotFound) {
		return library.Item{}, err
	}
	if p, err := h.Playlists.Get(ctx, id); err == nil && p.Visible(user.ID) {
		return library.Item{ID: p.ID, Kind: library.KindMusicPlaylist, Name: p.Name}, nil
	} else if err != nil && !errors.Is(err, playlists.ErrNotFound) {
		return library.Item{}, err
	}
	if rec, channel, err := h.visibleRecording(ctx, user, id); err == nil {
		return recordingItem(rec, channel), nil
	}
	return library.Item{}, library.ErrNotFound
}

// cmpErr returns err, else fallback.
func cmpErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

// editedItem binds the item a request edits and resolves it (see
// originalItem); ok is false once w was answered.
func (h *Handler) editedItem(w http.ResponseWriter, r *http.Request) (library.Item, bool) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return library.Item{}, false
	}
	item, err := h.originalItem(r, callerFrom(r.Context()).User, id)
	if err != nil {
		h.browseError(w, r, err)
		return library.Item{}, false
	}
	return item, true
}

// ExternalIDInfo is Jellyfin's ExternalIdInfo: a provider identifier an
// item may have.
type ExternalIDInfo struct {
	Name string
	Key  string
	Type string `json:",omitempty"`
}

// externalIDs lists the provider identifiers Polyfin knows for an item:
// those it reads from addons for movies and series, as Jellyfin names
// them, and those an item of another kind has.
func externalIDs(item library.Item) []ExternalIDInfo {
	imdb := ExternalIDInfo{Name: "IMDb", Key: "Imdb"}
	switch item.Kind {
	case library.KindMovie:
		return []ExternalIDInfo{imdb, {Name: "TheMovieDb", Key: "Tmdb", Type: "Movie"}}
	case library.KindSeries:
		return []ExternalIDInfo{imdb, {Name: "TheMovieDb", Key: "Tmdb", Type: "Series"}, {Name: "TheTVDB", Key: "Tvdb", Type: "Series"}}
	}
	known := []ExternalIDInfo{imdb, {Name: "TheMovieDb", Key: "Tmdb"}, {Name: "TheTVDB", Key: "Tvdb"}}
	return slices.DeleteFunc(known, func(info ExternalIDInfo) bool { return item.ProviderIDs[info.Key] == "" })
}

// MetadataEditorInfo is what Jellyfin's metadata editor offers to choose
// from. Polyfin has no content types to set on folders.
type MetadataEditorInfo struct {
	ParentalRatingOptions []ParentalRating
	Countries             []CountryInfo
	Cultures              []CultureDto
	ExternalIdInfos       []ExternalIDInfo
	ContentTypeOptions    []NameValuePair
}

func (h *Handler) metadataEditor(w http.ResponseWriter, r *http.Request) {
	item, ok := h.editedItem(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, MetadataEditorInfo{
		ParentalRatingOptions: parentalRatings(),
		Countries:             countries(),
		Cultures:              editorCultures(),
		ExternalIdInfos:       externalIDs(item),
		ContentTypeOptions:    []NameValuePair{},
	})
}

// editorCultures lists the languages as the metadata editor offers them:
// once per name, by name.
func editorCultures() []CultureDto {
	list := slices.Clone(cultures())
	slices.SortStableFunc(list, func(a, b CultureDto) int { return strings.Compare(a.DisplayName, b.DisplayName) })
	return slices.CompactFunc(list, func(a, b CultureDto) bool { return strings.EqualFold(a.DisplayName, b.DisplayName) })
}

func (h *Handler) externalIDInfos(w http.ResponseWriter, r *http.Request) {
	if item, ok := h.editedItem(w, r); ok {
		writeJSON(w, http.StatusOK, externalIDs(item))
	}
}

// flexibleNumber binds a number as Jellyfin does from the metadata editor,
// which sends the text of its fields: a number, or text holding one, and
// null or empty text for none.
type flexibleNumber struct{ value *float64 }

func (n *flexibleNumber) UnmarshalJSON(raw []byte) error {
	text := string(bytes.TrimSpace(raw))
	if unquoted, err := strconv.Unquote(text); err == nil {
		text = strings.TrimSpace(unquoted)
	}
	if text == "null" || text == "" {
		n.value = nil
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	n.value = &value
	return nil
}

// flexibleDate binds a date sent as a day or a time, null or empty for
// none.
type flexibleDate struct{ value *time.Time }

func (d *flexibleDate) UnmarshalJSON(raw []byte) error {
	var text *string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	if text == nil || strings.TrimSpace(*text) == "" {
		d.value = nil
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02"} {
		if value, err := time.Parse(layout, strings.TrimSpace(*text)); err == nil {
			d.value = &value
			return nil
		}
	}
	return errors.New("invalid date")
}

// itemUpdate is the BaseItemDto the metadata editor sends, of which
// Polyfin keeps the fields below; the others, people and provider
// identifiers among them, are the addons'.
type itemUpdate struct {
	Name            *string
	OriginalTitle   *string
	ForcedSortName  *string
	Overview        *string
	Taglines        []string
	Genres          []string
	Tags            []string
	Studios         []struct{ Name string }
	OfficialRating  *string
	CustomRating    *string
	CommunityRating flexibleNumber
	CriticRating    flexibleNumber
	PremiereDate    flexibleDate
	EndDate         flexibleDate
	ProductionYear  flexibleNumber
}

// updateItem keeps an administrator's edit of an item's metadata.
func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	var request itemUpdate
	present, ok := readJSONBody(w, r, &request, "request")
	if !ok {
		return
	}
	if !present {
		requireBody(w, "request")
		return
	}
	base, err := h.originalItem(r, callerFrom(r.Context()).User, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	edit := library.Overrides{
		Name: request.Name, OriginalTitle: request.OriginalTitle, SortName: request.ForcedSortName, Overview: request.Overview,
		Taglines: request.Taglines, Genres: request.Genres, Tags: request.Tags,
		OfficialRating: request.OfficialRating, CustomRating: request.CustomRating,
		CommunityRating: request.CommunityRating.value, CriticRating: request.CriticRating.value,
		PremiereDate: request.PremiereDate.value, EndDate: request.EndDate.value,
	}
	if request.Studios != nil {
		edit.Studios = []string{}
		for _, studio := range request.Studios {
			edit.Studios = append(edit.Studios, studio.Name)
		}
	}
	if year := request.ProductionYear.value; year != nil && *year == float64(int(*year)) {
		edit.ProductionYear = new(int(*year))
	}
	if err := h.Library.SaveOverrides(r.Context(), base, edit); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoteImageResult is Jellyfin's list of the images providers offer.
type RemoteImageResult struct {
	Images           []struct{}
	TotalRecordCount int
	Providers        []string
}

// remoteImages lists the images image providers offer for an item, as
// Jellyfin does without any: none.
func (h *Handler) remoteImages(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.int32(r, "startIndex")
	b.int32(r, "limit")
	b.bool(r, "includeAllLanguages")
	if raw, _ := queryParam(r, "type"); strings.TrimSpace(raw) != "" {
		b.enum("type", raw, imageTypes)
	}
	if _, ok := h.editedItemWith(w, r, b); ok {
		writeJSON(w, http.StatusOK, RemoteImageResult{Images: []struct{}{}, Providers: []string{}})
	}
}

// remoteImageProviders lists the image providers of an item: Polyfin has
// none.
func (h *Handler) remoteImageProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.editedItem(w, r); ok {
		writeJSON(w, http.StatusOK, []struct{}{})
	}
}

// editedItemWith is editedItem, answering the binding errors b already
// collected with those of the route.
func (h *Handler) editedItemWith(w http.ResponseWriter, r *http.Request, b bindErrors) (library.Item, bool) {
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return library.Item{}, false
	}
	item, err := h.originalItem(r, callerFrom(r.Context()).User, id)
	if err != nil {
		h.browseError(w, r, err)
		return library.Item{}, false
	}
	return item, true
}

// uploadItemImage keeps the image in the body, in base64 with its type as
// Content-Type as jellyfin-web sends it, as an item's artwork in place of
// the addon's. Like Jellyfin, it refuses a type that is not an image; like
// profile pictures, a picture that is not a JPEG, PNG or WebP image within
// library.MaxUploadedImageBytes. Only the image types an item shows can be
// uploaded (see library.UploadedImageTypes).
func (h *Handler) uploadItemImage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	imageType := r.PathValue("imageType")
	b.enum("imageType", imageType, imageTypes)
	if index := r.PathValue("imageIndex"); index != "" {
		if _, err := strconv.Atoi(index); err != nil {
			b.add("imageIndex", notValid(index))
		}
	}
	item, ok := h.editedItemWith(w, r, b)
	if !ok {
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		writeJSON(w, http.StatusBadRequest, "Incorrect ContentType.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, library.MaxUploadedImageBytes/3*4+64<<10))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.Map(func(c rune) rune {
		if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
			return -1
		}
		return c
	}, string(body)))
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	switch err := h.Library.UploadImage(r.Context(), item.ID, imageType, data); {
	case errors.Is(err, accounts.ErrInvalidImage), errors.Is(err, library.ErrUnsupportedImageType):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteItemImage drops an item's uploaded artwork of a type, which shows
// the addon's again. Polyfin cannot delete the addon's own artwork: an
// item without an upload is left as it is, as Jellyfin leaves an item
// without the image.
func (h *Handler) deleteItemImage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	imageType := r.PathValue("imageType")
	b.enum("imageType", imageType, imageTypes)
	if index := r.PathValue("imageIndex"); index != "" {
		if _, err := strconv.Atoi(index); err != nil {
			b.add("imageIndex", notValid(index))
		}
	} else {
		b.int32(r, "imageIndex")
	}
	item, ok := h.editedItemWith(w, r, b)
	if !ok {
		return
	}
	if err := h.Library.DeleteUploadedImage(r.Context(), item.ID, imageType); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// uploadedImage serves an item's uploaded artwork, tagged as items show
// it.
func (h *Handler) uploadedImage(w http.ResponseWriter, r *http.Request, id accounts.ID, imageType string) {
	picture, err := h.Library.UploadedImage(r.Context(), id, imageType)
	if errors.Is(err, library.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	url, _ := h.Library.UploadedArtwork(id, imageType)
	tag := library.ImageTag(url)
	header := w.Header()
	header.Set("ETag", `"`+tag+`"`)
	if r.Header.Get("If-None-Match") == `"`+tag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header.Set("Content-Type", picture.ContentType)
	header.Set("Content-Length", strconv.Itoa(len(picture.Data)))
	// A tag names one upload, which never changes; a request without one
	// gets whatever is uploaded now.
	if query(r, "tag") == tag {
		header.Set("Cache-Control", "public, max-age=604800")
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(picture.Data)
	}
}

// RemoteSearchResult is Jellyfin's description of a title a metadata
// provider found.
type RemoteSearchResult struct {
	Name               string
	ProviderIds        map[string]string
	ProductionYear     *int   `json:",omitempty"`
	PremiereDate       *Time  `json:",omitempty"`
	ImageUrl           string `json:",omitempty"`
	SearchProviderName string `json:",omitempty"`
	Overview           string `json:",omitempty"`
	Artists            []struct{}
}

// remoteSearchLimit bounds the titles Identify lists.
const remoteSearchLimit = 20

// remoteSearch is Identify's search for titles of kind: the server's addons
// that describe titles are searched by name for movies and series; Polyfin
// has no provider for the other types (kind empty), and finds nothing.
func (h *Handler) remoteSearch(kind library.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h.searchProviders(w, r, kind) }
}

func (h *Handler) searchProviders(w http.ResponseWriter, r *http.Request, kind library.Kind) {
	var request struct {
		SearchInfo *struct {
			Name string
			Year flexibleNumber
		}
	}
	present, ok := readJSONBody(w, r, &request, "query")
	if !ok {
		return
	}
	if !present {
		requireBody(w, "query")
		return
	}
	results := []RemoteSearchResult{}
	if kind == "" || request.SearchInfo == nil {
		writeJSON(w, http.StatusOK, results)
		return
	}
	found, err := h.Library.RemoteSearch(r.Context(), kind, request.SearchInfo.Name, remoteSearchLimit)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	year := request.SearchInfo.Year.value
	for _, f := range found {
		item := f.Item
		if year != nil && item.ProductionYear > 0 && float64(item.ProductionYear) != *year {
			continue
		}
		result := RemoteSearchResult{Name: item.Name, ProviderIds: item.ProviderIDs, ImageUrl: item.Images.Primary,
			SearchProviderName: f.Provider, Overview: item.Overview, Artists: []struct{}{}}
		if result.ProviderIds == nil {
			result.ProviderIds = map[string]string{}
		}
		if item.ProductionYear > 0 {
			result.ProductionYear = new(item.ProductionYear)
		}
		if item.PremiereDate != nil {
			result.PremiereDate = new(Time(*item.PremiereDate))
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, results)
}

// applyRemoteSearch is Identify's last step, which binds an item to the
// title chosen. An item is its addon's title: Polyfin cannot bind it to
// another, and answers as Jellyfin does when applying fails.
func (h *Handler) applyRemoteSearch(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.bool(r, "replaceAllImages")
	if _, ok := h.editedItemWith(w, r, b); !ok {
		return
	}
	h.Logger.Info("An administrator asked to identify an item again, which Polyfin does not do", "item", r.PathValue("itemId"))
	processingError(w, http.StatusInternalServerError)
}
