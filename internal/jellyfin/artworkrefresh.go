package jellyfin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The picture behind an artwork address may change while the address
// stays the same: a poster showing ratings, or the poster address
// template's. Apps keep an image by its tag, which would not change with
// it. So an image relayed is served as kept for imageMaxAge after it was
// downloaded or checked; past that, a request still gets it at once, and
// its server is asked in the background whether it changed: with the
// ETag and Last-Modified it gave, answered 304 with no picture when it
// did not, else by downloading it again. Only the images apps still ask
// for are asked, each once per imageMaxAge at most, among the downloads
// of its host's that warming may take (see host). A picture that changed
// replaces the one kept, and its URL is stamped with its bytes' sum (see
// library.StampImage): its tag changes, and apps fetch the new picture. A
// picture that did not change keeps its tag, and the apps their copies.

const (
	// stampsFile is the file of the images folder the stamps are kept in.
	stampsFile = "stamps.json"
	// maxImageStamps bounds the stamps kept: past it, the oldest is
	// forgotten, and its URL gets its own tag back.
	maxImageStamps = 50_000
)

// due reports whether image, downloaded, is to be asked of its server
// whether it changed: checked imageMaxAge ago, or never.
func (c *imageCache) due(image artwork) bool {
	return image.checked.IsZero() || c.now().Sub(image.checked) >= imageMaxAge
}

// fresh reports whether the image kept for key is known to have been
// checked less than imageMaxAge ago, without reading it from disk.
func (c *imageCache) fresh(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.memory[key]; ok {
		return !c.due(element.Value.(*memoryImage).image)
	}
	if kept, ok := c.disk[fileName(key)]; ok {
		return !c.due(artwork{checked: kept.checked})
	}
	return false
}

// refreshKept asks in the background the server of the image kept for
// target, confined or not, whether it changed, when it was checked
// imageMaxAge ago (see refreshArtwork). An app that keeps the image asked
// whether it changed.
func (h *Handler) refreshKept(ctx context.Context, target string, confined bool) {
	if h.images.fresh(artworkKey(target, confined)) {
		return
	}
	detached := context.WithoutCancel(ctx)
	h.images.refreshes.Go(func() {
		h.refreshArtwork(detached, target, confined, h.iptvHost(detached, target))
	})
}

// refreshArtwork asks in the background the server of the image kept for
// target, confined or not, whether it changed, once for all the requests
// that find it due at once (see revalidate); iptv tells that its host
// serves an IPTV source.
func (h *Handler) refreshArtwork(ctx context.Context, target string, confined, iptv bool) {
	c := &h.images
	key := artworkKey(target, confined)
	c.mu.Lock()
	if c.refreshing[key] {
		c.mu.Unlock()
		return
	}
	c.refreshing[key] = true
	c.mu.Unlock()
	detached := context.WithoutCancel(ctx)
	c.refreshes.Go(func() {
		defer func() {
			c.mu.Lock()
			delete(c.refreshing, key)
			c.mu.Unlock()
		}()
		h.revalidate(detached, target, confined, iptv)
	})
}

