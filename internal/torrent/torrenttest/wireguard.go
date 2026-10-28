package torrenttest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/net/dns/dnsmessage"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

// The addresses inside a WireGuardServer's tunnel: the client's, the DNS
// server's, and Public, where the test's peers, trackers and DHT nodes are
// reached through the tunnel. Nothing answers at those addresses outside
// it.
var (
	TunnelClient = netip.MustParseAddr("10.64.0.2")
	TunnelDNS    = netip.MustParseAddr("10.64.0.1")
	Public       = netip.MustParseAddr("198.51.100.1")
)

// WireGuardServer stands for a provider's WireGuard server: a second
// userspace WireGuard device, in-process, on the loopback interface, whose
// interface is a gVisor network stack. TCP and UDP the tunnel sends to
// Public reach the same port on 127.0.0.1, where the test's peers,
// trackers and DHT nodes listen; DNS queries to TunnelDNS are answered from
// the names it was given, and anything else is dropped. It records what it
// forwards and the names it is asked.
type WireGuardServer struct {
	privateKey, publicKey [32]byte
	clientKey             [32]byte
	port                  int
	names                 map[string]netip.Addr
	dev                   *device.Device

	mu      sync.Mutex
	closed  bool
	targets []string
	queries []string
	conns   []io.Closer
}

// StartWireGuardServer starts a WireGuard server whose DNS answers names,
// lower case names to IPv4 addresses. It stops when the test ends.
func StartWireGuardServer(tb testing.TB, names map[string]netip.Addr) *WireGuardServer {
	tb.Helper()
	s := &WireGuardServer{names: names, privateKey: newKey(), clientKey: newKey()}
	s.publicKey = publicKey(s.privateKey)
	nic := channel.New(1024, 1420, "")
	ip := stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol}})
	if err := ip.CreateNIC(1, nic); err != nil {
		tb.Fatal(err)
	}
	// The stack takes what the tunnel sends to any address, and answers
	// from it.
	ip.SetPromiscuousMode(1, true)
	ip.SetSpoofing(1, true)
	ip.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: 1})
	tcpForwarder := tcp.NewForwarder(ip, 0, 256, s.forwardTCP)
	ip.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)
	udpForwarder := udp.NewForwarder(ip, s.forwardUDP)
	ip.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tun.Event, 1)
	events <- tun.EventUp
	// Go's own UDP sockets, as the route itself uses (see wireguard.go).
	s.dev = device.NewDevice(&stackTUN{nic: nic, stack: ip, ctx: ctx, cancel: cancel, events: events},
		conn.NewStdNetBind(), &device.Logger{Verbosef: device.DiscardLogf, Errorf: device.DiscardLogf})
	config := "private_key=" + hex.EncodeToString(s.privateKey[:]) + "\nlisten_port=0\nreplace_peers=true\n" + s.clientPeer() +
		"replace_allowed_ips=true\nallowed_ip=" + TunnelClient.String() + "/32\n"
	if err := s.dev.IpcSet(config); err != nil {
		tb.Fatal(err)
	}
	if err := s.dev.Up(); err != nil {
		tb.Fatal(err)
	}
	state, err := s.dev.IpcGet()
	if err != nil {
		tb.Fatal(err)
	}
	for line := range strings.SplitSeq(state, "\n") {
		if value, ok := strings.CutPrefix(line, "listen_port="); ok {
			s.port, _ = strconv.Atoi(value)
		}
	}
	if s.port == 0 {
		tb.Fatal("the WireGuard server has no port")
	}
	tb.Cleanup(s.Close)
	return s
}

// clientPeer is the client's peer line, in wireguard-go's configuration.
func (s *WireGuardServer) clientPeer() string {
	client := publicKey(s.clientKey)
	return "public_key=" + hex.EncodeToString(client[:]) + "\n"
}

// Answer sets whether the server answers the handshakes of Config's
// client: one it does not answer is dropped, as by a server that is down.
func (s *WireGuardServer) Answer(tb testing.TB, answer bool) {
	tb.Helper()
	config := s.clientPeer() + "remove=true\n"
	if answer {
		config = s.clientPeer() + "replace_allowed_ips=true\nallowed_ip=" + TunnelClient.String() + "/32\n"
	}
	if err := s.dev.IpcSet(config); err != nil {
		tb.Fatal(err)
	}
}

