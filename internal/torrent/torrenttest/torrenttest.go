// Package torrenttest runs, on the loopback interface, what tests of
// torrents need: a torrent built from bytes, peers seeding or leeching it,
// HTTP and UDP trackers, a SOCKS5 proxy with or without UDP ASSOCIATE, and
// a DHT node. None of them reaches anything else.
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
}

// Seed starts a seeder of t listening on host ("127.0.0.1" for most tests).
func Seed(tb testing.TB, t *Torrent, host string) *Peer {
	return start(tb, t, host, true)
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
	if p.seed {
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
	dhtPort := p.dhtPort
	p.mu.Unlock()
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
		switch message[0] {
		case 1:
			unchoked = true
			request()
		case 0:
			unchoked = false
			clear(asked)
		case 2:
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
			index := int(binary.BigEndian.Uint32(payload))
			offset := int(binary.BigEndian.Uint32(payload[4:]))
			n := int(binary.BigEndian.Uint32(payload[8:]))
			start := index*p.torrent.PieceLength + offset
			if start+n > len(p.torrent.Data) {
				continue
			}
			p.mu.Lock()
			p.sent += int64(n)
			p.mu.Unlock()
			if send(7, payload[:8], p.torrent.Data[start:start+n]) != nil {
				return
			}
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
			if len(payload) > 0 && payload[0] == 3 && p.seed {
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
			d.mu.Unlock()
			var values []any
			for _, peer := range d.peers {
				values = append(values, string(binary.BigEndian.AppendUint16(peer.Addr().AsSlice(), peer.Port())))
			}
			reply := encode(map[string]any{"t": txid, "y": "r", "r": map[string]any{"id": string(make([]byte, 20)), "token": "tk",
				"values": values}})
			_, _ = conn.WriteToUDPAddrPort(reply, from)
		}
	}()
	return d
}

// Address is the node's address.
func (d *DHTNode) Address() string { return d.conn.LocalAddr().String() }

// Queries counts the get_peers queries received.
func (d *DHTNode) Queries() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queries
}

// Proxy is a SOCKS5 proxy on the loopback interface, with or without UDP
// ASSOCIATE. It records the addresses it is asked to reach.
type Proxy struct {
	listener net.Listener
	address  string
	udp      bool
	user     string
	password string

	mu      sync.Mutex
	targets []string
	conns   []net.Conn
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
		upstream, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
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
			p.record("udp " + target)
			resolved, err := net.ResolveUDPAddr("udp4", target)
			if err != nil {
				continue
			}
			_, _ = relay.WriteToUDP(buffer[3+(n-3-r.Len()):n], resolved)
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
	return slices.Contains(p.targets, "udp "+from.String())
}
