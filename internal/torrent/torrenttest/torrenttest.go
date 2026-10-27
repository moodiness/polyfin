// Package torrenttest runs, on the loopback interface, what tests of
// torrents need: a torrent built from bytes, peers seeding or leeching it
// over TCP, or seeding it over uTP alone, peers that never answer, HTTP
// and UDP trackers, a SOCKS5 proxy with or without UDP ASSOCIATE, and a
// DHT node. None of them reaches anything else.
package torrenttest

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

// File is a file of a torrent built for a test.
type File struct {
	Name string
	Data []byte
}

// Torrent is a torrent built for a test: its info dictionary, its hash,
// and its bytes, its files' end to end.
type Torrent struct {
	Info        []byte
	Hash        [20]byte
	Data        []byte
	PieceLength int
	Pieces      int
}

// HashHex is the torrent's info hash as addons write it.
func (t *Torrent) HashHex() string { return hex.EncodeToString(t.Hash[:]) }

// Build makes a torrent of files, one file alone being a single-file
// torrent.
func Build(name string, pieceLength int, files ...File) *Torrent {
	return build(name, pieceLength, false, files)
}

// BuildPrivate makes a private torrent (BEP 27) of files.
func BuildPrivate(name string, pieceLength int, files ...File) *Torrent {
	return build(name, pieceLength, true, files)
}

func build(name string, pieceLength int, private bool, files []File) *Torrent {
	var data []byte
	for _, f := range files {
		data = append(data, f.Data...)
	}
	var pieces []byte
	for start := 0; start < len(data); start += pieceLength {
		sum := sha1.Sum(data[start:min(start+pieceLength, len(data))])
		pieces = append(pieces, sum[:]...)
	}
	info := map[string]any{"name": name, "piece length": pieceLength, "pieces": string(pieces)}
	if private {
		info["private"] = 1
	}
	if len(files) == 1 {
		info["length"] = len(files[0].Data)
	} else {
		var list []any
		for _, f := range files {
			list = append(list, map[string]any{"length": len(f.Data), "path": []any{f.Name}})
		}
		info["files"] = list
	}
	encoded := encode(info)
	return &Torrent{Info: encoded, Hash: sha1.Sum(encoded), Data: data, PieceLength: pieceLength, Pieces: len(pieces) / 20}
}

// Bytes returns n random bytes, a file's content.
func Bytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func encode(value any) []byte {
	var b bytes.Buffer
	var write func(any)
	write = func(value any) {
		switch v := value.(type) {
		case int:
			fmt.Fprintf(&b, "i%de", v)
		case string:
			fmt.Fprintf(&b, "%d:%s", len(v), v)
		case []byte:
			fmt.Fprintf(&b, "%d:%s", len(v), v)
		case []any:
			b.WriteByte('l')
			for _, item := range v {
				write(item)
			}
			b.WriteByte('e')
		case map[string]any:
			b.WriteByte('d')
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				write(key)
				write(v[key])
			}
			b.WriteByte('e')
		default:
			panic(fmt.Sprintf("torrenttest: cannot bencode %T", value))
		}
	}
	write(value)
	return b.Bytes()
}

// Peer is a peer of a torrent on the loopback interface: a seeder, which
// has every piece and shares them, or a leecher, which has none and asks
// for them. It records the connections it accepts and the bytes it sends
// and receives.
type Peer struct {
	torrent  *Torrent
	seed     bool
	listener net.Listener

	mu       sync.Mutex
	remotes  []string
	sent     int64
	received int64
	got      []bool
	conns    []net.Conn
	// ports are the DHT ports the peer tells in PORT messages.
	dhtPort uint16
	// pexPeers are the peers the peer tells in ut_pex messages, with
	// pexFlags each, and pexSent counts the messages sent.
	pexPeers []netip.AddrPort
	pexFlags byte
	pexSent  int
	// released, once Hold is called, holds the blocks asked until it is
	// closed; delay is how long each block asked waits, one after the
	// other. requests and cancels count the REQUEST and CANCEL messages
	// received.
	released chan struct{}
	delay    time.Duration
	requests int
	cancels  int

	// eager has the peer unchoke at once, before any INTERESTED; strict
	// has it drop a REQUEST received before INTERESTED, as a strict
	// client does, dropped counting them. messages are the IDs of the
	// messages received, in order.
	eager, strict bool
	dropped       int
	messages      []byte
	// handshakes counts the handshakes received for the torrent.
	handshakes int
	// dropFirst has the seeder drop, without a word, the first REQUEST of
	// the blocks it tells, and dropAll every REQUEST,
	// dropped counting them too; asks counts the REQUESTs of each block,
	// by its index among the torrent's.
	dropFirst func(block int) bool
	dropAll   bool
	// keepAlive, when set, has the peer send a keep-alive that often.
	keepAlive time.Duration
	asks      map[int]int
	// noMetadata has a seeder offer no info dictionary (BEP 9), as a
	// client that takes no part in it: it sends pieces alone.
	noMetadata bool
}

