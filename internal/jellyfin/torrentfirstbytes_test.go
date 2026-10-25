package jellyfin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A torrent version whose first bytes come later than the analysis used to
// wait for them, 105 seconds, but keep coming, plays at its first play: the
// analysis waits for the torrent as the play does. The engine's start of
// 40 seconds scales the play's waits: past 70 seconds while the pieces
// keep coming, up to 140.
func TestATorrentWhoseFirstBytesComeSlowlyPlaysAtItsFirstPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, file := vodFile(t, "a-movie.mkv", "-s", "320x180", "-b:v", "150k")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= 1<<20 {
		t.Fatalf("the movie is %d bytes: its first bytes, the source's first block, are not the whole file", len(data))
	}
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 40*time.Second, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// The file's blocks of 16 KiB come in about 120 seconds, a block every
	// few seconds, well within a piece's wait.
	blocks := (len(data) + 16<<10 - 1) / (16 << 10)
	seeder.Delay(120 * time.Second / time.Duration(blocks))
	if info, code := p.play(t, picked); code != "" || len(info.MediaSources) == 0 {
		t.Fatalf("over a swarm whose first bytes come slowly but keep coming, PlaybackInfo answered %q", code)
	}
}

// A torrent version whose pieces come too slowly even for the play's
// longest wait is told to try later, never that it suits no app, and is not
// kept as failed: once its swarm turns fast, the next play plays it. The
// engine's start of 2 seconds scales the play's longest wait to 64
// seconds.
func TestATorrentTooSlowForThePlayIsToldToTryLater(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := slowTorrent(t)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// A block of 16 KiB every 5 seconds: the first MiB would take over 5
	// minutes, though a block comes well within a piece's wait.
	seeder.Delay(5 * time.Second)
	if _, code := p.play(t, picked); code != "RateLimitExceeded" {
		t.Fatalf("over a swarm too slow for the play, PlaybackInfo answered %q", code)
	}
	seeder.Delay(0)
	if info, code := p.play(t, picked); code != "" || len(info.MediaSources) == 0 {
		t.Fatalf("once the swarm is fast, PlaybackInfo answered %q", code)
	}
}