// Config is the WireGuard configuration a provider would hand out for
// this server: the client's private key, its address, the DNS server, and
// the server, with AllowedIPs of every IPv4 address.
func (s *WireGuardServer) Config() string {
	return s.ConfigWithKey(base64.StdEncoding.EncodeToString(s.clientKey[:]))
}

// ConfigWithKey is Config with another private key, which the server does
// not know.
func (s *WireGuardServer) ConfigWithKey(privateKey string) string {
	return "[Interface]\n" +
		"PrivateKey = " + privateKey + "\n" +
		"Address = " + TunnelClient.String() + "/32\n" +
		"DNS = " + TunnelDNS.String() + "\n\n" +
		"[Peer]\n" +
		"PublicKey = " + base64.StdEncoding.EncodeToString(s.publicKey[:]) + "\n" +
		"Endpoint = 127.0.0.1:" + strconv.Itoa(s.port) + "\n" +
		"AllowedIPs = 0.0.0.0/0\n"
}

// NewPrivateKey returns a WireGuard private key in base64.
func NewPrivateKey() string {
	key := newKey()
	return base64.StdEncoding.EncodeToString(key[:])
}

// Through is the address at which local, on 127.0.0.1, is reached through
// the tunnel.
func Through(local netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(Public, local.Port())
}

// Targets lists what the server forwarded: "tcp host:port" and "udp
// host:port", addresses inside the tunnel.
func (s *WireGuardServer) Targets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.targets)
}

// Queries lists the names its DNS server was asked, with their type, as
// "A tracker.test".
func (s *WireGuardServer) Queries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.queries)
}

// Close stops the server, and every connection it forwards.
func (s *WireGuardServer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	s.dev.Close()
	for _, c := range conns {
		c.Close()
	}
}

// track keeps c to close with the server, false once it is closed.
func (s *WireGuardServer) track(c ...io.Closer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns = append(s.conns, c...)
	return true
}

func (s *WireGuardServer) record(target string) {
	s.mu.Lock()
	s.targets = append(s.targets, target)
	s.mu.Unlock()
}

// local is where a destination in the tunnel is forwarded to.
func local(id stack.TransportEndpointID) (inside netip.AddrPort, outside string, ok bool) {
	addr, _ := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	inside = netip.AddrPortFrom(addr, id.LocalPort)
	return inside, net.JoinHostPort("127.0.0.1", strconv.Itoa(int(id.LocalPort))), addr == Public
}

func (s *WireGuardServer) forwardTCP(r *tcp.ForwarderRequest) {
	inside, outside, ok := local(r.ID())
	if !ok {
		r.Complete(true)
		return
	}
	s.record("tcp " + inside.String())
	there, err := net.DialTimeout("tcp", outside, 10*time.Second)
	if err != nil {
		r.Complete(true)
		return
	}
	var queue waiter.Queue
	endpoint, tcpErr := r.CreateEndpoint(&queue)
	if tcpErr != nil {
		r.Complete(true)
		there.Close()
		return
	}
	r.Complete(false)
	here := gonet.NewTCPConn(&queue, endpoint)
	if !s.track(here, there) {
		here.Close()
		there.Close()
		return
	}
	go func() {
		_, _ = io.Copy(there, here)
		there.Close()
		here.Close()
	}()
	go func() {
		_, _ = io.Copy(here, there)
		here.Close()
		there.Close()
	}()
}

// forwardUDP takes the first datagram of a flow: what follows it from the
// same client port to the same address comes to the endpoint it creates.
func (s *WireGuardServer) forwardUDP(r *udp.ForwarderRequest) {
	inside, outside, public := local(r.ID())
	dns := inside == netip.AddrPortFrom(TunnelDNS, 53)
	if !public && !dns {
		return
	}
	var queue waiter.Queue
	endpoint, err := r.CreateEndpoint(&queue)
	if err != nil {
		return
	}
	here := gonet.NewUDPConn(&queue, endpoint)
	if dns {
		if s.track(here) {
			go s.serveDNS(here)
		}
		return
	}
	s.record("udp " + inside.String())
	there, dialErr := net.Dial("udp", outside)
	if dialErr != nil || !s.track(here, there) {
		here.Close()
		if there != nil {
			there.Close()
		}
		return
	}
	go relayDatagrams(there, here)
	go relayDatagrams(here, there)
}

