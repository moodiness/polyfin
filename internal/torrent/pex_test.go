package torrent

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// compact writes addresses as a ut_pex message does: IPv4 ones in v4,
// IPv6 ones in v6.
func compact(addresses []netip.AddrPort) (v4, v6 []byte) {
	for _, a := range addresses {
		if a.Addr().Is4() {
			v4 = binary.BigEndian.AppendUint16(append(v4, a.Addr().AsSlice()...), a.Port())
		} else {
			v6 = binary.BigEndian.AppendUint16(append(v6, a.Addr().AsSlice()...), a.Port())
		}
	}
	return v4, v6
}

// pexPayload is a ut_pex message adding peers, each with flags, and
// dropping others.
func pexPayload(added []netip.AddrPort, flags byte, dropped ...netip.AddrPort) []byte {
	a4, a6 := compact(added)
	d4, d6 := compact(dropped)
	return encode(map[string]any{"added": a4, "added.f": bytes.Repeat([]byte{flags}, len(a4)/6),
		"added6": a6, "added6.f": bytes.Repeat([]byte{flags}, len(a6)/18), "dropped": d4, "dropped6": d6})
}

// addresses are n addresses of 10.0.0.0/8 from first, on port 6881.
func addresses(first, n int) []netip.AddrPort {
	list := make([]netip.AddrPort, n)
	for i := range list {
		k := first + i
		list[i] = netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, byte(k >> 16), byte(k >> 8), byte(k)}), 6881)
	}
	return list
}

// torrentOf is the torrent of hash the engine plays.
func torrentOf(e *Engine, hash Hash) *torrent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.torrents[hash]
}

// A peer that only another peer's ut_pex message tells is dialed over
// the route, its flags kept, and sends the pieces the other holds.
func TestPeerExchangeFindsAPeer(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("Exchanged", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(512 << 10)})
	told := torrenttest.Seed(t, tor, "127.0.0.1")
	teller := torrenttest.Seed(t, tor, "127.0.0.1")
	teller.Hold()
	teller.TellPeers(pexUTP|pexSeed, told.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", teller.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	if teller.PEXSent() == 0 || teller.Sent() != 0 || told.Sent() < int64(len(tor.Data)) {
		t.Fatalf("the teller sent %d messages and %d bytes, the peer it told %d bytes", teller.PEXSent(), teller.Sent(), told.Sent())
	}
	if !slices.Contains(proxy.Targets(), "tcp "+told.Address().String()) {
		t.Fatalf("the peer told was not reached through the proxy: %v", proxy.Targets())
	}
	tr := torrentOf(e, tor.Hash)
	tr.mu.Lock()
	flags := tr.flags[told.Address()]
	tr.mu.Unlock()
	if flags != pexUTP|pexSeed {
		t.Fatalf("the peer told has flags %#x, want %#x", flags, pexUTP|pexSeed)
	}
}

// A private torrent's peers come from its trackers alone: the peers a
// ut_pex message tells, received before its info, are never dialed.
func TestPrivateTorrentIgnoresPeerExchange(t *testing.T) {
	t.Parallel()
	tor := torrenttest.BuildPrivate("Private", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(256 << 10)})
	told := torrenttest.Seed(t, tor, "127.0.0.1")
	teller := torrenttest.Seed(t, tor, "127.0.0.1")
	teller.TellPeers(0, told.Address())
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", teller.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	// The teller sent its message before the info dictionary, which the
	// torrent read in order: what it told is settled.
	tr := torrentOf(e, tor.Hash)
	tr.mu.Lock()
	known, flags, waiting := tr.known[told.Address()], len(tr.flags), len(tr.pexWaiting)
	tr.mu.Unlock()
	if teller.PEXSent() == 0 || known || flags != 0 || waiting != 0 || len(told.Remotes()) != 0 {
		t.Fatalf("%d messages sent; the peer told known %v, dialed %d times; %d flags and %d peers kept",
			teller.PEXSent(), known, len(told.Remotes()), flags, waiting)
	}
}

