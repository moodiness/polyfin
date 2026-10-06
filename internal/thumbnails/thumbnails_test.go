package thumbnails

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/testdb"
)

// countingSource serves a file from memory, as a host would, recording
// each request: when it came and what it asked.
type countingSource struct {
	data []byte
	// slowDownAt is the request, counted from 1, that the host answers
	// with 429, and every one after it; 0 for none.
	slowDownAt int
	// odd answers request n, counted from 1, with its error, when not nil;
	// expired answers every request with an expired link until Renew.
	odd     func(n int) error
	expired bool

	mu       sync.Mutex
	requests []request
	released bool
	renewals int
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
	count := len(c.count())
	if c.slowDownAt > 0 && count >= c.slowDownAt {
		return nil, &source.StatusError{Status: 429, Kind: source.ErrSlowDown}
	}
	c.mu.Lock()
	expired := c.expired
	c.mu.Unlock()
	if expired {
		return nil, &source.StatusError{Status: 403, Kind: source.ErrExpired}
	}
	if c.odd != nil {
		if err := c.odd(count); err != nil {
			return nil, err
		}
	}
	if off < 0 || off > int64(len(c.data)) {
		return nil, errors.New("out of the file")
	}
	return slices.Clone(c.data[off:min(off+int64(n), int64(len(c.data)))]), nil
}

func (c *countingSource) Renew(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.renewals++
	c.expired = false
	return nil
}

