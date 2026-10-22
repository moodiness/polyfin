package admin

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A proxy for torrents that does not answer is an error of System ›
// Health, which names it; one without UDP ASSOCIATE, a warning.
func TestHealthTellsWhyTorrentsDoNotPlay(t *testing.T) {
	t.Parallel()
	down := torrenttest.StartProxy(t, true, "", "")
	down.Stop()
	noUDP := torrenttest.StartProxy(t, false, "", "")
	for _, test := range []struct {
		proxy string
		want  map[string]any
	}{
		{down.Address(), map[string]any{"key": "torrents:route", "code": "torrent_route", "tone": "error",
			"to": "/settings/torrents#torrent-route", "transient": false, "failure": "proxy_unreachable", "name": down.Address()}},
		{noUDP.Address(), map[string]any{"key": "torrents:udp", "code": "torrent_udp", "tone": "warning",
			"to": "/settings/torrents#torrent-route", "transient": false, "name": noUDP.Address()}},
	} {
		api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
			settings := o.Accounts.Settings()
			settings.TorrentStreams, settings.TorrentProxy = true, test.proxy
			if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
				t.Fatal(err)
			}
			engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(o.Accounts.Settings()) },
				Dir: t.TempDir(), Budget: func() int64 { return 1 << 20 }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(engine.Close)
			o.Health = HealthSources{Torrents: engine}
		})
		administrator := api.signedIn("administrator", true)
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		if len(found) != 1 || !reflect.DeepEqual(found[0], test.want) {
			t.Fatalf("problems: %v", found)
		}
	}
}

// A torrent that did not start in time is a warning of System › Health,
// which tells why, until a torrent starts.
func TestHealthTellsATorrentDidNotStart(t *testing.T) {
	t.Parallel()
	proxy := torrenttest.StartProxy(t, true, "", "")
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	var engine *torrent.Engine
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		settings := o.Accounts.Settings()
		settings.TorrentStreams, settings.TorrentProxy = true, proxy.Address()
		if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
		var err error
		engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(o.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 1 << 20 }, StartTimeout: time.Second, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		o.Health = HealthSources{Torrents: engine}
	})
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(100 << 10)})
	spec, err := torrent.NewSpec(tor.HashHex(), nil, []string{"tracker:" + empty.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Play(spec)()
	var unstarted *torrent.StartError
	if _, err := engine.Open(t.Context(), spec); !errors.As(err, &unstarted) {
		t.Fatalf("got %v, want a StartError", err)
	}
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	want := map[string]any{"key": "torrents:start", "code": "torrent_start", "tone": "warning",
		"to": "/settings/torrents#torrent-route", "transient": false, "failure": "no_peers"}
	if len(found) != 1 || !reflect.DeepEqual(found[0], want) {
		t.Fatalf("problems: %v", found)
	}
}

// A torrent whose peers were found but did not connect has Health count
// them by why: here one refused the connection, and one closed it at the
// handshake.
func TestHealthCountsWhyATorrentsPeersDidNotConnect(t *testing.T) {
	t.Parallel()
	proxy := torrenttest.StartProxy(t, false, "", "")
	closing, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closing.Close() })
	go func() {
		for {
			conn, err := closing.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	refusing, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusing.Close()
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", netip.MustParseAddrPort(closing.Addr().String()),
		netip.MustParseAddrPort(refusing.Addr().String()))
	var engine *torrent.Engine
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		settings := o.Accounts.Settings()
		settings.TorrentStreams, settings.TorrentProxy = true, proxy.Address()
		if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
		var err error
		engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(o.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 1 << 20 }, StartTimeout: 10 * time.Second, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		o.Health = HealthSources{Torrents: engine}
	})
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(100 << 10)})
	spec, err := torrent.NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Play(spec)()
	var unstarted *torrent.StartError
	if _, err := engine.Open(t.Context(), spec); !errors.As(err, &unstarted) {
		t.Fatalf("got %v, want a StartError", err)
	}
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	want := map[string]any{"key": "torrents:start", "code": "torrent_start", "tone": "warning",
		"to": "/settings/torrents#torrent-route", "transient": false, "failure": "silent",
		"peers": map[string]any{"found": float64(2), "connected": float64(0), "refused": float64(1), "closed": float64(1)}}
	// The proxy offers no UDP ASSOCIATE, which Health tells too.
	if !slices.ContainsFunc(found, func(p any) bool { return reflect.DeepEqual(p, want) }) {
		t.Fatalf("problems: %v", found)
	}
}

