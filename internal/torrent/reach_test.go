package torrent

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// localService is a service of the local network: it records what reaches
// it over HTTP, over TCP and over UDP.
type localService struct {
	web *httptest.Server
	tcp net.Listener
	udp *net.UDPConn

	mu        sync.Mutex
	requests  []string
	accepted  int
	datagrams int
}

func startLocalService(t *testing.T) *localService {
	t.Helper()
	s := &localService{}
	s.web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.RequestURI())
		s.mu.Unlock()
	}))
	t.Cleanup(s.web.Close)
	var err error
	if s.tcp, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.tcp.Close() })
	go func() {
		for {
			conn, err := s.tcp.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.accepted++
			s.mu.Unlock()
			conn.Close()
		}
	}()
	if s.udp, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.udp.Close() })
	go func() {
		buffer := make([]byte, 2048)
		for {
			if _, _, err := s.udp.ReadFromUDPAddrPort(buffer); err != nil {
				return
			}
			s.mu.Lock()
			s.datagrams++
			s.mu.Unlock()
		}
	}()
	return s
}

// peer is where the service accepts TCP connections, as a peer would,
// and node where it receives datagrams, as a tracker or a DHT node would.
func (s *localService) peer() netip.AddrPort { return netip.MustParseAddrPort(s.tcp.Addr().String()) }
func (s *localService) node() netip.AddrPort {
	return netip.MustParseAddrPort(s.udp.LocalAddr().String())
}

// untouched fails the test when anything reached the service, or when the
// proxy was asked for an address other than the public one of the tests.
func (s *localService) untouched(t *testing.T, proxy *torrenttest.Proxy) {
	t.Helper()
	s.mu.Lock()
	requests, accepted, datagrams := slices.Clone(s.requests), s.accepted, s.datagrams
	s.mu.Unlock()
	if len(requests) != 0 || accepted != 0 || datagrams != 0 {
		t.Errorf("the local service received the requests %q, %d connections and %d datagrams", requests, accepted, datagrams)
	}
	for _, target := range proxy.Targets() {
		_, address, _ := strings.Cut(target, " ")
		if ap, err := netip.ParseAddrPort(address); err != nil || ap.Addr() != torrenttest.Public {
			t.Errorf("the proxy was asked for %q", target)
		}
	}
}

// publicEngine is an engine with the rule of the server: the internet is
// what stremio.Public says, and the loopback is not.
func publicEngine(t *testing.T, settings *testSettings) *Engine {
	t.Helper()
	return newEngineWith(t, settings, func(c *Config) { c.Public = nil })
}

// portOf is the port of a host and a port.
func portOf(hostport string) string {
	_, p, _ := net.SplitHostPort(hostport)
	return p
}

// through is a tracker's URL at the public address of the tests, which a
// torrenttest.Proxy and a torrenttest.WireGuardServer carry to the
// loopback.
func through(raw string) string { return trackerName(raw, torrenttest.Public.String()) }

// localAddresses are addresses that are not on the internet: loopback,
// private, link-local, carrier-grade NAT, and their IPv6 likes.
var localAddresses = []string{"127.0.0.1", "10.0.0.5", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fe80::1",
	"fd00::1", "::ffff:192.168.1.1"}

