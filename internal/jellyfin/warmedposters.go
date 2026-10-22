package jellyfin

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Posters warmed ahead.
//
// Once Polyfin read the libraries' first pages, every hour (see
// library.Service.Warm), it downloads the posters of the titles they list,
// warmedTitles of each library at most, and resizes them to the sizes
// jellyfin-web asks for its cards (warmedSizes), so that the first view of
// a home row or a library is served from the image cache rather than
// waiting for the artwork server. The titles are those of the libraries
// some user sees, as each user sees them (see library.WarmedTitles).
//
// Warming is bounded: its downloads take a share of each host's places
// (see imageCache.host), its resizes half of their places, and what it
// keeps fills the disk up to what trimDisk leaves, never deleting another
// image for room, and goes first when apps fill it (see keepWarmed). It
// keeps nothing in memory, and nothing at all without a cache folder.
// Polyfin's generated and uploaded images are not warmed: they are not
// downloaded.
//
// Jellyfin downloads a title's images when it scans its library, and
// resizes them the first time an app asks. Polyfin has no scan: it warms
// what apps show first, and resizes it ahead too, since the catalogs it
// lists change every hour.

const (
	// warmedTitles is how many titles of each library's first page get
	// their posters warmed: a page of jellyfin-web's library grid.
	warmedTitles = 100
	// posterWarmers bounds the posters warmed at once.
	posterWarmers = 8
)

// warmedSizes are the sizes jellyfin-web asks for a poster's card in a
// window 1,400 pixels wide, as its card builder computes them: in a home
// row, then in a library's grid, at 1 and at 2 device pixels per pixel.
// Nearby sizes share their resizes (see resizedWidth): for a poster of
// 2 by 3, they are 240, 480 and 400 pixels wide.
var warmedSizes = []url.Values{
	{"fillWidth": {"217"}, "fillHeight": {"326"}},
	{"fillWidth": {"434"}, "fillHeight": {"652"}},
	{"fillWidth": {"200"}, "fillHeight": {"300"}},
	{"fillWidth": {"400"}, "fillHeight": {"600"}},
}

// warmedPoster is a poster to warm: relayed from url, or read from a local
// folder by local when set.
type warmedPoster struct {
	url            string
	local          string
	confined, iptv bool
}

// warmPosters warms the posters of the first pages the libraries' users
// see.
func (h *Handler) warmPosters(ctx context.Context) {
	dir := h.imageDir()
	if dir == "" {
		return
	}
	h.images.open(dir, h.Logger)
	started := time.Now()
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		h.Logger.Debug("Posters could not be warmed", "error", err)
		return
	}
	seen := map[accounts.ID]bool{}
	var posters []warmedPoster
	for _, user := range users {
		if user.Refused(started) {
			continue
		}
		titles, err := h.Library.WarmedTitles(ctx, user, warmedTitles)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			h.Logger.Debug("The titles whose posters are warmed could not be listed", "user", user.Name, "error", err)
			continue
		}
		for _, id := range titles {
			if seen[id] {
				continue
			}
			seen[id] = true
			if poster, ok := h.posterOf(ctx, id); ok {
				posters = append(posters, poster)
			}
		}
	}
	warmed := h.warmImages(ctx, posters)
	h.Logger.Debug("Posters warmed", "titles", len(seen), "posters", len(posters), "warmed", warmed,
		"took", time.Since(started).Round(time.Millisecond))
}

// posterOf returns the poster of a title as apps are served it (see
// taggedArtwork), unless it is not downloaded: generated or uploaded.
func (h *Handler) posterOf(ctx context.Context, id accounts.ID) (warmedPoster, bool) {
	found, err := h.Library.Artwork(ctx, id, "Primary")
	if err != nil || found.Generated != nil || found.URL == "" {
		return warmedPoster{}, false
	}
	if _, _, uploaded := library.Uploaded(found.URL); uploaded {
		return warmedPoster{}, false
	}
	if key, ok := library.LocalArtworkKey(found.URL); ok {
		return warmedPoster{url: found.URL, local: key}, true
	}
	if !strings.HasPrefix(found.URL, "http://") && !strings.HasPrefix(found.URL, "https://") {
		return warmedPoster{}, false
	}
	return warmedPoster{url: found.URL, confined: found.Confined, iptv: h.iptvHost(ctx, found.URL)}, true
}

// warmImages warms posters, posterWarmers at once, until the disk has no
// room left for them, and returns how many are warm. It then forgets the
// posters warmed before that it was not given.
func (h *Handler) warmImages(ctx context.Context, posters []warmedPoster) int {
	h.images.startWarming()
	var group errgroup.Group
	group.SetLimit(posterWarmers)
	var mu sync.Mutex
	warmed := map[string][]string{}
	for _, poster := range posters {
		if ctx.Err() != nil || !h.images.roomToWarm() {
			break
		}
		group.Go(func() error {
			if keys, ok := h.warmImage(ctx, poster); ok {
				mu.Lock()
				warmed[poster.url] = keys
				mu.Unlock()
			}
			return nil
		})
	}
	_ = group.Wait()
	h.images.mu.Lock()
	h.images.warmed = warmed
	h.images.mu.Unlock()
	return len(warmed)
}

// warmImage keeps a poster and its sizes for jellyfin-web's cards, unless
// they are kept already, and returns their keys once they all are.
func (h *Handler) warmImage(ctx context.Context, poster warmedPoster) ([]string, bool) {
	h.images.mu.Lock()
	keys, ok := h.images.warmed[poster.url]
	h.images.mu.Unlock()
	if ok && h.images.kept(keys) {
		return keys, true
	}
	if !h.images.roomToWarm() {
		return nil, false
	}
	var original artwork
	var err error
	if poster.local != "" {
		original, err = h.localArtworkFor(ctx, poster.url, poster.local, true)
	} else {
		original, err = h.downloadArtwork(ctx, poster.url, poster.confined, poster.iptv, true, nil)
	}
	if err != nil {
		return nil, false
	}
	keys = []string{poster.url}
	if resizable(original.contentType) {
		config, _, err := original.decodeConfig()
		if err != nil {
			return nil, false
		}
		for _, size := range warmedSizes {
			width := resizedWidth(size, config.Width, config.Height)
			if width == 0 || slices.Contains(keys, resizedKey(poster.url, width)) {
				continue
			}
			key := resizedKey(poster.url, width)
			keys = append(keys, key)
			if !h.images.kept([]string{key}) && h.images.roomToWarm() && ctx.Err() == nil {
				h.resizedFor(ctx, poster.url, original, width, true)
			}
		}
	}
	return keys, h.images.kept(keys)
}
