package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

var (
	// loaderAddress finds what the default Custom CSS imports and what the
	// default Custom JavaScript loads.
	loaderAddress = regexp.MustCompile(`(?:url\(|\.src = )'([^']+)'`)
	// extensionAddress finds the files the theme's script and its
	// extensions load beside it.
	extensionAddress = regexp.MustCompile(`'(extensions/[\w.-]+\.(?:js|css))'`)
	// outsideAddress finds an address with a scheme or a host.
	outsideAddress = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s'"\x60)]*|['"(]//[a-z0-9.-]+`)
	// notLoaded are the theme's addresses that name nothing to load: the
	// namespace of its inline SVG images, and the examples its settings
	// page shows in the empty field of a profile's address.
	notLoaded = []string{"http://www.w3.org/2000/svg", "https://example.com/profile.json", "https://exemple.com/profil.json"}
)

// themeFile is a file of the default theme as the web client served it.
type themeFile struct {
	target   string
	response *httptest.ResponseRecorder
}

// defaultThemeFiles are the files the default Custom CSS and JavaScript
// load from the web client of h, and those the theme's script loads in
// turn, each as served; it fails when the defaults load anything from
// outside Polyfin.
func defaultThemeFiles(t *testing.T, h http.Handler) []themeFile {
	t.Helper()
	var targets []string
	for _, code := range []string{accounts.DefaultCustomCss, accounts.DefaultCustomJs} {
		for _, match := range loaderAddress.FindAllStringSubmatch(code, -1) {
			address, err := url.Parse(match[1])
			if err != nil || address.Scheme != "" || address.Host != "" || strings.HasPrefix(match[1], "/") {
				t.Fatalf("the default custom code loads %q, not from the web client's own address", match[1])
			}
			// The page of jellyfin-web is /web/, which relative addresses
			// are taken from.
			targets = append(targets, "/web/"+match[1])
		}
	}
	if len(targets) != 2 {
		t.Fatalf("the default custom code loads %q, want a stylesheet and a script", targets)
	}
	var files []themeFile
	for len(targets) > 0 {
		target := targets[0]
		targets = targets[1:]
		if slices.ContainsFunc(files, func(f themeFile) bool { return f.target == target }) {
			continue
		}
		response := get(h, target)
		files = append(files, themeFile{target, response})
		if path.Ext(target) != ".js" {
			continue
		}
		// They are named from the theme's assets folder, the script's.
		assets, _, _ := strings.Cut(target, "/assets/")
		for _, match := range extensionAddress.FindAllStringSubmatch(response.Body.String(), -1) {
			targets = append(targets, assets+"/assets/"+match[1])
		}
	}
	return files
}

// The default theme loads from Polyfin itself: its stylesheet, its script
// and the extensions the script loads are served by the web client, with
// their types and cached for good at their versioned address. Another
// version, such as the one the theme's script asks for when it cannot tell
// its own address, leads to today's files, revalidated.
func TestDefaultThemeIsServedByPolyfin(t *testing.T) {
	h := webHandler(setupState{})
	files := defaultThemeFiles(t, h)
	types := map[string]string{".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8"}
	var extensions int
	for _, file := range files {
		if strings.Contains(file.target, "/extensions/") {
			extensions++
		}
		header := file.response.Header()
		if file.response.Code != http.StatusOK || file.response.Body.Len() == 0 {
			t.Errorf("%s: %d, %d bytes", file.target, file.response.Code, file.response.Body.Len())
			continue
		}
		if got, want := header.Get("Content-Type"), types[path.Ext(file.target)]; got != want {
			t.Errorf("%s: Content-Type %q, want %q", file.target, got, want)
		}
		if got := header.Get("Cache-Control"); got != immutableCache {
			t.Errorf("%s: Cache-Control %q, want %q", file.target, got, immutableCache)
		}
		if header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", file.target)
		}
		if file.response.Body.Len() < minCompressed {
			continue
		}
		request := httptest.NewRequest(http.MethodGet, file.target, nil)
		request.Header.Set("Accept-Encoding", "gzip")
		compressed := httptest.NewRecorder()
		h.ServeHTTP(compressed, request)
		if compressed.Code != http.StatusOK || compressed.Header().Get("Content-Encoding") != "gzip" ||
			compressed.Header().Get("Cache-Control") != immutableCache {
			t.Errorf("%s, gzip: %d, %v", file.target, compressed.Code, compressed.Header())
		}
	}
	// The settings page and its stylesheet, at least.
	if extensions < 2 {
		t.Errorf("the theme's script loads %d extensions from Polyfin", extensions)
	}

	script := files[1].target
	versioned := regexp.MustCompile(`/[^/]+/assets/`)
	for _, tc := range []struct{ target, location string }{
		{versioned.ReplaceAllString(script, "/latest/assets/"), script},
		{versioned.ReplaceAllString(script, "/0123456789abcdef/assets/") + "?v=2", script + "?v=2"},
		{"/jellyfin" + versioned.ReplaceAllString(script, "/latest/assets/"), "/jellyfin" + script},
	} {
		response := get(h, tc.target)
		if response.Code != http.StatusFound || response.Header().Get("Location") != tc.location ||
			response.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d to %q (%q), want %d to %q", tc.target, response.Code, response.Header().Get("Location"),
				response.Header().Get("Cache-Control"), http.StatusFound, tc.location)
		}
	}
	for _, target := range []string{path.Dir(script) + "/missing.js", path.Dir(script)} {
		if response := get(h, target); response.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want %d", target, response.Code, http.StatusNotFound)
		}
	}
}

