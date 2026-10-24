package jellyfin

import (
	"os"
	"path/filepath"
	"testing"
)

// Clearing the image cache drops the artwork relayed and resized, in
// memory and on disk, and the downloads remembered as failed, and tells
// what it freed: each image is downloaded again at its next request, a
// restart included.
func TestTheImageCacheCanBeCleared(t *testing.T) {
	t.Parallel()
	art := newArtServer(t, jpegOf(t, 400, 600))
	cacheDir := t.TempDir()
	dir := filepath.Join(cacheDir, "images")
	h := imageHandler(dir)
	h.CacheDir = cacheDir
	original, err := h.fetchArtwork(t.Context(), art.URL+"/one.jpg", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if small := h.resized(t.Context(), art.URL+"/one.jpg", original, 100); len(small.body) >= len(original.body) {
		t.Fatal("the image was not resized")
	}
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/missing.jpg", false, false); err == nil {
		t.Fatal("a missing image was found")
	}
	kept := h.ImageCache()
	if kept.Files != 2 || kept.Bytes <= int64(len(original.body)) || kept.MemoryImages != 2 || kept.Limit != imageDiskBytes {
		t.Fatalf("kept before clearing: %+v", kept)
	}

	if freed := h.ClearImageCache(); freed.Files != kept.Files || freed.Bytes != kept.Bytes || freed.MemoryImages != 2 {
		t.Errorf("freed %+v, kept %+v", freed, kept)
	}
	if left := h.ImageCache(); left.Files != 0 || left.Bytes != 0 || left.MemoryImages != 0 || left.MemoryBytes != 0 {
		t.Errorf("left after clearing: %+v", left)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("%d files left on disk", len(files))
	}

	before := art.requests.Load()
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/one.jpg", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/missing.jpg", false, false); err == nil {
		t.Fatal("a missing image was found")
	}
	if got := art.requests.Load() - before; got != 2 {
		t.Errorf("after clearing, %d downloads for an image kept and one that failed", got)
	}
}
