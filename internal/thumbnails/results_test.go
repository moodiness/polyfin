package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// resultOf returns where the images of a version stand.
func resultOf(t *testing.T, ts *testService, version accounts.ID) Result {
	t.Helper()
	results, err := ts.Results(t.Context(), []accounts.ID{version})
	if err != nil {
		t.Fatal(err)
	}
	return results[version]
}

// The task makes the images the local folders' files lack, one file at a
// time, and passes over the versions of other sources; while the settings
// turn the images off, it lists nothing and queues nothing.
func TestMakeLocalMakesTheLocalFilesOneAtATime(t *testing.T) {
	t.Parallel()
	ts, first := localFile(t)
	second := first
	second.ID, second.Item = accounts.ID{13}, accounts.ID{14}
	link := testVersion
	versions := []library.Version{first, link, second}
	var listed []accounts.ID
	walk := func(ctx context.Context, each func(library.Version, string) error) error {
		for _, version := range versions {
			listed = append(listed, version.ID)
			if version.ID == second.ID {
				// The first file is made before the next is fed.
				if result := resultOf(t, ts, first.ID); result.State != StateMade {
					t.Errorf("the first file %+v when the second is fed", result)
				}
			}
			if err := each(version, "Home video"); err != nil {
				return err
			}
		}
		return nil
	}

	ts.change(func(s *accounts.Settings) { s.Trickplay, s.ChapterImages = false, false })
	if err := ts.MakeLocal(t.Context(), walk); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 || len(ts.source.count()) != 0 || ts.Status().Waiting != 0 {
		t.Fatalf("turned off, the task listed %v and read %d times", listed, len(ts.source.count()))
	}

	ts.change(func(s *accounts.Settings) { s.Trickplay = true })
	if err := ts.MakeLocal(t.Context(), walk); err != nil {
		t.Fatal(err)
	}
	for _, version := range []library.Version{first, second} {
		if result := resultOf(t, ts, version.ID); result.State != StateMade || result.Count != 15 || result.At == nil {
			t.Errorf("file %v: %+v, made with 15 thumbnails expected", version.ID, result)
		}
	}
	if result := resultOf(t, ts, link.ID); result.State != StateNone {
		t.Errorf("an addon's link: %+v", result)
	}
	if made, cannot, failed, err := ts.LocalCounts(t.Context()); err != nil || made != 2 || cannot != 0 || failed != 0 {
		t.Errorf("local counts %d %d %d, %v", made, cannot, failed, err)
	}
}

// The result of making a version's images is kept: made, with its
// thumbnails; cannot, for an addon's file without an index, or a file
// without video; failed, for a read that failed. The queue tells the
// versions waiting.
func TestResultsAreKept(t *testing.T) {
	t.Parallel()
	ts, made := localFile(t)
	ts.generate(t.Context(), job{version: made, title: "Made", host: hostOf(made.URL)})

	cannot := testVersion
	cannot.ID = accounts.ID{15}
	ts.source = &countingSource{data: []byte("not a video file at all")}
	ts.generate(t.Context(), job{version: cannot, title: "Cannot", host: hostOf(cannot.URL)})

	failed := made
	failed.ID = accounts.ID{16}
	ts.source = &countingSource{data: fixture(t, "forced.mkv"), odd: func(int) error { return errors.New("the disk went away") }}
	ts.generate(t.Context(), job{version: failed, title: "Failed", host: hostOf(failed.URL)})

	// A file of sound alone, not analyzed yet: ffprobe finds no video.
	audio := made
	audio.ID = accounts.ID{18}
	ts.mu.Lock()
	analysis := ts.analysis
	ts.analysis = media.Analysis{}
	ts.mu.Unlock()
	ts.Analyze = func(context.Context, library.Version) (media.Analysis, error) {
		return media.Analysis{}, fmt.Errorf("%w: no video track", media.ErrNotMedia)
	}
	ts.generate(t.Context(), job{version: audio, title: "Audio", host: hostOf(audio.URL)})
	ts.Analyze = nil
	ts.mu.Lock()
	ts.analysis = analysis
	ts.mu.Unlock()

	for _, c := range []struct {
		version       accounts.ID
		state, reason string
		count         int
	}{
		{made.ID, StateMade, "", 15},
		{cannot.ID, StateCannot, ReasonNoIndex, 0},
		{failed.ID, StateFailed, ReasonError, 0},
		{audio.ID, StateCannot, ReasonNoVideo, 0},
	} {
		if result := resultOf(t, ts, c.version); result.State != c.state || result.Reason != c.reason || result.Count != c.count || result.At == nil {
			t.Errorf("version %v: %+v, want %s %q %d", c.version, result, c.state, c.reason, c.count)
		}
	}
	if m, c, f, err := ts.LocalCounts(t.Context()); err != nil || m != 1 || c != 1 || f != 1 {
		t.Errorf("local counts %d %d %d, %v", m, c, f, err)
	}

	waiting := made
	waiting.ID = accounts.ID{17}
	viewer := accounts.ID{7}
	ts.WatchPlaybacks(func() []Playing { return []Playing{{Device: viewer, URL: waiting.URL}} })
	ts.Queue(waiting, "Waiting", viewer)
	if result := resultOf(t, ts, waiting.ID); result.State != StateWaiting {
		t.Errorf("a version queued: %+v", result)
	}
}

