package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The place of the home page and of search results (the script's
// twenty-second part): jellyfin-web's modern layout builds the page anew
// on Back, at the top while its rows come, so the script keeps a place for
// each history entry and brings it back on a popstate: the row at the top
// of the screen at its height, else the scroll, and each row's sideways
// position through jellyfin-web's scroller. The user's wheel, touch or key
// stops it, it gives up a second after the page reached its place, and it
// leaves alone the legacy layout, which shows the very page it kept, and
// the TV layout. A push of the home page while it shows goes to the top.
//
// The page stands for jellyfin-web's: rows of 200 or 300 pixels from 60
// pixels down, under a screen 800 pixels high, an empty row 0 pixels high;
// a scroller holds 10 titles of 150 pixels in 1000.
func TestHomeAndSearchKeepTheirPlaceOnBack(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type scenario struct {
		start  string
		layout string
		steps  []string
		want   []string
	}
	// The rows, from the top: My Media 60-260, Continue Watching 260-560,
	// Polyfin's Trending 560-860, Recently Added in Movies 860-1160,
	// Recently Added in Shows 1160-1460, Next Up 1460-1760.
	scenarios := map[string]scenario{
		"Back": {steps: []string{"scroll 900", "slide Trending 250", "open", "back", "mount", "frame", "y", "fill", "frame", "y", "sideways Trending"},
			want: []string{"y=0", "y=900", "Trending at 250"}},
		// jellyfin-web draws a row anew, its scroller at its start.
		"a row drawn anew": {steps: []string{"scroll 900", "slide Trending 250", "open", "back", "mount", "fill", "frame", "redraw Trending", "frame", "sideways Trending"},
			want: []string{"Trending at 250"}},
		// jellyfin-web's scroller goes back to its start as the row's size
		// changes, which the row's scroll tells.
		"a row back at its start": {steps: []string{"scroll 900", "slide Trending 250", "open", "back", "mount", "fill", "frame", "reset Trending", "sideways Trending"},
			want: []string{"Trending at 250"}},
		"a row changed meanwhile": {steps: []string{"scroll 900", "open", "back", "mount", "fill without Continue Watching", "frame", "y"},
			want: []string{"y=600"}},
		"the row at the top gone": {steps: []string{"scroll 500", "open", "back", "mount", "fill without Continue Watching", "frame", "y"},
			want: []string{"y=500"}},
		"search results": {start: "#/search?query=e", steps: []string{"scroll 700", "slide Movies 300", "open", "back", "mount", "frame", "fill", "frame", "y", "sideways Movies"},
			want: []string{"y=700", "Movies at 300"}},
		"the logo": {steps: []string{"scroll 900", "logo", "y"}, want: []string{"y=0"}},
		"Home on the home page, then two Backs": {steps: []string{"scroll 900", "home link", "y", "scroll 400", "open", "back", "mount", "fill", "frame", "y", "back", "y"},
			want: []string{"y=0", "y=400", "y=900"}},
		"a push elsewhere keeps the place": {steps: []string{"scroll 900", "open", "y", "back", "mount", "fill", "frame", "y"},
			want: []string{"y=0", "y=900"}},
		"a wheel stops it": {steps: []string{"scroll 900", "open", "back", "mount", "fill", "frame", "y", "open", "back", "mount", "frame", "wheel", "fill", "frame", "y"},
			want: []string{"y=900", "y=0"}},
		"a touch stops it": {steps: []string{"scroll 900", "open", "back", "mount", "fill", "frame", "y", "open", "back", "mount", "frame", "touch", "fill", "frame", "y"},
			want: []string{"y=900", "y=0"}},
		"a key stops it": {steps: []string{"scroll 900", "open", "back", "mount", "fill", "frame", "y", "open", "back", "mount", "frame", "key", "fill", "frame", "y"},
			want: []string{"y=900", "y=0"}},
		"given up a second after": {steps: []string{"scroll 900", "open", "back", "mount", "fill", "frame", "y", "wait", "fill without Continue Watching", "frame", "y"},
			want: []string{"y=900", "y=900"}},
		// The legacy layout shows the home page it kept, its place brought
		// back by jellyfin-web, here to 905.
		"the legacy layout": {layout: "layout-desktop", steps: []string{"scroll 900", "slide Trending 250", "open legacy", "back", "unhide", "frame", "y", "sideways Trending"},
			want: []string{"y=905", "Trending at 250"}},
		"the TV layout": {layout: "layout-tv", steps: []string{"scroll 900", "open", "back", "mount", "fill", "frame", "y"},
			want: []string{"y=0"}},
	}
	const harness = scriptRunner + `
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const homeRows = [['section0', 'My Media', 200], ['section1', 'Continue Watching', 300], ['polyfin', 'Trending', 300], ['', 'Recently Added in Movies', 300], ['', 'Recently Added in Shows', 300], ['section4', 'Next Up', 300]]
const searchRows = [['', 'Movies', 300], ['', 'Shows', 300], ['', 'Episodes', 300], ['', 'People', 300], ['', 'Collections', 300], ['', 'Songs', 300]]
function run(scenario) {
  const log = []
  const search = (scenario.start || '').startsWith('#/search')
  const classes = (element) => element.className.split(' ').filter(Boolean)
  const matches = (element, selector) => {
    if (selector[0] === '#') return element.id === selector.slice(1)
    return /^(\.[\w-]+)+$/.test(selector) && selector.slice(1).split('.').every((c) => classes(element).includes(c))
  }
  const all = (root, selector) => root.children.flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
  let scrollY = 0
  class Element {
    constructor(tag, className) { this.tagName = tag.toUpperCase(); this.className = className || ''; this.id = ''; this.children = []; this.parentNode = null; this.attributes = {}; this.text = ''; this.style = {}; this.height = 0 }
    get classList() {
      return {
        contains: (c) => classes(this).includes(c),
        add: (c) => { if (!classes(this).includes(c)) this.className = [...classes(this), c].join(' ') },
        remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' ') },
        toggle: () => {},
      }
    }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    set textContent(text) { this.text = text; this.children = [] }
    appendChild(child) { if (child.parentNode) child.parentNode.removeChild(child); this.children.push(child); child.parentNode = this; return child }
    insertBefore(child) { return this.appendChild(child) }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; return child }
    contains(node) { for (let at = node; at; at = at.parentNode) if (at === this) return true; return false }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    setAttribute(name, value) { this.attributes[name] = String(value) }
    removeAttribute(name) { delete this.attributes[name] }
    addEventListener() {}
    removeEventListener() {}
    closest(selector) { for (let at = this; at && at.tagName; at = at.parentNode) if (matches(at, selector)) return at; return null }
    // A row's place: under the rows before it in its container, while its
    // page shows.
    getBoundingClientRect() {
      const container = this.parentNode
      const page = this.closest('.page')
      if (!container || (page && page.classList.contains('hide'))) return { top: 0, bottom: 0, height: 0, left: 0, right: 0, width: 0 }
      let top = 60
      for (const row of container.children) { if (row === this) break; top += row.height }
      return { top: top - scrollY, bottom: top - scrollY + this.height, height: this.height, left: 0, right: 1000, width: 1000 }
    }
  }
  const html = new Element('html', scenario.layout || 'layout-desktop')
  const body = html.appendChild(new Element('body'))
  const shownRows = () => {
    const page = body.children.find((child) => !child.classList.contains('hide'))
    if (!page) return []
    const container = page.querySelector('.homeSectionsContainer') || page
    return container.children
  }
  const scrollHeight = () => {
    const page = body.children.find((child) => !child.classList.contains('hide'))
    if (page && page.classList.contains('details')) return 1200
    return 60 + shownRows().reduce((sum, row) => sum + row.height, 0) + 200
  }
  const scrollTo = (x, y) => { scrollY = Math.max(0, Math.min(Math.round(y), scrollHeight() - 800)) }
  const document = {
    body, documentElement: html, head: new Element('head'), cookie: '', readyState: 'complete',
    get scrollingElement() { return { get scrollHeight() { return scrollHeight() } } },
    createElement: (tag) => new Element(tag), createTextNode: () => new Element('#text'),
    getElementById: (id) => all(html, '#' + id)[0] || null,
    getElementsByTagName: () => [],
    querySelector: (selector) => html.querySelector(selector),
    querySelectorAll: (selector) => html.querySelectorAll(selector),
  }
  // A scroller as jellyfin-web makes one: its titles 150 pixels wide in
  // 1000, its position kept within them.
  const scroller = () => {
    const element = new Element('div', 'padded-top-focusscale emby-scroller')
    element.items = 0
    element.position = 0
    element.getScrollPosition = () => element.position
    element.scrollToPosition = (at) => { element.position = Math.max(0, Math.min(at, element.items * 150 - 1000)) }
    return element
  }
  const row = ([section, title, height], filled) => {
    const element = new Element('div', 'verticalSection' + (section && section !== 'polyfin' ? ' ' + section : '') + (section === 'polyfin' ? ' polyfinHomeRow' : ''))
    if (section === 'polyfin') element.setAttribute('data-polyfin-row', 'trending')
    element.title = title
    element.full = height
    const heading = element.appendChild(new Element('div', 'sectionTitleContainer')).appendChild(new Element('h2', 'sectionTitle'))
    heading.text = title
    element.appendChild(scroller())
    if (filled) fill(element)
    return element
  }
  const fill = (element) => { element.height = element.full; element.querySelector('.emby-scroller').items = 10 }
  const rowsOf = () => (search ? searchRows : homeRows)
  // page builds a page as jellyfin-web renders it, with its rows filled or
  // empty, as jellyfin-web writes them before they come.
  const page = (filled) => {
    if (search) {
      const made = new Element('div', 'page mainAnimatedPage')
      made.id = 'searchPage'
      for (const spec of rowsOf()) made.appendChild(row(spec, filled))
      return made
    }
    const made = new Element('div', 'page mainAnimatedPage homePage')
    const container = made.appendChild(new Element('div', 'homeSectionsContainer'))
    for (const spec of rowsOf()) container.appendChild(row(spec, filled))
    return made
  }
  const findRow = (title) => shownRows().find((element) => element.title === title)
  let timers = new Map()
  let timer = 0
  let frames = []
  const listeners = {}
  const fire = (type) => { for (const listener of listeners[type] || []) listener({ type, target: body }) }
  const entries = [{ state: null, hash: scenario.start || '#/home' }]
  let index = 0
  let keys = 0
  const location = { protocol: 'http:', host: 'polyfin.test', pathname: '/web/', search: '', hash: entries[0].hash, get href() { return 'http://polyfin.test/web/' + this.hash }, set href(url) {}, replace: () => {}, reload: () => {} }
  // The router's history: each entry with react-router's state, its
  // own key.
  const history = {
    get state() { return entries[index].state },
    get length() { return entries.length },
    pushState: (state, title, url) => { entries.splice(index + 1); entries.push({ state, hash: url }); index++; location.hash = url },
    replaceState: (state, title, url) => { entries[index] = { state, hash: url }; location.hash = url },
    back: () => {},
  }
  const go = (hash, how) => { history[how]({ usr: null, key: 'k' + ++keys, idx: index + (how === 'pushState' ? 1 : 0) }, '', hash) }
  const storage = { getItem: () => null, setItem: () => {}, removeItem: () => {} }
  const context = {
    location, history, document,
    MutationObserver: class { observe() {} disconnect() {} takeRecords() { return [] } },
    window: { localStorage: storage, sessionStorage: storage, location },
    navigator: { userAgent: 'Node' }, Date, Promise, JSON, Math, Object, Array, String, Number, RegExp, isFinite,
    CustomEvent: class { constructor(type) { this.type = type } },
    setTimeout: (run) => { timers.set(++timer, run); return timer }, clearTimeout: (id) => { timers.delete(id) },
    setInterval: () => 0, clearInterval: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    removeEventListener: (type, listener) => { listeners[type] = (listeners[type] || []).filter((kept) => kept !== listener) },
    innerHeight: 800, scrollTo, requestAnimationFrame: (run) => { frames.push(run); return frames.length },
  }
  Object.defineProperty(context, 'pageYOffset', { get: () => scrollY })
  body.appendChild(page(true))
  runScript(script, context)
  for (const step of scenario.steps) {
    const words = step.split(' ')
    if (words[0] === 'scroll') scrollY = Number(words[1])
    else if (words[0] === 'slide') findRow(words.slice(1, -1).join(' ')).querySelector('.emby-scroller').scrollToPosition(Number(words[words.length - 1]))
    else if (step === 'open' || step === 'open legacy') {
      // jellyfin-web pushes the title's route, then shows its page, at
      // the top; the modern layout drops the page it leaves, the legacy
      // layout hides it.
      go('#/details?id=0123456789abcdef0123456789abcdef', 'pushState')
      for (const child of [...body.children]) {
        if (child.classList.contains('details')) body.removeChild(child)
        else if (step === 'open legacy') child.classList.add('hide')
        else body.removeChild(child)
      }
      body.appendChild(new Element('div', 'page details'))
      scrollY = 0
    } else if (step === 'back') {
      index--
      location.hash = entries[index].hash
      fire('popstate')
      fire('hashchange')
    } else if (step === 'mount') {
      // The modern layout builds the page anew, its rows empty, at the
      // top.
      for (const child of [...body.children]) body.removeChild(child)
      body.appendChild(page(false))
      scrollY = 0
    } else if (step === 'unhide') {
      for (const child of [...body.children]) {
        if (child.classList.contains('details')) body.removeChild(child)
        else child.classList.remove('hide')
      }
      scrollY = 905
    } else if (words[0] === 'fill') {
      const without = step.startsWith('fill without ') ? step.slice('fill without '.length) : ''
      for (const element of [...shownRows()]) {
        if (element.title === without) element.parentNode.removeChild(element)
        else fill(element)
      }
    } else if (step === 'frame') { const due = frames; frames = []; for (const run of due) run(0) }
    else if (words[0] === 'redraw') {
      const element = findRow(words.slice(1).join(' '))
      element.removeChild(element.querySelector('.emby-scroller'))
      element.appendChild(scroller()).items = 10
    }
    else if (words[0] === 'reset') { findRow(words.slice(1).join(' ')).querySelector('.emby-scroller').position = 0; fire('scroll') }
    else if (step === 'logo') { go('#/', 'pushState'); go('#/home', 'replaceState') }
    else if (step === 'home link') go('#/home', 'pushState')
    else if (step === 'wheel') fire('wheel')
    else if (step === 'touch') fire('touchstart')
    else if (step === 'key') fire('keydown')
    else if (step === 'wait') { const due = [...timers.values()]; timers = new Map(); for (const run of due) try { run() } catch (error) {} }
    else if (step === 'y') log.push('y=' + scrollY)
    else if (words[0] === 'sideways') { const title = words.slice(1).join(' '); log.push(title + ' at ' + findRow(title).querySelector('.emby-scroller').getScrollPosition()) }
    else throw new Error('unknown step ' + step)
  }
  return log
}
const results = {}
for (const [name, scenario] of Object.entries(scenarios)) {
  try {
    results[name] = run(scenario)
  } catch (error) {
    results[name] = ['harness: ' + error.stack]
  }
}
process.stdout.write(JSON.stringify(results))
`
	type input struct {
		Start  string   `json:"start,omitempty"`
		Layout string   `json:"layout,omitempty"`
		Steps  []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.start, s.layout, s.steps}
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
