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
//   - reads: the reads of the theme token's computed style so far.
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
// The theme's token as the stylesheet gives it (only the stylesheet sets
// --lg-motion), and the times it is read.
let motion = ' normal'
let reads = 0
const getComputedStyle = (node) => ({
  getPropertyValue(name) { if (node === root && name === '--lg-motion') { reads++; return motion } return '' },
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
  CSS: { supports: () => true, escape: (value) => String(value) },
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

// runTheme runs the theme's script in themePage, then steps, which may
// await, and returns what the steps logged.
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
	harness := themePage + ";(async () => {\ntry {\n" + steps + "\n} catch (error) {\n  log.push('harness: ' + error.stack)\n}\nprocess.stdout.write(JSON.stringify(log))\n})()\n"
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

// The random carousel reads its total from the random listing its window
// pages through: Polyfin draws among a library's first titles and reports
// those as the listing's total, so a window placed under the total of the
// unsorted library (8,111 titles) would be past the draw and empty
// (patches/0006-random-carousel-pool.patch in third_party/lumaaglaass).
func TestTheRandomCarouselStaysInTheDraw(t *testing.T) {
	t.Parallel()
	got := runTheme(t, `
// randomItems alone, with the highest random index and no shuffle.
const at = script.indexOf('async function randomItems(')
const randomItems = vm.runInNewContext(script.slice(at, script.indexOf('\n    }\n', at) + 6) + ';randomItems',
  { shuffle: list => list, Math: Object.assign(Object.create(Math), { random: () => 0.999 }) })
// Polyfin's answers: a draw of 100 titles at random, the library's 8,111
// otherwise.
const titles = (from, count) => Array.from({ length: Math.max(0, count) }, (_, i) => ({ Id: 'title-' + (from + i) }))
const api = {
  async getItems(userId, query) {
    const total = query.SortBy === 'Random' ? 100 : 8111
    const from = query.StartIndex || 0
    log.push('listing: ' + (query.SortBy || 'unsorted') + ' from ' + from + ' limit ' + query.Limit + ' of ' + query.ParentId)
    return { Items: titles(from, Math.min(query.Limit, total - from)), TotalRecordCount: total }
  },
}
const items = await randomItems(api, 'user', { ParentId: 'movies' }, 10)
log.push('items: ' + items.map(item => item.Id).join(' '))
`)
	want := []string{
		"listing: Random from 0 limit 1 of movies",
		"listing: Random from 90 limit 10 of movies",
		"items: title-90 title-91 title-92 title-93 title-94 title-95 title-96 title-97 title-98 title-99",
	}
	if !slices.Equal(got, want) {
		t.Errorf("\n got  %q\n want %q", got, want)
	}
}

// themeMarks gives <html> a class list that keeps its classes, logged by
// marks(): the theme's marks on <html> are what its stylesheet reads.
const themeMarks = `
const classes = new Set()
root.classList = {
  add: (name) => classes.add(name), remove: (name) => classes.delete(name), contains: (name) => classes.has(name),
  toggle(name, on = !classes.has(name)) { if (on) classes.add(name); else classes.delete(name); return on },
}
const marks = () => [...classes].filter(name => /^lg-is-(low-end|playback-page)$/.test(name)).sort().join(' ') || 'none'
const count = (name, change) => {
  const before = passes
  change()
  log.push(name + ': ' + (passes - before) + ' pass' + (passes - before === 1 ? '' : 'es'))
}
`

// While the player shows, its controls change many times a second: those
// changes, image loads in it and the clock run no pass of the theme. What
// opens over the player (outside it) and leaving it still do
// (patches/0007-low-end-devices.patch in third_party/lumaaglaass).
func TestTheThemeRunsNoPassesDuringPlayback(t *testing.T) {
	t.Parallel()
	got := runTheme(t, themeMarks+`
context.location.hash = '#/video'
start()
frame()
log.push('marks: ' + marks())
// An element of the player's controls.
const control = make('div')
control.closest = () => control
count('the controls change, the clock ticks, an image loads', () => {
  for (const o of observers) o.fn([{ type: 'childList', target: control, addedNodes: [make('div')], removedNodes: [] }])
  for (const fn of clocks) fn()
  fire(documentListeners, 'load', { type: 'load', target: control })
})
count('their frame', frame)
count('a menu opens over the player', batch)
count('its frame', frame)
context.location.hash = '#/details'
count('the player closes', () => fire(windowListeners, 'hashchange', { type: 'hashchange' }))
count('its frame', frame)
log.push('marks: ' + marks())
count('a clock tick', () => { for (const fn of clocks) fn() })
count('its frame', frame)
`)
	want := []string{
		"marks: lg-is-playback-page",
		"the controls change, the clock ticks, an image loads: 0 passes",
		"their frame: 0 passes",
		"a menu opens over the player: 0 passes",
		"its frame: 1 pass",
		"the player closes: 0 passes",
		"its frame: 1 pass",
		"marks: none",
		"a clock tick: 0 passes",
		"its frame: 1 pass",
	}
	if !slices.Equal(got, want) {
		t.Errorf("\n got  %q\n want %q", got, want)
	}
}

// A device with under 4 GB of memory or under 4 logical processors gets
// lg-is-low-end on <html>, which draws it as for reduced transparency;
// stop takes it back (patches/0007-low-end-devices.patch).
func TestTheThemeMarksLowEndDevices(t *testing.T) {
	t.Parallel()
	devices := map[string]struct {
		navigator string
		want      []string
	}{
		"2 GB, 8 processors":        {`{ deviceMemory: 2, hardwareConcurrency: 8 }`, []string{"marks: lg-is-low-end", "stopped: none"}},
		"8 GB, 2 processors":        {`{ deviceMemory: 8, hardwareConcurrency: 2 }`, []string{"marks: lg-is-low-end", "stopped: none"}},
		"4 GB, 4 processors":        {`{ deviceMemory: 4, hardwareConcurrency: 4 }`, []string{"marks: none", "stopped: none"}},
		"no memory, 8 processors":   {`{ hardwareConcurrency: 8 }`, []string{"marks: none", "stopped: none"}},
		"neither (another browser)": {`{}`, []string{"marks: none", "stopped: none"}},
	}
	for name, device := range devices {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := runTheme(t, themeMarks+`
context.navigator = { language: 'en', userAgent: 'node', ...`+device.navigator+` }
start()
log.push('marks: ' + marks())
theme().stop()
log.push('stopped: ' + marks())
`)
			if !slices.Equal(got, device.want) {
				t.Errorf("\n got  %q\n want %q", got, device.want)
			}
		})
	}
}

// The home's backdrop and a page card's are blurred: they are asked at half
// the size that covers the window, 960 pixels wide on a phone or a 1080p
// screen, where 1920 were (patches/0007-low-end-devices.patch).
func TestPageBackdropsAreAskedAtTheWindowsSize(t *testing.T) {
	t.Parallel()
	got := runTheme(t, `
const at = script.indexOf('    const PAGE_BACKDROP_WIDTHS')
const source = script.slice(at, script.indexOf('\n    }\n', at) + 6)
for (const [width, height] of [[360, 800], [390, 844], [1440, 900], [1920, 1080], [2560, 1440], [3840, 2160]]) {
  const pageBackdropWidth = vm.runInNewContext(source + ';pageBackdropWidth', { innerWidth: width, innerHeight: height })
  log.push(width + 'x' + height + ': ' + pageBackdropWidth())
}
`)
	want := []string{"360x800: 960", "390x844: 960", "1440x900: 960", "1920x1080: 960", "2560x1440: 1280", "3840x2160: 1920"}
	if !slices.Equal(got, want) {
		t.Errorf("\n got  %q\n want %q", got, want)
	}
}
