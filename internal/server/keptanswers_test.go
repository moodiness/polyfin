package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// jellyfin-web 12.2 keeps its answers in a React Query cache, in memory and
// in IndexedDB, some keyed without the user, such as the search page's
// Suggestions. When another user signs in than the one they were kept for,
// the script empties that cache, as jellyfin-web's own language change does,
// and deletes its copy in IndexedDB; on a load whose saved credentials sign
// in another user, it deletes the copy before jellyfin-web restores it. The
// same user, or signing out, keeps them. A browser without the database, or
// a jellyfin-web without the hooks, is left as it is. It runs in Node.js, in
// a context that stands for the browser and jellyfin-web.
func TestAnotherUserDoesNotGetTheAnswersKeptForTheLast(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// keptForKey is where the script keeps the user the cache is for.
	const keptForKey = "polyfinKeptFor"
	type server struct {
		Id               string
		UserId           string
		AccessToken      string `json:",omitempty"`
		DateLastAccessed int
	}
	type scenario struct {
		// keptFor is what storage holds under keptForKey as the script
		// loads, as server/user; empty for nothing.
		keptFor string
		// saved are the servers of jellyfin-web's saved credentials.
		saved []server
		// user is signed in on server "s" as the script loads; empty for none.
		user string
		// database is whether the browser has jellyfin-web's IndexedDB
		// database; bare is a jellyfin-web without LANGUAGE_CHANGE
		// listeners, a browser without IndexedDB and with storage off.
		database, bare bool
		// steps: a route the router moves to; "signin <user>" and "signout"
		// change the user jellyfin-web's client has, without a route; "kept"
		// logs what storage holds under keptForKey; "where" logs the route.
		steps []string
		// want logs "cache cleared" as jellyfin-web empties its cache,
		// "deleted <key>" as its copy in IndexedDB is deleted, and "upgrade
		// aborted" when the script would have made the database.
		want []string
	}
	const forgotten = "cache cleared,deleted jellyfin-query-cache"
	scenarios := map[string]scenario{
		"another user signing in drops the kept answers": {keptFor: "s/boss", database: true,
			steps: []string{"#/login", "signin viewer", "#/home", "kept", "#/search", "#/home"},
			want:  []string{"cache cleared", "deleted jellyfin-query-cache", "kept s/viewer"}},
		"a user switch without a sign-out drops them": {keptFor: "s/boss", user: "boss", database: true,
			saved: []server{{"s", "boss", "t", 1}},
			steps: []string{"#/search", "signin viewer", "#/home"},
			want:  strings.Split(forgotten, ",")},
		"the same user keeps them": {keptFor: "s/boss", user: "boss", database: true,
			saved: []server{{"s", "boss", "t", 1}},
			steps: []string{"#/home", "signout", "#/login", "signin boss", "#/home", "kept"},
			want:  []string{"kept s/boss"}},
		"signing out keeps them": {keptFor: "s/boss", user: "boss", database: true,
			saved: []server{{"s", "boss", "t", 1}},
			steps: []string{"signout", "#/login", "kept"},
			want:  []string{"kept s/boss"}},
		// The server used last, with a token, is the one jellyfin-web signs
		// in to; jellyfin-web adds its listeners after the script started.
		"saved credentials for another user drop the copy before it is restored": {keptFor: "s/boss", user: "viewer", database: true,
			saved: []server{{"old", "boss", "t", 1}, {"s", "viewer", "t", 2}, {"x", "boss", "", 3}},
			steps: []string{"#/home", "kept"},
			want:  []string{"deleted jellyfin-query-cache", "kept s/viewer"}},
		"a first sign-in drops what was kept before": {database: true,
			steps: []string{"#/login", "signin boss", "#/home", "kept"},
			want:  []string{"cache cleared", "deleted jellyfin-query-cache", "kept s/boss"}},
		"a browser without the database is left without one": {keptFor: "s/boss",
			steps: []string{"signin viewer", "#/home"},
			want:  []string{"cache cleared", "upgrade aborted"}},
		"a jellyfin-web without the hooks still routes": {keptFor: "s/boss", bare: true,
			steps: []string{"signin viewer", "#/home", "where"},
			want:  []string{"at #/home"}},
	}
	const harness = `
const vm = require('node:vm')
const { script, key, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  let user = scenario.user
  const listeners = {}
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: (url) => log.push('replace ' + url) }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  class MutationObserver { observe() {} disconnect() {} }
  const document = {
    head: { appendChild: () => {} },
    createElement: (tag) => (tag === 'style' ? { textContent: '' } : null),
    getElementById: () => null,
    querySelector: () => null,
    documentElement: { lang: 'en' },
  }
  const store = {}
  if (scenario.keptFor) store[key] = scenario.keptFor
  if (scenario.saved) store.jellyfin_credentials = JSON.stringify({ Servers: scenario.saved })
  const localStorage = { getItem: (name) => (name in store ? store[name] : null), setItem: (name, value) => { store[name] = String(value) } }
  const window = {
    ApiClient: null,
    get localStorage() {
      if (scenario.bare) throw new Error('SecurityError')
      return localStorage
    },
  }
  // IndexedDB, as idb-keyval uses it: database keyval-store, store keyval.
  const indexedDB = {
    open(name) {
      const request = {}
      setImmediate(() => {
        if (name !== 'keyval-store') return log.push('opened ' + name)
        if (!scenario.database) {
          request.transaction = { abort: () => log.push('upgrade aborted') }
          request.onupgradeneeded && request.onupgradeneeded()
          request.onerror && request.onerror({ preventDefault() {} })
          return
        }
        request.result = {
          objectStoreNames: { contains: (store) => store === 'keyval' },
          transaction: (store, mode) => ({ objectStore: () => ({ delete: (k) => log.push(mode === 'readwrite' ? 'deleted ' + k : 'read only') }) }),
          close() {},
        }
        request.onsuccess && request.onsuccess()
      })
      return request
    },
  }
  const ApiClient = { serverId: () => 's', getCurrentUserId: () => user || null, getUrl: (path) => 'http://polyfin.test/' + path, getJSON: () => new Promise(() => {}) }
  window.ApiClient = ApiClient
  const context = {
    location, history, URL, document, MutationObserver, window,
    setTimeout: () => 0, clearTimeout: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  }
  if (!scenario.bare) context.indexedDB = indexedDB
  vm.runInNewContext(script, context)
  // jellyfin-web, whose scripts run after this one, then listens to
  // language changes, which clear its query cache.
  if (!scenario.bare) document._callbacks = { LANGUAGE_CHANGE: [() => log.push('cache cleared')] }
  await flush()
  for (const step of scenario.steps) {
    if (step === 'kept') log.push('kept ' + (store[key] || 'nothing'))
    else if (step === 'where') log.push('at ' + location.hash)
    else if (step.startsWith('signin ')) user = step.slice(7)
    else if (step === 'signout') user = ''
    else history.pushState(null, '', step)
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
		KeptFor  string   `json:"keptFor,omitempty"`
		Saved    []server `json:"saved,omitempty"`
		User     string   `json:"user"`
		Database bool     `json:"database"`
		Bare     bool     `json:"bare"`
		Steps    []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.keptFor, s.saved, s.user, s.database, s.bare, s.steps}
	}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "key": keptForKey, "scenarios": inputs})
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
		got := results[name]
		if got == nil {
			got = []string{}
		}
		want := s.want
		if want == nil {
			want = []string{}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
	}
}
