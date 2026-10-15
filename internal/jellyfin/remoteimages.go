package jellyfin

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Remote images.
//
// jellyfin-web's Edit images searches two image providers for an item's
// artwork: TMDB, while the settings hold the server's TMDB key (see
// package tmdb), and Addons, the artwork each addon gives in its
// description of a movie or series (see library.ImageSources). Each
// listed image has Polyfin addresses, signed for the item with the
// server's secret: images are relayed as all artwork is, so apps never
// talk to TMDB or to an addon's artwork server, and Polyfin relays nothing
// but what it listed, an addon's artwork under its addon's confinement.
// Downloading one, for administrators, keeps it as an upload is (see
// uploadItemImage), and refuses an address Polyfin did not list for the
// item and image type.

const (
	// tmdbProvider and addonsProvider name the image providers.
	tmdbProvider   = "TMDB"
	addonsProvider = "Addons"
	// remoteThumbnailWidth is the most an addon's image is shown at as a
	// thumbnail.
	remoteThumbnailWidth = 400
)

// remoteImageTypes are the image types the providers offer, in the order
// they are listed when no type is asked for.
var remoteImageTypes = []string{"Primary", "Backdrop", "Logo", "Thumb"}

// ImageProviderInfo is Jellyfin's description of an image provider.
type ImageProviderInfo struct {
	Name            string
	SupportedImages []string
}

// RemoteImageInfo is Jellyfin's description of an image a provider
// offers.
type RemoteImageInfo struct {
	ProviderName    string
	Url             string
	ThumbnailUrl    string   `json:",omitempty"`
	Height          *int     `json:",omitempty"`
	Width           *int     `json:",omitempty"`
	CommunityRating *float64 `json:",omitempty"`
	VoteCount       *int     `json:",omitempty"`
	Language        string   `json:",omitempty"`
	Type            string
	RatingType      string
}

// RemoteImageResult is Jellyfin's list of the images providers offer.
type RemoteImageResult struct {
	Images           []RemoteImageInfo
	TotalRecordCount int
	Providers        []string
}

// remoteImage is an image a provider offers, as its Polyfin address
// carries it: listed for Type, downloaded from URL, from public addresses
// only when Confined. A thumbnail is shown at Width at most, and is never
// downloaded as the item's image.
type remoteImage struct {
	Provider string `json:"p"`
	Type     string `json:"t"`
	URL      string `json:"u"`
	Confined bool   `json:"c,omitempty"`
	Width    int    `json:"w,omitempty"`
}

// imageProviders are the image providers of an item of kind, with the
// image types they offer: TMDB's only while the settings hold its key.
func (h *Handler) imageProviders(kind library.Kind) []ImageProviderInfo {
	var providers []ImageProviderInfo
	if h.TMDB.Enabled() {
		switch kind {
		case library.KindMovie, library.KindSeries:
			providers = append(providers, ImageProviderInfo{Name: tmdbProvider, SupportedImages: remoteImageTypes})
		case library.KindSeason, library.KindEpisode, library.KindPerson:
			providers = append(providers, ImageProviderInfo{Name: tmdbProvider, SupportedImages: []string{"Primary"}})
		}
	}
	if kind == library.KindMovie || kind == library.KindSeries {
		providers = append(providers, ImageProviderInfo{Name: addonsProvider, SupportedImages: remoteImageTypes})
	}
	return providers
}

// remoteImageProviders lists the image providers of an item.
func (h *Handler) remoteImageProviders(w http.ResponseWriter, r *http.Request) {
	item, ok := h.editedItem(w, r)
	if !ok {
		return
	}
	providers := h.imageProviders(item.Kind)
	if providers == nil {
		providers = []ImageProviderInfo{}
	}
	writeJSON(w, http.StatusOK, providers)
}

