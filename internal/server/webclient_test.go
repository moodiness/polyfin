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
      // The page's head takes the style the script adds.
      const document = { head: { appendChild: () => {} }, createElement: () => ({}) }
      vm.runInNewContext(script, { location, history, URL, document, addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) } })
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

// The title of a collection library's "Recently Added" row on the home
// page, which jellyfin-web opens on Suggestions (#/mixed?…&tab=1), opens the
// library on the screen its user chose instead: the script drops the tab
// from the link as it is pressed or clicked, on the link or on the title
// within it, before jellyfin-web follows it. Other links keep theirs. It
// runs in Node.js, in a context that stands for the browser.
func TestRowTitlesOpenACollectionLibraryOnItsChosenScreen(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// The link each one becomes once pressed or clicked.
	links := map[string]string{
		"#/mixed?topParentId=0123&collectionType=mixed&tab=1":   "#/mixed?topParentId=0123&collectionType=mixed",
		"#!/mixed?tab=1&topParentId=0123":                       "#!/mixed?topParentId=0123",
		"#/mixed?topParentId=0123&tab=1&collectionType=mixed":   "#/mixed?topParentId=0123&collectionType=mixed",
		"#/mixed?topParentId=0123&collectionType=mixed":         "#/mixed?topParentId=0123&collectionType=mixed",
		"#/mixed?topParentId=0123&tab=3":                        "#/mixed?topParentId=0123&tab=3",
		"#/mixed?topParentId=0123&tab=12":                       "#/mixed?topParentId=0123&tab=12",
		"#/movies?topParentId=0123&collectionType=movies&tab=1": "#/movies?topParentId=0123&collectionType=movies&tab=1",
		"#/tv?topParentId=0123&collectionType=tvshows&tab=1":    "#/tv?topParentId=0123&collectionType=tvshows&tab=1",
		"#/details?id=0123&tab=1":                               "#/details?id=0123&tab=1",
	}
	const harness = `
const vm = require('node:vm')
const { script, links } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const results = {}
const node = (parentNode, matches) => ({ parentNode, matches, closest(selector) { for (let e = this; e; e = e.parentNode) if (e.matches(selector)) return e; return null } })
for (const href of links) {
  for (const type of ['mousedown', 'click']) {
    for (const on of ['link', 'title']) {
      const listeners = {}
      const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home', replace: () => {} }
      const history = { pushState: () => {}, replaceState: () => {} }
      const document = { head: { appendChild: () => {} }, createElement: () => ({}) }
      vm.runInNewContext(script, { location, history, URL, document, addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) } })
      const attributes = { href }
      const link = node(null, (selector) => selector === 'a[href]')
      link.getAttribute = (name) => attributes[name] ?? null
      link.setAttribute = (name, value) => { attributes[name] = String(value) }
      const title = node(link, () => false)
      for (const listener of listeners[type] || []) listener({ type, target: on === 'link' ? link : title })
      results[type + ' ' + on + ' ' + href] = attributes.href
    }
  }
}
// Elsewhere than on a link, nothing happens and nothing fails.
const listeners = {}
vm.runInNewContext(script, { location: { href: 'http://polyfin.test/web/#/home', hash: '#/home', replace: () => {} }, history: { pushState: () => {}, replaceState: () => {} }, URL, document: { head: { appendChild: () => {} }, createElement: () => ({}) }, addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) } })
for (const target of [node(null, () => false), {}, null]) for (const listener of listeners.click || []) listener({ type: 'click', target })
process.stdout.write(JSON.stringify(results))
`
	hrefs := make([]string, 0, len(links))
	for href := range links {
		hrefs = append(hrefs, href)
	}
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "links": hrefs})
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
	if len(results) != len(links)*2*2 {
		t.Fatalf("%d results for %d links", len(results), len(links))
	}
	for key, got := range results {
		href := key[strings.Index(key, "#"):]
		if want := links[href]; got != want {
			t.Errorf("%s: became %q, want %q", key, got, want)
		}
	}
}

