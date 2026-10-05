package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	webIndex = `<!doctype html><html dir="ltr"><head><meta charset="utf-8"><title>Jellyfin</title>` +
		`<script defer="defer" src="main.jellyfin.bundle.js?0123456789abcdef0123"></script></head><body></body></html>`
	webConfig = `{"multiserver": false, "servers": []}`
)

// webFiles stand for jellyfin-web: an index.html, bundles named with and
// without a hash, its config.json and a theme folder.
var webFiles = fstest.MapFS{
	"index.html":                         {Data: []byte(webIndex)},
	"main.jellyfin.bundle.js":            {Data: []byte("console.log('jellyfin')")},
	"1133.03edf3bb7ee048ee10be.css":      {Data: []byte("body{}")},
	"font.0123456789abcdef0123.woff2":    {Data: []byte("wOF2")},
	"config.json":                        {Data: []byte(webConfig)},
	"themes/dark/theme.css":              {Data: []byte("html{}")},
	"session-login.0123456789abcdef.htm": {Data: []byte("<html><head></head></html>")},
}

// setupState stands for the accounts store.
type setupState struct {
	required bool
	err      error
}

func (s setupState) SetupRequired(context.Context) (bool, error) { return s.required, s.err }

func webHandler(state setupState) http.Handler {
	return New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Polyfin</title>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           webFiles,
		SetupRequired: state.SetupRequired,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestRootOpensTheWebClientOnceSetUp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.Handler
		want    string
	}{
		{"set up, with a web client", webHandler(setupState{}), "/web/"},
		{"before setup", webHandler(setupState{required: true}), "/admin/"},
		{"database unavailable", webHandler(setupState{err: errors.New("connection refused")}), "/admin/"},
		{"without a web client", handler(database{}), "/admin/"},
	} {
		response := get(tc.handler, "/")
		if response.Code != http.StatusFound || response.Header().Get("Location") != tc.want {
			t.Errorf("%s: / answered %d to %q, want 302 to %s", tc.name, response.Code, response.Header().Get("Location"), tc.want)
		}
	}
}

func TestWebClientServesJellyfinWebFiles(t *testing.T) {
	h := webHandler(setupState{})
	for _, tc := range []struct {
		target      string
		body        string
		contentType string
		cache       string
	}{
		{"/web/", "<title>Jellyfin</title>", "text/html; charset=utf-8", "no-cache"},
		{"/web/index.html", "<title>Jellyfin</title>", "text/html; charset=utf-8", "no-cache"},
		// Bundles carry the build hash as their query, in index.html.
		{"/web/main.jellyfin.bundle.js?0123456789abcdef0123", "console.log", "text/javascript; charset=utf-8", immutableCache},
		{"/web/main.jellyfin.bundle.js", "console.log", "text/javascript; charset=utf-8", "no-cache"},
		{"/web/1133.03edf3bb7ee048ee10be.css", "body{}", "text/css; charset=utf-8", immutableCache},
		{"/web/font.0123456789abcdef0123.woff2", "wOF2", "font/woff2", immutableCache},
		{"/web/themes/dark/theme.css", "html{}", "text/css; charset=utf-8", "no-cache"},
		// Unmodified: no server listed and a single server, so that the
		// client connects to the one that served it.
		{"/web/config.json", webConfig, "application/json", "no-cache"},
		{"/web/polyfin.js", "location.replace", "text/javascript; charset=utf-8", "no-cache"},
	} {
		response := get(h, tc.target)
		if response.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.target, response.Code)
			continue
		}
		header := response.Header()
		if !strings.Contains(response.Body.String(), tc.body) {
			t.Errorf("%s: body %q does not contain %q", tc.target, response.Body, tc.body)
		}
		if header.Get("Content-Type") != tc.contentType {
			t.Errorf("%s: Content-Type %q, want %q", tc.target, header.Get("Content-Type"), tc.contentType)
		}
		if header.Get("Cache-Control") != tc.cache {
			t.Errorf("%s: Cache-Control %q, want %q", tc.target, header.Get("Cache-Control"), tc.cache)
		}
		// The admin app's policy would stop jellyfin-web from working.
		if policy := header.Get("Content-Security-Policy"); policy != "" {
			t.Errorf("%s: Content-Security-Policy %q", tc.target, policy)
		}
		if header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", tc.target)
		}
	}
	if body := get(h, "/web/config.json").Body.String(); body != webConfig {
		t.Errorf("config.json changed: %q", body)
	}
}

