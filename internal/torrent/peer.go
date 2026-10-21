package torrent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

// The peer wire protocol (BEP 3), its extension protocol (BEP 10) and the
// metadata exchange (BEP 9); peer exchange (BEP 11) is in pex.go.
const (
	msgChoke         = 0
	msgUnchoke       = 1
	msgInterested    = 2
	msgNotInterested = 3
	msgHave          = 4
	msgBitfield      = 5
	msgRequest       = 6
	msgPiece         = 7
	msgCancel        = 8
	msgPort          = 9
	msgExtended      = 20

	// extMetadata is the ID peers send Polyfin's ut_metadata messages
	// with.
	extMetadata = 1
	// blockSize is what one request asks for; metadataBlock a piece of
	// the metadata.
	blockSize     = 16 << 10
	metadataBlock = 16 << 10
	// maxMessage bounds a peer's message: a block and its header, or a
	// bitfield of a torrent of many pieces.
	maxMessage = 1 << 21
	// maxOutbox bounds the bytes queued for a peer: what Polyfin answers
	// is queued as the peer asks, and a peer that asks without reading
	// would have it grow without end. Pieces and metadata are answered
	// while fewer than pipeline messages wait, so a peer that reads stays
	// near 1 MiB; one past maxOutbox is disconnected.
	maxOutbox = 4 << 20
	// handshakeTimeout bounds the handshake; idleTimeout how long a peer
	// may send nothing, keep-alives being sent every keepAlive.
	handshakeTimeout = 20 * time.Second
	idleTimeout      = 3 * time.Minute
	keepAlive        = time.Minute
)

const protocolName = "BitTorrent protocol"

// errHandshake reports a peer that is not one of the torrent's.
var errHandshake = errors.New("the peer's handshake is not the torrent's")

// handshake sends Polyfin's handshake for hash and reads the peer's,
// telling whether it speaks the extension protocol.
func handshake(conn net.Conn, hash Hash, peerID [20]byte, dht bool) (extensions bool, err error) {
	message := make([]byte, 0, 68)
	message = append(message, byte(len(protocolName)))
	message = append(message, protocolName...)
	var reserved [8]byte
	reserved[5] |= 0x10
	if dht {
		reserved[7] |= 0x01
	}
	message = append(message, reserved[:]...)
	message = append(message, hash[:]...)
	message = append(message, peerID[:]...)
	if _, err := conn.Write(message); err != nil {
		return false, err
	}
	var answer [68]byte
	if _, err := io.ReadFull(conn, answer[:]); err != nil {
		return false, err
	}
	if answer[0] != byte(len(protocolName)) || string(answer[1:20]) != protocolName || !bytes.Equal(answer[28:48], hash[:]) {
		return false, errHandshake
	}
	return answer[25]&0x10 != 0, nil
}

// peer is a connection to one of a torrent's peers. Its fields are guarded
// by the torrent's lock, but for those of the outbox.
type peer struct {
	t       *torrent
	conn    net.Conn
	address netip.AddrPort

	// choked tells that the peer chokes Polyfin, interested that Polyfin
	// told it it wants pieces; peerInterested and unchoked the same the
	// other way round.
	choked, interested       bool
	peerInterested, unchoked bool
	has                      []bool
	// metadataID is the ID the peer reads ut_metadata messages with, 0
	// when it does not; metadataSize the size it announced.
	metadataID   int
	metadataSize int
	// extended tells that Polyfin sent the peer its extension handshake;
	// pexCount counts the peer's ut_pex messages read since pexSince.
	extended bool
	pexSince time.Time
	pexCount int
	// requests are the blocks asked for and not received, with when.
	requests map[block]time.Time
	// asked are the blocks the peer asked for, served in order.
	asked []block

	outMu  sync.Mutex
	outbox [][]byte
	// outBytes are the bytes of the outbox.
	outBytes int
	wake     chan struct{}
	closed   chan struct{}
	once     sync.Once
}

// block is a piece's block, by its offset in the piece.
type block struct {
	piece  int
	offset int64
	length int64
}

func newPeer(t *torrent, conn net.Conn, address netip.AddrPort) *peer {
	return &peer{t: t, conn: conn, address: address, choked: true, requests: map[block]time.Time{},
		wake: make(chan struct{}, 1), closed: make(chan struct{})}
}

