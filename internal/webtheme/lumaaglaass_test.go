package webtheme

import (
	"encoding/json"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// themePage runs the theme's script, read from the standard input, in
// Node.js, in a page that holds nothing it decorates. The steps that follow
// it start the script (start), make the page's changes and log what the
// theme did (log), which the page prints at the end as JSON.
//   - batch: jellyfin-web changes the page, a batch of the observers;
//   - frame: the browser runs the frame callbacks asked for so far;
//   - passes: the theme's passes so far, each reading the session once;
//   - reads: the reads of the accent token's computed style so far.
const themePage = `
const vm = require('node:vm')
const script = require('node:fs').readFileSync(0, 'utf8')
class Element {
  constructor(tag) {
    this.tagName = String(tag).toUpperCase(); this.children = []; this.parentNode = null; this.dataset = {}; this.attributes = {}
    this.style = { setProperty() {}, removeProperty() {}, getPropertyValue: () => '' }
    this.classList = { add() {}, remove() {}, toggle() {}, contains: () => false }
  }
  get isConnected() { return true }
  setAttribute(n, v) { this.attributes[n] = String(v) } getAttribute(n) { return n in this.attributes ? this.attributes[n] : null }
  hasAttribute(n) { return n in this.attributes } removeAttribute(n) { delete this.attributes[n] } toggleAttribute() {}
  append(...nodes) { for (const n of nodes) this.appendChild(n) } prepend(...nodes) { this.append(...nodes) }
  appendChild(n) { if (n && typeof n === 'object') { this.children.push(n); n.parentNode = this } return n }
  insertBefore(n) { return this.appendChild(n) } replaceChildren() { this.children = [] } remove() {} removeChild(n) { return n }
  addEventListener() {} removeEventListener() {}
  querySelector() { return null } querySelectorAll() { return [] } closest() { return null } matches() { return false } contains() { return false }
  getClientRects() { return [] } getBoundingClientRect() { return { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0 } }
  animate() { return { cancel() {}, finished: Promise.resolve() } } focus() {} blur() {}
}
class HTMLElement extends Element {}
class HTMLStyleElement extends HTMLElement {}
class HTMLLinkElement extends HTMLElement {}
const make = (tag) => tag === 'style' ? new HTMLStyleElement(tag) : tag === 'link' ? new HTMLLinkElement(tag) : new HTMLElement(tag)
const root = make('html')
const body = make('body')
// The document's stylesheets, as document.styleSheets lists them.
const sheets = []
const documentListeners = {}
const document = {
  documentElement: root, head: make('head'), body, styleSheets: sheets, adoptedStyleSheets: [], readyState: 'complete', hidden: false, currentScript: null,
  createElement: make, createElementNS: make, createTextNode: (text) => ({ textContent: text }), createDocumentFragment: () => make('fragment'),
  getElementById: () => null, querySelector: () => null, querySelectorAll: () => [],
  addEventListener(type, fn) { (documentListeners[type] ||= []).push(fn) }, removeEventListener() {},
}
// The accent token as the stylesheets give it, and the times it is read.
let accent = ' #f5f5f7'
let reads = 0
const getComputedStyle = (node) => ({
  getPropertyValue(name) { if (node === root && name === '--lg-color-accent') { reads++; return accent } return '' },
  position: 'static', overflowY: 'auto', direction: 'ltr', backgroundImage: 'none',
})
// The frame callbacks asked for and not yet run or cancelled.
const frames = new Map()
let lastFrame = 0
const frame = () => { const due = [...frames.values()]; frames.clear(); for (const fn of due) fn(0) }
// The clocks the script starts (setInterval).
const clocks = []
const windowListeners = {}
const observers = []
const context = {
  document, location: { hash: '#/home', href: 'http://polyfin.test/web/#/home', pathname: '/web/' }, history: { pushState() {}, replaceState() {} },
  navigator: { language: 'en', userAgent: 'node' }, console, URL, URLSearchParams,
  Element, HTMLElement, HTMLStyleElement, HTMLLinkElement, Node: Element, getComputedStyle,
  MutationObserver: class { constructor(fn) { this.fn = fn; observers.push(this) } observe() {} disconnect() {} takeRecords() { return [] } },
  ResizeObserver: class { observe() {} unobserve() {} disconnect() {} },
  IntersectionObserver: class { observe() {} unobserve() {} disconnect() {} },
  matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }),
  requestAnimationFrame(fn) { frames.set(++lastFrame, fn); return lastFrame }, cancelAnimationFrame(id) { frames.delete(id) },
  setTimeout: () => 0, clearTimeout() {}, setInterval(fn) { clocks.push(fn); return clocks.length }, clearInterval() {},
  localStorage: { getItem: () => null, setItem() {}, removeItem() {} }, sessionStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  addEventListener(type, fn) { (windowListeners[type] ||= []).push(fn) }, removeEventListener() {},
  innerWidth: 1440, innerHeight: 900, scrollY: 0, devicePixelRatio: 1, fetch: () => new Promise(() => {}),
}
context.window = context
// Each pass of the theme reads the session's client once.
let passes = 0
Object.defineProperty(context, 'ApiClient', { get() { passes++; return undefined } })
const log = []
const fire = (listeners, type, event) => { for (const fn of listeners[type] || []) fn(event) }
// jellyfin-web adds an element.
const batch = () => { for (const o of observers) o.fn([{ type: 'childList', target: body, addedNodes: [make('div')], removedNodes: [] }]) }
const start = () => vm.runInNewContext(script, context)
const theme = () => context.__lumaaGlaass
`

// runTheme runs the theme's script in themePage, then steps, and returns
// what the steps logged.
func runTheme(t *testing.T, steps string) []string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	script, err := fs.ReadFile(Files, "assets/lumaaglaass.js")
	if err != nil {
		t.Fatal(err)
	}
	harness := themePage + "try {\n" + steps + "\n} catch (error) {\n  log.push('harness: ' + error.stack)\n}\nprocess.stdout.write(JSON.stringify(log))\n"
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(script))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var got []string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	return got
}

