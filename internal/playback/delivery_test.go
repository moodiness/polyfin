package playback

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// changingSource serves a file, then, after its first answer, another of
// another size, as a link that started answering another file.
type changingSource struct {
	file, other []byte
	requests    atomic.Int32
}

func (c *changingSource) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	data := c.file
	if c.requests.Add(1) > 1 {
		data = c.other
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	http.ServeContent(recorder, request, "", time.Time{}, bytes.NewReader(data))
	response := recorder.Result()
	response.Request = request
	return response, nil
}

// A source failing while ffprobe reads it stops the analysis at once,
// rather than at the settings' timeout: the version's failure, kept, is
// what its host answered.
func TestAnalysesStopWhenTheirSourceFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffprobe")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	host := &changingSource{file: make([]byte, 16<<20), other: make([]byte, 20<<20)}
	s := newService(t, host, path, nil)
	version := library.Version{ID: accounts.ID{21}, URL: "https://93.184.216.34/movie.mp4"}
	// Something reads the file while it is analyzed, as ffprobe does:
	// the head, then far in.
	src := s.OpenSource(version)
	defer src.Release()
	go func() {
		if _, err := src.ReadAt(context.Background(), make([]byte, 10), 0); err == nil {
			_, _ = src.ReadAt(context.Background(), make([]byte, 10), 12<<20)
		}
	}()
	started := time.Now()
	_, err := s.Analyze(t.Context(), version)
	if !errors.Is(err, source.ErrUnavailable) || source.Answer(err) != "an answer of another size than the file" {
		t.Fatalf("got %v (%q)", err, source.Answer(err))
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("the analysis stopped after %v", took)
	}
	if kept, failed := s.failures.Get(version.ID); !failed || source.Answer(kept) != source.Answer(err) {
		t.Errorf("kept %v", kept)
	}
}

// The keyframe index of a Matroska file is read while it is analyzed, not
// after: an HLS play finds it read.
func TestMatroskaIndexesAreReadDuringTheAnalysis(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codecs.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join("testdata", "codecs.ffprobe.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "output.json"), probe, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ffprobe")
	// ffprobe takes its time; it reads nothing here.
	script := "#!/bin/sh\nsleep 1\ncat " + filepath.Join(dir, "output.json") + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, version := range []library.Version{
		{ID: accounts.ID{22}, URL: "https://93.184.216.34/a", Filename: "Movie.mkv"},
		// Named otherwise, the file's first bytes tell.
		{ID: accounts.ID{23}, URL: "https://93.184.216.34/b", Filename: "movie.bin"},
	} {
		s := newService(t, &fileSource{data: data}, path, nil)
		if _, err := s.Analyze(t.Context(), version); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.indexes.Get(version.ID); !ok {
			t.Errorf("%s: the index was not read during the analysis", version.Filename)
		}
		if _, ok := s.offsets.Get(version.ID); !ok {
			t.Errorf("%s: where the keyframes are was not kept", version.Filename)
		}
	}
}

