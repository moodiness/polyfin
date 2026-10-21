package torrent

import (
	"bytes"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// playFrom plays a torrent whose only seeder is peer, over TCP through a
// proxy, reads it whole, and returns how long the read took.
func playFrom(t *testing.T, tor *torrenttest.Torrent, peer *torrenttest.Peer) time.Duration {
	t.Helper()
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", peer.Address())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	started := time.Now()
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	return time.Since(started)
}

// A peer that unchokes Polyfin before it said it is interested, as many
// clients do, gets no REQUEST before an INTERESTED: one sent to a peer not
// told is dropped by a strict client.
func TestInterestedBeforeRequests(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(900<<10 + 77)})
	peer := torrenttest.SeedUnchoked(t, tor, "127.0.0.1", false)
	playFrom(t, tor, peer)
	interested, requests := false, 0
	for _, id := range peer.Messages() {
		switch id {
		case msgInterested:
			interested = true
		case msgNotInterested:
			interested = false
		case msgRequest:
			requests++
			if !interested {
				t.Fatalf("a REQUEST came before INTERESTED: %v", peer.Messages())
			}
		}
	}
	if requests == 0 {
		t.Fatal("the peer got no REQUEST")
	}
}

// A strict peer, which drops the REQUESTs received before INTERESTED,
// serves every block: it drops none, so none waits for requestAge to be
// asked again, however long a loaded machine takes to read.
func TestStrictPeerSendsAtOnce(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(900<<10 + 77)})
	peer := torrenttest.SeedUnchoked(t, tor, "127.0.0.1", true)
	took := playFrom(t, tor, peer)
	t.Logf("read from a strict peer in %v", took)
	if dropped := peer.Dropped(); dropped != 0 {
		t.Fatalf("the strict peer dropped %d requests sent before INTERESTED", dropped)
	}
}
