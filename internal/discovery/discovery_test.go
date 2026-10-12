package discovery

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"
)

// serverID is a server ID as Polyfin's look, as Jellyfin's do.
const serverID = "0123456789abcdef0123456789abcdef"

// fakeSettings are settings a test changes, telling the service as the
// accounts store does.
type fakeSettings struct {
	mu       sync.Mutex
	settings Settings
	changed  chan struct{}
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{settings: Settings{Enabled: true, Name: "Polyfin"}, changed: make(chan struct{})}
}

func (f *fakeSettings) get() Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings
}

func (f *fakeSettings) next() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changed
}

// set changes the settings and tells the service.
func (f *fakeSettings) set(change func(*Settings)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(&f.settings)
	close(f.changed)
	f.changed = make(chan struct{})
}

// testService is a service listening on a free port, or on port, with
// settings, an HTTP server on every address on port 8096, and applied
// told of each change of the settings applied.
type testService struct {
	*Service
	settings *fakeSettings
	applied  chan struct{}
}

func start(t *testing.T, port int, configure ...func(*Options)) testService {
	t.Helper()
	settings := newFakeSettings()
	applied := make(chan struct{}, 1)
	options := Options{Port: port, HTTP: &net.TCPAddr{Port: 8096}, ServerID: serverID, Settings: settings.get, Changed: settings.next,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), applied: func() { applied <- struct{}{} }}
	for _, c := range configure {
		c(&options)
	}
	s := Start(t.Context(), options)
	t.Cleanup(func() {
		s.mu.Lock()
		s.stop()
		s.mu.Unlock()
	})
	return testService{Service: s, settings: settings, applied: applied}
}

// change changes the settings and waits until the service applied them.
func (s testService) change(t *testing.T, change func(*Settings)) {
	t.Helper()
	s.settings.set(change)
	<-s.applied
}

// address is where the service listens, on loopback.
func (s testService) address(t *testing.T, loopback netip.Addr) netip.AddrPort {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		t.Fatal("not listening")
	}
	return netip.AddrPortFrom(loopback, uint16(s.port))
}

// client is a UDP socket sending to the service.
func client(t *testing.T, to netip.AddrPort) *net.UDPConn {
	t.Helper()
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(to))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func send(t *testing.T, conn *net.UDPConn, message string) {
	t.Helper()
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatal(err)
	}
}

// receive reads the answer conn received. Answers on loopback arrive at
// once: the deadline only keeps a failing test from hanging.
func receive(t *testing.T, conn *net.UDPConn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatal(err)
	}
	return string(buffer[:n])
}

// nothingPending checks that conn received nothing yet. The service
// answers messages in the order they arrive: once a message sent after
// those to check is answered, their answers, if any, have arrived.
func nothingPending(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now())
	buffer := make([]byte, 2048)
	if n, err := conn.Read(buffer); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("got %q, %v", buffer[:n], err)
	}
}

// Both messages, in any letter case and within other text as Jellyfin
// reads its own, get Jellyfin's JSON: the address of the interface that
// reaches the app with Polyfin's HTTP port, the server's ID and name, and
// a null EndpointAddress.
func TestAnswersBothMessages(t *testing.T) {
	s := start(t, 0)
	conn := client(t, s.address(t, netip.MustParseAddr("127.0.0.1")))
	want := `{"Address":"http://127.0.0.1:8096","Id":"` + serverID + `","Name":"Polyfin","EndpointAddress":null}`
	for _, message := range []string{"who is JellyfinServer?", "who is EmbyServer?", "WHO IS JELLYFINSERVER?", "who is embyserver?",
		"hello, who is JellyfinServer? bye"} {
		send(t, conn, message)
		if got := receive(t, conn); got != want {
			t.Errorf("%q: got %s, want %s", message, got, want)
		}
	}
	// IPv6 apps get an IPv6 address, where the system has one.
	if ipv6, err := net.DialUDP("udp6", nil, net.UDPAddrFromAddrPort(s.address(t, netip.IPv6Loopback()))); err == nil {
		defer ipv6.Close()
		if _, err := ipv6.Write([]byte("who is JellyfinServer?")); err != nil {
			t.Fatal(err)
		}
		want := `{"Address":"http://[::1]:8096","Id":"` + serverID + `","Name":"Polyfin","EndpointAddress":null}`
		if got := receive(t, ipv6); got != want {
			t.Errorf("IPv6: got %s, want %s", got, want)
		}
	}
}

