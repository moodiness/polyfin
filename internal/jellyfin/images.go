package jellyfin

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// imageCacheBytes bounds the memory used by recently relayed artwork.
const imageCacheBytes = 128 << 20

// artwork is a downloaded image.
type artwork struct {
	body        []byte
	contentType string
}

// imageCache keeps recently relayed artwork, oldest evicted first once the
// byte budget is exceeded.
type imageCache struct {
	mu    sync.Mutex
	size  int
	order []string
	items map[string]artwork
}

func (c *imageCache) get(url string) (artwork, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	image, ok := c.items[url]
	return image, ok
}

func (c *imageCache) put(url string, image artwork) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]artwork{}
	}
	if _, ok := c.items[url]; ok || len(image.body) > imageCacheBytes/4 {
		return
	}
	c.items[url] = image
	c.order = append(c.order, url)
	c.size += len(image.body)
	for c.size > imageCacheBytes && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.size -= len(c.items[oldest].body)
		delete(c.items, oldest)
	}
}

// image relays an item's artwork. Polyfin downloads it rather than
// redirecting: some apps (Infuse) do not follow image redirects, and apps
// may not reach the addresses addons use.
func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	id, err := accounts.ParseID(r.PathValue("itemId"))
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	if strings.EqualFold(r.PathValue("imageType"), "Chapter") {
		h.chapterImage(w, r, id)
		return
	}
	if index := r.PathValue("imageIndex"); index != "" && index != "0" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	url, confined, collection, err := h.collectionArtwork(r, id, r.PathValue("imageType"))
	if !collection {
		url, confined, err = h.Library.Artwork(r.Context(), id, r.PathValue("imageType"))
	}
	if errors.Is(err, library.ErrNotFound) {
		// Apps show a version opened as an item with its title's artwork.
		if owner, ok := h.Library.VersionOwner(id); ok {
			url, confined, err = h.Library.Artwork(r.Context(), owner, r.PathValue("imageType"))
		}
	}
	if errors.Is(err, library.ErrNotFound) {
		url, confined, err = h.recordingArtwork(r.Context(), id, r.PathValue("imageType"))
	}
	if errors.Is(err, library.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	tag := library.ImageTag(url)
	if r.Header.Get("If-None-Match") == `"`+tag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	image, ok := h.images.get(url)
	if !ok {
		body, contentType, err := h.Stremio.Image(r.Context(), url, confined)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, stremio.ErrPrivateNetwork) {
				status = http.StatusForbidden
			}
			h.Logger.Debug("Artwork could not be downloaded", "item", id.String(), "error", err)
			w.WriteHeader(status)
			return
		}
		image = artwork{body: body, contentType: contentType}
		h.images.put(url, image)
	}
	header := w.Header()
	header.Set("Content-Type", image.contentType)
	header.Set("Content-Length", strconv.Itoa(len(image.body)))
	header.Set("ETag", `"`+tag+`"`)
	// Tags change with the artwork, so a tagged image never changes.
	header.Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(image.body)
	}
}

// chapterImage serves the image of a chapter, by its index: of the version
// the item was opened as, or of the title's version whose image has the
// tag asked, else of the one used last. Like artwork, chapter images are
// served without credentials, as Jellyfin serves them: apps load them
// with none, by the tags of the items users can open.
func (h *Handler) chapterImage(w http.ResponseWriter, r *http.Request, id accounts.ID) {
	index, err := strconv.Atoi(r.PathValue("imageIndex"))
	if err != nil || index < 0 || h.Thumbnails == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	image, err := h.Thumbnails.ChapterImageOf(r.Context(), id, index, query(r, "tag"))
	switch {
	case errors.Is(err, thumbnails.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	if r.Header.Get("If-None-Match") == `"`+image.Tag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "image/jpeg")
	header.Set("ETag", `"`+image.Tag+`"`)
	header.Set("Cache-Control", "public, max-age=604800")
	http.ServeContent(w, r, "", image.Made, bytes.NewReader(image.Data))
}
