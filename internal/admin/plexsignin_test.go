package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// fakePlexAccount answers as plex.tv and a Plex Media Server do a user's
// sign-in and import: a PIN Plex claims with the account token once
// claimed is closed, the account and the server it reaches, and the
// server's library, where the server's token for the account has played
// one movie. It records the tokens signed out.
type fakePlexAccount struct {
	tv, server string
	claimed    chan struct{}
	mu         sync.Mutex
	signedOut  []string
}

func newFakePlexAccount(t *testing.T) *fakePlexAccount {
	t.Helper()
	f := &fakePlexAccount{claimed: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/identity":
			_, _ = io.WriteString(w, `{"MediaContainer":{"machineIdentifier":"0a1b2c"}}`)
		case r.Header.Get("X-Plex-Token") != "member-server-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/library/sections":
			_, _ = io.WriteString(w, `{"MediaContainer":{"Directory":[{"key":"1","type":"movie"}]}}`)
		case r.URL.Path == "/library/sections/1/all":
			_, _ = io.WriteString(w, `{"MediaContainer":{"totalSize":1,"Metadata":[{"ratingKey":"11","type":"movie","title":"One",`+
				`"guid":"plex://movie/11","Guid":[{"id":"imdb://tt0000001"}],"viewCount":1,"lastViewedAt":1788000000}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	f.server = server.URL
	tv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		token := r.Header.Get("X-Plex-Token")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/pins":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":7,"code":"c0de","expiresIn":900}`)
		case r.URL.Path == "/api/v2/pins/7":
			select {
			case <-f.claimed:
				_, _ = io.WriteString(w, `{"id":7,"authToken":"member-account-token"}`)
			default:
				_, _ = io.WriteString(w, `{"id":7,"authToken":null}`)
			}
		case token != "member-account-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/api/v2/user":
			_, _ = io.WriteString(w, `{"uuid":"u-member","title":"Member"}`)
		case r.URL.Path == "/api/v2/resources":
			_, _ = io.WriteString(w, `[{"name":"Shared","clientIdentifier":"0a1b2c","provides":"server","owned":false,`+
				`"accessToken":"member-server-token","connections":[{"uri":"`+f.server+`","local":true}]}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/users/signout":
			f.mu.Lock()
			f.signedOut = append(f.signedOut, token)
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/library/sections/watchlist/all":
			_, _ = io.WriteString(w, `{"MediaContainer":{"totalSize":0,"Metadata":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(tv.Close)
	f.tv = tv.URL
	return f
}

// A user signs in to Plex under My account, then imports their own watch
// data from the server their account reaches, into their account only.
// The tokens are neither answered nor kept: the import signs them out,
// and another user does not see the sign-in.
func TestPlexSignInImportsTheCallersOwnWatchData(t *testing.T) {
	t.Parallel()
	plex := newFakePlexAccount(t)
	var lib *library.Service
	var data *userdata.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib = library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		data = userdata.New(deps.pool)
		o.JellyfinImport = jellyfinimport.New(jellyfinimport.Options{Settings: o.Accounts.Settings, Titles: lib, UserData: data,
			Version: "1.2.3", Logger: logger, PlexTV: jellyfinimport.PlexTV{API: plex.tv, SignIn: plex.tv + "/auth", Discover: plex.tv}})
		t.Cleanup(o.JellyfinImport.Close)
	})
	member, other := api.signedIn("member", false), api.signedIn("other", false)

	status, body, _ := member.call(http.MethodPost, "/account/server-import/plex/sign-in", nil)
	if link, _ := body["link"].(string); status != http.StatusOK || !strings.HasPrefix(link, plex.tv+"/auth#?") || !strings.Contains(link, "code=c0de") {
		t.Fatalf("sign-in: %d %v", status, body)
	}
	close(plex.claimed)
	status, body, _ = member.call(http.MethodPost, "/account/server-import/plex/sign-in/poll", nil)
	servers, _ := body["servers"].([]any)
	if status != http.StatusOK || body["name"] != "Member" || len(servers) != 1 || servers[0].(map[string]any)["id"] != "0a1b2c" {
		t.Fatalf("signed in: %d %v", status, body)
	}
	if status, body, _ := other.call(http.MethodPost, "/account/server-import/plex/sign-in/poll", nil); status != http.StatusConflict ||
		body["error"] != "plex_sign_in_ended" {
		t.Errorf("another user: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/server-import", map[string]any{"kind": "plex", "server": "elsewhere"}); status != http.StatusBadRequest ||
		body["error"] != "plex_unknown_server" {
		t.Errorf("another server: %d %v", status, body)
	}
	status, started, _ := member.call(http.MethodPost, "/account/server-import", map[string]any{"kind": "plex", "server": "0a1b2c"})
	if status != http.StatusOK {
		t.Fatalf("import: %d %v", status, started)
	}
	deadline := time.Now().Add(10 * time.Second)
	var current map[string]any
	for {
		_, body, _ := member.call(http.MethodGet, "/account/server-import", nil)
		if current, _ = body["import"].(map[string]any); current != nil && current["state"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	users, _ := current["users"].([]any)
	if current["state"] != "done" || len(users) != 1 || users[0].(map[string]any)["played"] != 1.0 ||
		users[0].(map[string]any)["jellyfinName"] != "Member" {
		t.Errorf("import: %v", current)
	}
	for _, answer := range []map[string]any{body, started, current} {
		raw, _ := json.Marshal(answer)
		if strings.Contains(string(raw), "member-account-token") || strings.Contains(string(raw), "member-server-token") {
			t.Errorf("a token answered: %s", raw)
		}
	}
	plex.mu.Lock()
	signedOut := strings.Join(plex.signedOut, ",")
	plex.mu.Unlock()
	if signedOut != "member-account-token" {
		t.Errorf("signed out: %q", signedOut)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/server-import/plex/sign-in/poll", nil); status != http.StatusConflict ||
		body["error"] != "plex_sign_in_ended" {
		t.Errorf("after the import: %d %v", status, body)
	}
}
