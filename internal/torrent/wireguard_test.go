package torrent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// tunneled returns settings that play torrents through server, and an
// engine whose tunnel makes a new handshake once the last is 6 seconds
// old, and waits 5 seconds for it: past wireguard-go's 5 seconds between
// two handshakes, and short enough for a test.
func tunneled(t *testing.T, config string) (*Engine, *testSettings) {
	t.Helper()
	settings := &testSettings{s: Settings{Enabled: true, WireGuard: config}}
	e := newEngine(t, settings, 64<<20)
	e.tunnelFresh, e.tunnelWait = 6*time.Second, 5*time.Second
	return e, settings
}

// trackerName is a tracker's URL with a name the tunnel's DNS knows rather
// than its address, and the address reached through the tunnel.
func trackerName(raw, name string) string {
	host := raw[strings.Index(raw, "//")+2:]
	host = host[:strings.IndexAny(host, "/")]
	_, port, _ := net.SplitHostPort(host)
	return strings.Replace(raw, host, net.JoinHostPort(name, port), 1)
}

func TestWireGuardRoutePlays(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
	udpTracker := torrenttest.StartUDPTracker(t, torrenttest.Through(seeder.Address()))
	server := torrenttest.StartWireGuardServer(t, map[string]netip.Addr{"tracker.test": torrenttest.Public, "udp.tracker.test": torrenttest.Public})
	e, _ := tunneled(t, server.Config())

	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerName(tracker.URL(), "tracker.test"),
		"tracker:" + trackerName(udpTracker.URL(), "udp.tracker.test")})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatal("the largest file was not read whole")
	}
	status := e.Status(context.Background())
	if status.RouteError != nil || status.Tunnel == nil || status.Tunnel.LastHandshake.IsZero() || !strings.Contains(status.Route, "WireGuard") {
		t.Fatalf("Health does not tell the tunnel is up: %+v", status)
	}
	stop()
	waitFor(t, "the torrent to stop", func() bool { return e.Status(context.Background()).Playing == 0 })

	// The seeder and both trackers were reached through the tunnel, the
	// tracker names looked up by its DNS.
	targets := server.Targets()
	seederInside := "tcp " + torrenttest.Through(seeder.Address()).String()
	if remotes := seeder.Remotes(); len(remotes) == 0 || len(remotes) != countPrefix(targets, seederInside) {
		t.Fatalf("the seeder saw %d connections, the tunnel forwarded %v", len(remotes), targets)
	}
	if !slices.Contains(targets, "tcp "+torrenttest.Public.String()+":"+port(tracker.URL())) {
		t.Fatalf("the HTTP tracker was not reached through the tunnel: %v", targets)
	}
	if !slices.Contains(targets, "udp "+torrenttest.Public.String()+":"+port(udpTracker.URL())) || udpTracker.Announces() == 0 {
		t.Fatalf("the UDP tracker was not reached through the tunnel: %v", targets)
	}
	if queries := server.Queries(); !slices.Contains(queries, "A tracker.test") || !slices.Contains(queries, "A udp.tracker.test") {
		t.Fatalf("the tracker names were not looked up by the tunnel's DNS: %v", queries)
	}

	// The DHT, from a node the sources name.
	other := torrenttest.Build("Another Movie", 32<<10, torrenttest.File{Name: "b.mkv", Data: torrenttest.Bytes(200 << 10)})
	otherSeeder := torrenttest.Seed(t, other, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, torrenttest.Through(otherSeeder.Address()))
	nodeInside := torrenttest.Through(netip.MustParseAddrPort(node.Address()))
	spec, err = NewSpec(other.HashHex(), nil, []string{"dht:" + nodeInside.String()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, other.Data) {
		t.Fatal("the file was not read from the DHT's peer")
	}
	if !slices.Contains(server.Targets(), "udp "+nodeInside.String()) || node.Queries() == 0 {
		t.Fatalf("the DHT node was not reached through the tunnel: %v", server.Targets())
	}
}

// port is the port of a tracker's URL.
func port(raw string) string {
	host := raw[strings.Index(raw, "//")+2:]
	host = host[:strings.IndexAny(host, "/")]
	_, p, _ := net.SplitHostPort(host)
	return p
}

