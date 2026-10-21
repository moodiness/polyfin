package torrent

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A private torrent is not looked up on the DHT once its info is known
// (BEP 27): the lookup running when the info arrives stops, and asks no
// node the answer it was waiting for tells.
func TestPrivateTorrentStopsItsDHTLookup(t *testing.T) {
	t.Parallel()
	tor := torrenttest.BuildPrivate("Private", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(256 << 10)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	node := torrenttest.StartDHTNode(t)
	next := torrenttest.StartDHTNode(t)
	node.Hold()
	node.TellNodes(netip.MustParseAddrPort(next.Address()))
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"dht:" + node.Address()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	// Before the info, the magnet is looked up: the node holds its answer.
	waitFor(t, "the lookup to ask the node", func() bool { return node.Queries() > 0 })

	more, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(more)()
	if got := readAll(t, e, more); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	// The info is known: the node's answer, telling the next node, comes
	// to a lookup stopped.
	node.Release()
	tr := torrentOf(e, tor.Hash)
	waitFor(t, "the lookup to stop", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return tr.stopLookup == nil
	})
	tr.mu.Lock()
	uses := tr.usesDHT()
	tr.mu.Unlock()
	if uses || node.Queries() != 1 || next.Queries() != 0 {
		t.Fatalf("looked up on the DHT %v; %d queries to the node, %d to the next", uses, node.Queries(), next.Queries())
	}
}

// A torrent whose info is known to be private from the start is never
// looked up on the DHT, whatever its sources ask: its lookup ends before
// it opens a socket.
func TestPrivateTorrentIsNotLookedUp(t *testing.T) {
	t.Parallel()
	tor := torrenttest.BuildPrivate("Private", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(128 << 10)})
	info, err := parseInfo(tor.Info)
	if err != nil || !info.Private {
		t.Fatalf("got %v, private %v", err, info.Private)
	}
	node := torrenttest.StartDHTNode(t)
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	route, err := e.currentRoute()
	if err != nil {
		t.Fatal(err)
	}
	tr := newTorrent(e, Spec{Hash: tor.Hash, File: -1, Nodes: []string{node.Address()}, DHT: true}, route, true)
	defer tr.cancel()
	tr.info, tr.infoData, tr.have = &info, tor.Info, make([]bool, len(info.Pieces))
	// lookup returns: a public torrent's runs until the torrent stops.
	tr.lookup()
	for _, target := range proxy.Targets() {
		if strings.HasPrefix(target, "udp ") {
			t.Fatalf("the proxy relayed UDP: %v", proxy.Targets())
		}
	}
	if tr.looked || node.Queries() != 0 {
		t.Fatalf("looked up %v, %d queries to the node", tr.looked, node.Queries())
	}
}
