package jellyfin

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Extras are the videos and theme songs local folders keep beside their
// titles (see library.Extras): a title's local trailers, which apps play
// from its Trailer button, its special features, and its theme songs and
// videos, which apps play in the background of its page for the users who
// turned them on. Addons give trailers too, which apps get as
// RemoteTrailers, and no other extras.
func (h *Handler) extrasRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	for _, prefix := range []string{"/Items/{itemId}", "/Users/{userId}/Items/{itemId}"} {
		signedIn(http.MethodGet, prefix+"/LocalTrailers", h.localTrailers)
		signedIn(http.MethodGet, prefix+"/SpecialFeatures", h.specialFeatures)
	}
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeSongs", h.themeSongs)
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeVideos", h.themeVideos)
	signedIn(http.MethodGet, "/Items/{itemId}/ThemeMedia", h.themeMedia)
}

// localTrailers lists a title's local trailers.
func (h *Handler) localTrailers(w http.ResponseWriter, r *http.Request) {
	h.writeExtras(w, r, func(item library.Item) bool { return item.ExtraType == library.ExtraTrailer })
}

// specialFeatures lists a title's special features: its extras other than
// its trailers and theme media.
func (h *Handler) specialFeatures(w http.ResponseWriter, r *http.Request) {
	h.writeExtras(w, r, library.SpecialFeature)
}

// writeExtras answers the extras of the item a request names that keep
// keeps, as Jellyfin answers them: an array of items, empty for an item
// without extras.
func (h *Handler) writeExtras(w http.ResponseWriter, r *http.Request, keep func(library.Item) bool) {
	id, ok := h.itemRequest(w, r, func(bindErrors) {})
	if !ok {
		return
	}
	user := callerFrom(r.Context()).User
	extras, err := h.extrasOf(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	extras = slices.DeleteFunc(extras, func(item library.Item) bool { return !keep(item) })
	state, err := h.userState(r.Context(), user, extras)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.listDtos(r, user, extras, requestedFields(r), state))
}

// extrasOf lists the extras of an item, none for an item that has none
// of its own, as a version of a title or a playlist.
func (h *Handler) extrasOf(ctx context.Context, user accounts.User, id accounts.ID) ([]library.Item, error) {
	if id == (accounts.ID{}) {
		return nil, nil
	}
	extras, err := h.Library.Extras(ctx, user, id)
	if errors.Is(err, library.ErrNotFound) {
		return nil, nil
	}
	return extras, err
}

// themeMediaResult is Jellyfin's ThemeMediaResult: the theme songs or
// videos of OwnerId, the item they were found on.
type themeMediaResult struct {
	OwnerId          string
	Items            []BaseItemDto
	TotalRecordCount int
	StartIndex       int
}

func (h *Handler) themeSongs(w http.ResponseWriter, r *http.Request) {
	h.writeTheme(w, r, library.ExtraThemeSong)
}

func (h *Handler) themeVideos(w http.ResponseWriter, r *http.Request) {
	h.writeTheme(w, r, library.ExtraThemeVideo)
}

// themeRequest binds a request for an item's theme media: the item, and
// whether a season's or an episode's are those of its parents when it has
// none of its own.
func (h *Handler) themeRequest(w http.ResponseWriter, r *http.Request) (accounts.ID, bool, bool) {
	inherit := false
	id, ok := h.itemRequest(w, r, func(errs bindErrors) { inherit, _ = errs.bool(r, "inheritFromParent") })
	return id, inherit, ok
}

func (h *Handler) writeTheme(w http.ResponseWriter, r *http.Request, extraType string) {
	id, inherit, ok := h.themeRequest(w, r)
	if !ok {
		return
	}
	result, err := h.theme(r, callerFrom(r.Context()).User, id, extraType, inherit)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// themeMedia answers an item's theme videos and songs together; Polyfin
// has no soundtracks.
func (h *Handler) themeMedia(w http.ResponseWriter, r *http.Request) {
	id, inherit, ok := h.themeRequest(w, r)
	if !ok {
		return
	}
	user := callerFrom(r.Context()).User
	videos, err := h.theme(r, user, id, library.ExtraThemeVideo, inherit)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	songs, err := h.theme(r, user, id, library.ExtraThemeSong, inherit)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ThemeVideosResult     themeMediaResult
		ThemeSongsResult      themeMediaResult
		SoundtrackSongsResult themeMediaResult
	}{videos, songs, themeMediaResult{OwnerId: accounts.ID{}.String(), Items: []BaseItemDto{}}})
}

// theme lists the theme media of extraType of an item; with inherit, a
// season without any has its series', and an episode its season's, else
// its series', as Jellyfin climbs an item's parents. OwnerId is the item
// they were found on, the item itself when none were. sortBy=Random
// shuffles them, as apps ask.
func (h *Handler) theme(r *http.Request, user accounts.User, id accounts.ID, extraType string, inherit bool) (themeMediaResult, error) {
	result := themeMediaResult{OwnerId: id.String(), Items: []BaseItemDto{}}
	if id == (accounts.ID{}) {
		return result, nil
	}
	ctx := r.Context()
	item, err := h.Library.Item(ctx, user, id)
	if errors.Is(err, library.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return themeMediaResult{}, err
	}
	owners := []accounts.ID{id}
	if inherit {
		switch item.Kind {
		case library.KindEpisode:
			owners = append(owners, item.SeasonID, item.SeriesID)
		case library.KindSeason:
			owners = append(owners, item.SeriesID)
		}
	}
	for _, owner := range owners {
		if owner == (accounts.ID{}) {
			continue
		}
		extras, err := h.extrasOf(ctx, user, owner)
		if err != nil {
			return themeMediaResult{}, err
		}
		extras = slices.DeleteFunc(extras, func(extra library.Item) bool { return extra.ExtraType != extraType })
		if len(extras) == 0 {
			continue
		}
		if slices.ContainsFunc(listQuery(r, "sortBy"), func(order string) bool { return strings.EqualFold(order, "Random") }) {
			rand.Shuffle(len(extras), func(i, j int) { extras[i], extras[j] = extras[j], extras[i] })
		}
		state, err := h.userState(ctx, user, extras)
		if err != nil {
			return themeMediaResult{}, err
		}
		result.OwnerId, result.Items, result.TotalRecordCount = owner.String(), h.listDtos(r, user, extras, requestedFields(r), state), len(extras)
		break
	}
	return result, nil
}

// setExtraCounts gives a title's details the number of its local
// trailers and special features.
func (h *Handler) setExtraCounts(r *http.Request, user accounts.User, dto *BaseItemDto, item library.Item) {
	if dto.LocalTrailerCount == nil {
		return
	}
	trailers, features, err := h.Library.ExtraCounts(r.Context(), user, item)
	if err != nil {
		if r.Context().Err() == nil {
			h.Logger.Warn("The extras of a title could not be counted", "error", err)
		}
		return
	}
	*dto.LocalTrailerCount, *dto.SpecialFeatureCount = trailers, features
}
