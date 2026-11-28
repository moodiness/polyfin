package playback

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// spreadStream is the SubRip stream of the file spread makes.
const spreadStream = 1

// spread makes, with ffmpeg, a Matroska file of seconds of uncompressed
// audio, 192 kB a second, and a SubRip track of a cue every two seconds:
// each block takes a range of its own, as a movie's, and the track spans
// the file, past what reading its head and Cues brings into the source
// cache. It returns the file's folder, its analysis, and the track's cues.
func spread(t *testing.T, seconds int) (string, media.Analysis, []subtitles.Cue) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	var srt strings.Builder
	for i := range seconds / 2 {
		at := 2 * i
		fmt.Fprintf(&srt, "%d\n00:%02d:%02d,000 --> 00:%02d:%02d,500\nLine %d.\n\n", i+1, at/60, at%60, at/60, at%60, i+1)
	}
	if err := os.WriteFile(filepath.Join(dir, "spread.srt"), []byte(srt.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-i", filepath.Join(dir, "spread.srt"), "-map", "0:a", "-map", "1",
		"-t", strconv.Itoa(seconds), "-c:a", "pcm_s16le", "-c:s", "srt", filepath.Join(dir, "spread.mkv"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	info, err := os.Stat(filepath.Join(dir, "spread.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	analysis := media.Analysis{Format: "matroska,webm", Duration: time.Duration(seconds) * time.Second, Size: info.Size(),
		Streams: []media.Stream{{Index: 0, Type: "audio", Codec: "pcm_s16le"}, {Index: spreadStream, Type: "subtitle", Codec: "subrip"}}}
	want, err := subtitles.Parse([]byte(srt.String()))
	if err != nil {
		t.Fatal(err)
	}
	return dir, analysis, want
}

// oneRangeHost serves the files of a folder as hosts serving one range per
// request do, a request for several answered with 416. It records the
// offset each request starts at, and the bytes it sends, answers 509 once
// quota is set, as a host whose bandwidth quota is spent, and sends at
// most rate bytes a second when rate is set.
type oneRangeHost struct {
	files   http.Handler
	rate    int64
	quota   atomic.Bool
	several atomic.Int64
	sent    atomic.Int64

	mu     sync.Mutex
	starts []int64
}

// serveSpread serves spread's folder from a host serving one range per
// request, at rate bytes a second when it is set, with a service reading
// it, its version and that version's analysis kept.
func serveSpread(t *testing.T, dir string, analysis media.Analysis, rate int64) (*Service, *oneRangeHost, library.Version) {
	t.Helper()
	host := &oneRangeHost{files: http.FileServer(http.Dir(dir)), rate: rate}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	s := newService(t, &fileOpener{}, "ffprobe", nil)
	s.fills.poll = 10 * time.Millisecond
	s.fills.speed = func() (int64, int64) { return 0, 0 }
	version := library.Version{ID: accounts.ID{7}, URL: server.URL + "/spread.mkv", Addon: "files"}
	s.analyses.Put(version.ID, analysis)
	return s, host, version
}

func (h *oneRangeHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ranges := r.Header.Get("Range")
	if strings.Contains(ranges, ",") {
		h.several.Add(1)
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(ranges, "bytes="), "-")
	start, _ := strconv.ParseInt(first, 10, 64)
	h.mu.Lock()
	h.starts = append(h.starts, start)
	h.mu.Unlock()
	if h.quota.Load() {
		w.WriteHeader(statusQuotaSpent)
		return
	}
	w = &countingWriter{ResponseWriter: w, host: h, started: time.Now()}
	h.files.ServeHTTP(w, r)
}

// requests returns the offsets the requests made since the first n
// started at, and how many were made in all.
func (h *oneRangeHost) requests(n int) ([]int64, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.starts[n:]), len(h.starts)
}

// countingWriter writes a response, counting the bytes its host sends, at
// the host's rate at most.
type countingWriter struct {
	http.ResponseWriter
	host    *oneRangeHost
	started time.Time
	written int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.written += int64(n)
	w.host.sent.Add(int64(n))
	if w.host.rate > 0 {
		time.Sleep(time.Until(w.started.Add(time.Duration(w.written * int64(time.Second) / w.host.rate))))
	}
	return n, err
}

// keptTrackRows counts the tracks of a version kept in the database.
func keptTrackRows(t *testing.T, s *Service, version library.Version) int {
	t.Helper()
	var kept int
	if err := s.db.QueryRow(t.Context(), "SELECT count(*) FROM media_subtitle_tracks WHERE version_id = $1", version.ID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	return kept
}

// A track a host serving one range per request would take more than its
// share of requests to serve awaits a fill: it is not offered, and once
// the version played, the fill reads its span into the source cache while
// nothing plays, then the track whole from there, and offers it, kept.
func TestTracksPastTheirShareAreOfferedAfterAFill(t *testing.T) {
	t.Parallel()
	dir, analysis, want := spread(t, 60)
	s, host, version := serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	s.fills.quiet, s.fills.wait = 0, 10*time.Millisecond
	ctx := t.Context()
	if located := s.SubtitlesLocated(ctx, version, analysis); len(located) != 0 {
		t.Fatalf("located from a host serving a range per request: %v", located)
	}
	if index, _ := s.locations.Get(version.ID); !slices.Equal(index.awaiting, []int{spreadStream}) {
		t.Fatalf("awaiting %v", index.awaiting)
	}
	s.FillSubtitles(version, analysis)
	deadline := time.Now().Add(20 * time.Second)
	for !s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		if time.Now().After(deadline) {
			t.Fatal("the track is not offered after the fill")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, made := host.requests(0)
	track, err := s.SubtitleTrack(ctx, version, analysis, spreadStream)
	if err != nil || track.Format != "srt" {
		t.Fatalf("the track: %s %v", track.Format, err)
	}
	if got, err := subtitles.Parse(track.Data); err != nil || !slices.EqualFunc(got, want, sameCue) {
		t.Errorf("cues:\n got %+v\nwant %+v", got, want)
	}
	if _, all := host.requests(0); all != made {
		t.Errorf("%d requests to read the track after the fill", all-made)
	}
	if kept := keptTrackRows(t, s, version); kept != 1 {
		t.Errorf("%d tracks kept", kept)
	}
	var located, awaiting []int32
	if err := s.db.QueryRow(ctx, "SELECT located, awaiting FROM media_subtitle_index WHERE version_id = $1", version.ID).
		Scan(&located, &awaiting); err != nil || !slices.Equal(located, []int32{spreadStream}) || len(awaiting) != 0 {
		t.Errorf("the index kept: located %v, awaiting %v, %v", located, awaiting, err)
	}
}

// A playback from the host coming while a fill reads holds it at once,
// within the piece being read, and no track is offered; the fill resumes
// after it, fetching only what it had not read.
func TestAFillHeldByAPlaybackResumesAfterIt(t *testing.T) {
	t.Parallel()
	dir, analysis, want := spread(t, 60)
	s, host, version := serveSpread(t, dir, analysis, 4<<20)
	s.trackShare = 4
	ctx := t.Context()
	if s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Fatal("the track is located before the fill")
	}
	src := s.open(version)
	defer src.Release()
	whole := source.Span{Off: 0, End: analysis.Size}
	missing := src.Missing(whole)
	other := s.open(library.Version{ID: accounts.ID{8}, URL: strings.TrimSuffix(version.URL, "spread.mkv") + "other.mkv"})
	defer other.Release()
	// A playback comes once the fill read half of what it lacked, about
	// halfway through its first 8 MiB piece, which takes 2 s at 4 MB/s.
	var stopPlaying func()
	urged := make(chan struct{})
	go func() {
		defer close(urged)
		for src.Missing(whole) > missing/2 {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		stopPlaying = other.Urge()
	}()
	fetched, err := s.fill(ctx, version, analysis, 0, src.Host())
	<-urged
	if !errors.Is(err, errFillHeld) || stopPlaying == nil {
		t.Fatalf("the fill a playback came during: %v", err)
	}
	// Preload counts the pieces it read whole: the fill stopped within
	// its first, not at its end, as Preload alone would have.
	if fetched != 0 {
		t.Errorf("the fill stopped after reading %d bytes, at the end of a piece", fetched)
	}
	if s.SubtitlesLocated(ctx, version, analysis)[spreadStream] || keptTrackRows(t, s, version) != 0 {
		t.Fatal("a track offered from a fill held")
	}
	if _, err := s.SubtitleTrack(ctx, version, analysis, spreadStream); !errors.Is(err, errTrackAwaited) {
		t.Errorf("the track asked during the fill: %v", err)
	}

	stopPlaying()
	// What the connections of the fill held had under way lands meanwhile.
	time.Sleep(100 * time.Millisecond)
	left, sent := src.Missing(whole), host.sent.Load()
	if left == 0 || left >= missing {
		t.Errorf("%d bytes of %d left once held", left, missing)
	}
	if _, err := s.fill(ctx, version, analysis, fetched, src.Host()); err != nil {
		t.Fatalf("the fill after the playback: %v", err)
	}
	// A block a connection had under way may be fetched again.
	if again := host.sent.Load() - sent; again > left+1<<20 {
		t.Errorf("the fill resumed fetched %d bytes, %d were left", again, left)
	}
	if !s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Fatal("the track is not offered after the fill")
	}
	track, err := s.SubtitleTrack(ctx, version, analysis, spreadStream)
	if got, perr := subtitles.Parse(track.Data); err != nil || perr != nil || !slices.EqualFunc(got, want, sameCue) {
		t.Errorf("the track: %v, %v", err, perr)
	}
}

// A track whose span the source cache holds, as a relayed playback brought
// it in, is located and read with no request.
func TestTracksTheCacheHoldsAreReadWithNoRequest(t *testing.T) {
	t.Parallel()
	dir, analysis, want := spread(t, 60)
	s, host, version := serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	ctx := t.Context()
	src := s.open(version)
	err := src.Warm(ctx, source.Span{Off: 0, End: analysis.Size})
	src.Release()
	if err != nil {
		t.Fatal(err)
	}
	_, made := host.requests(0)
	if !s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Fatal("the track the cache holds is not located")
	}
	track, err := s.SubtitleTrack(ctx, version, analysis, spreadStream)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := subtitles.Parse(track.Data); err != nil || !slices.EqualFunc(got, want, sameCue) {
		t.Errorf("cues:\n got %+v\nwant %+v", got, want)
	}
	if _, all := host.requests(0); all != made || host.several.Load() != 0 {
		t.Errorf("%d requests, %d for several ranges", all-made, host.several.Load())
	}
}

// A fill its host stops answering, with 509 as a host whose bandwidth
// quota is spent, offers no track, and the host's fills wait a day,
// rather than ask again.
func TestAFillThatCannotEndOffersNoTrack(t *testing.T) {
	t.Parallel()
	dir, analysis, _ := spread(t, 60)
	s, host, version := serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	s.fills.quiet, s.fills.wait = time.Hour, time.Hour
	ctx := t.Context()
	if s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Fatal("the track is located before the fill")
	}
	host.quota.Store(true)
	if _, err := s.SubtitleTrack(ctx, version, analysis, spreadStream); !errors.Is(err, errTrackAwaited) {
		t.Fatalf("the track asked: %v", err)
	}
	s.fills.mu.Lock()
	job := s.fills.jobs[version.ID]
	s.fills.mu.Unlock()
	if job == nil {
		t.Fatal("asking for the track queued no fill")
	}
	_, before := host.requests(0)
	s.runFill(job)
	if _, after := host.requests(0); after == before || after-before > 5 {
		t.Errorf("%d requests to a host over its quota", after-before)
	}
	if s.SubtitlesLocated(ctx, version, analysis)[spreadStream] || keptTrackRows(t, s, version) != 0 {
		t.Error("a track offered from a fill that did not end")
	}
	s.fills.mu.Lock()
	held, queued := s.fills.held[hostOf(version.URL)], s.fills.jobs[version.ID] == job
	s.fills.quiet = 0
	s.fills.mu.Unlock()
	if time.Until(held) < quotaWait-time.Minute || !queued {
		t.Errorf("the host's fills wait until %v; the fill queued: %v", held, queued)
	}
	if next, wait := s.nextFill(time.Now()); next != nil || wait <= 0 {
		t.Errorf("the next fill: %v, in %v", next, wait)
	}
}

// queued queues the fill of version, which runs only when the test runs
// it, and returns its job. The fills' loop is taken as running, so that
// queueFill starts none: the loop would run the job once the test makes it
// ready, beside the test's own fills of the version.
func queued(t *testing.T, s *Service, version library.Version, analysis media.Analysis) *fillJob {
	t.Helper()
	s.fills.mu.Lock()
	s.fills.running = true
	s.fills.mu.Unlock()
	s.queueFill(version, analysis)
	s.fills.mu.Lock()
	defer s.fills.mu.Unlock()
	job := s.fills.jobs[version.ID]
	if job == nil {
		t.Fatal("no fill queued")
	}
	return job
}

// A host asking to slow down or overloaded holds its fills an hour, one
// whose quota is spent a day, and they start again once that passed. A
// fill failing otherwise is tried again 1, 2 and 4 hours later, then given
// up for a while: asked again, it is not queued.
func TestFillsWaitForTheirHostAndTheirFailures(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileOpener{}, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{7}, URL: "http://host.example/movie.mkv", Addon: "files"}
	job := queued(t, s, version, media.Analysis{})
	held := func() time.Time {
		s.fills.mu.Lock()
		defer s.fills.mu.Unlock()
		return s.fills.held["host.example"]
	}
	now := time.Now()
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		s.fillDone(job, 0, &source.StatusError{Status: status, Kind: source.ErrUnavailable}, now)
		if until := held(); !until.Equal(now.Add(slowWait)) {
			t.Errorf("HTTP %d holds the host until %v, want an hour", status, until)
		}
	}
	s.fillDone(job, 0, fmt.Errorf("fetching: %w", &source.StatusError{Status: statusQuotaSpent, Kind: source.ErrUnavailable}), now)
	if until := held(); !until.Equal(now.Add(quotaWait)) {
		t.Errorf("HTTP 509 holds the host until %v, want a day", until)
	}
	s.fills.mu.Lock()
	s.fills.quiet = 0
	s.fills.mu.Unlock()
	if next, wait := s.nextFill(now.Add(quotaWait - time.Minute)); next != nil || wait <= 0 {
		t.Errorf("a fill of a host held: %v, in %v", next, wait)
	}
	if next, _ := s.nextFill(now.Add(quotaWait + time.Second)); next != job {
		t.Error("the fill does not start once the host's quota passed")
	}
	s.fills.mu.Lock()
	if len(s.fills.held) != 0 {
		t.Errorf("hosts held kept once passed: %v", s.fills.held)
	}
	s.fills.mu.Unlock()

	failed := errors.New("the connection broke")
	for _, wait := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour} {
		s.fillDone(job, 0, failed, now)
		s.fills.mu.Lock()
		retry, kept := job.retry, s.fills.jobs[version.ID] == job
		s.fills.mu.Unlock()
		if !retry.Equal(now.Add(wait)) || !kept {
			t.Errorf("tried again at %v, queued: %v; want after %v", retry, kept, wait)
		}
	}
	s.fillDone(job, 0, failed, now)
	s.queueFill(version, media.Analysis{})
	s.fills.mu.Lock()
	defer s.fills.mu.Unlock()
	if _, queued := s.fills.jobs[version.ID]; queued {
		t.Error("a fill given up is queued again")
	}
}