func (c *countingSource) KnownSize() (int64, bool) {
	return int64(len(c.data)), true
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
		Pace:   20 * time.Millisecond,
		Poll:   5 * time.Millisecond,
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
	testJob     = job{version: testVersion, host: "host.example"}
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
	data := fixture(t, "forced.mkv")
	ts.source = &countingSource{data: data}
	ts.analysis = forcedAnalysis
	ctx := t.Context()

	ts.generate(ctx, testJob)

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
		if closest := closestKeyframe(t, ffmpeg, "forced.mkv", img, cell, 240, keyframes); closest != at {
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
		if closest := closestKeyframe(t, ffmpeg, "forced.mkv", img, img.Bounds(), 64, keyframes); closest != at {
			t.Errorf("chapter %d shows the keyframe at %vs, not %vs", chapter, closest, at)
		}
	}

	// One request for the size, one for the head and the Cues of so small
	// a file, then one per keyframe shown: five, for four thumbnails and
	// three chapters, two of which share theirs, as the budget allows a
	// keyframe for each, and perhaps one more to learn how long Cluster
	// headers are: each exactly what it needs.
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
	ts.generate(ctx, testJob)
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

// closestKeyframe returns which of the keyframes of a fixture, by time,
// the part cell of img looks most like, FFmpeg decoding each at width.
func closestKeyframe(t *testing.T, ffmpeg, file string, img image.Image, cell image.Rectangle, width int, keyframes []float64) float64 {
	t.Helper()
	best, closest := math.MaxFloat64, -1.0
	for _, at := range keyframes {
		reference := referenceFrame(t, ffmpeg, file, at, width)
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

// referenceFrame is the frame of a fixture at a keyframe, by time, as
// FFmpeg decodes it, width pixels square.
func referenceFrame(t *testing.T, ffmpeg, file string, at float64, width int) image.Image {
	t.Helper()
	key := fmt.Sprint(file, at, width)
	if img, ok := references.Load(key); ok {
		return img.(image.Image)
	}
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-ss", strconv.FormatFloat(at, 'f', -1, 64),
		"-i", filepath.Join("..", "container", "testdata", file), "-frames:v", "1",
		"-vf", "scale=w="+strconv.Itoa(width)+":h=trunc(ow/dar/2)*2", "-f", "image2pipe", "-c:v", "png", "-").Output()
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

// The budget bounds the requests of a version, its index included: the
// thumbnails are stretched over its runtime, one per keyframe the budget
// lets read, each showing the one read nearest its time.
func TestGenerateWithinTheBudget(t *testing.T) {
	ts := newTestService(t, testFFmpeg(t))
	ts.budget = 5
	ts.source = &countingSource{data: fixture(t, "forced.mkv")}
	ts.analysis = forcedAnalysis
	ctx := t.Context()
	ts.generate(ctx, testJob)
	if n := len(ts.source.count()); n != 5 {
		t.Errorf("%d requests, for a budget of 5", n)
	}
	// The size and the index take two: the three left give three
	// thumbnails rather than four every 5 s, one every 6 s, at 0, 6 and
	// 12 s, which show the keyframes at 0, 6 and 12.5 s.
	manifest, err := ts.Manifest(ctx, testVersion.Item)
	if info := manifest[testVersion.ID][240]; err != nil || info.ThumbnailCount != 3 || info.Interval != 6000 {
		t.Fatalf("manifest %+v, %v", manifest, err)
	}
	tile, err := ts.Tile(ctx, testVersion.ID, 240, 0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(tile))
	if err != nil {
		t.Fatal(err)
	}
	all := []float64{0, 1.333, 2.916, 3.125, 5.125, 6, 7.708, 9.708, 11, 12.5, 14.5}
	ffmpeg := testFFmpeg(t)
	for i, at := range []float64{0, 6, 12.5} {
		if closest := closestKeyframe(t, ffmpeg, "forced.mkv", img, image.Rect(i*240, 0, (i+1)*240, 240), 240, all); closest != at {
			t.Errorf("thumbnail %d shows the keyframe at %vs, not %vs", i, closest, at)
		}
	}
	// The chapters at 0, 5 and 12 s share the same reads.
	for chapter, at := range []float64{0, 6, 12.5} {
		got, err := ts.ChapterImageOf(ctx, testVersion.ID, chapter, "")
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(got.Data))
		if err != nil {
			t.Fatal(err)
		}
		if closest := closestKeyframe(t, ffmpeg, "forced.mkv", img, img.Bounds(), 64, all); closest != at {
			t.Errorf("chapter %d shows the keyframe at %vs, not %vs", chapter, closest, at)
		}
	}
}

// A host asking to slow down stops the version at once, with nothing
// kept, and pauses every image of that host; other hosts go on.
func TestSlowDownPausesTheHost(t *testing.T) {
	ts := newTestService(t, testFFmpeg(t))
	// The size and the index are read; the first keyframe is refused.
	ts.source = &countingSource{data: fixture(t, "forced.mkv"), slowDownAt: 3}
	ts.analysis = forcedAnalysis
	ctx := t.Context()
	other := testVersion
	other.ID = accounts.ID{3}
	// Another version of the host waits for a playback.
	ts.WatchPlaybacks(func() []Playing { return []Playing{{Device: accounts.ID{9}, URL: "https://third.example/x.mkv"}} })
	ts.Service.mu.Lock()
	ts.jobs = append(ts.jobs, job{version: other, host: "host.example", after: accounts.ID{9}})
	ts.Service.mu.Unlock()
	ts.generate(ctx, testJob)
	if n := len(ts.source.count()); n != 3 {
		t.Errorf("%d requests: the one refused should be the last", n)
	}
	if manifest, err := ts.Manifest(ctx, testVersion.Item); err != nil || len(manifest) != 0 {
		t.Errorf("thumbnails kept: %v %v", manifest, err)
	}
	if images, err := ts.ChapterImages(ctx, testVersion.ID); err != nil || len(images) != 0 {
		t.Errorf("chapter images kept: %v %v", images, err)
	}
	if !ts.gate.paused("host.example") || ts.gate.ready("host.example") {
		t.Error("the host is not paused")
	}
	if _, refused := ts.refused.Get(testVersion.ID); !refused {
		t.Error("the version is not left alone for a day")
	}
	ts.Service.mu.Lock()
	waiting := len(ts.jobs)
	ts.Service.mu.Unlock()
	if waiting != 0 {
		t.Errorf("%d versions of the paused host still wait", waiting)
	}
	// Nothing more for that host, even another version.
	before := len(ts.source.count())
	ts.Queue(other, accounts.ID{})
	ts.generate(ctx, job{version: other, host: "host.example"})
	if n := len(ts.source.count()); n != before {
		t.Errorf("%d requests to a paused host", n-before)
	}
	// Another host is not paused.
	elsewhere := library.Version{ID: accounts.ID{4}, Item: accounts.ID{2}, URL: "https://other.example/file.mkv"}
	ts.source = &countingSource{data: fixture(t, "forced.mkv")}
	if ts.gate.paused("other.example") {
		t.Fatal("another host is paused")
	}
	ts.generate(ctx, job{version: elsewhere, host: "other.example"})
	if manifest, _ := ts.Manifest(ctx, testVersion.Item); manifest[elsewhere.ID][240].ThumbnailCount != 4 {
		t.Errorf("another host's thumbnails: %+v", manifest)
	}
}

// Images wait for the playback that asked for them to stop, and for
// every playback from the same host to end.
func TestGenerateWaitsForPlaybacks(t *testing.T) {
	ts := newTestService(t, testFFmpeg(t))
	ts.mu.Lock()
	ts.source = &countingSource{data: fixture(t, "forced.mkv")}
	ts.analysis = forcedAnalysis
	ts.mu.Unlock()
	var mu sync.Mutex
	var playing []Playing
	set := func(p ...Playing) {
		mu.Lock()
		playing = p
		mu.Unlock()
		ts.Wake()
	}
	ts.WatchPlaybacks(func() []Playing {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(playing)
	})
	viewer, other := accounts.ID{7}, accounts.ID{8}
	idle := func(what string) {
		t.Helper()
		time.Sleep(100 * time.Millisecond)
		if n := len(ts.source.count()); n != 0 {
			t.Fatalf("%s: %d requests", what, n)
		}
	}
	// The viewer still plays, from another host.
	set(Playing{Device: viewer, URL: "https://other.example/a.mkv"})
	ts.Queue(testVersion, viewer)
	idle("while its playback goes on")
	// It stopped, but someone else plays from the same host, or a version
	// whose host is unknown.
	set(Playing{Device: other, URL: "https://HOST.example/b.mkv"})
	idle("while the host serves a playback")
	set(Playing{Device: other})
	idle("while a playback of an unknown host goes on")
	// Only another host's playback is left.
	set(Playing{Device: other, URL: "https://other.example/b.mkv"})
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if manifest, _ := ts.Manifest(t.Context(), testVersion.Item); len(manifest) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("no thumbnails were made once the host was free")
}

// rangeSource is a countingSource serving several ranges with one
// request, as some hosts do.
type rangeSource struct{ *countingSource }

func (r rangeSource) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	r.record(request{kind: "ranges", ranges: len(ranges)})
	result := make([][]byte, len(ranges))
	for i, part := range ranges {
		result[i] = slices.Clone(r.data[part.Off:min(part.Off+int64(part.N), int64(len(r.data)))])
	}
	return result, nil
}

// hevcKeyframes are the keyframes of hevc.mkv, by time: every 4 s, CRA
// pictures of an open GOP, after which a decoder reorders what it outputs.
var hevcKeyframes = []float64{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56}

// Every thumbnail and chapter image shows the keyframe read nearest its
// time, whatever order the keyframes were read in (coarse to fine), one
// range a request or several, and with a budget too small for a thumbnail
// every 5 s, which stretches their interval: a decoded frame is tied to
// its keyframe's time, not to its place in the reads, nor in what the
// decoder outputs.
func TestImagesFollowTheKeyframesTimes(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	data := fixture(t, "hevc.mkv")
	analysis := media.Analysis{Format: "matroska,webm", Duration: 60 * time.Second,
		Streams:  []media.Stream{{Index: 0, Type: "video", Codec: "hevc", Width: 128, Height: 72}},
		Chapters: []media.Chapter{{Start: 0}, {Start: 21 * time.Second}, {Start: 50 * time.Second}}}
	for _, tc := range []struct {
		name   string
		ranges bool
		budget int
	}{
		{"one range a request", false, budget},
		{"several ranges a request", true, budget},
		{"one range a request, a small budget", false, 8},
		{"several ranges a request, a small budget", true, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestService(t, ffmpeg)
			ts.budget = tc.budget
			counting := &countingSource{data: data}
			ts.Open = func(library.Version) Source {
				if tc.ranges {
					return rangeSource{counting}
				}
				return counting
			}
			ts.analysis = analysis
			ctx := t.Context()
			ts.generate(ctx, testJob)
			manifest, err := ts.Manifest(ctx, testVersion.Item)
			info := manifest[testVersion.ID][240]
			step := time.Duration(info.Interval) * time.Millisecond
			wantStep := 5 * time.Second
			if tc.budget < budget {
				wantStep = time.Duration(info.Interval/1000) * time.Second
				if wantStep <= 5*time.Second || info.ThumbnailCount > tc.budget {
					t.Errorf("a budget of %d: %d thumbnails every %v", tc.budget, info.ThumbnailCount, step)
				}
			}
			if err != nil || step != wantStep || info.ThumbnailCount != thumbnailCount(analysis.Duration, step) {
				t.Fatalf("manifest %+v, %v", manifest, err)
			}
			tile, err := ts.Tile(ctx, testVersion.ID, 240, 0)
			if err != nil {
				t.Fatal(err)
			}
			img, err := jpeg.Decode(bytes.NewReader(tile))
			if err != nil {
				t.Fatal(err)
			}
			shownAt := make([]float64, info.ThumbnailCount)
			for i := range shownAt {
				cell := image.Rect(i%10*info.Width, i/10*info.Height, (i%10+1)*info.Width, (i/10+1)*info.Height)
				shownAt[i] = closestKeyframe(t, ffmpeg, "hevc.mkv", img, cell, info.Width, hevcKeyframes)
			}
			var chapters []float64
			for chapter := range analysis.Chapters {
				got, err := ts.ChapterImageOf(ctx, testVersion.ID, chapter, "")
				if err != nil {
					t.Fatal(err)
				}
				img, err := jpeg.Decode(bytes.NewReader(got.Data))
				if err != nil {
					t.Fatal(err)
				}
				chapters = append(chapters, closestKeyframe(t, ffmpeg, "hevc.mkv", img, img.Bounds(), img.Bounds().Dx(), hevcKeyframes))
			}
			// With the whole budget, every keyframe is read; when it runs
			// out, those read are at least those shown, spread over the
			// runtime.
			read := hevcKeyframes
			if tc.budget < budget {
				read = slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(shownAt), chapters...))))
				if len(read) < 2 || read[0] > 20 || read[len(read)-1] < 36 {
					t.Errorf("the keyframes read do not cover the runtime: %v", read)
				}
			}
			nearestRead := func(at float64) float64 {
				best := read[0]
				for _, k := range read {
					if math.Abs(k-at) < math.Abs(best-at) {
						best = k
					}
				}
				return best
			}
			for i, at := range shownAt {
				if want := nearestRead((time.Duration(i) * step).Seconds()); at != want {
					t.Errorf("thumbnail %d, at %v, shows the keyframe at %vs, not %vs: %v", i, time.Duration(i)*step, at, want, shownAt)
				}
			}
			for c, at := range chapters {
				if want := nearestRead(analysis.Chapters[c].Start.Seconds()); at != want {
					t.Errorf("chapter %d shows the keyframe at %vs, not %vs", c, at, want)
				}
			}
		})
	}
}

