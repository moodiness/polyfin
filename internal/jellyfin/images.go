package jellyfin

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif" // registers GIF with image.DecodeConfig
	"image/jpeg"
	"image/png"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP with image.Decode
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// Artwork relayed from addons' artwork servers, IPTV channel logos
// included.
//
// Each image is downloaded once for every request that asks for it at
// once, on a context of its own: an app that gives up does not cut the
// download short for the others. At most imageHostFetches downloads go to
// one host at once. A download that failed answers the same for
// imageFailureLife without asking again. Downloaded images are kept in
// memory, the most recently used up to imageCacheBytes, and on disk under
// the cache folder's images folder, up to imageDiskBytes, which a restart
// keeps. An app asking for an image much smaller than the original (see
// resizedWidth) gets it resized, kept the same way.

const (
	// imageCacheBytes bounds the memory used by recently relayed artwork.
	imageCacheBytes = 128 << 20
	// imageDiskBytes bounds the artwork kept on disk.
	imageDiskBytes = 1 << 30
	// imageHostFetches bounds the downloads from one host at once.
	imageHostFetches = 4
	// imageFailureLife is how long a failed download answers again without
	// asking.
	imageFailureLife = 2 * time.Minute
	// imageResizes bounds the images resized at once.
	imageResizes = 2
	// immutableImage is how apps may keep an image they asked for by its
	// tag, which changes with the image.
	immutableImage = "public, max-age=31536000, immutable"
	taggedImage    = "public, max-age=604800"
)

// artwork is a downloaded image.
type artwork struct {
	body        []byte
	contentType string
}

// imageCache keeps the artwork relayed recently, in memory and on disk,
// and shares and bounds its downloads. Its zero value is ready to use
// once opened.
type imageCache struct {
	once     sync.Once
	dir      string
	logger   *slog.Logger
	flight   singleflight.Group
	failures *cache.Cache[string, int]
	resizes  chan struct{}

	mu sync.Mutex
	// memory holds the images in memory, most recently used first, by key;
	// size counts their bytes.
	memory map[string]*list.Element
	order  *list.List
	size   int
	// disk holds the images kept on disk, by key; diskSize counts their
	// bytes.
	disk     map[string]*diskImage
	diskSize int64
	// hosts holds a place for each download from a host.
	hosts map[string]chan struct{}
}

type memoryImage struct {
	key   string
	image artwork
}

type diskImage struct {
	name string
	size int64
	used time.Time
}

// open prepares the cache, keeping images on disk under dir, none when
// dir is empty, and reads what the disk keeps already.
func (c *imageCache) open(dir string, logger *slog.Logger) {
	c.once.Do(func() {
		c.logger = logger
		c.failures = cache.New[string, int](5000, imageFailureLife)
		c.resizes = make(chan struct{}, imageResizes)
		c.memory, c.order = map[string]*list.Element{}, list.New()
		c.disk, c.hosts = map[string]*diskImage{}, map[string]chan struct{}{}
		if dir == "" {
			return
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			logger.Warn("Artwork cannot be kept on disk", "folder", dir, "error", err)
			return
		}
		c.dir = dir
		entries, err := os.ReadDir(dir)
		if err != nil {
			logger.Warn("The artwork kept on disk cannot be read", "folder", dir, "error", err)
			return
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil || !entry.Type().IsRegular() {
				continue
			}
			if strings.HasPrefix(entry.Name(), ".") {
				// A file left half written.
				_ = os.Remove(filepath.Join(dir, entry.Name()))
				continue
			}
			c.disk[entry.Name()] = &diskImage{name: entry.Name(), size: info.Size(), used: info.ModTime()}
			c.diskSize += info.Size()
		}
		c.mu.Lock()
		c.trimDisk()
		c.mu.Unlock()
	})
}

// fileName is the file an image is kept in on disk.
func fileName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

// get returns the image kept for key, in memory or on disk.
func (c *imageCache) get(key string) (artwork, bool) {
	c.mu.Lock()
	if element, ok := c.memory[key]; ok {
		c.order.MoveToFront(element)
		c.mu.Unlock()
		return element.Value.(*memoryImage).image, true
	}
	kept, ok := c.disk[fileName(key)]
	if ok {
		now := time.Now()
		// The file's time tells the use after a restart; it is not
		// written for every use.
		if now.Sub(kept.used) > time.Hour {
			_ = os.Chtimes(filepath.Join(c.dir, kept.name), now, now)
		}
		kept.used = now
	}
	c.mu.Unlock()
	if !ok {
		return artwork{}, false
	}
	image, err := readImage(filepath.Join(c.dir, kept.name))
	if err != nil {
		c.mu.Lock()
		if c.disk[kept.name] == kept {
			delete(c.disk, kept.name)
			c.diskSize -= kept.size
		}
		c.mu.Unlock()
		return artwork{}, false
	}
	c.mu.Lock()
	c.remember(key, image)
	c.mu.Unlock()
	return image, true
}

