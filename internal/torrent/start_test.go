package torrent

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// records is a log handler keeping every record's message and attributes.
type records struct {
	mu    sync.Mutex
	lines []string
}

func (r *records) Enabled(context.Context, slog.Level) bool { return true }

func (r *records) Handle(_ context.Context, record slog.Record) error {
	line := record.Message
	record.Attrs(func(a slog.Attr) bool {
		line += " " + a.Key + "=" + a.Value.String()
		return true
	})
	r.mu.Lock()
	r.lines = append(r.lines, line)
	r.mu.Unlock()
	return nil
}

func (r *records) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *records) WithGroup(string) slog.Handler      { return r }

// logged reports whether a line logged holds every part.
func (r *records) logged(parts ...string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.lines, func(line string) bool {
		for _, part := range parts {
			if !strings.Contains(line, part) {
				return false
			}
		}
		return true
	})
}

// newEngineWith is newEngine with change made to its config.
func newEngineWith(t *testing.T, settings *testSettings, change func(*Config)) *Engine {
	t.Helper()
	config := Config{Settings: settings.get, Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 },
		Linger: 50 * time.Millisecond, Logger: slog.New(slog.DiscardHandler)}
	change(&config)
	e, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	e.checkEvery = 100 * time.Millisecond
	t.Cleanup(e.Close)
	return e
}

// refusingHTTPTracker is an HTTP tracker refusing every announce for
// reason, as BEP 3 has trackers do.
func refusingHTTPTracker(t *testing.T, reason string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "d14:failure reason%d:%se", len(reason), reason)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/announce"
}

// refusingUDPTracker is a UDP tracker that connects, then refuses every
// announce for reason, as BEP 15 has trackers do.
func refusingUDPTracker(t *testing.T, reason string) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
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
			txid := buffer[12:16]
			if binary.BigEndian.Uint32(buffer[8:]) == udpActionConnect {
				reply := append(binary.BigEndian.AppendUint32(nil, udpActionConnect), txid...)
				_, _ = conn.WriteToUDPAddrPort(append(reply, 1, 2, 3, 4, 5, 6, 7, 8), from)
				continue
			}
			reply := append(binary.BigEndian.AppendUint32(nil, udpActionError), txid...)
			_, _ = conn.WriteToUDPAddrPort(append(reply, reason...), from)
		}
	}()
	return "udp://" + conn.LocalAddr().String() + "/announce"
}

// A tracker refusing the announce has its own reason logged, at debug,
// for HTTP and UDP trackers alike, without the tracker's path, which may
// hold a passkey; the torrent that then does not start in time says how
// its trackers answered, and Health tells it until a torrent starts.
func TestATrackerRefusalIsLoggedWithItsReason(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	byHTTP := refusingHTTPTracker(t, "unregistered torrent")
	byUDP := refusingUDPTracker(t, "invalid port")
	proxy := torrenttest.StartProxy(t, true, "", "")
	logs := &records{}
	e := newEngineWith(t, proxied(proxy), func(c *Config) {
		c.StartTimeout = 3 * time.Second
		c.Logger = slog.New(logs)
	})
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + byHTTP, "tracker:" + byUDP})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	waitFor(t, "the refusals to be logged", func() bool {
		return logs.logged("A tracker could not be reached", "tracker=127.0.0.1", "refused the announce: unregistered torrent") &&
			logs.logged("A tracker could not be reached", "refused the announce: invalid port")
	})
	if logs.logged("/announce") {
		t.Error("a tracker's path was logged")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err = e.Open(ctx, spec)
	var unstarted *StartError
	if !errors.As(err, &unstarted) || unstarted.Trackers != 2 || unstarted.Refused != 2 || unstarted.Answered != 0 || unstarted.Found != 0 {
		t.Fatalf("got %v, want a StartError telling both trackers refused", err)
	}
	if status := e.Status(ctx); status.Unstarted != unstarted || !logs.logged("A torrent did not start in time", "2 refused") {
		t.Fatalf("Health tells %v", status.Unstarted)
	}

	// The torrent starting once a tracker lists its seeder clears it.
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	other, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(other)()
	if got := readAll(t, e, other); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
	if status := e.Status(ctx); status.Unstarted != nil {
		t.Fatalf("Health still tells %v", status.Unstarted)
	}
}