// Nothing of the default theme comes from outside Polyfin: neither the
// default Custom CSS and JavaScript, nor any file of the theme they load,
// names an address elsewhere.
func TestDefaultThemeLoadsNothingFromOutside(t *testing.T) {
	h := webHandler(setupState{})
	sources := map[string]string{"Custom CSS": accounts.DefaultCustomCss, "Custom JavaScript": accounts.DefaultCustomJs}
	for _, file := range defaultThemeFiles(t, h) {
		if file.response.Code != http.StatusOK {
			t.Fatalf("%s: %d", file.target, file.response.Code)
		}
		sources[file.target] = file.response.Body.String()
	}
	for name, source := range sources {
		for _, address := range outsideAddress.FindAllString(source, -1) {
			if !slices.Contains(notLoaded, address) {
				t.Errorf("%s names %q", name, address)
			}
		}
	}
}

// The theme's settings page starts from the options the default Custom
// JavaScript gives the theme: a user who never saved it gets the home
// carousel off, the server's choice, and the page's other choices as the
// theme recommends them; what a user saved stays theirs, and a script
// without options keeps the theme's own defaults. The theme's settings
// extension runs in Node.js, in a context that stands for the browser and
// for the theme's main script, after the default Custom JavaScript.
func TestDefaultThemeSettingsStartFromTheLoaderOptions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	var extension string
	for _, file := range defaultThemeFiles(t, webHandler(setupState{})) {
		if strings.HasSuffix(file.target, "/extensions/preferences.js") && file.response.Code == http.StatusOK {
			extension = file.response.Body.String()
		}
	}
	if extension == "" {
		t.Fatal("the default theme loads no settings page from Polyfin")
	}
	const harness = `
const vm = require('node:vm')
const { loader, extension, cases } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const run = async ({ script, saved }) => {
  const applied = []
  const node = () => ({
    style: { removeProperty () {}, setProperty () {}, getPropertyValue: () => '' },
    setAttribute () {}, hasAttribute: () => false, append () {}, appendChild () {}, remove () {}, addEventListener () {}
  })
  const document = {
    documentElement: node(), head: node(), styleSheets: [],
    createElement: node, getElementById: () => null, querySelector: () => null, querySelectorAll: () => []
  }
  // A signed-in user, with the theme's settings they saved, if any.
  const api = {
    getCurrentUserId: () => 'user', serverId: () => 'server',
    getDisplayPreferences: async () => ({ CustomPrefs: saved ? { settings: JSON.stringify(saved) } : {} })
  }
  const context = {
    document, location: { hash: '#/home' }, URLSearchParams, console,
    MutationObserver: class { observe () {} disconnect () {} },
    requestAnimationFrame: () => 0, cancelAnimationFrame () {},
    matchMedia: () => ({ matches: false, addEventListener () {}, removeEventListener () {} }),
    localStorage: { getItem: () => null, setItem () {}, removeItem () {} },
    addEventListener () {}, removeEventListener () {}
  }
  context.window = context
  vm.runInNewContext(script ? loader : '', context)
  // The main script hands its options over, with the theme's own defaults
  // for those not set.
  const options = context.LumaaGlaassOptions || {}
  const flag = (name, fallback) => typeof options[name] === 'boolean' ? options[name] : fallback
  const defaults = Object.freeze({ homeCarousel: flag('homeCarousel', true),
    homeLibraryNamesOnly: flag('homeLibraryNamesOnly', false), collectionFilter: flag('collectionFilter', true) })
  context.__lumaaGlaass = {
    selects: { create: () => ({}) },
    preferences: {
      assets: '', assetUrl: (path) => path, defaults, api: () => api,
      sessionKey: (c) => c.serverId() + ':' + c.getCurrentUserId(),
      element: node, setAttribute () {}, translate: () => null, language: () => 'en',
      params: () => new URLSearchParams(), route: () => '#/home', sameId: (a, b) => a === b, enabled: () => true,
      applyCore: (values) => applied.push(values)
    }
  }
  vm.runInNewContext(extension, context)
  await new Promise((resolve) => setTimeout(resolve, 0))
  return applied.at(-1)
}
;(async () => {
  const results = {}
  for (const [name, tc] of Object.entries(cases)) results[name] = await run(tc)
  process.stdout.write(JSON.stringify(results))
})()
`
	type settings struct{ HomeCarousel, HomeLibraryNamesOnly, CollectionFilter bool }
	type testCase struct {
		Script bool           `json:"script"`
		Saved  map[string]any `json:"saved"`
		want   settings
	}
	cases := map[string]testCase{
		"never saved":      {Script: true, want: settings{false, true, true}},
		"saved":            {Script: true, Saved: map[string]any{"homeCarousel": true, "collectionFilter": false}, want: settings{true, true, false}},
		"saved, partly":    {Script: true, Saved: map[string]any{"homeLibraryNamesOnly": false}, want: settings{false, false, true}},
		"without options":  {want: settings{true, true, true}},
		"saved, no option": {Saved: map[string]any{"homeCarousel": false}, want: settings{false, true, true}},
	}
	input, _ := json.Marshal(map[string]any{"loader": accounts.DefaultCustomJs, "extension": extension, "cases": cases})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string]*settings
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	for name, tc := range cases {
		if got := results[name]; got == nil || *got != tc.want {
			t.Errorf("%s: the theme applies %+v, want %+v", name, got, tc.want)
		}
	}
}
