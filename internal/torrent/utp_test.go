package torrent

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// utpDeadline bounds a read in the uTP tests. uTP runs in Polyfin's own
// goroutines, acknowledgments included, where the kernel runs TCP's: on a
// loaded machine, under the race detector, its round trips slow down more
// than TCP's, and a read needs a wider margin than readAll's minute.
const utpDeadline = 3 * time.Minute

// readWithin plays spec and reads its file whole, within d.
func readWithin(t *testing.T, e *Engine, spec Spec, d time.Duration) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
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

// playsOverUTP plays a torrent whose only seeder is reachable over uTP
// alone, listed by a tracker at listed, and checks Health and the log tell
// the peer was reached over uTP.
func playsOverUTP(t *testing.T, settings *testSettings, tor *torrenttest.Torrent, seeder *torrenttest.Peer, listed netip.AddrPort) *Engine {
	t.Helper()
	logs := &records{}
	e := newEngineWith(t, settings, func(c *Config) { c.Logger = slog.New(logs) })
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", listed)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerURL(settings, tracker.URL())})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	if got := readWithin(t, e, spec, utpDeadline); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole over uTP")
	}
	if status := e.Status(context.Background()); status.Peers != 1 || status.UTPPeers != 1 {
		t.Fatalf("Health does not tell the peer was reached over uTP: %+v", status)
	}
	if !logs.logged("A torrent reached a peer", "over=uTP") {
		t.Fatal("the log does not tell the peer was reached over uTP")
	}
	if len(seeder.Remotes()) == 0 {
		t.Fatal("the uTP seeder saw no connection")
	}
	return e
}

// trackerURL is a tracker's URL as the route reaches it: through a
// WireGuard tunnel, at the address the tunnel forwards to the loopback.
func trackerURL(settings *testSettings, raw string) string {
	if settings.get().WireGuard != "" {
		return trackerName(raw, torrenttest.Public.String())
	}
	return raw
}

func utpTorrent() *torrenttest.Torrent {
	return torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(900<<10 + 77)})
}

func TestPlaysOverUTPThroughProxy(t *testing.T) {
	t.Parallel()
	tor := utpTorrent()
	seeder := torrenttest.SeedUTP(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, true, "", "")
	playsOverUTP(t, proxied(proxy), tor, seeder, seeder.Address())
	// The proxy relayed the uTP packets: the seeder saw them from its
	// relay, on the loopback.
	if !slices.Contains(proxy.Targets(), "udp "+seeder.Address().String()) {
		t.Fatalf("the uTP seeder was not reached through the proxy: %v", proxy.Targets())
	}
}

func TestPlaysOverUTPThroughWireGuard(t *testing.T) {
	t.Parallel()
	tor := utpTorrent()
	seeder := torrenttest.SeedUTP(t, tor, "127.0.0.1")
	server := torrenttest.StartWireGuardServer(t, nil)
	settings := &testSettings{s: Settings{Enabled: true, WireGuard: server.Config()}}
	playsOverUTP(t, settings, tor, seeder, torrenttest.Through(seeder.Address()))
	if !slices.Contains(server.Targets(), "udp "+torrenttest.Through(seeder.Address()).String()) {
		t.Fatalf("the uTP seeder was not reached through the tunnel: %v", server.Targets())
	}
}

// A peer reachable over both TCP and uTP gets one handshake: only the
// connections race, and a real client drops a second handshake with the
// same peer ID as a duplicate. As the torrent's first peer, both connect at
// once, and uTP, up about as soon as TCP, carries it.
func TestOneHandshakeAtATime(t *testing.T) {
	t.Parallel()
	tor := utpTorrent()
	seeder := torrenttest.SeedBoth(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readWithin(t, e, spec, utpDeadline); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole")
	}
	if got := seeder.Handshakes(); got != 1 {
		t.Fatalf("the peer got %d handshakes", got)
	}
	if status := e.Status(context.Background()); status.Peers != 1 {
		t.Fatalf("Health does not tell the peer: %+v", status)
	}
}

func TestProxyWithoutUDPAssociateLeavesUTPOff(t *testing.T) {
	t.Parallel()
	tor := utpTorrent()
	overUDP := torrenttest.SeedUTP(t, tor, "127.0.0.1")
	overTCP := torrenttest.Seed(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", overUDP.Address(), overTCP.Address())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the file was not read whole over TCP")
	}
	if status := e.Status(context.Background()); !status.NoUDP || status.UTPPeers != 0 {
		t.Fatalf("Health does not tell uTP is off: %+v", status)
	}
	stop()
	waitFor(t, "the torrent to stop", func() bool { return e.Status(context.Background()).Playing == 0 })
	if len(overUDP.Remotes()) != 0 || slices.ContainsFunc(proxy.Targets(), func(target string) bool { return strings.HasPrefix(target, "udp ") }) {
		t.Fatalf("uTP was tried without UDP ASSOCIATE: %v", proxy.Targets())
	}
}