func TestWireGuardKillSwitch(t *testing.T) {
	t.Parallel()
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
	server := torrenttest.StartWireGuardServer(t, nil)
	e, settings := tunneled(t, server.Config())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerName(tracker.URL(), torrenttest.Public.String())})
	if err != nil {
		t.Fatal(err)
	}

	// The server goes away during a playback: the torrent stops, and
	// nothing reaches the swarm another way.
	settings.set(func(s *Settings) { s.DownloadMbps = 1 })
	stop := e.Play(spec)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadAt(make([]byte, 1000), 0); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if _, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size())); err == nil || !strings.Contains(err.Error(), "WireGuard") {
		t.Fatalf("a read past the server's end gave %v", err)
	}
	if _, err := e.Open(ctx, spec); err == nil {
		t.Fatal("the torrent opened with the tunnel down")
	}
	status := e.Status(ctx)
	var routeErr *RouteError
	if !errors.As(status.RouteError, &routeErr) || routeErr.Kind != WireGuardHandshake || status.Failure == nil || status.Playing != 0 {
		t.Fatalf("Health does not tell the tunnel is down: %+v", status)
	}
	if got, forwarded := len(seeder.Remotes()), countPrefix(server.Targets(), "tcp "+torrenttest.Through(seeder.Address()).String()); got != forwarded {
		t.Fatalf("the seeder saw %d connections, the tunnel forwarded %d", got, forwarded)
	}
	// A playback with the tunnel down starts nothing.
	before := len(seeder.Remotes())
	e.Play(spec)()
	if _, err := e.Open(ctx, spec); err == nil {
		t.Fatal("the torrent opened with the tunnel down")
	}
	if got := len(seeder.Remotes()); got != before {
		t.Fatalf("the seeder saw %d new connections with the tunnel down", got-before)
	}
	if announces := len(tracker.Announces()); announces == 0 {
		t.Fatal("the tracker was never reached")
	}
}

func TestWireGuardWrongKeyFailsTheHandshake(t *testing.T) {
	t.Parallel()
	server := torrenttest.StartWireGuardServer(t, nil)
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
	e, _ := tunneled(t, server.ConfigWithKey(torrenttest.NewPrivateKey()))
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerName(tracker.URL(), torrenttest.Public.String())})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if _, err := e.Open(context.Background(), spec); err == nil {
		t.Fatal("a torrent opened over a tunnel whose key the server does not know")
	}
	status := e.Status(context.Background())
	var routeErr *RouteError
	if !errors.As(status.RouteError, &routeErr) || routeErr.Kind != WireGuardHandshake || status.Tunnel == nil || !status.Tunnel.LastHandshake.IsZero() {
		t.Fatalf("Health does not tell the handshake fails: %+v", status)
	}
	if !RouteFailure(status.RouteError) {
		t.Fatal("a failed handshake reads as the version's failure")
	}
	if len(seeder.Remotes()) != 0 || len(tracker.Announces()) != 0 || len(server.Targets()) != 0 {
		t.Fatalf("something went through: %v %v %v", seeder.Remotes(), tracker.Announces(), server.Targets())
	}
}

// Tracker names are looked up through the tunnel, by its DNS, and never by
// the system. The test stays sequential: it replaces net.DefaultResolver,
// which parallel tests would use, with one that records what it is asked.
func TestWireGuardNamesNeverReachTheSystem(t *testing.T) {
	var asked []string
	system := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(_ context.Context, network, address string) (net.Conn, error) {
		asked = append(asked, network+" "+address)
		return nil, errors.New("the system's resolver was asked")
	}}
	t.Cleanup(func() { net.DefaultResolver = system })

	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
	server := torrenttest.StartWireGuardServer(t, map[string]netip.Addr{"tracker.test": torrenttest.Public})
	e, _ := tunneled(t, server.Config())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerName(tracker.URL(), "tracker.test"),
		"tracker:" + trackerName(tracker.URL(), "unknown.test")})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatal("the file was not read whole")
	}
	stop()
	waitFor(t, "the torrent to stop", func() bool { return e.Status(context.Background()).Playing == 0 })
	queries := server.Queries()
	if !slices.Contains(queries, "A tracker.test") || !slices.Contains(queries, "A unknown.test") {
		t.Fatalf("the tunnel's DNS was asked %v", queries)
	}
	if len(asked) != 0 {
		t.Fatalf("the system's resolver was asked: %v", asked)
	}

	// Without a DNS server in the configuration, names are not looked up.
	config, err := ParseWireGuardConfig(server.Config())
	if err != nil {
		t.Fatal(err)
	}
	config.DNS = nil
	route := WireGuardRoute(config, e.config.Logger)
	defer route.Close()
	if _, err := route.Dial(context.Background(), "tracker.test:80"); !errors.Is(err, ErrNoDNS) {
		t.Fatalf("a name was dialed without a DNS server: %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("the system's resolver was asked: %v", asked)
	}
}

func TestWireGuardRouteChangeStopsTheTunnel(t *testing.T) {
	t.Parallel()
	server := torrenttest.StartWireGuardServer(t, nil)
	e, settings := tunneled(t, server.Config())
	if err := e.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	tunnel := e.route.(*wireGuardRoute)
	e.mu.Unlock()
	settings.set(func(s *Settings) { s.Enabled = false })
	if err := e.Ready(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("got %v, want ErrDisabled", err)
	}
	tunnel.mu.Lock()
	closed, dev := tunnel.closed, tunnel.dev
	tunnel.mu.Unlock()
	if !closed || dev != nil {
		t.Fatal("the tunnel runs on with torrents off")
	}
}

