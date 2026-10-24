package admin

import (
	"net/http"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/jellyfin"
)

// fakeImageCache keeps usage until it is cleared.
type fakeImageCache struct {
	mu    sync.Mutex
	usage jellyfin.ImageCacheUsage
}

func (c *fakeImageCache) ImageCache() jellyfin.ImageCacheUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usage
}

func (c *fakeImageCache) ClearImageCache() jellyfin.ImageCacheUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	freed := c.usage
	c.usage = jellyfin.ImageCacheUsage{Limit: freed.Limit}
	return freed
}

// System › Health tells what the image cache keeps, and an administrator
// empties it there, told what it freed; a member cannot.
func TestAdministratorsClearTheImageCache(t *testing.T) {
	t.Parallel()
	images := &fakeImageCache{usage: jellyfin.ImageCacheUsage{Files: 3, Bytes: 3000, Limit: 1 << 30, MemoryImages: 2, MemoryBytes: 2000}}
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) { o.Health = HealthSources{Images: images} })
	admin := api.signedIn("root", true)
	member := api.signedIn("member", false)
	var health healthJSON
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || health.Images == nil ||
		*health.Images != (imageCacheJSON{Files: 3, Used: 3000, Limit: 1 << 30, MemoryImages: 2, MemoryBytes: 2000}) {
		t.Fatalf("health: %d %+v", status, health.Images)
	}
	if status := member.raw(http.MethodPost, "/health/images/clear", "", nil); status != http.StatusForbidden {
		t.Errorf("a member clearing: %d", status)
	}
	var freed imageCacheJSON
	if status := admin.raw(http.MethodPost, "/health/images/clear", "", &freed); status != http.StatusOK || freed.Files != 3 || freed.Used != 3000 ||
		freed.MemoryImages != 2 {
		t.Errorf("clearing: %d %+v", status, freed)
	}
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || health.Images.Files != 0 || health.Images.Used != 0 {
		t.Errorf("after clearing: %+v", health.Images)
	}
}