// A uTP peer that drops off the network mid-transfer, sending nothing more,
// not even a FIN, is replaced: the blocks asked of it are asked of another
// peer, and the read goes on.
func TestLostUTPPeerIsReplaced(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(2<<20 + 99)})
	lost := torrenttest.SeedUTP(t, tor, "127.0.0.1")
	other := torrenttest.SeedUTP(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, true, "", "")
	settings := proxied(proxy)
	// At 2 Mbit/s, the file takes seconds: the peer is lost midway.
	settings.set(func(s *Settings) { s.DownloadMbps = 2 })
	e := newEngine(t, settings, 64<<20)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", lost.Address(), other.Address())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), utpDeadline)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// The playback reads on while the peer is lost.
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size()))
		read <- result{data, err}
	}()
	waitFor(t, "both peers to send", func() bool { return lost.Sent() > 0 && other.Sent() > 0 })
	if status := e.Status(ctx); status.UTPPeers != 2 {
		t.Fatalf("Health does not tell both peers were reached over uTP: %+v", status)
	}
	lost.Close()
	otherBefore := other.Sent()
	got := <-read
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !bytes.Equal(got.data, tor.Data) {
		t.Fatal("the file was not read whole once a peer was lost")
	}
	if other.Sent() <= otherBefore {
		t.Fatal("the other peer did not take over")
	}
}

// With the route down, nothing goes out over uTP either: the torrent stops,
// and a playback starts nothing.
func TestUTPStopsWithTheRoute(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"proxy", "wireguard"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			tor := utpTorrent()
			seeder := torrenttest.SeedUTP(t, tor, "127.0.0.1")
			listed := seeder.Address()
			var settings *testSettings
			var down func()
			switch route {
			case "proxy":
				proxy := torrenttest.StartProxy(t, true, "", "")
				settings, down = proxied(proxy), proxy.Stop
			case "wireguard":
				server := torrenttest.StartWireGuardServer(t, nil)
				settings, down = &testSettings{s: Settings{Enabled: true, WireGuard: server.Config()}}, server.Close
				listed = torrenttest.Through(listed)
			}
			settings.set(func(s *Settings) { s.DownloadMbps = 1 })
			e := newEngine(t, settings, 64<<20)
			e.tunnelFresh, e.tunnelWait = 6*time.Second, 5*time.Second
			tracker := torrenttest.HTTPTracker(t, "127.0.0.1", listed)
			spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + trackerURL(settings, tracker.URL())})
			if err != nil {
				t.Fatal(err)
			}
			stop := e.Play(spec)
			defer stop()
			ctx, cancel := context.WithTimeout(context.Background(), utpDeadline)
			defer cancel()
			r, err := e.Open(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if _, err := r.ReadAt(make([]byte, 1000), 0); err != nil {
				t.Fatal(err)
			}
			down()
			if _, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size())); err == nil {
				t.Fatal("the read went on with the route down")
			}
			waitFor(t, "the torrent to stop", func() bool { return e.Status(ctx).Playing == 0 })
			before := len(seeder.Remotes())
			e.Play(spec)()
			if _, err := e.Open(ctx, spec); err == nil {
				t.Fatal("the torrent opened with the route down")
			}
			if got := len(seeder.Remotes()); got != before {
				t.Fatalf("the uTP seeder saw %d new connections with the route down", got-before)
			}
		})
	}
}

// BenchmarkUTPSwarm measures what uTP brings on a local swarm reached
// through the SOCKS5 proxy: the time from a playback's start to its first
// byte, and the peers reached 3 seconds after it, with peers dialed over
// TCP alone and over uTP first. The mixed swarm has 4 seeders over TCP
// alone, 4 over uTP alone, and 4 addresses where nothing answers; the TCP
// swarm 4 seeders over TCP alone, where uTP only delays TCP. The numbers of
// docs/torrents.md come from
// go test -run '^$' -bench UTPSwarm -benchtime 5x ./internal/torrent.
func BenchmarkUTPSwarm(b *testing.B) {
	proxy := torrenttest.StartProxy(b, true, "", "")
	for _, swarm := range []string{"mixed", "tcp"} {
		for _, dial := range []string{"tcp", "utp"} {
			b.Run(swarm+"/"+dial, func(b *testing.B) {
				var first time.Duration
				var peers, overUTP int
				for b.Loop() {
					tor := torrenttest.Build("A Movie", 256<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(4 << 20)})
					var listed []netip.AddrPort
					for range 4 {
						listed = append(listed, torrenttest.Seed(b, tor, "127.0.0.1").Address())
						if swarm == "mixed" {
							listed = append(listed, torrenttest.SeedUTP(b, tor, "127.0.0.1").Address())
							gone := torrenttest.SeedUTP(b, tor, "127.0.0.1")
							gone.Close()
							listed = append(listed, gone.Address())
						}
					}
					tracker := torrenttest.HTTPTracker(b, "127.0.0.1", listed...)
					e, err := New(Config{Settings: proxied(proxy).get, Dir: b.TempDir(), Budget: func() int64 { return 64 << 20 },
						Logger: slog.New(slog.DiscardHandler)})
					if err != nil {
						b.Fatal(err)
					}
					e.utpOff = dial == "tcp"
					spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
					if err != nil {
						b.Fatal(err)
					}
					started := time.Now()
					stop := e.Play(spec)
					r, err := e.Open(context.Background(), spec)
					if err != nil {
						b.Fatal(err)
					}
					if _, err := r.ReadAt(make([]byte, 1), 0); err != nil {
						b.Fatal(err)
					}
					first += time.Since(started)
					time.Sleep(3 * time.Second)
					status := e.Status(context.Background())
					peers += status.Peers
					overUTP += status.UTPPeers
					r.Close()
					stop()
					e.Close()
				}
				b.ReportMetric(float64(first.Milliseconds())/float64(b.N), "ms-to-first-byte")
				b.ReportMetric(float64(peers)/float64(b.N), "peers")
				b.ReportMetric(float64(overUTP)/float64(b.N), "peers-over-uTP")
			})
		}
	}
}