// HideMetadata has a seeder offer no info dictionary from its next
// connection on: a torrent that does not know it yet cannot start from
// this peer alone.
func (p *Peer) HideMetadata() {
	p.mu.Lock()
	p.noMetadata = true
	p.mu.Unlock()
}

// Silent returns the address of a peer on host that accepts connections
// and never answers them, as one whose address the DHT keeps but that is
// gone or cannot be reached: its dials time out. Nothing answers on its
// UDP port either.
func Silent(tb testing.TB, host string) netip.AddrPort {
	tb.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		tb.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	tb.Cleanup(func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			conn.Close()
		}
	})
	return netip.MustParseAddrPort(listener.Addr().String())
}

// Seed starts a seeder of t listening on host ("127.0.0.1" for most tests).
func Seed(tb testing.TB, t *Torrent, host string) *Peer {
	return start(tb, t, host, true)
}

// SeedUnchoked starts a seeder of t that unchokes whoever connects at once,
// before any INTERESTED, as many clients do; strict, it drops the REQUESTs
// received before INTERESTED, as a strict client does.
func SeedUnchoked(tb testing.TB, t *Torrent, host string, strict bool) *Peer {
	p := start(tb, t, host, true)
	p.mu.Lock()
	p.eager, p.strict = true, strict
	p.mu.Unlock()
	return p
}

// Messages lists the IDs of the messages the peer received, in order,
// keep-alives aside.
func (p *Peer) Messages() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.messages)
}

// Dropped counts the REQUESTs the peer dropped: a strict peer's received
// before INTERESTED, and those DropFirst and DropAll drop.
func (p *Peer) Dropped() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}

// DropFirst has the seeder drop, without a word (no REJECT, no CHOKE), the
// first REQUEST of the blocks drop tells, by their index among the
// torrent's, as a client that loses requests does; it sends those asked
// again.
func (p *Peer) DropFirst(drop func(block int) bool) {
	p.mu.Lock()
	p.dropFirst = drop
	p.mu.Unlock()
}

// DropAll has the seeder drop every REQUEST without a word, as a client
// that keeps the connection but sends nothing does.
func (p *Peer) DropAll() {
	p.mu.Lock()
	p.dropAll = true
	p.mu.Unlock()
}

// KeepAlive has the peer send a keep-alive every d, as clients do.
func (p *Peer) KeepAlive(d time.Duration) {
	p.mu.Lock()
	p.keepAlive = d
	p.mu.Unlock()
}

// MostAsked is how many REQUESTs the block asked most received.
func (p *Peer) MostAsked() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	most := 0
	for _, n := range p.asks {
		most = max(most, n)
	}
	return most
}

// Asked reports whether the peer was asked for a block holding any of the
// length bytes at offset among the torrent's.
func (p *Peer) Asked(offset, length int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for block := offset / blockSize; block*blockSize < offset+length; block++ {
		if p.asks[block] > 0 {
			return true
		}
	}
	return false
}

// Leech starts a leecher of t: it asks for every piece of whoever connects.
func Leech(tb testing.TB, t *Torrent, host string) *Peer {
	return start(tb, t, host, false)
}

func start(tb testing.TB, t *Torrent, host string, seed bool) *Peer {
	tb.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		tb.Fatal(err)
	}
	return listen(tb, t, listener, seed)
}

// listen serves the peers that connect to listener.
func listen(tb testing.TB, t *Torrent, listener net.Listener, seed bool) *Peer {
	p := &Peer{torrent: t, seed: seed, listener: listener, got: make([]bool, t.Pieces)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			p.remotes = append(p.remotes, conn.RemoteAddr().String())
			p.conns = append(p.conns, conn)
			p.mu.Unlock()
			go p.serve(conn)
		}
	}()
	tb.Cleanup(p.Close)
	return p
}

// TellDHT has the peer tell a DHT port to those that connect.
func (p *Peer) TellDHT(port uint16) {
	p.mu.Lock()
	p.dhtPort = port
	p.mu.Unlock()
}