// The bounds on ut_pex messages: a malformed or oversized one ends its
// connection, the messages read a minute, the peers kept a torrent, and
// what the torrent's info allows.
func TestPeerExchangeLimits(t *testing.T) {
	t.Parallel()
	s := newFuzzSwarm(t, false)
	e := fuzzEngine(t, nil, s)
	pex := func(payload []byte) []byte { return wire(msgExtended, []byte{extPEX}, payload) }
	read := func(t *testing.T, tr *torrent, stream []byte) (*peer, error) {
		t.Helper()
		address := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.2"), uint16(7000+len(tr.peers)))
		p := newPeer(tr, &streamConn{data: stream}, address)
		tr.mu.Lock()
		tr.peers[address] = p
		tr.mu.Unlock()
		return p, p.read()
	}
	newTorrentOf := func(t *testing.T, mode byte) *torrent {
		tr, _, err := newFuzzPeer(e, s, mode, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tr.cancel)
		clear(tr.peers)
		return tr
	}

	t.Run("malformed or oversized", func(t *testing.T) {
		v4, _ := compact(addresses(0, 2))
		for name, payload := range map[string][]byte{
			"oversized":               append(pexPayload(addresses(0, 1), 0), make([]byte, maxPEXMessage)...),
			"too many added":          pexPayload(addresses(0, maxPEXEntries+1), 0),
			"too many dropped":        pexPayload(nil, 0, addresses(0, maxPEXEntries+1)...),
			"not a dictionary":        []byte("le"),
			"truncated":               []byte("d5:added6:\x7f\x00"),
			"trailing bytes":          append(pexPayload(addresses(0, 1), 0), 'x'),
			"partial address":         encode(map[string]any{"added": v4[:10]}),
			"partial IPv6":            encode(map[string]any{"added6": make([]byte, 20)}),
			"flags of other peers":    encode(map[string]any{"added": v4, "added.f": "\x04"}),
			"added of another type":   encode(map[string]any{"added": 6}),
			"dropped of another type": encode(map[string]any{"dropped": []any{"x"}}),
		} {
			tr := newTorrentOf(t, 1)
			other, _ := read(t, tr, nil)
			_, err := read(t, tr, pex(payload))
			tr.mu.Lock()
			queued, failed := len(tr.queue), tr.err
			_, stays := tr.peers[other.address]
			tr.mu.Unlock()
			if !errors.Is(err, errPEX) && !errors.Is(err, errBencode) {
				t.Errorf("%s: got %v", name, err)
			}
			if queued != 0 || failed != nil || !stays {
				t.Errorf("%s: %d peers queued, the torrent failed with %v, the other peer stays %v", name, queued, failed, stays)
			}
		}
	})
	t.Run("messages read a minute", func(t *testing.T) {
		tr := newTorrentOf(t, 1)
		var stream []byte
		for i := range 10 {
			stream = append(stream, pex(pexPayload(addresses(i, 1), 0))...)
		}
		if _, err := read(t, tr, stream); !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if len(tr.queue) != pexPerMinute {
			t.Fatalf("%d peers queued, want %d", len(tr.queue), pexPerMinute)
		}
	})
	t.Run("peers kept a torrent", func(t *testing.T) {
		tr := newTorrentOf(t, 1)
		for k := range 3 {
			var stream []byte
			for i := range pexPerMinute {
				stream = append(stream, pex(pexPayload(addresses((k*pexPerMinute+i)*maxPEXEntries, maxPEXEntries), 0))...)
			}
			if _, err := read(t, tr, stream); !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
		}
		if len(tr.flags) != maxPEXPeers || len(tr.queue) != maxPEXPeers {
			t.Fatalf("%d peers kept, %d queued, want %d", len(tr.flags), len(tr.queue), maxPEXPeers)
		}
	})
	t.Run("flags and addresses", func(t *testing.T) {
		tr := newTorrentOf(t, 1)
		v6 := netip.MustParseAddrPort("[2001:db8::7]:6881")
		mapped := netip.AddrPortFrom(netip.AddrFrom16(netip.MustParseAddr("10.9.9.9").As16()), 6881)
		payload := encode(map[string]any{
			"added":    string(netip.MustParseAddr("10.1.1.1").AsSlice()) + "\x1a\xe1" + string(netip.MustParseAddr("10.1.1.2").AsSlice()) + "\x00\x00",
			"added.f":  "\x04\x04",
			"added6":   string(v6.Addr().AsSlice()) + "\x1a\xe1" + string(mapped.Addr().AsSlice()) + "\x1a\xe1" + string(make([]byte, 16)) + "\x1a\xe1",
			"added6.f": "\x02\x00\x00",
		})
		if _, err := read(t, tr, pex(payload)); !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		want := map[netip.AddrPort]byte{netip.MustParseAddrPort("10.1.1.1:6881"): pexUTP, v6: pexSeed, netip.MustParseAddrPort("10.9.9.9:6881"): 0}
		if !maps.Equal(tr.flags, want) || len(tr.queue) != 3 {
			t.Fatalf("kept %v, queued %v", tr.flags, tr.queue)
		}
	})
	t.Run("peers told before the info", func(t *testing.T) {
		tr := newTorrentOf(t, 0)
		told := addresses(0, 3)
		stream := slices.Concat(pex(pexPayload(told, 0)), pex(pexPayload(nil, 0, told[0])))
		if _, err := read(t, tr, stream); !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if len(tr.queue) != 0 || !slices.Equal(tr.pexWaiting, told[1:]) {
			t.Fatalf("queued %v, waiting %v", tr.queue, tr.pexWaiting)
		}
		info := s.info
		tr.info = &info
		tr.queuePEX()
		if !slices.Equal(tr.queue, told[1:]) || len(tr.pexWaiting) != 0 {
			t.Fatalf("once the info is known, queued %v, waiting %v", tr.queue, tr.pexWaiting)
		}

		private := newTorrentOf(t, 0)
		if _, err := read(t, private, pex(pexPayload(told, 0))); !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		info.Private = true
		private.info = &info
		private.queuePEX()
		if len(private.queue) != 0 || len(private.pexWaiting) != 0 || len(private.flags) != 0 {
			t.Fatalf("a private torrent queued %v, waiting %v", private.queue, private.pexWaiting)
		}
	})
	t.Run("a private torrent", func(t *testing.T) {
		tr := newTorrentOf(t, 1)
		info := *tr.info
		info.Private = true
		tr.info = &info
		p, err := read(t, tr, pex(pexPayload(addresses(0, 3), 0)))
		if !errors.Is(err, io.EOF) || len(tr.queue) != 0 || len(tr.flags) != 0 {
			t.Fatalf("got %v, queued %v", err, tr.queue)
		}
		// Its handshake offers no peer exchange; a public one's does.
		tr.mu.Lock()
		p.sendExtendedHandshake(0)
		info.Private = false
		p.sendExtendedHandshake(0)
		tr.mu.Unlock()
		if len(p.outbox) != 2 || strings.Contains(string(p.outbox[0]), "ut_pex") || !strings.Contains(string(p.outbox[1]), "6:ut_pexi2e") {
			t.Fatalf("handshakes %q", p.outbox)
		}
	})
}

