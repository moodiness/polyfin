package thumbnails

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
)

func saveThumbnails(t *testing.T, ts *testService, version, item accounts.ID, width int) Info {
	t.Helper()
	info := Info{Width: width, Height: width / 2, TileWidth: 10, TileHeight: 10, ThumbnailCount: 120, Interval: 10000, Bandwidth: 900}
	if err := ts.SaveTrickplay(t.Context(), version, item, info, [][]byte{[]byte("tile 0"), []byte("tile 1")}); err != nil {
		t.Fatal(err)
	}
	return info
}

// A title lists its versions' thumbnails by version and width; the
// settings turning them off hide them.
func TestTrickplayStored(t *testing.T) {
	ts := newTestService(t, "")
	ctx := t.Context()
	title, other := accounts.ID{10}, accounts.ID{11}
	first, second := accounts.ID{20}, accounts.ID{21}
	small := saveThumbnails(t, ts, first, title, 240)
	large := saveThumbnails(t, ts, first, title, 480)
	alone := saveThumbnails(t, ts, second, title, 320)
	saveThumbnails(t, ts, accounts.ID{22}, other, 320)

	manifest, err := ts.Manifest(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 || manifest[first][240] != small || manifest[first][480] != large || manifest[second][320] != alone || len(manifest[first]) != 2 {
		t.Errorf("manifest %+v", manifest)
	}
	if version, info, err := ts.Trickplay(ctx, title, first, 480); err != nil || version != first || info != large {
		t.Errorf("by version: %v %+v %v", version, info, err)
	}
	// Without a version, the title's used last.
	if version, _, err := ts.Trickplay(ctx, title, accounts.ID{}, 320); err != nil || version != second {
		t.Errorf("without a version: %v %v", version, err)
	}
	if _, _, err := ts.Trickplay(ctx, title, second, 480); !errors.Is(err, ErrNotFound) {
		t.Errorf("a width the version lacks: %v", err)
	}
	if _, _, err := ts.Trickplay(ctx, other, first, 240); !errors.Is(err, ErrNotFound) {
		t.Errorf("another title's version: %v", err)
	}
	if tile, err := ts.Tile(ctx, first, 240, 1); err != nil || string(tile) != "tile 1" {
		t.Errorf("tile 1: %q %v", tile, err)
	}
	if _, err := ts.Tile(ctx, first, 240, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tile past the last: %v", err)
	}

	ts.change(func(s *accounts.Settings) { s.Trickplay = false })
	if manifest, err := ts.Manifest(ctx, title); err != nil || len(manifest) != 0 {
		t.Errorf("turned off: %v %v", manifest, err)
	}
	if _, _, err := ts.Trickplay(ctx, title, first, 240); !errors.Is(err, ErrNotFound) {
		t.Errorf("turned off: %v", err)
	}
	if _, err := ts.Tile(ctx, first, 240, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("turned off: %v", err)
	}
}

// A chapter image is found by its version, or by its title: the version
// of the tag asked, else the title's version used last, as reading its
// images uses it.
func TestChapterImagesStored(t *testing.T) {
	ts := newTestService(t, "")
	ctx := t.Context()
	title := accounts.ID{10}
	first, second := accounts.ID{20}, accounts.ID{21}
	if err := ts.SaveChapterImages(ctx, first, title, [][]byte{[]byte("first 0"), []byte("first 1")}); err != nil {
		t.Fatal(err)
	}
	if err := ts.SaveChapterImages(ctx, second, title, [][]byte{[]byte("second 0")}); err != nil {
		t.Fatal(err)
	}
	images, err := ts.ChapterImages(ctx, first)
	if err != nil || len(images) != 2 || images[0].Tag == images[1].Tag || images[0].Tag == "" {
		t.Fatalf("images %+v %v", images, err)
	}
	for _, tc := range []struct {
		id      accounts.ID
		chapter int
		tag     string
		want    string
	}{
		{title, 0, "", "second 0"},
		{title, 0, images[0].Tag, "first 0"},
		{title, 1, "", "first 1"},
		{first, 0, "", "first 0"},
		{first, 1, "", "first 1"},
	} {
		got, err := ts.ChapterImageOf(ctx, tc.id, tc.chapter, tc.tag)
		if err != nil || string(got.Data) != tc.want {
			t.Errorf("%v chapter %d tag %q: %q %v", tc.id, tc.chapter, tc.tag, got.Data, err)
		}
	}
	if _, err := ts.ChapterImageOf(ctx, second, 1, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("a chapter past the last: %v", err)
	}
	// Made again, an image changes its tag.
	if err := ts.SaveChapterImages(ctx, first, title, [][]byte{[]byte("again 0")}); err != nil {
		t.Fatal(err)
	}
	if again, _ := ts.ChapterImages(ctx, first); len(again) != 1 || again[0].Tag == images[0].Tag {
		t.Errorf("made again: %+v", again)
	}
	ts.change(func(s *accounts.Settings) { s.ChapterImages = false })
	if images, err := ts.ChapterImages(ctx, first); err != nil || len(images) != 0 {
		t.Errorf("turned off: %v %v", images, err)
	}
	if _, err := ts.ChapterImageOf(ctx, first, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("turned off: %v", err)
	}
}

// Past the storage the settings give them, the versions used longest ago
// lose their images, the others keep theirs.
func TestStorageCapEvictsTheOldest(t *testing.T) {
	ts := newTestService(t, "")
	ctx := t.Context()
	versions := []accounts.ID{{1}, {2}, {3}}
	for i, version := range versions {
		saveThumbnails(t, ts, version, accounts.ID{100}, 320)
		// Each is said to take 400 MB, the first used longest ago.
		if _, err := ts.DB.Exec(ctx, "UPDATE thumbnail_versions SET bytes = 400 << 20, used_at = now() - make_interval(hours => $2) WHERE version_id = $1",
			version, 10-i); err != nil {
			t.Fatal(err)
		}
	}
	// The first is used again: the second is now the one used longest ago.
	ts.touched.Delete(versions[0])
	if _, _, err := ts.Trickplay(ctx, accounts.ID{100}, versions[0], 320); err != nil {
		t.Fatal(err)
	}
	newest := accounts.ID{4}
	if err := ts.SaveChapterImages(ctx, newest, accounts.ID{100}, [][]byte{[]byte("image")}); err != nil {
		t.Fatal(err)
	}
	var kept []accounts.ID
	rows, err := ts.DB.Query(ctx, "SELECT version_id FROM thumbnail_versions ORDER BY version_id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id accounts.ID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, id)
	}
	if want := []accounts.ID{versions[0], versions[2], newest}; !slices.Equal(kept, want) {
		t.Errorf("kept %v, want %v", kept, want)
	}
	if _, err := ts.Tile(ctx, versions[1], 320, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("the tiles of the evicted version: %v", err)
	}
	// More room keeps everything.
	ts.change(func(s *accounts.Settings) { s.ThumbnailStorageGB = 50 })
	saveThumbnails(t, ts, versions[1], accounts.ID{100}, 320)
	var count int
	if err := ts.DB.QueryRow(ctx, "SELECT count(*) FROM thumbnail_versions").Scan(&count); err != nil || count != 4 {
		t.Errorf("%d versions kept: %v", count, err)
	}
}

// Keyframes are read through the index, a request each, paced per host,
// and a generation's requests are capped.
func TestPacedReads(t *testing.T) {
	data := fixture(t, "forced.mkv")
	src := &countingSource{data: data}
	ctx := context.Background()
	p := &paced{src: src, host: "host.example", pacer: newPacer(15 * time.Millisecond), left: 100}
	video, err := container.OpenVideo(ctx, p, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	head := len(src.count())
	read := 0
	if err := video.ReadFrames(ctx, p, []int{0, 4, 8, 10}, func(int, []byte) error {
		read++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requests := src.count()
	if head > 2 || read != 4 || len(requests)-head < 4 || len(requests)-head > 5 || p.requests != len(requests) {
		t.Errorf("%d requests for the index, %d for 4 keyframes, %d counted", head, len(requests)-head, p.requests)
	}
	for i, r := range requests[head:] {
		if r.kind != "fetch" {
			t.Errorf("request %d after the index: %s", i, r.kind)
		}
	}
	for i := 1; i < len(requests); i++ {
		if gap := requests[i].at.Sub(requests[i-1].at); gap < 14*time.Millisecond {
			t.Errorf("requests %d and %d %v apart", i-1, i, gap)
		}
	}
	// Another host is not held up by this one.
	start := time.Now()
	other := &paced{src: src, host: "other.example", pacer: p.pacer, left: 1}
	if _, err := other.Fetch(ctx, 0, 10); err != nil || time.Since(start) > 10*time.Millisecond {
		t.Errorf("another host waited %v: %v", time.Since(start), err)
	}
	// Past its share, a generation's requests fail.
	if _, err := other.Fetch(ctx, 0, 10); !errors.Is(err, errTooManyRequests) {
		t.Errorf("past the cap: %v", err)
	}
	if !lasting(errTooManyRequests) {
		t.Error("a version taking too many requests would be tried again soon")
	}
}
