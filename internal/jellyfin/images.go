package jellyfin

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // registers GIF with image.DecodeConfig
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/netfail"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// Artwork relayed from addons' artwork servers, IPTV channel logos
// included.
//
// Each image is downloaded once for every request that asks for it at
// once, on a context of its own: an app that gives up does not cut the
// download short for the others. At most imageHostFetches downloads go to
// one host at once, imageSlowHostFetches to a host that serves an IPTV
// source or its logos, often the provider's panel, and to a host that
// showed it was overloaded, for imageSlowDown. A download that failed
// answers the same for imageFailureLife without asking again, its error
// telling the first one's cause. Downloaded images are kept in
// memory, the most recently used up to imageCacheBytes, and on disk under
// the cache folder's images folder, up to imageDiskBytes, which a restart
// keeps. An app asking for an image much smaller than the original (see
// resizedWidth) gets it resized, kept the same way. Posters are also
// warmed ahead of any request (see warmedposters.go).

const (
	// imageCacheBytes bounds the memory used by recently relayed artwork.
	imageCacheBytes = 128 << 20
	// imageDiskBytes bounds the artwork kept on disk: 1 GB, in the
	// decimal units sizes are shown in, as the source cache's 10 GB.
	imageDiskBytes = 1_000_000_000
	// imageHostFetches bounds the downloads from one host at once, and
	// imageSlowHostFetches those from a host to spare (see host);
	// imageSlowDown is how long a host that showed it was overloaded is
	// spared.
	imageHostFetches     = 16
	imageSlowHostFetches = 4
	imageSlowDown        = 10 * time.Minute
	// imageWarmShare is the share of a host's places that downloads
	// warming posters take at most, one at least: 1 in imageWarmShare.
	imageWarmShare = 4
	// imageFailureLife is how long a failed download answers again without
	// asking, with the same status and cause.
	imageFailureLife = 2 * time.Minute
	// immutableImage is how apps may keep an image they asked for by its
	// tag, which changes with the image.
	immutableImage = "public, max-age=31536000, immutable"
	taggedImage    = "public, max-age=604800"
)

// imageResizes bounds the images resized at once: one for each two of the
// processors Go runs on, two at least. A resize keeps one busy.
var imageResizes = max(2, runtime.GOMAXPROCS(0)/2)

// neverUsed is the use time of an image warmed that no app asked for yet:
// the least recently used of all (see trimDisk).
var neverUsed = time.Unix(0, 0)

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
	failures *cache.Cache[string, downloadError]
	resizes  chan struct{}
	// warmResizes holds a place for each resize warming a poster, half of
	// resizes at most (see warmImage).
	warmResizes chan struct{}

	mu sync.Mutex
	// memory holds the images in memory, most recently used first, by key;
	// size counts their bytes.
	memory map[string]*list.Element
	order  *list.List
	size   int
	// disk holds the images kept on disk, by key; diskSize counts their
	// bytes, which diskBytes bounds: imageDiskBytes, less in tests.
	// warmFull tells that warming lacked room (see roomToWarm).
	disk      map[string]*diskImage
	diskSize  int64
	diskBytes int64
	warmFull  bool
	// warmed holds the keys of each poster warmed, its original's then its
	// sizes' (see warmImage).
	warmed map[string][]string
	// hosts are the downloads under way from each host.
	hosts map[string]*hostFetches
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
		c.failures = cache.New[string, downloadError](5000, imageFailureLife)
		c.resizes = make(chan struct{}, imageResizes)
		c.warmResizes = make(chan struct{}, max(1, imageResizes/2))
		c.memory, c.order = map[string]*list.Element{}, list.New()
		c.disk, c.diskBytes, c.hosts = map[string]*diskImage{}, imageDiskBytes, map[string]*hostFetches{}
		c.warmed = map[string][]string{}
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

// trimDisk deletes the least recently used images beyond diskBytes, down
// to keptDisk; c.mu is held.
func (c *imageCache) trimDisk() {
	if c.diskSize <= c.diskBytes {
		return
	}
	kept := make([]*diskImage, 0, len(c.disk))
	for _, image := range c.disk {
		kept = append(kept, image)
	}
	slices.SortFunc(kept, func(a, b *diskImage) int { return a.used.Compare(b.used) })
	for _, image := range kept {
		if c.diskSize <= c.keptDisk() {
			return
		}
		_ = os.Remove(filepath.Join(c.dir, image.name))
		delete(c.disk, image.name)
		c.diskSize -= image.size
	}
}

