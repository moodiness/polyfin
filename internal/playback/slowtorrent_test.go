package playback

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// slowTorrent is a service playing a version that is a single-file
// torrent of data, analyzed as analysis, whose only peer holds its pieces
// until released: what reads it waits, as over a swarm too slow for the
// read. The torrent starts within 2 seconds, and runs all along, as for a
// playback, once play is called.
type slowTorrent struct {
	*Service
	version  library.Version
	analysis media.Analysis
	seeder   *torrenttest.Peer
	set      func(change func(*accounts.Settings))
	// play joins the torrent, as a playback does, until the test ends;
	// rest has the torrent stop, as once a play's linger ended.
	play, rest func()
}

func newSlowTorrent(t *testing.T, ffprobe string, id byte, data []byte, analysis media.Analysis) slowTorrent {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sources, err := source.New(t.TempDir(), func() int64 { return 1 << 30 }, &fakeSource{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sources.Close() })
	segments, err := hls.NewManager("ffmpeg", t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	var mu sync.Mutex
	settings := accounts.DefaultSettings()
	s, err := New(migrated.New(t), &fakeSource{}, ffprobe, NewSigner([]byte("test secret")), sources, segments, nil, logger, func() accounts.Settings {
		mu.Lock()
		defer mu.Unlock()
		return settings
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return torrent.Settings{Enabled: true, Proxy: proxy.Address()} },
		Dir: t.TempDir(), Budget: func() int64 { return 256 << 20 }, StartTimeout: 2 * time.Second, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	s.Torrents(engine)
	sources2 := []string{"tracker:" + tracker.URL()}
	version := library.Version{ID: accounts.ID{id}, URL: "magnet:?xt=urn:btih:" + tor.HashHex(), Addon: "Torrents", Filename: "a-movie.mkv",
		Torrent: &library.Torrent{InfoHash: tor.HashHex(), Sources: sources2}}
	analysis.Size = int64(len(data))
	s.analyses.Put(version.ID, analysis)
	spec, err := torrent.NewSpec(tor.HashHex(), nil, sources2)
	if err != nil {
		t.Fatal(err)
	}
	var stops []func()
	// The peer tells the torrent's files, then holds every piece.
	seeder.Hold()
	t.Cleanup(seeder.Release)
	return slowTorrent{Service: s, version: version, analysis: analysis, seeder: seeder, set: func(change func(*accounts.Settings)) {
		mu.Lock()
		defer mu.Unlock()
		change(&settings)
	}, play: func() {
		stop := engine.Play(spec)
		t.Cleanup(stop)
		stops = append(stops, stop)
	}, rest: func() {
		for _, stop := range stops {
			stop()
		}
		stops = nil
		hash, err := torrent.ParseHash(tor.HashHex())
		if err != nil {
			t.Fatal(err)
		}
		engine.End(hash)
		for deadline := time.Now().Add(10 * time.Second); engine.Running(hash); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the torrent still runs")
			}
		}
	}}
}

// played has the version read as the next play reads it, its source urged,
// until the test ends: a stall its read met before is forgotten (see
// source.Source.Urge).
func (p slowTorrent) played(t *testing.T) {
	t.Helper()
	src := p.OpenSource(p.version)
	t.Cleanup(src.Release)
	t.Cleanup(src.Urge())
}