// Answers some hosts give now and then, which a second request does not
// get: a whole file for a range, another range, an answer cut short.
var oddAnswers = map[string]error{
	"ranges ignored": fmt.Errorf("fetching 262144 bytes at 0: %w", source.ErrRangesIgnored),
	"HTTP 500":       &source.StatusError{Status: 500, Kind: source.ErrUnavailable},
	"no answer":      fmt.Errorf("%w: connection reset", source.ErrUnavailable),
}

// An odd answer is asked once more, after a pause, within the budget: the
// images are made. Two in a row fail the version, which is left alone for a
// while, its host not paused.
func TestOddAnswersAreAskedOnceMore(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	for name, answer := range oddAnswers {
		t.Run(name, func(t *testing.T) {
			ts := newTestService(t, ffmpeg)
			ts.analysis = forcedAnalysis
			ctx := t.Context()
			ts.source = &countingSource{data: fixture(t, "forced.mkv"), odd: func(n int) error {
				if n == 1 {
					return answer
				}
				return nil
			}}
			ts.generate(ctx, testJob)
			requests := ts.source.count()
			if manifest, err := ts.Manifest(ctx, testVersion.Item); err != nil || manifest[testVersion.ID][240].ThumbnailCount != 4 {
				t.Fatalf("after one odd answer: %v %v", manifest, err)
			}
			// One more request than without the odd answer, after a pause of the
			// host's pace (20 ms in tests, 3 s otherwise).
			if len(requests) < 8 || len(requests) > 9 || requests[1].at.Sub(requests[0].at) < 19*time.Millisecond {
				t.Errorf("%d requests, the second %v after the first", len(requests), requests[1].at.Sub(requests[0].at))
			}

			ts = newTestService(t, ffmpeg)
			ts.analysis = forcedAnalysis
			ts.source = &countingSource{data: fixture(t, "forced.mkv"), odd: func(n int) error {
				if n <= 2 {
					return answer
				}
				return nil
			}}
			ts.generate(ctx, testJob)
			if n := len(ts.source.count()); n != 2 {
				t.Errorf("two odd answers in a row: %d requests", n)
			}
			if manifest, _ := ts.Manifest(ctx, testVersion.Item); len(manifest) != 0 {
				t.Errorf("images kept: %v", manifest)
			}
			if _, failed := ts.failed.Get(testVersion.ID); !failed {
				t.Error("the version is not left alone")
			}
			if ts.gate.paused("host.example") {
				t.Error("the host is paused")
			}
		})
	}
}

