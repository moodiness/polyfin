package torrent

import (
	"bytes"
	"context"
	"net/netip"
	"testing"
	"time"
)

// One answer of a tracker adds 50 peers at most, as the announce asks,
// however many it lists and in whichever form, IPv4 and IPv6 in turn; so
// does one answer of a DHT node, and one lookup adds 200.
func TestOneAnswerAddsFiftyPeersAtMost(t *testing.T) {
	t.Parallel()
	const most = 50
	compact := func(n int, a, b byte) []byte {
		list := make([]byte, 0, 6*n)
		for i := range n {
			list = append(list, 127, a, b+byte(i>>8), byte(i), 0x1a, 0xe1)
		}
		return list
	}
	compact6 := func(n int) []byte {
		list := make([]byte, 0, 18*n)
		for i := range n {
			list = append(append(list, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i>>8), byte(i)), 0x1a, 0xe1)
		}
		return list
	}
	t.Run("an HTTP tracker's compact list", func(t *testing.T) {
		t.Parallel()
		answer, err := parseHTTPAnswer(encode(map[string]any{"interval": 1800, "peers": compact(2001, 1, 0)}))
		if err != nil || len(answer.peers) != most {
			t.Fatalf("%d peers, %v", len(answer.peers), err)
		}
		if first := netip.MustParseAddrPort("127.1.0.0:6881"); answer.peers[0] != first {
			t.Fatalf("the first peer is %v, want %v", answer.peers[0], first)
		}
	})
	t.Run("an HTTP tracker's list of dictionaries", func(t *testing.T) {
		t.Parallel()
		var list []any
		for i := range 300 {
			list = append(list, map[string]any{"ip": netip.AddrFrom4([4]byte{127, 1, byte(i >> 8), byte(i)}).String(), "port": 6881})
		}
		answer, err := parseHTTPAnswer(encode(map[string]any{"interval": 1800, "peers": list}))
		if err != nil || len(answer.peers) != most {
			t.Fatalf("%d peers, %v", len(answer.peers), err)
		}
	})
	t.Run("an HTTP tracker's peers in IPv4 and IPv6", func(t *testing.T) {
		t.Parallel()
		answer, err := parseHTTPAnswer(encode(map[string]any{"interval": 1800, "peers": compact(60, 1, 0), "peers6": compact6(60)}))
		if err != nil || len(answer.peers) != most {
			t.Fatalf("%d peers, %v", len(answer.peers), err)
		}
		v4 := 0
		for _, peer := range answer.peers {
			if peer.Addr().Is4() {
				v4++
			}
		}
		if v4 != most/2 {
			t.Fatalf("%d of the %d peers are in IPv4, want half", v4, most)
		}
		few, err := parseHTTPAnswer(encode(map[string]any{"interval": 1800, "peers": compact(10, 1, 0), "peers6": compact6(60)}))
		if err != nil || len(few.peers) != most {
			t.Fatalf("%d peers of 10 in IPv4 and 60 in IPv6, %v", len(few.peers), err)
		}
	})
	t.Run("a UDP tracker", func(t *testing.T) {
		t.Parallel()
		connect := append(u32(udpActionConnect, 0), 1, 2, 3, 4, 5, 6, 7, 8)
		reply := append(u32(udpActionAnnounce, 0, 60, 0, 1), compact(300, 1, 0)...)
		conn := &datagramConn{datagrams: [][]byte{connect, reply}, echo: true}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		answer, err := announceUDP(ctx, packetRoute{conn}, "tracker.invalid:6969", announce{port: 51413, left: 1})
		if err != nil || len(answer.peers) != most {
			t.Fatalf("%d peers, %v", len(answer.peers), err)
		}
	})
	t.Run("the DHT", func(t *testing.T) {
		t.Parallel()
		values := func(n int, a, b byte) []any {
			list := compact(n, a, b)
			peers := make([]any, n)
			for i := range peers {
				peers[i] = string(list[6*i : 6*i+6])
			}
			return peers
		}
		answer := func(txid int, values []any, nodes string) []byte {
			return encode(map[string]any{"t": string([]byte{byte(txid >> 8), byte(txid)}), "y": "r",
				"r": map[string]any{"id": string(bytes.Repeat([]byte{byte(txid)}, 20)), "values": values, "nodes": nodes}})
		}
		lookup := func(t *testing.T, datagrams ...[]byte) int {
			t.Helper()
			conn := &datagramConn{datagrams: datagrams, from: netip.MustParseAddrPort("127.0.0.1:6881")}
			s := newFuzzSwarm(t, true)
			tr := newTorrent(fuzzEngine(t, nil), Spec{Hash: s.tor.Hash, File: -1}, packetRoute{conn}, true)
			t.Cleanup(tr.cancel)
			if !tr.lookupOnce([]string{"127.0.0.1:6881"}) {
				t.Fatal("the lookup found no peer")
			}
			tr.mu.Lock()
			defer tr.mu.Unlock()
			return len(tr.known)
		}
		// One node's answer of 400 peers, as many as a datagram read holds.
		if n := lookup(t, answer(1, values(400, 2, 0), "")); n != most {
			t.Fatalf("%d peers known after one answer", n)
		}
		// The first node names 16 others, each of which answers 60 peers
		// of its own.
		var nodes string
		for i := range dhtKeep {
			nodes += string(bytes.Repeat([]byte{byte(i + 1)}, 20)) + string([]byte{127, 0, 9, byte(i + 1), 0x1a, 0xe1})
		}
		datagrams := [][]byte{answer(1, nil, nodes)}
		for i := range dhtKeep {
			datagrams = append(datagrams, answer(i+2, values(60, 3, byte(4*i)), ""))
		}
		if n := lookup(t, datagrams...); n != 200 {
			t.Fatalf("%d peers known after one lookup of %d answers, want 200", n, dhtKeep)
		}
	})
}