// When jellyfin-web refuses a playback for a PlaybackInfo's ErrorCode, it
// shows the code's alert, then its generic error alert over it, with the
// same title. The script hides the second at once, with its backdrop, and
// closes it with its button once it is open, or 2 seconds later. It leaves
// alone a lone alert, alerts of other titles, one shown more than a second
// later or after the first was closed, and dialogs that are not alerts. It
// runs in Node.js, in a context that stands for the browser: dialog
// containers added to the body, as jellyfin-web adds them.
func TestTheGenericErrorOverARefusedPlaybackIsClosed(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// What became of the first and the second dialog: "-" untouched,
	// "closed" hidden at once then closed once open, "closed late" hidden
	// then closed without opening.
	scenarios := map[string]string{
		"the refusal's two alerts":                      "- closed",
		"the refusal's two alerts, the body after load": "- closed",
		"a lone alert":                                  "-",
		"alerts of other titles":                        "- -",
		"a second alert, a second later":                "- -",
		"a second alert, the first closed":              "- -",
		"a dialog that is not an alert":                 "- -",
		"the second alert never opening":                "- closed late",
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const results = {}
for (const name of scenarios) {
  let now = 0
  const timers = []
  const listeners = {}
  const advance = (ms) => {
    const until = now + ms
    for (;;) {
      timers.sort((a, b) => a.at - b.at)
      const next = timers[0]
      if (!next || next.at > until) break
      timers.shift()
      now = next.at
      next.run()
    }
    now = until
  }
  // The observer of the body's children; the one of its whole subtree
  // follows the menus.
  let observer = null
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target, options) { if (target !== body) throw new Error('not the body'); if (!options.subtree) observer = this }
    disconnect() {}
  }
  // A dialog as jellyfin-web's dialog helper adds it: a container after
  // its backdrop, holding a dialog with a title and its buttons.
  const dialog = (title, buttons) => {
    const backdrop = { classList: { contains: (c) => c === 'dialogBackdrop' }, style: {} }
    const inner = { opened: false, classList: { contains: (c) => c === 'opened' && inner.opened } }
    const container = {
      isConnected: true, style: {}, previousElementSibling: backdrop, backdrop, inner, clicks: 0, clicksBeforeOpen: 0,
      classList: { contains: (c) => c === 'dialogContainer' },
      querySelector: (s) => (s === '.dialog' ? inner : null),
    }
    const list = Array.from({ length: buttons }, () => ({ click: () => { container.clicks++; if (!inner.opened) container.clicksBeforeOpen++; container.isConnected = false } }))
    inner.querySelector = (s) => (s === '.formDialogHeaderTitle' && title ? { textContent: title } : null)
    inner.querySelectorAll = (s) => (s === '.btnOption' ? list : [])
    return container
  }
  const add = (container) => observer && observer.callback([{ addedNodes: [container.backdrop] }, { addedNodes: [container] }])
  // jellyfin-web opens a dialog with an animation.
  const open = (container) => { timers.push({ at: now + 150, run: () => { container.inner.opened = true } }) }
  const body = {}
  const document = { head: { appendChild: () => {} }, createElement: () => ({}), body: name.includes('after load') ? undefined : body }
  const location = { href: 'http://polyfin.test/web/#/details?id=x', hash: '#/details?id=x', replace: () => {} }
  vm.runInNewContext(script, {
    location, history: { pushState: () => {}, replaceState: () => {} }, URL, document, MutationObserver,
    Date: { now: () => now }, setTimeout: (run, delay) => timers.push({ at: now + delay, run }), clearTimeout: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  if (!document.body) { document.body = body; for (const listener of listeners.DOMContentLoaded || []) listener() }
  const title = 'Playback Error'
  const first = dialog(title, 1)
  add(first); open(first)
  let second = null
  switch (name) {
    case 'a lone alert': break
    case 'alerts of other titles': advance(2); second = dialog('Something else', 1); break
    case 'a second alert, a second later': advance(1200); second = dialog(title, 1); break
    case 'a second alert, the first closed': advance(2); first.isConnected = false; second = dialog(title, 1); break
    case 'a dialog that is not an alert': advance(2); second = dialog(title, 2); break
    default: advance(2); second = dialog(title, 1)
  }
  if (second) {
    add(second)
    if (!name.includes('never opening')) open(second)
  }
  advance(3000)
  const fate = (c) => {
    if (!c.style.visibility && !c.clicks) return '-'
    if (c.style.visibility !== 'hidden' || c.backdrop.style.visibility !== 'hidden' || c.clicks !== 1) return 'wrong: ' + JSON.stringify({ hidden: c.style.visibility, backdrop: c.backdrop.style.visibility, clicks: c.clicks })
    return c.clicksBeforeOpen ? 'closed late' : 'closed'
  }
  results[name] = [first, second].filter(Boolean).map(fate).join(' ')
}
process.stdout.write(JSON.stringify(results))
`
	names := make([]string, 0, len(scenarios))
	for name := range scenarios {
		names = append(names, name)
	}
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": names})
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
	for name, want := range scenarios {
		if got := results[name]; got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

// On a title's page, the script asks Polyfin for the title's versions at
// once, then every second while addons are pending. When the page lists
// fewer versions than there are, or a different number once none is
// pending, it asks for the title's details and lists their versions in the
// version menu, in place, keeping the version picked; it reloads the page
// as jellyfin-web reloads a page shown again when the version picked is gone
// or no longer the same.
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
	// The route asks at once; each tick after it runs the next timer.
	steps := func(n int, more ...string) []string { return slices.Concat([]string{title}, ticks(n-1), more) }
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
			steps: []string{title, "#/home", "tick"}, want: []string{ask, "idle"}},
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
			steps: slices.Concat([]string{"focus", title}, ticks(3), []string{"menu", "blur", "menu", "tick"}),
			want:  []string{ask, ask, ask, "idle", "menu t *v1 shown", details, "menu t *v1 v2 shown", "idle"}},
		// Once no addon is pending, the menu lists the versions details
		// list, even fewer.
		"no addon pending with another count": {listed: 3, answers: []answer{{1, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: steps(4, "menu"),
			want:  []string{ask, ask, details, ask, "idle", "menu *t v1 shown"}},
		// A push is taken as an answer at once; the script then asks only
		// every 10 seconds, as long as addons are pending.
		"pushed versions come at once": {listed: 1, answers: []answer{{1, 1, nil}, {1, 3, nil}, {0, 3, nil}},
			steps: []string{title, "push 1 3", "menu", "tick", "clock", "tick", "clock", "tick"},
			want:  []string{ask, details, "menu *t v1 v2 shown", ask, "clock 1", ask, "clock 11", "idle"}},
		"a push for another title changes nothing": {listed: 1, answers: []answer{{1, 1, nil}, {1, 1, nil}, {0, 1, nil}},
			steps: []string{title, "push other", "tick", "tick", "clock", "tick", "menu"},
			want:  []string{ask, ask, ask, "clock 2", "idle", "menu *t hidden"}},
		// Pushes still come once the script no longer asks.
		"a push after the last answer": {listed: 2, answers: []answer{{0, 2, nil}, {0, 2, nil}, {0, 2, nil}},
			steps: []string{title, "tick", "tick", "tick", "push 0 3", "menu"},
			want:  []string{ask, ask, ask, "idle", details, "menu *t v1 v2 shown"}},
	}
	ninety := slices.Repeat([]string{ask}, 90)
	scenarios["90 seconds at most"] = scenario{listed: 1, answers: []answer{{1, 1, nil}}, forever: true,
		// Asked at 0 to 89 seconds; the 90th asks nothing, and the polling
		// ends.
		steps: append([]string{title}, ticks(91)...), want: append(ninety, "idle")}

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
    head: { appendChild: () => {} },
    createElement: (tag) => (tag === 'option' ? { value: '', textContent: '', selected: false } : tag === 'style' ? {} : null),
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
      // Polyfin answers the user's views too, which hide nothing here.
      if (url === 'http://polyfin.test/Polyfin/UserViews') return Promise.resolve({ Items: [] })
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

// On a movie's or an episode's page, the play buttons wait for a source:
// while no version is known and addons are searched, they are disabled with
// a tooltip, greyed, with a spinner; once a version is known, by an answer
// or a push, they are as jellyfin-web made them; once no addon is left to
// search, a line under them says no source is available, with a button that
// has Polyfin search again. The first answer, asked as the route changes,
// never says no source; before it, the buttons wait only when the version
// menu lists the placeholder alone. jellyfin-web's renders find them held
// again; a video is never touched, and leaving the page gives everything
// back. It runs in Node.js, in a context that stands for the browser: a
// small document holding the title page, whose buttons jellyfin-web titled
// "Resume" and "Play", a clock and timers the harness moves on.
func TestPlayButtonsWaitForASource(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		id     = "0123456789abcdef0123456789abcdef"
		title  = "#/details?id=" + id + "&serverId=fedcba9876543210fedcba9876543210"
		ask    = "ask http://polyfin.test/Polyfin/Items/" + id + "/Versions"
		search = "search http://polyfin.test/Polyfin/Items/" + id + "/Versions/Search"
	)
	type progress struct{ Pending, Count, Known int }
	// A search answers a progress, or asks to wait RetryAfter seconds.
	type searchAnswer struct {
		progress
		RetryAfter int `json:",omitempty"`
	}
	type scenario struct {
		// lang is jellyfin-web's language, "en-US" by default.
		lang     string
		answers  []progress
		searches []searchAnswer
		// deferred answers wait for an "answer" step.
		deferred bool
		// menu lists the version menu's options at first, "t" standing for
		// the title's own identifier; none without it.
		menu []string
		// steps: a route the router moves to; "tick" runs the next timer,
		// logging "idle" when there is none; "answer" gives the oldest
		// deferred answer; "push <pending> <count> <known>" has Polyfin push
		// the title's progress; "show" logs the buttons, each "free" or
		// "held", "+spin" with a spinner, then its tooltip, and the line
		// under them, "+wait" when its button waits; "render" has
		// jellyfin-web title the buttons again, as it does when it shows the
		// page, and "rebuild" replace them; "list <option>…" has it list the
		// version menu's options; "again" clicks the line's button; "video"
		// and "end video" start and end a video.
		steps []string
		want  []string
	}
	const (
		searching   = `show play held+spin "Looking for sources…" replay held+spin "Looking for sources…"`
		none        = `show play held "No source is available for this title." replay held "No source is available for this title." note "No source is available for this title." "Try again"`
		free        = `show play free "Resume" replay free "Play"`
		frSearching = `show play held+spin "Recherche des sources…" replay held+spin "Recherche des sources…"`
		frNone      = `show play held "Aucune source disponible pour ce titre." replay held "Aucune source disponible pour ce titre." note "Aucune source disponible pour ce titre." "Réessayer"`
	)
	looking, found, empty := progress{1, 1, 0}, progress{0, 1, 1}, progress{0, 1, 0}
	scenarios := map[string]scenario{
		"searching, then a version by an answer": {answers: []progress{looking, found, found},
			steps: []string{title, "show", "tick", "show", "tick", "tick"},
			want:  []string{ask, searching, ask, free, ask, "idle"}},
		"a pushed version frees them at once": {answers: []progress{looking, looking},
			steps: []string{title, "show", "push 1 1 0", "show", "push 0 2 2", "show"},
			want:  []string{ask, searching, searching, "reload", free}},
		// The first answer may come before the details asked any addon:
		// the next one tells that no source is available.
		"no source, then searching again": {answers: []progress{empty, empty, empty, looking, {0, 3, 3}, {0, 3, 3}},
			searches: []searchAnswer{{progress: looking}},
			steps:    []string{title, "show", "tick", "show", "tick", "tick", "again", "show", "tick", "show", "tick", "show", "tick", "tick"},
			want:     []string{ask, searching, ask, none, ask, "idle", search, searching, ask, searching, ask, "reload", free, ask, "idle"}},
		"searching again too soon waits": {answers: []progress{empty, empty, empty},
			searches: []searchAnswer{{RetryAfter: 7}, {progress: looking}},
			steps:    []string{title, "tick", "tick", "tick", "again", "show", "again", "tick", "show", "again", "show"},
			want:     []string{ask, ask, ask, "idle", search, none + " +wait", none, search, searching}},
		"jellyfin-web's renders find them held": {answers: []progress{looking, empty},
			steps: []string{title, "render", "show", "rebuild", "show", "tick", "rebuild", "show", "render", "show", "push 0 1 1", "show"},
			want: []string{ask, searching, searching, ask, none, none,
				// A render titled the play button anew: it keeps that title.
				`show play free "Play" replay free "Play"`}},
		"never during a video": {answers: []progress{looking, looking, looking},
			steps: []string{"video", title, "show", "end video", "tick", "show", "video", "push 0 1 1", "show", "end video", "push 0 1 1", "show"},
			want:  []string{ask, free, ask, searching, searching, free}},
		"leaving gives everything back": {answers: []progress{empty, empty},
			steps: []string{title, "tick", "show", "#/home", "show", "tick"},
			want:  []string{ask, ask, none, free, "idle"}},
		"leaving while searching": {answers: []progress{looking, looking},
			steps: []string{title, "#/home", "show", "tick"},
			want:  []string{ask, free, "idle"}},
		"other items are left alone": {answers: []progress{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}},
			steps: []string{title, "show"},
			want:  []string{ask, free}},
		"in French": {lang: "fr-FR", answers: []progress{looking, empty, empty},
			steps: []string{title, "show", "tick", "show"},
			want:  []string{ask, frSearching, ask, frNone}},
		"in another language, English": {lang: "de", answers: []progress{looking},
			steps: []string{title, "show"},
			want:  []string{ask, searching}},
		// Before the first answer: item details list the placeholder alone,
		// a single source under the title's own identifier, while no
		// version is known.
		"held from the start with only the placeholder": {deferred: true, menu: []string{"t"}, answers: []progress{found},
			steps: []string{title, "show", "answer", "show"},
			want:  []string{ask, searching, free}},
		"held as the page lists only the placeholder": {deferred: true, answers: []progress{looking},
			steps: []string{title, "show", "list t", "show", "answer", "show"},
			want:  []string{ask, free, searching, searching}},
		"untouched while real versions are listed": {deferred: true, menu: []string{"t", "v1"}, answers: []progress{found},
			steps: []string{title, "show", "list v1", "show", "answer", "show"},
			want:  []string{ask, free, free, free}},
	}

	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
const TITLE = '0123456789abcdef0123456789abcdef'
async function run(scenario) {
  const log = []
  let now = 0
  let video = false
  const timers = []
  const listeners = {}
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: (url) => log.push('replace ' + url) }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  class CustomEvent { constructor(type, init) { this.type = type; this.detail = init && init.detail } }
  // Observers hear the changes within what they observe once the code that
  // made them is done: within its subtree, or of the element itself.
  const observers = new Set()
  const queued = new Set()
  const changed = (element) => {
    for (const observer of observers) {
      const heard = observer.options.subtree ? observer.target.contains(element) : observer.target === element
      if (!heard || queued.has(observer)) continue
      queued.add(observer)
      queueMicrotask(() => { queued.delete(observer); if (observers.has(observer)) observer.callback([]) })
    }
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target, options) { this.target = target; this.options = options; observers.add(this) }
    disconnect() { observers.delete(this) }
  }
  // A small document: elements with attributes, classes, children and text,
  // found by their classes.
  class Element {
    constructor(tag) { this.tagName = tag; this.attributes = {}; this.children = []; this.parentNode = null; this.text = ''; this.listeners = {} }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    hasAttribute(name) { return name in this.attributes }
    setAttribute(name, value) { this.attributes[name] = String(value); changed(this) }
    removeAttribute(name) { if (name in this.attributes) { delete this.attributes[name]; changed(this) } }
    get className() { return this.getAttribute('class') || '' }
    set className(value) { this.setAttribute('class', value) }
    get id() { return this.getAttribute('id') || '' }
    set id(value) { this.setAttribute('id', value) }
    get classList() {
      const names = () => this.className.split(' ').filter(Boolean)
      return {
        contains: (name) => names().includes(name),
        add: (name) => { if (!names().includes(name)) this.className = names().concat(name).join(' ') },
        remove: (name) => { if (names().includes(name)) this.className = names().filter((n) => n !== name).join(' ') },
        toggle: (name, force) => { if (force ?? !names().includes(name)) this.classList.add(name); else this.classList.remove(name) },
      }
    }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    set textContent(value) { this.children = []; this.text = value; changed(this) }
    get firstChild() { return this.children[0] || null }
    get lastChild() { return this.children[this.children.length - 1] || null }
    get nextSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) + 1] || null }
    contains(element) { for (let e = element; e; e = e.parentNode) if (e === this) return true; return false }
    insertBefore(child, before) {
      if (child.parentNode) child.remove()
      child.parentNode = this
      this.children.splice(before ? this.children.indexOf(before) : this.children.length, 0, child)
      changed(this)
      return child
    }
    appendChild(child) { return this.insertBefore(child, null) }
    remove() { const parent = this.parentNode; if (!parent) return; parent.children.splice(parent.children.indexOf(this), 1); this.parentNode = null; changed(parent) }
    matches(selector) {
      return selector.split(',').some((one) => {
        const [classes, not] = one.trim().split(':not(')
        const names = classes.split('.').filter(Boolean)
        if (names.some((name) => !this.classList.contains(name))) return false
        return !not || !this.classList.contains(not.replace(/^\.|\)$/g, ''))
      })
    }
    querySelectorAll(selector) {
      const found = []
      const walk = (element) => element.children.forEach((child) => { if (child.matches(selector)) found.push(child); walk(child) })
      walk(this)
      return found
    }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null }
    closest(selector) { for (let e = this; e; e = e.parentNode) if (e.matches && e.matches(selector)) return e; return null }
    addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener) }
    dispatchEvent(event) {
      if (event.type === 'viewshow') log.push('reload')
      for (const listener of this.listeners[event.type] || []) listener(event)
    }
    click() { if (!this.hasAttribute('disabled')) this.dispatchEvent({ type: 'click' }) }
    get options() { return this.children }
    get value() { return this.getAttribute('value') || '' }
  }
  const element = (tag, className, parent) => { const e = new Element(tag); e.className = className; if (parent) parent.appendChild(e); return e }
  const root = element('html', '')
  root.lang = scenario.lang || 'en-US'
  const head = element('head', '', root)
  const body = element('body', '', root)
  const page = element('div', 'page itemDetailPage', body)
  const detail = element('div', 'detailPagePrimaryContainer', page)
  // The buttons share the ribbon with the title's name; the line goes
  // under the ribbon.
  const ribbon = element('div', 'detailRibbon padded-left padded-right', detail)
  element('div', 'infoWrapper', ribbon)
  let row
  // jellyfin-web's buttons, as its details template has them: resuming,
  // the play button is titled Resume, and the replay button is shown.
  function build() {
    if (row) row.remove()
    row = element('div', 'mainDetailButtons focuscontainer-x', ribbon)
    for (const [name, tooltip] of [['btnPlay', 'Resume'], ['btnReplay', 'Play'], ['btnDownload', 'Download']]) {
      const button = element('button', 'button-flat ' + name + ' detailButton', row)
      button.setAttribute('title', tooltip)
      element('div', 'detailButton-content', button)
    }
  }
  build()
  element('div', 'overview', detail)
  // jellyfin-web's version menu, which lists the sources of the details.
  let menu = null
  function list(values) {
    if (menu) menu.remove()
    menu = element('select', 'selectSource', detail)
    for (const value of values) element('option', '', menu).setAttribute('value', value === 't' ? TITLE : value)
  }
  if (scenario.menu) list(scenario.menu)
  const document = {
    documentElement: root,
    head,
    body,
    createElement: (tag) => new Element(tag),
    getElementById: (id) => [root, ...root.querySelectorAll('*')].find((e) => e.id === id) || null,
    querySelector: (selector) => {
      if (selector === '.videoPlayerContainer') return video ? {} : null
      return body.querySelector(selector)
    },
  }
  root.querySelectorAll = (function (original) {
    return function (selector) {
      if (selector !== '*') return original.call(this, selector)
      const all = []
      const walk = (e) => e.children.forEach((child) => { all.push(child); walk(child) })
      walk(this)
      return all
    }
  })(root.querySelectorAll)
  function describe(button) {
    const held = button.hasAttribute('disabled') && button.getAttribute('aria-disabled') === 'true' && button.classList.contains('polyfinHeld')
    const free = !button.hasAttribute('disabled') && !button.hasAttribute('aria-disabled') && !button.classList.contains('polyfinHeld')
    const spinner = button.querySelector('.polyfinSpinner')
    const state = held ? 'held' + (spinner ? '+spin' : '') : free && !spinner ? 'free' : 'broken'
    return state + ' "' + button.getAttribute('title') + '"'
  }
  function show() {
    const words = ['show', 'play', describe(page.querySelector('.btnPlay')), 'replay', describe(page.querySelector('.btnReplay'))]
    const notes = body.querySelectorAll('.polyfinNoSource')
    if (notes.length > 1 || (notes.length && ribbon.nextSibling !== notes[0])) words.push('misplaced')
    if (notes.length === 1) {
      const [text, again] = notes[0].children
      words.push('note', '"' + text.textContent + '"', '"' + again.textContent + '"')
      if (again.hasAttribute('disabled')) words.push('+wait')
    }
    log.push(words.join(' '))
  }
  const answers = scenario.answers.slice()
  const searches = (scenario.searches || []).slice()
  const deferred = []
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => 'user',
    getJSON: (url) => {
      // Polyfin answers the user's views too, which hide nothing here.
      if (url === 'http://polyfin.test/Polyfin/UserViews') return Promise.resolve({ Items: [] })
      log.push('ask ' + url)
      const answer = answers.shift()
      if (!answer) return Promise.reject(new Error('down'))
      if (scenario.deferred) return new Promise((resolve) => deferred.push(() => resolve({ ...answer })))
      return Promise.resolve({ ...answer })
    },
    ajax: (request) => {
      log.push('search ' + request.url + (request.type === 'POST' ? '' : ' with ' + request.type))
      const answer = searches.shift()
      if (!answer) return Promise.reject(new Error('down'))
      if (answer.RetryAfter) return Promise.reject({ status: 429, headers: { get: (name) => (name === 'Retry-After' ? String(answer.RetryAfter) : null) } })
      return Promise.resolve({ Pending: answer.Pending, Count: answer.Count, Known: answer.Known })
    },
    getItem: (user, id) => {
      log.push('details')
      return Promise.resolve({ Id: id, MediaSources: [] })
    },
  }
  vm.runInNewContext(script, {
    location, history, URL, document, CustomEvent, MutationObserver, window: { ApiClient },
    Date: { now: () => now },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: (id) => { if (id) timers[id - 1] = null },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  for (const step of scenario.steps) {
    if (step === 'show') show()
    else if (step === 'answer') {
      const give = deferred.shift()
      if (give) give()
      else log.push('no answer')
    } else if (step.startsWith('list ')) list(step.split(' ').slice(1))
    else if (step === 'video') video = true
    else if (step === 'end video') video = false
    else if (step === 'render') {
      // As jellyfin-web titles the play button when it shows the page.
      page.querySelector('.btnPlay').setAttribute('title', 'Play')
      page.querySelector('.btnReplay').classList.toggle('hide', false)
    } else if (step === 'rebuild') build()
    else if (step === 'again') {
      const again = body.querySelector('.polyfinAgain')
      if (again) again.click()
      else log.push('no button')
    } else if (step.startsWith('push ')) {
      const [, pending, count, known] = step.split(' ').map(Number)
      const message = { MessageType: 'PolyfinVersions', MessageId: 'm', Data: { ItemId: TITLE.toUpperCase(), Pending: pending, Count: count, Known: known } }
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
  for (const [name, scenario] of Object.entries(scenarios)) {
    try {
      results[name] = await run(scenario)
    } catch (error) {
      results[name] = ['harness: ' + error.stack]
    }
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	type input struct {
		Lang     string         `json:"lang,omitempty"`
		Answers  []progress     `json:"answers"`
		Searches []searchAnswer `json:"searches,omitempty"`
		Deferred bool           `json:"deferred"`
		Menu     []string       `json:"menu,omitempty"`
		Steps    []string       `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.lang, s.answers, s.searches, s.deferred, s.menu, s.steps}
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

// The libraries a user hides from the web player's menus leave them: once a
// user is signed in, the script asks Polyfin for the user's views and hides
// the links of those marked in the top bar, its More menu, the side menu of
// that layout and the legacy layout's side menu, never another link to them,
// and hides the More button at the widths where it would list only such
// views. It asks again when the user changes, looks for a user restored
// after the first route for 30 seconds, and drops an answer for the user
// before. It keeps the last answer in the browser's storage and hides its
// views as it starts, before any answer, until the user signed in is known
// to be another; storage that fails or holds anything else counts as none.
// It runs in Node.js, in a context that stands for the browser: the style
// the script keeps is read back and its rules are tried on the links
// jellyfin-web 12.2 renders in each place, at widths around MUI's lg and xl
// breakpoints, with storage, a clock and timers the harness moves on.
func TestHiddenLibrariesLeaveTheWebPlayerMenus(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		title = "0123456789abcdef0123456789abcdef"
		// savedKey is where the script keeps the last answer, from one load
		// of the web player to the next.
		savedKey = "polyfinUserViews"
	)
	type view struct {
		Id          string
		HideInMenus bool
	}
	type scenario struct {
		// user is signed in as the script loads; empty for none.
		user string
		// views are Polyfin's answers by user: the user's views in order,
		// each named by one character that its identifier repeats, a "*"
		// after those hidden from the menus. A user without views gets an
		// error.
		views map[string]string
		// deferred holds each answer until an "answer <user>" step gives it.
		deferred bool
		// stored is what storage holds under savedKey as the script loads;
		// storage is "off" when using it throws, "full" when saving throws.
		stored, storage string
		// steps: a route the router moves to; "signin <user>" and "signout"
		// change the user jellyfin-web's client has, without a route; "tick"
		// runs the next timer, logging "idle" when there is none; "clock"
		// logs the time, in seconds; "show" logs, for each place, the views
		// whose link the style hides there at every width ("bar", "more",
		// "drawer", "legacy"), those with another link hidden ("other"), and
		// the widths at which the More button is hidden ("button"); "storage"
		// logs what storage holds under savedKey, as "saved <user> <views>".
		steps []string
		want  []string
	}
	asks := func(user string) string { return "views for " + user }
	// parse writes views as Polyfin answers them.
	parse := func(list string) []view {
		views := []view{}
		for _, name := range strings.Fields(list) {
			views = append(views, view{strings.Repeat(strings.TrimSuffix(name, "*"), 32), strings.HasSuffix(name, "*")})
		}
		return views
	}
	// savedAs is an answer for user kept by an earlier load.
	savedAs := func(user, views string) string {
		data, _ := json.Marshal(map[string]any{"user": user, "items": parse(views)})
		return string(data)
	}
	// hidden is what "show" logs when the style hides views in the four
	// places, and the More button at widths.
	hidden := func(views, widths string) []string {
		var lines []string
		for _, place := range []string{"bar", "more", "drawer", "legacy"} {
			lines = append(lines, strings.TrimSpace(place+" "+views))
		}
		return append(lines, "other", strings.TrimSpace("button "+widths))
	}
	nothing := hidden("", "")
	scenarios := map[string]scenario{
		"hidden views leave the four places": {user: "u1", views: map[string]string{"u1": "a* b c*"},
			steps: []string{"show"}, want: slices.Concat([]string{asks("u1")}, hidden("a c", ""))},
		// The top bar shows 3 views below 1200 pixels, 5 below 1536, 8 from
		// there, or all of them when only one more would go under More.
		"more lists hidden views only below 1200 pixels": {user: "u1", views: map[string]string{"u1": "1 2 3 4* 5* 6*"},
			steps: []string{"show"}, want: slices.Concat([]string{asks("u1")}, hidden("4 5 6", "600 1199"))},
		"more lists hidden views only from 1200 pixels": {user: "u1", views: map[string]string{"u1": "1 2 3 4 5 6* 7* 8* 9* 0*"},
			steps: []string{"show"}, want: slices.Concat([]string{asks("u1")}, hidden("6 7 8 9 0", "1200 1535 1536 1920"))},
		"nine views have no more from 1536 pixels": {user: "u1", views: map[string]string{"u1": "1 2 3 4 5 6 7 8 9*"},
			steps: []string{"show"}, want: slices.Concat([]string{asks("u1")}, hidden("9", ""))},
		// jellyfin-web may sign its user in again after the first route.
		"a user restored after the first route": {views: map[string]string{"u1": "a*"},
			steps: []string{"tick", "tick", "tick", "tick", "signin u1", "tick", "clock", "show"},
			want:  slices.Concat([]string{asks("u1"), "clock 5"}, hidden("a", ""))},
		"a user later than 30 seconds waits for a route": {views: map[string]string{"u1": "a*"},
			steps: slices.Concat(slices.Repeat([]string{"tick"}, 30), []string{"clock", "tick", "signin u1", "tick", "show", "#/home", "show"}),
			want:  slices.Concat([]string{"clock 30", "idle", "idle"}, nothing, []string{asks("u1")}, hidden("a", ""))},
		"the same user is asked once": {user: "u1", views: map[string]string{"u1": "a*"},
			steps: []string{"#/home", "#/movies?topParentId=" + strings.Repeat("a", 32), "tick", "show"},
			want:  slices.Concat([]string{asks("u1"), "idle"}, hidden("a", ""))},
		"another user is asked": {user: "u1", views: map[string]string{"u1": "a* b", "u2": "a b*"},
			steps: []string{"show", "signin u2", "#/home", "show"},
			want:  slices.Concat([]string{asks("u1")}, hidden("a", ""), []string{asks("u2")}, hidden("b", ""))},
		"an answer for the user before is dropped": {user: "u1", views: map[string]string{"u1": "a* b", "u2": "a b*"}, deferred: true,
			steps: []string{"signin u2", "#/home", "answer u1", "show", "answer u2", "show"},
			want:  slices.Concat([]string{asks("u1"), asks("u2")}, nothing, hidden("b", ""))},
		// A title's page still asks for its versions.
		"a failed answer hides nothing": {user: "u1", views: map[string]string{"u2": "a*"},
			steps: []string{"show", "#/details?id=" + title, "tick", "show"},
			want: slices.Concat([]string{asks("u1")}, nothing,
				[]string{"ask http://polyfin.test/Polyfin/Items/" + title + "/Versions", "idle"}, nothing)},
		// jellyfin-web, whose scripts run after this one, signs its user in
		// after the script started.
		"a saved answer hides at once": {stored: savedAs("u1", "a* b"), views: map[string]string{"u1": "a b*"}, deferred: true,
			steps: []string{"show", "signin u1", "tick", "show", "answer u1", "show", "storage"},
			want:  slices.Concat(hidden("a", ""), []string{asks("u1")}, hidden("a", ""), hidden("b", ""), []string{"saved u1 a b*"})},
		"a saved answer for another user is dropped": {stored: savedAs("u2", "a*"), views: map[string]string{"u1": "a b*"}, deferred: true,
			steps: []string{"show", "signin u1", "tick", "show", "answer u1", "show", "storage"},
			want:  slices.Concat(hidden("a", ""), []string{asks("u1")}, nothing, hidden("b", ""), []string{"saved u1 a b*"})},
		// Signing out drops the rules, not the answer kept.
		"an answer is kept for the next load": {user: "u1", views: map[string]string{"u1": "a* b"},
			steps: []string{"storage", "signout", "#/home", "show", "storage"},
			want:  slices.Concat([]string{asks("u1"), "saved u1 a* b"}, nothing, []string{"saved u1 a* b"})},
		"storage that is off changes nothing else": {storage: "off", user: "u1", views: map[string]string{"u1": "a*"},
			steps: []string{"show", "#/details?id=" + title, "tick", "show", "storage"},
			want: slices.Concat([]string{asks("u1")}, hidden("a", ""),
				[]string{"ask http://polyfin.test/Polyfin/Items/" + title + "/Versions", "idle"}, hidden("a", ""), []string{"saved nothing"})},
		"full storage keeps the answer before": {storage: "full", stored: savedAs("u1", "a*"), user: "u1", views: map[string]string{"u1": "b*"},
			steps: []string{"show", "storage"},
			want:  slices.Concat([]string{asks("u1")}, hidden("b", ""), []string{"saved u1 a*"})},
		"a bad saved answer counts as none": {stored: `{"user":"u1",`, views: map[string]string{"u1": "a*"}, deferred: true,
			steps: []string{"show", "signin u1", "tick", "answer u1", "show", "storage"},
			want:  slices.Concat(nothing, []string{asks("u1")}, hidden("a", ""), []string{"saved u1 a*"})},
		"a saved answer for no user counts as none": {stored: savedAs("", "a*"), views: map[string]string{"u1": "a*"}, deferred: true,
			steps: []string{"show", "signin u1", "tick", "answer u1", "show"},
			want:  slices.Concat(nothing, []string{asks("u1")}, hidden("a", ""))},
	}

	const harness = `
const vm = require('node:vm')
const { script, key, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
const short = (id) => (id === id[0].repeat(32) ? id[0] : id)
// The widths the style is tried at: around MUI's lg (1200) and xl (1536).
const WIDTHS = [600, 1199, 1200, 1535, 1536, 1920]
// The rules of a style that hide what they select, each with its media
// query, or '' for none.
function hiding(text) {
  const found = []
  let i = 0
  // until reads up to one of stop, outside quotes.
  const until = (stop) => {
    const start = i
    for (let quote = null; i < text.length; i++) {
      const c = text[i]
      if (quote) { if (c === '\\') i++; else if (c === quote) quote = null }
      else if (c === '"' || c === "'") quote = c
      else if (stop.includes(c)) break
    }
    return text.slice(start, i)
  }
  const block = (media) => {
    while (i < text.length) {
      const head = until('{}').trim()
      if (i >= text.length) throw new Error('unclosed ' + head)
      if (text[i++] === '}') return
      if (head.startsWith('@media')) { block(head.slice(6).trim()); continue }
      const body = until('}')
      i++
      if (/display\s*:\s*none/.test(body)) found.push({ media, selectors: list(head) })
    }
  }
  block('')
  return found
}
// list splits a selector list at its commas, outside quotes and brackets.
function list(text) {
  const selectors = []
  let start = 0, depth = 0, quote = null
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quote) { if (c === '\\') i++; else if (c === quote) quote = null }
    else if (c === '"' || c === "'") quote = c
    else if (c === '[' || c === '(') depth++
    else if (c === ']' || c === ')') depth--
    else if (c === ',' && !depth) { selectors.push(text.slice(start, i).trim()); start = i + 1 }
  }
  return selectors.concat(text.slice(start).trim())
}
// compound reads a tag, ids, classes and attribute tests; null for anything else.
function compound(text) {
  const m = /^([a-z]*)((?:[.#][\w-]+|\[[\w-]+(?:[*^$]?=(?:"(?:[^"\\]|\\.)*"|[\w-]+))?\])*)$/i.exec(text)
  if (!m) return null
  const parts = { tag: m[1].toLowerCase(), ids: [], classes: [], attributes: [] }
  for (const part of m[2].match(/[.#][\w-]+|\[[\w-]+(?:[*^$]?=(?:"(?:[^"\\]|\\.)*"|[\w-]+))?\]/g) || []) {
    if (part[0] === '.') parts.classes.push(part.slice(1))
    else if (part[0] === '#') parts.ids.push(part.slice(1))
    else {
      const [, name, op, value] = /^\[([\w-]+)(?:([*^$]?=)(.*))?\]$/.exec(part)
      parts.attributes.push({ name, op, value: value && value[0] === '"' ? JSON.parse(value) : value })
    }
  }
  return parts
}
function fits(parts, element) {
  const value = (name) => (element.attributes || {})[name]
  return (!parts.tag || parts.tag === element.tag) &&
    parts.ids.every((id) => element.id === id) &&
    parts.classes.every((name) => (element.classes || []).includes(name)) &&
    parts.attributes.every(({ name, op, value: wanted }) => {
      const v = value(name)
      if (v === undefined) return false
      return !op || (op === '=' ? v === wanted : op === '*=' ? v.includes(wanted) : op === '^=' ? v.startsWith(wanted) : v.endsWith(wanted))
    })
}
// matches tells whether a selector of compounds and descendant combinators
// selects the last element of a chain, from the root down.
function matches(selector, chain) {
  const parts = selector.split(/\s+/).map(compound)
  if (parts.some((part) => !part)) throw new Error('selector not understood: ' + selector)
  if (!fits(parts[parts.length - 1], chain[chain.length - 1])) return false
  let at = chain.length - 2
  for (let k = parts.length - 2; k >= 0; k--) {
    while (at >= 0 && !fits(parts[k], chain[at])) at--
    if (at < 0) return false
    at--
  }
  return true
}
function atWidth(media, width) {
  if (!media) return true
  return media.split(/\s+and\s+/).every((feature) => {
    const m = /^\((min|max)-width\s*:\s*([\d.]+)px\)$/.exec(feature.trim())
    if (!m) throw new Error('media query not understood: ' + media)
    return m[1] === 'min' ? width >= +m[2] : width <= +m[2]
  })
}
// What jellyfin-web 12.2 renders, from the root down to a library's link:
// its top bar's buttons, its More menu, the side menu of that layout, and
// the legacy layout's side menu.
const appBar = { tag: 'header', classes: ['MuiPaper-root', 'MuiAppBar-root', 'MuiAppBar-positionFixed'] }
const toolbar = { tag: 'div', classes: ['MuiToolbar-root'] }
const route = (id) => '#/movies?topParentId=' + id + '&collectionType=movies'
const button = ['MuiButtonBase-root', 'MuiButton-root', 'MuiButton-text']
const places = {
  bar: (id) => [appBar, toolbar, { tag: 'div', classes: ['MuiStack-root'] }, { tag: 'a', classes: button, attributes: { href: route(id) } }],
  more: (id) => [{ tag: 'div', id: 'user-view-overflow-menu', classes: ['MuiPopover-root', 'MuiMenu-root'] }, { tag: 'ul', classes: ['MuiList-root', 'MuiMenu-list'] },
    { tag: 'a', classes: ['MuiButtonBase-root', 'MuiMenuItem-root'], attributes: { href: route(id) } }],
  drawer: (id) => [{ tag: 'div', classes: ['MuiDrawer-root', 'MuiDrawer-docked'] }, { tag: 'ul', classes: ['MuiList-root'] }, { tag: 'li', classes: ['MuiListItem-root'] },
    { tag: 'a', classes: ['MuiButtonBase-root', 'MuiListItemButton-root'], attributes: { href: route(id) } }],
  legacy: (id) => [{ tag: 'div', classes: ['mainDrawer'] }, { tag: 'div', classes: ['libraryMenuOptions'] },
    { tag: 'a', classes: ['lnkMediaFolder', 'navMenuOption'], attributes: { is: 'emby-linkbutton', 'data-itemid': id, href: route(id) } }],
}
// Other links to a library, which stay: its home row's title, and the
// search button of its page, in the top bar.
const others = (id) => [
  [{ tag: 'div', classes: ['homeSectionsContainer'] }, { tag: 'div', classes: ['verticalSection'] },
    { tag: 'a', classes: ['more', 'button-flat', 'sectionTitleTextButton'], attributes: { href: route(id) + '&tab=1' } }],
  [appBar, toolbar, { tag: 'a', classes: ['MuiButtonBase-root', 'MuiIconButton-root'], attributes: { href: '#/search?parentId=' + id + '&collectionType=movies' } }],
]
const more = [appBar, toolbar, { tag: 'div', classes: ['MuiStack-root'] },
  { tag: 'button', classes: button, attributes: { type: 'button', 'aria-controls': 'user-view-overflow-menu', 'aria-haspopup': 'true' } }]
async function run(scenario) {
  const log = []
  let now = 0
  let user = scenario.user
  const timers = []
  const listeners = {}
  const sheets = []
  const ids = [...new Set(Object.values(scenario.views).flat().map((view) => view.Id))]
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: (url) => log.push('replace ' + url) }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  class MutationObserver { observe() {} disconnect() {} }
  const document = {
    head: { appendChild: (element) => sheets.push(element) },
    createElement: (tag) => (tag === 'style' ? { textContent: '' } : null),
    getElementById: () => null,
    querySelector: () => null,
  }
  // The browser's storage, which may be off or full.
  const store = {}
  if (scenario.stored) store[key] = scenario.stored
  const localStorage = {
    getItem: (name) => (name in store ? store[name] : null),
    setItem: (name, value) => {
      if (scenario.storage === 'full') throw new Error('QuotaExceededError')
      store[name] = String(value)
    },
  }
  const window = {
    ApiClient: null,
    get localStorage() {
      if (scenario.storage === 'off') throw new Error('SecurityError')
      return localStorage
    },
  }
  function stored() {
    if (!(key in store)) return log.push('saved nothing')
    let saved
    try {
      saved = JSON.parse(store[key])
    } catch (error) {
      return log.push('saved ' + store[key])
    }
    log.push(['saved', saved.user, ...saved.items.map((view) => short(view.Id) + (view.HideInMenus ? '*' : ''))].join(' '))
  }
  const answers = {}
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => user || undefined,
    getJSON: (url) => {
      if (url !== 'http://polyfin.test/Polyfin/UserViews') {
        log.push('ask ' + url)
        return Promise.reject(new Error('down'))
      }
      // Asked with the credentials of the user signed in then.
      const views = scenario.views[user]
      log.push('views for ' + user)
      const answer = () => (views ? Promise.resolve({ Items: views.map((view) => ({ ...view })) }) : Promise.reject(new Error('down')))
      if (!scenario.deferred) return answer()
      return new Promise((resolve, reject) => (answers[user] ||= []).push(() => answer().then(resolve, reject)))
    },
  }
  window.ApiClient = ApiClient
  function show() {
    const rules = hiding(sheets.map((sheet) => sheet.textContent).join(''))
    const hides = (chain, width) => rules.some((rule) => atWidth(rule.media, width) && rule.selectors.some((selector) => matches(selector, chain)))
    for (const [name, place] of Object.entries(places)) {
      const shown = ids.flatMap((id) => {
        const at = WIDTHS.filter((width) => hides(place(id), width)).length
        return at === WIDTHS.length ? [short(id)] : at ? [short(id) + ' at some widths'] : []
      })
      log.push([name, ...shown].join(' '))
    }
    log.push(['other', ...ids.filter((id) => others(id).some((chain) => WIDTHS.some((width) => hides(chain, width)))).map(short)].join(' '))
    log.push(['button', ...WIDTHS.filter((width) => hides(more, width))].join(' '))
  }
  vm.runInNewContext(script, {
    location, history, URL, document, MutationObserver, window,
    Date: { now: () => now },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: (id) => { if (id) timers[id - 1] = null },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  await flush()
  for (const step of scenario.steps) {
    if (step === 'show') show()
    else if (step === 'storage') stored()
    else if (step === 'clock') log.push('clock ' + now / 1000)
    else if (step.startsWith('signin ')) user = step.slice(7)
    else if (step === 'signout') user = ''
    else if (step.startsWith('answer ')) {
      const give = (answers[step.slice(7)] || []).shift()
      if (give) give()
      else log.push('no answer')
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
  for (const [name, scenario] of Object.entries(scenarios)) {
    try {
      results[name] = await run(scenario)
    } catch (error) {
      results[name] = ['harness: ' + error.stack]
    }
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	type input struct {
		User     string            `json:"user"`
		Views    map[string][]view `json:"views"`
		Deferred bool              `json:"deferred"`
		Stored   string            `json:"stored,omitempty"`
		Storage  string            `json:"storage,omitempty"`
		Steps    []string          `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		views := map[string][]view{}
		for user, list := range s.views {
			views[user] = parse(list)
		}
		inputs[name] = input{s.user, views, s.deferred, s.stored, s.storage, s.steps}
	}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "key": savedKey, "scenarios": inputs})
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