// remoteImages lists the images of a type the image providers offer for
// an item, of every type when none is asked for, of one provider when
// providerName names it: TMDB's in the server's language and without
// one, in every language with includeAllLanguages, the server's language
// first, then by votes; then the addons', by addon name.
func (h *Handler) remoteImages(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, 0)
	allLanguages, _ := b.bool(r, "includeAllLanguages")
	imageType := ""
	if raw, _ := queryParam(r, "type"); strings.TrimSpace(raw) != "" {
		imageType = b.enum("type", raw, imageTypes)
	}
	item, ok := h.editedItemWith(w, r, b)
	if !ok {
		return
	}
	result := RemoteImageResult{Images: []RemoteImageInfo{}, Providers: []string{}}
	providers := h.imageProviders(item.Kind)
	for _, provider := range providers {
		result.Providers = append(result.Providers, provider.Name)
	}
	if name := strings.TrimSpace(query(r, "providerName")); name != "" {
		providers = slices.DeleteFunc(providers, func(p ImageProviderInfo) bool { return !strings.EqualFold(p.Name, name) })
	}
	types := remoteImageTypes
	if imageType != "" {
		types = []string{imageType}
	}
	var images []RemoteImageInfo
	if len(providers) > 0 {
		sources, err := h.Library.ImageSources(r.Context(), item.ID)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		for _, provider := range providers {
			for _, t := range types {
				if !slices.Contains(provider.SupportedImages, t) {
					continue
				}
				switch provider.Name {
				case tmdbProvider:
					images = append(images, h.tmdbImages(r, item.ID, sources, t, allLanguages)...)
				case addonsProvider:
					images = append(images, h.addonImages(r, item.ID, sources, t)...)
				}
			}
		}
	}
	result.TotalRecordCount = len(images)
	start = min(max(start, 0), len(images))
	end := len(images)
	if limit > 0 {
		end = min(start+limit, end)
	}
	result.Images = append(result.Images, images[start:end]...)
	writeJSON(w, http.StatusOK, result)
}

// addonImages lists the addons' artwork of an image type: posters for
// Primary, backgrounds for Backdrop, logos for Logo, and wide posters,
// then backgrounds, for Thumb.
func (h *Handler) addonImages(r *http.Request, item accounts.ID, sources library.ImageSources, imageType string) []RemoteImageInfo {
	var images []RemoteImageInfo
	add := func(addon library.AddonImages, address string) {
		if address == "" || slices.ContainsFunc(images, func(i RemoteImageInfo) bool { return i.ProviderName == addon.Addon && i.Url == address }) {
			return
		}
		image := remoteImage{Provider: addonsProvider, Type: imageType, URL: address, Confined: addon.Confined}
		thumbnail := image
		thumbnail.Width = remoteThumbnailWidth
		images = append(images, RemoteImageInfo{ProviderName: addon.Addon, Url: h.remoteImageURL(r, item, image),
			ThumbnailUrl: h.remoteImageURL(r, item, thumbnail), Type: imageType, RatingType: "Score"})
	}
	for _, addon := range sources.Addons {
		switch imageType {
		case "Primary":
			add(addon, addon.Images.Primary)
		case "Backdrop":
			add(addon, addon.Images.Backdrop)
		case "Logo":
			add(addon, addon.Images.Logo)
		case "Thumb":
			add(addon, addon.Images.Thumb)
			add(addon, addon.Images.Backdrop)
		}
	}
	return images
}