func FuzzPEX(f *testing.F) {
	f.Add(pexPayload(addresses(0, 3), pexUTP, addresses(10, 2)...))
	f.Add(pexPayload([]netip.AddrPort{netip.MustParseAddrPort("[2001:db8::1]:6881"), netip.MustParseAddrPort("10.0.0.1:1")}, pexSeed|pexReachable))
	f.Add(pexPayload(addresses(0, maxPEXEntries), 0))
	f.Add([]byte("de"))
	s := newFuzzSwarm(f, false)
	f.Fuzz(func(t *testing.T, payload []byte) {
		added, dropped, err := parsePEX(payload)
		if err == nil {
			if len(payload) > maxPEXMessage || len(added) > maxPEXEntries || len(dropped) > maxPEXEntries {
				t.Fatalf("a message of %d bytes read: %d added, %d dropped", len(payload), len(added), len(dropped))
			}
			for _, a := range slices.Concat(dropped, func() []netip.AddrPort {
				var list []netip.AddrPort
				for _, p := range added {
					list = append(list, p.address)
				}
				return list
			}()) {
				if !a.IsValid() || a.Port() == 0 || a.Addr().IsUnspecified() || a.Addr() != a.Addr().Unmap() {
					t.Fatalf("kept %v", a)
				}
			}
		}
		// Many peers send it, before and after the info: what the
		// torrent keeps stays within its bounds.
		tr := newTorrent(nil, Spec{Hash: s.tor.Hash}, nil, false)
		defer tr.cancel()
		for k := range 2 * maxPEXPeers / maxPEXEntries {
			p := newPeer(tr, &streamConn{}, netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(k+1)))
			for range pexPerMinute + 1 {
				if tr.receivedPEX(p, payload) != nil {
					break
				}
			}
			if k == 1 {
				info := s.info
				tr.info = &info
				tr.queuePEX()
			}
		}
		if len(tr.flags) > maxPEXPeers || len(tr.queue) > maxPEXPeers || len(tr.pexWaiting) != 0 {
			t.Fatalf("%d peers kept, %d queued, %d waiting", len(tr.flags), len(tr.queue), len(tr.pexWaiting))
		}
	})
}

