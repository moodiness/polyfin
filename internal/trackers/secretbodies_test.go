package trackers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A token request, which holds the app's secret, a code or a refresh
// token, and a Last.fm write, which holds the session key, answered with a
// 307 to another host, here the same server named localhost rather than
// 127.0.0.1, fail rather than send their body there.
func TestTokenRequestsAndWritesAreNotSentAgainToAnotherHost(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		bodies []string
	)
	var elsewhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "127.0.0.1:") {
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	elsewhere = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	s := New(Options{Version: "1.2.3", URLs: map[string]string{Simkl: server.URL + "/simkl", LastFM: server.URL + "/lastfm"},
		AuthURLs: map[string]string{Trakt: server.URL + "/trakt", MyAnimeList: server.URL + "/mal"}})
	t.Cleanup(s.Close)
	settings := accounts.Settings{TraktClientID: "trakt-id", TraktClientSecret: "trakt-app-secret", LastFMAPIKey: "lastfm-key",
		LastFMSecret: "lastfm-secret"}

	ctx := t.Context()
	for name, send := range map[string]func() (reply, error){
		"Trakt": func() (reply, error) {
			return s.traktAuth(ctx, settings, "/oauth/token", traktApp(settings, map[string]string{"refresh_token": "secret-refresh-1"}))
		},
		"Simkl": func() (reply, error) {
			return s.form(ctx, "/oauth2/token", url.Values{"client_id": {"simkl-id"}, "refresh_token": {"secret-refresh-2"}})
		},
		"MyAnimeList": func() (reply, error) {
			return s.oauthForm(ctx, MyAnimeList, url.Values{"client_secret": {"mal-app-secret"}, "refresh_token": {"secret-refresh-3"}})
		},
		"Last.fm": func() (reply, error) {
			return s.lastFM(ctx, settings, http.MethodPost, url.Values{"method": {"track.scrobble"}, "sk": {"secret-session-4"}})
		},
	} {
		if r, err := send(); err == nil {
			t.Errorf("%s: answered %d through another host, want an error", name, r.status)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 0 {
		t.Errorf("the other host was sent %q", bodies)
	}
}
