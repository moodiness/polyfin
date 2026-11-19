package jellyfin

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A torrent whose info dictionary is refused, its name being longer than
// a file's may be, fails its play with why as soon as the dictionary
// came, rather than as a torrent no peer sent in time: the version keeps
// that reason as its failure.
func TestATorrentWhoseInfoIsRefusedFailsItsPlayWithWhy(t *testing.T) {
	t.Parallel()
	refused := torrenttest.Build(strings.Repeat("A", 766), 64<<10,
		torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(300 << 10)})
	seeder := torrenttest.Seed(t, refused, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	// The dictionary comes within moments; a play that waits for the
	// start's whole time took the torrent for one that did not start.
	p := newTorrentPlay(t, "ffprobe-not-installed", 15*time.Second, []*torrenttest.Torrent{refused}, []string{tracker.URL()})
	if info, code := p.play(t, p.details(t)); code == "" || len(info.MediaSources) != 0 {
		t.Fatalf("the play answered %q, %d sources", code, len(info.MediaSources))
	}
	const why = "the torrent's info dictionary is refused: its name is longer than 765 bytes"
	failure := p.handler.Playback.Failure(p.versions[0].ID)
	if !errors.Is(failure, torrent.ErrInfoRefused) || !strings.Contains(failure.Error(), why) {
		t.Fatalf("the version's failure: %v, want %q", failure, why)
	}
	if p.handler.Playback.Unstarted(p.versions[0].ID) {
		t.Error("the version is kept as a torrent that may start later")
	}
}

// The start of a play whose torrent's info dictionary was refused is told
// as failed, for that reason.
func TestAStartWhoseTorrentInfoWasRefusedIsToldSo(t *testing.T) {
	t.Parallel()
	h := newTestServer(t, 10).handler
	s := &playStart{failed: map[accounts.ID]string{}}
	version := library.Version{ID: accounts.ID{1}, Torrent: &library.Torrent{InfoHash: "0123456789abcdef0123456789abcdef01234567"}}
	s.trying(version)
	s.tried(version, fmt.Errorf("analyze: %w", fmt.Errorf("%w: it lists more than 50000 files", torrent.ErrInfoRefused)))
	if got := h.progress(s); got != (startProgressJSON{Stage: "failed", Failure: "info_refused"}) {
		t.Errorf("progress: %+v", got)
	}
}