// A stream's trackers and DHT nodes on addresses that are not public are
// not announced to nor asked, over HTTP or UDP, and the stream plays with
// its other sources, a tracker and a peer on public addresses, through
// the proxy.
func TestAStreamsTrackersAndNodesAreOnPublicAddresses(t *testing.T) {
	t.Parallel()
	local := startLocalService(t)
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := publicEngine(t, proxied(proxy))

	sources := []string{
		"tracker:" + local.web.URL + "/admin/restart?confirm=1",
		"tracker:udp://" + local.node().String() + "/announce",
		"dht:" + local.node().String(),
		// Addresses in other notations, which a proxy may read as
		// addresses.
		"tracker:http://127.1:" + portOf(local.web.Listener.Addr().String()) + "/announce",
		"tracker:http://0x7f000001:" + portOf(local.web.Listener.Addr().String()) + "/announce",
		"tracker:http://2130706433:" + portOf(local.web.Listener.Addr().String()) + "/announce",
		"dht:127.1:" + portOf(local.node().String()),
	}
	for _, address := range localAddresses {
		host := address
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		sources = append(sources, "tracker:http://"+host+":8080/announce", "tracker:udp://"+host+":6969/announce", "dht:"+host+":6881")
	}
	public := through(tracker.URL())
	sources = append(sources, "tracker:"+public)
	// One listing names 30 trackers and 20 nodes at most: each kind of
	// source goes in a listing of its own, which the torrent joins.
	var specs []Spec
	for chunk := range slices.Chunk(sources, 10) {
		spec, err := NewSpec(tor.HashHex(), nil, chunk)
		if err != nil {
			t.Fatal(err)
		}
		specs = append(specs, spec)
	}
	// The last listing, with the public tracker, starts the torrent, and
	// the others join it.
	last := specs[len(specs)-1]
	if !slices.Contains(last.Trackers, public) {
		t.Fatalf("the last listing: %+v", last)
	}
	defer e.Play(last)()
	for _, spec := range specs[:len(specs)-1] {
		if len(spec.Trackers)+len(spec.Nodes) != 10 {
			t.Fatalf("a listing's sources were not all read: %+v", spec)
		}
		defer e.Play(spec)()
	}
	if got := readAll(t, e, last); !bytes.Equal(got, large) {
		t.Fatalf("read %d bytes, not the file's", len(got))
	}
	tr := torrentOf(e, last.Hash)
	tr.mu.Lock()
	trackers, nodes := slices.Clone(tr.trackers), slices.Clone(tr.nodes)
	tr.mu.Unlock()
	if !slices.Equal(trackers, []string{public}) || len(nodes) != 0 {
		t.Errorf("the torrent kept the trackers %q and the nodes %q", trackers, nodes)
	}
	if len(tracker.Announces()) == 0 {
		t.Error("the public tracker was not announced to")
	}
	local.untouched(t, proxy)
}