// TellPeers has the peer tell peers in a ut_pex message (BEP 11), each
// with flags, to those that connect and offer peer exchange.
func (p *Peer) TellPeers(flags byte, peers ...netip.AddrPort) {
	p.mu.Lock()
	p.pexPeers, p.pexFlags = slices.Clone(peers), flags
	p.mu.Unlock()
}

// PEXSent counts the ut_pex messages the peer sent.
func (p *Peer) PEXSent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pexSent
}

// Hold has the seeder hold the blocks it is asked for until Release,
// as a peer that answers nothing; those cancelled meanwhile are never
// sent.
func (p *Peer) Hold() {
	p.mu.Lock()
	p.released = make(chan struct{})
	p.mu.Unlock()
}

// Release sends the blocks held, and those asked next at once, until the
// peer holds them again.
func (p *Peer) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released != nil {
		select {
		case <-p.released:
		default:
			close(p.released)
		}
	}
}

// Delay has the seeder send each block d after the one before, as a slow
// peer does; those cancelled meanwhile are never sent.
func (p *Peer) Delay(d time.Duration) {
	p.mu.Lock()
	p.delay = d
	p.mu.Unlock()
}

// Requests and Cancels count the REQUEST and CANCEL messages received.
func (p *Peer) Requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

func (p *Peer) Cancels() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancels
}

// Address is where the peer listens.
func (p *Peer) Address() netip.AddrPort {
	return netip.MustParseAddrPort(p.listener.Addr().String())
}

// Remotes lists the addresses the peer's connections came from.
func (p *Peer) Remotes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.remotes)
}

// Open counts the peer's connections still open.
func (p *Peer) Open() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

// Sent and Received are the bytes of pieces the peer sent and received.
func (p *Peer) Sent() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sent
}

func (p *Peer) Received() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.received
}

// Close stops the peer and its connections.
func (p *Peer) Close() {
	p.listener.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		c.Close()
	}
}

// Disconnect closes the peer's connections, as a peer that drops them
// does; it still listens.
func (p *Peer) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		c.Close()
	}
}

const blockSize = 16 << 10

