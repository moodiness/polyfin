package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// A home row's note shows under its name, after its signer's name when it
// has one. A row shown to chosen users has a Hide button, which posts
// /Polyfin/HomeRows/{RowId}/Hidden, takes every element of the row off the
// page and says so in a toast, or says it failed and keeps the row; while
// the user did not see it, a New badge, which goes with a post of
// /Polyfin/HomeRows/{RowId}/Seen the first time they open the row's list
// or a title from it, and never for a click on Hide. A row shown to
// everyone has neither. The words follow jellyfin-web's language. It runs
// in Node.js, in a context that stands for the browser and jellyfin-web.
func TestHomeRowsShowNotesBadgesAndHide(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type row = map[string]any
	item := func(id, name string) row {
		return row{"Id": id, "Name": name, "Type": "Movie", "ProductionYear": 2020, "UserData": row{}, "ImageTags": row{"Primary": "t" + id}}
	}
	everyone := row{"Id": "f1", "RowId": "r1", "Name": "Trending", "Place": "Top", "Note": "Fresh this week.", "Items": []row{item("a1", "Alpha")}}
	suggested := row{"Id": "f2", "RowId": "r2", "Name": "For you", "Place": "Bottom", "Note": "Watch this weekend!", "NoteBy": "Morgan",
		"Suggested": true, "New": true, "Items": []row{item("b2", "Bravo"), item("c3", "Charlie")}}
	seen := row{"Id": "f3", "RowId": "r3", "Name": "Seen", "Place": "Bottom", "Suggested": true, "Items": []row{item("d4", "Delta")}}
	// Two folders of one Because you watched row, which go together.
	because := func(id, name string) row {
		return row{"Id": id, "RowId": "r4", "Name": name, "Place": "AfterResume", "Kind": "BecauseYouWatched", "Suggested": true, "New": true,
			"Items": []row{item("e"+id, "Echo")}}
	}
	type scenario struct {
		rows     []row
		lang     string
		failures []string
		steps    []string
		want     []string
	}
	const drawn = "Trending | note Fresh this week. ; For you [New] [Hide] | note From Morgan: Watch this weekend! ; Seen [Hide]"
	scenarios := map[string]scenario{
		"notes, badges and buttons": {rows: []row{everyone, suggested, seen}, steps: []string{"show"}, want: []string{drawn}},
		"in French": {rows: []row{suggested}, lang: "fr-FR", steps: []string{"show"},
			want: []string{"For you [Nouveau] [Masquer] | note De la part de Morgan\u202f: Watch this weekend!"}},
		"a note without a signer": {rows: []row{{"Id": "f5", "RowId": "r5", "Name": "Plain", "Place": "Top", "Note": "Just a note.",
			"Items": []row{item("a1", "Alpha")}}}, steps: []string{"show"}, want: []string{"Plain | note Just a note."}},
		"seen from a card": {rows: []row{everyone, suggested, seen},
			steps: []string{"click card f2", "show", "click card f2", "click title f2", "show"},
			want: []string{"POST Polyfin/HomeRows/r2/Seen",
				"Trending | note Fresh this week. ; For you [Hide] | note From Morgan: Watch this weekend! ; Seen [Hide]",
				"Trending | note Fresh this week. ; For you [Hide] | note From Morgan: Watch this weekend! ; Seen [Hide]"}},
		"seen from its name": {rows: []row{suggested}, steps: []string{"click title f2", "show"},
			want: []string{"POST Polyfin/HomeRows/r2/Seen", "For you [Hide] | note From Morgan: Watch this weekend!"}},
		"a click elsewhere in the row": {rows: []row{suggested}, steps: []string{"click note f2", "show"},
			want: []string{"For you [New] [Hide] | note From Morgan: Watch this weekend!"}},
		"seen once for every folder of the row": {rows: []row{because("f6", "Because you watched A"), because("f7", "Because you watched B")},
			steps: []string{"show", "click card f6", "show", "click card f7"},
			want: []string{"Because you watched A [New] [Hide] ; Because you watched B [New] [Hide]", "POST Polyfin/HomeRows/r4/Seen",
				"Because you watched A [Hide] ; Because you watched B [Hide]"}},
		"hidden": {rows: []row{everyone, suggested, seen}, steps: []string{"click hide f2", "show", "render", "show", "wait 4000", "show"},
			want: []string{"POST Polyfin/HomeRows/r2/Hidden",
				"Trending | note Fresh this week. ; Seen [Hide] ; toast Row hidden. My account › Home rows shows it again.",
				"Trending | note Fresh this week. ; Seen [Hide] ; toast Row hidden. My account › Home rows shows it again.",
				"Trending | note Fresh this week. ; Seen [Hide]"}},
		"hidden, every folder of the row": {rows: []row{because("f6", "Because you watched A"), because("f7", "Because you watched B"), seen},
			steps: []string{"click hide f7", "wait 4000", "show"},
			want:  []string{"POST Polyfin/HomeRows/r4/Hidden", "Seen [Hide]"}},
		"hiding failing": {rows: []row{suggested}, failures: []string{"Polyfin/HomeRows/r2/Hidden"}, steps: []string{"click hide f2", "show"},
			want: []string{"POST Polyfin/HomeRows/r2/Hidden (500)",
				"For you [New] [Hide] | note From Morgan: Watch this weekend! ; toast The row could not be hidden. Try again."}},
	}
	const harness = scriptRunner + `
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  let now = 0
  const timers = []
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
  const classes = (element) => (element.className || '').split(' ').filter(Boolean)
  const matches = (element, selector) => /^(\.[\w-]+)+$/.test(selector) && selector.slice(1).split('.').every((c) => classes(element).includes(c))
  const all = (root, selector) => (root.children || []).flatMap((child) => (child.children ? [...(matches(child, selector) ? [child] : []), ...all(child, selector)] : []))
  const classList = (element) => ({
    contains: (c) => classes(element).includes(c),
    add: (c) => { if (!classes(element).includes(c)) { element.className = [...classes(element), c].join(' '); changed() } },
    remove: (c) => { element.className = classes(element).filter((x) => x !== c).join(' '); changed() },
  })
  class Element {
    constructor(tag, className) { this.tagName = tag; this.className = className || ''; this.children = []; this.parentNode = null; this.style = {}; this.text = ''; this.attributes = {}; this.listeners = {} }
    get classList() { return classList(this) }
    set textContent(text) { this.text = text; this.children = []; changed() }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    get nextSibling() { return this.parentNode ? this.parentNode.children[this.parentNode.children.indexOf(this) + 1] || null : null }
    get firstChild() { return this.children[0] || null }
    appendChild(child) { return this.insertBefore(child, null) }
    insertBefore(child, before) {
      if (child.parentNode) child.parentNode.removeChild(child)
      const at = before ? this.children.indexOf(before) : -1
      this.children.splice(at < 0 ? this.children.length : at, 0, child)
      child.parentNode = this
      changed()
      return child
    }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; changed(); return child }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    setAttribute(name, value) { this.attributes[name] = String(value) }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    addEventListener(type, listener, capture) { (this.listeners[type] ||= []).push({ listener, capture: !!capture }) }
    closest(selector) { for (let e = this; e && e.children; e = e.parentNode) if (matches(e, selector)) return e; return null }
  }
  // click sends a click to target as a browser does: down the page to it
  // for the listeners of the capture phase, then up again, until a
  // listener stops it. jellyfin-web's items container handles the clicks
  // on its cards, and stops them there.
  const click = (target) => {
    let stopped = false
    const event = { type: 'click', target, preventDefault: () => {}, stopPropagation: () => { stopped = true } }
    const path = []
    for (let e = target; e; e = e.parentNode) path.push(e)
    for (const e of [...path].reverse()) for (const l of (e.listeners && e.listeners.click) || []) if (l.capture && !stopped) l.listener(event)
    for (const e of path) {
      for (const l of (e.listeners && e.listeners.click) || []) if (!l.capture && !stopped) l.listener(event)
      if (e.children && classes(e).includes('itemsContainer')) stopped = true
    }
  }
  const body = new Element('body')
  const page = body.appendChild(new Element('div', 'page homePage'))
  const container = page.appendChild(new Element('div', 'sections homeSectionsContainer'))
  const render = () => {
    for (const child of [...container.children]) container.removeChild(child)
    for (let i = 0; i < 10; i++) container.appendChild(new Element('div', 'verticalSection section' + i))
  }
  render()
  const document = {
    body,
    documentElement: { lang: scenario.lang || 'en-US', className: 'layout-desktop', get classList() { return classList(this) } },
    head: { appendChild: () => {} },
    createElement: (tag) => new Element(tag),
    createTextNode: (text) => ({ textContent: text, parentNode: null }),
    getElementById: () => null,
    querySelector: (selector) => body.querySelector(selector),
    querySelectorAll: (selector) => body.querySelectorAll(selector),
  }
  const describe = (row) => {
    let text = row.querySelector('.sectionTitle').textContent
    const badge = row.querySelector('.polyfinRowNew')
    if (badge) text += ' [' + badge.textContent + ']'
    const hide = row.querySelector('.polyfinRowHide')
    if (hide) text += ' [' + hide.textContent + ']'
    const note = row.querySelector('.polyfinRowNote')
    if (note) text += ' | note ' + note.textContent
    return text
  }
  const show = () => {
    const shown = container.querySelectorAll('.polyfinHomeRow').map(describe)
    for (const toast of body.querySelectorAll('.toast')) shown.push('toast ' + toast.textContent)
    log.push(shown.join(' ; '))
  }
  const rowOf = (id) => container.querySelectorAll('.polyfinHomeRow').find((row) => row.getAttribute('data-polyfin-row') === id)
  const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home' }
  const history = { pushState: () => {}, replaceState: () => {} }
  const failures = scenario.failures || []
  const ApiClient = {
    serverId: () => 'srv',
    getUrl: (path, query) => 'http://polyfin.test/' + path + (query ? '?' + new URLSearchParams(query) : ''),
    getCurrentUserId: () => 'user',
    getScaledImageUrl: (id) => 'img ' + id,
    getDisplayPreferences: () => Promise.resolve({ CustomPrefs: {} }),
    getJSON: () => Promise.resolve({ Items: JSON.parse(JSON.stringify(scenario.rows)) }),
    ajax: (request) => {
      const path = request.url.replace('http://polyfin.test/', '')
      const failing = failures.includes(path)
      log.push(request.type + ' ' + path + (failing ? ' (500)' : ''))
      return failing ? Promise.reject(new Error('500')) : Promise.resolve()
    },
  }
  runScript(script, {
    location, history, URL, URLSearchParams, document, MutationObserver, window: { ApiClient, sessionStorage: { getItem: () => null } },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
    Event: class { constructor(type) { this.type = type } },
    Date: { now: () => now },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: () => {},
    addEventListener: () => {},
  })
  await flush()
  await flush()
  for (const step of scenario.steps) {
    if (step === 'show') show()
    else if (step === 'render') render()
    else if (step.startsWith('click ')) {
      const [, what, id] = step.split(' ')
      const row = rowOf(id)
      const target = what === 'card' ? row.querySelector('.cardImageContainer')
        : what === 'title' ? row.querySelector('.sectionTitle')
        : what === 'hide' ? row.querySelector('.polyfinRowHide').querySelector('.material-icons')
        : row.querySelector('.polyfinRowNote')
      click(target)
    } else if (step.startsWith('wait ')) {
      const until = now + Number(step.slice(5))
      for (;;) {
        let next = -1
        timers.forEach((timer, i) => { if (timer && timer.at <= until && (next < 0 || timer.at < timers[next].at)) next = i })
        if (next < 0) break
        const timer = timers[next]
        timers[next] = null
        now = timer.at
        timer.run()
        await flush()
      }
      now = until
    }
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
		Rows     []row    `json:"rows"`
		Lang     string   `json:"lang,omitempty"`
		Failures []string `json:"failures,omitempty"`
		Steps    []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.rows, s.lang, s.failures, s.steps}
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
