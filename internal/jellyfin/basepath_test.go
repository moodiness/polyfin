package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/basepath"
)

// Reached under /jellyfin, as through a reverse proxy that forwards only
// that path to Polyfin, the absolute addresses Polyfin hands back keep the
// base, as Jellyfin's carry its BaseUrl: a stream's Path, LocalAddress, and
// every address built on baseURL. At the root they have none.
func TestAddressesKeepTheBasePath(t *testing.T) {
	t.Parallel()
	p := playing(t)
	under := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.handler.ServeHTTP(w, r.WithContext(basepath.With(r.Context(), "/jellyfin")))
	}))
	t.Cleanup(under.Close)
	viaBase := p.testServer
	viaBase.url = under.URL

	for _, s := range []struct {
		server testServer
		want   string
	}{{p.testServer, p.url}, {viaBase, under.URL + "/jellyfin"}} {
		status, data := s.server.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{})
		var answer playbackInfoResponse
		if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil || len(answer.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo: %d %v %s", status, err, data)
		}
		if path := answer.MediaSources[0].Path; !strings.HasPrefix(path, s.want+"/Videos/"+p.movie+"/stream.") {
			t.Errorf("Path %s, want it under %s", path, s.want)
		}
		status, data = s.server.call(http.MethodGet, "/System/Info/Public", "", nil)
		var info PublicSystemInfo
		if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil {
			t.Fatalf("/System/Info/Public: %d %v %s", status, err, data)
		}
		if info.LocalAddress != s.want {
			t.Errorf("LocalAddress %q, want %q", info.LocalAddress, s.want)
		}
	}

	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8096/Items/1/PlaybackInfo", nil)
	r.Header.Set("X-Forwarded-Host", "media.example.org")
	r.Header.Set("X-Forwarded-Proto", "https")
	r = r.WithContext(basepath.With(r.Context(), "/JellyFin"))
	if got := baseURL(r); got != "https://media.example.org/JellyFin" {
		t.Errorf("baseURL %q, want the base as the app wrote it", got)
	}
}
