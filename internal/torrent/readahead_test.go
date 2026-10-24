package torrent

import (
	"bytes"
	"testing"

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
