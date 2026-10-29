package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// jellyfin-web's search page lists the user's recent searches while its
// field is empty, above its Suggestions: up to 10, newest first, each
// searching again, with a button that removes it and one that clears them
// all. A search is kept as the user leaves its results, provided it found
// something; typing on replaces it, a repeated search moves to the top.
// They are kept in the browser, for the user signed in: another user never
// sees them, and the same user finds them after signing out and in again.
// It runs in Node.js, in a context that stands for the browser and
// jellyfin-web.
func TestSearchPageListsTheUsersRecentSearches(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// search types query on the search page, waits for jellyfin-web to show
	// what it found, then opens a result.
	search := func(query string) []string {
		return []string{"#/search", "type " + query, "draw", "open"}
	}
	searches := func(queries ...string) []string {
		var steps []string
		for _, query := range queries {
			steps = append(steps, search(query)...)
		}
		return steps
	}
	then := func(steps ...[]string) []string { return slices.Concat(steps...) }
	type scenario struct {
		// user is signed in as the script loads; empty for none.
		user string
		// steps: a route the router moves to, the search page drawn with its
		// field and the Suggestions; "type <query>" types in the field, ""
		// clearing it; "draw" has jellyfin-web show what the field's search
		// found, half a second after the last key, or the Suggestions for an
		// empty field; "open" opens a result; "choose <query>", "remove
		// <query>" and "clear" press the list's buttons; "signin <user>" and
		// "signout" change the user signed in; "reload" loads the page again
		// on its route, "reload <route>" on that one; "close" closes the tab;
		// "show" logs the list; "where" logs the route.
		steps []string
		// want logs "recent(a|b)" for the list shown, newest first, with
		// "misplaced" when it is not between the field and the Suggestions,
		// or "none".
		want []string
	}
	scenarios := map[string]scenario{
		"a search that found something is listed while the field is empty": {user: "boss",
			steps: then(search("matrix"), []string{"#/search", "show"}),
			want:  []string{"recent(matrix)"}},
		"nothing is listed while the field holds a search": {user: "boss",
			steps: then(search("matrix"), []string{"#/search", "show", "type ma", "show", "draw", "show", "type ", "show", "draw", "show"}),
			want:  []string{"recent(matrix)", "none", "none", "none", "recent(matrix)"}},
		"a search that found nothing is not kept": {user: "boss",
			steps: then(searches("matrix", "zzz"), []string{"#/search", "show", "type zzz", "draw", "type ", "draw", "show"}),
			want:  []string{"recent(matrix)", "recent(matrix)"}},
		"clearing the field keeps the search left": {user: "boss",
			steps: []string{"#/search", "type matrix", "draw", "type ", "draw", "show"},
			want:  []string{"recent(matrix)"}},
		"typing on keeps the longer search only": {user: "boss",
			steps: []string{"#/search", "type mat", "draw", "type matrix", "draw", "type matrix re", "draw", "type matrix", "draw", "open", "#/search", "show"},
			want:  []string{"recent(matrix)"}},
		"typing another search keeps the one before": {user: "boss",
			steps: []string{"#/search", "type matrix", "draw", "type a", "type alien", "draw", "open", "#/search", "show"},
			want:  []string{"recent(alien|matrix)"}},
		"a search left before jellyfin-web showed it is not kept": {user: "boss",
			steps: []string{"#/search", "type matrix", "draw", "type ", "type alien", "open", "#/search", "show"},
			want:  []string{"recent(matrix)"}},
		"newest first, a repeated search moving to the top": {user: "boss",
			steps: then(searches("matrix", "alien", "heat", "Matrix"), []string{"#/search", "show"}),
			want:  []string{"recent(Matrix|heat|alien)"}},
		"ten at most": {user: "boss",
			steps: then(searches("a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11", "a12"), []string{"#/search", "show"}),
			want:  []string{"recent(a12|a11|a10|a9|a8|a7|a6|a5|a4|a3)"}},
		"choosing one searches it again and moves it to the top": {user: "boss",
			steps: then(searches("matrix", "alien"), []string{"#/search", "choose matrix", "where", "show", "draw", "open", "#/search", "show"}),
			want:  []string{"at #/search?query=matrix", "none", "recent(matrix|alien)"}},
		"choosing one in a library searches the library": {user: "boss",
			steps: then(search("the matrix"), []string{"#/search?parentId=p1&collectionType=movies", "choose the matrix", "where"}),
			want:  []string{"at #/search?parentId=p1&collectionType=movies&query=the+matrix"}},
		"removing one": {user: "boss",
			steps: then(searches("matrix", "alien", "heat"), []string{"#/search", "remove alien", "show", "reload", "show"}),
			want:  []string{"recent(heat|matrix)", "recent(heat|matrix)"}},
		"removing them all": {user: "boss",
			steps: then(searches("matrix", "alien"), []string{"#/search", "clear", "show", "reload", "show"}),
			want:  []string{"none", "none"}},
		"kept across loads": {user: "boss",
			steps: then(searches("matrix"), []string{"reload", "#/search", "show"}),
			want:  []string{"recent(matrix)"}},
		"closing the tab on results keeps them": {user: "boss",
			steps: []string{"#/search", "type matrix", "draw", "close", "reload #/search", "show"},
			want:  []string{"recent(matrix)"}},
		"another user sees none": {user: "boss",
			steps: then(searches("matrix"), []string{"#/search", "show", "signout", "#/login", "signin viewer", "#/search", "show"}),
			want:  []string{"recent(matrix)", "none"}},
		"each user their own": {user: "boss",
			steps: then(searches("matrix"), []string{"signout", "#/login", "signin viewer", "reload"}, searches("alien"),
				[]string{"#/search", "show", "signout", "#/login", "signin boss", "#/search", "show"}),
			want: []string{"recent(alien)", "recent(matrix)"}},
		"another user signed in on a later load sees none": {user: "boss",
			steps: then(searches("matrix"), []string{"#/search", "show", "signout", "signin viewer", "reload", "show"}),
			want:  []string{"recent(matrix)", "none"}},
		"signing out keeps them for the next sign-in": {user: "boss",
			steps: then(searches("matrix"), []string{"signout", "#/login", "reload", "#/search", "show", "signin boss", "#/home", "#/search", "show"}),
			want:  []string{"none", "recent(matrix)"}},
		"a search left as its user signs out is theirs": {user: "boss",
			steps: []string{"#/search", "type matrix", "draw", "signout", "#/login", "signin viewer", "#/search", "show", "signout", "#/login", "signin boss", "#/search", "show"},
			want:  []string{"none", "recent(matrix)"}},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
// The searches the server finds something for.
const found = ['matrix', 'the matrix', 'mat', 'matrix re', 'alien', 'heat', ...Array.from({ length: 12 }, (_, i) => 'a' + (i + 1))]
async function run(scenario) {
  const log = []
  const store = {}
  let user = scenario.user
  let route = ''
  let field = ''
  let load = null
  // Each load of the page: its document, its router and the script.
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
    const matches = (element, selector) => /^(\.[\w-]+)+$/.test(selector) && selector.slice(1).split('.').every((c) => classes(element).includes(c))
    const all = (root, selector) => (root.children || []).flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
    class Element {
      constructor(tag) { this.tagName = tag.toUpperCase(); this.className = ''; this.id = ''; this.children = []; this.parentNode = null; this.text = ''; this.attributes = {}; this.handlers = {} }
      get classList() {
        return {
          contains: (c) => classes(this).includes(c),
          add: (c) => { if (!classes(this).includes(c)) { this.className = [...classes(this), c].join(' '); changed() } },
          remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' '); changed() },
          toggle: (c, force) => { (force === undefined ? !classes(this).includes(c) : force) ? this.classList.add(c) : this.classList.remove(c) },
        }
      }
      set textContent(text) { this.text = text; this.children = []; changed() }
      get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
      get previousSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) - 1] || null }
      get nextSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) + 1] || null }
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
      removeAttribute(name) { delete this.attributes[name] }
      addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler) }
      focus() {}
      closest(selector) { for (let at = this; at && at.children; at = at.parentNode) if (matches(at, selector)) return at; return null }
      contains(other) { for (let at = other; at; at = at.parentNode) if (at === this) return true; return false }
    }
    const body = new Element('body')
    const sheets = []
    const document = {
      body,
      documentElement: { lang: 'en-US', classList: { contains: () => false } },
      head: { appendChild: (sheet) => sheets.push(sheet) },
      createElement: (tag) => new Element(tag),
      createTextNode: (text) => ({ textContent: text, parentNode: null }),
      getElementById: (id) => sheets.find((sheet) => sheet.id === id) || (function find(at) {
        for (const child of at.children || []) { if (child.id === id) return child; const inner = find(child); if (inner) return inner }
        return null
      })(body),
      querySelector: (selector) => body.querySelector(selector),
      querySelectorAll: (selector) => body.querySelectorAll(selector),
    }
    const fire = (type) => { for (const listener of listeners[type] || []) listener({ type }) }
    // jellyfin-web 12.4's search page: the field, then the Suggestions or
    // what the search found. React keeps nodes it did not make in place.
    let page = null
    const own = new WeakSet()
    const made = (tag, className, parent) => { const element = new Element(tag); element.className = className; own.add(element); parent.appendChild(element); return element }
    const searchRoute = (hash) => /^#\/search(\?|$)/.test(hash)
    const query = (hash) => new URLSearchParams(hash.split('?')[1] || '').get('query') || ''
    function draw() {
      if (!page) return
      for (const child of [...page.children]) if (own.has(child) && !child.classList.contains('searchFields')) page.removeChild(child)
      const shown = field
      if (!shown) {
        const suggestions = made('div', 'verticalSection searchSuggestions', page)
        made('a', 'button-link emby-button', suggestions).textContent = 'Suggested'
      } else if (found.includes(shown.trim().toLowerCase())) {
        const results = made('div', 'searchResults', page)
        made('div', 'card', made('div', 'itemsContainer', made('div', 'verticalSection', results))).textContent = shown
      } else made('div', 'noItemsMessage centerMessage', page).textContent = 'Sorry! No results found for "' + shown + '"'
    }
    // routed shows the page of the route, as jellyfin-web's router does.
    function routed() {
      if (route === location.hash) return
      const was = route
      route = location.hash
      if (!searchRoute(route)) {
        if (page) body.removeChild(page)
        page = null
        return
      }
      field = query(route)
      if (page && searchRoute(was)) return draw()
      if (page) body.removeChild(page)
      page = made('div', 'page libraryPage', body)
      page.id = 'searchPage'
      made('div', 'padded-left padded-right searchFields', page)
      draw()
    }
    const location = { href: 'http://polyfin.test/web/' }
    let hash = route
    Object.defineProperty(location, 'hash', {
      get: () => hash,
      set: (value) => {
        hash = value.startsWith('#') ? value : '#' + value
        location.href = 'http://polyfin.test/web/' + hash
        setImmediate(() => { fire('hashchange'); fire('popstate'); routed() })
      },
    })
    const go = (url) => { hash = url; location.href = 'http://polyfin.test/web/' + url }
    const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
    const localStorage = { getItem: (name) => (name in store ? store[name] : null), setItem: (name, value) => { store[name] = String(value) }, removeItem: (name) => { delete store[name] } }
    const ApiClient = {
      serverId: () => 's',
      getCurrentUserId: () => user || null,
      getCurrentUser: () => (user ? Promise.resolve({ Id: user, Policy: { IsAdministrator: true } }) : Promise.reject(new Error('signed out'))),
      getUrl: (path) => 'http://polyfin.test/' + path,
      getJSON: () => Promise.reject(new Error('not asked here')),
      ajax: () => Promise.reject(new Error('not asked here')),
    }
    vm.runInNewContext(script, {
      location, history, document, MutationObserver, Object, JSON, String, Date,
      window: { ApiClient, localStorage, sessionStorage: localStorage },
      CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
      setTimeout: () => 0, clearTimeout: () => {},
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    })
    route = ''
    routed()
    return {
      go: (url) => { history.pushState(null, '', url); routed() },
      type: (text) => {
        field = text
        // useSearchParam keeps the field in the route, replacing it.
        const params = new URLSearchParams(location.hash.split('?')[1] || '')
        if (text) params.set('query', text)
        else params.delete('query')
        const next = '#/search' + (params.toString() ? '?' + params : '')
        history.replaceState(null, '', next)
        route = next
      },
      draw,
      fire,
      page: () => page,
      where: () => location.hash,
    }
  }
  const at = (text, className) => {
    const page = load.page()
    const entry = page && page.querySelectorAll('.polyfinRecentSearch').find((entry) => entry.querySelector('.polyfinRecentSearchLink').textContent === text)
    return entry ? entry.querySelector(className) : null
  }
  // click presses a button, or follows a link to its address.
  const click = (element) => {
    if (!element) return log.push('nothing to press')
    for (const handler of element.handlers.click || []) handler({ target: element, preventDefault() {} })
    const href = element.getAttribute('href')
    if (href && href.startsWith('#')) load.go(href)
  }
  load = start()
  await flush()
  for (const step of scenario.steps) {
    if (step === 'show') {
      const page = load.page()
      const list = page && page.querySelector('.polyfinRecentSearches')
      if (!list) log.push('none')
      else {
        const words = 'recent(' + list.querySelectorAll('.polyfinRecentSearchLink').map((link) => link.textContent).join('|') + ')'
        const fields = page.querySelector('.searchFields')
        const suggestions = page.querySelector('.searchSuggestions')
        const placed = list.previousSibling === fields && (!suggestions || list.nextSibling === suggestions)
        log.push(words + (placed ? '' : ' misplaced'))
      }
    } else if (step === 'where') log.push('at ' + load.where())
    else if (step === 'draw') load.draw()
    else if (step === 'open') load.go('#/details?id=0123456789abcdef0123456789abcdef')
    else if (step.startsWith('type ')) load.type(step.slice(5))
    else if (step.startsWith('choose ')) click(at(step.slice(7), '.polyfinRecentSearchLink'))
    else if (step.startsWith('remove ')) click(at(step.slice(7), '.polyfinRecentSearchRemove'))
    else if (step === 'clear') click(load.page() && load.page().querySelector('.polyfinRecentSearchesClear'))
    else if (step.startsWith('signin ')) user = step.slice(7)
    else if (step === 'signout') user = ''
    else if (step === 'close') load.fire('pagehide')
    else if (step.startsWith('reload')) {
      const where = step.slice(7) || load.where()
      load = start()
      if (where) load.go(where)
    } else load.go(step)
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
		User  string   `json:"user"`
		Steps []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.user, s.steps}
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