// Make again drops a local file's images and makes them again from its
// file, its earlier refusal forgotten; a link is refused, and so is any
// while the settings turn the images off.
func TestMakeAgainReplacesTheImages(t *testing.T) {
	t.Parallel()
	ts, file := localFile(t)
	// A file of sound alone, not analyzed yet: ffprobe finds no video.
	ts.mu.Lock()
	analysis := ts.analysis
	ts.analysis = media.Analysis{}
	ts.mu.Unlock()
	ts.Analyze = func(context.Context, library.Version) (media.Analysis, error) {
		return media.Analysis{}, fmt.Errorf("%w: no video track", media.ErrNotMedia)
	}
	ts.generate(t.Context(), job{version: file, title: "Home video", host: hostOf(file.URL)})
	if result := resultOf(t, ts, file.ID); result.State != StateCannot {
		t.Fatalf("a file without video: %+v", result)
	}
	// The file was replaced by a video.
	ts.mu.Lock()
	ts.analysis = analysis
	ts.mu.Unlock()
	if err := ts.MakeAgain(t.Context(), file, "Home video"); err != nil {
		t.Fatal(err)
	}
	if !madeWithin(t, ts, file, 10*time.Second) {
		t.Fatal("no thumbnails were made again")
	}
	deadline := time.Now().Add(5 * time.Second)
	for resultOf(t, ts, file.ID).State != StateMade && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if result := resultOf(t, ts, file.ID); result.State != StateMade || result.Count != 15 {
		t.Errorf("made again: %+v", result)
	}

	if err := ts.MakeAgain(t.Context(), testVersion, "Link"); !errors.Is(err, ErrNotLocal) {
		t.Errorf("a link made again: %v", err)
	}
	ts.change(func(s *accounts.Settings) { s.Trickplay, s.ChapterImages = false, false })
	if err := ts.MakeAgain(t.Context(), file, "Home video"); !errors.Is(err, ErrTurnedOff) {
		t.Errorf("turned off, made again: %v", err)
	}
	if manifest, _ := ts.Manifest(t.Context(), file.Item); len(manifest) != 0 {
		t.Error("turned off, the thumbnails were dropped")
	}
}

// shortVideo makes a Matroska file of two minutes, a keyframe every 2
// seconds, of noise kept lossless: about 80 MiB, so that what a play
// reads ahead covers only part of it.
func shortVideo(t *testing.T, ffmpeg string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "short.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "nullsrc=s=320x180:r=8,geq=lum='random(1)*255':cb=128:cr=128",
		"-t", "120", "-c:v", "libx264", "-preset", "ultrafast", "-qp", "0", "-g", "16", "-pix_fmt", "yuv420p", "-f", "matroska", path).CombinedOutput()
	if err != nil {
		t.Skipf("FFmpeg cannot make the test file: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A torrent's file gets thumbnails from what its play read alone: none is
// read from its swarm nor opened, during the play or after it, and a later
// play extends the set it kept.
func TestTorrentThumbnailsComeFromItsPlays(t *testing.T) {
	t.Parallel()
	ffmpeg := testFFmpeg(t)
	data := shortVideo(t, ffmpeg)
	swarm := newHost(t, data)
	ts := newTestService(t, ffmpeg)
	ts.change(func(s *accounts.Settings) { s.ChapterImages = false })
	cache, err := source.New(t.TempDir(), func() source.Policy { return source.Policy{Limit: 1 << 30} }, httpOpener{swarm.client}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	var opened atomic.Int32
	ts.mu.Lock()
	ts.cache = cache
	ts.analysis = media.Analysis{Format: "matroska,webm", Duration: 120 * time.Second, Size: int64(len(data)),
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 320, Height: 180}}}
	ts.mu.Unlock()
	ts.Open = func(library.Version) Source {
		opened.Add(1)
		return &countingSource{data: data}
	}
	version := library.Version{ID: accounts.ID{31}, Item: accounts.ID{32}, URL: "magnet:?xt=urn:btih:00ff", Addon: "Torrents",
		Torrent: &library.Torrent{InfoHash: "00ff"}}
	viewer := accounts.ID{3}
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
	src := cache.Open(version.ID, source.Location{URL: swarm.url}, nil)
	defer src.Release()
	keyframes, err := container.KeyframeOffsets(t.Context(), src, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	// pass plays the version to at, extends its thumbnails during the
	// play, stops it and waits for the pass after it, and returns the
	// thumbnails kept.
	pass := func(at time.Duration) Info {
		t.Helper()
		set(Playing{Device: viewer, URL: version.URL})
		ts.Queue(version, "Night Train (2021)", viewer)
		jobs := ts.playingJobs()
		if len(jobs) != 1 || jobs[0].version.ID != version.ID {
			t.Fatalf("versions playing: %+v", jobs)
		}
		play(t, src, keyframes, at)
		requests := swarm.requests.Load()
		ts.extendPlaying(t.Context(), jobs[0])
		info := covered(t, ts, version, at)
		set()
		deadline := time.Now().Add(5 * time.Second)
		for status := ts.Status(); (status.Waiting > 0 || status.Working) && time.Now().Before(deadline); status = ts.Status() {
			time.Sleep(5 * time.Millisecond)
		}
		if status := ts.Status(); status.Waiting > 0 || status.Working {
			t.Fatalf("the version still waits after its play: %+v", status)
		}
		if after := swarm.requests.Load(); after != requests || opened.Load() != 0 {
			t.Fatalf("the thumbnails made %d requests and opened the file %d times", after-requests, opened.Load())
		}
		if result := resultOf(t, ts, version.ID); result.State != StateMade || result.Count < info.ThumbnailCount {
			t.Errorf("after the play: %+v, %d thumbnails made", result, info.ThumbnailCount)
		}
		return info
	}
	first := pass(30 * time.Second)
	if complete(first, 120*time.Second) {
		t.Fatalf("half a minute played made every thumbnail: %+v", first)
	}
	second := pass(80 * time.Second)
	if second.ThumbnailCount <= first.ThumbnailCount {
		t.Errorf("a later play kept %d thumbnails, after %d", second.ThumbnailCount, first.ThumbnailCount)
	}
}