// What no file answers goes to the Jellyfin API, which has routes under
// /web/ too, as on Jellyfin.
func TestWebClientLeavesOtherPathsToJellyfin(t *testing.T) {
	h := webHandler(setupState{})
	for _, target := range []string{"/web/missing.0123456789abcdef0123.js", "/web/themes", "/web/ConfigurationPages", "/web/themes/"} {
		if response := get(h, target); response.Body.String() != "jellyfin" {
			t.Errorf("%s answered %d %q, want Jellyfin's handler", target, response.Code, response.Body)
		}
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/web/index.html", nil))
	if recorder.Body.String() != "jellyfin" {
		t.Errorf("POST /web/index.html answered %q, want Jellyfin's handler", recorder.Body)
	}
	// Jellyfin's answers.
	for _, tc := range []struct {
		target, location string
		status           int
	}{
		{"/web", "/web/", http.StatusMovedPermanently},
		{"/robots.txt", "/web/robots.txt", http.StatusFound},
	} {
		response := get(h, tc.target)
		if response.Code != tc.status || response.Header().Get("Location") != tc.location {
			t.Errorf("%s: %d to %q, want %d to %s", tc.target, response.Code, response.Header().Get("Location"), tc.status, tc.location)
		}
	}
}

func TestOnlyTheWebClientPageLoadsPolyfinScript(t *testing.T) {
	h := webHandler(setupState{})
	tag := `<script src="polyfin.js"></script>`
	for range 2 {
		for _, target := range []string{"/web/", "/web/index.html"} {
			body := get(h, target).Body.String()
			if strings.Count(body, tag) != 1 {
				t.Fatalf("%s: %d script tags in %s", target, strings.Count(body, tag), body)
			}
			// In the head, before jellyfin-web's deferred scripts run.
			if at := strings.Index(body, tag); at > strings.Index(body, "</head>") || !strings.HasPrefix(body, webIndex[:strings.Index(webIndex, "</head>")]) {
				t.Errorf("%s: script misplaced in %s", target, body)
			}
		}
	}
	if body := get(h, "/web/session-login.0123456789abcdef.htm").Body.String(); strings.Contains(body, "polyfin.js") {
		t.Errorf("another page got the script: %s", body)
	}
	if string(webFiles["index.html"].Data) != webIndex {
		t.Error("jellyfin-web's index.html was changed")
	}
	if page := string(withWebScripts([]byte("<p>Jellyfin</p>"), hashedScript{})); page != "<p>Jellyfin</p>"+tag {
		t.Errorf("a page without a head: %q", page)
	}
	// The page is revalidated, not downloaded again.
	first := get(h, "/web/")
	request := httptest.NewRequest(http.MethodGet, "/web/", nil)
	request.Header.Set("If-None-Match", first.Header().Get("ETag"))
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if first.Header().Get("ETag") == "" || recorder.Code != http.StatusNotModified {
		t.Errorf("revalidating /web/ with ETag %q: %d", first.Header().Get("ETag"), recorder.Code)
	}
}

// Before setup nobody can sign in to the web client: its page leads to the
// admin app, where setup is.
func TestWebClientPageLeadsToSetupFirst(t *testing.T) {
	h := webHandler(setupState{required: true})
	for _, target := range []string{"/web/", "/web/index.html"} {
		response := get(h, target)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin/" {
			t.Errorf("%s before setup: %d to %q", target, response.Code, response.Header().Get("Location"))
		}
	}
	if response := get(h, "/web/config.json"); response.Code != http.StatusOK {
		t.Errorf("config.json before setup: %d", response.Code)
	}
}

