package torrent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// testSettings are settings a test changes as it goes.
type testSettings struct {
	mu sync.Mutex
	s  Settings
}

func (t *testSettings) get() Settings {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.s
}

func (t *testSettings) set(change func(*Settings)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	change(&t.s)
}

func newEngine(t *testing.T, settings *testSettings, budget int64) *Engine {
	t.Helper()
	e, err := New(Config{Settings: settings.get, Dir: t.TempDir(), Budget: func() int64 { return budget },
		Linger: 50 * time.Millisecond, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	e.checkEvery = 100 * time.Millisecond
	t.Cleanup(e.Close)
	return e
}

func proxied(proxy *torrenttest.Proxy) *testSettings {
	return &testSettings{s: Settings{Enabled: true, Proxy: proxy.Address()}}
}

// readAll plays spec and reads its file whole.
func readAll(t *testing.T, e *Engine, spec Spec) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size()))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func twoFiles() (*torrenttest.Torrent, []byte, []byte) {
	small, large := torrenttest.Bytes(300<<10+123), torrenttest.Bytes(700<<10+45)
	return torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "extra.mkv", Data: small},
		torrenttest.File{Name: "a-movie.mkv", Data: large}), small, large
}

func TestPlaysTheFileAddonsName(t *testing.T) {
	t.Parallel()
	tor, small, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "someone", "secret")
	settings := proxied(proxy)
	settings.set(func(s *Settings) { s.ProxyUser, s.ProxyPassword = "someone", "secret" })
	e := newEngine(t, settings, 64<<20)

	zero := 0
	spec, err := NewSpec(tor.HashHex(), &zero, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	if got := readAll(t, e, spec); !bytes.Equal(got, small) {
		t.Fatalf("file 0: read %d bytes, not the file's", len(got))
	}
	spec.File = -1
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatalf("largest file: read %d bytes, not the file's", len(got))
	}
	// Every connection to the seeder and the tracker went through the
	// proxy.
	targets := proxy.Targets()
	if remotes := seeder.Remotes(); len(remotes) == 0 || len(remotes) != countPrefix(targets, "tcp "+seeder.Address().String()) {
		t.Fatalf("the seeder saw %d connections, the proxy relayed %v", len(remotes), targets)
	}
	if !slices.ContainsFunc(targets, func(target string) bool {
		return strings.HasPrefix(target, "tcp 127.0.0.1:") && target != "tcp "+seeder.Address().String()
	}) {
		t.Fatalf("the tracker was not reached through the proxy: %v", targets)
	}
}

func countPrefix(list []string, value string) int {
	n := 0
	for _, item := range list {
		if item == value {
			n++
		}
	}
	return n
}

func TestProxyWithoutUDPAssociatePlaysOverTCP(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	udpTracker := torrenttest.StartUDPTracker(t, seeder.Address())
	node := torrenttest.StartDHTNode(t, seeder.Address())
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)

	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + udpTracker.URL(), "tracker:" + tracker.URL(), "dht:" + node.Address()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatal("the largest file was not read whole")
	}
	if !e.Status(context.Background()).NoUDP {
		t.Fatal("Health does not tell the proxy offers no UDP ASSOCIATE")
	}
	stop()
	waitFor(t, "the torrent to stop", func() bool { return e.Status(context.Background()).Playing == 0 })
	// The torrent stopped: whatever it sent is in.
	if udpTracker.Announces() != 0 || node.Queries() != 0 {
		t.Fatalf("UDP was used without UDP ASSOCIATE: %d announces, %d DHT queries", udpTracker.Announces(), node.Queries())
	}
	for _, target := range proxy.Targets() {
		if strings.HasPrefix(target, "udp ") {
			t.Fatalf("the proxy relayed UDP: %v", proxy.Targets())
		}
	}
}

func TestProxyWithUDPAssociateUsesUDPTrackersAndDHT(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	udpTracker := torrenttest.StartUDPTracker(t, seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)

	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + udpTracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	if got := readAll(t, e, spec); !bytes.Equal(got, large) {
		t.Fatal("the file was not read from the UDP tracker's peer")
	}
	stop()
	if !slices.Contains(proxy.Targets(), "udp "+strings.TrimSuffix(strings.TrimPrefix(udpTracker.URL(), "udp://"), "/announce")) {
		t.Fatalf("the UDP tracker was not reached through the proxy: %v", proxy.Targets())
	}

	// The DHT, from a node the sources name.
	other := torrenttest.Build("Another Movie", 32<<10, torrenttest.File{Name: "b.mkv", Data: torrenttest.Bytes(200 << 10)})
	otherSeeder := torrenttest.Seed(t, other, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, otherSeeder.Address())
	spec, err = NewSpec(other.HashHex(), nil, []string{"dht:" + node.Address()})
	if err != nil {
		t.Fatal(err)
	}
	stop = e.Play(spec)
	defer stop()
	if got := readAll(t, e, spec); !bytes.Equal(got, other.Data) {
		t.Fatal("the file was not read from the DHT's peer")
	}
	if !slices.Contains(proxy.Targets(), "udp "+node.Address()) {
		t.Fatalf("the DHT node was not reached through the proxy: %v", proxy.Targets())
	}
}

