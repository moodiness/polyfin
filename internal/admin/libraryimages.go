package admin

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// LibraryImages finds and keeps the images Jellyfin apps show on the tiles
// of libraries (see package library), and drops those uploaded for the
// libraries of an addon removed.
type LibraryImages interface {
	LibraryImages(ctx context.Context, scope addons.Scope, confined bool, catalogs []addons.Library) ([]library.LibraryImage, error)
	UploadImage(ctx context.Context, id accounts.ID, imageType string, data []byte) error
	DownloadImage(ctx context.Context, id accounts.ID, imageType, address string, confined bool) error
	DeleteUploadedImage(ctx context.Context, id accounts.ID, imageType string) error
	DeleteLibraryImages(ctx context.Context, addon addons.Addon) error
}

// libraryImageRequest chooses the image of one of the scope's libraries:
// "none", "automatic", or "custom" with a picture, sent in Data (base64)
// or found at URL, which the server downloads once.
type libraryImageRequest struct {
	AddonID     string `json:"addonId"`
	CatalogType string `json:"catalogType"`
	CatalogID   string `json:"catalogId"`
	Image       string `json:"image"`
	Data        string `json:"data"`
	URL         string `json:"url"`
}

// saveLibraryImage chooses the image one of the scope's enabled libraries
// shows in Jellyfin apps, and answers the scope's libraries. A custom
// picture is kept as the library item's uploaded Primary image, within the
// limits of the artwork jellyfin-web uploads; choosing none or automatic
// drops it. Only administrators' downloads may reach local networks.
func (h *handler) saveLibraryImage(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	if h.LibraryImages == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body libraryImageRequest
	if !decodeUpTo(w, r, &body, library.MaxUploadedImageBytes/3*4+64<<10) {
		return
	}
	custom := body.Image == "custom"
	if !custom && body.Image != addons.LibraryImageNone && body.Image != addons.LibraryImageAutomatic ||
		custom && (body.Data == "") == (body.URL == "") || !custom && (body.Data != "" || body.URL != "") {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	addonID, err := accounts.ParseID(body.AddonID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_library")
		return
	}
	catalogs, err := h.Addons.Libraries(r.Context(), scope)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	index := slices.IndexFunc(catalogs, func(l addons.Library) bool {
		return l.Enabled && l.AddonID == addonID && l.Catalog.Type == body.CatalogType && l.Catalog.ID == body.CatalogID &&
			!library.LiveCatalog(l.Catalog.Type)
	})
	if index < 0 {
		writeError(w, http.StatusBadRequest, "invalid_library")
		return
	}
	id := library.LibraryID(catalogs[index])
	key := addons.LibraryKey{AddonID: addonID, CatalogType: body.CatalogType, CatalogID: body.CatalogID}
	switch {
	case custom && body.Data != "":
		data, decodeErr := base64.StdEncoding.DecodeString(body.Data)
		if decodeErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_image")
			return
		}
		err = h.LibraryImages.UploadImage(r.Context(), id, "Primary", data)
	case custom:
		address := strings.TrimSpace(body.URL)
		if parsed, parseErr := url.Parse(address); parseErr != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" ||
			len(address) > 4096 {
			writeError(w, http.StatusBadRequest, "invalid_image_url")
			return
		}
		err = h.LibraryImages.DownloadImage(r.Context(), id, "Primary", address, confined(r))
	}
	if libraryImageError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	// The upload wins; removing it shows no image.
	choice := body.Image
	if custom {
		choice = addons.LibraryImageNone
	}
	if err := h.Addons.SetLibraryImage(r.Context(), scope, key, choice); addonError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !custom {
		if err := h.LibraryImages.DeleteUploadedImage(r.Context(), id, "Primary"); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	h.listLibraries(w, r)
}

// libraryImageError answers the errors of a picture that cannot be kept,
// reporting whether err was one of them.
func libraryImageError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, accounts.ErrInvalidImage):
		writeError(w, http.StatusBadRequest, "invalid_image")
	case errors.Is(err, stremio.ErrPrivateNetwork):
		writeError(w, http.StatusForbidden, "image_private_network")
	case errors.Is(err, stremio.ErrUnreachable), errors.Is(err, stremio.ErrInvalidResponse):
		writeError(w, http.StatusBadGateway, "image_unreachable")
	default:
		return false
	}
	return true
}