func (p *Peer) serve(conn net.Conn) {
	defer func() {
		conn.Close()
		p.mu.Lock()
		p.conns = slices.DeleteFunc(p.conns, func(c net.Conn) bool { return c == conn })
		p.mu.Unlock()
	}()
	var hello [68]byte
	if _, err := io.ReadFull(conn, hello[:]); err != nil || !bytes.Equal(hello[28:48], p.torrent.Hash[:]) {
		return
	}
	p.mu.Lock()
	p.handshakes++
	p.mu.Unlock()
	reply := append([]byte{19}, "BitTorrent protocol"...)
	reply = append(reply, 0, 0, 0, 0, 0, 0x10, 0, 1)
	reply = append(reply, p.torrent.Hash[:]...)
	reply = append(reply, "-TT0001-000000000000"...)
	var writeMu sync.Mutex
	send := func(id byte, parts ...[]byte) error {
		length := 1
		for _, part := range parts {
			length += len(part)
		}
		message := binary.BigEndian.AppendUint32(nil, uint32(length))
		message = append(message, id)
		for _, part := range parts {
			message = append(message, part...)
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_, err := conn.Write(message)
		return err
	}
	writeMu.Lock()
	_, err := conn.Write(reply)
	writeMu.Unlock()
	if err != nil {
		return
	}
	metadataSize := 0
	p.mu.Lock()
	hidden := p.noMetadata
	p.mu.Unlock()
	if p.seed && !hidden {
		metadataSize = len(p.torrent.Info)
	}
	extended := map[string]any{"m": map[string]any{"ut_metadata": 3}}
	if metadataSize > 0 {
		extended["metadata_size"] = metadataSize
	}
	_ = send(20, []byte{0}, encode(extended))
	bits := make([]byte, (p.torrent.Pieces+7)/8)
	if p.seed {
		for i := range p.torrent.Pieces {
			bits[i/8] |= 0x80 >> (i % 8)
		}
	}
	_ = send(5, bits)
	p.mu.Lock()
	dhtPort, eager, strict := p.dhtPort, p.eager, p.strict
	p.mu.Unlock()
	if eager {
		_ = send(1)
	}
	interested := false
	if dhtPort != 0 {
		_ = send(9, binary.BigEndian.AppendUint16(nil, dhtPort))
	}
	if !p.seed {
		_ = send(2)
	}
	remoteHas := make([]bool, p.torrent.Pieces)
	asked := map[int]bool{}
	unchoked := false
	request := func() {
		if !unchoked || p.seed {
			return
		}
		for i, has := range remoteHas {
			p.mu.Lock()
			got := p.got[i]
			p.mu.Unlock()
			if !has || got || asked[i] {
				continue
			}
			asked[i] = true
			size := min(p.torrent.PieceLength, len(p.torrent.Data)-i*p.torrent.PieceLength)
			for offset := 0; offset < size; offset += blockSize {
				payload := binary.BigEndian.AppendUint32(nil, uint32(i))
				payload = binary.BigEndian.AppendUint32(payload, uint32(offset))
				payload = binary.BigEndian.AppendUint32(payload, uint32(min(blockSize, size-offset)))
				_ = send(6, payload)
			}
		}
	}
	// answer sends a block asked for, a REQUEST's payload.
	answer := func(payload []byte) error {
		index := int(binary.BigEndian.Uint32(payload))
		offset := int(binary.BigEndian.Uint32(payload[4:]))
		n := int(binary.BigEndian.Uint32(payload[8:]))
		start := index*p.torrent.PieceLength + offset
		if start+n > len(p.torrent.Data) {
			return nil
		}
		p.mu.Lock()
		p.sent += int64(n)
		p.mu.Unlock()
		return send(7, payload[:8], p.torrent.Data[start:start+n])
	}
	// Blocks held or delayed are answered in order by one goroutine,
	// unless cancelled meanwhile.
	var cancelMu sync.Mutex
	cancelled := map[string]bool{}
	waiting := make(chan []byte, 1024)
	done := make(chan struct{})
	defer close(done)
	p.mu.Lock()
	every := p.keepAlive
	p.mu.Unlock()
	if every > 0 {
		go func() {
			ticker := time.NewTicker(every)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					writeMu.Lock()
					_, err := conn.Write([]byte{0, 0, 0, 0})
					writeMu.Unlock()
					if err != nil {
						return
					}
				}
			}
		}()
	}
	go func() {
		for {
			var payload []byte
			select {
			case <-done:
				return
			case payload = <-waiting:
			}
			p.mu.Lock()
			released, delay := p.released, p.delay
			p.mu.Unlock()
			if released != nil {
				select {
				case <-done:
					return
				case <-released:
				}
			}
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-done:
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			cancelMu.Lock()
			skip := cancelled[string(payload)]
			delete(cancelled, string(payload))
			cancelMu.Unlock()
			if !skip && answer(payload) != nil {
				return
			}
		}
	}()
	r := bufio.NewReader(conn)
	for {
		var header [4]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 {
			continue
		}
		message := make([]byte, length)
		if _, err := io.ReadFull(r, message); err != nil {
			return
		}
		payload := message[1:]
		p.mu.Lock()
		p.messages = append(p.messages, message[0])
		p.mu.Unlock()
		switch message[0] {
		case 1:
			unchoked = true
			request()
		case 0:
			unchoked = false
			clear(asked)
		case 3:
			interested = false
		case 2:
			interested = true
			if p.seed {
				_ = send(1)
			}
		case 4:
			if index := int(binary.BigEndian.Uint32(payload)); index < len(remoteHas) {
				remoteHas[index] = true
			}
			request()
		case 5:
			for i := range remoteHas {
				remoteHas[i] = i/8 < len(payload) && payload[i/8]&(0x80>>(i%8)) != 0
			}
			request()
		case 6:
			if !p.seed || len(payload) != 12 {
				continue
			}
			if strict && !interested {
				p.mu.Lock()
				p.dropped++
				p.mu.Unlock()
				continue
			}
			index := int(binary.BigEndian.Uint32(payload))*p.torrent.PieceLength + int(binary.BigEndian.Uint32(payload[4:]))
			p.mu.Lock()
			p.requests++
			if p.asks == nil {
				p.asks = map[int]int{}
			}
			p.asks[index/blockSize]++
			drop := p.dropAll || p.dropFirst != nil && p.asks[index/blockSize] == 1 && p.dropFirst(index/blockSize)
			if drop {
				p.dropped++
			}
			slow := p.released != nil || p.delay > 0
			p.mu.Unlock()
			if drop {
				continue
			}
			if slow {
				cancelMu.Lock()
				delete(cancelled, string(payload))
				cancelMu.Unlock()
				select {
				case waiting <- payload:
				default:
					// Past what any client asks at once: dropped.
				}
				continue
			}
			if answer(payload) != nil {
				return
			}
		case 8:
			if len(payload) != 12 {
				continue
			}
			p.mu.Lock()
			p.cancels++
			p.mu.Unlock()
			cancelMu.Lock()
			cancelled[string(payload)] = true
			cancelMu.Unlock()
		case 7:
			if len(payload) < 8 {
				continue
			}
			index := int(binary.BigEndian.Uint32(payload))
			p.mu.Lock()
			p.received += int64(len(payload) - 8)
			if index < len(p.got) {
				p.got[index] = true
			}
			p.mu.Unlock()
		case 20:
			if len(payload) > 0 && payload[0] == 0 {
				// The engine's extension handshake: peer exchange is
				// answered with the ID it gives ut_pex.
				if i := bytes.Index(payload, []byte("6:ut_pexi")); i >= 0 {
					end := bytes.IndexByte(payload[i+9:], 'e')
					if id, _ := strconv.Atoi(string(payload[i+9 : i+9+end])); id > 0 && id < 256 {
						p.sendPEX(byte(id), send)
					}
				}
			}
			if len(payload) > 0 && payload[0] == 3 && p.seed && !hidden {
				// A metadata request: msg_type 0 and its piece.
				piece := 0
				if i := bytes.Index(payload, []byte("5:piecei")); i >= 0 {
					end := bytes.IndexByte(payload[i+8:], 'e')
					piece, _ = strconv.Atoi(string(payload[i+8 : i+8+end]))
				}
				start := piece * blockSize
				if start >= len(p.torrent.Info) {
					continue
				}
				end := min(start+blockSize, len(p.torrent.Info))
				// The engine's ut_metadata ID is 1.
				_ = send(20, []byte{1}, encode(map[string]any{"msg_type": 1, "piece": piece, "total_size": len(p.torrent.Info)}),
					p.torrent.Info[start:end])
			}
		}
	}
}