// put keeps image for key, in memory and on disk.
func (c *imageCache) put(key string, image artwork) {
	c.mu.Lock()
	c.remember(key, image)
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	name := fileName(key)
	size, err := writeImage(c.dir, name, image)
	if err != nil {
		c.logger.Debug("Artwork could not be kept on disk", "error", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if kept, ok := c.disk[name]; ok {
		c.diskSize -= kept.size
	}
	c.disk[name] = &diskImage{name: name, size: size, used: time.Now()}
	c.diskSize += size
	c.trimDisk()
}

// remember keeps image in memory, forgetting the least recently used
// beyond imageCacheBytes; c.mu is held.
func (c *imageCache) remember(key string, image artwork) {
	if element, ok := c.memory[key]; ok {
		c.order.MoveToFront(element)
		return
	}
	if len(image.body) > imageCacheBytes/4 {
		return
	}
	c.memory[key] = c.order.PushFront(&memoryImage{key: key, image: image})
	c.size += len(image.body)
	for c.size > imageCacheBytes {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		kept := oldest.Value.(*memoryImage)
		delete(c.memory, kept.key)
		c.size -= len(kept.image.body)
	}
}

// trimDisk deletes the least recently used images beyond imageDiskBytes;
// c.mu is held.
func (c *imageCache) trimDisk() {
	if c.diskSize <= imageDiskBytes {
		return
	}
	kept := make([]*diskImage, 0, len(c.disk))
	for _, image := range c.disk {
		kept = append(kept, image)
	}
	slices.SortFunc(kept, func(a, b *diskImage) int { return a.used.Compare(b.used) })
	for _, image := range kept {
		if c.diskSize <= imageDiskBytes*9/10 {
			return
		}
		_ = os.Remove(filepath.Join(c.dir, image.name))
		delete(c.disk, image.name)
		c.diskSize -= image.size
	}
}

// readImage reads an image file: its content type on the first line, then
// its bytes.
func readImage(path string) (artwork, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return artwork{}, err
	}
	contentType, body, ok := bytes.Cut(data, []byte("\n"))
	if !ok || !bytes.HasPrefix(contentType, []byte("image/")) {
		return artwork{}, fs.ErrInvalid
	}
	return artwork{body: body, contentType: string(contentType)}, nil
}

// writeImage writes an image file whole, or not at all, and returns its
// size.
func writeImage(dir, name string, image artwork) (int64, error) {
	file, err := os.CreateTemp(dir, ".image-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(append([]byte(image.contentType+"\n"), image.body...))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}
	if err := os.Rename(file.Name(), filepath.Join(dir, name)); err != nil {
		return 0, err
	}
	return int64(len(image.contentType) + 1 + len(image.body)), nil
}

// fetch returns the image kept for key, else the one produce makes, once
// for all the callers at once, on a context the callers giving up do not
// cancel; each caller waits on its own ctx. What produce makes is kept.
func (c *imageCache) fetch(ctx context.Context, key string, produce func(context.Context) (artwork, error)) (artwork, error) {
	if image, ok := c.get(key); ok {
		return image, nil
	}
	detached := context.WithoutCancel(ctx)
	results := c.flight.DoChan(key, func() (any, error) {
		image, err := produce(detached)
		if err == nil {
			c.put(key, image)
		}
		return image, err
	})
	select {
	case result := <-results:
		if result.Err != nil {
			return artwork{}, result.Err
		}
		return result.Val.(artwork), nil
	case <-ctx.Done():
		return artwork{}, ctx.Err()
	}
}

// host waits for a place among the downloads from target's host, and
// returns what gives it back.
func (c *imageCache) host(ctx context.Context, target string) (func(), error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return func() {}, nil
	}
	c.mu.Lock()
	places, ok := c.hosts[parsed.Host]
	if !ok {
		places = make(chan struct{}, imageHostFetches)
		c.hosts[parsed.Host] = places
	}
	c.mu.Unlock()
	select {
	case places <- struct{}{}:
		return func() { <-places }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// downloadError is a download that failed, with the status it answers.
type downloadError struct {
	status int
	err    error
}

func (e downloadError) Error() string { return e.err.Error() }

// fetchArtwork returns the artwork at target, downloaded once (see
// imageCache).
func (h *Handler) fetchArtwork(ctx context.Context, target string, confined bool) (artwork, error) {
	if status, failed := h.images.failures.Get(target); failed {
		return artwork{}, downloadError{status, errors.New("the last download failed")}
	}
	return h.images.fetch(ctx, target, func(ctx context.Context) (artwork, error) {
		release, err := h.images.host(ctx, target)
		if err != nil {
			return artwork{}, err
		}
		defer release()
		body, contentType, err := h.Stremio.Image(ctx, target, confined)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, stremio.ErrPrivateNetwork) {
				status = http.StatusForbidden
			}
			h.images.failures.Put(target, status)
			return artwork{}, downloadError{status, err}
		}
		return artwork{body: body, contentType: contentType}, nil
	})
}

// widthSteps are the widths images are resized to: the one asked for,
// rounded up, so that apps asking for nearby sizes share them.
var widthSteps = []int{120, 160, 240, 320, 400, 480, 640, 800, 960, 1280, 1600, 1920, 2560, 3200}

// resizedWidth tells the width to resize an image of width by height to
// for what query asks: Jellyfin's maxWidth, maxHeight, fillWidth,
// fillHeight, width and height, bounds the image fits in keeping its
// proportions, named in any case. It is 0 when the image is to be sent as
// it is: when nothing is asked, or the size asked for, rounded up to
// widthSteps, is not under three quarters of the image's width.
func resizedWidth(query url.Values, width, height int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	scale := math.Inf(1)
	for name, values := range query {
		side := 0
		switch strings.ToLower(name) {
		case "maxwidth", "width", "fillwidth":
			side = width
		case "maxheight", "height", "fillheight":
			side = height
		default:
			continue
		}
		if value, err := strconv.Atoi(values[0]); err == nil && value > 0 {
			scale = min(scale, float64(value)/float64(side))
		}
	}
	if scale >= 1 {
		return 0
	}
	wanted := int(math.Ceil(float64(width) * scale))
	i, _ := slices.BinarySearch(widthSteps, wanted)
	if i == len(widthSteps) {
		return 0
	}
	if step := widthSteps[i]; step*4 < width*3 {
		return step
	}
	return 0
}

// resized returns the image at target resized to width, made once and
// kept (see imageCache), or the original when it cannot be resized.
func (h *Handler) resized(ctx context.Context, target string, original artwork, width int) artwork {
	image, err := h.images.fetch(ctx, target+"\x00width="+strconv.Itoa(width), func(context.Context) (artwork, error) {
		h.images.resizes <- struct{}{}
		defer func() { <-h.images.resizes }()
		return resize(original, width)
	})
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Debug("Artwork could not be resized", "error", err)
		}
		return original
	}
	return image
}

