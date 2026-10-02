package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type database struct{ err error }

func (d database) Ping(context.Context) error { return d.err }

// owner answers with its name, so tests see which handler served a path.
func owner(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(name)) })
}

func handler(db Pinger) http.Handler {
	return New(Options{
		Database: db,
		Admin: fstest.MapFS{
			"index.html":         {Data: []byte("<!doctype html><title>Polyfin</title>")},
			"polyfin.svg":        {Data: []byte("<svg/>")},
			"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
		},
		AdminAPI: owner("admin api"),
		Jellyfin: owner("jellyfin"),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestAdminApplicationRouting(t *testing.T) {
	h := handler(database{})
	for _, tc := range []struct {
		target string
		status int
		body   string
		cache  string
	}{
		{"/admin/", http.StatusOK, "<!doctype html>", "no-cache"},
		{"/admin/users/42", http.StatusOK, "<!doctype html>", "no-cache"},
		{"/admin/assets/app-1a2b.js", http.StatusOK, "console.log", immutableCache},
		{"/admin/polyfin.svg", http.StatusOK, "<svg/>", "no-cache"},
		{"/admin/assets/app-old.js", http.StatusNotFound, "", ""},
	} {
		response := get(h, tc.target)
		if response.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.target, response.Code, tc.status)
			continue
		}
		if !strings.Contains(response.Body.String(), tc.body) {
			t.Errorf("%s: body %q does not contain %q", tc.target, response.Body, tc.body)
		}
		if tc.cache != "" && response.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: Cache-Control %q, want %q", tc.target, response.Header().Get("Cache-Control"), tc.cache)
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: no Content-Security-Policy", tc.target)
		}
	}
}

func TestRootAndBareAdminRedirectToTheApplication(t *testing.T) {
	h := handler(database{})
	for _, target := range []string{"/", "/admin"} {
		response := get(h, target)
		if response.Code/100 != 3 || response.Header().Get("Location") != "/admin/" {
			t.Errorf("%s: got %d to %q, want a redirect to /admin/", target, response.Code, response.Header().Get("Location"))
		}
	}
}

func TestReadinessFollowsTheDatabase(t *testing.T) {
	if code := get(handler(database{}), "/readyz").Code; code != http.StatusOK {
		t.Errorf("ready database: /readyz = %d", code)
	}
	down := handler(database{err: errors.New("connection refused")})
	if code := get(down, "/readyz").Code; code != http.StatusServiceUnavailable {
		t.Errorf("unreachable database: /readyz = %d", code)
	}
	// Liveness must not depend on the database, or a supervisor restarts a
	// healthy process while PostgreSQL is briefly away.
	if code := get(down, "/healthz").Code; code != http.StatusOK {
		t.Errorf("unreachable database: /healthz = %d", code)
	}
}

func TestPathsReachTheirOwner(t *testing.T) {
	h := handler(database{})
	for target, want := range map[string]string{
		"/admin/api/users":    "admin api",
		"/admin/api/missing":  "admin api",
		"/Users/Me":           "jellyfin",
		"/System/Info/Public": "jellyfin",
		"/web/index.html":     "jellyfin",
	} {
		if got := get(h, target).Body.String(); got != want {
			t.Errorf("%s served by %q, want %q", target, got, want)
		}
	}
}