// BenchmarkPeerExchange measures, on the local swarm, the peers found and
// the time to the first byte and to the end of a range of 1 MiB, when the
// one peer a tracker lists sends a block every 50 ms and tells four
// others in a ut_pex message, or tells none:
//
//	go test -run '^$' -bench PeerExchange -benchtime 5x ./internal/torrent
func BenchmarkPeerExchange(b *testing.B) {
	for _, exchange := range []bool{true, false} {
		name := "exchange"
		if !exchange {
			name = "without"
		}
		b.Run(name, func(b *testing.B) {
			var found int
			var first, end time.Duration
			for range b.N {
				n, f, r := measurePeerExchange(b, exchange)
				found += n
				first += f
				end += r
			}
			b.ReportMetric(float64(found)/float64(b.N), "peers-found")
			b.ReportMetric(float64(first.Milliseconds())/float64(b.N), "ms-first-byte")
			b.ReportMetric(float64(end.Milliseconds())/float64(b.N), "ms-range-end")
			b.ReportMetric(0, "ns/op")
		})
	}
}

func measurePeerExchange(b *testing.B, exchange bool) (found int, first, end time.Duration) {
	tor := torrenttest.Build("Exchanged", 256<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(4 << 20)})
	teller := torrenttest.Seed(b, tor, "127.0.0.1")
	teller.Delay(50 * time.Millisecond)
	var others []netip.AddrPort
	for range 4 {
		others = append(others, torrenttest.Seed(b, tor, "127.0.0.1").Address())
	}
	if exchange {
		teller.TellPeers(pexSeed, others...)
	}
	e := newEngine(b, proxied(torrenttest.StartProxy(b, true, "", "")), 64<<20)
	spec, _ := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(b, "127.0.0.1", teller.Address()).URL()})
	defer e.Play(spec)()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadAt(make([]byte, 1), 0); err != nil {
		b.Fatal(err)
	}
	first = time.Since(started)
	buf := make([]byte, 1<<20)
	if _, err := io.ReadFull(io.NewSectionReader(r, 0, int64(len(buf))), buf); err != nil {
		b.Fatal(err)
	}
	end = time.Since(started)
	if !bytes.Equal(buf, tor.Data[:len(buf)]) {
		b.Fatal("the range read is not the torrent's")
	}
	tr := torrentOf(e, tor.Hash)
	tr.mu.Lock()
	found = len(tr.known)
	tr.mu.Unlock()
	return found, first, end
}
