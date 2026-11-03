package jellyfin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/moodiness/polyfin/internal/boxset"
	"github.com/moodiness/polyfin/internal/library"
)

// The artwork of the TMDB collections Silo's apps show as box sets (see
// the boxset package): GET /Polyfin/BoxSets/{collectionId}/Images/Primary
// or Backdrop serves a collection's poster or backdrop, downloaded from
// TMDB's image server once and kept, resized to the width asked, as the
// artwork of titles is (see image), so that apps never reach TMDB. Like
// titles' artwork, it needs no credentials: a collection's artwork is
// TMDB's, the same for every user.
func (h *Handler) boxSetImageRoutes(rt *router) {
	rt.handle(http.MethodGet, "/Polyfin/BoxSets/{collectionId}/Images/{imageType}", http.HandlerFunc(h.boxSetImage))
}

func (h *Handler) boxSetImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("collectionId"), 10, 64)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	url, err := h.BoxSets.Image(r.Context(), id, r.PathValue("imageType"))
	if errors.Is(err, boxset.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	header := w.Header()
	etag := `"` + library.ImageTag(url) + `"`
	header.Set("ETag", etag)
	header.Set("Cache-Control", taggedImage)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.images.open(h.imageDir(), h.Logger)
	image, err := h.fetchArtwork(r.Context(), url, false, false)
	if err != nil {
		header.Del("ETag")
		header.Del("Cache-Control")
		status := http.StatusBadGateway
		if failed, ok := errors.AsType[downloadError](err); ok {
			status = failed.status
		}
		if r.Context().Err() == nil {
			h.Logger.Debug("A box set's artwork could not be downloaded", "collection", id, "error", err)
		}
		w.WriteHeader(status)
		return
	}
	if resizable(image.contentType) {
		if config, _, err := image.decodeConfig(); err == nil {
			if width := resizedWidth(r.URL.Query(), config.Width, config.Height); width > 0 {
				image = h.resized(r.Context(), url, image, width)
			}
		}
	}
	header.Set("Content-Type", image.contentType)
	header.Set("Content-Length", strconv.Itoa(len(image.body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(image.body)
	}
}