func TestWebClientNeedsJellyfinWebIndex(t *testing.T) {
	dir := t.TempDir()
	if WebClientFiles(filepath.Join(dir, "missing")) != nil || WebClientFiles(dir) != nil || WebClientFiles("") != nil {
		t.Fatal("a web client without jellyfin-web's index.html")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(webIndex), 0o600); err != nil {
		t.Fatal(err)
	}
	files := WebClientFiles(dir)
	if files == nil {
		t.Fatal("no web client with an index.html")
	}
	h := New(Options{
		Database: database{}, Admin: fstest.MapFS{}, AdminAPI: owner("admin api"), Jellyfin: owner("jellyfin"),
		Web: files, SetupRequired: setupState{}.SetupRequired, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if body := get(h, "/web/").Body.String(); !strings.Contains(body, `<script src="polyfin.js"></script>`) {
		t.Errorf("/web/ from a folder: %q", body)
	}
}

// The script sends jellyfin-web's administration routes to the admin app
// however the client gets to them: on load, through its router (history),
// a link (hashchange) or Back (popstate). A dashboard page with a match in the
// admin app opens that page; the others open its home. It runs in Node.js, in
// a context that stands for the browser.
func TestDashboardRoutesOpenTheAdminApp(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// The admin page each route opens, after /admin/; "-" for a route left alone.
	routes := map[string]string{
		"#/dashboard":       "",
		"#/dashboard/users": "users",
		"#/dashboard/users/profile?userId=0123456789ABCDEF0123456789abcdef":    "users/0123456789abcdef0123456789abcdef",
		"#/dashboard/users/access?userId=01234567-89ab-cdef-0123-456789abcdef": "users/0123456789abcdef0123456789abcdef",
		"#/dashboard/devices":                      "users",
		"#/dashboard/libraries/display":            "libraries",
		"#/dashboard/livetv":                       "live-tv",
		"#/dashboard/livetv/recordings":            "settings/recordings",
		"#/dashboard/playback/transcoding":         "settings/conversion",
		"#/dashboard/playback/resume":              "settings/content",
		"#/dashboard/playback/trickplay":           "settings/thumbnails",
		"#/dashboard/playback/streaming":           "settings/playback",
		"#/dashboard/branding":                     "settings/web-player",
		"#/dashboard/settings":                     "settings/general",
		"#/dashboard/backups":                      "settings/backups",
		"#/dashboard/logs":                         "system/logs",
		"#/dashboard/keys":                         "system/api-keys",
		"#/dashboard/plugins/0123/settings":        "",
		"#/Dashboard/Tasks":                        "system/schedule",
		"#/dashboard/tasks/edit?id=3":              "system/schedule",
		"#/dashboard/activity":                     "",
		"#/dashboard?tab=1":                        "",
		"#!/dashboard":                             "",
		"#!/dashboard/logs":                        "system/logs",
		"#/metadata":                               "",
		"#/configurationpage?name=Plugin":          "",
		"#/wizard/start":                           "",
		"":                                         "-",
		"#/home":                                   "-",
		"#/dashboards":                             "-",
		"#/metadatas":                              "-",
		"#/details?id=dashboard":                   "-",
		"#/search?query=dashboard":                 "-",
		"#/mypreferencesmenu":                      "-",
		"#/livetv":                                 "-",
		"#/list?parentId=0123&serverId=dashboard":  "-",
		"#/userprofile?userId=0123&from=dashboard": "-",
	}
	const harness = `
const vm = require('node:vm')
const { script, routes } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const results = {}
for (const route of routes) {
  for (const way of ['load', 'pushState', 'replaceState', 'hashchange', 'popstate']) {
    for (const page of ['http://polyfin.test/web/', 'http://polyfin.test/web/index.html']) {
      let replaced = ''
      const listeners = {}
      const location = { replace: (url) => { replaced = url } }
      const go = (url) => { const next = new URL(url, location.href); location.href = next.href; location.hash = next.hash }
      location.href = page
      go(way === 'load' ? route || page : '#/home')
      const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
      vm.runInNewContext(script, { location, history, URL, addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) } })
      if (way === 'pushState' || way === 'replaceState') history[way](null, '', route || page)
      if (way === 'hashchange' || way === 'popstate') { go(route || page); (listeners[way] || []).forEach((listener) => listener()) }
      results[way + ' ' + page + route] = replaced
    }
  }
}
process.stdout.write(JSON.stringify(results))
`
	names := make([]string, 0, len(routes))
	for route := range routes {
		names = append(names, route)
	}
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "routes": names})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string]string
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	if len(results) != len(routes)*5*2 {
		t.Fatalf("%d results for %d routes", len(results), len(routes))
	}
	for key, replaced := range results {
		route := ""
		if at := strings.Index(key, "#"); at >= 0 {
			route = key[at:]
		}
		want := ""
		if page := routes[route]; page != "-" {
			want = "http://polyfin.test/admin/" + page
		}
		if replaced != want {
			t.Errorf("%s: went to %q, want %q", key, replaced, want)
		}
	}
}