// subtitledFixture is the Matroska fixture with an SRT and an ASS track,
// and its analysis.
func subtitledFixture(t *testing.T) ([]byte, media.Analysis) {
	t.Helper()
	dir := filepath.Join("..", "container", "testdata")
	data, err := os.ReadFile(filepath.Join(dir, "subtitles.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join(dir, "subtitles.ffprobe.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	return data, analysis
}

// The subtitles of a torrent version whose index could not be read in
// time, its swarm too slow for the read, are located at the next play once
// the swarm sends, rather than left unlocated for a while: the swarm's
// slowness is no failure of the version.
func TestSubtitlesOfASlowTorrentAreLocatedOnceItSends(t *testing.T) {
	t.Parallel()
	data, analysis := subtitledFixture(t)
	p := newSlowTorrent(t, "ffprobe", 31, data, analysis)
	p.play()
	// The read gives up at its own bound, the peer sending nothing.
	if located := p.SubtitlesLocated(t.Context(), p.version, p.analysis); len(located) != 0 {
		t.Fatalf("located %v from a peer sending nothing", located)
	}
	p.seeder.Release()
	if located := p.SubtitlesLocated(t.Context(), p.version, p.analysis); len(located) != 2 {
		t.Fatalf("once the peer sends, located %v", located)
	}
}

// A subtitle track of a torrent version, located before, whose read
// stalled over a swarm sending nothing, is read at the next ask once the
// swarm sends, rather than refused for a while.
func TestASubtitleTrackOfAStalledTorrentIsReadOnceItSends(t *testing.T) {
	t.Parallel()
	data, analysis := subtitledFixture(t)
	p := newSlowTorrent(t, "ffprobe", 32, data, analysis)
	p.play()
	const srtStream = 1
	// Located at an earlier play: the index is not read again.
	if _, err := p.db.Exec(t.Context(), "INSERT INTO media_subtitle_index (version_id, located) VALUES ($1, $2)",
		p.version.ID, []int32{1, 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SubtitleTrack(t.Context(), p.version, p.analysis, srtStream); !errors.Is(err, source.ErrStalled) {
		t.Fatalf("over a peer sending nothing, the track read gave %v", err)
	}
	p.seeder.Release()
	p.played(t)
	if track, err := p.SubtitleTrack(t.Context(), p.version, p.analysis, srtStream); err != nil || len(track.Data) == 0 {
		t.Fatalf("once the peer sends, the track read gave %v", err)
	}
}

// The first frame of a torrent version's HDR video, which ffprobe could
// not read in time over a swarm too slow for it, is read at the next play
// once the swarm sends, rather than leaving the version to the processor
// for a while.
func TestTheMasteringDisplayOfASlowTorrentIsLearnedOnceItSends(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	data, analysis := subtitledFixture(t)
	// An analysis made before Polyfin recorded the mastering display, of
	// PQ video.
	analysis.Streams = slices.Clone(analysis.Streams)
	analysis.Streams[0].Codec, analysis.Streams[0].ColorTransfer, analysis.Streams[0].MasteringDisplay = "hevc", "smpte2084", nil
	p := newSlowTorrent(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 33, data, analysis)
	p.play()
	p.set(func(s *accounts.Settings) { s.AnalysisTimeout = accounts.MinAnalysisTimeout; s.GPUToneMapping = true })
	intel := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	conversion := &VideoConversion{Codec: "h264", Encoder: "h264_vaapi", ToneMap: true, ToneMapOnCPU: true, Hardware: intel}
	if _, learned := p.LearnMasteringDisplay(t.Context(), p.version, p.analysis, conversion); learned {
		t.Fatal("learned from a peer sending nothing")
	}
	p.seeder.Release()
	if got, learned := p.LearnMasteringDisplay(t.Context(), p.version, p.analysis, conversion); !learned || got.Streams[0].MasteringDisplay == nil {
		t.Fatalf("once the peer sends, learned %v: %+v", learned, got.Streams[0])
	}
}

// A picture subtitle track of a torrent version, whose read into text
// stalled over a swarm sending nothing, is read when queued again once the
// swarm sends, rather than left unread for hours.
func TestAPictureTrackOfAStalledTorrentIsReadOnceItSends(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "captions.sup"), pgsCaptions.PGS(), 0o600); err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(dir, "movie.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-i", filepath.Join(dir, "captions.sup"), "-t", "8", "-map", "0", "-map", "1", "-c:v", "libx264", "-preset", "veryfast",
		"-c:s", "copy", "-metadata:s:s:0", "language=eng", movie).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	data, err := os.ReadFile(movie)
	if err != nil {
		t.Fatal(err)
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), movie)
	if err != nil {
		t.Fatal(err)
	}
	p := newSlowTorrent(t, ffprobe, 34, data, analysis)
	p.play()
	p.set(func(s *accounts.Settings) { s.SubtitleOCR = "played" })
	reader := &fakeReader{languages: []string{"eng"}}
	p.UseTesseract(ffmpeg, t.TempDir(), reader)
	read := func() []TextCopy {
		p.queueTracks(context.WithoutCancel(t.Context()), p.version, p.analysis, nil, -1)
		for deadline := time.Now().Add(3 * time.Minute); ; time.Sleep(50 * time.Millisecond) {
			p.pictures.mu.Lock()
			idle := p.pictures.running == 0 && len(p.pictures.queue) == 0
			p.pictures.mu.Unlock()
			if idle {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the reads did not end")
			}
		}
		return p.TextCopies(t.Context(), p.version, p.analysis, nil)
	}
	if copies := read(); len(copies) != 0 {
		t.Fatalf("read into text from a peer sending nothing: %+v", copies)
	}
	p.seeder.Release()
	p.played(t)
	if copies := read(); len(copies) != 1 {
		t.Fatalf("once the peer sends, the text copies are %+v", copies)
	}
}

// The subtitles of a torrent version looked for ahead of a play, as its
// page opens, while no playback plays its torrent, are located at the
// play: a torrent is read only while it plays, which is no failure of the
// version.
func TestSubtitlesOfATorrentLookedForAheadOfItsPlayAreLocatedAtThePlay(t *testing.T) {
	t.Parallel()
	data, analysis := subtitledFixture(t)
	p := newSlowTorrent(t, "ffprobe", 35, data, analysis)
	p.seeder.Release()
	if located := p.SubtitlesLocated(Background(t.Context()), p.version, p.analysis); len(located) != 0 {
		t.Fatalf("located %v while no playback plays the torrent", located)
	}
	p.play()
	p.played(t)
	if located := p.SubtitlesLocated(t.Context(), p.version, p.analysis); len(located) != 2 {
		t.Fatalf("at the play, located %v", located)
	}
}

// A subtitle track of a torrent version asked for while no playback plays
// its torrent, as when the player paused past the torrent's linger, is read
// at the next play rather than refused for a while. The file's head was
// read at the play, the track's blocks past it are fetched by their
// ranges.
func TestASubtitleTrackAskedForWhileItsTorrentRestsIsReadAtTheNextPlay(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "late.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:15,000 --> 00:00:17,000\nThe lighthouse keeper waited.\n\n"+
		"2\n00:00:18,000 --> 00:00:19,500\nSeven boats came home.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(dir, "movie.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=20",
		"-i", srt, "-map", "0", "-map", "1", "-c:v", "libx264", "-preset", "veryfast", "-b:v", "1500k", "-g", "48", "-c:s", "srt",
		"-metadata:s:s:0", "language=eng", movie).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	data, err := os.ReadFile(movie)
	if err != nil {
		t.Fatal(err)
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), movie)
	if err != nil {
		t.Fatal(err)
	}
	p := newSlowTorrent(t, ffprobe, 36, data, analysis)
	p.seeder.Release()
	const srtStream = 1
	// The play locates the track, reading the file's head and its index.
	p.play()
	if !p.SubtitlesLocated(t.Context(), p.version, p.analysis)[srtStream] {
		t.Fatal("the track is not located")
	}
	p.rest()
	if _, err := p.SubtitleTrack(t.Context(), p.version, p.analysis, srtStream); err == nil {
		t.Fatal("the track was read while no playback plays the torrent")
	}
	p.play()
	p.played(t)
	if !p.SubtitlesLocated(t.Context(), p.version, p.analysis)[srtStream] {
		t.Fatal("at the next play, the track is not offered")
	}
	if track, err := p.SubtitleTrack(t.Context(), p.version, p.analysis, srtStream); err != nil || len(track.Data) == 0 {
		t.Fatalf("at the next play, the track read gave %v", err)
	}
}