// A WireGuard configuration is refused with its line, key and reason,
// never shown back once saved, only what it says without its keys; a
// tunnel whose key the server does not know is an error of System ›
// Health, which still answers meanwhile, backups on as by default.
func TestWireGuardSettingsAndHealth(t *testing.T) {
	t.Parallel()
	server := torrenttest.StartWireGuardServer(t, nil)
	private := torrenttest.NewPrivateKey()
	wrongKey := server.ConfigWithKey(private)
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		settings, dataDir := o.Accounts.Settings, o.DataDir
		o.Backups = backup.New(backup.Config{Folder: func() string { return settings().BackupDir(dataDir) }, DB: d.pool, Settings: settings,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(o.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 1 << 20 }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		o.Health = HealthSources{Torrents: engine}
	})
	administrator := api.signedIn("administrator", true)

	bad := strings.Replace(wrongKey, "Endpoint = 127.0.0.1:", "Endpoint = 127.0.0.1:x", 1)
	backups := t.TempDir()
	put := func(config string) map[string]any {
		return map[string]any{"serverName": "Polyfin", "language": "en", "torrentStreams": true, "torrentWireGuard": config,
			"backups": true, "backupFolder": backups}
	}
	status, body, _ := administrator.call(http.MethodPut, "/settings", put(bad))
	if status != http.StatusBadRequest || body["error"] != "invalid_wireguard_config" || body["key"] != "Endpoint" ||
		body["reason"] != "invalid" || body["line"] != float64(8) || strings.Contains(fmt.Sprint(body), private) {
		t.Fatalf("an invalid endpoint: %d %v", status, body)
	}
	status, body, _ = administrator.call(http.MethodPut, "/settings", put(wrongKey))
	if status != http.StatusOK {
		t.Fatalf("saving: %d %v", status, body)
	}
	_, body, _ = administrator.call(http.MethodGet, "/settings", nil)
	saved, _ := body["torrentWireGuardSaved"].(map[string]any)
	if _, sent := body["torrentWireGuard"]; sent || strings.Contains(fmt.Sprint(body), private) || saved == nil ||
		!strings.HasPrefix(saved["endpoint"].(string), "127.0.0.1:") || !reflect.DeepEqual(saved["addresses"], []any{"10.64.0.2/32"}) ||
		!reflect.DeepEqual(saved["dns"], []any{"10.64.0.1"}) {
		t.Fatalf("settings: %v", body)
	}

	// While the tunnel's server does not answer, Health still answers,
	// its route check given up at its own deadline, before the check
	// waited long enough to know the handshake failed. The check goes on
	// in the background: the requests that follow answer at once and tell
	// the same failure, rather than each waiting out the deadline, 2
	// seconds, for a check of its own. The admin app asks for Health and
	// the problems every few seconds.
	status, body, _ = administrator.call(http.MethodGet, "/health", nil)
	if torrents, _ := body["torrents"].(map[string]any); status != http.StatusOK || torrents["route"] != "wireguard" {
		t.Fatalf("Health while the tunnel does not answer: %d %v", status, body)
	}
	asked := time.Now()
	for range 3 {
		_, body, _ = administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		if len(found) != 1 {
			t.Fatalf("problems: %v", found)
		}
		problem, _ := found[0].(map[string]any)
		if problem["code"] != "torrent_route" || problem["failure"] != torrent.WireGuardHandshake || !strings.HasPrefix(problem["name"].(string), "127.0.0.1:") {
			t.Fatalf("problem: %v", problem)
		}
		_, body, _ = administrator.call(http.MethodGet, "/health", nil)
		torrents, _ := body["torrents"].(map[string]any)
		if torrents["route"] != "wireguard" || torrents["lastHandshake"] != nil || !reflect.DeepEqual(torrents["addresses"], []any{"10.64.0.2/32"}) {
			t.Fatalf("Health's torrents: %v", torrents)
		}
	}
	if took := time.Since(asked); took > 3*readyWait {
		t.Errorf("3 requests for the problems and 3 for Health took %v while the tunnel does not answer, each waiting for a check", took)
	}

	// With the right key, Health tells the last handshake.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", put(server.Config())); status != http.StatusOK {
		t.Fatalf("saving: %d %v", status, body)
	}
	_, body, _ = administrator.call(http.MethodGet, "/health", nil)
	torrents, _ := body["torrents"].(map[string]any)
	if torrents["route"] != "wireguard" || torrents["lastHandshake"] == nil || torrents["peers"] != float64(0) || torrents["utpPeers"] != float64(0) {
		t.Fatalf("Health's torrents: %v", torrents)
	}
}