// sendPEX sends the peers to tell, if any, in a ut_pex message of id.
func (p *Peer) sendPEX(id byte, send func(byte, ...[]byte) error) {
	p.mu.Lock()
	peers, flags := p.pexPeers, p.pexFlags
	p.mu.Unlock()
	if len(peers) == 0 {
		return
	}
	var added, addedFlags, added6, added6Flags []byte
	for _, peer := range peers {
		if peer.Addr().Is4() {
			added = append(added, peer.Addr().AsSlice()...)
			added = binary.BigEndian.AppendUint16(added, peer.Port())
			addedFlags = append(addedFlags, flags)
		} else {
			added6 = append(added6, peer.Addr().AsSlice()...)
			added6 = binary.BigEndian.AppendUint16(added6, peer.Port())
			added6Flags = append(added6Flags, flags)
		}
	}
	message := map[string]any{"added": added, "added.f": addedFlags, "added6": added6, "added6.f": added6Flags}
	if send(20, []byte{id}, encode(message)) == nil {
		p.mu.Lock()
		p.pexSent++
		p.mu.Unlock()
	}
}

// Tracker is an HTTP tracker on the loopback interface listing peers.
type Tracker struct {
	server *httptest.Server

	mu        sync.Mutex
	peers     []netip.AddrPort
	announces []url.Values
	remotes   []string
}

// HTTPTracker starts an HTTP tracker listing peers, listening on host.
func HTTPTracker(tb testing.TB, host string, peers ...netip.AddrPort) *Tracker {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		tb.Fatal(err)
	}
	t := &Tracker{peers: peers}
	t.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.mu.Lock()
		t.announces = append(t.announces, r.URL.Query())
		t.remotes = append(t.remotes, r.RemoteAddr)
		var compact []byte
		for _, peer := range t.peers {
			compact = append(compact, peer.Addr().AsSlice()...)
			compact = binary.BigEndian.AppendUint16(compact, peer.Port())
		}
		t.mu.Unlock()
		_, _ = w.Write(encode(map[string]any{"interval": 60, "peers": string(compact)}))
	}))
	t.server.Listener.Close()
	t.server.Listener = listener
	t.server.Start()
	tb.Cleanup(t.server.Close)
	return t
}

// URL is the tracker's announce URL.
func (t *Tracker) URL() string { return t.server.URL + "/announce" }

// Announces lists the events of the announces received, "" for regular
// ones.
func (t *Tracker) Announces() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var events []string
	for _, q := range t.announces {
		events = append(events, q.Get("event"))
	}
	return events
}

