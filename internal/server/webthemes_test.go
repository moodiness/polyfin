package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/webtheme"
)

// testServerID names the server of themesHandler.
const testServerID = "0123456789abcdef0123456789abcdef"

// themesTag finds the page's description of the themes.
var themesTag = regexp.MustCompile(`<script type="application/json" id="polyfinWebThemes">([^<]*)</script>`)

// themesHandler serves the web client with the themes the test keeps in
// themes, which it can change.
func themesHandler(themes *atomic.Pointer[WebThemes]) http.Handler {
	return New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html><head><title>Polyfin</title></head>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           webFiles,
		SetupRequired: setupState{}.SetupRequired,
		WebThemes:     func() WebThemes { return *themes.Load() },
		ServerID:      testServerID,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// describedThemes reads the themes the page at target describes, failing
// unless it does once, in its head, right before Polyfin's script.
func describedThemes(t *testing.T, h http.Handler, target string) pageThemes {
	t.Helper()
	body := get(h, target).Body.String()
	tags := themesTag.FindAllStringSubmatchIndex(body, -1)
	if len(tags) != 1 {
		t.Fatalf("%s: %d descriptions of the themes in %s", target, len(tags), body)
	}
	if !strings.HasPrefix(body[tags[0][1]:], `<script src="polyfin.js"></script></head>`) {
		t.Errorf("%s: the themes are not described right before Polyfin's script: %s", target, body)
	}
	var themes pageThemes
	if err := json.Unmarshal([]byte(body[tags[0][2]:tags[0][3]]), &themes); err != nil {
		t.Fatalf("%s: %v", target, err)
	}
	return themes
}

// The page describes the themes to Polyfin's script, which loads the
// user's: the server's theme, LumaaGlaass's stylesheet and loader, and the
// Custom theme's texts while they are set, each at an address that follows
// its content, with the cookie the script names its theme in. The page no
// longer loads a script of its own. Without the themes, it describes none.
func TestThePageDescribesTheThemes(t *testing.T) {
	themes := new(atomic.Pointer[WebThemes])
	themes.Store(&WebThemes{Theme: accounts.WebThemeLumaaGlaass})
	h := themesHandler(themes)
	lumaaGlaass := pageTheme{Css: webtheme.Address("assets/lumaaglaass.css"), Js: webtheme.Address(webtheme.Loader)}
	for _, target := range []string{"/web/", "/web/index.html"} {
		described := describedThemes(t, h, target)
		want := pageThemes{Cookie: webtheme.Cookie(testServerID), Theme: accounts.WebThemeLumaaGlaass,
			Themes: map[string]pageTheme{accounts.WebThemeLumaaGlaass: lumaaGlaass}}
		if got, _ := json.Marshal(described); string(got) != string(must(json.Marshal(want))) {
			t.Errorf("%s: %s", target, got)
		}
	}
	// What the server's LumaaGlaass CSS imports is the stylesheet the page
	// loads, so that the script knows jellyfin-web's copy of it.
	if accounts.LumaaGlaassCss != webtheme.Import(lumaaGlaass.Css) {
		t.Errorf("LumaaGlaass's CSS %q imports another stylesheet than %q", accounts.LumaaGlaassCss, lumaaGlaass.Css)
	}

	themes.Store(&WebThemes{Theme: accounts.WebThemeCustom, CustomCss: ".first {}"})
	first := describedThemes(t, h, "/web/")
	custom, ok := first.Themes[accounts.WebThemeCustom]
	if first.Theme != accounts.WebThemeCustom || !ok || custom.Css != webtheme.CustomCss+"?v="+webtheme.Hash(".first {}") || custom.Js != "" {
		t.Errorf("a Custom CSS: %+v", first)
	}
	themes.Store(&WebThemes{Theme: accounts.WebThemeNone, CustomCss: ".second {}", CustomJs: "second()"})
	second := describedThemes(t, h, "/web/").Themes[accounts.WebThemeCustom]
	if second.Css == custom.Css || second.Js != webtheme.CustomJs+"?v="+webtheme.Hash("second()") {
		t.Errorf("the addresses did not follow the texts: %+v, was %+v", second, custom)
	}
	themes.Store(&WebThemes{Theme: accounts.WebThemeNone})
	if described := describedThemes(t, h, "/web/"); described.Theme != accounts.WebThemeNone || len(described.Themes) != 1 {
		t.Errorf("without Custom texts: %+v", described)
	}
	themes.Store(&WebThemes{Theme: accounts.WebThemeCustom, CustomCss: "</script><script>alert(1)</script>", CustomJs: "x()"})
	// The page loads no script but Polyfin's, which loads the theme's.
	for _, target := range []string{"/web/", "/admin/", "/admin/users", "/web/session-login.0123456789abcdef.htm"} {
		body := get(h, target).Body.String()
		if strings.Contains(body, "custom.js") && target != "/web/" || strings.Contains(body, `<script src="custom.js`) || strings.Contains(body, "alert(1)") {
			t.Errorf("%s: %s", target, body)
		}
		if target != "/web/" && strings.Contains(body, "polyfinWebThemes") {
			t.Errorf("%s describes the themes: %s", target, body)
		}
	}
	if body := get(webHandler(setupState{}), "/web/").Body.String(); strings.Contains(body, "polyfinWebThemes") {
		t.Errorf("without the themes: %s", body)
	}
}

// must returns value, which err leaves nil.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// The Custom theme's texts are served while set, whatever the server's
// theme, as a user may have it: at the address the page gives, cached for
// good; at another, revalidated, so an old address gets today's text.
func TestCustomTextsAreCachedUntilTheyChange(t *testing.T) {
	themes := new(atomic.Pointer[WebThemes])
	themes.Store(&WebThemes{Theme: accounts.WebThemeLumaaGlaass})
	h := themesHandler(themes)
	for _, target := range []string{"/web/custom.css", "/web/custom.js"} {
		if body := get(h, target).Body.String(); body != "jellyfin" {
			t.Errorf("no text set: %s answered %q, want Jellyfin's handler", target, body)
		}
	}
	themes.Store(&WebThemes{Theme: accounts.WebThemeLumaaGlaass, CustomCss: ".skinHeader {}", CustomJs: "document.body.dataset.custom = 'yes'"})
	described := describedThemes(t, h, "/web/").Themes[accounts.WebThemeCustom]
	for _, tc := range []struct{ target, body, contentType, cache string }{
		{"/web/" + described.Css, ".skinHeader {}", "text/css; charset=utf-8", immutableCache},
		{"/web/custom.css", ".skinHeader {}", "text/css; charset=utf-8", "no-cache"},
		{"/web/custom.css?v=0123456789abcdef", ".skinHeader {}", "text/css; charset=utf-8", "no-cache"},
		{"/web/" + described.Js, "document.body.dataset.custom = 'yes'", "application/javascript; charset=utf-8", immutableCache},
		{"/web/custom.js", "document.body.dataset.custom = 'yes'", "application/javascript; charset=utf-8", "no-cache"},
		{"/web/custom.js?v=0123456789abcdef", "document.body.dataset.custom = 'yes'", "application/javascript; charset=utf-8", "no-cache"},
	} {
		response := get(h, tc.target)
		header := response.Header()
		if response.Code != http.StatusOK || response.Body.String() != tc.body {
			t.Errorf("%s: %d %q", tc.target, response.Code, response.Body)
		}
		if header.Get("Content-Type") != tc.contentType || header.Get("Cache-Control") != tc.cache {
			t.Errorf("%s: Content-Type %q, Cache-Control %q", tc.target, header.Get("Content-Type"), header.Get("Cache-Control"))
		}
		if header.Get("Content-Security-Policy") != "" {
			t.Errorf("%s: a Content-Security-Policy", tc.target)
		}
	}
}

// The script that loads LumaaGlaass, which Polyfin writes, is served beside
// the theme's files, cached for good at their version; another version
// leads to today's.
func TestTheLumaaGlaassLoaderIsServedBesideTheTheme(t *testing.T) {
	themes := new(atomic.Pointer[WebThemes])
	themes.Store(&WebThemes{Theme: accounts.WebThemeNone})
	h := themesHandler(themes)
	loader := "/web/" + describedThemes(t, h, "/web/").Themes[accounts.WebThemeLumaaGlaass].Js
	response := get(h, loader)
	if response.Code != http.StatusOK || response.Body.String() != accounts.LumaaGlaassJs ||
		response.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || response.Header().Get("Cache-Control") != immutableCache {
		t.Errorf("%s: %d %v %q", loader, response.Code, response.Header(), response.Body)
	}
	old := strings.Replace(loader, webtheme.Version, "latest", 1)
	if response := get(h, old); response.Code != http.StatusFound || response.Header().Get("Location") != loader {
		t.Errorf("%s: %d to %q", old, response.Code, response.Header().Get("Location"))
	}
}

// webThemeAnswer is what /Polyfin/WebTheme answers for a user.
type webThemeAnswer struct {
	Theme     string
	UserTheme string
}

// The page shows each user's web player theme, which Polyfin's script picks
// before jellyfin-web starts: the theme kept for the user jellyfin-web's
// saved credentials sign in, else the last user's, else the server's;
// jellyfin-web's own look on a browser too old for LumaaGlaass. It writes
// the stylesheet, unless the user turned off the server's CSS, and the
// script, names the theme in the cookie, and forgets what jellyfin-web kept
// when the stylesheet changed. Once jellyfin-web shows the same stylesheet,
// loaded, the script's goes. Once a user signs in, it asks their theme:
// another one with a script before reloads the page, once, when the
// browser keeps it; else it changes at once, and jellyfin-web's copy of the
// stylesheet before is turned off. It runs in Node.js, in a context that
// stands for the browser, the page being parsed, and jellyfin-web.
func TestThePageShowsEachUsersTheme(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type scenario struct {
		// storage is what the browser keeps as the page loads; failing is a
		// browser whose storage refuses what is written.
		storage map[string]string
		failing bool
		// customCssOnly is a server whose Custom theme is a stylesheet only.
		customCssOnly bool
		// old is a browser without :has() (css) or ES2021 syntax (syntax).
		old string
		// database is a browser with jellyfin-web's IndexedDB database.
		database bool
		// answers are what /Polyfin/WebTheme answers each user.
		answers map[string]webThemeAnswer
		// steps: "head" logs the stylesheets and scripts of the head, and
		// of the body when it has some; "kept" what the browser keeps;
		// "styles" jellyfin-web's stylesheets; "signin <id>" signs the
		// user in, and the router moves; "user <id>" has jellyfin-web sign
		// the user in, no route moving; "jellyfin <address>" has
		// jellyfin-web show the stylesheet at address, loaded,
		// "jellyfin-loading <address>" not yet loaded, "loaded" loads them;
		// "fetch <path>" has jellyfin-web's SDK fetch path; "tick" runs the
		// timers; "load" loads the page again in the same browser.
		steps []string
		want  []string
	}
	saved := func(user string) string {
		return `{"Servers":[{"Id":"s","UserId":"` + user + `","AccessToken":"t","DateLastAccessed":1}]}`
	}
	lumaaGlaass, custom := "lg/lumaaglaass.css", "custom.css?v=1"
	const (
		lumaaGlaassHead = "head: link lg/lumaaglaass.css, script lg/loader.js"
		customHead      = "head: link custom.css?v=1, script custom.js?v=1"
	)
	scenarios := map[string]scenario{
		"a first visit shows the server's theme": {
			steps: []string{"head", "kept"},
			want:  []string{"cookie lumaaglaass", lumaaGlaassHead, "kept users= last= applied=" + lumaaGlaass}},
		"the saved user's own theme shows from the first paint": {database: true,
			storage: map[string]string{"jellyfin_credentials": saved("u1"), "polyfinKeptFor": "s/u1",
				"polyfinWebTheme": `{"Users":[["s/u1","none"]],"Last":"s/u1","Applied":"lg/lumaaglaass.css"}`},
			steps: []string{"head"},
			want:  []string{"cookie none", "deleted jellyfin-query-cache", "head: none"}},
		"signed out, the last user's theme shows": {database: true,
			storage: map[string]string{"polyfinWebTheme": `{"Users":[["s/u1","custom"]],"Last":"s/u1","Applied":"custom.css?v=1"}`},
			steps:   []string{"head"},
			want:    []string{"cookie custom", customHead}},
		"another user's saved credentials show the server's theme": {
			storage: map[string]string{"jellyfin_credentials": saved("u2"),
				"polyfinWebTheme": `{"Users":[["s/u1","none"]],"Last":"s/u1","Applied":""}`},
			steps: []string{"head", "kept"},
			want:  []string{"cookie lumaaglaass", lumaaGlaassHead, "kept users=s/u1:none last=s/u2 applied=" + lumaaGlaass}},
		"a browser without :has() shows jellyfin-web's own look": {old: "css",
			steps: []string{"head"},
			want:  []string{"cookie none", "head: none"}},
		"a browser without ES2021 shows jellyfin-web's own look": {old: "syntax",
			steps: []string{"head"},
			want:  []string{"cookie none", "head: none"}},
		"the server's CSS turned off leaves the stylesheet out": {
			storage: map[string]string{"jellyfin_credentials": saved("u1"), "u1-disableCustomCss": "true"},
			steps:   []string{"head"},
			want:    []string{"cookie lumaaglaass", "head: script lg/loader.js"}},
		"jellyfin-web's copy, once loaded, replaces the script's stylesheet": {
			steps: []string{"jellyfin-loading lg/lumaaglaass.css", "tick", "head", "loaded", "tick", "head"},
			want:  []string{"cookie lumaaglaass", lumaaGlaassHead, "head: script lg/loader.js"}},
		"a user signed in with the same theme keeps the page": {
			storage: map[string]string{"jellyfin_credentials": saved("u1")},
			answers: map[string]webThemeAnswer{"u1": {"lumaaglaass", ""}},
			steps:   []string{"signin u1", "signin u1", "head", "kept"},
			want:    []string{"cookie lumaaglaass", "asked for u1", lumaaGlaassHead, "kept users=s/u1: last=s/u1 applied=" + lumaaGlaass}},
		"leaving a theme with a script reloads the page once": {database: true,
			storage: map[string]string{"jellyfin_credentials": saved("u1"), "polyfinKeptFor": "s/u1"},
			answers: map[string]webThemeAnswer{"u1": {"none", "none"}},
			steps:   []string{"signin u1", "kept", "load", "head", "signin u1", "head"},
			want: []string{"cookie lumaaglaass", "deleted jellyfin-query-cache", "asked for u1", "cookie none", "reload",
				"kept users=s/u1:none last=s/u1 applied=" + lumaaGlaass,
				"cookie none", "deleted jellyfin-query-cache", "head: none", "asked for u1", "head: none"}},
		"a load signed in asks without a route moving": {
			storage: map[string]string{"jellyfin_credentials": saved("u1")},
			answers: map[string]webThemeAnswer{"u1": {"none", "none"}},
			steps:   []string{"tick", "user u1", "tick", "tick"},
			want:    []string{"cookie lumaaglaass", "asked for u1", "cookie none", "reload"}},
		"a browser that cannot keep the theme is not reloaded": {failing: true,
			storage: map[string]string{"jellyfin_credentials": saved("u1")},
			answers: map[string]webThemeAnswer{"u1": {"none", "none"}},
			steps:   []string{"signin u1", "head"},
			want:    []string{"cookie lumaaglaass", "asked for u1", lumaaGlaassHead}},
		"a theme added changes at once": {
			storage: map[string]string{"jellyfin_credentials": saved("u1"),
				"polyfinWebTheme": `{"Users":[["s/u1","none"]],"Last":"s/u1","Applied":""}`},
			answers: map[string]webThemeAnswer{"u1": {"lumaaglaass", "lumaaglaass"}},
			steps:   []string{"head", "signin u1", "head", "kept"},
			want: []string{"cookie none", "head: none", "asked for u1", "cookie lumaaglaass",
				"head: script lg/loader.js; body: link lg/lumaaglaass.css", "kept users=s/u1:lumaaglaass last=s/u1 applied=null"}},
		"a stylesheet left changes at once, jellyfin-web's copy turned off": {customCssOnly: true,
			storage: map[string]string{"jellyfin_credentials": saved("u1"),
				"polyfinWebTheme": `{"Users":[["s/u1","custom"]],"Last":"s/u1","Applied":"custom.css?v=1"}`},
			answers: map[string]webThemeAnswer{"u1": {"none", "none"}},
			steps:   []string{"jellyfin " + custom, "tick", "head", "styles", "signin u1", "head", "styles"},
			want: []string{"cookie custom", "head: none", "styles: custom.css?v=1 on", "asked for u1", "cookie none", "head: none",
				"styles: custom.css?v=1 off"}},
		"jellyfin-web's fetches of the branding go with the page's cookies": {
			steps: []string{"fetch /Branding/Configuration", "fetch /Branding/Css.css?v=2", "fetch /Users/Me"},
			want: []string{"cookie lumaaglaass", `fetch /Branding/Configuration {"cache":"no-store","credentials":"same-origin"}`,
				`fetch /Branding/Css.css?v=2 {"cache":"no-store","credentials":"same-origin"}`, `fetch /Users/Me {"cache":"no-store"}`}},
	}
	const harness = `
const vm = require('node:vm')
const { script, page, cssOnlyPage, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
class Element {
  constructor(tag) { this.tagName = tag.toUpperCase(); this.attributes = {}; this.children = []; this.parentNode = null; this.textContent = ''; this.sheet = null }
  setAttribute(name, value) { this.attributes[name] = String(value) }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null }
  get id() { return this.attributes.id || '' }
  set id(value) { this.attributes.id = value }
  get src() { return this.attributes.src || '' }
  set src(value) { this.attributes.src = value }
  get rel() { return this.attributes.rel || '' }
  set rel(value) { this.attributes.rel = value }
  appendChild(child) { if (child.parentNode) child.parentNode.removeChild(child); child.parentNode = this; this.children.push(child); return child }
  removeChild(child) { this.children = this.children.filter((other) => other !== child); child.parentNode = null; return child }
}
const storageOf = (values, failing) => ({
  getItem: (name) => (name in values ? values[name] : null),
  setItem: (name, value) => { if (failing) throw new Error('QuotaExceededError'); values[name] = String(value) },
  removeItem: (name) => { delete values[name] },
})
async function run(scenario) {
  const log = []
  const local = Object.assign({}, scenario.storage)
  const session = {}
  const answers = scenario.answers || {}
  let current = null
  let styles = []
  const timers = new Map()
  let nextTimer = 1
  function start() {
    const head = new Element('head')
    const body = new Element('body')
    const description = new Element('script')
    description.id = 'polyfinWebThemes'
    description.textContent = JSON.stringify(scenario.customCssOnly ? cssOnlyPage : page)
    head.appendChild(description)
    styles = []
    timers.clear()
    const listeners = {}
    const location = { href: 'http://polyfin.test/web/', hash: '', reload: () => log.push('reload'), replace: (url) => log.push('replace ' + url) }
    const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
    const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
    const document = {
      readyState: 'loading',
      head,
      body,
      documentElement: { lang: 'en' },
      getElementById: (id) => head.children.concat(body.children).find((element) => element.id === id) || null,
      getElementsByTagName: (tag) => head.children.concat(body.children, styles).filter((element) => element.tagName === tag.toUpperCase()),
      createElement: (tag) => new Element(tag),
      querySelector: () => null,
      querySelectorAll: () => [],
      write(html) {
        let match
        if ((match = /^<link rel="stylesheet" id="([^"]+)" href="([^"]+)">$/.exec(html))) {
          const link = new Element('link')
          link.rel = 'stylesheet'
          link.id = match[1]
          link.setAttribute('href', match[2])
          head.appendChild(link)
        } else if ((match = /^<script src="([^"]+)" defer><\/script>$/.exec(html))) {
          const element = new Element('script')
          element.src = match[1]
          head.appendChild(element)
        } else log.push('wrote ' + html)
      },
      set cookie(value) { log.push('cookie ' + value.split(';')[0].replace(page.Cookie + '=', '')) },
    }
    const ApiClient = {
      serverId: () => 's',
      getCurrentUserId: () => current,
      getUrl: (path) => 'http://polyfin.test/' + path,
      getJSON: (url) => {
        if (!url.endsWith('/Polyfin/WebTheme')) return new Promise(() => {})
        log.push('asked for ' + current)
        return Promise.resolve(answers[current])
      },
    }
    const CSS = { supports: (query) => query === 'selector(:has(a))' && scenario.old !== 'css' }
    // jellyfin-web's SDK asks with a Request made with credentials: 'omit'.
    const fetch = (input, init) => {
      log.push('fetch ' + input.url.replace('http://polyfin.test', '') + ' ' + JSON.stringify(init))
      return Promise.resolve()
    }
    const window = { ApiClient, CSS, fetch, localStorage: storageOf(local, scenario.failing), sessionStorage: storageOf(session, false) }
    class MutationObserver { observe() {} disconnect() {} }
    const context = {
      location, history, document, window, CSS, MutationObserver,
      setTimeout: (run) => { const id = nextTimer++; timers.set(id, run); return id },
      clearTimeout: (id) => { timers.delete(id) },
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    }
    if (scenario.old === 'syntax') context.Function = function () { throw new SyntaxError('Unexpected token') }
    if (scenario.database) {
      context.indexedDB = {
        open() {
          const request = {}
          setImmediate(() => {
            request.result = {
              objectStoreNames: { contains: (store) => store === 'keyval' },
              transaction: () => ({ objectStore: () => ({ delete: (key) => log.push('deleted ' + key) }) }),
              close() {},
            }
            request.onsuccess && request.onsuccess()
          })
          return request
        },
      }
    }
    vm.runInNewContext(script, context)
    document.readyState = 'complete'
    return { head, body, history, window }
  }
  const sheet = (loaded) => ({ disabled: false, cssRules: [{ styleSheet: { cssRules: loaded ? [{}] : [] } }] })
  let page_ = start()
  await flush()
  for (const step of scenario.steps) {
    if (step === 'head') {
      const shown = (parent) => parent.children
        .filter((element) => (element.tagName === 'LINK' && element.rel === 'stylesheet') || (element.tagName === 'SCRIPT' && element.src))
        .map((element) => (element.tagName === 'LINK' ? 'link ' + element.getAttribute('href') : 'script ' + element.src))
      const inHead = shown(page_.head)
      const inBody = shown(page_.body)
      log.push('head: ' + (inHead.length ? inHead.join(', ') : 'none') + (inBody.length ? '; body: ' + inBody.join(', ') : ''))
    } else if (step === 'kept') {
      const value = JSON.parse(local.polyfinWebTheme)
      log.push('kept users=' + value.Users.map((entry) => entry.join(':')).join(',') + ' last=' + value.Last + ' applied=' + value.Applied)
    } else if (step === 'styles') {
      log.push('styles: ' + styles.map((style) => /url\('([^']+)'\)/.exec(style.textContent)[1] + (style.sheet.disabled ? ' off' : ' on')).join(', '))
    } else if (step.startsWith('signin ')) {
      current = step.slice(7)
      page_.history.pushState(null, '', '#/home')
    } else if (step.startsWith('user ')) {
      current = step.slice(5)
    } else if (step.startsWith('fetch ')) {
      page_.window.fetch({ url: 'http://polyfin.test' + step.slice(6), credentials: 'omit' }, { cache: 'no-store' })
    } else if (step.startsWith('jellyfin')) {
      const style = new Element('style')
      style.textContent = "@import url('" + step.split(' ')[1] + "');"
      style.sheet = sheet(!step.startsWith('jellyfin-loading'))
      styles.push(style)
    } else if (step === 'loaded') {
      styles.forEach((style) => { style.sheet = sheet(true) })
    } else if (step === 'tick') {
      const due = [...timers.values()]
      timers.clear()
      due.forEach((run) => run())
    } else if (step === 'load') {
      current = null
      page_ = start()
    } else log.push('unknown step ' + step)
    await flush()
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
		Storage       map[string]string         `json:"storage"`
		Failing       bool                      `json:"failing"`
		CustomCssOnly bool                      `json:"customCssOnly"`
		Old           string                    `json:"old"`
		Database      bool                      `json:"database"`
		Answers       map[string]webThemeAnswer `json:"answers"`
		Steps         []string                  `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.storage, s.failing, s.customCssOnly, s.old, s.database, s.answers, s.steps}
	}
	page := pageThemes{Cookie: webtheme.Cookie("s"), Theme: accounts.WebThemeLumaaGlaass, Themes: map[string]pageTheme{
		accounts.WebThemeLumaaGlaass: {Css: lumaaGlaass, Js: "lg/loader.js"},
		accounts.WebThemeCustom:      {Css: custom, Js: "custom.js?v=1"},
	}}
	cssOnlyPage := pageThemes{Cookie: page.Cookie, Theme: accounts.WebThemeLumaaGlaass, Themes: map[string]pageTheme{
		accounts.WebThemeLumaaGlaass: page.Themes[accounts.WebThemeLumaaGlaass],
		accounts.WebThemeCustom:      {Css: custom},
	}}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "page": page, "cssOnlyPage": cssOnlyPage, "scenarios": inputs})
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
