// Package utp speaks uTP, the Micro Transport Protocol of BitTorrent (BEP
// 29): reliable, ordered streams over UDP, whose LEDBAT congestion control
// yields to other traffic as the delay it adds grows. A Socket carries the
// streams of one UDP socket, told apart by their connection IDs; it runs
// over any PacketConn, such as the UDP socket of a route.
package utp

import (
	"encoding/binary"
	"errors"
)

// The packet types, and the protocol's version.
const (
	stData  = 0
	stFin   = 1
	stState = 2
	stReset = 3
	stSyn   = 4

	version = 1
	// headerSize is the size of a packet's header, before its extensions.
	headerSize = 20
	// extSACK is the selective acknowledgment extension.
	extSACK = 1
	// maxExtensions bounds the extensions a packet is read with, and
	// maxSACK the bytes of a selective acknowledgment kept: 256 packets.
	maxExtensions = 8
	maxSACK       = 32
)

var errMalformed = errors.New("a malformed uTP packet")

// header is a packet's header: its type, the connection ID it is sent
// with, the sender's clock and the delay it measured (in microseconds),
// the sender's receive window in bytes, the packet's sequence number, the
// last one the sender received in order, and the packets received past
// that, a bit each from ack+2, when sack is set.
type header struct {
	typ           byte
	connID        uint16
	timestamp     uint32
	timestampDiff uint32
	wnd           uint32
	seq, ack      uint16
	sack          []byte
}

// parse reads a packet's header, its extensions and its payload, which
// alias packet. Extensions are walked within the packet, at most
// maxExtensions of them; an unknown one is skipped, and a selective
// acknowledgment of a length BEP 29 does not allow (not a multiple of 4)
// or past maxSACK is not kept.
func parse(packet []byte) (header, []byte, error) {
	if len(packet) < headerSize || packet[0]&0x0f != version || packet[0]>>4 > stSyn {
		return header{}, nil, errMalformed
	}
	h := header{
		typ:           packet[0] >> 4,
		connID:        binary.BigEndian.Uint16(packet[2:]),
		timestamp:     binary.BigEndian.Uint32(packet[4:]),
		timestampDiff: binary.BigEndian.Uint32(packet[8:]),
		wnd:           binary.BigEndian.Uint32(packet[12:]),
		seq:           binary.BigEndian.Uint16(packet[16:]),
		ack:           binary.BigEndian.Uint16(packet[18:]),
	}
	next, rest := packet[1], packet[headerSize:]
	for count := 0; next != 0; count++ {
		if count == maxExtensions || len(rest) < 2 || int(rest[1]) > len(rest)-2 {
			return header{}, nil, errMalformed
		}
		kind, length := next, int(rest[1])
		next = rest[0]
		if kind == extSACK && length >= 4 && length%4 == 0 && length <= maxSACK && h.sack == nil {
			h.sack = rest[2 : 2+length]
		}
		rest = rest[2+length:]
	}
	return h, rest, nil
}

// appendHeader appends h to b, with its selective acknowledgment.
func appendHeader(b []byte, h header) []byte {
	extension := byte(0)
	if len(h.sack) > 0 {
		extension = extSACK
	}
	b = append(b, h.typ<<4|version, extension)
	b = binary.BigEndian.AppendUint16(b, h.connID)
	b = binary.BigEndian.AppendUint32(b, h.timestamp)
	b = binary.BigEndian.AppendUint32(b, h.timestampDiff)
	b = binary.BigEndian.AppendUint32(b, h.wnd)
	b = binary.BigEndian.AppendUint16(b, h.seq)
	b = binary.BigEndian.AppendUint16(b, h.ack)
	if len(h.sack) > 0 {
		b = append(b, 0, byte(len(h.sack)))
		b = append(b, h.sack...)
	}
	return b
}

// seqLess reports whether sequence number a comes before b, the numbers
// wrapping at 65536.
func seqLess(a, b uint16) bool { return int16(a-b) < 0 }
