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
	"slices"
	"strconv"
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

// On a title's page, the script asks Polyfin every second for the title's
// versions while addons are pending. When the page lists fewer versions than
// there are, or a different number once none is pending, it asks for the
// title's details and lists their versions in the version menu, in place,
// keeping the version picked; it reloads the page as jellyfin-web reloads a
// page shown again when the version picked is gone or no longer the same.
// It runs in Node.js, in a context that stands for the browser and
// jellyfin-web: a title page whose version menu lists what the details gave
// when it was last loaded, a clock and timers the harness moves on.
func TestTitlePagesAddVersionsAsAddonsAnswer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		id      = "0123456789abcdef0123456789abcdef"
		title   = "#/details?id=" + id + "&serverId=fedcba9876543210fedcba9876543210"
		ask     = "ask http://polyfin.test/Polyfin/Items/" + id + "/Versions"
		details = "details t"
	)
	// A source is a version as the title's details list it.
	type source struct {
		Id, Name string
		Size     int64 `json:",omitempty"`
	}
	// Without Sources, an answer's details list Count versions as Polyfin
	// names them: the first under the title's own identifier, then v1, v2…,
	// named "Version 0", "Version 1"…
	type answer struct {
		Pending, Count int
		Sources        []source `json:",omitempty"`
	}
	type scenario struct {
		// The menu lists the first listed versions, or options, with picked
		// picked, the first by default. jellyfin-web hides it with one
		// version, and empties it for an item it cannot play (unplayable).
		listed     int
		options    []source
		picked     string
		unplayable bool
		noMenu     bool
		video      bool
		// answers are Polyfin's, in order; then it fails, unless forever
		// keeps the last.
		answers []answer
		forever bool
		// steps: a route the router moves to; "tick" runs the next timer,
		// logging "idle" when there is none; "focus" and "blur" move focus
		// to and from the version menu; "pick <id>" picks a version as a
		// user does, which jellyfin-web answers with the version's tracks
		// ("tracks <id>"), or an error for a version missing from its own
		// copy of the versions; "menu" logs the menu: its versions, "*"
		// before the one picked, then whether it is shown; "push <pending>
		// <count>" has Polyfin push the title's progress on the socket,
		// details then listing count versions, and "push other" another
		// title's; "clock" logs the time, in seconds. The title's own
		// identifier is written "t".
		steps []string
		want  []string
	}
	reloads := []string{"viewbeforehide", "viewshow false"}
	ticks := func(n int) []string { return slices.Repeat([]string{"tick"}, n) }
	steps := func(n int, more ...string) []string { return slices.Concat([]string{title}, ticks(n), more) }
	versions := func(sizes ...int64) []source {
		list := make([]source, len(sizes))
		for i, size := range sizes {
			list[i] = source{"v" + strconv.Itoa(i), "Version " + strconv.Itoa(i), size}
		}
		list[0].Id = id
		return list
	}
	sized := versions(100, 200, 300)
	grown := append(versions(100, 250, 300), source{"v3", "Version 3", 0})
	scenarios := map[string]scenario{
		// From the one version a hidden menu lists, the menu shows them all.
		"versions come as addons answer": {listed: 1, answers: []answer{{2, 1, nil}, {1, 2, nil}, {0, 4, nil}},
			steps: steps(4, "menu"),
			want:  []string{ask, ask, details, ask, details, "idle", "menu *t v1 v2 v3 shown"}},
		// An addon asked again lists more: they join the versions listed,
		// the one picked staying picked. jellyfin-web plays a version added
		// once it has reloaded the page's details, and plays the others as
		// it did.
		"a follow-up adds versions": {listed: 2, picked: "v1", answers: []answer{{1, 2, nil}, {1, 2, nil}, {1, 4, nil}, {0, 4, nil}},
			steps: steps(5, "menu", "pick v3", "pick t"),
			want: slices.Concat([]string{ask, ask, ask, details, ask, "idle", "menu t *v1 v2 v3 shown"},
				reloads, []string{"tracks v3", "tracks t"})},
		"a known title is asked three times": {listed: 2, answers: []answer{{0, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: steps(4), want: []string{ask, ask, ask, "idle"}},
		"leaving the page stops": {listed: 1, answers: []answer{{2, 1, nil}, {2, 1, nil}},
			steps: []string{title, "tick", "#/home", "tick"}, want: []string{ask, "idle"}},
		"other routes ask nothing": {listed: 1, answers: []answer{{2, 3, nil}},
			steps: []string{"#/details?id=dashboard", "#/home", "#/list?parentId=" + id, "tick"}, want: []string{"idle"}},
		"never during a video": {listed: 1, video: true, answers: []answer{{1, 1, nil}, {1, 3, nil}, {0, 3, nil}},
			steps: steps(4), want: []string{ask, ask, ask, "idle"}},
		// A menu that cannot list the versions is reloaded once for each
		// count, not every second.
		"an empty menu counts one": {unplayable: true, answers: []answer{{1, 1, nil}, {1, 2, nil}, {0, 2, nil}},
			steps: steps(4), want: slices.Concat([]string{ask, ask, details}, reloads, []string{ask, "idle"})},
		"a page without a version menu is reloaded": {noMenu: true, answers: []answer{{1, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: steps(4), want: slices.Concat([]string{ask}, reloads, []string{ask, ask, "idle"})},
		"an error stops": {listed: 1, steps: steps(2), want: []string{ask, "idle"}},
		// The track menus describe the version picked: when it is gone, the
		// page is reloaded, and jellyfin-web picks the first version.
		"the version picked is gone": {listed: 2, picked: "v1",
			answers: []answer{{0, 3, []source{{id, "Version 0", 0}, {"v2", "Version 2", 0}, {"v3", "Version 3", 0}}}}, forever: true,
			steps: steps(4, "menu"),
			want:  slices.Concat([]string{ask, details}, reloads, []string{ask, ask, "idle", "menu *t v2 v3 shown"})},
		// The title's own identifier now names another version.
		"the version picked is renamed": {listed: 2,
			answers: []answer{{0, 3, []source{{id, "Version 9", 0}, {"v1", "Version 1", 0}, {"v2", "Version 2", 0}}}}, forever: true,
			steps: steps(4, "menu"),
			want:  slices.Concat([]string{ask, details}, reloads, []string{ask, ask, "idle", "menu *t v1 v2 shown"})},
		// Until a version is known, details list a placeholder named after
		// the title under its own identifier, which the first version takes.
		"the placeholder gives way to the first version": {options: []source{{id, "Movie", 0}},
			answers: []answer{{1, 1, []source{{id, "Movie", 0}}}, {1, 3, nil}, {0, 3, nil}},
			steps:   steps(4, "menu"),
			want:    slices.Concat([]string{ask, ask, details}, reloads, []string{ask, "idle", "menu *t v1 v2 shown"})},
		// The version picked, listed by the script, has another size now:
		// the title's identifier names another version under the same name.
		"the version picked changes size": {listed: 2, picked: "v1",
			answers: []answer{{1, 3, sized}, {1, 4, grown}, {0, 4, grown}},
			steps:   steps(4),
			want:    slices.Concat([]string{ask, details, ask, details}, reloads, []string{ask, "idle"})},
		// The menu may be open while it has focus: the versions wait for it
		// to lose focus, even once Polyfin is no longer asked.
		"not while the menu has focus": {listed: 2, picked: "v1", answers: []answer{{1, 3, nil}, {0, 3, nil}, {0, 3, nil}},
			steps: slices.Concat([]string{title, "focus"}, ticks(4), []string{"menu", "blur", "menu", "tick"}),
			want:  []string{ask, ask, ask, "idle", "menu t *v1 shown", details, "menu t *v1 v2 shown", "idle"}},
		// Once no addon is pending, the menu lists the versions details
		// list, even fewer.
		"no addon pending with another count": {listed: 3, answers: []answer{{1, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: steps(4, "menu"),
			want:  []string{ask, ask, details, ask, "idle", "menu *t v1 shown"}},
		// A push is taken as an answer at once; the script then asks only
		// every 10 seconds, as long as addons are pending.
		"pushed versions come at once": {listed: 1, answers: []answer{{1, 1, nil}, {1, 3, nil}, {0, 3, nil}},
			steps: []string{title, "tick", "push 1 3", "menu", "tick", "clock", "tick", "clock", "tick"},
			want:  []string{ask, details, "menu *t v1 v2 shown", ask, "clock 2", ask, "clock 12", "idle"}},
		"a push for another title changes nothing": {listed: 1, answers: []answer{{1, 1, nil}, {1, 1, nil}, {0, 1, nil}},
			steps: []string{title, "push other", "tick", "tick", "tick", "clock", "tick", "menu"},
			want:  []string{ask, ask, ask, "clock 3", "idle", "menu *t hidden"}},
		// Pushes still come once the script no longer asks.
		"a push after the last answer": {listed: 2, answers: []answer{{0, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: []string{title, "tick", "tick", "tick", "tick", "push 0 3", "menu"},
			want:  []string{ask, ask, ask, "idle", details, "menu *t v1 v2 shown"}},
	}
	ninety := slices.Repeat([]string{ask}, 90)
	scenarios["90 seconds at most"] = scenario{listed: 1, answers: []answer{{1, 1, nil}}, forever: true,
		// The 91st second asks nothing, and the polling ends.
		steps: append([]string{title}, ticks(92)...), want: append(ninety, "idle")}

	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
const TITLE = '0123456789abcdef0123456789abcdef'
const short = (id) => (id === TITLE ? 't' : id)
const versions = (n) => Array.from({ length: n }, (_, i) => ({ Id: i ? 'v' + i : TITLE, Name: 'Version ' + i }))
async function run(scenario) {
  const log = []
  let now = 0
  const timers = []
  const listeners = {}
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: (url) => log.push('replace ' + url) }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  class CustomEvent { constructor(type, init) { this.type = type; this.detail = init && init.detail } }
  class Event { constructor(type) { this.type = type } stopPropagation() { this.stopped = true } }
  // Changes to the menu's options reach its observers once the code that
  // made them is done.
  const observers = new Set()
  let pending = false
  const changed = () => {
    if (pending) return
    pending = true
    queueMicrotask(() => {
      pending = false
      for (const observer of [...observers]) if (observers.has(observer)) observer.callback([])
    })
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe() { observers.add(this) }
    disconnect() { observers.delete(this) }
  }
  // What the title's details list now, and jellyfin-web's own copy of the
  // versions it listed last.
  let sources = scenario.options || versions(scenario.listed)
  let copy = []
  let focused = null
  const container = { hidden: false, classList: { toggle: (name, force) => { if (name === 'hide') container.hidden = force } } }
  const menu = {
    children: [],
    get options() { return this.children },
    get selectedIndex() { return this.children.findIndex((option) => option.selected) },
    get value() { const option = this.children.find((option) => option.selected); return option ? option.value : '' },
    set value(value) { for (const option of this.children) option.selected = option.value === value },
    matches: (selector) => selector === '.selectSource',
    closest: (selector) => {
      if (selector === '.hide') return scenario.unplayable || container.hidden ? {} : null
      if (selector === '.selectSourceContainer') return container
      return selector === '.itemDetailPage' ? page : null
    },
    insertBefore(option, before) {
      if (this.children.includes(option)) this.children.splice(this.children.indexOf(option), 1)
      this.children.splice(before ? this.children.indexOf(before) : this.children.length, 0, option)
      changed()
    },
    removeChild(option) { this.children.splice(this.children.indexOf(option), 1); changed() },
    dispatchEvent: (event) => dispatch(event),
  }
  // jellyfin-web fills the menu from the details, keeping the version picked
  // if listed, else the first; it hides it with one version and lists none
  // for an item it cannot play.
  function fill(picked) {
    copy = scenario.unplayable ? [] : sources
    menu.children = copy.map((source) => ({ value: source.Id, textContent: source.Name, selected: false }))
    menu.value = copy.some((source) => source.Id === picked) ? picked : copy.length ? copy[0].Id : ''
    container.hidden = copy.length < 2
    changed()
  }
  fill(scenario.picked || (sources[0] || {}).Id)
  // Events on the menu reach the window's capturing listeners first, then
  // jellyfin-web's, which fills the track menus from its own copy.
  function dispatch(event) {
    event.target = menu
    for (const listener of listeners[event.type] || []) listener(event)
    if (event.stopped || event.type !== 'change') return
    log.push(copy.some((source) => source.Id === menu.value) ? 'tracks ' + short(menu.value) : 'error')
  }
  const page = {
    querySelector: (selector) => (selector === '.selectSource' && !scenario.noMenu ? menu : null),
    dispatchEvent: (event) => {
      log.push(event.type + (event.detail && 'isRestored' in event.detail ? ' ' + event.detail.isRestored : ''))
      // Reloaded, the page lists what the details give now.
      if (event.type === 'viewshow') fill(menu.value)
    },
  }
  const document = {
    get activeElement() { return focused },
    createElement: (tag) => (tag === 'option' ? { value: '', textContent: '', selected: false } : null),
    querySelector: (selector) => {
      if (selector === '.videoPlayerContainer') return scenario.video ? {} : null
      return selector === '.page.itemDetailPage:not(.hide)' ? page : null
    },
  }
  const answers = (scenario.answers || []).slice()
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => 'user',
    getJSON: (url) => {
      log.push('ask ' + url)
      const answer = scenario.forever && answers.length === 1 ? answers[0] : answers.shift()
      if (!answer) return Promise.reject(new Error('down'))
      sources = answer.Sources || versions(answer.Count)
      return Promise.resolve({ Pending: answer.Pending, Count: answer.Count })
    },
    getItem: (user, id) => {
      log.push('details ' + short(id) + (user === 'user' ? '' : ' for ' + user))
      return Promise.resolve({ Id: id, MediaSources: sources.map((source) => ({ ...source })) })
    },
  }
  vm.runInNewContext(script, {
    location, history, URL, document, CustomEvent, Event, MutationObserver, window: { ApiClient },
    Date: { now: () => now },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: (id) => { if (id) timers[id - 1] = null },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  for (const step of scenario.steps) {
    if (step === 'focus') focused = menu
    else if (step === 'blur') {
      focused = null
      dispatch(new Event('blur'))
    } else if (step.startsWith('pick ')) {
      const id = step.slice(5)
      menu.value = id === 't' ? TITLE : id
      dispatch(new Event('change'))
    } else if (step === 'menu') {
      const options = menu.children.map((option) => (option.selected ? '*' : '') + short(option.value))
      log.push(['menu', ...options, container.hidden ? 'hidden' : 'shown'].join(' '))
    } else if (step === 'clock') log.push('clock ' + now / 1000)
    else if (step.startsWith('push ')) {
      // As jellyfin-web's events trigger ApiClient's message event.
      const [, pending, count] = step.split(' ')
      const data = pending === 'other' ? { ItemId: 'f'.repeat(32), Pending: 0, Count: 5 } : { ItemId: TITLE.toUpperCase(), Pending: +pending, Count: +count }
      if (pending !== 'other') sources = versions(+count)
      const message = { MessageType: 'PolyfinVersions', MessageId: 'm', Data: data }
      for (const callback of ((ApiClient._callbacks || {}).message || []).slice()) callback.apply(ApiClient, [{ type: 'message' }, message])
    } else if (step !== 'tick') history.pushState(null, '', step)
    else {
      let next = -1
      timers.forEach((timer, i) => { if (timer && (next < 0 || timer.at < timers[next].at)) next = i })
      if (next < 0) log.push('idle')
      else {
        const timer = timers[next]
        timers[next] = null
        now = timer.at
        timer.run()
      }
    }
    await flush()
  }
  return log
}
;(async () => {
  const results = {}
  for (const [name, scenario] of Object.entries(scenarios)) results[name] = await run(scenario)
  process.stdout.write(JSON.stringify(results))
})()
`
	type input struct {
		Listed     int      `json:"listed"`
		Options    []source `json:"options,omitempty"`
		Picked     string   `json:"picked,omitempty"`
		Unplayable bool     `json:"unplayable"`
		NoMenu     bool     `json:"noMenu"`
		Video      bool     `json:"video"`
		Answers    []answer `json:"answers"`
		Forever    bool     `json:"forever"`
		Steps      []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.listed, s.options, s.picked, s.unplayable, s.noMenu, s.video, s.answers, s.forever, s.steps}
	}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": inputs})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(data))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string][]string
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	for name, s := range scenarios {
		if got := results[name]; !slices.Equal(got, s.want) {
			t.Errorf("%s:\n got  %q\n want %q", name, got, s.want)
		}
	}
}