func TestParseWireGuardConfig(t *testing.T) {
	t.Parallel()
	private, public, shared := torrenttest.NewPrivateKey(), torrenttest.NewPrivateKey(), torrenttest.NewPrivateKey()
	valid := "# A provider's configuration\r\n[Interface]\r\nPrivateKey = " + private + "\r\n" +
		"Address = 10.2.0.2/32, fd00::2/128\r\nDNS = 10.2.0.1\r\nMTU = 1380\r\nPostUp = something the host would run\r\n\r\n" +
		"[Peer]\r\npublickey = " + public + "\r\nPresharedKey = " + shared + "\r\n" +
		"AllowedIPs = 0.0.0.0/0\r\nAllowedIPs = ::/0\r\nEndpoint = VPN.Example.com:51820 # the server\r\nPersistentKeepalive = 25\r\n"
	c, err := ParseWireGuardConfig(valid)
	if err != nil {
		t.Fatal(err)
	}
	if c.Endpoint != "vpn.example.com:51820" || len(c.Addresses) != 2 || len(c.DNS) != 1 || c.MTU != 1380 || len(c.AllowedIPs) != 2 ||
		c.PersistentKeepalive != 25 || c.presharedKey == [32]byte{} {
		t.Fatalf("read %+v", c)
	}
	again, err := ParseWireGuardConfig(c.Marshal())
	if err != nil || again.Marshal() != c.Marshal() {
		t.Fatalf("written as\n%s\nit reads back as %v", c.Marshal(), err)
	}
	if strings.Contains(c.Marshal(), "PostUp") {
		t.Fatal("a command for the host is kept")
	}

	minimal := func(change func(string) string) string {
		return change("[Interface]\nPrivateKey = " + private + "\nAddress = 10.2.0.2/32\n[Peer]\nPublicKey = " + public +
			"\nEndpoint = 203.0.113.5:51820\nAllowedIPs = 0.0.0.0/0\n")
	}
	replace := func(old, new string) func(string) string {
		return func(s string) string { return strings.Replace(s, old, new, 1) }
	}
	for _, test := range []struct {
		name   string
		text   string
		line   int
		key    string
		reason string
	}{
		{"short key", minimal(replace(private, "c2hvcnQ=")), 2, "PrivateKey", WireGuardInvalid},
		{"zero key", minimal(replace(private, strings.Repeat("A", 43)+"=")), 2, "PrivateKey", WireGuardInvalid},
		{"no private key", minimal(replace("PrivateKey = "+private+"\n", "")), 1, "PrivateKey", WireGuardMissing},
		{"bad address", minimal(replace("10.2.0.2/32", "10.2.0.2/33")), 3, "Address", WireGuardInvalid},
		{"IPv6 alone", minimal(replace("10.2.0.2/32", "fd00::2/128")), 3, "Address", WireGuardNoIPv4},
		{"bad endpoint", minimal(replace("203.0.113.5:51820", "203.0.113.5")), 6, "Endpoint", WireGuardInvalid},
		{"endpoint port 0", minimal(replace("203.0.113.5:51820", "vpn.example.com:0")), 6, "Endpoint", WireGuardInvalid},
		{"no endpoint", minimal(replace("Endpoint = 203.0.113.5:51820\n", "")), 4, "Endpoint", WireGuardMissing},
		{"narrow AllowedIPs", minimal(replace("0.0.0.0/0", "10.0.0.0/8")), 7, "AllowedIPs", WireGuardNotAll},
		{"bad AllowedIPs", minimal(replace("0.0.0.0/0", "0.0.0.0/0,")), 7, "AllowedIPs", WireGuardInvalid},
		{"bad DNS", minimal(replace("Address", "DNS = resolver.example\nAddress")), 3, "DNS", WireGuardInvalid},
		{"bad MTU", minimal(replace("Address", "MTU = 100\nAddress")), 3, "MTU", WireGuardInvalid},
		{"bad keepalive", minimal(replace("AllowedIPs", "PersistentKeepalive = soon\nAllowedIPs")), 7, "PersistentKeepalive", WireGuardInvalid},
		{"bad preshared key", minimal(replace("AllowedIPs", "PresharedKey = nope\nAllowedIPs")), 7, "PresharedKey", WireGuardInvalid},
		{"two peers", minimal(func(s string) string { return s + "[Peer]\n" }), 8, "Peer", WireGuardRepeated},
		{"two private keys", minimal(replace("Address", "PrivateKey = "+private+"\nAddress")), 3, "PrivateKey", WireGuardRepeated},
		{"no peer", minimal(func(s string) string { return s[:strings.Index(s, "[Peer]")] }), 0, "Peer", WireGuardMissing},
		{"unknown key", minimal(replace("Address", "Colour = blue\nAddress")), 3, "Colour", WireGuardUnknown},
		{"unknown section", minimal(replace("[Peer]", "[Server]")), 4, "[Server]", WireGuardUnknown},
		{"before a section", "PrivateKey = " + private + "\n", 1, "PrivateKey", WireGuardOutside},
		{"no equals", minimal(replace("Address = ", "Address ")), 3, "", WireGuardSyntax},
		{"too long", strings.Repeat("#", MaxWireGuardConfigBytes+1), 0, "", WireGuardTooLong},
	} {
		_, err := ParseWireGuardConfig(test.text)
		var configErr *WireGuardConfigError
		if !errors.As(err, &configErr) || configErr.Line != test.line || configErr.Key != test.key || configErr.Reason != test.reason {
			t.Errorf("%s: got %#v", test.name, err)
			continue
		}
		if strings.Contains(err.Error(), private) || strings.Contains(err.Error(), public) {
			t.Errorf("%s: the error tells a key: %v", test.name, err)
		}
	}
}

