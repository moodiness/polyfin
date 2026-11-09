package localfiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// sparse writes a file of size bytes that takes no room on disk.
func sparse(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// The streams of a folder's movies and extras tell their sizes past 4 GiB
// whole on every target: on 32-bit targets they were cut modulo 2^32, so
// that a file of 5 GB told 705,032,704 bytes, which the source cache then
// held against the file.
func TestLocalSizesPast4GiBAreWhole(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, nil)
	const movie, extra = int64(5_000_000_000), int64(4_294_967_297)
	sparse(t, filepath.Join(dir, "Night of the Living Dead (1968).mkv"), movie)
	sparse(t, filepath.Join(dir, "The General (1926)", "The.General.1080p.mkv"), 10)
	sparse(t, filepath.Join(dir, "The General (1926)", "Extras", "The General - Interview.mkv"), extra)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0063350")
	if err != nil || len(streams) != 1 || int64(streams[0].BehaviorHints.VideoSize) != movie {
		t.Errorf("the movie's streams: %+v %v", streams, err)
	}
	var id accounts.ID
	if err := e.pool.QueryRow(t.Context(), "SELECT id FROM local_extras WHERE addon_id = $1", addon.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	streams, err = e.service.Streams(t.Context(), addon.ID, library.ExtraStremioID(id))
	if err != nil || len(streams) != 1 || int64(streams[0].BehaviorHints.VideoSize) != extra {
		t.Errorf("the extra's streams: %+v %v", streams, err)
	}
}
