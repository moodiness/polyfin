package utp

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"testing"
)

// The fuzz tests feed the uTP parser and a connection what peers send. None
// may panic, and what a connection keeps stays within its windows. The
// inputs that made one fail are in testdata/fuzz, run as regular tests.

func packetOf(h header, payload []byte) []byte {
	return append(appendHeader(nil, h), payload...)
}

func FuzzHeader(f *testing.F) {
	f.Add(packetOf(header{typ: stSyn, connID: 7, seq: 1}, nil))
	f.Add(packetOf(header{typ: stState, connID: 7, seq: 9, ack: 3, sack: []byte{0x05, 0, 0, 0x80}}, nil))
	f.Add(packetOf(header{typ: stData, connID: 8, seq: 2, ack: 9, wnd: 1 << 20}, []byte("payload")))
	// An unknown extension, then a selective acknowledgment.
	f.Add(append([]byte{stState<<4 | version, 2}, append(make([]byte, 18), 1, 2, 0xaa, 0xbb, 0, 4, 1, 0, 0, 0)...))
	f.Add([]byte{stData<<4 | version, 1, 0, 0})
	f.Fuzz(func(t *testing.T, packet []byte) {
		h, payload, err := parse(packet)
		if err != nil {
			return
		}
		if len(h.sack) > maxSACK || len(h.sack)%4 != 0 || len(payload) > len(packet)-headerSize {
			t.Fatalf("parse kept a SACK of %d bytes, a payload of %d of %d", len(h.sack), len(payload), len(packet))
		}
		// What is read is written back the same, the unknown extensions
		// aside.
		again, payloadAgain, err := parse(packetOf(h, payload))
		if err != nil || again.typ != h.typ || again.connID != h.connID || again.seq != h.seq || again.ack != h.ack ||
			again.timestamp != h.timestamp || again.timestampDiff != h.timestampDiff || again.wnd != h.wnd ||
			!bytes.Equal(again.sack, h.sack) || !bytes.Equal(payloadAgain, payload) {
			t.Fatalf("a packet written back reads otherwise: %+v, %+v: %v", h, again, err)
		}
	})
}

// sink is a PacketConn that keeps nothing: the connection under fuzz
// sends into it, and reads nothing from it.
type sink struct{ closed chan struct{} }

func (s sink) WriteToAddrPort([]byte, netip.AddrPort) error { return nil }

func (s sink) ReadFrom([]byte) (int, netip.AddrPort, error) {
	<-s.closed
	return 0, netip.AddrPort{}, ErrClosed
}

func (s sink) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

// FuzzConn feeds an accepted connection, with bytes of its own in flight,
// the packets of data: each after its length on two bytes, its header's
// sequence and acknowledgment numbers taken relative to the connection's,
// so that the fuzzer reaches past the first packet.
func FuzzConn(f *testing.F) {
	frame := func(packets ...[]byte) []byte {
		var b []byte
		for _, p := range packets {
			b = binary.BigEndian.AppendUint16(b, uint16(len(p)))
			b = append(b, p...)
		}
		return b
	}
	f.Add(frame(
		packetOf(header{typ: stData, seq: 2, ack: 1}, []byte("second")),
		packetOf(header{typ: stData, seq: 1, ack: 2}, []byte("first")),
		packetOf(header{typ: stFin, seq: 3, ack: 3}, nil),
	))
	f.Add(frame(
		packetOf(header{typ: stState, ack: 0, sack: []byte{0xff, 0xff, 0, 0}}, nil),
		packetOf(header{typ: stState, ack: 0}, nil),
		packetOf(header{typ: stState, ack: 0}, nil),
		packetOf(header{typ: stState, ack: 0}, nil),
	))
	f.Add(frame(packetOf(header{typ: stData, seq: 1000, ack: 60000, wnd: 0}, []byte("far")), packetOf(header{typ: stReset}, nil)))
	f.Fuzz(func(t *testing.T, data []byte) {
		s := newSocket(sink{closed: make(chan struct{})}, false)
		defer s.Close()
		remote := netip.MustParseAddrPort("192.0.2.1:6881")
		c := newConn(s, remote, 11, 10)
		s.mu.Lock()
		s.conns[connKey{remote, 11}] = c
		s.mu.Unlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.accepted(header{typ: stSyn, connID: 10, seq: 500, wnd: 1 << 20})
		// Bytes of its own in flight, and more waiting for the window.
		c.sendData = append(c.sendData, make([]byte, 40<<10)...)
		c.flush()
		seq, ack := c.seq, c.ack
		for len(data) >= 2 {
			n := int(binary.BigEndian.Uint16(data))
			data = data[2:]
			if n > len(data) {
				n = len(data)
			}
			h, payload, err := parse(data[:n])
			data = data[n:]
			if err != nil {
				continue
			}
			h.seq += ack
			h.ack += seq - 1 - uint16(len(c.inflight))
			c.handle(h, payload)
			if c.buffered() > 2*recvWindow || c.reorderBytes > recvWindow || len(c.reorder) > maxReorder || c.flightBytes < 0 || len(c.inflight) > maxInflight ||
				c.cwnd < minCwnd || c.cwnd > maxCwnd || c.recvOff > len(c.recv) {
				t.Fatalf("the connection keeps %d bytes, %d packets out of order, %d bytes in flight, a window of %d",
					c.buffered(), len(c.reorder), c.flightBytes, c.cwnd)
			}
			flight := 0
			for _, p := range c.inflight {
				if p.counted {
					flight += len(p.payload)
				}
			}
			if flight != c.flightBytes {
				t.Fatalf("%d bytes counted in flight, %d are", c.flightBytes, flight)
			}
		}
	})
}
