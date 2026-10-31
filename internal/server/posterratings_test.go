package server

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// While the administrator turns it on, the script shows a movie's or a
// series' ratings over its poster as the pointer or the focus comes to its
// card: the star rating, then the critics' and audience scores, those the
// title lacks left out. The first card not known asks /Polyfin/Items/Ratings
// once for the title cards of the page, its row first; cards answered get
// their ratings without asking again, and a card drawn anew gets them back.
// Turned off, nothing shows and the server is asked a minute later at the
// earliest. Other cards, a video and another user's answers get nothing. It
// runs in Node.js, in a context that stands for the browser and
// jellyfin-web.
func TestPostersShowTheirRatings(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	// The page: a row of movie A, series B and episode E, and a row of movie
	// D, then series F, whose poster has a version count.
	on := map[string]any{"Enabled": true, "Items": []map[string]any{
		{"Id": "a", "CommunityRating": 7.46, "CriticsScore": 88, "AudienceScore": 90.6},
		{"Id": "b", "CommunityRating": 8.1, "CriticsScore": nil, "AudienceScore": nil},
		{"Id": "f", "CommunityRating": nil, "CriticsScore": 40, "AudienceScore": nil},
	}}
	off := map[string]any{"Enabled": false, "Items": []any{}}
	const (
		askAll   = "ask a,b,d,f"
		shownAll = "a[community 7.5|critics 88%|audience 91%] b[community 8.1] d[] f[critics 40%]"
		nothing  = "a[] b[] d[] f[]"
	)
	type scenario struct {
		lang string
		// steps: "over <card>" and "focus <card>" bring the pointer or the
		// focus to a card; "answer" answers the request under way with the
		// next answer, "fail" fails it; "redraw <card>" has jellyfin-web draw
		// the card anew; "wait <ms>" lets time pass; "user <id>" signs
		// another user in; "#/…" routes; "show" logs the ratings on each
		// title card.
		steps   []string
		answers []map[string]any
		want    []string
	}
	scenarios := map[string]scenario{
		"turned on: one request for the page, every title card answered": {
			answers: []map[string]any{on},
			steps:   []string{"over a", "answer", "show", "over b", "over d", "focus f", "show"},
			want:    []string{askAll, shownAll, shownAll},
		},
		"the focus asks as the pointer does": {
			answers: []map[string]any{on},
			steps:   []string{"focus b", "answer", "show"},
			want:    []string{"ask b,a,d,f", shownAll},
		},
		"its row first": {
			answers: []map[string]any{on},
			steps:   []string{"over d", "answer", "show"},
			want:    []string{"ask d,f,a,b", shownAll},
		},
		"turned off: nothing, and asked again a minute later": {
			answers: []map[string]any{off, on},
			steps:   []string{"over a", "answer", "show", "over a", "wait 30000", "over b", "wait 31000", "over a", "answer", "show"},
			want:    []string{askAll, nothing, askAll, shownAll},
		},
		"a server that fails is asked again a minute later": {
			answers: []map[string]any{on},
			steps:   []string{"over a", "fail", "show", "over a", "wait 61000", "over a", "answer", "show"},
			want:    []string{askAll, nothing, askAll, shownAll},
		},
		"a card drawn anew gets its ratings back without asking": {
			answers: []map[string]any{on},
			steps:   []string{"over a", "answer", "redraw a", "show", "over a", "show"},
			want:    []string{askAll, "a[] b[community 8.1] d[] f[critics 40%]", shownAll},
		},
		"a card met while asking waits for the answer": {
			answers: []map[string]any{on},
			steps:   []string{"over a", "over b", "redraw b", "over b", "answer", "show"},
			want:    []string{askAll, shownAll},
		},
		"answers are kept for a minute, then a server turning it off takes them away": {
			answers: []map[string]any{on, off},
			steps:   []string{"over a", "answer", "wait 59000", "over b", "show", "wait 2000", "over b", "answer", "show"},
			want:    []string{askAll, shownAll, "ask b,a,d,f", nothing},
		},
		"another user's answers are not kept": {
			answers: []map[string]any{on, on},
			steps:   []string{"over a", "answer", "user other", "over a", "answer"},
			want:    []string{askAll, askAll},
		},
		"an episode's card asks nothing": {
			answers: []map[string]any{on},
			steps:   []string{"over e", "focus e", "show"},
			want:    []string{nothing},
		},
		"nothing during a video": {
			answers: []map[string]any{on},
			steps:   []string{"#/video", "over a", "show", "#/home", "over a", "answer", "show"},
			want:    []string{nothing, askAll, shownAll},
		},
		"in French": {
			lang:    "fr-FR",
			answers: []map[string]any{on},
			steps:   []string{"over a", "answer", "show"},
			want:    []string{askAll, "a[community 7.5|critics 88\u00a0%|audience 91\u00a0%] b[community 8.1] d[] f[critics 40\u00a0%]"},
		},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  let now = 1000000
  let user = 'boss'
  const classes = (element) => String(element.className || '').split(' ').filter(Boolean)
  // Selectors of class names, with [attribute] for one it must have.
  const matches = (element, selector) => selector.split(',').some((one) => {
    const m = /^((?:\.[\w-]+)+)((?:\[[\w-]+\])*)$/.exec(one.trim())
    if (!m || !element.attributes) return false
    return m[1].slice(1).split('.').every((c) => classes(element).includes(c)) &&
      (m[2].match(/[\w-]+/g) || []).every((name) => name in element.attributes)
  })
  const all = (root, selector) => (root.children || []).flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
  class Element {
    constructor(tag) { this.tagName = tag.toUpperCase(); this.className = ''; this.id = ''; this.children = []; this.parentNode = null; this.text = ''; this.attributes = {}; this.innerHTML = '' }
    get classList() {
      return {
        contains: (c) => classes(this).includes(c),
        add: (c) => { if (!classes(this).includes(c)) this.className = [...classes(this), c].join(' ') },
        remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' ') },
      }
    }
    set textContent(text) { this.text = text; this.children = [] }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    appendChild(child) { return this.insertBefore(child, null) }
    insertBefore(child, before) {
      if (child.parentNode) child.parentNode.removeChild(child)
      const at = before ? this.children.indexOf(before) : -1
      this.children.splice(at < 0 ? this.children.length : at, 0, child)
      child.parentNode = this
      return child
    }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; return child }
    replaceWith(other) { this.parentNode.insertBefore(other, this); this.parentNode.removeChild(this) }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    setAttribute(name, value) { this.attributes[name] = String(value) }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    removeAttribute(name) { delete this.attributes[name] }
    addEventListener() {}
    closest(selector) { for (let at = this; at && at.attributes; at = at.parentNode) if (matches(at, selector)) return at; return null }
  }
  const make = (tag, className, parent) => { const element = new Element(tag); element.className = className; if (parent) parent.appendChild(element); return element }
  const body = new Element('body')
  const sheets = []
  const document = {
    body,
    documentElement: { lang: scenario.lang || 'en-US', classList: { contains: () => false } },
    head: { appendChild: (sheet) => sheets.push(sheet) },
    createElement: (tag) => new Element(tag),
    createTextNode: (text) => ({ textContent: text, parentNode: null }),
    getElementById: (id) => sheets.find((sheet) => sheet.id === id) || null,
    querySelector: (selector) => body.querySelector(selector),
    querySelectorAll: (selector) => body.querySelectorAll(selector),
  }
  // jellyfin-web 12.4's cards: the poster in .cardScalable, inside
  // .cardBox, its image a link that takes the focus.
  const cards = {}
  const card = (id, type, extra) => {
    const element = make('div', 'card overflowPortraitCard card-hoverable')
    element.setAttribute('data-id', id.toUpperCase())
    element.setAttribute('data-type', type)
    const scalable = make('div', 'cardScalable', make('div', 'cardBox cardBox-bottompadded', element))
    make('div', 'cardPadder cardPadder-overflowPortrait', scalable)
    if (extra) make('div', extra, scalable)
    element.image = make('a', 'cardImageContainer coveredImage cardContent itemAction lazy', scalable)
    element.image.setAttribute('data-action', 'link')
    cards[id] = element
    return element
  }
  const page = make('div', 'page homePage libraryPage', body)
  const first = make('div', 'itemsContainer scrollSlider focuscontainer-x', page)
  first.appendChild(card('a', 'Movie'))
  first.appendChild(card('b', 'Series'))
  first.appendChild(card('e', 'Episode'))
  const second = make('div', 'itemsContainer scrollSlider focuscontainer-x', page)
  second.appendChild(card('d', 'Movie'))
  second.appendChild(card('f', 'Series', 'mediaSourceIndicator'))
  function show() {
    log.push(['a', 'b', 'd', 'f'].map((id) => {
      const badge = cards[id].querySelector('.polyfinPosterRatings')
      const ok = !badge || (badge.attributes['aria-hidden'] === 'true' && badge.parentNode === cards[id].querySelector('.cardScalable'))
      const scores = badge ? badge.children.map((score) => {
        const kind = classes(score).find((c) => c.startsWith('polyfinPosterRating-')).slice('polyfinPosterRating-'.length)
        const icon = score.children[0]
        const drawn = classes(icon).includes('polyfinPosterRatingIcon') && icon.innerHTML.startsWith('<svg')
        return kind + ' ' + score.children[1].textContent + (drawn ? '' : ' (no icon)')
      }) : []
      return id + '[' + scores.join('|') + ']' + (ok ? '' : ' (misplaced)')
    }).join(' '))
  }
  const listeners = {}
  const fire = (type, target) => { for (const listener of listeners[type] || []) listener({ type, target }) }
  const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home' }
  const history = { pushState: (state, title, url) => { location.hash = url; location.href = 'http://polyfin.test/web/' + url }, replaceState: () => {} }
  const answers = scenario.answers.slice()
  let pending = null
  const ApiClient = {
    serverId: () => 's',
    getCurrentUserId: () => user,
    getCurrentUser: () => Promise.resolve({ Id: user, Policy: { IsAdministrator: true } }),
    getUrl: (path, query) => 'http://polyfin.test/' + path + (query ? '?' + new URLSearchParams(query) : ''),
    getJSON: (url) => {
      const address = new URL(url)
      if (address.pathname !== '/Polyfin/Items/Ratings') return Promise.reject(new Error('not asked here'))
      log.push('ask ' + address.searchParams.get('Ids'))
      return new Promise((resolve, reject) => { pending = { resolve, reject } })
    },
    ajax: () => Promise.reject(new Error('not asked here')),
  }
  const store = {}
  const localStorage = { getItem: (name) => (name in store ? store[name] : null), setItem: (name, value) => { store[name] = String(value) }, removeItem: (name) => { delete store[name] } }
  class FakeDate extends Date { static now() { return now } }
  class MutationObserver { observe() {} disconnect() {} }
  vm.runInNewContext(script, {
    location, history, URL, URLSearchParams, document, MutationObserver, Object, JSON, String, Date: FakeDate,
    window: { ApiClient, localStorage, sessionStorage: localStorage },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
    setTimeout: () => 0, clearTimeout: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  await flush()
  for (const step of scenario.steps) {
    const [verb, what] = step.split(' ')
    if (step === 'show') show()
    else if (verb === 'over') fire('mouseover', cards[what].image)
    else if (verb === 'focus') fire('focusin', cards[what].image)
    else if (step === 'answer' || step === 'fail') {
      const request = pending
      pending = null
      if (!request) log.push('nothing to answer')
      else if (step === 'fail') request.reject(new Error('down'))
      else request.resolve(JSON.parse(JSON.stringify(answers.shift())))
    } else if (verb === 'redraw') {
      const old = cards[what]
      const parent = old.parentNode
      const fresh = card(what, old.getAttribute('data-type'))
      parent.insertBefore(fresh, old)
      parent.removeChild(old)
    } else if (verb === 'wait') now += Number(what)
    else if (verb === 'user') user = what
    else history.pushState(null, '', step)
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
		Lang    string           `json:"lang,omitempty"`
		Steps   []string         `json:"steps"`
		Answers []map[string]any `json:"answers"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.lang, s.steps, s.answers}
	}
	payload, err := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": inputs})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(payload))
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
