package admin

import (
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"testing"

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
