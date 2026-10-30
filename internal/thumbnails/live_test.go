package thumbnails

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// longVideo makes a Matroska file of 15 minutes, a keyframe every 2
// seconds, of about 40 MiB: many of the source cache's blocks.
func longVideo(t *testing.T, ffmpeg string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "long.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=4",
		"-t", "900", "-c:v", "libx264", "-preset", "ultrafast", "-g", "8", "-b:v", "400k", "-pix_fmt", "yuv420p", "-f", "matroska", path).CombinedOutput()
	if err != nil {
		t.Skipf("FFmpeg cannot make the test file: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// host serves a file with ranges, counting the requests it receives.
type host struct {
	requests atomic.Int32
	url      string
}

func newHost(t *testing.T, data []byte) *host {
	t.Helper()
	h := &host{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.requests.Add(1)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	h.url = server.URL + "/long.mkv"
	return h
}

type httpOpener struct{}

func (httpOpener) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header = header
	return http.DefaultClient.Do(request)
}

// play reads a version's file through the cache as a playback from its
// start reaches at, its keyframe index read first, as its analysis does:
// up to the Cluster after the keyframe following at.
func play(t *testing.T, src *source.Source, keyframes []container.Keyframe, at time.Duration) {
	t.Helper()
	end := int64(0)
	for i, k := range keyframes {
		if k.Time > at && i+1 < len(keyframes) {
			end = keyframes[i+1].Offset
			break
		}
	}
	buf := make([]byte, 1<<20)
	for off := int64(0); off < end; off += int64(len(buf)) {
		n := min(int64(len(buf)), end-off)
		if _, err := src.ReadAt(t.Context(), buf[:n], off); err != nil {
			t.Fatal(err)
		}
	}
}

// The first minutes of a version played get thumbnails during the play,
// made of what the playback read alone: its host gets no request more,
// and a conversion on the processor holds them back. After the play, the
// thumbnails left are read, and the kept ones are not read again.
func TestThumbnailsDuringThePlay(t *testing.T) {
	ffmpeg := testFFmpeg(t)
	data := longVideo(t, ffmpeg)
	origin := newHost(t, data)
	ts := newTestService(t, ffmpeg)
	cache, err := source.New(t.TempDir(), func() int64 { return 1 << 30 }, httpOpener{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	ts.cache = cache
	ts.analysis = media.Analysis{Format: "matroska,webm", Duration: 900 * time.Second, Size: int64(len(data)),
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 320, Height: 180}}}
	version := library.Version{ID: accounts.ID{1}, Item: accounts.ID{2}, URL: origin.url, Addon: "Test"}
	viewer := accounts.ID{3}
	ctx := t.Context()

	// The play starts: its version waits for its images, and is played.
	ts.WatchPlaybacks(func() []Playing { return []Playing{{Device: viewer, URL: version.URL}} })
	ts.Queue(version, viewer)
	if playing := ts.playingVersions(); len(playing) != 1 || playing[0].ID != version.ID {
		t.Fatalf("versions playing: %+v", playing)
	}
	src := cache.Open(version.ID, source.Location{URL: version.URL}, nil)
	defer src.Release()
	keyframes, err := container.KeyframeOffsets(ctx, src, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	play(t, src, keyframes, 5*time.Minute)

	// No thumbnail while a conversion runs on the processor.
	ts.converting.Store(true)
	ts.extendPlaying(ctx, version)
	if _, found, err := ts.trickplaySet(ctx, version.ID, 240); found || err != nil {
		t.Fatalf("thumbnails made during a conversion: %v", err)
	}
	ts.converting.Store(false)

	requests := origin.requests.Load()
	ts.extendPlaying(ctx, version)
	info := covered(t, ts, version, 5*time.Minute)
	if after := origin.requests.Load(); after != requests {
		t.Errorf("the thumbnails made %d requests to the host", after-requests)
	}
	if info.ThumbnailCount >= 100 {
		t.Fatalf("five minutes read gave %d thumbnails", info.ThumbnailCount)
	}

	// Five minutes more: the first tile is completed, the second started.
	play(t, src, keyframes, 10*time.Minute)
	requests = origin.requests.Load()
	ts.extendPlaying(ctx, version)
	info = covered(t, ts, version, 10*time.Minute)
	if after := origin.requests.Load(); after != requests {
		t.Errorf("the thumbnails made %d requests to the host", after-requests)
	}
	madeDuringPlay := info.ThumbnailCount
	first := tileImage(t, ts, version, 0)
	for place := range 100 {
		if lit(first, place%10, place/10, 240) == 0 {
			t.Errorf("thumbnail %d of the first tile is black", place)
		}
	}
	second := tileImage(t, ts, version, 1)
	if last := madeDuringPlay - 1 - 100; lit(second, last%10, last/10, 240) == 0 || lit(second, 9, 9, 240) != 0 {
		t.Error("the second tile does not hold the thumbnails made, and them alone")
	}

	// After the play, the rest is read: none of the keyframes kept. The
	// version is taken from the queue here rather than by the service.
	// With fewer requests left than thumbnails, the interval is stretched,
	// and each thumbnail kept nearest a time shows there.
	ts.dropHost(hostOf(version.URL))
	ts.WatchPlaybacks(func() []Playing { return nil })
	partial, _, err := ts.trickplaySet(ctx, version.ID, 240)
	if err != nil {
		t.Fatal(err)
	}
	var partialTiles [][]byte
	for index := range 2 {
		tile, err := ts.keptTile(ctx, version.ID, 240, index)
		if err != nil {
			t.Fatal(err)
		}
		partialTiles = append(partialTiles, tile)
	}
	keptEnd := keyframes[0].Offset
	for _, k := range keyframes {
		if k.Time < time.Duration(madeDuringPlay-1)*5*time.Second {
			keptEnd = k.Offset
		}
	}
	for _, budget := range []int{60, 30} {
		if err := ts.SaveTrickplay(ctx, version.ID, version.Item, partial, partialTiles); err != nil {
			t.Fatal(err)
		}
		ts.budget = budget
		ts.source = &countingSource{data: data}
		ts.generate(ctx, job{version: version, host: hostOf(version.URL), after: viewer})
		info = covered(t, ts, version, 900*time.Second)
		t.Logf("%d thumbnails made during the play, then %d every %v with %d requests", madeDuringPlay,
			info.ThumbnailCount, info.interval(), len(ts.source.count()))
		if !complete(info, ts.analysis.Duration) || budget < 60 && info.Interval <= 5000 {
			t.Errorf("after the play, with %d requests: %+v", budget, info)
		}
		for _, r := range ts.source.count() {
			// The head, read for the index, apart.
			if r.off > 1<<20 && r.off < keptEnd {
				t.Errorf("a keyframe kept was read again: %+v", r)
			}
		}
		final := tileImage(t, ts, version, 0)
		for n := range min(100, info.ThumbnailCount) {
			at := time.Duration(n) * info.interval()
			if at >= time.Duration(madeDuringPlay)*5*time.Second {
				break
			}
			k := min(madeDuringPlay-1, int((at+2500*time.Millisecond)/(5*time.Second)))
			kept := []image.Image{first, second}[k/100]
			if got, want := lit(final, n%10, n/10, 240), lit(kept, k%100%10, k%100/10, 240); got < want-3 || got > want+3 {
				t.Errorf("thumbnail %d (%v) does not show the thumbnail kept at %v", n, at, time.Duration(k)*5*time.Second)
			}
		}
	}
}

// covered returns the thumbnails of version, checking that they cover
// the first d of it.
func covered(t *testing.T, ts *testService, version library.Version, d time.Duration) Info {
	t.Helper()
	manifest, err := ts.Manifest(t.Context(), version.Item)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := manifest[version.ID][240]
	if !ok {
		t.Fatalf("no thumbnails: %+v", manifest)
	}
	if reach := time.Duration(info.ThumbnailCount) * info.interval(); reach < d {
		t.Fatalf("the thumbnails reach %v, not %v: %+v", reach, d, info)
	}
	return info
}

func tileImage(t *testing.T, ts *testService, version library.Version, index int) image.Image {
	t.Helper()
	tile, err := ts.Tile(t.Context(), version.ID, 240, index)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(tile))
	if err != nil {
		t.Fatal(err)
	}
	return img
}