// Remotes lists where the announces came from.
func (t *Tracker) Remotes() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.remotes)
}

// UDPTracker is a UDP tracker (BEP 15) on the loopback interface.
type UDPTracker struct {
	conn  *net.UDPConn
	peers []netip.AddrPort

	mu        sync.Mutex
	announces int
	remotes   []string
}

// StartUDPTracker starts a UDP tracker listing peers.
func StartUDPTracker(tb testing.TB, peers ...netip.AddrPort) *UDPTracker {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		tb.Fatal(err)
	}
	t := &UDPTracker{conn: conn, peers: peers}
	tb.Cleanup(func() { conn.Close() })
	go func() {
		buffer := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDPAddrPort(buffer)
			if err != nil {
				return
			}
			if n < 16 {
				continue
			}
			t.mu.Lock()
			t.remotes = append(t.remotes, from.String())
			t.mu.Unlock()
			action := binary.BigEndian.Uint32(buffer[8:])
			txid := buffer[12:16]
			switch {
			case action == 0:
				reply := binary.BigEndian.AppendUint32(nil, 0)
				reply = append(reply, txid...)
				reply = append(reply, 1, 2, 3, 4, 5, 6, 7, 8)
				_, _ = conn.WriteToUDPAddrPort(reply, from)
			case action == 1 && n >= 98:
				t.mu.Lock()
				t.announces++
				t.mu.Unlock()
				reply := binary.BigEndian.AppendUint32(nil, 1)
				reply = append(reply, txid...)
				reply = binary.BigEndian.AppendUint32(reply, 60)
				reply = binary.BigEndian.AppendUint32(reply, 0)
				reply = binary.BigEndian.AppendUint32(reply, uint32(len(t.peers)))
				for _, peer := range t.peers {
					reply = append(reply, peer.Addr().AsSlice()...)
					reply = binary.BigEndian.AppendUint16(reply, peer.Port())
				}
				_, _ = conn.WriteToUDPAddrPort(reply, from)
			}
		}
	}()
	return t
}

// URL is the tracker's announce URL.
func (t *UDPTracker) URL() string { return "udp://" + t.conn.LocalAddr().String() + "/announce" }

// Announces counts the announces received.
func (t *UDPTracker) Announces() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.announces
}

// DHTNode is a DHT node on the loopback interface that answers get_peers
// with peers.
type DHTNode struct {
	conn    *net.UDPConn
	peers   []netip.AddrPort
	mu      sync.Mutex
	queries int
	drop    int
	// released, once Hold is called, holds the answers until it is
	// closed; nodes are the nodes the answers tell.
	released    chan struct{}
	releaseOnce sync.Once
	nodes       []netip.AddrPort
}

// StartDHTNode starts a DHT node listing peers for any info hash.
func StartDHTNode(tb testing.TB, peers ...netip.AddrPort) *DHTNode {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		tb.Fatal(err)
	}
	d := &DHTNode{conn: conn, peers: peers}
	tb.Cleanup(func() { conn.Close() })
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, from, err := conn.ReadFromUDPAddrPort(buffer)
			if err != nil {
				return
			}
			message := buffer[:n]
			i := bytes.Index(message, []byte("1:t"))
			if i < 0 || !bytes.Contains(message, []byte("9:get_peers")) {
				continue
			}
			// The transaction ID, "1:t" then a string.
			rest := message[i+3:]
			colon := bytes.IndexByte(rest, ':')
			length, _ := strconv.Atoi(string(rest[:colon]))
			txid := string(rest[colon+1 : colon+1+length])
			d.mu.Lock()
			d.queries++
			dropped := d.drop > 0
			if dropped {
				d.drop--
			}
			released, nodes := d.released, d.nodes
			d.mu.Unlock()
			if dropped {
				continue
			}
			var values []any
			for _, peer := range d.peers {
				values = append(values, string(binary.BigEndian.AppendUint16(peer.Addr().AsSlice(), peer.Port())))
			}
			var compactNodes []byte
			for k, node := range nodes {
				id := bytes.Repeat([]byte{byte(k + 1)}, 20)
				compactNodes = binary.BigEndian.AppendUint16(append(append(compactNodes, id...), node.Addr().AsSlice()...), node.Port())
			}
			reply := encode(map[string]any{"t": txid, "y": "r", "r": map[string]any{"id": string(make([]byte, 20)), "token": "tk",
				"values": values, "nodes": compactNodes}})
			if released == nil {
				_, _ = conn.WriteToUDPAddrPort(reply, from)
				continue
			}
			go func() {
				<-released
				_, _ = conn.WriteToUDPAddrPort(reply, from)
			}()
		}
	}()
	tb.Cleanup(d.Release)
	return d
}