// A span over half the source cache is not read, nor one a version's
// fills fetched twice already. A day's fills fetch at most the cache's
// size, and wait for the next day; a pass at most a quarter of it, or a
// piece, the next pass going on from there.
func TestFillsAreBoundedByTheCache(t *testing.T) {
	t.Parallel()
	dir, analysis, _ := spread(t, 60)
	s, _, version := serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	ctx := t.Context()
	if s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Fatal("the track is located before the fill")
	}
	src := s.open(version)
	defer src.Release()
	host := src.Host()

	s.fills.limit = func() int64 { return analysis.Size }
	if _, err := s.fill(ctx, version, analysis, 0, host); !errors.Is(err, errFillTooLarge) {
		t.Errorf("a span over half the cache: %v", err)
	}
	s.fills.limit = func() int64 { return 1 << 30 }
	if _, err := s.fill(ctx, version, analysis, 2*analysis.Size, host); !errors.Is(err, errFillSpent) {
		t.Errorf("a span fetched twice: %v", err)
	}

	now := time.Now()
	s.fills.mu.Lock()
	s.fills.day, s.fills.spent = now, 1<<30
	s.fills.mu.Unlock()
	if _, err := s.fill(ctx, version, analysis, 0, host); !errors.Is(err, errFillDay) {
		t.Errorf("past the day's bytes: %v", err)
	}
	job := queued(t, s, version, analysis)
	s.fillDone(job, 0, errFillDay, now)
	s.fills.mu.Lock()
	s.fills.quiet = 0
	s.fills.mu.Unlock()
	if next, _ := s.nextFill(now.Add(time.Hour)); next != nil {
		t.Error("a fill started past the day's bytes")
	}
	if next, _ := s.nextFill(now.Add(fillDay + time.Second)); next != job {
		t.Error("the fill does not start the next day")
	}

	// A cache of 24 MB takes a span of 12, in passes of a piece.
	s.fills.limit = func() int64 { return 24 << 20 }
	missing := src.Missing(source.Span{Off: 0, End: analysis.Size})
	fetched, err := s.fill(ctx, version, analysis, 0, host)
	if !errors.Is(err, source.ErrPreloadHeld) || fetched == 0 || fetched >= missing {
		t.Fatalf("the first pass fetched %d of %d: %v", fetched, missing, err)
	}
	s.fillDone(job, fetched, err, now)
	s.fills.mu.Lock()
	retry := job.retry
	s.fills.mu.Unlock()
	if !retry.Equal(now.Add(fillPause)) {
		t.Errorf("the next pass at %v, want %v", retry, now.Add(fillPause))
	}
	if _, err := s.fill(ctx, version, analysis, fetched, host); err != nil {
		t.Fatalf("the second pass: %v", err)
	}
	if !s.SubtitlesLocated(ctx, version, analysis)[spreadStream] {
		t.Error("the track is not offered after the second pass")
	}
}

