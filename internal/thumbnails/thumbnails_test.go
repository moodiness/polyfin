package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/testdb"
)

// countingSource serves a file from memory, as a host would, recording
// each request: when it came and what it asked.
type countingSource struct {
	data []byte

	mu       sync.Mutex
	requests []request
	released bool
}

type request struct {
	at        time.Time
	kind      string
	off, n    int64
	ranges    int
	fetchedAt int64
}

func (c *countingSource) record(r request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r.at = time.Now()
	c.requests = append(c.requests, r)
}

func (c *countingSource) Fetch(_ context.Context, off int64, n int) ([]byte, error) {
	c.record(request{kind: "fetch", off: off, n: int64(n)})
	if off < 0 || off > int64(len(c.data)) {
		return nil, errors.New("out of the file")
	}
	return slices.Clone(c.data[off:min(off+int64(n), int64(len(c.data)))]), nil
}

func (c *countingSource) Size(context.Context) (int64, error) {
	c.record(request{kind: "size"})
	return int64(len(c.data)), nil
}

func (c *countingSource) Release() {
	c.mu.Lock()
	c.released = true
	c.mu.Unlock()
}

func (c *countingSource) count() []request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.requests)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "container", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// testService is a service over a fresh database, its settings changed
// through settings.
type testService struct {
	*Service
	mu       sync.Mutex
	settings accounts.Settings
	analysis media.Analysis
	source   *countingSource
}

func newTestService(t *testing.T, ffmpeg string) *testService {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	ts := &testService{settings: accounts.Settings{Trickplay: true, TrickplayInterval: 5, TrickplayWidth: 240,
		ChapterImages: true, ThumbnailStorageGB: 1}}
	ts.Service = New(Options{
		DB:     pool,
		FFmpeg: ffmpeg,
		Settings: func() accounts.Settings {
			ts.mu.Lock()
			defer ts.mu.Unlock()
			return ts.settings
		},
		Open: func(library.Version) Source { return ts.source },
		Analyzed: func(context.Context, accounts.ID) (media.Analysis, bool) {
			ts.mu.Lock()
			defer ts.mu.Unlock()
			return ts.analysis, ts.analysis.Duration > 0
		},
		Logger: slog.New(slog.DiscardHandler),
	})
	t.Cleanup(ts.Close)
	return ts
}

func (ts *testService) change(fn func(*accounts.Settings)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	fn(&ts.settings)
}

func testFFmpeg(t *testing.T) string {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg
}

var (
	testVersion = library.Version{ID: accounts.ID{1}, Item: accounts.ID{2}, URL: "https://host.example/file.mkv", Addon: "Test"}
	// forcedAnalysis is what ffprobe finds in forced.mkv, whose keyframes
	// are at 0, 1.333, 2.917, 3.125, 5.125, 6, 7.708, 9.708, 11, 12.5 and
	// 14.5 s.
	forcedAnalysis = media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond,
		Streams:  []media.Stream{{Index: 0, Type: "audio", Codec: "opus"}, {Index: 1, Type: "video", Codec: "h264", Width: 64, Height: 64}},
		Chapters: []media.Chapter{{Start: 0, End: 5 * time.Second}, {Start: 5 * time.Second, End: 10 * time.Second}, {Start: 12 * time.Second, End: 15 * time.Second}}}
)

// A version played gets its thumbnails, packed in Jellyfin's tiles, and
// its chapter images, from its keyframes alone, read paced through its
// index and decoded by FFmpeg.
func TestGenerateFromMatroska(t *testing.T) {
	ts := newTestService(t, testFFmpeg(t))
	ts.pacer = newPacer(20 * time.Millisecond)
	data := fixture(t, "forced.mkv")
	ts.source = &countingSource{data: data}
	ts.analysis = forcedAnalysis
	ctx := t.Context()

	ts.generate(ctx, testVersion)

	manifest, err := ts.Manifest(ctx, testVersion.Item)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := manifest[testVersion.ID][240]
	// 15 s, one thumbnail every 5 s from the start: four, in one tile of
	// 10 by 10.
	want := Info{Width: 240, Height: 240, TileWidth: 10, TileHeight: 10, ThumbnailCount: 4, Interval: 5000}
	if !ok || info.Bandwidth <= 0 {
		t.Fatalf("manifest: %+v", manifest)
	}
	want.Bandwidth = info.Bandwidth
	if info != want {
		t.Errorf("thumbnails: %+v, want %+v", info, want)
	}
	tile, err := ts.Tile(ctx, testVersion.ID, 240, 0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(tile))
	if err != nil {
		t.Fatal(err)
	}
	if size := img.Bounds().Size(); size != (image.Point{2400, 2400}) {
		t.Errorf("tile of %v", size)
	}
	// Each thumbnail shows the keyframe nearest its time, 0, 5.125, 9.708
	// and 14.5 s, row by row; the rest of the grid is black.
	ffmpeg := testFFmpeg(t)
	keyframes := []float64{0, 5.125, 9.708, 14.5, 3.125, 6, 11, 12.5}
	for i, at := range keyframes[:4] {
		cell := image.Rect(i*240, 0, (i+1)*240, 240)
		if closest := closestKeyframe(t, ffmpeg, img, cell, 240, keyframes); closest != at {
			t.Errorf("thumbnail %d shows the keyframe at %vs, not %vs", i, closest, at)
		}
	}
	if lit(img, 4, 0, 240) != 0 || lit(img, 0, 1, 240) != 0 || lit(img, 9, 9, 240) != 0 {
		t.Error("the free places of the tile are not black")
	}
	if _, err := ts.Tile(ctx, testVersion.ID, 240, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("a second tile: %v", err)
	}
	if want := int(float64(len(tile)*8)/100/5 + 1); info.Bandwidth != want && info.Bandwidth != want-1 {
		t.Errorf("bandwidth %d for a tile of %d bytes", info.Bandwidth, len(tile))
	}

	images, err := ts.ChapterImages(ctx, testVersion.ID)
	if err != nil || len(images) != 3 {
		t.Fatalf("chapter images: %v %v", images, err)
	}
	// Each chapter's image is the keyframe nearest its start: 0, 5.125
	// and 12.5 s, at the size of the source, narrower than they are made.
	for chapter, at := range []float64{0, 5.125, 12.5} {
		got, err := ts.ChapterImageOf(ctx, testVersion.Item, chapter, "")
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(got.Data))
		if err != nil {
			t.Fatal(err)
		}
		if size := img.Bounds().Size(); size != (image.Point{64, 64}) || got.Tag != images[chapter].Tag {
			t.Errorf("chapter %d: %v, tag %q", chapter, size, got.Tag)
		}
		if closest := closestKeyframe(t, ffmpeg, img, img.Bounds(), 64, keyframes); closest != at {
			t.Errorf("chapter %d shows the keyframe at %vs, not %vs", chapter, closest, at)
		}
	}

	// The size, the head and the Cues (one request, for so small a file),
	// then one request per keyframe read, five, and perhaps one more to
	// learn how long Cluster headers are: each exactly what it needs.
	requests := ts.source.count()
	if len(requests) < 7 || len(requests) > 8 {
		t.Errorf("%d requests: %+v", len(requests), requests)
	}
	for _, r := range requests[2:] {
		if r.kind != "fetch" || r.n > 256<<10 {
			t.Errorf("a keyframe read %+v", r)
		}
	}
	// Paced: never two requests closer than the interval.
	for i := 1; i < len(requests); i++ {
		if gap := requests[i].at.Sub(requests[i-1].at); gap < 19*time.Millisecond {
			t.Errorf("requests %d and %d %v apart", i-1, i, gap)
		}
	}
	if !ts.source.released {
		t.Error("the source was not released")
	}

	// Made once: another play reads nothing.
	before := len(ts.source.count())
	ts.generate(ctx, testVersion)
	if after := len(ts.source.count()); after != before {
		t.Errorf("a second play made %d requests", after-before)
	}
}