func TestDHTStartsFromNodesPeersTell(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(100 << 10)})
	// The first seeder is known from the tracker, and tells a DHT node
	// that knows a second one.
	first := torrenttest.Seed(t, tor, "127.0.0.1")
	second := torrenttest.Seed(t, tor, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, second.Address())
	first.TellDHT(netip.MustParseAddrPort(node.Address()).Port())
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", first.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)

	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL(), "dht:" + tor.HashHex()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	readAll(t, e, spec)
	waitFor(t, "the second seeder to be found on the DHT", func() bool { return len(second.Remotes()) > 0 })
}

// A DHT lookup that found no peer, its node's answer lost, is made again
// soon rather than after dhtEvery: a torrent the DHT alone lists still
// starts within its StartTimeout.
func TestADHTLookupThatFoundNoPeerIsMadeAgainSoon(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, seeder.Address())
	node.Drop(1)
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"dht:" + node.Address()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
	if queries := node.Queries(); queries < 2 {
		t.Fatalf("the node was asked %d times", queries)
	}
}

func TestKillSwitch(t *testing.T) {
	t.Parallel()
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	settings := proxied(proxy)
	e := newEngine(t, settings, 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("no route", func(t *testing.T) {
		e := newEngine(t, &testSettings{s: Settings{Enabled: true}}, 64<<20)
		defer e.Play(spec)()
		if _, err := e.Open(context.Background(), spec); !errors.Is(err, ErrNoRoute) {
			t.Fatalf("got %v, want ErrNoRoute", err)
		}
	})

	t.Run("switch off", func(t *testing.T) {
		e := newEngine(t, &testSettings{s: Settings{Proxy: proxy.Address()}}, 64<<20)
		defer e.Play(spec)()
		if _, err := e.Open(context.Background(), spec); !errors.Is(err, ErrDisabled) {
			t.Fatalf("got %v, want ErrDisabled", err)
		}
	})

	// The proxy drops during a playback: the torrent stops, and nothing is
	// sent another way.
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
	proxy.Stop()
	if _, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size())); err == nil || !strings.Contains(err.Error(), "SOCKS5 proxy") {
		t.Fatalf("a read past the proxy's end gave %v", err)
	}
	if _, err := e.Open(ctx, spec); err == nil {
		t.Fatal("the torrent opened with the proxy down")
	}
	status := e.Status(ctx)
	if status.RouteError == nil || status.Failure == nil || status.Playing != 0 {
		t.Fatalf("Health does not tell the proxy is down: %+v", status)
	}
	// The torrent stopped: its peers all came through the proxy.
	if got, relayed := len(seeder.Remotes()), countPrefix(proxy.Targets(), "tcp "+seeder.Address().String()); got != relayed {
		t.Fatalf("the seeder saw %d connections, the proxy relayed %d", got, relayed)
	}
	// A playback with the proxy down starts nothing.
	before := len(seeder.Remotes())
	e.Play(spec)()
	if _, err := e.Open(ctx, spec); err == nil {
		t.Fatal("the torrent opened with the proxy down")
	}
	if got := len(seeder.Remotes()); got != before {
		t.Fatalf("the seeder saw %d new connections with the proxy down", got-before)
	}
}

func TestOnlyAPlaybackStartsATorrent(t *testing.T) {
	t.Parallel()
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Open(context.Background(), spec); !errors.Is(err, ErrNotPlaying) {
		t.Fatalf("got %v, want ErrNotPlaying", err)
	}
	stop := e.Play(spec)
	readAll(t, e, spec)
	stop()
	waitFor(t, "the torrent to stop", func() bool { return e.Status(context.Background()).Playing == 0 })
	waitFor(t, "the seeder's connections to close", func() bool { return seeder.Open() == 0 })
	waitFor(t, "the tracker to be told", func() bool { return slices.Contains(tracker.Announces(), "stopped") })
	if _, err := e.Open(context.Background(), spec); !errors.Is(err, ErrNotPlaying) {
		t.Fatalf("after the playback: got %v, want ErrNotPlaying", err)
	}
	if e.Usage() != 0 {
		t.Fatalf("the stopped torrent's pieces take %d bytes", e.Usage())
	}
}