// An index that cannot be read because the host failed is read again at
// the next play; one the file does not have is not read again for a
// while.
func TestOnlyLastingIndexFailuresAreKept(t *testing.T) {
	s := newService(t, &fakeSource{status: http.StatusNotFound}, "ffprobe", nil)
	failing := library.Version{ID: accounts.ID{24}, URL: "https://93.184.216.34/gone.mkv"}
	if _, err := s.keyframes(t.Context(), failing, media.Analysis{}); !errors.Is(err, source.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if _, kept := s.unindexed.Get(failing.ID); kept {
		t.Error("a host's failure was kept as the index's")
	}
	page, err := os.ReadFile(filepath.Join("testdata", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	s = newService(t, &fileSource{data: page}, "ffprobe", nil)
	unindexed := library.Version{ID: accounts.ID{25}, URL: "https://93.184.216.34/page"}
	if _, err := s.keyframes(t.Context(), unindexed, media.Analysis{}); !errors.Is(err, ErrNotRemuxable) {
		t.Fatalf("got %v", err)
	}
	if _, kept := s.unindexed.Get(unindexed.ID); !kept {
		t.Error("a file without an index is read again")
	}
}

// A source that answered the server's own reads lately is not asked again
// before a player is sent to it; the player goes to the addon's link, never
// where it redirected the server.
func TestPlayersAreSentToSourcesThatAnsweredLately(t *testing.T) {
	host := &fileSource{data: make([]byte, 1000)}
	s := newService(t, host, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{26}, URL: "https://93.184.216.34/movie.mkv"}
	src := s.OpenSource(version)
	if _, err := src.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	src.Release()
	before := host.count.Load()
	response := httptest.NewRecorder()
	if err := s.Serve(response, httptest.NewRequest(http.MethodGet, "/", nil), version, Delivery{}); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusFound || response.Header().Get("Location") != version.URL || host.count.Load() != before {
		t.Errorf("%d to %q after %d requests", response.Code, response.Header().Get("Location"), host.count.Load()-before)
	}
	// Another link of the version is checked.
	fresh := version
	fresh.URL = "https://93.184.216.34/movie.mkv?fresh"
	if err := s.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), fresh, Delivery{}); err != nil {
		t.Fatal(err)
	}
	if host.count.Load() != before+1 {
		t.Errorf("another link: %d checks", host.count.Load()-before)
	}
}

// A link that could not be renewed is not kept as the file's failure:
// the version is still offered.
func TestFailedRenewalsAreNotTheFilesFailure(t *testing.T) {
	const link = "https://93.184.216.34/old.mkv"
	origin := &fakeSource{expired: map[string]bool{link: true}}
	s := newService(t, origin, "ffprobe", func(context.Context, library.Version) (library.Version, error) {
		return library.Version{}, errors.New("the addon did not answer")
	})
	version := library.Version{ID: accounts.ID{27}, URL: link}
	if err := s.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), version, Delivery{}); err == nil {
		t.Fatal("an expired link was served")
	}
	if s.Failed(version.ID) {
		t.Error("the renewal's failure was kept as the file's")
	}
	// A source answering an error is the file's failure.
	origin.status = http.StatusInternalServerError
	other := library.Version{ID: accounts.ID{28}, URL: "https://93.184.216.34/broken.mkv"}
	if err := s.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), other, Delivery{}); err == nil || !s.Failed(other.ID) {
		t.Errorf("a broken file: %v, failed %v", err, s.Failed(other.ID))
	}
}

// A file relayed to a player is served from the source cache: the bytes
// read before take no request, ranges and the delivery's headers are the
// player's. A live stream is relayed as it comes.
func TestRelayedFilesGoThroughTheCache(t *testing.T) {
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i)
	}
	host := &fileSource{data: data}
	s := newService(t, host, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{29}, URL: "https://93.184.216.34/movie.mkv"}
	s.analyses.Put(version.ID, media.Analysis{Duration: time.Hour, Size: int64(len(data)), Format: "matroska,webm"})
	serve := func(ranges string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Range", ranges)
		response := httptest.NewRecorder()
		if err := s.Serve(response, request, version, Delivery{Relay: true, ContentType: "video/x-matroska", Attachment: "Movie.mkv"}); err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := serve("bytes=100-199")
	if first.Code != http.StatusPartialContent || !bytes.Equal(first.Body.Bytes(), data[100:200]) ||
		first.Header().Get("Content-Range") != "bytes 100-199/3145728" || first.Header().Get("Content-Type") != "video/x-matroska" ||
		first.Header().Get("Content-Disposition") == "" {
		t.Fatalf("%d %q %q", first.Code, first.Header().Get("Content-Range"), first.Header().Get("Content-Type"))
	}
	before := host.count.Load()
	if again := serve("bytes=500-599"); !bytes.Equal(again.Body.Bytes(), data[500:600]) || host.count.Load() != before {
		t.Errorf("a cached range took %d requests", host.count.Load()-before)
	}
	// A live stream, analyzed without a size, is relayed as it comes.
	live := library.Version{ID: accounts.ID{30}, URL: "https://93.184.216.34/channel.ts"}
	s.analyses.Put(live.ID, media.Analysis{Format: "mpegts"})
	if s.cacheable(t.Context(), live) {
		t.Error("a live stream goes through the cache")
	}
}