// benchSize is what BenchmarkWireGuardRoute downloads each time.
const benchSize = 256 << 20

// BenchmarkWireGuardRoute measures a download through the tunnel against
// a direct connection, on the loopback interface: the WireGuard server and
// the source run in a process of their own (see
// TestWireGuardBenchServer), so that cpu-ms/MB is Polyfin's side alone.
// The numbers of docs/torrents.md come from
// go test -run '^$' -bench WireGuardRoute -benchtime 5x ./internal/torrent.
func BenchmarkWireGuardRoute(b *testing.B) {
	child := exec.Command(os.Args[0], "-test.run=^TestWireGuardBenchServer$")
	child.Env = append(os.Environ(), "POLYFIN_WIREGUARD_BENCH=1")
	stdin, err := child.StdinPipe()
	if err != nil {
		b.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		b.Fatal(err)
	}
	if err := child.Start(); err != nil {
		b.Fatal(err)
	}
	defer child.Wait()
	defer stdin.Close()
	var server struct {
		Config string
		Source netip.AddrPort
	}
	if err := json.NewDecoder(stdout).Decode(&server); err != nil {
		b.Fatal(err)
	}
	go io.Copy(io.Discard, stdout)
	config, err := ParseWireGuardConfig(server.Config)
	if err != nil {
		b.Fatal(err)
	}
	route := WireGuardRoute(config, slog.New(slog.DiscardHandler))
	defer route.Close()
	for _, test := range []struct {
		name string
		dial func(context.Context) (net.Conn, error)
	}{
		{"direct", func(ctx context.Context) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", server.Source.String())
		}},
		{"tunnel", func(ctx context.Context) (net.Conn, error) {
			return route.Dial(ctx, torrenttest.Through(server.Source).String())
		}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.SetBytes(benchSize)
			start := processCPU()
			for b.Loop() {
				conn, err := test.dial(context.Background())
				if err != nil {
					b.Fatal(err)
				}
				n, err := io.Copy(io.Discard, conn)
				conn.Close()
				if err != nil || n != benchSize {
					b.Fatalf("read %d bytes: %v", n, err)
				}
			}
			if used := processCPU() - start; used > 0 {
				b.ReportMetric(float64(used.Milliseconds())/float64(b.N*benchSize>>20), "cpu-ms/MB")
			}
		})
	}
}

// TestWireGuardBenchServer is the WireGuard server and the source of
// BenchmarkWireGuardRoute, which runs it in a process of its own: it
// writes the configuration and the source's address, then serves until
// its standard input closes. It is skipped otherwise.
func TestWireGuardBenchServer(t *testing.T) {
	if os.Getenv("POLYFIN_WIREGUARD_BENCH") == "" {
		t.Skip("run by BenchmarkWireGuardRoute")
	}
	source := startSource(t, benchSize)
	server := torrenttest.StartWireGuardServer(t, nil)
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"Config": server.Config(), "Source": source}); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// startSource listens on the loopback interface and sends size bytes to
// whoever connects.
func startSource(tb testing.TB, size int) netip.AddrPort {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { listener.Close() })
	chunk := bytes.Repeat([]byte("polyfin!"), 8<<10)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for sent := 0; sent < size; sent += len(chunk) {
					if _, err := conn.Write(chunk[:min(len(chunk), size-sent)]); err != nil {
						return
					}
				}
			}()
		}
	}()
	return netip.MustParseAddrPort(listener.Addr().String())
}