// Other messages get nothing, and a datagram holding both messages one
// answer.
func TestIgnoresOtherMessages(t *testing.T) {
	s := start(t, 0)
	to := s.address(t, netip.MustParseAddr("127.0.0.1"))
	other, marker := client(t, to), client(t, to)
	for _, message := range []string{"", "who is JellyfinServer", "who is PlexServer?", "who is Jellyfin Server?", "hello",
		"who is JellyfinServer? who is EmbyServer?"} {
		send(t, other, message)
	}
	send(t, marker, "who is JellyfinServer?")
	receive(t, marker)
	if got := receive(t, other); got == "" {
		t.Error("no answer to the datagram holding both messages")
	}
	nothingPending(t, other)
}

// Only senders on the local network are answered: private IPv4 ranges,
// IPv6 unique local addresses, link-local addresses and loopback, IPv4
// alike when mapped to IPv6; public addresses, shared address space
// included, get nothing.
func TestAnswersOnlyTheLocalNetwork(t *testing.T) {
	s := &Service{options: Options{HTTP: &net.TCPAddr{Port: 8096}, ServerID: serverID, Settings: newFakeSettings().get,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), networks: func() ([]netip.Prefix, error) { return nil, nil },
		route: func(netip.Addr) (netip.Addr, error) { return netip.MustParseAddr("192.168.1.10"), nil }}}
	message := []byte("who is JellyfinServer?")
	for _, sender := range []string{"192.168.1.20:40000", "10.1.2.3:40000", "172.16.0.9:40000", "169.254.3.4:40000", "[fd12::5]:40000",
		"[fe80::1%eth0]:40000", "127.0.0.1:40000", "[::1]:40000", "[::ffff:192.168.1.20]:40000"} {
		if s.answer(message, netip.MustParseAddrPort(sender)) == nil {
			t.Errorf("%s: no answer", sender)
		}
	}
	for _, sender := range []string{"203.0.113.7:40000", "8.8.8.8:40000", "100.64.0.1:40000", "[2001:db8::1]:40000",
		"[::ffff:203.0.113.7]:40000", "192.168.1.20:0"} {
		if answer := s.answer(message, netip.MustParseAddrPort(sender)); answer != nil {
			t.Errorf("%s: %s", sender, answer)
		}
	}
}

// The setting gives the address announced, and the next message gets it
// without a restart; an HTTP server listening on one address announces
// that one.
func TestTheSettingChangesTheAddress(t *testing.T) {
	s := start(t, 0)
	conn := client(t, s.address(t, netip.MustParseAddr("127.0.0.1")))
	s.change(t, func(settings *Settings) { settings.Address, settings.Name = "http://192.168.1.10:8096", "Living room" })
	send(t, conn, "who is JellyfinServer?")
	want := `{"Address":"http://192.168.1.10:8096","Id":"` + serverID + `","Name":"Living room","EndpointAddress":null}`
	if got := receive(t, conn); got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	bound := start(t, 0, func(o *Options) { o.HTTP = &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 9000} })
	conn = client(t, bound.address(t, netip.MustParseAddr("127.0.0.1")))
	send(t, conn, "who is EmbyServer?")
	want = `{"Address":"http://10.0.0.5:9000","Id":"` + serverID + `","Name":"Polyfin","EndpointAddress":null}`
	if got := receive(t, conn); got != want {
		t.Errorf("listening on one address: got %s, want %s", got, want)
	}
}

// listen takes port as the service does, failing the test when it cannot.
func listen(t *testing.T, port int) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

// Turned off, discovery answers nothing and frees its port; turned on
// again, it listens anew, without a restart.
func TestTheSwitchSilencesTheAnswers(t *testing.T) {
	s := start(t, 0)
	to := s.address(t, netip.MustParseAddr("127.0.0.1"))
	s.change(t, func(settings *Settings) { settings.Enabled = false })
	if answer := s.answer([]byte("who is JellyfinServer?"), netip.MustParseAddrPort("127.0.0.1:40000")); answer != nil {
		t.Errorf("answered while off: %s", answer)
	}
	// The port is free: nothing listens on it any more.
	listen(t, int(to.Port())).Close()
	if status := s.Status(); status.Unavailable || status.Port != int(to.Port()) {
		t.Errorf("off: %+v", status)
	}
	s.change(t, func(settings *Settings) { settings.Enabled = true })
	conn := client(t, s.address(t, netip.MustParseAddr("127.0.0.1")))
	send(t, conn, "who is JellyfinServer?")
	if got := receive(t, conn); got == "" {
		t.Error("no answer once on again")
	}
}