// An announce escapes the info hash and the peer ID byte by byte, a space
// never written "+", which trackers reading the query's bytes take for
// itself; it tells a port of the dynamic range, as trackers refuse 0 and
// often 6881 to 6889.
func TestAnnounceEscapesItsBytesAndTellsAPortTrackersTake(t *testing.T) {
	t.Parallel()
	queries := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.RawQuery
		_, _ = w.Write([]byte("d8:intervali60e5:peers0:e"))
	}))
	defer server.Close()
	proxy := torrenttest.StartProxy(t, false, "", "")
	var hash Hash
	copy(hash[:], " +~%&=\x00\xff aZ09-._ xyz12")
	peerID := newPeerID()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a := announce{hash: hash, peerID: peerID, port: announcePort(peerID), event: "started", left: 1}
	if _, err := announceTo(ctx, ProxyRoute(proxy.Address(), "", ""), server.URL+"/announce?key=a+b", a, false); err != nil {
		t.Fatal(err)
	}
	query := <-queries
	if !strings.HasPrefix(query, "key=a+b&") {
		t.Errorf("the tracker's own query was changed: %s", query)
	}
	fields := map[string]string{}
	for field := range strings.SplitSeq(query, "&") {
		name, value, _ := strings.Cut(field, "=")
		fields[name] = value
	}
	// Read as raw bytes, as trackers that do not take "+" for a space do.
	raw := func(value string) string {
		var b []byte
		for i := 0; i < len(value); i++ {
			if value[i] == '%' && i+2 < len(value) {
				n, _ := strconv.ParseUint(value[i+1:i+3], 16, 8)
				b = append(b, byte(n))
				i += 2
				continue
			}
			b = append(b, value[i])
		}
		return string(b)
	}
	if got := raw(fields["info_hash"]); got != string(hash[:]) || strings.Contains(fields["info_hash"], "+") {
		t.Errorf("info_hash %q reads %q", fields["info_hash"], got)
	}
	if got := raw(fields["peer_id"]); got != string(peerID[:]) {
		t.Errorf("peer_id %q reads %q", fields["peer_id"], got)
	}
	if !strings.HasPrefix(string(peerID[:]), "-PF0001-") || strings.Trim(string(peerID[8:]), peerIDAlphabet) != "" {
		t.Errorf("peer ID %q", peerID)
	}
	port, err := strconv.Atoi(fields["port"])
	if err != nil || port < 49152 || port > 65535 {
		t.Errorf("port %q", fields["port"])
	}
}

// A torrent whose sources name no DHT node is looked up from the engine's
// bootstrap nodes over a route that carries UDP, and a torrent that names
// no tracker is looked up there too; over a proxy without UDP ASSOCIATE,
// no bootstrap node is asked, and the torrent that cannot start says the
// DHT was off for want of UDP.
func TestBootstrapNodesStartLookupsOverUDPOnly(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, seeder.Address())
	withUDP := torrenttest.StartProxy(t, true, "", "")
	e := newEngineWith(t, proxied(withUDP), func(c *Config) { c.Bootstrap = []string{node.Address()} })
	for _, sources := range [][]string{{"dht:" + tor.HashHex()}, nil} {
		spec, err := NewSpec(tor.HashHex(), nil, sources)
		if err != nil {
			t.Fatal(err)
		}
		stop := e.Play(spec)
		if got := readAll(t, e, spec); len(got) != len(large) {
			t.Fatalf("sources %q: read %d bytes", sources, len(got))
		}
		stop()
		waitFor(t, "the torrent to stop", func() bool { return !e.Running(spec.Hash) })
	}
	if node.Queries() == 0 || !slices.Contains(withUDP.Targets(), "udp "+node.Address()) {
		t.Fatalf("the bootstrap node was not asked through the proxy: %v", withUDP.Targets())
	}

	other := torrenttest.StartDHTNode(t, seeder.Address())
	withoutUDP := torrenttest.StartProxy(t, false, "", "")
	e = newEngineWith(t, proxied(withoutUDP), func(c *Config) {
		c.Bootstrap = []string{other.Address()}
		c.StartTimeout = time.Second
	})
	spec, err := NewSpec(tor.HashHex(), nil, []string{"dht:" + tor.HashHex()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err = e.Open(ctx, spec)
	var unstarted *StartError
	if !errors.As(err, &unstarted) || !unstarted.NoUDP || unstarted.DHT || !strings.Contains(err.Error(), "no DHT: the route carries no UDP") {
		t.Fatalf("got %v, want a StartError telling the DHT was off", err)
	}
	if other.Queries() != 0 || slices.ContainsFunc(withoutUDP.Targets(), func(target string) bool { return strings.HasPrefix(target, "udp ") }) {
		t.Fatalf("a bootstrap node was asked without UDP: %d queries, %v", other.Queries(), withoutUDP.Targets())
	}
}

// The bootstrap nodes are host names with their ports.
func TestBootstrapNodesAreNodes(t *testing.T) {
	t.Parallel()
	if len(BootstrapNodes) == 0 {
		t.Fatal("no bootstrap node")
	}
	for _, node := range BootstrapNodes {
		if !validNode(node) {
			t.Errorf("%q is not host:port", node)
		}
	}
}
