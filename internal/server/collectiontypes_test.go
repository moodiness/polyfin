package server

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// A collection's page offers a type filter when the collection holds
// several types: "All", then a button for each type jellyfin-web groups
// in a section of its own, before the sections. A type picked has
// jellyfin-web list the page anew, its request asking for that type
// alone, so that the browser gets and draws it alone; "All" asks for
// everything again. The choice is kept in the browser for the collection
// and the user, and applies from the page's first request the next time.
// A title's page, or a collection of a single type, gets nothing, and the
// script asks Polyfin nothing there. Under the LumaaGlaass theme, the
// filter takes the classes of the theme's own. It runs in Node.js, in a
// context that stands for the browser and jellyfin-web 12.4's collection
// page.
func TestCollectionPagesFilterByType(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		mixed   = "#/details?id=c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
		another = "#/details?id=c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2"
		single  = "#/details?id=c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3"
		movie   = "#/details?id=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	type scenario struct {
		// language is the document's; English when empty.
		language string
		// lumaaGlaass runs the LumaaGlaass theme: its loader's script is in
		// the page.
		lumaaGlaass bool
		// steps: a route the router moves to, where jellyfin-web draws the
		// title's page and lists a collection's entries; "back <route>"
		// shows the page of that route as jellyfin-web left it, asking
		// nothing; "pick <word>" presses the filter's button; "play" lists
		// the collection's titles as its Play button does; "signin <user>"
		// changes the user signed in; "reload" loads the page again on its
		// route; "server <collection> <Type>=<count>,…" changes what the
		// collection holds; "hold" has the listings' answers wait, until
		// "draw"; "show" logs the page.
		steps []string
		// want logs "asked <collection> <type>" for each listing of the
		// page, "all" for every type, "played <collection> <type>" for
		// the Play button's, "types <collection>" for each question to
		// Polyfin, and for "show", "filter(<buttons>)", the pressed one
		// starred, or "none", then the sections drawn with their counts.
		want []string
	}
	const c1, c2, c3 = "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1", "c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2", "c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3"
	scenarios := map[string]scenario{
		"a collection of movies and series offers All and both": {
			steps: []string{mixed, "show"},
			want:  []string{"asked c1 all", "types c1", "filter(All*|Movies|Series) sections(Movies 3|Series 2)"}},
		"a type picked is the only one listed": {
			steps: []string{mixed, "pick Series", "show"},
			want:  []string{"asked c1 all", "types c1", "asked c1 Series", "filter(All|Movies|Series*) sections(Series 2)"}},
		"All lists every type again": {
			steps: []string{mixed, "pick Series", "pick All", "show"},
			want: []string{"asked c1 all", "types c1", "asked c1 Series", "asked c1 all",
				"filter(All*|Movies|Series) sections(Movies 3|Series 2)"}},
		"the choice applies from the first request of the next visit": {
			steps: []string{mixed, "pick Movies", movie, mixed, "show", "reload", "show"},
			want: []string{"asked c1 all", "types c1", "asked c1 Movie", "asked c1 Movie", "types c1",
				"filter(All|Movies*|Series) sections(Movies 3)", "asked c1 Movie", "types c1", "filter(All|Movies*|Series) sections(Movies 3)"}},
		"each collection its own choice": {
			steps: []string{mixed, "pick Series", another, "show"},
			want: []string{"asked c1 all", "types c1", "asked c1 Series", "asked c2 all", "types c2",
				"filter(All*|Movies|Series|Collections) sections(Movies 1|Series 1|Collections 1)"}},
		"each user their own": {
			steps: []string{mixed, "pick Series", "signin viewer", "reload", "show"},
			want:  []string{"asked c1 all", "types c1", "asked c1 Series", "asked c1 all", "types c1", "filter(All*|Movies|Series) sections(Movies 3|Series 2)"}},
		"a collection of a single type has no filter": {
			steps: []string{single, "show"},
			want:  []string{"asked c3 all", "types c3", "none sections(Movies 2)"}},
		"a title's page asks nothing": {
			steps: []string{movie, "show"},
			want:  []string{"none"}},
		"a type the collection no longer holds gives way to All": {
			steps: []string{mixed, "pick Series", "server c1 Movie=3,BoxSet=1", "reload", "show"},
			want: []string{"asked c1 all", "types c1", "asked c1 Series", "asked c1 Series", "types c1", "asked c1 all",
				"filter(All*|Movies|Collections) sections(Movies 3|Collections 1)"}},
		"back on the collection's page the filter shows again": {
			steps: []string{mixed, "pick Series", movie, "back " + mixed, "show"},
			want:  []string{"asked c1 all", "types c1", "asked c1 Series", "filter(All|Movies|Series*) sections(Series 2)"}},
		"the Play button's listing goes as it is": {
			steps: []string{mixed, "pick Series", "play"},
			want:  []string{"asked c1 all", "types c1", "asked c1 Series", "played c1 all"}},
		"a page drawn after Polyfin answered gets the filter as it is drawn": {
			steps: []string{"hold", mixed, "show", "draw", "show"},
			want:  []string{"asked c1 all", "types c1", "none", "filter(All*|Movies|Series) sections(Movies 3|Series 2)"}},
		"its words follow the page's language": {language: "fr-FR",
			steps: []string{mixed, "show"},
			want:  []string{"asked c1 all", "types c1", "filter(Tout*|Films|Séries) sections(Movies 3|Series 2)"}},
		"under LumaaGlaass the filter takes the theme's look": {lumaaGlaass: true,
			steps: []string{mixed, "show", "pick Series", "show"},
			want: []string{"asked c1 all", "types c1", "filter(All*|Movies|Series) themed sections(Movies 3|Series 2)",
				"asked c1 Series", "filter(All|Movies|Series*) themed sections(Series 2)"}},
	}
	server := map[string]map[string]int{
		c1: {"Movie": 3, "Series": 2},
		c2: {"Movie": 1, "Series": 1, "BoxSet": 1},
		c3: {"Movie": 2},
	}
	const harness = scriptRunner + `
const { script, scenarios, server } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = async () => { for (let i = 0; i < 10; i++) await new Promise((resolve) => setImmediate(resolve)) }
// The order of jellyfin-web 12.4's sections of a collection's page.
const order = ['Movie', 'Series', 'Episode', 'Video', 'BoxSet']
const names = { Movie: 'Movies', Series: 'Series', Episode: 'Episodes', Video: 'Videos', BoxSet: 'Collections' }
async function run(scenario) {
  const log = []
  const store = {}
  const held = JSON.parse(JSON.stringify(server))
  let user = 'boss'
  // Listings held wait for "draw".
  let holding = false
  const waiting = []
  let load = null
  const short = (id) => id.slice(0, 2)
  const query = (address) => new URLSearchParams(address.split('?')[1] || '')
  // Each load of the page: its document, its router, jellyfin-web's
  // client and the script.
  function start() {
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
    // matches takes the selectors the script uses: classes, and
    // :not(.class).
    const matches = (element, selector) => {
      const match = /^((?:\.[\w-]+)+)(?::not\(\.([\w-]+)\))?$/.exec(selector)
      return !!match && match[1].slice(1).split('.').every((c) => classes(element).includes(c)) && !(match[2] && classes(element).includes(match[2]))
    }
    const all = (root, selector) => (root.children || []).flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
    class Element {
      constructor(tag) { this.tagName = tag.toUpperCase(); this.className = ''; this.id = ''; this.children = []; this.parentNode = null; this.text = ''; this.attributes = {}; this.handlers = {} }
      get classList() {
        return {
          contains: (c) => classes(this).includes(c),
          add: (c) => { if (!classes(this).includes(c)) { this.className = [...classes(this), c].join(' '); changed() } },
          remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' '); changed() },
        }
      }
      set textContent(text) { this.text = text; this.children = []; changed() }
      get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
      get previousSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) - 1] || null }
      get nextSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) + 1] || null }
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
      addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler) }
    }
    const body = new Element('body')
    const sheets = []
    const document = {
      body,
      documentElement: { lang: scenario.language || 'en-US', classList: { contains: () => false } },
      head: { appendChild: (sheet) => sheets.push(sheet) },
      createElement: (tag) => new Element(tag),
      getElementById: (id) => sheets.find((sheet) => sheet.id === id) || (id === 'lg-script' && scenario.lumaaGlaass ? new Element('script') : null),
      querySelector: (selector) => body.querySelector(selector),
      querySelectorAll: (selector) => body.querySelectorAll(selector),
    }
    const fire = (type) => { for (const listener of listeners[type] || []) listener({ type }) }
    const location = { href: 'http://polyfin.test/web/' }
    let hash = ''
    Object.defineProperty(location, 'hash', { get: () => hash })
    const go = (url) => { hash = url; location.href = 'http://polyfin.test/web/' + url }
    const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
    const localStorage = { getItem: (name) => (name in store ? store[name] : null), setItem: (name, value) => { store[name] = String(value) }, removeItem: (name) => { delete store[name] } }
    // The server: a collection's listing, narrowed by IncludeItemTypes,
    // and the types it holds.
    const window = {
      localStorage,
      sessionStorage: localStorage,
      fetch: (address) => {
        const asked = query(address)
        const id = asked.get('ParentId')
        const type = asked.get('IncludeItemTypes')
        log.push((asked.has('SortBy') ? 'played ' : 'asked ') + short(id) + ' ' + (type || 'all'))
        const items = []
        for (const [kind, count] of Object.entries(held[id] || {})) {
          if (!type || type === kind) for (let i = 0; i < count; i++) items.push({ Type: kind })
        }
        const answer = { json: () => Promise.resolve({ Items: items, TotalRecordCount: items.length }) }
        return holding ? new Promise((resolve) => waiting.push(() => resolve(answer))) : Promise.resolve(answer)
      },
    }
    window.ApiClient = {
      serverId: () => 's',
      getCurrentUserId: () => user || null,
      getCurrentUser: () => Promise.reject(new Error('not asked here')),
      getUrl: (path) => 'http://polyfin.test/' + path,
      getJSON: (address) => {
        const match = /Polyfin\/Items\/(\w+)\/CollectionTypes$/.exec(address)
        if (!match) return Promise.reject(new Error('not asked here'))
        log.push('types ' + short(match[1]))
        const kinds = Object.entries(held[match[1]] || {}).filter(([, count]) => count > 0).map(([kind]) => kind)
        return Promise.resolve({ Types: kinds.sort() })
      },
      ajax: () => Promise.reject(new Error('not asked here')),
    }
    // jellyfin-web 12.4's title page: a page kept for each route it showed,
    // and for a collection, its entries listed in one request without a
    // limit, grouped in sections of their own, each container carrying
    // notifyRefreshNeeded, which lists them anew. A listing of nothing is
    // drawn as one empty section, Items.
    const pages = {}
    let shown = null
    const made = (tag, className, parent) => { const element = new Element(tag); element.className = className; parent.appendChild(element); return element }
    function list(page, id) {
      window.fetch('http://polyfin.test/Users/' + user + '/Items?ParentId=' + id + '&Fields=ItemCounts%2CPrimaryImageAspectRatio%2CCanDelete%2CMediaSourceCount')
        .then((response) => response.json())
        .then((result) => {
          const sections = page.querySelector('.collectionItems')
          sections.classList.remove('hide')
          for (const child of [...sections.children]) sections.removeChild(child)
          const groups = order.map((kind) => [names[kind], result.Items.filter((item) => item.Type === kind)]).filter(([, items]) => items.length)
          for (const [name, items] of groups.length ? groups : [['Items', []]]) {
            const section = made('div', 'verticalSection', sections)
            made('h2', 'sectionTitle', section).textContent = name
            const container = made('div', 'itemsContainer collectionItemsContainer', section)
            container.count = items.length
            container.notifyRefreshNeeded = () => list(page, id)
          }
        })
    }
    function show(page) {
      if (shown && shown !== page) shown.classList.add('hide')
      page.classList.remove('hide')
      shown = page
    }
    function routed(restored) {
      const match = /id=(\w+)/.exec(hash)
      if (!match) return
      const id = match[1]
      if (restored && pages[hash]) return show(pages[hash])
      const page = new Element('div')
      page.className = 'page itemDetailPage hide'
      const content = made('div', 'detailPageContent', page)
      made('div', 'collectionItems hide', content)
      made('div', 'nextUpSection verticalSection hide', content)
      body.appendChild(page)
      pages[hash] = page
      show(page)
      if (held[id]) list(page, id)
    }
    runScript(script, {
      location, history, document, MutationObserver, Object, JSON, String, Date, Promise, encodeURIComponent, decodeURIComponent,
      window,
      setTimeout: (callback, delay) => { if (!delay) setImmediate(callback); return 0 },
      clearTimeout: () => {},
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    })
    return {
      go: (url) => { history.pushState(null, '', url); routed(false) },
      back: (url) => { go(url); fire('popstate'); routed(true) },
      page: () => shown,
      play: () => {
        const id = /id=(\w+)/.exec(hash)[1]
        window.fetch('http://polyfin.test/Users/' + user + '/Items?ParentId=' + id + '&Recursive=true&SortBy=SortName&Filters=IsNotFolder')
      },
      where: () => hash,
    }
  }
  function describe() {
    const page = load.page()
    if (!page) return 'none'
    const filter = page.querySelector('.polyfinCollectionTypes')
    const sections = page.querySelector('.collectionItems')
    let words = 'none'
    if (filter) {
      words = 'filter(' + filter.querySelectorAll('.polyfinCollectionType')
        .map((button) => button.textContent + (button.getAttribute('aria-pressed') === 'true' ? '*' : '')).join('|') + ')'
      if (filter.nextSibling !== sections) words += ' misplaced'
      const group = filter.querySelector('.lg-collection-filter-group')
      if (filter.classList.contains('lg-collection-filter') && group && group.querySelectorAll('.lg-collection-filter-button').length === filter.querySelectorAll('.polyfinCollectionType').length) {
        words += ' themed'
      }
    }
    const containers = sections ? sections.querySelectorAll('.collectionItemsContainer') : []
    if (containers.length) {
      words += ' sections(' + containers.map((container) => container.parentNode.querySelector('.sectionTitle').textContent + ' ' + container.count).join('|') + ')'
    }
    return words
  }
  load = start()
  await flush()
  for (const step of scenario.steps) {
    if (step === 'show') log.push(describe())
    else if (step.startsWith('pick ')) {
      const page = load.page()
      const button = page && page.querySelectorAll('.polyfinCollectionType').find((each) => each.textContent === step.slice(5))
      if (!button) log.push('no button ' + step.slice(5))
      else for (const handler of button.handlers.click || []) handler({ target: button })
    } else if (step === 'play') load.play()
    else if (step === 'hold') holding = true
    else if (step === 'draw') {
      holding = false
      for (const release of waiting.splice(0)) release()
    } else if (step.startsWith('signin ')) user = step.slice(7)
    else if (step.startsWith('server ')) {
      const [, id, kinds] = step.split(' ')
      held[Object.keys(held).find((key) => short(key) === id)] = Object.fromEntries(kinds.split(',').map((pair) => pair.split('=')).map(([kind, count]) => [kind, Number(count)]))
    } else if (step === 'reload') {
      const where = load.where()
      load = start()
      await flush()
      load.go(where)
    } else if (step.startsWith('back ')) load.back(step.slice(5))
    else load.go(step)
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
  process.exit(0)
})()
`
	type input struct {
		Language    string   `json:"language"`
		LumaaGlaass bool     `json:"lumaaGlaass"`
		Steps       []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.language, s.lumaaGlaass, s.steps}
	}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": inputs, "server": server})
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
		if got := results[name]; strings.Join(got, "\n") != strings.Join(s.want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", name, got, s.want)
		}
	}
}