// Hold has the node hold its answers until Release.
func (d *DHTNode) Hold() {
	d.mu.Lock()
	d.released = make(chan struct{})
	d.mu.Unlock()
}

// Release sends the answers held, and those asked next at once.
func (d *DHTNode) Release() {
	d.mu.Lock()
	released := d.released
	d.mu.Unlock()
	if released != nil {
		d.releaseOnce.Do(func() { close(released) })
	}
}

// TellNodes has the node's answers tell nodes, which a lookup asks next.
func (d *DHTNode) TellNodes(nodes ...netip.AddrPort) {
	d.mu.Lock()
	d.nodes = slices.Clone(nodes)
	d.mu.Unlock()
}

// Address is the node's address.
func (d *DHTNode) Address() string { return d.conn.LocalAddr().String() }

// Queries counts the get_peers queries received.
func (d *DHTNode) Queries() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queries
}

// Drop has the node leave the next n queries unanswered, as when a
// datagram is lost or an answer comes too late.
func (d *DHTNode) Drop(n int) {
	d.mu.Lock()
	d.drop = n
	d.mu.Unlock()
}

// Proxy is a SOCKS5 proxy on the loopback interface, with or without UDP
// ASSOCIATE. It records the addresses it is asked to reach.
type Proxy struct {
	listener net.Listener
	address  string
	udp      bool
	user     string
	password string

	mu       sync.Mutex
	targets  []string
	conns    []net.Conn
	udpDelay time.Duration
	// udpTargets are the UDP targets recorded, each once.
	udpTargets map[string]bool
	// refuse are the targets whose next request is refused.
	refuse map[string]bool
}

// StartProxy starts a SOCKS5 proxy; udp offers UDP ASSOCIATE, and user,
// when set, requires that user and password.
func StartProxy(tb testing.TB, udp bool, user, password string) *Proxy {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	p := &Proxy{listener: listener, address: listener.Addr().String(), udp: udp, user: user, password: password}
	tb.Cleanup(p.Stop)
	go p.accept(listener)
	return p
}

// Address is where the proxy listens.
func (p *Proxy) Address() string { return p.address }

// DelayUDP has the proxy answer each UDP ASSOCIATE only after d, as a
// remote proxy is slow to.
func (p *Proxy) DelayUDP(d time.Duration) {
	p.mu.Lock()
	p.udpDelay = d
	p.mu.Unlock()
}

// RefuseFirst has the proxy answer the next request to reach target that
// the connection was refused (reply 5), as when the peer refused it.
func (p *Proxy) RefuseFirst(target netip.AddrPort) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.refuse == nil {
		p.refuse = map[string]bool{}
	}
	p.refuse[target.String()] = true
}

// Resume starts the proxy again on its address, once stopped.
func (p *Proxy) Resume(tb testing.TB) {
	tb.Helper()
	listener, err := net.Listen("tcp", p.address)
	if err != nil {
		tb.Fatal(err)
	}
	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()
	go p.accept(listener)
}

func (p *Proxy) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.conns = append(p.conns, conn)
		p.mu.Unlock()
		go p.serve(conn)
	}
}

// Targets lists what the proxy was asked to reach: "tcp host:port" and
// "udp host:port".
func (p *Proxy) Targets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.targets)
}

// Stop stops the proxy and every connection through it.
func (p *Proxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listener.Close()
	for _, c := range p.conns {
		c.Close()
	}
}

func (p *Proxy) record(target string) {
	p.mu.Lock()
	p.targets = append(p.targets, target)
	p.mu.Unlock()
}

// recordOnce records a target the first time it is asked to reach it.
func (p *Proxy) recordOnce(target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.udpTargets[target] {
		return
	}
	if p.udpTargets == nil {
		p.udpTargets = map[string]bool{}
	}
	p.udpTargets[target] = true
	p.targets = append(p.targets, target)
}

func readAddress(r io.Reader) (string, error) {
	var kind [1]byte
	if _, err := io.ReadFull(r, kind[:]); err != nil {
		return "", err
	}
	var host string
	switch kind[0] {
	case 1, 4:
		raw := make([]byte, 4)
		if kind[0] == 4 {
			raw = make([]byte, 16)
		}
		if _, err := io.ReadFull(r, raw); err != nil {
			return "", err
		}
		ip, _ := netip.AddrFromSlice(raw)
		host = ip.String()
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return "", err
		}
		raw := make([]byte, n[0])
		if _, err := io.ReadFull(r, raw); err != nil {
			return "", err
		}
		host = string(raw)
	default:
		return "", fmt.Errorf("address type %d", kind[0])
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))), nil
}