// A link the source says expired is renewed, once a generation, and asked
// again.
func TestExpiredLinksAreRenewedOnce(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	ts := newTestService(t, ffmpeg)
	ts.analysis = forcedAnalysis
	ctx := t.Context()
	ts.source = &countingSource{data: fixture(t, "forced.mkv"), expired: true}
	ts.generate(ctx, testJob)
	if manifest, err := ts.Manifest(ctx, testVersion.Item); err != nil || manifest[testVersion.ID][240].ThumbnailCount != 4 {
		t.Fatalf("after renewing the link: %v %v", manifest, err)
	}
	if ts.source.renewals != 1 {
		t.Errorf("%d renewals", ts.source.renewals)
	}
	// A link expiring again in the same generation is not renewed again:
	// the request is asked once more, and the version fails.
	ts = newTestService(t, ffmpeg)
	ts.analysis = forcedAnalysis
	counting := &countingSource{data: fixture(t, "forced.mkv"), expired: true}
	counting.odd = func(n int) error {
		if n >= 3 {
			counting.mu.Lock()
			counting.expired = true
			counting.mu.Unlock()
			return &source.StatusError{Status: 403, Kind: source.ErrExpired}
		}
		return nil
	}
	ts.source = counting
	ts.generate(ctx, testJob)
	if counting.renewals != 1 || len(counting.count()) != 4 {
		t.Errorf("%d renewals, %d requests", counting.renewals, len(counting.count()))
	}
	if manifest, _ := ts.Manifest(ctx, testVersion.Item); len(manifest) != 0 {
		t.Errorf("images kept: %v", manifest)
	}
}