// A torrent that starts with a listing whose sources are all on addresses
// that are not public sends nothing, and tells that no tracker was named.
func TestAStreamOfLocalSourcesAloneSendsNothing(t *testing.T) {
	t.Parallel()
	local := startLocalService(t)
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngineWith(t, proxied(proxy), func(c *Config) {
		c.Public = nil
		c.StartTimeout = 200 * time.Millisecond
	})
	spec, err := NewSpec(strings.Repeat("ab", 20), nil, []string{
		"tracker:" + local.web.URL + "/admin/restart?confirm=1",
		"tracker:udp://" + local.node().String() + "/announce",
		"dht:" + local.node().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Trackers) != 2 || len(spec.Nodes) != 1 {
		t.Fatalf("the sources were not all read: %+v", spec)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err = e.Open(ctx, spec)
	var unstarted *StartError
	if !errors.As(err, &unstarted) || unstarted.Trackers != 0 || !unstarted.NoneNamed {
		t.Fatalf("got %v, want a StartError of a torrent without a tracker", err)
	}
	local.untouched(t, proxy)
}

// The peers and nodes a tracker's answer, a DHT answer and peer exchange
// give on addresses that are not public are neither dialed nor asked,
// and the torrent plays from the peer on a public address.
func TestTheSwarmsPeersAndNodesAreOnPublicAddresses(t *testing.T) {
	t.Parallel()
	local := startLocalService(t)
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	seeder.TellPeers(0, local.peer(), netip.MustParseAddrPort("10.0.0.5:6881"), netip.MustParseAddrPort("[fd00::1]:6881"))
	swarm := []netip.AddrPort{local.peer(), netip.MustParseAddrPort("192.168.1.1:6881"), netip.MustParseAddrPort("169.254.1.1:6881"),
		netip.MustParseAddrPort("100.64.0.1:6881"), torrenttest.Through(seeder.Address())}
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", swarm...)
	udpTracker := torrenttest.StartUDPTracker(t, swarm...)
	node := torrenttest.StartDHTNode(t, swarm...)
	node.TellNodes(local.node(), netip.MustParseAddrPort("10.0.0.5:6881"), netip.MustParseAddrPort("100.64.0.1:6881"))
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := publicEngine(t, proxied(proxy))

	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + through(tracker.URL()), "tracker:" + through(udpTracker.URL()),
		"dht:" + torrenttest.Through(netip.MustParseAddrPort(node.Address())).String()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatalf("read %d bytes, not the file's", len(got))
	}
	waitFor(t, "the trackers, the node and the peer to tell their peers", func() bool {
		return len(tracker.Announces()) > 0 && udpTracker.Announces() > 0 && node.Queries() > 0 && seeder.PEXSent() > 0
	})
	tr := torrentOf(e, spec.Hash)
	tr.mu.Lock()
	known := len(tr.known)
	tr.mu.Unlock()
	if known != 1 {
		t.Errorf("the torrent knows of %d peers, want the public one alone", known)
	}
	local.untouched(t, proxy)
}

// What a tracker, the DHT and a peer tell is kept only on the internet:
// the peers of a tracker's or the DHT's answer, those of a peer's ut_pex
// message, the node of a peer's PORT message, and the nodes of a DHT
// answer.
func TestWhatTheSwarmTellsIsKeptOnPublicAddressesOnly(t *testing.T) {
	t.Parallel()
	e := publicEngine(t, &testSettings{s: Settings{Enabled: true}})
	public := netip.MustParseAddrPort("203.0.113.7:6881")
	for _, address := range localAddresses {
		ip := netip.MustParseAddr(address)
		if stremio.Public(ip) {
			t.Fatalf("%s is public", address)
		}
		told := netip.AddrPortFrom(ip.Unmap(), 6881)
		tr := newTorrent(e, Spec{}, nil, false)

		tr.addPeers([]netip.AddrPort{told, public})
		if len(tr.known) != 1 || !tr.knows(public) || !slices.Equal(tr.queue, []netip.AddrPort{public}) {
			t.Errorf("%s: a tracker's answer queued %v", address, tr.queue)
		}

		other := netip.MustParseAddrPort("203.0.113.8:6881")
		tr.mu.Lock()
		err := tr.receivedPEX(&peer{address: public}, pexPayload([]netip.AddrPort{told, other}, pexSeed))
		tr.mu.Unlock()
		if _, kept := tr.flags[told]; err != nil || kept || !slices.Equal(tr.pexWaiting, []netip.AddrPort{other}) {
			t.Errorf("%s: peer exchange kept the flags %v and the peers %v (%v)", address, tr.flags, tr.pexWaiting, err)
		}

		tr.learnNode(&peer{address: told}, 6881)
		if len(tr.nodes) != 0 {
			t.Errorf("%s: a PORT message kept the node %v", address, tr.nodes)
		}
		tr.learnNode(&peer{address: public}, 6882)
		if !slices.Equal(tr.nodes, []string{"203.0.113.7:6882"}) {
			t.Errorf("a public peer's PORT message kept %v", tr.nodes)
		}

		if told.Addr().Is4() {
			// The node asked answers with two nodes: one is public.
			compact := func(node netip.AddrPort) string {
				return strings.Repeat("\x01", 20) + string(node.Addr().AsSlice()) + string([]byte{byte(node.Port() >> 8), byte(node.Port())})
			}
			answer := encode(map[string]any{"t": "\x00\x01", "y": "r", "r": map[string]any{"id": strings.Repeat("\x02", 20),
				"nodes": compact(told) + compact(other)}})
			conn := &sentConn{datagramConn: datagramConn{datagrams: [][]byte{answer}, from: public}}
			if _, err := lookupPeers(context.Background(), conn, Hash{}, []string{public.String()}, e.public, func([]netip.AddrPort) {}); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(conn.to, []string{public.String(), other.String()}) {
				t.Errorf("%s: the lookup asked %v", address, conn.to)
			}
		}
		tr.cancel()
	}
}

// sentConn is a datagramConn that records where it sends.
type sentConn struct {
	datagramConn
	to []string
}

func (c *sentConn) WriteTo(p []byte, address string) error {
	c.to = append(c.to, address)
	return c.datagramConn.WriteTo(p, address)
}

// An HTTP tracker's redirect is not followed: the announce fails, and the
// address the redirect names is not asked.
func TestATrackersRedirectIsNotFollowed(t *testing.T) {
	t.Parallel()
	local := startLocalService(t)
	asked := make(chan string, 4)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- r.URL.Path
		http.Redirect(w, r, local.web.URL+"/redirected/reset?token=from-the-tracker", http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)
	proxy := torrenttest.StartProxy(t, false, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	peerID := newPeerID()
	a := announce{hash: Hash{1}, peerID: peerID, port: announcePort(peerID), event: "started", left: 1}
	_, err := announceTo(ctx, ProxyRoute(proxy.Address(), "", ""), through(redirecting.URL+"/announce"), a, false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("got %v, want the redirect told as the tracker's answer", err)
	}
	if path := <-asked; path != "/announce" || len(asked) != 0 {
		t.Fatalf("the tracker was asked %q, then %d times more", path, len(asked))
	}
	local.untouched(t, proxy)
}

// The trackers of the settings are the administrator's: one on the local
// network is announced to, whether a listing names it too or not, and
// the peers it lists are on public addresses like any other's.
func TestTheSettingsTrackersMayBeOnTheLocalNetwork(t *testing.T) {
	t.Parallel()
	for _, named := range []bool{false, true} {
		tor, _, large := twoFiles()
		seeder := torrenttest.Seed(t, tor, "127.0.0.1")
		local := startLocalService(t)
		tracker := torrenttest.HTTPTracker(t, "127.0.0.1", local.peer(), torrenttest.Through(seeder.Address()))
		address := strings.TrimPrefix(strings.TrimSuffix(tracker.URL(), "/announce"), "http://")
		if stremio.Public(netip.MustParseAddrPort(address).Addr()) {
			t.Fatal("the tracker of the settings is on a public address")
		}
		proxy := torrenttest.StartProxy(t, false, "", "")
		settings := proxied(proxy)
		settings.set(func(s *Settings) { s.Trackers = []string{tracker.URL()} })
		e := publicEngine(t, settings)
		var sources []string
		if named {
			sources = []string{"tracker:" + tracker.URL()}
		}
		spec, err := NewSpec(tor.HashHex(), nil, sources)
		if err != nil {
			t.Fatal(err)
		}
		stop := e.Play(spec)
		if got := readAll(t, e, spec); !bytes.Equal(got, large) {
			t.Fatalf("named by the listing %v: read %d bytes, not the file's", named, len(got))
		}
		stop()
		if len(tracker.Announces()) == 0 || !slices.Contains(proxy.Targets(), "tcp "+address) {
			t.Fatalf("named by the listing %v: the tracker of the settings was not announced to: %v", named, proxy.Targets())
		}
		// The peer it lists on the loopback is not dialed.
		if slices.Contains(proxy.Targets(), "tcp "+local.peer().String()) {
			t.Fatalf("named by the listing %v: the local peer was dialed: %v", named, proxy.Targets())
		}
	}
}

// lookupRoute is a route that looks names up itself, as the interface and
// WireGuard routes do, and records what it is asked to reach.
type lookupRoute struct {
	names map[string]netip.Addr

	mu      sync.Mutex
	looked  []string
	reached []string
}

func (r *lookupRoute) record(list *[]string, entry string) {
	r.mu.Lock()
	*list = append(*list, entry)
	r.mu.Unlock()
}

func (r *lookupRoute) lookup(_ context.Context, address string, _ bool) (netip.AddrPort, error) {
	host, _, _ := net.SplitHostPort(address)
	ip, known := r.names[host]
	if !known {
		return netip.AddrPort{}, errors.New("no such host")
	}
	r.record(&r.looked, host)
	return netip.MustParseAddrPort(net.JoinHostPort(ip.String(), portOf(address))), nil
}

func (r *lookupRoute) Dial(_ context.Context, address string) (net.Conn, error) {
	r.record(&r.reached, "tcp "+address)
	return nil, net.ErrClosed
}

func (r *lookupRoute) ListenPacket(context.Context) (PacketConn, error) {
	return &lookupPacketConn{route: r}, nil
}
func (r *lookupRoute) Check(context.Context) error { return nil }
func (r *lookupRoute) String() string              { return "lookup" }
func (r *lookupRoute) Close() error                { return nil }

type lookupPacketConn struct {
	datagramConn
	route *lookupRoute
}

func (c *lookupPacketConn) WriteTo(_ []byte, address string) error {
	c.route.record(&c.route.reached, "udp "+address)
	return nil
}

func (c *lookupPacketConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	return c.WriteTo(p, to.String())
}

// Where the route looks names up, a tracker's or a node's name is looked
// up once, and reached at the address found when it is public, not at all
// when it is not. A tracker of the settings is reached by its name, as
// the route would any other.
func TestANameIsReachedAtThePublicAddressItHas(t *testing.T) {
	t.Parallel()
	route := &lookupRoute{names: map[string]netip.Addr{"tracker.test": torrenttest.Public, "router.test": netip.MustParseAddr("192.168.1.1"),
		"host.test": netip.MustParseAddr("127.0.0.1"), "shared.test": netip.MustParseAddr("100.64.0.1"),
		"mapped.test": netip.MustParseAddr("::ffff:10.0.0.5")}}
	e := publicEngine(t, &testSettings{s: Settings{Enabled: true}})
	tr := newTorrent(e, Spec{}, route, true)
	defer tr.cancel()
	ctx := context.Background()
	conn, err := tr.internet.ListenPacket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"router.test", "host.test", "shared.test", "mapped.test"} {
		if _, err := tr.internet.Dial(ctx, name+":80"); !errors.Is(err, errNotPublic) {
			t.Errorf("a connection to %s: %v", name, err)
		}
		if err := conn.WriteTo([]byte("x"), name+":6881"); !errors.Is(err, errNotPublic) {
			t.Errorf("a datagram to %s: %v", name, err)
		}
	}
	if err := conn.WriteToAddrPort([]byte("x"), netip.MustParseAddrPort("192.168.1.1:6881")); !errors.Is(err, errNotPublic) {
		t.Errorf("a datagram to a private address: %v", err)
	}
	if len(route.reached) != 0 {
		t.Fatalf("the route was asked for %q", route.reached)
	}
	_, _ = tr.internet.Dial(ctx, "tracker.test:80")
	_ = conn.WriteTo([]byte("x"), "tracker.test:6881")
	if want := []string{"tcp " + torrenttest.Public.String() + ":80", "udp " + torrenttest.Public.String() + ":6881"}; !slices.Equal(route.reached, want) {
		t.Fatalf("the route was asked for %q, want %q", route.reached, want)
	}
	if n := len(route.looked); n != 10 {
		t.Fatalf("%d lookups, want one for each connection and datagram", n)
	}

	// An announce to a listing's tracker goes that way, and one to a
	// tracker of the settings as the route carries it.
	route.reached = nil
	tr.mu.Lock()
	tr.trust([]string{"http://router.test/announce"})
	tr.mu.Unlock()
	a := tr.announced("started")
	for _, tracker := range []string{"http://host.test/announce", "http://router.test/announce"} {
		_, _ = announceTo(ctx, tr.routeTo(tracker), tracker, a, true)
	}
	if want := []string{"tcp router.test:80"}; !slices.Equal(route.reached, want) {
		t.Fatalf("the route was asked for %q, want %q", route.reached, want)
	}
}

// A host is a public address or a name: an address that is not public,
// and an address in a notation a proxy may read as one, are neither.
func TestAHostIsAPublicAddressOrAName(t *testing.T) {
	t.Parallel()
	route := publicRoute{public: stremio.Public}
	for _, host := range []string{"tracker.example", "tracker.example.", "tracker.FACE", "tracker.0xyz", "1.example", "203.0.113.7", "2001:db8::1"} {
		if !route.allows(host) {
			t.Errorf("%q is refused", host)
		}
	}
	for _, host := range []string{"", ".", "127.1", "127.0.0.1.", "0x7f000001", "0X7F.1", "0x", "2130706433", "tracker.123", "10.0.0.5", "::1",
		"fe80::1%eth0", "::ffff:127.0.0.1"} {
		if route.allows(host) {
			t.Errorf("%q is allowed", host)
		}
	}
}

// Through a WireGuard tunnel, whose names Polyfin looks up, a torrent
// plays from a tracker and a peer on public addresses, and a listing's
// tracker whose name has a private address is looked up, then not
// reached.
func TestATrackersNameOnAPrivateAddressIsNotReachedThroughTheTunnel(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()), netip.AddrPortFrom(torrenttest.TunnelDNS, 53))
	server := torrenttest.StartWireGuardServer(t, map[string]netip.Addr{"tracker.test": torrenttest.Public, "router.test": torrenttest.TunnelDNS})
	settings := &testSettings{s: Settings{Enabled: true, WireGuard: server.Config()}}
	e := publicEngine(t, settings)
	e.tunnelFresh, e.tunnelWait = 6*time.Second, 5*time.Second

	private := "http://router.test:8080/announce"
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + private, "tracker:" + trackerName(tracker.URL(), "tracker.test"),
		"tracker:http://" + torrenttest.TunnelDNS.String() + ":8080/announce"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatalf("read %d bytes, not the file's", len(got))
	}
	tr := torrentOf(e, spec.Hash)
	var outcome error
	waitFor(t, "the announce to the tracker on a private address to end", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		outcome = tr.announces[private]
		return outcome != nil
	})
	if !errors.Is(outcome, errNotPublic) {
		t.Fatalf("the announce ended with %v", outcome)
	}
	tr.mu.Lock()
	trackers, known := slices.Clone(tr.trackers), len(tr.known)
	tr.mu.Unlock()
	if len(trackers) != 2 || known != 1 {
		t.Errorf("the torrent kept the trackers %q, and knows of %d peers", trackers, known)
	}
	if !slices.Contains(server.Queries(), "A router.test") {
		t.Errorf("the name was not looked up through the tunnel: %v", server.Queries())
	}
	for _, target := range server.Targets() {
		_, address, _ := strings.Cut(target, " ")
		if netip.MustParseAddrPort(address).Addr() != torrenttest.Public {
			t.Errorf("the tunnel carried %q", target)
		}
	}
}