// A port another program uses does not stop the start: Status tells it,
// and a change of the settings tries the port again.
func TestATakenPortIsReported(t *testing.T) {
	taken := listen(t, 0)
	port := taken.LocalAddr().(*net.UDPAddr).Port
	s := start(t, port)
	if status := s.Status(); !status.Unavailable || status.Port != port {
		t.Fatalf("taken: %+v", status)
	}
	// Turned off, nothing is wrong.
	s.change(t, func(settings *Settings) { settings.Enabled = false })
	if status := s.Status(); status.Unavailable {
		t.Errorf("off: %+v", status)
	}
	taken.Close()
	s.change(t, func(settings *Settings) { settings.Enabled = true })
	if status := s.Status(); status.Unavailable {
		t.Errorf("freed: %+v", status)
	}
	conn := client(t, s.address(t, netip.MustParseAddr("127.0.0.1")))
	send(t, conn, "who is JellyfinServer?")
	if got := receive(t, conn); got == "" {
		t.Error("no answer once the port is free")
	}
}

// In a container, an announced address whose network does not hold the
// sender is the container's, which apps on the local network cannot
// reach: Status tells it until the setting gives the address. With host
// networking, or outside a container, nothing is told.
func TestStatusTellsAContainersAddress(t *testing.T) {
	type answered struct {
		inContainer bool
		local       string
		networks    []string
		sender      string
		want        string
	}
	for name, c := range map[string]answered{
		// Docker Desktop's forwarder sends from its own address.
		"bridge, forwarded": {true, "172.17.0.3", []string{"127.0.0.1/8", "172.17.0.3/16"}, "192.168.65.1", "http://172.17.0.3:8096"},
		// A published port keeps the app's address on Linux.
		"bridge, published": {true, "172.18.0.2", []string{"172.18.0.2/16"}, "192.168.1.20", "http://172.18.0.2:8096"},
		"host networking":   {true, "192.168.1.10", []string{"172.17.0.1/16", "192.168.1.10/24"}, "192.168.1.20", ""},
		"same host":         {true, "127.0.0.1", []string{"127.0.0.1/8"}, "127.0.0.1", ""},
		"IPv6 bridge":       {true, "fd00:dead::2", []string{"fd00:dead::2/64"}, "fd00:1::7", "http://[fd00:dead::2]:8096"},
		"outside":           {false, "172.17.0.3", []string{"172.17.0.3/16"}, "192.168.65.1", ""},
		"unknown interface": {true, "172.17.0.3", []string{"10.0.0.2/8"}, "192.168.65.1", ""},
	} {
		settings := newFakeSettings()
		s := &Service{options: Options{HTTP: &net.TCPAddr{Port: 8096}, ServerID: serverID, Settings: settings.get, InContainer: c.inContainer,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			route:  func(netip.Addr) (netip.Addr, error) { return netip.MustParseAddr(c.local), nil },
			networks: func() ([]netip.Prefix, error) {
				var networks []netip.Prefix
				for _, n := range c.networks {
					networks = append(networks, netip.MustParsePrefix(n))
				}
				return networks, nil
			}}}
		if s.answer([]byte("who is JellyfinServer?"), netip.AddrPortFrom(netip.MustParseAddr(c.sender), 40000)) == nil {
			t.Fatalf("%s: no answer", name)
		}
		if got := s.Status().ContainerAddress; got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
		if c.want == "" {
			continue
		}
		// The setting gives the address: nothing is wrong any more.
		settings.set(func(settings *Settings) { settings.Address = "http://192.168.1.10:8096" })
		if got := s.Status().ContainerAddress; got != "" {
			t.Errorf("%s, with the setting: %q", name, got)
		}
		s.answer([]byte("who is JellyfinServer?"), netip.AddrPortFrom(netip.MustParseAddr(c.sender), 40000))
		settings.set(func(settings *Settings) { settings.Address = "" })
		if got := s.Status().ContainerAddress; got != "" {
			t.Errorf("%s, after an answer with the setting: %q", name, got)
		}
	}
}