func TestUploads(t *testing.T) {
	t.Parallel()
	for _, mbps := range []int{0, 1} {
		t.Run(map[int]string{0: "none at 0", 1: "within the limit"}[mbps], func(t *testing.T) {
			t.Parallel()
			tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(1 << 20)})
			seeder := torrenttest.Seed(t, tor, "127.0.0.1")
			leecher := torrenttest.Leech(t, tor, "127.0.0.1")
			tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address(), leecher.Address())
			proxy := torrenttest.StartProxy(t, true, "", "")
			settings := proxied(proxy)
			settings.set(func(s *Settings) { s.UploadMbps = mbps })
			e := newEngine(t, settings, 64<<20)
			spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			stop := e.Play(spec)
			readAll(t, e, spec)
			waitFor(t, "the leecher's connection", func() bool { return len(leecher.Remotes()) > 0 })
			if mbps > 0 {
				waitFor(t, "an upload", func() bool { return leecher.Received() >= 128<<10 })
			}
			received, elapsed := leecher.Received(), time.Since(started)
			// A second of the limit may pass at once, and the block being
			// sent.
			rate := int64(mbps) * 1_000_000 / 8
			if limit := int64(elapsed.Seconds()*float64(rate)) + rate + 32<<10; mbps > 0 && received > limit {
				t.Fatalf("%d bytes uploaded in %v, over the limit of %d", received, elapsed, limit)
			}
			stop()
			waitFor(t, "the leecher's connections to close", func() bool { return leecher.Open() == 0 })
			final := leecher.Received()
			if mbps == 0 && final != 0 {
				t.Fatalf("%d bytes uploaded at 0", final)
			}
			if mbps == 0 && e.Uploaded() != 0 {
				t.Fatalf("the engine counted %d bytes uploaded at 0", e.Uploaded())
			}
		})
	}
}

func TestPiecesStayWithinTheBudget(t *testing.T) {
	t.Parallel()
	const pieceLength = 32 << 10
	const budget = 4 * pieceLength
	data := torrenttest.Bytes(2 << 20)
	tor := torrenttest.Build("A Movie", pieceLength, torrenttest.File{Name: "a.mkv", Data: data})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), budget)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	buffer := make([]byte, 100<<10)
	for off := int64(0); off < r.Size(); off += int64(len(buffer)) {
		n, err := r.ReadAt(buffer, off)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if !bytes.Equal(buffer[:n], data[off:off+int64(n)]) {
			t.Fatalf("wrong bytes at %d", off)
		}
		if used := e.Usage(); used > budget {
			t.Fatalf("the pieces take %d bytes, over the budget of %d", used, budget)
		}
	}
}

func TestNewSpec(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("ab", 20)
	two := 2
	spec, err := NewSpec(hash, &two, []string{"tracker:udp://tracker.example:80", "tracker:http://tracker.example/announce",
		"tracker:ftp://nothing", "dht:" + hash, "dht:node.example:6881", "other"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.File != 2 || len(spec.Trackers) != 2 || !spec.DHT || !slices.Equal(spec.Nodes, []string{"node.example:6881"}) {
		t.Fatalf("got %+v", spec)
	}
	if _, err := NewSpec("nothex", nil, nil); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("got %v, want ErrInvalidSpec", err)
	}
}

// The pieces at a read come first, those after it next, then the start
// and end of the file read, for its index; the pieces held are not asked.
func TestPiecesAtTheReadComeFirst(t *testing.T) {
	t.Parallel()
	const length = 4 << 20
	e := newEngine(t, &testSettings{}, 64<<20)
	info := Info{PieceLength: length, Pieces: make([]Hash, 100), Length: 100 * length}
	file := File{Path: "a.mkv", Length: info.Length}
	tor := newTorrent(e, Spec{}, nil, false)
	tor.info, tor.have, tor.files = &info, make([]bool, 100), []File{file}
	reader := &Reader{t: tor, file: file}
	tor.reads[reader] = 50*length + 10
	tor.have[51] = true
	got := tor.wanted()
	if want := []int{50, 52, 53, 0, 99, 1, 98}; !slices.Equal(got, want) {
		t.Fatalf("wanted %v, want %v", got, want)
	}
}