// revalidate asks the server of the image kept for target, confined or
// not, whether it changed, if it is due (see due), and keeps the answer.
// A server that fails is asked again imageMaxAge later, the image kept
// served meanwhile; a host that asks to wait is asked nothing. An image
// kept before check times were is not asked about: it is given the time
// of its first request since, and checked imageMaxAge later, so that the
// images kept are not all asked about at once after an upgrade.
func (h *Handler) revalidate(ctx context.Context, target string, confined, iptv bool) {
	c := &h.images
	key := artworkKey(target, confined)
	kept, ok := c.peek(key)
	if !ok || !c.due(kept) {
		return
	}
	if kept.checked.IsZero() {
		kept.checked = c.now()
		c.put(key, kept)
		return
	}
	release, err := c.host(ctx, target, iptv, true)
	if err != nil {
		return
	}
	picture, err := h.Stremio.ImageSince(ctx, target, confined, kept.Validators)
	release()
	checked := kept
	checked.checked = c.now()
	switch {
	case err != nil:
		if wait, ok := stremio.RetryAfter(err); ok {
			c.wait(target, min(wait, imageHostWaitMost))
		}
		if overloaded(err) {
			c.spare(target)
		}
		h.Logger.Debug("Whether an image changed could not be asked", "error", err)
	case picture.Unchanged, bytes.Equal(picture.Body, kept.body):
		checked.Validators = picture.Validators
	default:
		// The new picture is kept before its tag is given, so that a
		// request by the new tag never gets the picture before.
		c.put(key, artwork{body: picture.Body, contentType: picture.Type, Validators: picture.Validators, checked: checked.checked})
		c.drop(artworkKey(target, !confined))
		c.stamps.record(target, pictureStamp(picture.Body), checked.checked)
		return
	}
	c.put(key, checked)
}

// pictureStamp is the stamp of a picture that changed behind its URL: a
// sum of its bytes, the same for the same picture.
func pictureStamp(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:8])
}

// imageStamps keeps the stamps of the pictures that changed behind their
// URL (see library.StampImage), by URL, in stampsFile, so that their tags
// outlast a restart and the image cache: an app that kept the picture
// before the change would show it again under the URL's own tag. Clearing
// the image cache keeps them for that reason.
type imageStamps struct {
	once   sync.Once
	mu     sync.Mutex
	path   string
	logger *slog.Logger
	stamps map[string]keptStamp
	// saving tells that the stamps are being written, and dirty that
	// they changed since.
	saving, dirty bool
}

// keptStamp is a stamp, set at At.
type keptStamp struct {
	Stamp string    `json:"stamp"`
	At    time.Time `json:"at"`
}

// load reads the stamps kept in dir, none when dir is empty, and gives
// them to the tags.
func (s *imageStamps) load(dir string, logger *slog.Logger) {
	s.once.Do(func() {
		s.logger = logger
		s.stamps = map[string]keptStamp{}
		if dir == "" {
			return
		}
		s.path = filepath.Join(dir, stampsFile)
		data, err := os.ReadFile(s.path)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err == nil {
			err = json.Unmarshal(data, &s.stamps)
		}
		if err != nil {
			logger.Warn("The tags of the images that changed cannot be read", "error", err)
			s.stamps = map[string]keptStamp{}
			return
		}
		for url, kept := range s.stamps {
			library.StampImage(url, kept.Stamp)
		}
	})
}

// record stamps the URL whose picture changed, at at, and keeps the
// stamps.
func (s *imageStamps) record(url, stamp string, at time.Time) {
	s.mu.Lock()
	s.stamps[url] = keptStamp{stamp, at}
	if len(s.stamps) > maxImageStamps {
		var oldest string
		for candidate, stamped := range s.stamps {
			if oldest == "" || stamped.At.Before(s.stamps[oldest].At) {
				oldest = candidate
			}
		}
		delete(s.stamps, oldest)
		library.StampImage(oldest, "")
	}
	s.mu.Unlock()
	library.StampImage(url, stamp)
	s.save()
}

// save writes the stamps whole, once for all the changes made while they
// are written.
func (s *imageStamps) save() {
	s.mu.Lock()
	if s.path == "" {
		s.mu.Unlock()
		return
	}
	if s.saving {
		s.dirty = true
		s.mu.Unlock()
		return
	}
	s.saving = true
	for {
		data, err := json.Marshal(s.stamps)
		s.dirty = false
		s.mu.Unlock()
		if err == nil {
			err = writeFileWhole(s.path, data)
		}
		if err != nil {
			s.logger.Warn("The tags of the images that changed could not be kept", "error", err)
		}
		s.mu.Lock()
		if !s.dirty {
			s.saving = false
			s.mu.Unlock()
			return
		}
	}
}

// writeFileWhole writes data to path whole, or not at all.
func writeFileWhole(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".stamps-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
