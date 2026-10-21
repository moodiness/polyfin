package torrent

import (
	"errors"
	"net/netip"
	"slices"
	"time"
)

// Peer exchange (BEP 11): the peers a torrent's peers are connected to,
// which they tell in ut_pex messages of the extension protocol. Polyfin
// reads them, and sends none: it accepts no connection, so no peer could
// reach it through another's message, and the peers it knows came from
// trackers, the DHT and other peers' messages, which the peers it would
// tell can ask themselves.
const (
	// extPEX is the ID peers send Polyfin's ut_pex messages with.
	extPEX = 2
	// The flags a message gives each added peer.
	pexEncryption = 0x01
	pexSeed       = 0x02
	pexUTP        = 0x04
	pexHolepunch  = 0x08
	pexReachable  = 0x10
	// maxPEXMessage and maxPEXEntries bound a message: clients send 50
	// added and 50 dropped peers at most, of 18 bytes and a flag each in
	// IPv6. A message past either ends its connection.
	maxPEXMessage = 8 << 10
	maxPEXEntries = 100
	// pexPerMinute is how many messages of a peer are read a minute;
	// clients send one, and Polyfin ignores those past the bound.
	pexPerMinute = 3
	// maxPEXPeers bounds the peers a torrent keeps from its peers'
	// messages.
	maxPEXPeers = 200
)

// errPEX reports a ut_pex message that is malformed or past the bounds.
var errPEX = errors.New("a malformed peer exchange message")

// pexPeer is a peer a ut_pex message adds, with its flags.
type pexPeer struct {
	address netip.AddrPort
	flags   byte
}

// parsePEX reads a ut_pex message: the peers it adds, IPv4 and IPv6,
// with their flags, and those it drops. Addresses no peer listens on
// (port 0, unspecified or multicast) are left out.
func parsePEX(payload []byte) (added []pexPeer, dropped []netip.AddrPort, err error) {
	if len(payload) > maxPEXMessage {
		return nil, nil, errPEX
	}
	d, rest, err := decodeDict(payload)
	if err != nil || len(rest) != 0 {
		return nil, nil, errPEX
	}
	field := func(key string) (string, bool) {
		value, found := d[key]
		if !found {
			return "", true
		}
		s, ok := value.(string)
		return s, ok
	}
	added4, ok4 := field("added")
	flags4, okf4 := field("added.f")
	added6, ok6 := field("added6")
	flags6, okf6 := field("added6.f")
	dropped4, okd4 := field("dropped")
	dropped6, okd6 := field("dropped6")
	if !ok4 || !okf4 || !ok6 || !okf6 || !okd4 || !okd6 ||
		len(added4)%6 != 0 || len(added6)%18 != 0 || len(dropped4)%6 != 0 || len(dropped6)%18 != 0 ||
		len(added4)/6+len(added6)/18 > maxPEXEntries || len(dropped4)/6+len(dropped6)/18 > maxPEXEntries ||
		flags4 != "" && len(flags4) != len(added4)/6 || flags6 != "" && len(flags6) != len(added6)/18 {
		return nil, nil, errPEX
	}
	add := func(list, flags string, size int) {
		for i := 0; i < len(list); i += size {
			address, ok := compactAddress(list[i : i+size])
			if !ok {
				continue
			}
			var f byte
			if flags != "" {
				f = flags[i/size]
			}
			added = append(added, pexPeer{address: address, flags: f})
		}
	}
	add(added4, flags4, 6)
	add(added6, flags6, 18)
	for _, list := range []struct {
		data string
		size int
	}{{dropped4, 6}, {dropped6, 18}} {
		for i := 0; i < len(list.data); i += list.size {
			if address, ok := compactAddress(list.data[i : i+list.size]); ok {
				dropped = append(dropped, address)
			}
		}
	}
	return added, dropped, nil
}

// compactAddress reads an address and its port, of 6 bytes in IPv4 and
// 18 in IPv6; false for one no peer listens on.
func compactAddress(b string) (netip.AddrPort, bool) {
	ip, ok := netip.AddrFromSlice([]byte(b[:len(b)-2]))
	port := uint16(b[len(b)-2])<<8 | uint16(b[len(b)-1])
	if !ok || port == 0 || ip.IsUnspecified() || ip.IsMulticast() {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ip.Unmap(), port), true
}

// receivedPEX reads a peer's ut_pex message, while the torrent is not
// private and the peer sends no more than pexPerMinute a minute. The
// lock is held.
func (t *torrent) receivedPEX(p *peer, payload []byte) error {
	if len(payload) > maxPEXMessage {
		return errPEX
	}
	if t.info != nil && t.info.Private {
		return nil
	}
	now := time.Now()
	if now.Sub(p.pexSince) >= time.Minute {
		p.pexSince, p.pexCount = now, 0
	}
	if p.pexCount >= pexPerMinute {
		return nil
	}
	p.pexCount++
	added, dropped, err := parsePEX(payload)
	if err != nil {
		return err
	}
	t.dropPEX(dropped)
	for _, a := range added {
		if _, kept := t.flags[a.address]; !kept && len(t.flags) >= maxPEXPeers {
			continue
		} else if !kept && t.peers[a.address] == nil && !t.known[a.address] {
			t.pexWaiting = append(t.pexWaiting, a.address)
		}
		t.flags[a.address] = a.flags
	}
	t.queuePEX()
	return nil
}

// dropPEX forgets the peers others no longer reach that wait for the
// torrent's info to be dialed. The lock is held.
func (t *torrent) dropPEX(dropped []netip.AddrPort) {
	if len(dropped) == 0 || len(t.pexWaiting) == 0 {
		return
	}
	t.pexWaiting = slices.DeleteFunc(t.pexWaiting, func(a netip.AddrPort) bool {
		if slices.Contains(dropped, a) {
			delete(t.flags, a)
			return true
		}
		return false
	})
}

// queuePEX queues the peers peer exchange told once the torrent's info
// tells it is not private; those of a private torrent are forgotten, and
// its peers told Polyfin reads no more. The lock is held.
func (t *torrent) queuePEX() {
	if t.info == nil {
		return
	}
	if t.info.Private {
		t.pexWaiting = nil
		clear(t.flags)
		return
	}
	if len(t.pexWaiting) == 0 {
		return
	}
	for _, address := range t.pexWaiting {
		if !t.known[address] && len(t.known) <= 2000 {
			t.known[address] = true
			t.queue = append(t.queue, address)
		}
	}
	t.pexWaiting = nil
	t.signal()
}