// ImageCacheUsage is what the image cache keeps, or what clearing it
// freed: Files images on disk, of Bytes bytes, which Limit bounds, and
// MemoryImages images in memory, of MemoryBytes bytes.
type ImageCacheUsage struct {
	Files        int
	Bytes        int64
	Limit        int64
	MemoryImages int
	MemoryBytes  int64
}

// usage tells what the cache keeps; c.mu is held.
func (c *imageCache) usage() ImageCacheUsage {
	return ImageCacheUsage{Files: len(c.disk), Bytes: c.diskSize, Limit: c.diskBytes, MemoryImages: len(c.memory), MemoryBytes: int64(c.size)}
}

// clear forgets every image kept, in memory and on disk, the downloads
// remembered as failed and the posters warmed, and returns what it freed.
// A download or resize under way keeps its image once it ends, as for any
// request.
func (c *imageCache) clear() ImageCacheUsage {
	c.mu.Lock()
	var freed ImageCacheUsage
	names := make([]string, 0, len(c.disk))
	for name, image := range c.disk {
		names = append(names, name)
		freed.Bytes += image.size
	}
	freed.Files, freed.Limit = len(names), c.diskBytes
	freed.MemoryImages, freed.MemoryBytes = len(c.memory), int64(c.size)
	// The places of images warmed being written stay held (see
	// keepWarmed): only those of the images dropped are given back.
	c.diskSize -= freed.Bytes
	c.disk = map[string]*diskImage{}
	c.memory, c.order, c.size = map[string]*list.Element{}, list.New(), 0
	c.warmed = map[string][]string{}
	c.warmFull = false
	dir := c.dir
	c.mu.Unlock()
	c.failures.DeleteFunc(func(string, downloadError) bool { return true })
	for _, name := range names {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			c.logger.Debug("A cached image could not be deleted", "error", err)
		}
	}
	return freed
}

// ImageCache tells what the image cache keeps: the artwork relayed, its
// resized copies and the images Polyfin draws (see generated.go).
func (h *Handler) ImageCache() ImageCacheUsage {
	h.images.open(h.imageDir(), h.Logger)
	h.images.mu.Lock()
	defer h.images.mu.Unlock()
	return h.images.usage()
}

// ClearImageCache empties the image cache, and returns what it freed.
// Each image is downloaded, resized or drawn again at its next request;
// the posters the libraries' first pages show are warmed again by the
// next warming (see warmPosters). The scrubbing thumbnails and chapter
// images, kept in the database, uploaded artwork, local folders' artwork
// files and the web player's theme are not in it.
func (h *Handler) ClearImageCache() ImageCacheUsage {
	h.images.open(h.imageDir(), h.Logger)
	freed := h.images.clear()
	h.Logger.Info("The image cache was cleared", "images", freed.Files, "bytes", freed.Bytes,
		"in_memory", freed.MemoryImages)
	return freed
}

// keptDisk is what trimDisk leaves on disk, and what warming fills it to
// at most: nine tenths of diskBytes.
func (c *imageCache) keptDisk() int64 { return c.diskBytes * 9 / 10 }

// peek returns the image kept for key, in memory or on disk, without
// making it recently used, nor keeping in memory one read from disk.
func (c *imageCache) peek(key string) (artwork, bool) {
	c.mu.Lock()
	if element, ok := c.memory[key]; ok {
		c.mu.Unlock()
		return element.Value.(*memoryImage).image, true
	}
	kept, ok := c.disk[fileName(key)]
	c.mu.Unlock()
	if !ok {
		return artwork{}, false
	}
	image, err := readImage(filepath.Join(c.dir, kept.name))
	return image, err == nil
}

// roomToWarm reports whether the disk has room for images warmed: it is
// kept, below keptDisk, and refused none since warming started (see
// startWarming).
func (c *imageCache) roomToWarm() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dir != "" && c.diskSize < c.keptDisk() && !c.warmFull
}

// startWarming lets warming fill the disk again (see roomToWarm).
func (c *imageCache) startWarming() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warmFull = false
}