// The theme's script runs one pass per frame, whatever the number of
// jellyfin-web's batches of changes, routes and clock ticks in it: each
// pass reads the layout, which costs a style recalculation after each
// batch's writes. A page shown (viewbeforeshow, viewshow) still gets its
// pass at once, which leaves nothing due for the frame. A hidden page has
// no frames: its due pass runs as it hides or shows. Without frames, each
// change gets its pass at once; stop cancels the frame asked for
// (patches/0003-pass-per-frame.patch in third_party/lumaaglaass).
func TestTheThemeRunsOnePassPerFrame(t *testing.T) {
	t.Parallel()
	steps := map[string]struct {
		steps string
		want  []string
	}{
		"with frames": {
			steps: `
const count = (name, change) => {
  const before = passes
  change()
  log.push(name + ': ' + (passes - before) + ' pass' + (passes - before === 1 ? '' : 'es'))
}
start()
frame()
count('three batches', () => { batch(); batch(); batch() })
count('their frame', frame)
count('a frame with nothing due', frame)
count('a route, a clock tick and an image', () => {
  fire(windowListeners, 'hashchange', { type: 'hashchange' })
  fire(windowListeners, 'popstate', { type: 'popstate' })
  for (const fn of clocks) fn()
  fire(documentListeners, 'load', { type: 'load', target: make('img') })
})
count('their frame', frame)
count('a batch, then a page shown', () => { batch(); fire(documentListeners, 'viewbeforeshow', { type: 'viewbeforeshow', target: body }) })
count('the next frame', frame)
count('the page shown', () => fire(documentListeners, 'viewshow', { type: 'viewshow', target: body }))
count('a batch while hidden', () => { document.hidden = true; batch() })
count('the page hidden', () => fire(documentListeners, 'visibilitychange', { type: 'visibilitychange' }))
count('a batch while hidden', batch)
count('the page shown again', () => { document.hidden = false; fire(documentListeners, 'visibilitychange', { type: 'visibilitychange' }) })
count('a batch, then stop', () => { batch(); theme().stop() })
log.push('frames left: ' + frames.size)
`,
			want: []string{
				"three batches: 0 passes",
				"their frame: 1 pass",
				"a frame with nothing due: 0 passes",
				"a route, a clock tick and an image: 0 passes",
				"their frame: 1 pass",
				"a batch, then a page shown: 1 pass",
				"the next frame: 0 passes",
				"the page shown: 1 pass",
				"a batch while hidden: 0 passes",
				"the page hidden: 1 pass",
				"a batch while hidden: 0 passes",
				"the page shown again: 1 pass",
				"a batch, then stop: 0 passes",
				"frames left: 0",
			},
		},
		"without frames": {
			steps: `
delete context.requestAnimationFrame
delete context.cancelAnimationFrame
start()
const before = passes
batch(); batch(); batch()
log.push('three batches: ' + (passes - before) + ' passes')
`,
			want: []string{"three batches: 3 passes"},
		},
	}
	for name, test := range steps {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := runTheme(t, test.steps); !slices.Equal(got, test.want) {
				t.Errorf("\n got  %q\n want %q", got, test.want)
			}
		})
	}
}