// tmdbImages lists TMDB's images of an image type for an item: a movie's
// or series' posters for Primary, backdrops for Backdrop and Thumb, logos
// for Logo; a season's posters, an episode's stills and a person's photos
// for Primary. A failure lists none.
func (h *Handler) tmdbImages(r *http.Request, item accounts.ID, sources library.ImageSources, imageType string, allLanguages bool) []RemoteImageInfo {
	found, thumbnailSize, err := h.tmdbImagesOf(r.Context(), sources, imageType)
	if err != nil {
		if !errors.Is(err, tmdb.ErrNotFound) && r.Context().Err() == nil {
			h.Logger.Warn("TMDB's images could not be listed", "item", item.String(), "error", err)
		}
		return nil
	}
	language, _, _ := strings.Cut(strings.ToLower(h.Accounts.Settings().Language), "-")
	rank := func(i tmdb.Image) int {
		switch i.Language {
		case language:
			return 0
		case "":
			return 1
		}
		return 2
	}
	found = slices.DeleteFunc(slices.Clone(found), func(i tmdb.Image) bool {
		// TMDB gives some logos as SVG files, which cannot be kept.
		return i.Path == "" || strings.HasSuffix(strings.ToLower(i.Path), ".svg") || !allLanguages && rank(i) == 2
	})
	slices.SortStableFunc(found, func(a, b tmdb.Image) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(b.Rating, a.Rating), cmp.Compare(b.Votes, a.Votes), cmp.Compare(b.Width, a.Width))
	})
	images := make([]RemoteImageInfo, 0, len(found))
	for _, i := range found {
		image := remoteImage{Provider: tmdbProvider, Type: imageType, URL: h.TMDB.ImageURL(i.Path, "original")}
		thumbnail := remoteImage{Provider: tmdbProvider, Type: imageType, URL: h.TMDB.ImageURL(i.Path, thumbnailSize), Width: remoteThumbnailWidth}
		info := RemoteImageInfo{ProviderName: tmdbProvider, Url: h.remoteImageURL(r, item, image), ThumbnailUrl: h.remoteImageURL(r, item, thumbnail),
			Language: i.Language, Type: imageType, RatingType: "Score", CommunityRating: new(i.Rating), VoteCount: new(i.Votes)}
		if i.Width > 0 && i.Height > 0 {
			info.Width, info.Height = new(i.Width), new(i.Height)
		}
		images = append(images, info)
	}
	return images
}

// tmdbImagesOf asks TMDB for the images of an image type for an item,
// with the size of their thumbnails.
func (h *Handler) tmdbImagesOf(ctx context.Context, sources library.ImageSources, imageType string) ([]tmdb.Image, string, error) {
	if sources.Kind == library.KindPerson {
		photos, err := h.TMDB.PersonPhotos(ctx, sources.Person)
		return photos, "w185", err
	}
	kind := tmdb.TV
	if sources.Kind == library.KindMovie {
		kind = tmdb.Movie
	}
	id := sources.TMDB
	if !digits(id) {
		if sources.IMDb == "" {
			return nil, "", tmdb.ErrNotFound
		}
		var err error
		if id, err = h.TMDB.FindByIMDb(ctx, kind, sources.IMDb); err != nil {
			return nil, "", err
		}
	}
	switch sources.Kind {
	case library.KindSeason:
		posters, err := h.TMDB.SeasonPosters(ctx, id, sources.Season)
		return posters, "w342", err
	case library.KindEpisode:
		stills, err := h.TMDB.EpisodeStills(ctx, id, sources.Season, sources.Episode)
		return stills, "w300", err
	}
	images, err := h.TMDB.TitleImages(ctx, kind, id)
	switch imageType {
	case "Primary":
		return images.Posters, "w342", err
	case "Logo":
		return images.Logos, "w300", err
	}
	return images.Backdrops, "w300", err
}

// digits reports whether text is a positive number in decimal digits.
func digits(text string) bool {
	n, err := strconv.Atoi(text)
	return err == nil && n > 0 && strconv.Itoa(n) == text
}

// remoteImagePurpose is what the addresses of an item's remote images are
// signed for.
func remoteImagePurpose(item accounts.ID) string {
	return "remote-image\x00" + item.String()
}

// remoteImageURL is the Polyfin address of an image listed for an item.
func (h *Handler) remoteImageURL(r *http.Request, item accounts.ID, image remoteImage) string {
	payload, _ := json.Marshal(image)
	value := base64.RawURLEncoding.EncodeToString(payload)
	token := value + "." + h.Playback.Signer().Mark(remoteImagePurpose(item), value)
	return baseURL(r) + "/Items/" + item.String() + "/RemoteImages/Image?" + url.Values{"image": {token}}.Encode()
}