// resize scales an image down to width, keeping its proportions: as a
// PNG when it was one, which may be transparent, else as a JPEG.
func resize(original artwork, width int) (artwork, error) {
	source, format, err := image.Decode(bytes.NewReader(original.body))
	if err != nil {
		return artwork{}, err
	}
	bounds := source.Bounds()
	height := max(1, int(math.Round(float64(bounds.Dy())*float64(width)/float64(bounds.Dx()))))
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), source, bounds, xdraw.Src, nil)
	var out bytes.Buffer
	if format == "png" {
		if err := png.Encode(&out, scaled); err != nil {
			return artwork{}, err
		}
		return artwork{body: out.Bytes(), contentType: "image/png"}, nil
	}
	if err := jpeg.Encode(&out, scaled, &jpeg.Options{Quality: 85}); err != nil {
		return artwork{}, err
	}
	return artwork{body: out.Bytes(), contentType: "image/jpeg"}, nil
}

// resizable tells the formats resized: animated GIFs and formats Go does
// not decode are sent as they are.
func resizable(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	}
	return false
}

// image relays an item's artwork. Polyfin downloads it rather than
// redirecting: some apps (Infuse) do not follow image redirects, and apps
// may not reach the addresses addons use. Artwork an administrator
// uploaded comes first, for any item, and is served from the database.
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
	imageType := r.PathValue("imageType")
	url, uploaded := h.Library.UploadedArtwork(id, imageType)
	var confined, collection bool
	if !uploaded {
		url, confined, collection, err = h.collectionArtwork(r, id, imageType)
		if !collection {
			url, confined, err = h.Library.Artwork(r.Context(), id, imageType)
		}
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
	if owner, ownerType, ok := library.Uploaded(url); ok {
		h.uploadedImage(w, r, owner, ownerType)
		return
	}
	tag := library.ImageTag(url)
	header := w.Header()
	header.Set("ETag", `"`+tag+`"`)
	// Tags change with the artwork, so a tagged image never changes; an
	// app asking by tag may keep it for good.
	if query(r, "tag") != "" {
		header.Set("Cache-Control", immutableImage)
	} else {
		header.Set("Cache-Control", taggedImage)
	}
	if r.Header.Get("If-None-Match") == `"`+tag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.images.open(h.imageDir(), h.Logger)
	image, err := h.fetchArtwork(r.Context(), url, confined)
	if err != nil {
		header.Del("ETag")
		header.Del("Cache-Control")
		status := http.StatusBadGateway
		if failed, ok := errors.AsType[downloadError](err); ok {
			status = failed.status
		}
		if r.Context().Err() == nil {
			h.Logger.Debug("Artwork could not be downloaded", "item", id.String(), "error", err)
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

// decodeConfig reads the image's dimensions.
func (a artwork) decodeConfig() (image.Config, string, error) {
	return image.DecodeConfig(bytes.NewReader(a.body))
}

// imageDir is where relayed artwork is kept on disk, none without a
// cache folder.
func (h *Handler) imageDir() string {
	if h.CacheDir == "" {
		return ""
	}
	return filepath.Join(h.CacheDir, "images")
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
