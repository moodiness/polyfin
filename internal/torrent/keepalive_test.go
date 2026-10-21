package torrent

import (
	"bytes"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

// A keep-alive, four bytes of zero length, is written as it is: a peer that
// sends nothing for a minute keeps its connection.
func TestKeepAliveIsSent(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &testSettings{}, 64<<20)
	tor := newTorrent(e, Spec{}, nil, false)
	defer tor.cancel()
	here, there := net.Pipe()
	defer there.Close()
	p := newPeer(tor, here, netip.MustParseAddrPort("192.0.2.1:6881"))
	defer p.close()
	p.outMu.Lock()
	p.outbox = append(p.outbox, []byte{0, 0, 0, 0})
	p.outMu.Unlock()
	p.wake <- struct{}{}
	go p.write()
	_ = there.SetReadDeadline(time.Now().Add(time.Minute))
	got := make([]byte, 4)
	if _, err := io.ReadFull(there, got); err != nil || !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("read %v: %v, not a keep-alive", got, err)
	}
}