// listedImage reads the image a token of remoteImageURL stands for, and
// reports whether Polyfin signed it for the item.
func (h *Handler) listedImage(item accounts.ID, token string) (remoteImage, bool) {
	value, signature, found := strings.Cut(token, ".")
	if !found || !h.Playback.Signer().VerifyMark(remoteImagePurpose(item), value, signature) {
		return remoteImage{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	var image remoteImage
	if err != nil || json.Unmarshal(payload, &image) != nil || image.URL == "" {
		return remoteImage{}, false
	}
	return image, true
}

// fetchRemoteImage downloads a listed image, as relayed artwork is (see
// fetchArtwork), TMDB's within the requests to TMDB at once.
func (h *Handler) fetchRemoteImage(ctx context.Context, image remoteImage) (artwork, error) {
	h.images.open(h.imageDir(), h.Logger)
	var hold func(context.Context) (func(), error)
	if image.Provider == tmdbProvider && h.TMDB != nil {
		hold = h.TMDB.Hold
	}
	return h.fetchArtworkHeld(ctx, image.URL, image.Confined, false, hold)
}

// remoteImage relays an image listed for an item, without credentials as
// artwork is: its address is signed for the item. A thumbnail is resized
// to its width.
func (h *Handler) remoteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(r.PathValue("itemId"))
	image, listed := h.listedImage(id, query(r, "image"))
	if !ok || !listed {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	found, err := h.fetchRemoteImage(r.Context(), image)
	if err != nil {
		status := http.StatusBadGateway
		if failed, ok := errors.AsType[downloadError](err); ok {
			status = failed.status
		}
		if r.Context().Err() == nil {
			h.Logger.Debug("A remote image could not be downloaded", "item", id.String(), "error", err)
		}
		w.WriteHeader(status)
		return
	}
	if image.Width > 0 && resizable(found.contentType) {
		if config, _, err := found.decodeConfig(); err == nil && config.Width > image.Width {
			found = h.resized(r.Context(), image.URL, found, image.Width)
		}
	}
	header := w.Header()
	header.Set("Content-Type", found.contentType)
	header.Set("Content-Length", strconv.Itoa(len(found.body)))
	header.Set("Cache-Control", taggedImage)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(found.body)
	}
}

// downloadRemoteImage keeps an image listed for an item as its artwork of
// type, as an upload is kept (see uploadItemImage). An imageUrl that is
// not the address of an image Polyfin listed for the item and type, or
// that of a thumbnail, is refused (400); so is an image that cannot be
// kept.
func (h *Handler) downloadRemoteImage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	imageType := b.enum("type", query(r, "type"), imageTypes)
	rawURL := strings.TrimSpace(query(r, "imageUrl"))
	if rawURL == "" {
		b.add("imageUrl", "The imageUrl field is required.")
	}
	item, ok := h.editedItemWith(w, r, b)
	if !ok {
		return
	}
	parsed, err := url.Parse(rawURL)
	var image remoteImage
	listed := false
	if err == nil && strings.HasSuffix(strings.ToLower(parsed.Path), "/remoteimages/image") {
		image, listed = h.listedImage(item.ID, parsed.Query().Get("image"))
	}
	if !listed || image.Width > 0 || image.Type != imageType {
		processingError(w, http.StatusBadRequest)
		return
	}
	found, err := h.fetchRemoteImage(r.Context(), image)
	if err != nil {
		status := http.StatusBadGateway
		if failed, ok := errors.AsType[downloadError](err); ok {
			status = failed.status
		}
		if r.Context().Err() == nil {
			h.Logger.Warn("A remote image could not be downloaded", "item", item.ID.String(), "provider", image.Provider, "error", err)
		}
		processingError(w, status)
		return
	}
	switch err := h.Library.UploadImage(r.Context(), item.ID, imageType, found.body); {
	case errors.Is(err, accounts.ErrInvalidImage), errors.Is(err, library.ErrUnsupportedImageType):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