// relayDatagrams copies the datagrams from reads to writes until either
// closes, or none comes for a minute.
func relayDatagrams(from, to net.Conn) {
	defer from.Close()
	defer to.Close()
	buf := make([]byte, 65535)
	for {
		_ = from.SetReadDeadline(time.Now().Add(time.Minute))
		n, err := from.Read(buf)
		if err != nil {
			return
		}
		if _, err := to.Write(buf[:n]); err != nil {
			return
		}
	}
}

// serveDNS answers the queries of a client port: A records from the
// server's names, no other record, and NXDOMAIN for a name it does not
// know.
func (s *WireGuardServer) serveDNS(c net.Conn) {
	defer c.Close()
	buf := make([]byte, 1500)
	for {
		_ = c.SetReadDeadline(time.Now().Add(time.Minute))
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		var parser dnsmessage.Parser
		header, err := parser.Start(buf[:n])
		if err != nil {
			continue
		}
		question, err := parser.Question()
		if err != nil {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(question.Name.String(), "."))
		s.mu.Lock()
		s.queries = append(s.queries, strings.TrimPrefix(question.Type.String(), "Type")+" "+name)
		s.mu.Unlock()
		answer := dnsmessage.Message{Header: dnsmessage.Header{ID: header.ID, Response: true, Authoritative: true, RecursionAvailable: true},
			Questions: []dnsmessage.Question{question}}
		addr, known := s.names[name]
		switch {
		case !known:
			answer.Header.RCode = dnsmessage.RCodeNameError
		case question.Type == dnsmessage.TypeA:
			answer.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: addr.As4()},
			}}
		}
		packed, err := answer.Pack()
		if err != nil {
			continue
		}
		if _, err := c.Write(packed); err != nil {
			return
		}
	}
}

func newKey() [32]byte {
	var key [32]byte
	_, _ = rand.Read(key[:])
	return key
}

func publicKey(private [32]byte) [32]byte {
	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	if err != nil {
		panic(err)
	}
	return [32]byte(public)
}

// stackTUN is the WireGuard device's interface: a gVisor stack's link.
type stackTUN struct {
	nic    *channel.Endpoint
	stack  *stack.Stack
	ctx    context.Context
	cancel context.CancelFunc
	events chan tun.Event
}

func (t *stackTUN) File() *os.File { return nil }

func (t *stackTUN) MTU() (int, error) { return int(t.nic.MTU()), nil }

func (t *stackTUN) Name() (string, error) { return "test", nil }

func (t *stackTUN) BatchSize() int { return 1 }

func (t *stackTUN) Events() <-chan tun.Event { return t.events }

// Read takes a packet the stack sends, to encrypt for the client.
func (t *stackTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	packet := t.nic.ReadContext(t.ctx)
	if packet == nil {
		return 0, os.ErrClosed
	}
	defer packet.DecRef()
	view := packet.ToView()
	defer view.Release()
	n, err := view.Read(bufs[0][offset:])
	if err != nil {
		return 0, err
	}
	sizes[0] = n
	return 1, nil
}

// Write gives the stack the packets the client sent.
func (t *stackTUN) Write(bufs [][]byte, offset int) (int, error) {
	if t.ctx.Err() != nil {
		return 0, os.ErrClosed
	}
	for _, buf := range bufs {
		packet := buf[offset:]
		if len(packet) == 0 {
			continue
		}
		if packet[0]>>4 != 4 {
			return 0, syscall.EAFNOSUPPORT
		}
		pkb := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(packet)})
		t.nic.InjectInbound(header.IPv4ProtocolNumber, pkb)
		pkb.DecRef()
	}
	return len(bufs), nil
}

func (t *stackTUN) Close() error {
	t.cancel()
	t.stack.Close()
	t.nic.Close()
	close(t.events)
	return nil
}

var _ tun.Device = (*stackTUN)(nil)