// lit is how bright, on average, the thumbnail at column x and row y of a
// tile is.
func lit(img image.Image, x, y, width int) int {
	total, n := 0, 0
	b := img.Bounds()
	height := b.Dy() / 10
	for py := y * height; py < (y+1)*height; py += 4 {
		for px := x * width; px < (x+1)*width; px += 4 {
			r, g, bl, _ := img.At(px, py).RGBA()
			total += int(r>>8+g>>8+bl>>8) / 3
			n++
		}
	}
	if total/n < 8 {
		return 0
	}
	return total / n
}

// closestKeyframe returns which of the keyframes of forced.mkv, by time,
// the part cell of img looks most like, FFmpeg decoding each at width.
func closestKeyframe(t *testing.T, ffmpeg string, img image.Image, cell image.Rectangle, width int, keyframes []float64) float64 {
	t.Helper()
	best, closest := math.MaxFloat64, -1.0
	for _, at := range keyframes {
		reference := referenceFrame(t, ffmpeg, at, width)
		difference := 0.0
		for y := range cell.Dy() {
			for x := range cell.Dx() {
				r1, g1, b1, _ := img.At(cell.Min.X+x, cell.Min.Y+y).RGBA()
				r2, g2, b2, _ := reference.At(x, y).RGBA()
				difference += math.Abs(float64(r1)-float64(r2)) + math.Abs(float64(g1)-float64(g2)) + math.Abs(float64(b1)-float64(b2))
			}
		}
		if difference < best {
			best, closest = difference, at
		}
	}
	return closest
}

// references holds the keyframes FFmpeg decoded for referenceFrame.
var references sync.Map

// referenceFrame is the frame of forced.mkv at a keyframe, by time, as
// FFmpeg decodes it, width pixels square.
func referenceFrame(t *testing.T, ffmpeg string, at float64, width int) image.Image {
	t.Helper()
	key := [2]float64{at, float64(width)}
	if img, ok := references.Load(key); ok {
		return img.(image.Image)
	}
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-ss", strconv.FormatFloat(at, 'f', -1, 64),
		"-i", filepath.Join("..", "container", "testdata", "forced.mkv"), "-frames:v", "1",
		"-vf", "scale="+strconv.Itoa(width)+":"+strconv.Itoa(width), "-f", "image2pipe", "-c:v", "png", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	references.Store(key, img)
	return img
}

// A version whose images would take its host more requests than a
// generation may make is left alone, for long: its next play reads
// nothing.
func TestGenerateRequestCap(t *testing.T) {
	ts := newTestService(t, testFFmpeg(t))
	ts.pacer = newPacer(time.Millisecond)
	ts.maxRequests = 4
	ts.source = &countingSource{data: fixture(t, "forced.mkv")}
	ts.analysis = forcedAnalysis
	ctx := t.Context()
	ts.generate(ctx, testVersion)
	if n := len(ts.source.count()); n != 4 {
		t.Errorf("%d requests, capped at 4", n)
	}
	if _, err := ts.Tile(ctx, testVersion.ID, 240, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("thumbnails past the cap: %v", err)
	}
	if _, refused := ts.refused.Get(testVersion.ID); !refused {
		t.Error("the version is not left alone")
	}
	ts.Queue(testVersion)
	if len(ts.queue) != 0 {
		t.Error("a refused version was queued again")
	}
}
