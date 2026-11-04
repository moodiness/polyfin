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
// seconds. The next play asks as the first one's analysis gives up on the
// torrent, and may join it just before it does: that verdict is not the
// next play's.
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

// A play that joins the analysis of a torrent version an earlier play
// began waits for the torrent as long as it would alone: when the earlier
// play's wait runs out, and with it the analysis's, the later play is not
// told to try later, and plays once the swarm turns fast. The engine's
// start of 2 seconds scales each play's longest wait to 64 seconds.
func TestAPlayJoiningASlowTorrentsAnalysisWaitsAsLongAsAlone(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := slowTorrent(t)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	p.warnings.all.Store(true)
	picked := p.details(t)
	// A block of 16 KiB every 5 seconds: too slow for a play.
	seeder.Delay(5 * time.Second)
	type answer struct {
		info playbackInfoResponse
		code string
	}
	play := func() <-chan answer {
		answered := make(chan answer, 1)
		go func() {
			info, code := p.play(t, picked)
			answered <- answer{info, code}
		}()
		return answered
	}
	first := play()
	// The second play asks once the first waited the torrent's start and a
	// piece, 32 seconds, and waits on while its pieces keep coming: it joins
	// the analysis half its wait before the first gives up.
	const waitsOn = "A torrent version is slow to start but its pieces keep coming: the play waits on for it"
	for deadline := time.Now().Add(2 * time.Minute); len(p.warnings.attrsOf(waitsOn)) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the first play did not wait on for the torrent")
		}
	}
	second := play()
	if a := <-first; a.code != "RateLimitExceeded" {
		t.Fatalf("over a swarm too slow for the first play, PlaybackInfo answered %q", a.code)
	}
	// The swarm turns fast as the first play's wait runs out: the second
	// one's goes on for about 30 seconds more.
	seeder.Delay(0)
	if a := <-second; a.code != "" || len(a.info.MediaSources) == 0 {
		t.Fatalf("a play that joined an earlier one's analysis answered %q once the swarm is fast", a.code)
	}
}