// A member, a user signed in who is not an administrator, gets a "Polyfin"
// entry where jellyfin-web 12.2 shows administrators Dashboard: the default
// layout's user menu, between dividers before Quick Connect; the settings
// page, before its user section; the legacy layouts' side menu, first in
// its user section.
// It is a plain link to the admin app, never jellyfin-web's emby-linkbutton,
// named "Polyfin". An administrator, or no user, gets none. It comes back
// as jellyfin-web draws its menus anew, and goes when the user signs out or
// another signs in. The last member is kept in the browser's storage, so
// that the entry shows before jellyfin-web's client answers. It runs in
// Node.js, in a context that stands for the browser, with a small document
// whose menus the harness draws as jellyfin-web does.
func TestMembersGetAPolyfinEntry(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// savedKey is where the script keeps the last member.
	const savedKey = "polyfinMember"
	type scenario struct {
		// user is signed in as the script loads; empty for none.
		user string
		// administrators are the users jellyfin-web's client says are.
		administrators []string
		// apps draws the user menu as an app with client settings does.
		apps bool
		// deferred holds the client's answer about a user until an
		// "answer <user>" step gives it.
		deferred bool
		// stored is what storage holds under savedKey as the script loads.
		stored string
		// steps: "draw" draws the three menus as jellyfin-web does for the
		// user signed in, none without one; "signin <user>" and "signout"
		// change the user, then move to the home route as jellyfin-web
		// does; "show" logs each menu ("drawer", "settings", "menu") with
		// its items, "Polyfin" standing for an entry that opens the admin
		// app, "|" for a divider; "storage" logs what storage holds.
		steps []string
		want  []string
	}
	const (
		memberDrawer   = "drawer libraries user: settings signout"
		adminDrawer    = "drawer libraries admin user: settings signout"
		memberSettings = "settings preferences user"
		adminSettings  = "settings preferences admin user"
		memberMenu     = "menu profile settings | quickconnect signout"
		adminMenu      = "menu profile settings | dashboard metadata | quickconnect signout"
	)
	withEntry := []string{"drawer libraries user: Polyfin settings signout", "settings preferences Polyfin user", "menu profile settings | Polyfin | quickconnect signout"}
	without := []string{memberDrawer, memberSettings, memberMenu}
	scenarios := map[string]scenario{
		"a member gets the entry in each menu": {user: "alice",
			steps: []string{"draw", "show"}, want: withEntry},
		"an administrator keeps Dashboard": {user: "root", administrators: []string{"root"},
			steps: []string{"draw", "show"}, want: []string{adminDrawer, adminSettings, adminMenu}},
		"no user gets none": {steps: []string{"draw", "signin alice", "show"},
			want: []string{"drawer", "settings", "menu"}},
		"an app's user menu has the entry before Quick Connect": {user: "alice", apps: true,
			steps: []string{"draw", "show"},
			want:  []string{withEntry[0], withEntry[1], "menu profile settings | clientsettings | Polyfin | quickconnect signout"}},
		"menus drawn anew keep the entry": {user: "alice",
			steps: []string{"draw", "draw", "show"}, want: withEntry},
		"signing out takes it away": {user: "alice",
			steps: []string{"draw", "signout", "show"}, want: without},
		"an administrator signing in takes it away": {user: "alice", administrators: []string{"root"},
			steps: []string{"draw", "signin root", "show", "draw", "show", "storage"},
			want:  slices.Concat(without, []string{adminDrawer, adminSettings, adminMenu, "saved nothing"})},
		"a member signing in after an administrator gets it": {user: "root", administrators: []string{"root"},
			steps: []string{"draw", "signin alice", "draw", "show", "storage"},
			want:  slices.Concat(withEntry, []string{"saved alice"})},
		"the member kept shows it before the client answers": {user: "alice", deferred: true, stored: `{"user":"alice"}`,
			steps: []string{"draw", "show", "answer alice", "show"}, want: slices.Concat(withEntry, withEntry)},
		"a member waits for the client's answer": {user: "alice", deferred: true,
			steps: []string{"draw", "show", "answer alice", "show", "storage"},
			want:  slices.Concat(without, withEntry, []string{"saved alice"})},
		"the member kept for another user shows nothing": {user: "bob", deferred: true, stored: `{"user":"alice"}`, administrators: []string{"bob"},
			steps: []string{"draw", "show", "answer bob", "show", "storage"},
			want:  []string{adminDrawer, adminSettings, adminMenu, adminDrawer, adminSettings, adminMenu, "saved nothing"}},
		"the member kept who became an administrator keeps Dashboard": {user: "alice", deferred: true, stored: `{"user":"alice"}`, administrators: []string{"alice"},
			steps: []string{"draw", "show"}, want: []string{adminDrawer, adminSettings, adminMenu}},
		"an answer for the user before is dropped": {user: "alice", deferred: true, administrators: []string{"root"},
			steps: []string{"signin root", "answer alice", "draw", "show"}, want: []string{adminDrawer, adminSettings, adminMenu}},
	}

	const harness = `
const vm = require('node:vm')
const { script, key, dashboardIcon, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  let user = scenario.user
  // Observers hear the changes within what they observe once the code that
  // made them is done.
  const observers = new Set()
  let queued = false
  const changed = () => {
    if (queued || !observers.size) return
    queued = true
    queueMicrotask(() => { queued = false; for (const observer of observers) observer.callback([]) })
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target, options) { if (!options.subtree) return; observers.add(this) }
    disconnect() { observers.delete(this) }
  }
  // A small document: elements with attributes, classes, children and
  // text, found by tag, identifier, classes, attributes and descendants.
  class Element {
    constructor(tag) { this.tagName = tag.toUpperCase(); this.attributes = {}; this.children = []; this.parentNode = null; this.text = '' }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    hasAttribute(name) { return name in this.attributes }
    setAttribute(name, value) { this.attributes[name] = String(value); changed() }
    removeAttribute(name) { delete this.attributes[name]; changed() }
    get className() { return this.getAttribute('class') || '' }
    set className(value) { this.setAttribute('class', value) }
    get id() { return this.getAttribute('id') || '' }
    get classList() {
      const names = () => this.className.split(' ').filter(Boolean)
      return {
        contains: (name) => names().includes(name),
        add: (...add) => { this.className = [...new Set(names().concat(add))].join(' ') },
        remove: (...remove) => { this.className = names().filter((n) => !remove.includes(n)).join(' ') },
      }
    }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    set textContent(value) { this.children = []; this.text = value; changed() }
    get previousElementSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) - 1] || null }
    get nextSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) + 1] || null }
    get nextElementSibling() { return this.nextSibling }
    insertBefore(child, before) {
      if (child.parentNode) child.remove()
      child.parentNode = this
      this.children.splice(before ? this.children.indexOf(before) : this.children.length, 0, child)
      changed()
      return child
    }
    appendChild(child) { return this.insertBefore(child, null) }
    remove() { const parent = this.parentNode; if (!parent) return; parent.children.splice(parent.children.indexOf(this), 1); this.parentNode = null; changed() }
    cloneNode(deep) {
      const copy = new Element(this.tagName)
      copy.attributes = { ...this.attributes }
      copy.text = this.text
      copy.name = this.name
      if (deep) for (const child of this.children) { const c = child.cloneNode(true); c.parentNode = copy; copy.children.push(c) }
      return copy
    }
    fits(compound) {
      const m = /^([a-z][a-z0-9]*)?((?:[.#][\w-]+|\[[\w-]+="[^"]*"\])*)$/i.exec(compound)
      if (!m) throw new Error('selector not understood: ' + compound)
      if (m[1] && m[1].toUpperCase() !== this.tagName) return false
      return (m[2].match(/[.#][\w-]+|\[[\w-]+="[^"]*"\]/g) || []).every((part) => {
        if (part[0] === '.') return this.classList.contains(part.slice(1))
        if (part[0] === '#') return this.id === part.slice(1)
        const [, name, value] = /^\[([\w-]+)="([^"]*)"\]$/.exec(part)
        return this.getAttribute(name) === value
      })
    }
    matches(selector) {
      const parts = selector.trim().split(/\s+/)
      if (!this.fits(parts[parts.length - 1])) return false
      let at = this.parentNode
      for (let k = parts.length - 2; k >= 0; k--) {
        while (at && !(at.fits && at.fits(parts[k]))) at = at.parentNode
        if (!at) return false
        at = at.parentNode
      }
      return true
    }
    querySelectorAll(selector) {
      const found = []
      const walk = (element) => element.children.forEach((child) => { if (child.matches(selector)) found.push(child); walk(child) })
      walk(this)
      return found
    }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null }
    closest(selector) { for (let e = this; e && e.matches; e = e.parentNode) if (e.matches(selector)) return e; return null }
  }
  const element = (tag, className, attributes, children, name) => {
    const e = new Element(tag)
    e.attributes = { ...(attributes || {}) }
    if (className) e.attributes.class = className
    for (const child of children || []) { child.parentNode = e; e.children.push(child) }
    e.name = name
    return e
  }
  const root = element('html', '')
  const head = element('head', '')
  const body = element('body', '')
  root.appendChild(head)
  root.appendChild(body)
  const document = { head, body, createElement: (tag) => new Element(tag), querySelectorAll: (s) => root.querySelectorAll(s), querySelector: (s) => root.querySelector(s) }
  // The three menus jellyfin-web 12.2 draws, as for user.
  const link = (href, name) => element('a', 'navMenuOption lnkMediaFolder emby-button', { is: 'emby-linkbutton', href }, [], name)
  const muiItem = (href, name, icon, text) => element(href ? 'a' : 'li', 'MuiButtonBase-root MuiMenuItem-root', href ? { href, tabindex: '-1', role: 'menuitem' } : { role: 'menuitem' }, [
    element('div', 'MuiListItemIcon-root', {}, [element('svg', 'MuiSvgIcon-root', { 'data-testid': icon }, [element('path', '', { d: icon + ' path' })])]),
    element('div', 'MuiListItemText-root', {}, [Object.assign(element('span', 'MuiTypography-root MuiListItemText-primary'), { text })]),
    element('span', 'MuiTouchRipple-root', {}, [element('span', 'ripple')]),
  ], name)
  const divider = () => element('hr', 'MuiDivider-root', {}, [], '|')
  function draw() {
    for (const old of body.children.slice()) old.remove()
    if (!user) return
    const admin = scenario.administrators.includes(user)
    body.appendChild(element('div', 'mainDrawer', {}, [element('div', 'scrollContainer', {}, [
      element('div', 'libraryMenuOptions', {}, [], 'libraries'),
      ...(admin ? [element('div', 'adminMenuOptions', {}, [element('h3', 'sidebarHeader'), link('#/dashboard'), link('#/metadata')], 'admin')] : []),
      element('div', 'userMenuOptions', {}, [element('h3', 'sidebarHeader'), link('#', 'settings'), link('#', 'signout')], 'user'),
    ])], 'drawer'))
    body.appendChild(element('div', 'page libraryPage', {}, [element('div', 'readOnlyContent', {}, [
      element('div', 'verticalSection', {}, [], 'preferences'),
      ...(admin ? [element('div', 'adminSection verticalSection', {}, [element('h2', 'sectionTitle'), element('a', 'emby-button listItem-border', { is: 'emby-linkbutton', href: '#/dashboard' })], 'admin')] : []),
      element('div', 'userSection verticalSection', {}, [], 'user'),
    ], 'settings')]))
    const items = [
      muiItem('#/userprofile?userId=' + user, 'profile', 'AccountCircleIcon', 'Profile'),
      muiItem('#/mypreferencesmenu', 'settings', 'SettingsIcon', 'Settings'),
      ...(scenario.apps ? [divider(), muiItem('', 'clientsettings', 'DevicesIcon', 'Client settings')] : []),
      ...(admin ? [divider(), muiItem('#/dashboard', 'dashboard', 'DashboardIcon', 'Dashboard'), muiItem('#/metadata', 'metadata', 'EditIcon', 'Metadata')] : []),
      divider(),
      muiItem('#/quickconnect', 'quickconnect', 'PhonelinkLockIcon', 'Quick Connect'),
      muiItem('', 'signout', 'LogoutIcon', 'Sign out'),
    ]
    body.appendChild(element('div', 'MuiPopover-root MuiMenu-root', { id: 'app-user-menu' }, [element('div', 'MuiPaper-root', {}, [element('ul', 'MuiList-root MuiMenu-list', {}, items)])]))
  }
  // describe names an item, an entry as "Polyfin" when it is a plain link
  // to the admin app named so, in the style of the menu's own items.
  function describe(item) {
    if (!item.classList.contains('polyfinEntry')) return item.name
    if (item.classList.contains('MuiDivider-root')) return '|'
    const a = item.tagName === 'A' ? item : item.querySelector('a')
    const problems = []
    if (!a || a.getAttribute('href') !== 'http://polyfin.test/admin/' || a.hasAttribute('is')) problems.push('link')
    if (item.textContent !== 'Polyfin') problems.push('text ' + item.textContent)
    if (item.querySelector('h2') || item.querySelector('h3')) problems.push('heading')
    if (item.parentNode.tagName === 'UL') {
      const path = item.querySelector('svg path')
      if (!item.classList.contains('MuiMenuItem-root') || !path || path.getAttribute('d') !== dashboardIcon || item.querySelector('[data-testid="SettingsIcon"]')) problems.push('item')
    } else if (!a || !a.querySelector('.material-icons.dashboard') || !a.classList.contains(item.closest('.mainDrawer') ? 'navMenuOption' : 'listItem-border')) problems.push('item')
    return problems.length ? 'Polyfin(' + problems.join(', ') + ')' : 'Polyfin'
  }
  function show() {
    for (const [name, selector] of [['drawer', '.mainDrawer .scrollContainer'], ['settings', '.readOnlyContent'], ['menu', '#app-user-menu ul']]) {
      const container = root.querySelector(selector)
      // The side menu's user section is listed after its name, heading left out.
      const list = (item) => (item.name === 'user' && item.parentNode.classList.contains('scrollContainer') ? ['user:', ...item.children.slice(1).map(describe)] : [describe(item)])
      log.push([name, ...(container ? container.children.flatMap(list) : [])].join(' '))
    }
  }
  const store = {}
  if (scenario.stored) store[key] = scenario.stored
  const localStorage = {
    getItem: (name) => (name in store ? store[name] : null),
    setItem: (name, value) => { store[name] = String(value) },
    removeItem: (name) => { delete store[name] },
  }
  const answers = {}
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => user || undefined,
    getJSON: () => Promise.reject(new Error('down')),
    // Asked as the user signed in then.
    getCurrentUser: () => {
      const asked = user
      if (!asked) return Promise.reject(new Error('no user'))
      const answer = () => ({ Id: asked, Policy: { IsAdministrator: scenario.administrators.includes(asked) } })
      if (!scenario.deferred) return Promise.resolve(answer())
      return new Promise((resolve) => (answers[asked] ||= []).push(() => resolve(answer())))
    },
  }
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: (url) => log.push('replace ' + url) }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  vm.runInNewContext(script, {
    location, history, URL, document, MutationObserver, window: { ApiClient, localStorage },
    Date, setTimeout: () => 0, clearTimeout: () => {},
    addEventListener: () => {},
  })
  await flush()
  for (const step of scenario.steps) {
    if (step === 'show') show()
    else if (step === 'draw') draw()
    else if (step === 'storage') log.push(key in store ? 'saved ' + JSON.parse(store[key]).user : 'saved nothing')
    else if (step.startsWith('answer ')) {
      const give = (answers[step.slice(7)] || []).shift()
      if (give) give()
      else log.push('no answer')
    } else {
      user = step === 'signout' ? '' : step.slice(7)
      history.pushState(null, '', '#/home')
    }
    await flush()
  }
  return log
}
;(async () => {
  const results = {}
  for (const [name, scenario] of Object.entries(scenarios)) {
    try {
      results[name] = await run(scenario)
    } catch (error) {
      results[name] = ['harness: ' + error.stack]
    }
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	type input struct {
		User           string   `json:"user"`
		Administrators []string `json:"administrators"`
		Apps           bool     `json:"apps"`
		Deferred       bool     `json:"deferred"`
		Stored         string   `json:"stored,omitempty"`
		Steps          []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.user, append([]string{}, s.administrators...), s.apps, s.deferred, s.stored, s.steps}
	}
	data, _ := json.Marshal(map[string]any{
		"script": string(webScriptBody), "key": savedKey, "scenarios": inputs,
		// MUI's Dashboard icon, which jellyfin-web 12.2 draws in the user menu.
		"dashboardIcon": "M3 13h8V3H3zm0 8h8v-6H3zm10 0h8V11h-8zm0-18v6h8V3z",
	})
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