// send queues a message, written by the peer's writer. A peer that lets
// more than maxOutbox bytes wait is disconnected: it does not read.
func (p *peer) send(id byte, payload ...[]byte) {
	length := 1
	for _, part := range payload {
		length += len(part)
	}
	p.outMu.Lock()
	if p.outBytes+4+length > maxOutbox {
		p.outMu.Unlock()
		p.close()
		return
	}
	message := make([]byte, 4, 4+length)
	binary.BigEndian.PutUint32(message, uint32(length))
	message = append(message, id)
	for _, part := range payload {
		message = append(message, part...)
	}
	p.outbox = append(p.outbox, message)
	p.outBytes += len(message)
	p.outMu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// queued counts the messages waiting for the peer.
func (p *peer) queued() int {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	return len(p.outbox)
}

func (p *peer) close() {
	p.once.Do(func() {
		close(p.closed)
		p.conn.Close()
	})
}

// write sends the outbox, and keep-alives while it is empty. Its end, a
// panic included, closes the connection.
func (p *peer) write() {
	defer p.t.recovered("peer connection", nil)
	defer p.close()
	w := bufio.NewWriterSize(p.conn, 64<<10)
	ticker := time.NewTicker(keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-p.closed:
			return
		case <-ticker.C:
			p.outMu.Lock()
			p.outbox = append(p.outbox, []byte{0, 0, 0, 0})
			p.outMu.Unlock()
		case <-p.wake:
		}
		p.outMu.Lock()
		messages := p.outbox
		p.outbox, p.outBytes = nil, 0
		p.outMu.Unlock()
		for _, message := range messages {
			if len(message) > 13 && message[4] == msgPiece {
				// Shared bytes wait for the upload limit.
				if err := p.t.engine.up.wait(p.t.ctx, int64(len(message)-13)); err != nil {
					return
				}
				p.t.uploaded(int64(len(message) - 13))
			}
			_ = p.conn.SetWriteDeadline(time.Now().Add(idleTimeout))
			if _, err := w.Write(message); err != nil {
				return
			}
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// read reads the peer's messages until the connection ends.
func (p *peer) read() error {
	r := bufio.NewReaderSize(p.conn, 64<<10)
	var header [4]byte
	for {
		_ = p.conn.SetReadDeadline(time.Now().Add(idleTimeout))
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 {
			continue
		}
		if length > maxMessage {
			return fmt.Errorf("a message of %d bytes", length)
		}
		message := make([]byte, length)
		if _, err := io.ReadFull(r, message); err != nil {
			return err
		}
		if message[0] == msgPiece && len(message) > 9 {
			// Downloaded bytes wait for the download limit, which TCP
			// passes on to the peer.
			if err := p.t.engine.down.wait(p.t.ctx, int64(len(message)-9)); err != nil {
				return err
			}
		}
		if err := p.t.handle(p, message[0], message[1:]); err != nil {
			return err
		}
	}
}

// sendExtendedHandshake tells the peer the extensions Polyfin speaks:
// peer exchange unless the torrent is private, and the size of the
// metadata when it shares it. The lock is held.
func (p *peer) sendExtendedHandshake(metadataSize int) {
	m := map[string]any{"ut_metadata": extMetadata}
	if info := p.t.info; info == nil || !info.Private {
		m["ut_pex"] = extPEX
	}
	d := map[string]any{"m": m, "v": "Polyfin", "reqq": 250}
	if metadataSize > 0 {
		d["metadata_size"] = metadataSize
	}
	p.extended = true
	p.send(msgExtended, []byte{0}, encode(d))
}

func requestPayload(b block) []byte {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload, uint32(b.piece))
	binary.BigEndian.PutUint32(payload[4:], uint32(b.offset))
	binary.BigEndian.PutUint32(payload[8:], uint32(b.length))
	return payload
}

func readBlock(payload []byte) (block, bool) {
	if len(payload) != 12 {
		return block{}, false
	}
	return block{piece: int(binary.BigEndian.Uint32(payload)), offset: int64(binary.BigEndian.Uint32(payload[4:])),
		length: int64(binary.BigEndian.Uint32(payload[8:]))}, true
}

// dialPeer connects to a peer over route and shakes hands.
func dialPeer(ctx context.Context, route Route, address netip.AddrPort, hash Hash, peerID [20]byte, dht bool) (net.Conn, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	conn, err := route.Dial(ctx, address.String())
	if err != nil {
		return nil, false, err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	extensions, err := handshake(conn, hash, peerID, dht)
	if err != nil {
		conn.Close()
		return nil, false, err
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, extensions, nil
}