// What a play from a time reads first: from the start, the head to past
// the second segment's keyframe; from later, the head FFmpeg probes and
// the segment playing then, within the budget.
func TestWarmSpans(t *testing.T) {
	keyframes := []container.Keyframe{{Time: 0, Offset: 1000}, {Time: 6 * time.Second, Offset: 30 << 20},
		{Time: 12 * time.Second, Offset: 60 << 20}, {Time: 18 * time.Second, Offset: 100 << 20}, {Time: 24 * time.Second, Offset: 300 << 20}}
	times := make([]time.Duration, len(keyframes))
	for i, k := range keyframes {
		times[i] = k.Time
	}
	plan := hls.NewPlan(times, 30*time.Second)
	const size = 330 << 20
	for _, tc := range []struct {
		start time.Duration
		want  []source.Span
	}{
		{0, []source.Span{{Off: 0, End: plan0End(plan, keyframes)}}},
		{13 * time.Second, []source.Span{{Off: 0, End: warmHead}, {Off: offsetAt(keyframes, plan.Start(plan.Segment(13*time.Second))),
			End: offsetAt(keyframes, plan.Start(plan.Segment(13*time.Second)+1)) + warmMargin}}},
		// The last segment runs to the end of the file.
		{29 * time.Second, []source.Span{{Off: 0, End: warmHead}, {Off: 300 << 20, End: size}}},
	} {
		got := warmSpans(plan, keyframes, tc.start, size)
		if !slices.Equal(got, tc.want) {
			t.Errorf("from %v: %v, want %v", tc.start, got, tc.want)
		}
		for _, span := range got[1:] {
			if span.End-span.Off > warmBudget {
				t.Errorf("from %v: %v is past the budget", tc.start, span)
			}
		}
	}
	// A long segment is warmed within the budget.
	long := []container.Keyframe{{Time: 0, Offset: 0}, {Time: 10 * time.Second, Offset: 500 << 20}}
	got := warmSpans(hls.NewPlan([]time.Duration{0, 10 * time.Second}, 20*time.Second), long, 0, 1<<30)
	if !slices.Equal(got, []source.Span{{Off: 0, End: warmBudget}}) {
		t.Errorf("a long segment: %v", got)
	}
}

// plan0End is where a play from the start stops being warmed: past the
// keyframe of the second segment.
func plan0End(plan hls.Plan, keyframes []container.Keyframe) int64 {
	return offsetAt(keyframes, plan.Start(1)) + warmMargin
}

// Plan warms the first segment once; Warm from another start replaces a
// warm under way, and is not repeated.
func TestPlanWarmsTheFirstSegment(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codecs.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join("testdata", "codecs.ffprobe.json"))
	if err != nil {
		t.Fatal(err)
	}
	path, _ := fakeProbe(t, string(probe), false)
	host := &fileSource{data: data}
	s := newService(t, host, path, nil)
	version := library.Version{ID: accounts.ID{31}, URL: "https://93.184.216.34/movie.mkv", Filename: "movie.mkv"}
	if _, err := s.Plan(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	warmed := func(start time.Duration) bool {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if at, ok := s.warmed.Get(version.ID); ok && at == start {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	if !warmed(0) {
		t.Fatal("Plan warmed nothing")
	}
	s.Warm(version, 3*time.Second)
	if !warmed(3 * time.Second) {
		t.Fatal("Warm from a resume warmed nothing")
	}
	// Plan does not warm the start over a resume warmed lately.
	if _, err := s.Plan(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if at, _ := s.warmed.Get(version.ID); at != 3*time.Second {
		t.Errorf("warmed from %v", at)
	}
}

// A source failing while a remux reads it has the version kept as failed,
// so that the next PlaybackInfo plays another.
func TestSourcesFailingDuringARemuxFailTheirVersion(t *testing.T) {
	s := newService(t, &fakeSource{status: http.StatusNotFound}, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{32}, URL: "https://93.184.216.34/gone.mkv"}
	src := s.OpenSource(version)
	defer src.Release()
	done := make(chan struct{})
	defer close(done)
	go s.watchSource(version, src, done)
	if _, err := src.ReadAt(t.Context(), make([]byte, 10), 0); !errors.Is(err, source.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !s.Failed(version.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.Failed(version.ID) {
		t.Error("the version is still offered")
	}
}