// keepWarmed keeps on disk, not in memory, an image warmed ahead of any
// request, if the disk has room for it below keptDisk: it never makes
// room by deleting others, and refuses the next ones once it lacks room
// (see roomToWarm). It is the least recently used until an app asks for
// it (see get), so the images apps asked for outlast it.
func (c *imageCache) keepWarmed(key string, image artwork) {
	name := fileName(key)
	size := int64(len(image.contentType) + 1 + len(image.body))
	c.mu.Lock()
	if _, ok := c.disk[name]; ok || c.dir == "" {
		c.mu.Unlock()
		return
	}
	if c.diskSize+size > c.keptDisk() {
		c.warmFull = true
		c.mu.Unlock()
		return
	}
	// Its place is held while it is written.
	c.diskSize += size
	c.mu.Unlock()
	written, err := writeImage(c.dir, name, image)
	if err == nil {
		err = os.Chtimes(filepath.Join(c.dir, name), neverUsed, neverUsed)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diskSize -= size
	if err != nil {
		c.logger.Debug("Artwork could not be kept on disk", "error", err)
		return
	}
	if kept, ok := c.disk[name]; ok {
		// An app's request kept it meanwhile.
		c.diskSize -= kept.size
	}
	c.disk[name] = &diskImage{name: name, size: written, used: neverUsed}
	c.diskSize += written
}

// kept reports whether every one of keys is kept on disk.
func (c *imageCache) kept(keys []string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		if _, ok := c.disk[fileName(key)]; !ok {
			return false
		}
	}
	return true
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
	return c.fetchFor(ctx, key, false, produce)
}

