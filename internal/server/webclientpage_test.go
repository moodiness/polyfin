package server

import (
	"encoding/json"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// scriptRunner is JavaScript the harnesses that run Polyfin's script in
// Node.js start with: runScript(script, context) runs it as
// vm.runInNewContext does, in context completed with what a browser has
// that the harness's stand-ins may lack: events on the document, on which
// the script tells the page's changes (see the start of the header of
// webclient.js), and Event. A listener added twice is added once, as in a
// browser.
const scriptRunner = `
const runScript = (script, context) => {
  const document = context.document
  if (document && typeof document.addEventListener !== 'function') {
    const listeners = new Map()
    const of = (type) => listeners.get(type) || []
    document.addEventListener = (type, listener) => { if (!of(type).includes(listener)) listeners.set(type, [...of(type), listener]) }
    document.removeEventListener = (type, listener) => { listeners.set(type, of(type).filter((kept) => kept !== listener)) }
    document.dispatchEvent = (event) => { for (const listener of of(event.type)) listener.call(document, event); return true }
  }
  if (!context.Event) context.Event = class Event { constructor(type) { this.type = type } }
  return require('node:vm').runInNewContext(script, context)
}
`

// Five parts of the script follow the whole page as jellyfin-web renders
// it (see the start of the header of webclient.js): the menus' entry, the
// home page's rows, the play buttons until the first answer, the
// reminders and the recent searches. One observer of the body serves them
// all, whatever the route, rather than one each; each part listens to the
// page's changes only on the routes it follows, and the changes of a
// frame, however many, are told once, as the frame is drawn. Each part's
// own test shows it does its work when told, and fails without the event:
// TestMembersGetAPolyfinEntry, TestTheHomePageGetsTheServersRows,
// TestPlayButtonsWaitForASource, TestProgrammePagesGetReminders and
// TestSearchPageListsTheUsersRecentSearches.
//
// Each step logs the route, the observers of the whole body, the parts
// listening, and, after the page changed 40 times, how many times each
// listening part was told before the frame was drawn and after.
func TestOnePageObserverServesTheFiveParts(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const title = "#/details?id=0123456789abcdef0123456789abcdef"
	scenarios := map[string][]string{
		"in a browser": {
			"#/home: 1 observer, 2 parts, told 0,0 then 1,1",
			title + ": 1 observer, 4 parts, told 0,0,0,0 then 1,1,1,1",
			"#/search?query=heat: 1 observer, 3 parts, told 0,0,0 then 1,1,1",
			"#/home: 1 observer, 2 parts, told 0,0 then 1,1",
		},
		// The script runs in the head: the observer waits for the body, as
		// the home page's rows do; the menus' entry listens from the start.
		"the body made after the script": {
			"#/home: 0 observers, 1 part",
			"#/home: 1 observer, 2 parts, told 0,0 then 1,1",
		},
		"without requestAnimationFrame, told at once": {
			"#/home: 1 observer, 2 parts, told 40,40",
		},
	}
	names := slices.Sorted(maps.Keys(scenarios))
	const harness = scriptRunner + `
const { script, names, title } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const results = {}
for (const name of names) {
  const log = []
  // An element that holds nothing jellyfin-web would draw.
  const element = () => ({
    children: [], style: {}, classList: { add() {}, remove() {}, contains: () => false, toggle() {} },
    querySelector: () => null, querySelectorAll: () => [], getAttribute: () => null, setAttribute() {}, removeAttribute() {},
    appendChild: (child) => child, insertBefore: (child) => child, removeChild: (child) => child, contains: () => false,
  })
  const body = element()
  // The document's events, each listener with the times it was told.
  const listening = new Map()
  const document = Object.assign(element(), {
    head: element(), documentElement: { lang: 'en' }, cookie: '', readyState: 'loading',
    body: name.includes('made after') ? null : body,
    createElement: element, createTextNode: () => ({}), getElementById: () => null,
    addEventListener: (type, listener) => { if (type === 'polyfinpagechange' && !listening.has(listener)) listening.set(listener, 0) },
    removeEventListener: (type, listener) => { if (type === 'polyfinpagechange') listening.delete(listener) },
    dispatchEvent: (event) => {
      if (event.type !== 'polyfinpagechange') return true
      for (const [listener, told] of [...listening]) { listening.set(listener, told + 1); listener.call(document, event) }
      return true
    },
  })
  const observers = []
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target, options) { observers.push({ observer: this, target, options }) }
    disconnect() { for (let i = observers.length - 1; i >= 0; i--) if (observers[i].observer === this) observers.splice(i, 1) }
  }
  const whole = () => observers.filter((o) => o.target === body && o.options.subtree)
  let frames = []
  const draw = () => { const due = frames; frames = []; due.forEach((run) => run(1000)) }
  const listeners = {}
  const fire = (type) => { for (const listener of listeners[type] || []) listener({ type }) }
  const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home', pathname: '/web/', search: '', replace: () => {} }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash; fire('hashchange') }
  const storage = { getItem: () => null, setItem: () => {}, removeItem: () => {} }
  const context = {
    location, history: { pushState: () => {}, replaceState: () => {} }, document, MutationObserver,
    window: { localStorage: storage, sessionStorage: storage }, Date,
    setTimeout: () => 0, clearTimeout: () => {}, setInterval: () => 0, clearInterval: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  }
  if (!name.includes('without requestAnimationFrame')) context.requestAnimationFrame = (run) => { frames.push(run); return frames.length }
  const step = (changes) => {
    const count = (n, one, many) => n + ' ' + (n === 1 ? one : many)
    let line = location.hash + ': ' + count(whole().length, 'observer', 'observers') + ', ' + count(listening.size, 'part', 'parts')
    if (changes) {
      for (const [listener] of listening) listening.set(listener, 0)
      // jellyfin-web renders the route: the page changes many times
      // before the next frame.
      for (let i = 0; i < changes; i++) for (const o of observers.filter((o) => o.target === body)) o.observer.callback([{ type: 'childList', target: body, addedNodes: [], removedNodes: [] }], o.observer)
      line += ', told ' + [...listening.values()].join(',')
      if (frames.length) {
        draw()
        line += ' then ' + [...listening.values()].join(',')
      }
    }
    log.push(line)
  }
  try {
    runScript(script, context)
    if (name.includes('made after')) {
      step(0)
      document.body = body
      document.readyState = 'interactive'
      fire('DOMContentLoaded')
    }
    step(40)
    if (name === 'in a browser') {
      for (const hash of [title, '#/search?query=heat', '#/home']) {
        go(hash)
        draw()
        step(40)
      }
    }
    results[name] = log
  } catch (error) {
    results[name] = log.concat('harness: ' + error.stack)
  }
}
process.stdout.write(JSON.stringify(results))
`
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "names": names, "title": title})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string][]string
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	for _, name := range names {
		if got, want := results[name], scenarios[name]; !slices.Equal(got, want) {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
	}
}