// The queue keeps maxFills versions, dropping the one asked the longest
// ago, and never a torrent's.
func TestTheFillQueueIsBounded(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileOpener{}, "ffprobe", nil)
	versions := make([]library.Version, maxFills+1)
	base := time.Now()
	for i := range versions {
		versions[i] = library.Version{ID: accounts.ID{byte(i), 1}, URL: fmt.Sprintf("http://host.example/%d.mkv", i), Addon: "files"}
		if i < maxFills {
			job := queued(t, s, versions[i], media.Analysis{})
			s.fills.mu.Lock()
			job.asked = base.Add(time.Duration(i) * time.Second)
			s.fills.mu.Unlock()
		}
	}
	s.queueFill(versions[maxFills], media.Analysis{})
	s.queueFill(library.Version{ID: accounts.ID{0, 2}, URL: "http://host.example/torrent.mkv", Torrent: &library.Torrent{}}, media.Analysis{})
	s.fills.mu.Lock()
	defer s.fills.mu.Unlock()
	if _, kept := s.fills.jobs[versions[0].ID]; kept || len(s.fills.jobs) != maxFills {
		t.Errorf("%d fills queued, the one asked first kept: %v", len(s.fills.jobs), kept)
	}
	if _, kept := s.fills.jobs[versions[maxFills].ID]; !kept {
		t.Error("the version asked last is not queued")
	}
	if _, kept := s.fills.jobs[accounts.ID{0, 2}]; kept {
		t.Error("a torrent's file is queued")
	}
}

