package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// While jellyfin-web's spinner turns, the script asks how the play's
// torrent starts, every second after each answer, and writes it under the
// spinner in the page's language; it asks nothing while the spinner does not
// turn, and the line goes with the spinner. It runs in Node.js, in a context
// that stands for the browser and jellyfin-web.
func TestTheSpinnerTellsHowATorrentStarts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type scenario struct {
		// lang is the document's language.
		lang string
		// steps: "spin" has jellyfin-web show its spinner, making it the
		// first time; "stop" hides it; "answer <json>" is what Polyfin
		// answers the next asks; "tick" runs the ask waiting for its second;
		// "show" logs the line, or "none"; "where" logs where it is placed,
		// the spinner showing 40 pixels wide from 680, its bottom at 110, as
		// a web theme moves it; "asks" logs how many asks were made.
		steps []string
		want  []string
	}
	peers := `answer {"Stage":"peers","PeersFound":0,"PeersConnected":0}`
	found := `answer {"Stage":"details","PeersFound":12,"PeersConnected":3}`
	pieces := `answer {"Stage":"pieces","PeersFound":12,"PeersConnected":3,"Received":2400000,"Waiting":1048576}`
	one := `answer {"Stage":"pieces","PeersFound":4,"PeersConnected":1,"Received":150000}`
	failed := `answer {"Stage":"failed","Failure":"no_trackers"}`
	none := `answer {"Stage":""}`
	scenarios := map[string]scenario{
		"a start told as it goes": {lang: "en-US",
			steps: []string{peers, "spin", "show", found, "tick", "show", pieces, "tick", "show", one, "tick", "show"},
			want: []string{"Finding peers…", "Finding peers: 12 found, 3 connected", "Receiving the start: 2.4 MB from 3 peers",
				"Receiving the start: 0.2 MB from 1 peer"}},
		"the line goes with the spinner, and the asks stop": {lang: "en",
			steps: []string{pieces, "spin", "show", "stop", "show", "tick", "tick", "asks", "spin", "asks", "show"},
			want:  []string{"Receiving the start: 2.4 MB from 3 peers", "none", "asks 1", "asks 2", "Receiving the start: 2.4 MB from 3 peers"}},
		"nothing asked while no spinner turns": {lang: "en",
			steps: []string{pieces, "tick", "asks", "show"},
			want:  []string{"asks 0", "none"}},
		"a spinner that is no torrent's start shows no line": {lang: "en",
			steps: []string{none, "spin", "show", "tick", "show"},
			want:  []string{"none", "none"}},
		"why it did not start": {lang: "en",
			steps: []string{failed, "spin", "show"},
			want:  []string{"The torrent did not start: the addon named no tracker, and none was found otherwise"}},
		"the line is placed under the spinner where it shows": {lang: "en",
			steps: []string{pieces, "spin", "show", "where"},
			want:  []string{"Receiving the start: 2.4 MB from 3 peers", "at 122px 700px"}},
		"in French": {lang: "fr-FR",
			steps: []string{pieces, "spin", "show", found, "tick", "show"},
			want:  []string{"Réception du début : 2,4 Mo depuis 3 pairs", "Recherche de pairs : 12 trouvés, 3 connectés"}},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  const listeners = {}
  const observers = new Set()
  let queued = false
  const changed = () => {
    if (queued) return
    queued = true
    queueMicrotask(() => {
      queued = false
      for (const observer of [...observers]) if (observers.has(observer)) try { observer.callback([]) } catch (error) {}
    })
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe() { observers.add(this) }
    disconnect() { observers.delete(this) }
  }
  const classes = (element) => String(element.className || '').split(' ').filter(Boolean)
  const matches = (element, selector) => /^(\.[\w-]+)+$/.test(selector) && selector.slice(1).split('.').every((c) => classes(element).includes(c))
  const all = (root, selector) => (root.children || []).flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
  class Element {
    constructor(tag) { this.tagName = tag.toUpperCase(); this._className = ''; this.id = ''; this.children = []; this.parentNode = null; this.text = ''; this.attributes = {}; this.handlers = {}; this.style = {} }
    get className() { return this._className }
    set className(value) { this._className = value; changed() }
    get classList() {
      return {
        contains: (c) => classes(this).includes(c),
        add: (c) => { if (!classes(this).includes(c)) this.className = [...classes(this), c].join(' ') },
        remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' ') },
        toggle: (c, force) => { (force === undefined ? !classes(this).includes(c) : force) ? this.classList.add(c) : this.classList.remove(c) },
      }
    }
    set textContent(text) { this.text = text; this.children = []; changed() }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    appendChild(child) { if (child.parentNode) child.parentNode.removeChild(child); this.children.push(child); child.parentNode = this; changed(); return child }
    insertBefore(child, before) { return this.appendChild(child) }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; changed(); return child }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    setAttribute(name, value) { this.attributes[name] = String(value) }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    removeAttribute(name) { delete this.attributes[name] }
    addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler) }
    closest() { return null }
    contains() { return false }
  }
  const body = new Element('body')
  const sheets = []
  const document = {
    body,
    documentElement: { lang: scenario.lang, classList: { contains: () => false } },
    head: { appendChild: (sheet) => sheets.push(sheet) },
    createElement: (tag) => new Element(tag),
    createTextNode: (text) => ({ textContent: text, parentNode: null }),
    getElementById: (id) => sheets.find((sheet) => sheet.id === id) || null,
    querySelector: (selector) => body.querySelector(selector),
    querySelectorAll: (selector) => body.querySelectorAll(selector),
  }
  // The script's own timers: the ask waiting for its second runs on "tick".
  let timers = []
  const setTimeout = (run, delay) => { const timer = { run, delay }; timers.push(timer); return timer }
  const clearTimeout = (timer) => { timers = timers.filter((t) => t !== timer) }
  let answer = null
  let asks = 0
  const store = {}
  const localStorage = { getItem: (name) => (name in store ? store[name] : null), setItem: (name, value) => { store[name] = String(value) }, removeItem: (name) => { delete store[name] } }
  const ApiClient = {
    serverId: () => 's',
    getCurrentUserId: () => 'u',
    getCurrentUser: () => Promise.reject(new Error('not asked here')),
    getUrl: (path) => 'http://polyfin.test/' + path,
    getJSON: (url) => {
      if (url !== 'http://polyfin.test/Polyfin/Playback/Starting') return Promise.reject(new Error('not asked here'))
      asks++
      return Promise.resolve(JSON.parse(answer))
    },
    ajax: () => Promise.reject(new Error('not asked here')),
  }
  const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home' }
  const history = { pushState: () => {}, replaceState: () => {} }
  vm.runInNewContext(script, {
    location, history, URL, URLSearchParams, document, MutationObserver, Object, JSON, String, Date, Number,
    window: { ApiClient, localStorage, sessionStorage: localStorage },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
    setTimeout, clearTimeout, setInterval: () => 0, clearInterval: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  await flush()
  let spinner = null
  for (const step of scenario.steps) {
    if (step.startsWith('answer ')) answer = step.slice(7)
    else if (step === 'spin') {
      // jellyfin-web's loading.show: the spinner made once, then turned on.
      if (!spinner) {
        spinner = new Element('div')
        spinner.className = 'docspinner mdl-spinner'
        spinner.getBoundingClientRect = () => ({ left: 680, width: 40, top: 70, bottom: 110, height: 40 })
        body.appendChild(spinner)
        await flush()
      }
      spinner.classList.add('mdlSpinnerActive')
    } else if (step === 'stop') spinner.classList.remove('mdlSpinnerActive')
    else if (step === 'tick') { const due = timers.filter((t) => t.delay === 1000); timers = timers.filter((t) => t.delay !== 1000); due.forEach((t) => t.run()) }
    else if (step === 'asks') log.push('asks ' + asks)
    else if (step === 'where') {
      const line = body.querySelector('.polyfinTorrentStart')
      log.push(line ? 'at ' + line.style.top + ' ' + line.style.left : 'none')
    }
    else if (step === 'show') {
      await flush()
      const line = body.querySelector('.polyfinTorrentStart')
      log.push(line ? line.textContent : 'none')
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
		Lang  string   `json:"lang"`
		Steps []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.lang, s.steps}
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
			t.Errorf("%s:\n got %q\nwant %q", name, got, s.want)
		}
	}
}