// fetchFor is fetch, for warming when warming: an image kept is then not
// made recently used, and the one produce makes is kept as warmed (see
// keepWarmed).
func (c *imageCache) fetchFor(ctx context.Context, key string, warming bool, produce func(context.Context) (artwork, error)) (artwork, error) {
	if warming {
		if image, ok := c.peek(key); ok {
			return image, nil
		}
	} else if image, ok := c.get(key); ok {
		return image, nil
	}
	detached := context.WithoutCancel(ctx)
	results := c.flight.DoChan(key, func() (any, error) {
		image, err := produce(detached)
		switch {
		case err != nil:
		case warming:
			c.keepWarmed(key, image)
		default:
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

// hostFetches are the downloads under way from a host: active of them,
// warming of those warming posters, those waiting woken when one ends, and
// the host spared until slowUntil.
type hostFetches struct {
	active    int
	warming   int
	slowUntil time.Time
	ended     chan struct{}
}

// host waits for a place among the downloads from target's host, and
// returns what gives it back: imageHostFetches places, imageSlowHostFetches
// for a host to spare, iptv telling a host that serves an IPTV source. A
// download warming a poster takes one of those places only while the
// downloads warming take less than 1 in imageWarmShare of them: the others
// stay for apps.
func (c *imageCache) host(ctx context.Context, target string, iptv, warming bool) (func(), error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return func() {}, nil
	}
	for {
		c.mu.Lock()
		fetches, ok := c.hosts[parsed.Host]
		if !ok {
			fetches = &hostFetches{ended: make(chan struct{})}
			c.hosts[parsed.Host] = fetches
		}
		places := imageHostFetches
		if iptv || time.Now().Before(fetches.slowUntil) {
			places = imageSlowHostFetches
		}
		if fetches.active < places && (!warming || fetches.warming < max(1, places/imageWarmShare)) {
			fetches.active++
			if warming {
				fetches.warming++
			}
			c.mu.Unlock()
			return func() {
				c.mu.Lock()
				defer c.mu.Unlock()
				fetches.active--
				if warming {
					fetches.warming--
				}
				close(fetches.ended)
				fetches.ended = make(chan struct{})
			}, nil
		}
		ended := fetches.ended
		c.mu.Unlock()
		select {
		case <-ended:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// spare limits the downloads from target's host for imageSlowDown.
func (c *imageCache) spare(target string) {
	parsed, err := url.Parse(target)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if fetches, ok := c.hosts[parsed.Host]; ok {
		fetches.slowUntil = time.Now().Add(imageSlowDown)
	}
}

// overloaded reports a download failure by which a host shows it gets too
// many requests: a reset connection, or a 429, 502, 503 or 504.
func overloaded(err error) bool {
	switch stremio.StatusOf(err) {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return netfail.Reset(err)
}

// downloadError is a download that failed, with the status it answers.
type downloadError struct {
	status int
	err    error
}

func (e downloadError) Error() string { return e.err.Error() }

// fetchArtwork returns the artwork at target, downloaded once (see
// imageCache); iptv tells that its host serves an IPTV source. Artwork its
// server does not have (404 or 410) is not found, as an image the item
// does not have, for apps to show their placeholder.
func (h *Handler) fetchArtwork(ctx context.Context, target string, confined, iptv bool) (artwork, error) {
	return h.fetchArtworkHeld(ctx, target, confined, iptv, nil)
}

// fetchArtworkHeld is fetchArtwork, the download holding what hold gives
// besides its place among those from its host, when hold is set.
func (h *Handler) fetchArtworkHeld(ctx context.Context, target string, confined, iptv bool, hold func(context.Context) (func(), error)) (artwork, error) {
	return h.downloadArtwork(ctx, target, confined, iptv, false, hold)
}

// downloadArtwork is fetchArtworkHeld, for warming a poster when warming
// (see fetchFor and host).
func (h *Handler) downloadArtwork(ctx context.Context, target string, confined, iptv, warming bool, hold func(context.Context) (func(), error)) (artwork, error) {
	if failed, ok := h.images.failures.Get(target); ok {
		return artwork{}, downloadError{failed.status, fmt.Errorf("the last download failed: %w", failed.err)}
	}
	return h.images.fetchFor(ctx, target, warming, func(ctx context.Context) (artwork, error) {
		release, err := h.images.host(ctx, target, iptv, warming)
		if err != nil {
			return artwork{}, err
		}
		defer release()
		if hold != nil {
			release, err := hold(ctx)
			if err != nil {
				return artwork{}, err
			}
			defer release()
		}
		body, contentType, err := h.Stremio.Image(ctx, target, confined)
		if err != nil {
			status := http.StatusBadGateway
			switch {
			case errors.Is(err, stremio.ErrPrivateNetwork):
				status = http.StatusForbidden
			case stremio.StatusOf(err) == http.StatusNotFound, stremio.StatusOf(err) == http.StatusGone:
				status = http.StatusNotFound
			}
			if overloaded(err) {
				h.images.spare(target)
			}
			failed := downloadError{status, err}
			h.images.failures.Put(target, failed)
			return artwork{}, failed
		}
		return artwork{body: body, contentType: contentType}, nil
	})
}

// maxLocalImageBytes bounds a local folder's image, as relayed artwork is
// bounded: a large backdrop's PNG.
const maxLocalImageBytes = 32 << 20

// localArtwork returns a local folder's image (see
// library.LocalArtworkKey), read from its folder or share once for every
// request that asks for it at once, and kept as relayed artwork is (see
// imageCache): its URL changes with it. An image no longer in its folder,
// or that is no image, is not found.
func (h *Handler) localArtwork(ctx context.Context, target, key string) (artwork, error) {
	return h.localArtworkFor(ctx, target, key, false)
}

// localArtworkFor is localArtwork, for warming a poster when warming (see
// fetchFor).
func (h *Handler) localArtworkFor(ctx context.Context, target, key string, warming bool) (artwork, error) {
	return h.images.fetchFor(ctx, target, warming, func(ctx context.Context) (artwork, error) {
		file, err := h.Library.OpenLocalArtwork(ctx, key)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, localfiles.ErrGone) || errors.Is(err, library.ErrNotFound) {
				status = http.StatusNotFound
			}
			return artwork{}, downloadError{status, err}
		}
		defer file.Close()
		body, err := io.ReadAll(io.LimitReader(file, maxLocalImageBytes+1))
		if err != nil {
			return artwork{}, downloadError{http.StatusBadGateway, err}
		}
		contentType := http.DetectContentType(body)
		switch {
		case len(body) > maxLocalImageBytes:
			return artwork{}, downloadError{http.StatusNotFound, errors.New("the image is too large")}
		case !strings.HasPrefix(contentType, "image/"):
			return artwork{}, downloadError{http.StatusNotFound, errors.New("the file is no image")}
		}
		return artwork{body: body, contentType: contentType}, nil
	})
}

// iptvHost reports whether target's host serves an IPTV source or its
// logos (see library.Service.IPTVHost).
func (h *Handler) iptvHost(ctx context.Context, target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && h.Library != nil && h.Library.IPTVHost(ctx, parsed.Host)
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

// resizedKey is the key an image resized to width is kept by.
func resizedKey(target string, width int) string {
	return target + "\x00width=" + strconv.Itoa(width)
}

// resized returns the image at target resized to width, made once and
// kept (see imageCache), or the original when it cannot be resized.
func (h *Handler) resized(ctx context.Context, target string, original artwork, width int) artwork {
	return h.resizedFor(ctx, target, original, width, false)
}

// resizedFor is resized, for warming a poster when warming: the resize
// then also takes a place among warmResizes, and is kept as warmed (see
// fetchFor).
func (h *Handler) resizedFor(ctx context.Context, target string, original artwork, width int, warming bool) artwork {
	image, err := h.images.fetchFor(ctx, resizedKey(target, width), warming, func(context.Context) (artwork, error) {
		if warming {
			h.images.warmResizes <- struct{}{}
			defer func() { <-h.images.warmResizes }()
		}
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
// uploaded comes first, for any item, and is served from the database; a
// local folder's comes next, read from the folder (see localArtwork). A
// generated image is drawn from the posters it shows (see
// generatedArtwork).
func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	tag := query(r, "tag")
	h.taggedArtwork(w, r, tag, tag != "")
}

// taggedArtwork is image for the artwork whose tag is tag, if any (see
// library.TaggedArtwork). immutable lets the client keep it for good: an
// address that names the tag changes with the image.
func (h *Handler) taggedArtwork(w http.ResponseWriter, r *http.Request, tag string, immutable bool) {
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
	var found library.Artwork
	url, uploaded := h.Library.UploadedArtwork(id, imageType)
	if uploaded {
		found.URL = url
	} else {
		var collection bool
		found, collection, err = h.collectionArtwork(r, id, imageType)
		if !collection {
			found, err = h.Library.Artwork(r.Context(), id, imageType)
		}
	}
	if errors.Is(err, library.ErrNotFound) {
		// Apps show a version opened as an item with its title's artwork.
		if owner, ok := h.Library.VersionOwner(id); ok {
			found, err = h.Library.Artwork(r.Context(), owner, r.PathValue("imageType"))
		}
	}
	if errors.Is(err, library.ErrNotFound) {
		found, err = h.recordingArtwork(r.Context(), id, r.PathValue("imageType"))
	}
	if errors.Is(err, library.ErrNotFound) {
		found = library.Artwork{}
		found.URL, found.Confined, err = h.Library.ProgramArtwork(r.Context(), id, imageType)
	}
	// A badged tag stands for the image of its own tag with badges over
	// it (see badgedTag).
	tag, badges, preview, badged := h.badgedRequest(id, tag)
	// Image requests come without a user: a tag that is not the image found
	// may be that of one in a user's metadata language (see
	// library.TaggedArtwork).
	if tag != "" && !uploaded && found.Generated == nil && (errors.Is(err, library.ErrNotFound) ||
		err == nil && library.ImageTag(found.URL) != tag) {
		if tagged, ok := h.Library.TaggedArtwork(r.Context(), id, tag); ok {
			found, err = tagged, nil
		}
	}
	if errors.Is(err, library.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	url = found.URL
	if owner, ownerType, ok := library.Uploaded(url); ok {
		h.uploadedImage(w, r, owner, ownerType)
		return
	}
	// Badges are drawn over a Primary image Polyfin does not draw, while
	// the settings draw them, or for a preview.
	drawn := badged && found.Generated == nil && strings.EqualFold(imageType, "Primary") &&
		(preview || h.Accounts.Settings().PosterBadges)
	etag := `"` + library.ImageTag(url) + `"`
	if drawn {
		etag = `"` + library.ImageTag(url) + "-" + badges + `"`
	}
	header := w.Header()
	header.Set("ETag", etag)
	// Tags change with the artwork, so a tagged image never changes; an
	// app asking by tag may keep it for good. A badged tag asked for once
	// the badges are off answers the image without them, which the app
	// keeps for a while only; a preview's is not kept.
	switch {
	case drawn && preview:
		header.Set("Cache-Control", "no-store")
	case immutable && (drawn || !badged):
		header.Set("Cache-Control", immutableImage)
	default:
		header.Set("Cache-Control", taggedImage)
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.images.open(h.imageDir(), h.Logger)
	var image artwork
	// source is the address of the image downloaded: url, else its
	// fallback.
	source := url
	// Only an image Polyfin chose is drawn, whatever an artwork URL says.
	if found.Generated != nil {
		image, err = h.generatedArtwork(r.Context(), url, *found.Generated, found.Confined)
	} else if key, ok := library.LocalArtworkKey(url); ok {
		image, err = h.localArtwork(r.Context(), url, key)
	} else {
		// The poster address template is an administrator's: its posters
		// are not confined as the addons' may be.
		image, err = h.fetchArtwork(r.Context(), url, found.Confined && !found.Templated, h.iptvHost(r.Context(), url))
		// The poster address template's poster that cannot be downloaded
		// gives way to the addon's, which apps keep for a while only.
		if err != nil && found.Fallback != "" && r.Context().Err() == nil {
			h.Logger.Debug("The templated poster could not be downloaded", "item", id.String(), "error", err)
			source = found.Fallback
			image, err = h.fetchArtwork(r.Context(), source, found.Confined, h.iptvHost(r.Context(), source))
			if !preview {
				header.Set("Cache-Control", taggedImage)
			}
		}
	}
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
	width := 0
	if resizable(image.contentType) {
		if config, _, err := image.decodeConfig(); err == nil {
			width = resizedWidth(r.URL.Query(), config.Width, config.Height)
		}
	}
	switch {
	case drawn:
		image = h.badged(r.Context(), source, image, badges, width)
	case width > 0:
		image = h.resized(r.Context(), source, image, width)
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