// An index kept before fills, whose awaiting is NULL, is read again only
// when it does not locate every stream that may be: the streams its host
// would take too many requests to serve then await a fill. A read again
// that fails keeps the streams located before.
func TestIndexesKeptBeforeFillsAreReadAgain(t *testing.T) {
	t.Parallel()
	dir, analysis, _ := spread(t, 60)
	keep := func(s *Service, version library.Version, located []int32) {
		t.Helper()
		if _, err := s.db.Exec(t.Context(), "INSERT INTO media_subtitle_index (version_id, located) VALUES ($1, $2)", version.ID, located); err != nil {
			t.Fatal(err)
		}
	}

	// Every stream located: not read again.
	s, host, version := serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	keep(s, version, []int32{spreadStream})
	if located := s.SubtitlesLocated(t.Context(), version, analysis); !located[spreadStream] {
		t.Errorf("located %v", located)
	}
	if _, made := host.requests(0); made != 0 {
		t.Errorf("%d requests for an index that locates every stream", made)
	}

	// A stream left out: read again, it awaits a fill.
	s, host, version = serveSpread(t, dir, analysis, 0)
	s.trackShare = 4
	keep(s, version, []int32{})
	if located := s.SubtitlesLocated(t.Context(), version, analysis); len(located) != 0 {
		t.Errorf("located %v", located)
	}
	var awaiting []int32
	if err := s.db.QueryRow(t.Context(), "SELECT awaiting FROM media_subtitle_index WHERE version_id = $1", version.ID).Scan(&awaiting); err != nil ||
		!slices.Equal(awaiting, []int32{spreadStream}) {
		t.Errorf("awaiting %v, %v", awaiting, err)
	}
	if _, made := host.requests(0); made == 0 {
		t.Error("the index was not read again")
	}

	// Read again from a host that fails: what was located stays offered.
	s, host, version = serveSpread(t, dir, analysis, 0)
	s.locateTime = 2 * time.Second
	host.quota.Store(true)
	more := analysis
	more.Streams = append(slices.Clone(analysis.Streams), media.Stream{Index: 2, Type: "subtitle", Codec: "ass"})
	s.analyses.Put(version.ID, more)
	keep(s, version, []int32{spreadStream})
	if located := s.SubtitlesLocated(t.Context(), version, more); !located[spreadStream] {
		t.Errorf("located %v once the index could not be read again", located)
	}
	if _, failed := s.unlocated.Get(version.ID); failed {
		t.Error("the version is kept as not located")
	}
	if _, made := host.requests(0); made == 0 {
		t.Error("the index was not read again")
	}
}
