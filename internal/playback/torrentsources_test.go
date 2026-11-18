package playback

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A torrent version's sources are reached on public addresses only, as
// every other request made for a version of a member's own addon is: a
// tracker on the loopback, which a confined request does not reach, is
// not announced to, nor are a tracker and a DHT node on private
// addresses, and the version plays from its tracker on a public address.
// The same holds for a version that is not confined.
func TestATorrentVersionsSourcesAreOnPublicAddresses(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requests []string
	local := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
	}))
	t.Cleanup(local.Close)
	// The same address, asked for as a member's addon is: refused.
	confined, err := stremio.NewClient("test").Open(t.Context(), http.MethodGet, local.URL+"/internal", nil, true)
	if err == nil {
		confined.Body.Close()
		t.Fatal("a confined request reached the loopback: the test wants an address members may not reach")
	}

	for _, confined := range []bool{true, false} {
		data := torrenttest.Bytes(300 << 10)
		tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
		seeder := torrenttest.Seed(t, tor, "127.0.0.1")
		tracker := torrenttest.HTTPTracker(t, "127.0.0.1", torrenttest.Through(seeder.Address()))
		version := library.Version{Confined: confined, Torrent: &library.Torrent{InfoHash: tor.HashHex(), Sources: []string{
			"tracker:" + local.URL + "/internal", "tracker:udp://10.0.0.1:53/x", "dht:192.168.1.1:6881", "tracker:" + tracker.PublicURL()}}}
		spec, err := torrentSpec(version)
		if err != nil {
			t.Fatal(err)
		}
		proxy := torrenttest.StartProxy(t, true, "", "")
		engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return torrent.Settings{Enabled: true, Proxy: proxy.Address()} },
			Dir: t.TempDir(), Budget: func() int64 { return 16 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		stop := engine.Play(spec)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		reader, err := engine.Open(ctx, spec)
		if err != nil {
			t.Fatalf("confined %v: %v", confined, err)
		}
		got, err := io.ReadAll(io.NewSectionReader(reader, 0, reader.Size()))
		reader.Close()
		cancel()
		stop()
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("confined %v: read %d bytes, not the file's (%v)", confined, len(got), err)
		}
		if len(tracker.Announces()) == 0 {
			t.Errorf("confined %v: the tracker on a public address was not announced to", confined)
		}
		for _, target := range proxy.Targets() {
			_, address, _ := strings.Cut(target, " ")
			if ap, err := netip.ParseAddrPort(address); err != nil || ap.Addr() != torrenttest.Public {
				t.Errorf("confined %v: the route was asked for %q", confined, target)
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 0 {
		t.Errorf("the tracker on the loopback received %q", requests)
	}
}
