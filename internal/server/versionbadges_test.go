package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The script shows the badges of a version (the twenty-first part): on a
// title's page, a row of pills for the version picked, right under the
// version menu, after its container where its label sits beside it, as in
// jellyfin-web's own look, last in it where the label sits over it, as in
// LumaaGlaass, or last in the track menus' block when the menu is hidden
// for one version, which follows a pick; the version menu's tooltip names
// the version picked as its addon does, even while the setting is off,
// which draws no pill; and during a video, a row after the title's
// heading of the controls, from the version the device plays, asked again
// every 2 seconds, 5 times at most, while the answer names no title. Each
// pill shows its text, with its label in full as its aria-label and its
// tooltip. It runs in Node.js, in a context that stands for the browser
// and jellyfin-web 12.4's title page and video controls.
func TestVersionBadgesShowForTheVersionPickedAndPlayed(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type scenario struct {
		// lang is the document's language, "en" when empty.
		lang string
		// theme "lumaaglaass" lays the version menu's container out as a
		// column, its label over the menu, as LumaaGlaass does; else it is
		// a row, as in jellyfin-web's own look.
		theme string
		// steps: "items <answer>" and "playback <answer>" set what Polyfin
		// answers /Polyfin/Items/{id}/Badges and /Polyfin/Playback/Badges
		// from then on (see the harness's answers); "route" opens the
		// title's page, its version menu listing the title and v1, "route
		// one" with the title alone, the menu then hidden, "route focused"
		// with the focus in the version menu; "home" leaves it; "focus" and
		// "blur" give the version menu the focus and take it; "pick
		// <value>" picks a version as a user does, the menu keeping the
		// focus; "list <value>" has the page list one more version;
		// "redraw" has jellyfin-web draw the page anew; "push <count>" has
		// Polyfin push the title's versions; "video" starts a video, its
		// controls showing; "osd hide" and "osd show" hide and show them;
		// "next" plays another video in the same element; "end" ends the
		// video, back to the title's page when one was open; "wait <ms>"
		// lets time pass; "row", "tooltip", "osd" and "asks" log the title
		// page's row, the version menu's tooltip, the controls' row and the
		// asks so far.
		steps []string
		want  []string
	}
	const (
		rowTitle  = "row under the menu: 4K [4K Ultra HD] | DV [Dolby Vision] | HEVC [HEVC (H.265)]"
		rowV1     = "row under the menu: 1080p [Full HD] | Atmos [Dolby Atmos] | FR [Audio: French]"
		tipTitle  = "tooltip Movie 2160p WEB-DL DV HEVC [Addon A]"
		tipV1     = "tooltip Movie 1080p BluRay Atmos FR [Addon B]"
		osdPlayed = "osd after the title: 1080p [Full HD] | Atmos [Dolby Atmos] | FR [Audio: French]"
	)
	scenarios := map[string]scenario{
		"pills for the version picked": {
			steps: []string{"items full", "route", "row", "tooltip", "asks"},
			want:  []string{rowTitle, tipTitle, "asks items 1, playback 0"}},
		"a pick changes them at once": {
			steps: []string{"items full", "route", "focus", "pick v1", "row", "tooltip", "pick t", "row", "tooltip"},
			want:  []string{rowV1, tipV1, rowTitle, tipTitle}},
		"settings off: no pill, the tooltip stays": {
			steps: []string{"items hidden", "route", "row", "tooltip", "pick v1", "row", "tooltip"},
			want:  []string{"row none", tipTitle, "row none", tipV1}},
		"one version: the row goes after the track menus": {
			steps: []string{"items one", "route one", "row", "tooltip"},
			want:  []string{"row after the track menus: 720p [HD] | EN [Audio: English]", "tooltip Movie 720p [Addon C]"}},
		"LumaaGlaass: the row goes last in the version menu's container": {theme: "lumaaglaass",
			steps: []string{"items full", "route", "row", "pick v1", "row", "redraw", "row"},
			want: []string{"row in the menu's container: 4K [4K Ultra HD] | DV [Dolby Vision] | HEVC [HEVC (H.265)]",
				"row in the menu's container: 1080p [Full HD] | Atmos [Dolby Atmos] | FR [Audio: French]",
				"row in the menu's container: 1080p [Full HD] | Atmos [Dolby Atmos] | FR [Audio: French]"}},
		"LumaaGlaass, one version: the row goes after the track menus": {theme: "lumaaglaass",
			steps: []string{"items one", "route one", "row"},
			want:  []string{"row after the track menus: 720p [HD] | EN [Audio: English]"}},
		"nothing changes while the menu has focus": {
			steps: []string{"items full", "route focused", "row", "tooltip", "blur", "row", "tooltip"},
			want:  []string{"row none", "tooltip none", rowTitle, tipTitle}},
		"jellyfin-web's renders find the row again": {
			steps: []string{"items full", "route", "redraw", "row", "tooltip", "asks"},
			want:  []string{rowTitle, tipTitle, "asks items 1, playback 0"}},
		// A version listed after the answer, then picked: it shows once
		// the second ask answers, a second later, and the menu, which the
		// pick left with the focus, loses it.
		"a version the answer does not know asks again": {
			steps: []string{"items full", "route", "list v2", "pick v2", "row", "tooltip", "items more", "wait 1000", "row", "blur", "row",
				"tooltip", "asks"},
			want: []string{"row none", "tooltip Movie 480p", "row none", "row under the menu: 480p [SD]", "tooltip Movie 480p DVD [Addon D]",
				"asks items 2, playback 0"}},
		"a push of another number of versions asks again": {
			steps: []string{"items full", "route", "push 2", "wait 1000", "asks", "push 3", "push 3", "wait 1000", "asks"},
			want:  []string{"asks items 1, playback 0", "asks items 2, playback 0"}},
		"leaving the title takes the row and the tooltip off": {
			steps: []string{"items full", "route", "home", "row", "tooltip"},
			want:  []string{"row none", "tooltip none"}},
		"the language asked is the page's": {lang: "fr",
			steps: []string{"items full", "playback full", "route", "video", "asks"},
			want: []string{"asks items 1, playback 1", "asked Polyfin/Items/0123456789abcdef0123456789abcdef/Badges?language=fr",
				"asked Polyfin/Playback/Badges?language=fr"}},
		// jellyfin-web tells the server of the play a moment after the
		// video starts: the first answer names no title.
		"during a video, the version played once the server knows it": {
			steps: []string{"playback empty", "video", "osd", "asks", "wait 2000", "asks", "playback full", "wait 2000", "osd", "asks",
				"wait 10000", "asks"},
			want: []string{"osd none", "asks items 0, playback 1", "asks items 0, playback 2", osdPlayed, "asks items 0, playback 3",
				"asks items 0, playback 3"}},
		"asked again 5 times at most": {
			steps: []string{"playback empty", "video", "wait 2000", "wait 2000", "wait 2000", "wait 2000", "wait 2000", "wait 2000",
				"wait 2000", "asks", "osd"},
			want: []string{"asks items 0, playback 6", "osd none"}},
		"asked once the controls show": {
			steps: []string{"playback full", "osd hide", "video", "asks", "osd", "osd show", "asks", "osd"},
			want:  []string{"asks items 0, playback 0", "osd none", "asks items 0, playback 1", osdPlayed}},
		"settings off: no pill during a video": {
			steps: []string{"playback hidden", "video", "osd", "asks", "wait 4000", "asks"},
			want:  []string{"osd none", "asks items 0, playback 1", "asks items 0, playback 1"}},
		"the row goes as the video ends, and a new play asks again": {
			steps: []string{"playback full", "video", "osd", "end", "osd", "video", "osd", "next", "osd", "asks"},
			want:  []string{osdPlayed, "osd none", osdPlayed, osdPlayed, "asks items 0, playback 3"}},
		// The row stays on the page hidden under the video, which shows it
		// again at once, and asks again in case the play analyzed a version.
		"the title page changes nothing during a video": {
			steps: []string{"items full", "route", "playback full", "video", "row", "tooltip", "end", "row", "tooltip", "asks"},
			want:  []string{rowTitle, tipTitle, rowTitle, tipTitle, "asks items 2, playback 1"}},
	}
	const harness = scriptRunner + `
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
const TITLE = '0123456789abcdef0123456789abcdef'
const badge = (Kind, Text, Label) => ({ Kind, Text, Label })
const title = { Id: TITLE, ETag: 'e0', Name: 'Movie 2160p WEB-DL DV HEVC [Addon A]',
  Badges: [badge('resolution', '4K', '4K Ultra HD'), badge('range', 'DV', 'Dolby Vision'), badge('codec', 'HEVC', 'HEVC (H.265)')] }
const v1 = { Id: 'v1', ETag: 'e1', Name: 'Movie 1080p BluRay Atmos FR [Addon B]',
  Badges: [badge('resolution', '1080p', 'Full HD'), badge('audio', 'Atmos', 'Dolby Atmos'), badge('language', 'FR', 'Audio: French')] }
const v2 = { Id: 'v2', ETag: 'e2', Name: 'Movie 480p DVD [Addon D]', Badges: [badge('resolution', '480p', 'SD')] }
const one = { Id: TITLE, ETag: 'e0', Name: 'Movie 720p [Addon C]', Badges: [badge('resolution', '720p', 'HD'), badge('language', 'EN', 'Audio: English')] }
const answers = {
  'items full': { Show: true, Versions: [title, v1] },
  'items more': { Show: true, Versions: [title, v1, v2] },
  'items hidden': { Show: false, Versions: [title, v1] },
  'items one': { Show: true, Versions: [one] },
  'playback empty': { Show: true, ItemId: '', Name: '', Badges: [] },
  'playback full': { Show: true, ItemId: TITLE, Name: v1.Name, Badges: v1.Badges },
  'playback hidden': { Show: false, ItemId: TITLE, Name: v1.Name, Badges: v1.Badges },
}
// The option texts jellyfin-web lists, as Polyfin names the versions.
const optionText = { [TITLE]: 'Movie 2160p', v1: 'Movie 1080p', v2: 'Movie 480p' }

// A small DOM: elements with classes, attributes, children and events,
// selectors of tags, classes, ids and :not(), with descendants, and
// observers of children and of attributes.
function makeDom(lang) {
  const observers = new Set()
  const pending = new Map()
  const notify = (observer, record) => {
    if (!pending.has(observer)) {
      pending.set(observer, [])
      queueMicrotask(() => {
        const records = pending.get(observer)
        pending.delete(observer)
        if (observers.has(observer)) try { observer.callback(records, observer) } catch (error) {}
      })
    }
    pending.get(observer).push(record)
  }
  const mutated = (target, record) => {
    for (const observer of observers) {
      for (const [watched, options] of observer.targets) {
        const within = watched === target || (options.subtree && watched.contains(target))
        if (!within) continue
        if (record.type === 'childList' && options.childList) notify(observer, record)
        if (record.type === 'attributes' && options.attributes && (!options.attributeFilter || options.attributeFilter.includes(record.attributeName))) notify(observer, record)
      }
    }
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback; this.targets = new Map() }
    observe(target, options) { this.targets.set(target, options || {}); observers.add(this) }
    disconnect() { this.targets.clear(); observers.delete(this) }
  }
  const compound = (text) => {
    const parts = { tag: '', classes: [], ids: [], not: [] }
    const re = /^([a-z][a-z0-9]*|\*)|\.([\w-]+)|#([\w-]+)|:not\(([^)]*)\)|\[([^\]]*)\]/gi
    let at = 0
    let match
    while (at < text.length && (match = re.exec(text)) && match.index === at) {
      if (match[1]) parts.tag = match[1] === '*' ? '' : match[1].toUpperCase()
      else if (match[2]) parts.classes.push(match[2])
      else if (match[3]) parts.ids.push(match[3])
      else if (match[4]) parts.not.push(compound(match[4]))
      else parts.attrs = (parts.attrs || []).concat(match[5])
      at = re.lastIndex
    }
    if (at !== text.length) parts.unknown = true
    return parts
  }
  const fits = (element, parts) => {
    if (parts.unknown || !element.classList) return false
    if (parts.tag && element.tagName !== parts.tag) return false
    if (!parts.classes.every((c) => element.classList.contains(c))) return false
    if (!parts.ids.every((id) => element.id === id)) return false
    if (parts.attrs && !parts.attrs.every((a) => { const [name, value] = a.split('='); return value === undefined ? element.getAttribute(name) !== null : element.getAttribute(name) === value.replace(/["']/g, '') })) return false
    return !parts.not.some((not) => fits(element, not))
  }
  const matches = (element, selector) => selector.split(',').some((one) => {
    const chain = one.trim().split(/\s+/).map(compound)
    if (!fits(element, chain[chain.length - 1])) return false
    let at = element.parentNode
    for (let i = chain.length - 2; i >= 0; i--) {
      while (at && !fits(at, chain[i])) at = at.parentNode
      if (!at) return false
      at = at.parentNode
    }
    return true
  })
  class Element {
    constructor(tag, className) {
      this.tagName = tag.toUpperCase(); this.id = ''; this.children = []; this.parentNode = null
      this.attributes = {}; this.handlers = {}; this.style = {}; this.ownText = ''
      this.attributes.class = className || ''
    }
    get className() { return this.attributes.class }
    set className(value) { this.setAttribute('class', value) }
    get classList() {
      const list = () => this.className.split(/\s+/).filter(Boolean)
      return {
        contains: (c) => list().includes(c),
        add: (c) => { if (!list().includes(c)) this.className = list().concat(c).join(' ') },
        remove: (c) => { if (list().includes(c)) this.className = list().filter((x) => x !== c).join(' ') },
        toggle: (c, force) => { if (force === undefined ? !list().includes(c) : force) this.classList.add(c); else this.classList.remove(c) },
      }
    }
    get title() { return this.getAttribute('title') || '' }
    set title(value) { this.setAttribute('title', value) }
    get textContent() { return this.ownText + this.children.map((child) => child.textContent).join('') }
    set textContent(text) { for (const child of this.children.slice()) this.removeChild(child); this.ownText = String(text); mutated(this, { type: 'childList' }) }
    get nextSibling() { const p = this.parentNode; return p ? p.children[p.children.indexOf(this) + 1] || null : null }
    get previousSibling() { const p = this.parentNode; return p ? p.children[p.children.indexOf(this) - 1] || null : null }
    get firstChild() { return this.children[0] || null }
    get isConnected() { let at = this; while (at.parentNode) at = at.parentNode; return at === html }
    appendChild(child) { return this.insertBefore(child, null) }
    insertBefore(child, before) {
      if (child.parentNode) child.parentNode.removeChild(child)
      const at = before ? this.children.indexOf(before) : -1
      if (at < 0) this.children.push(child)
      else this.children.splice(at, 0, child)
      child.parentNode = this
      mutated(this, { type: 'childList' })
      return child
    }
    removeChild(child) {
      this.children.splice(this.children.indexOf(child), 1)
      child.parentNode = null
      mutated(this, { type: 'childList' })
      return child
    }
    contains(node) { for (let at = node; at; at = at.parentNode) if (at === this) return true; return false }
    setAttribute(name, value) {
      this.attributes[name] = String(value)
      mutated(this, { type: 'attributes', attributeName: name })
    }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    removeAttribute(name) { delete this.attributes[name]; mutated(this, { type: 'attributes', attributeName: name }) }
    hasAttribute(name) { return name in this.attributes }
    matches(selector) { return matches(this, selector) }
    closest(selector) { for (let at = this; at && at.tagName; at = at.parentNode) if (matches(at, selector)) return at; return null }
    querySelectorAll(selector) {
      const found = []
      const walk = (element) => { for (const child of element.children) { if (matches(child, selector)) found.push(child); walk(child) } }
      walk(this)
      return found
    }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null }
    addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler) }
    removeEventListener(type, handler) { this.handlers[type] = (this.handlers[type] || []).filter((h) => h !== handler) }
    dispatchEvent(event) { event.target = event.target || this; for (const handler of (this.handlers[event.type] || []).slice()) handler.call(this, event); return true }
    getBoundingClientRect() { return { left: 0, top: 0, width: 0, height: 0, right: 0, bottom: 0 } }
    focus() {}
    blur() {}
  }
  // A select whose options are its children.
  class Select extends Element {
    get options() { return this.children }
    get selectedIndex() { return this.children.findIndex((option) => option.selected) }
    get value() { const option = this.children.find((option) => option.selected); return option ? option.value : '' }
    set value(value) { for (const option of this.children) option.selected = option.value === value }
  }
  const html = new Element('html')
  html.lang = lang
  const head = new Element('head')
  const body = new Element('body')
  html.appendChild(head)
  html.appendChild(body)
  const make = (tag, className, parent) => {
    const element = tag === 'select' ? new Select(tag, className) : new Element(tag, className)
    if (parent) parent.appendChild(element)
    return element
  }
  return { html, head, body, make, MutationObserver, Element }
}

async function run(scenario) {
  const log = []
  const dom = makeDom(scenario.lang || 'en')
  const { html, head, body, make } = dom
  let focused = null
  const document = {
    documentElement: html, head, body, readyState: 'complete', cookie: '',
    get activeElement() { return focused || body },
    createElement: (tag) => make(tag),
    createTextNode: (text) => { const node = make('#text'); node.ownText = String(text); return node },
    getElementById: (id) => html.querySelector('#' + id),
    querySelector: (selector) => html.querySelector(selector),
    querySelectorAll: (selector) => html.querySelectorAll(selector),
    getElementsByTagName: (tag) => html.querySelectorAll(tag),
  }
  let now = 0
  let timers = []
  const setTimeout = (run, delay) => { const timer = { run, at: now + (delay || 0) }; timers.push(timer); return timer }
  const clearTimeout = (timer) => { timers = timers.filter((t) => t !== timer) }
  const listeners = {}
  const fire = (type, event) => { for (const listener of (listeners[type] || []).slice()) listener(event) }
  const location = { href: 'http://polyfin.test/web/#/home', hash: '#/home', pathname: '/web/', search: '', protocol: 'http:', host: 'polyfin.test', replace: () => {} }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  const answer = {}
  const asks = { items: 0, playback: 0 }
  const asked = []
  const subscribers = {}
  const ApiClient = {
    serverId: () => 's',
    getCurrentUserId: () => 'u',
    getCurrentUser: () => new Promise(() => {}),
    getUrl: (path, params) => 'http://polyfin.test/' + path + (params ? '?' + Object.keys(params).map((k) => k + '=' + encodeURIComponent(params[k])).join('&') : ''),
    getJSON: (url) => {
      const path = url.replace('http://polyfin.test/', '')
      const kind = /^Polyfin\/Items\/[0-9a-f]{32}\/Badges(\?|$)/.test(path) ? 'items' : /^Polyfin\/Playback\/Badges(\?|$)/.test(path) ? 'playback' : ''
      if (!kind) return Promise.reject(new Error('not asked here'))
      asks[kind]++
      asked.push(path)
      return Promise.resolve(JSON.parse(JSON.stringify(answer[kind])))
    },
    getItem: () => Promise.reject(new Error('not asked here')),
    ajax: () => Promise.reject(new Error('not asked here')),
    subscribe: (types, callback) => { for (const type of types) (subscribers[type] ||= []).push(callback); return () => {} },
  }
  const storage = { getItem: () => null, setItem: () => {}, removeItem: () => {} }
  runScript(script, {
    location, history, document, MutationObserver: dom.MutationObserver, Date,
    window: { ApiClient, localStorage: storage, sessionStorage: storage },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
    setTimeout, clearTimeout, setInterval: () => 0, clearInterval: () => {},
    // The track menus' containers are flex boxes: rows in jellyfin-web's
    // own look, columns in LumaaGlaass.
    getComputedStyle: (element) => {
      const container = element.classList.contains('selectContainer')
      return { display: container ? 'flex' : 'block', flexDirection: container && scenario.theme === 'lumaaglaass' ? 'column' : 'row' }
    },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  await flush()

  // jellyfin-web 12.4's title page (itemDetails/index.html): its track
  // menus' form, the version menu first, in its container.
  let page = null
  let menu = null
  const route = '#/details?id=' + TITLE + '&serverId=s'
  function openPage(ids) {
    page = make('div', 'page itemDetailPage', body)
    const section = make('div', 'detailSection', make('div', 'detailPagePrimaryContent', page))
    const form = make('form', 'trackSelections focuscontainer-x', section)
    const container = make('div', 'selectContainer selectSourceContainer trackSelectionFieldContainer' + (ids.length < 2 ? ' hide' : ''), form)
    make('label', 'selectLabel', container).textContent = 'Version'
    menu = make('select', 'selectSource detailTrackSelect', container)
    fill(ids, ids[0])
    for (const name of ['Video', 'Audio', 'Subtitles']) make('select', 'select' + name + ' detailTrackSelect', make('div', 'selectContainer select' + name + 'Container', form))
    make('div', 'detailSectionContent', section)
  }
  function fill(ids, picked) {
    menu.textContent = ''
    for (const id of ids) {
      const option = make('option', '', menu)
      option.value = id
      option.textContent = optionText[id]
    }
    menu.value = picked
  }
  // The video, as jellyfin-web 12.4 plays one (htmlVideoPlayer): the
  // player and its element, which starts playing; then the route to its
  // page of controls (playback/video/index.html).
  let video = null
  let player = null
  let osdPage = null
  let osdHidden = false
  function startVideo() {
    player = make('div', 'videoPlayerContainer', body)
    video = make('video', 'htmlvideoplayer', player)
    video.src = 'blob:http://polyfin.test/1'
    video.currentSrc = video.src
    fire('playing', { type: 'playing', target: video })
    if (page) page.classList.add('hide')
    osdPage = make('div', 'page libraryPage', body)
    osdPage.id = 'videoOsdPage'
    const bottom = make('div', 'videoOsdBottom videoOsdBottom-maincontrols' + (osdHidden ? ' videoOsdBottom-hidden' : ''), osdPage)
    const text = make('div', 'osdTextContainer osdMainTextContainer', make('div', 'osdControls', bottom))
    make('h3', 'osdTitle', text)
    make('div', 'osdMediaStatus hide', text)
    history.pushState(null, '', '#/video')
  }
  function pills(row) {
    return row.querySelectorAll('.polyfinVersionBadge').map((pill) => {
      const label = pill.getAttribute('aria-label')
      return pill.textContent + ' [' + label + ']' + (pill.getAttribute('title') === label ? '' : ' title ' + pill.getAttribute('title'))
    }).join(' | ')
  }
  for (const step of scenario.steps) {
    const [verb, ...args] = step.split(' ')
    if (verb === 'items' || verb === 'playback') answer[verb] = answers[step]
    else if (verb === 'route') {
      openPage(args[0] === 'one' ? [TITLE] : [TITLE, 'v1'])
      // The menu may have the focus before Polyfin answers.
      if (args[0] === 'focused') focused = menu
      history.pushState(null, '', route)
    } else if (verb === 'home') {
      page.classList.add('hide')
      history.pushState(null, '', '#/home')
    } else if (verb === 'focus') focused = menu
    else if (verb === 'blur') {
      focused = null
      menu.dispatchEvent({ type: 'blur' })
    } else if (verb === 'pick') {
      // A user picks as the menu closes: it keeps the focus.
      focused = menu
      menu.value = args[0] === 't' ? TITLE : args[0]
      menu.dispatchEvent({ type: 'change', bubbles: true })
    } else if (verb === 'list') {
      const option = make('option', '', menu)
      option.value = args[0]
      option.textContent = optionText[args[0]]
    } else if (verb === 'redraw') {
      // jellyfin-web draws the title's page anew, as a new view of the
      // same route, the version picked kept.
      const picked = menu.value
      body.removeChild(page)
      openPage([TITLE, 'v1'])
      menu.value = picked
    } else if (verb === 'push') {
      const message = { MessageType: 'PolyfinVersions', Data: { ItemId: TITLE.toUpperCase(), Pending: 0, Count: Number(args[0]), Known: Number(args[0]), AddVersions: true } }
      for (const callback of subscribers.PolyfinVersions || []) callback(message)
    } else if (verb === 'video') {
      startVideo()
    } else if (verb === 'next') {
      fire('emptied', { type: 'emptied', target: video })
      video.src = 'blob:http://polyfin.test/2'
      video.currentSrc = video.src
      fire('playing', { type: 'playing', target: video })
    } else if (verb === 'end') {
      // jellyfin-web goes back to the page before, then lets the player go.
      fire('ended', { type: 'ended', target: video })
      osdPage.classList.add('hide')
      if (page) {
        page.classList.remove('hide')
        history.pushState(null, '', route)
      }
      body.removeChild(player)
    } else if (verb === 'osd' && args[0] === 'hide') {
      osdHidden = true
      const bottom = osdPage && osdPage.querySelector('.videoOsdBottom')
      if (bottom) bottom.classList.add('videoOsdBottom-hidden')
    } else if (verb === 'osd' && args[0] === 'show') {
      osdHidden = false
      const bottom = osdPage && osdPage.querySelector('.videoOsdBottom')
      if (bottom) bottom.classList.remove('videoOsdBottom-hidden')
    } else if (verb === 'wait') {
      const until = now + Number(args[0])
      for (;;) {
        await flush()
        const due = timers.filter((t) => t.at <= until).sort((a, b) => a.at - b.at)[0]
        if (!due) break
        now = due.at
        timers = timers.filter((t) => t !== due)
        try { due.run() } catch (error) {}
      }
      now = until
    }
    await flush()
    if (verb === 'row') {
      const rows = page ? page.querySelectorAll('.polyfinVersionBadges') : []
      const row = rows[0]
      const container = page && page.querySelector('.selectSourceContainer')
      const form = page && page.querySelector('.trackSelections')
      const where = !row ? ''
        : container && container.nextSibling === row ? 'under the menu'
        : row.parentNode === container && !row.nextSibling ? "in the menu's container"
        : row.parentNode === form && !row.nextSibling ? 'after the track menus'
        : 'elsewhere'
      log.push(rows.length > 1 ? 'rows ' + rows.length : row ? 'row ' + where + ': ' + pills(row) : 'row none')
    } else if (verb === 'tooltip') log.push('tooltip ' + (menu && menu.getAttribute('title') !== null ? menu.getAttribute('title') : 'none'))
    else if (verb === 'osd' && !args.length) {
      const rows = osdPage ? osdPage.querySelectorAll('.polyfinVersionBadges') : []
      const row = rows[0]
      const heading = osdPage && osdPage.querySelector('.osdTitle')
      const where = row && heading.nextSibling === row ? 'after the title' : 'elsewhere'
      log.push(rows.length > 1 ? 'osd rows ' + rows.length : row ? 'osd ' + where + ': ' + pills(row) : 'osd none')
    } else if (verb === 'asks') {
      log.push('asks items ' + asks.items + ', playback ' + asks.playback)
      if (scenario.lang) for (const path of asked) log.push('asked ' + path)
    }
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
		Lang  string   `json:"lang,omitempty"`
		Theme string   `json:"theme,omitempty"`
		Steps []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.lang, s.theme, s.steps}
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
