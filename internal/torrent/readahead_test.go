package torrent

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A playback's read fetches ahead within its file alone: up to the piece
// holding the file's last byte, which it shares with the next file, and
// no piece of the files after it, the next episode of a season's pack for
// one. A file shorter than a piece fetches the pieces holding it, not
// those beside them as the start and end of its index.
func TestReadAheadStopsAtTheFilesEnd(t *testing.T) {
	t.Parallel()
	const piece = 32 << 10
	episodes := [][]byte{torrenttest.Bytes(10*piece + 100), torrenttest.Bytes(3000), torrenttest.Bytes(10 * piece)}
	pack := torrenttest.Build("Show.S01", piece, torrenttest.File{Name: "Show.S01E01.mkv", Data: episodes[0]},
		torrenttest.File{Name: "Show.S01E02.mkv", Data: episodes[1]}, torrenttest.File{Name: "Show.S01E03.mkv", Data: episodes[2]})
	// The first episode's last byte lies in piece 10, with the second
	// episode, whose 3000 bytes lie in it too; the third begins in it and
	// has pieces 11 to 20 to itself.
	own := 11 * piece
	for _, c := range []struct {
		name   string
		file   int
		before int
	}{
		{"the first episode", 0, 0},
		{"the second, shorter than a piece", 1, 10 * piece},
	} {
		seeder := torrenttest.Seed(t, pack, "127.0.0.1")
		tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
		e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
		index := c.file
		spec, err := NewSpec(pack.HashHex(), &index, []string{"tracker:" + tracker.URL()})
		if err != nil {
			t.Fatal(err)
		}
		stop := e.Play(spec)
		if got := readAll(t, e, spec); !bytes.Equal(got, episodes[c.file]) {
			t.Fatalf("%s: read %d bytes, not the file's", c.name, len(got))
		}
		stop()
		if seeder.Asked(own, len(pack.Data)-own) {
			t.Errorf("%s: blocks of the third episode's own pieces were asked", c.name)
		}
		if c.before > 0 && seeder.Asked(0, c.before) {
			t.Errorf("%s: blocks of the first episode's own pieces were asked", c.name)
		}
		t.Logf("%s: %d bytes downloaded", c.name, seeder.Sent())
	}
}

// A reader paced by playback, slower than its swarm, never waits for a
// piece: the read-ahead rolls with it, the next pieces asked once a read
// moves past the back half of the window rather than once a read misses
// one. The reader reads each piece only once the swarm has nothing left
// in flight, as a player at its bitrate does behind a faster swarm.
// Before, the swarm then stayed idle until the read at the window's step
// (16 MiB) missed its piece, and that read waited for it. The read-ahead
// stays within the window past the read.
func TestReadAheadRollsWithAPacedReader(t *testing.T) {
	t.Parallel()
	const piece = 1 << 20
	data := torrenttest.Bytes(48 * piece)
	movie := torrenttest.Build("Paced Movie", piece, torrenttest.File{Name: "Paced.Movie.mkv", Data: data})
	seeder := torrenttest.Seed(t, movie, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 256<<20)
	spec, err := NewSpec(movie.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.t.mu.Lock()
	window := r.t.window()
	r.t.mu.Unlock()
	if window != readWindow/piece {
		t.Fatalf("the window is %d pieces, want %d", window, readWindow/piece)
	}
	buf := make([]byte, piece)
	for i := range len(data) / piece {
		// The first read starts the torrent's reads, and waits.
		if i > 0 && !idleWith(r.t, i, 10*time.Second) {
			t.Fatalf("the read at %d MiB waited for its piece: nothing was in flight, and it was not asked", i)
		}
		// Within the window past the last read, the edge pieces of the
		// file's end, for its index, aside.
		if beyond := (i - 1 + window) * piece; i > 0 && beyond < len(data)-edgePieces*piece &&
			seeder.Asked(beyond, len(data)-edgePieces*piece-beyond) {
			t.Fatalf("with the read at %d MiB, pieces past the window (%d MiB on) were asked", i-1, beyond/piece)
		}
		if _, err := r.ReadAt(buf, int64(i*piece)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf, data[i*piece:(i+1)*piece]) {
			t.Fatalf("the read at %d MiB is not the file's", i)
		}
	}
}

// idleWith waits, up to wait, for the torrent to hold piece index with no
// piece in flight; false if it does not.
func idleWith(t *torrent, index int, wait time.Duration) bool {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		t.mu.Lock()
		idle, changed := t.have[index] && len(t.inflight) == 0, t.changed
		t.mu.Unlock()
		if idle {
			return true
		}
		select {
		case <-changed:
		case <-deadline.C:
			return false
		}
	}
}