func (p *Proxy) serve(conn net.Conn) {
	defer conn.Close()
	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if p.user != "" {
		if !bytes.Contains(methods, []byte{2}) {
			_, _ = conn.Write([]byte{5, 0xff})
			return
		}
		_, _ = conn.Write([]byte{5, 2})
		var head [2]byte
		if _, err := io.ReadFull(conn, head[:]); err != nil {
			return
		}
		user := make([]byte, head[1])
		_, _ = io.ReadFull(conn, user)
		var n [1]byte
		_, _ = io.ReadFull(conn, n[:])
		password := make([]byte, n[0])
		_, _ = io.ReadFull(conn, password)
		if string(user) != p.user || string(password) != p.password {
			_, _ = conn.Write([]byte{1, 1})
			return
		}
		_, _ = conn.Write([]byte{1, 0})
	} else {
		_, _ = conn.Write([]byte{5, 0})
	}
	var request [3]byte
	if _, err := io.ReadFull(conn, request[:]); err != nil {
		return
	}
	target, err := readAddress(conn)
	if err != nil {
		return
	}
	bound := []byte{1, 127, 0, 0, 1, 0, 0}
	switch {
	case request[1] == 1:
		p.record("tcp " + target)
		p.mu.Lock()
		refused := p.refuse[target]
		delete(p.refuse, target)
		p.mu.Unlock()
		var upstream net.Conn
		var err error
		if !refused {
			upstream, err = net.DialTimeout("tcp", target, 10*time.Second)
		}
		if refused || err != nil {
			_, _ = conn.Write(append([]byte{5, 5, 0}, bound...))
			return
		}
		defer upstream.Close()
		p.mu.Lock()
		p.conns = append(p.conns, upstream)
		p.mu.Unlock()
		_, _ = conn.Write(append([]byte{5, 0, 0}, bound...))
		go func() {
			_, _ = io.Copy(upstream, conn)
			upstream.Close()
		}()
		_, _ = io.Copy(conn, upstream)
	case request[1] == 3 && p.udp:
		p.mu.Lock()
		delay := p.udpDelay
		p.mu.Unlock()
		time.Sleep(delay)
		relay, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			return
		}
		defer relay.Close()
		port := relay.LocalAddr().(*net.UDPAddr).Port
		_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(port >> 8), byte(port)})
		go p.relay(relay)
		_, _ = io.Copy(io.Discard, conn)
	default:
		_, _ = conn.Write(append([]byte{5, 7, 0}, bound...))
	}
}

// relay forwards a client's datagrams and the answers to them.
func (p *Proxy) relay(relay *net.UDPConn) {
	var client netip.AddrPort
	buffer := make([]byte, 65536)
	for {
		n, from, err := relay.ReadFromUDPAddrPort(buffer)
		if err != nil {
			return
		}
		if from.Addr().IsLoopback() && (client == from || !client.IsValid()) && n > 3 && buffer[0] == 0 && buffer[1] == 0 &&
			buffer[2] == 0 && !p.isTarget(from) {
			r := bytes.NewReader(buffer[3:n])
			target, err := readAddress(r)
			if err != nil {
				continue
			}
			client = from
			// A target is recorded once: uTP sends it many datagrams.
			p.recordOnce("udp " + target)
			// An address is not looked up: the resolver is the system's,
			// which a test may replace.
			resolved, err := netip.ParseAddrPort(target)
			if err != nil {
				addr, err := net.ResolveUDPAddr("udp4", target)
				if err != nil {
					continue
				}
				resolved = addr.AddrPort()
			}
			_, _ = relay.WriteToUDPAddrPort(buffer[3+(n-3-r.Len()):n], resolved)
			continue
		}
		header := []byte{0, 0, 0, 1}
		header = append(header, from.Addr().AsSlice()...)
		header = binary.BigEndian.AppendUint16(header, from.Port())
		_, _ = relay.WriteToUDPAddrPort(append(header, buffer[:n]...), client)
	}
}

// isTarget tells whether a datagram comes from an address the client sent
// to, rather than from the client.
func (p *Proxy) isTarget(from netip.AddrPort) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.udpTargets["udp "+from.String()]
}
