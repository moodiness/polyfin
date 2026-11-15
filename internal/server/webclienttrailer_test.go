package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The trailer's frame (the script's twenty-third part): YouTube's player
// refuses to play without a Referer, and the page's policy sends none.
// YouTube's script marks its frame with a referrerpolicy attribute, then
// gives it its address once it is in the page; some browsers apply the
// attribute only as a frame is inserted. The script gives the YouTube
// frames of jellyfin-web's trailer player referrerpolicy="strict-origin"
// and puts each back in its place, which loads it again with it, once for
// each address. Any other frame is left as it was.
//
// The page stands for jellyfin-web's: the trailer player is a
// .youtubePlayerContainer first in the body, holding a div#player that
// YouTube's script replaces with its frame, as jellyfin-web 12.4 and the
// YouTube player script do. Each load of a frame is logged with the
// attribute it had then, and whether the frame was inserted or given an
// address while in the page; "flush" delivers the page's changes to the
// observers, as the browser does after each task.
func TestTrailerFramesSendTheServersOrigin(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const embed = "https://www.youtube.com/embed/abcdefghijk?enablejsapi=1"
	const other = "https://www.youtube.com/embed/kjihgfedcba?enablejsapi=1"
	type scenario struct {
		steps []string
		want  []string
	}
	trailer := []string{
		"trailer loads, given its address, referrerpolicy strict-origin-when-cross-origin",
		"trailer loads, inserted, referrerpolicy strict-origin",
		"trailer: referrerpolicy strict-origin",
	}
	scenarios := map[string]scenario{
		"a trailer": {steps: []string{"player", "flush", "frame trailer " + embed, "flush", "flush", "frames"}, want: trailer},
		// The second trailer of a load: YouTube's script is there, and
		// makes the frame before the player's change is delivered.
		"the frame made with the player": {steps: []string{"player", "frame trailer " + embed, "flush", "flush", "frames"}, want: trailer},
		"the privacy-enhanced host": {steps: []string{"player", "flush", "frame trailer https://www.youtube-nocookie.com/embed/abcdefghijk", "flush", "frames"},
			want: trailer},
		"the body made after the script": {steps: []string{"body", "player", "flush", "frame trailer " + embed, "flush", "frames"}, want: trailer},
		"a new address, once": {steps: []string{"player", "flush", "frame trailer " + embed, "flush", "address trailer " + other, "flush", "flush", "frames"},
			want: []string{
				"trailer loads, given its address, referrerpolicy strict-origin-when-cross-origin",
				"trailer loads, inserted, referrerpolicy strict-origin",
				"trailer loads, given its address, referrerpolicy strict-origin",
				"trailer loads, inserted, referrerpolicy strict-origin",
				"trailer: referrerpolicy strict-origin",
			}},
		"a player closed, then another": {steps: []string{"player", "flush", "frame first " + embed, "flush", "close", "flush", "player", "flush", "frame second " + embed, "flush", "frames"},
			want: []string{
				"first loads, given its address, referrerpolicy strict-origin-when-cross-origin",
				"first loads, inserted, referrerpolicy strict-origin",
				"second loads, given its address, referrerpolicy strict-origin-when-cross-origin",
				"second loads, inserted, referrerpolicy strict-origin",
				"second: referrerpolicy strict-origin",
			}},
		// Another site's frame in the player, and frames elsewhere, a
		// YouTube embed among them.
		"other frames": {steps: []string{"player", "flush", "frame site https://video.example/embed/1", "elsewhere page " + embed, "elsewhere banner https://banner.example/",
			"flush", "flush", "frames"},
			want: []string{
				"site loads, given its address, referrerpolicy strict-origin-when-cross-origin",
				"page loads, inserted, referrerpolicy none",
				"banner loads, inserted, referrerpolicy none",
				"site: referrerpolicy strict-origin-when-cross-origin",
				"page: referrerpolicy none",
				"banner: referrerpolicy none",
			}},
	}
	const harness = scriptRunner + `
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
function run(steps) {
  const log = []
  // The page's changes not yet delivered, and the observers.
  let records = []
  const observers = []
  const classes = (element) => element.className.split(' ').filter(Boolean)
  const matches = (element, selector) => {
    if (selector[0] === '#') return element.id === selector.slice(1)
    if (selector[0] === '.') return selector.slice(1).split('.').every((c) => classes(element).includes(c))
    return element.tagName === selector.toUpperCase()
  }
  const all = (root, selector) => root.children.flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
  class Element {
    constructor(tag, className) { this.tagName = tag.toUpperCase(); this.className = className || ''; this.id = ''; this.children = []; this.parentNode = null; this.attributes = {}; this.style = {} }
    get classList() {
      return {
        contains: (c) => classes(this).includes(c),
        add: (c) => { if (!classes(this).includes(c)) this.className = [...classes(this), c].join(' ') },
        remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' ') },
        toggle: () => {},
      }
    }
    get nextSibling() { const siblings = this.parentNode ? this.parentNode.children : []; return siblings[siblings.indexOf(this) + 1] || null }
    get connected() { for (let at = this; at; at = at.parentNode) if (at === html) return true; return false }
    get src() { return this.getAttribute('src') || '' }
    set src(address) { this.setAttribute('src', address) }
    // A frame loads as it is inserted in the page with an address, or
    // given one while in it.
    load(how) { log.push(this.name + ' loads, ' + how + ', referrerpolicy ' + (this.getAttribute('referrerpolicy') || 'none')) }
    inserted() { for (const frame of [this, ...all(this, 'iframe')]) if (frame.tagName === 'IFRAME' && frame.src) frame.load('inserted') }
    insertBefore(child, before) {
      if (child.parentNode) child.parentNode.removeChild(child)
      const at = before ? this.children.indexOf(before) : -1
      if (at < 0) this.children.push(child)
      else this.children.splice(at, 0, child)
      child.parentNode = this
      records.push({ type: 'childList', target: this })
      if (child.connected) child.inserted()
      return child
    }
    appendChild(child) { return this.insertBefore(child, null) }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; records.push({ type: 'childList', target: this }); return child }
    replaceChild(child, old) { const before = old.nextSibling; this.removeChild(old); return this.insertBefore(child, before) }
    contains(node) { for (let at = node; at; at = at.parentNode) if (at === this) return true; return false }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    getElementsByTagName(tag) { return all(this, tag) }
    getAttribute(name) { name = name.toLowerCase(); return name in this.attributes ? this.attributes[name] : null }
    setAttribute(name, value) {
      name = name.toLowerCase()
      this.attributes[name] = String(value)
      records.push({ type: 'attributes', target: this, attributeName: name })
      if (name === 'src' && this.tagName === 'IFRAME' && this.connected) this.load('given its address')
    }
    removeAttribute(name) { delete this.attributes[name.toLowerCase()] }
    addEventListener() {}
    removeEventListener() {}
    getBoundingClientRect() { return { top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 } }
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target, options) { observers.push({ observer: this, target, options }) }
    disconnect() { for (let i = observers.length - 1; i >= 0; i--) if (observers[i].observer === this) observers.splice(i, 1) }
    takeRecords() { return [] }
  }
  const seen = (o, record) => {
    if (record.target !== o.target && !(o.options.subtree && o.target.contains(record.target))) return false
    if (record.type === 'childList') return !!o.options.childList
    return !!o.options.attributes && (!o.options.attributeFilter || o.options.attributeFilter.includes(record.attributeName))
  }
  // flush delivers the changes to the observers, and those their own
  // changes make, as the browser does.
  const flush = () => {
    for (let round = 0; records.length; round++) {
      if (round === 20) throw new Error('the observers never settle')
      const due = records
      records = []
      for (const o of [...observers]) {
        const mine = due.filter((record) => seen(o, record))
        if (mine.length) try { o.observer.callback(mine, o.observer) } catch (error) {}
      }
    }
  }
  const html = new Element('html', 'layout-desktop')
  const head = html.appendChild(new Element('head'))
  const body = new Element('body')
  const listeners = {}
  const fire = (type) => { for (const listener of listeners[type] || []) listener({ type, target: document }) }
  const document = {
    body: steps[0] === 'body' ? null : html.appendChild(body), documentElement: html, head, cookie: '', readyState: steps[0] === 'body' ? 'loading' : 'complete',
    createElement: (tag) => new Element(tag), createTextNode: () => new Element('#text'),
    getElementById: (id) => all(html, '#' + id)[0] || null,
    getElementsByTagName: (tag) => all(html, tag),
    querySelector: (selector) => html.querySelector(selector),
    querySelectorAll: (selector) => html.querySelectorAll(selector),
  }
  const location = { protocol: 'http:', host: 'polyfin.test', pathname: '/web/', search: '', hash: '#/details?id=0123456789abcdef0123456789abcdef', href: 'http://polyfin.test/web/#/details?id=0123456789abcdef0123456789abcdef', replace: () => {}, reload: () => {} }
  const storage = { getItem: () => null, setItem: () => {}, removeItem: () => {} }
  const context = {
    location, document, MutationObserver,
    history: { state: null, length: 1, pushState: () => {}, replaceState: () => {}, back: () => {} },
    window: { localStorage: storage, sessionStorage: storage, location },
    navigator: { userAgent: 'Node' }, Date, Promise, JSON, Math, Object, Array, String, Number, RegExp, isFinite,
    CustomEvent: class { constructor(type) { this.type = type } },
    setTimeout: () => 0, clearTimeout: () => {}, setInterval: () => 0, clearInterval: () => {},
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    removeEventListener: (type, listener) => { listeners[type] = (listeners[type] || []).filter((kept) => kept !== listener) },
    innerHeight: 800, scrollTo: () => {}, requestAnimationFrame: () => 0,
  }
  runScript(script, context)
  const frames = {}
  for (const step of steps) {
    const [word, name, address] = step.split(' ')
    if (word === 'body') {
      document.body = html.appendChild(body)
      document.readyState = 'interactive'
      fire('DOMContentLoaded')
    } else if (word === 'player') {
      // jellyfin-web's trailer player, first in the body.
      const player = new Element('div', 'youtubePlayerContainer')
      player.appendChild(new Element('div')).id = 'player'
      body.insertBefore(player, body.children[0] || null)
    } else if (word === 'frame') {
      // YouTube's script, as its player script does: the frame takes the
      // place of div#player, then gets its address.
      const placeholder = document.getElementById('player')
      const frame = frames[name] = new Element('iframe')
      frame.name = name
      frame.setAttribute('referrerPolicy', 'strict-origin-when-cross-origin')
      placeholder.parentNode.replaceChild(frame, placeholder)
      frame.src = address
    } else if (word === 'address') {
      frames[name].src = address
    } else if (word === 'elsewhere') {
      const frame = frames[name] = new Element('iframe')
      frame.name = name
      frame.src = address
      body.appendChild(new Element('div', 'page')).appendChild(frame)
    } else if (word === 'close') {
      body.removeChild(body.querySelector('.youtubePlayerContainer'))
    } else if (word === 'flush') {
      flush()
    } else if (word === 'frames') {
      for (const frame of all(html, 'iframe')) log.push(frame.name + ': referrerpolicy ' + (frame.getAttribute('referrerpolicy') || 'none'))
    }
  }
  return log
}
const results = {}
for (const [name, steps] of Object.entries(scenarios)) {
  try {
    results[name] = run(steps)
  } catch (error) {
    results[name] = ['harness: ' + error.stack]
  }
}
process.stdout.write(JSON.stringify(results))
`
	inputs := map[string][]string{}
	for name, s := range scenarios {
		inputs[name] = s.steps
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
