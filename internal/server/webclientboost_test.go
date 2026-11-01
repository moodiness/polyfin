package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// Boost the volume (the script's twentieth part): a video of a user who
// boosts goes through a gain and a compressor, its element hooked once,
// once the AudioContext runs; an element whose sound Web Audio would play
// silent is left alone: native HLS, another site's stream without CORS,
// the server's own stream unless marked relayed. Turning the boost off
// sends a hooked element straight to the speakers. The player's settings
// menu gets an entry that turns it on or off and posts the choice; a video
// that cannot be boosted now is from the next one, which a toast says.
func TestVideosAreBoostedForTheUsersWhoAsk(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		relayed  = "http://polyfin.test/Videos/a/stream.mkv?PolyfinGrant=g&PolyfinRelay=1&static=true"
		own      = "http://polyfin.test/Videos/a/stream.mkv?PolyfinGrant=g&static=true"
		boosted  = "source>gain>compressor>speakers"
		straight = "source>speakers"
		alone    = "alone"
	)
	type scenario struct {
		answers []bool
		lang    string
		steps   []string
		want    []string
	}
	scenarios := map[string]scenario{
		"a relayed stream":                {answers: []bool{true}, steps: []string{"video " + relayed, "play", "show"}, want: []string{boosted}},
		"hls.js":                          {answers: []bool{true}, steps: []string{"video blob:http://polyfin.test/1", "play", "show"}, want: []string{boosted}},
		"a file fetched with CORS":        {answers: []bool{true}, steps: []string{"video cors " + own, "play", "show"}, want: []string{boosted}},
		"the server's own stream":         {answers: []bool{true}, steps: []string{"video " + own, "play", "show"}, want: []string{alone}},
		"another site's stream":           {answers: []bool{true}, steps: []string{"video https://cdn.example/a.mp4?PolyfinRelay=1", "play", "show"}, want: []string{alone}},
		"native HLS":                      {answers: []bool{true}, steps: []string{"video cors http://polyfin.test/videos/a/master.m3u8?x=1", "play", "show"}, want: []string{alone}},
		"not asked":                       {answers: []bool{false}, steps: []string{"video " + relayed, "play", "show"}, want: []string{alone}},
		"no answer":                       {steps: []string{"video " + relayed, "play", "show"}, want: []string{alone}},
		"once for each element":           {answers: []bool{true, true, true}, steps: []string{"video " + relayed, "play", "play", "play", "show", "count"}, want: []string{boosted, "1 sources, 1 contexts"}},
		"a context that waits for a play": {answers: []bool{true}, steps: []string{"suspended", "video " + relayed, "play", "show"}, want: []string{boosted}},
		"a context that never runs":       {answers: []bool{true}, steps: []string{"stuck", "video " + relayed, "play", "show"}, want: []string{alone}},
		"turned off on the server": {answers: []bool{true, true, false}, steps: []string{"video " + relayed, "play", "show", "play", "show"},
			want: []string{boosted, straight}},
		"the player's menu": {answers: []bool{true}, steps: []string{"video " + relayed, "play", "menu", "entry", "pick", "show", "menu", "entry"},
			want: []string{"Boost the volume: On", "post false", straight, "Boost the volume: Off"}},
		"turned on from the menu": {answers: []bool{false}, steps: []string{"video " + relayed, "play", "menu", "pick", "show"},
			want: []string{"post true", boosted}},
		"from the next video": {answers: []bool{false}, steps: []string{"video " + own, "play", "menu", "pick", "show"},
			want: []string{"post true", "toast The volume is boosted from the next video.", alone}},
		"another menu":    {answers: []bool{true}, steps: []string{"video " + relayed, "play", "item menu", "entry"}, want: []string{"no entry"}},
		"without a click": {answers: []bool{true}, steps: []string{"video " + relayed, "play", "unasked menu", "entry"}, want: []string{"no entry"}},
		"without a video": {answers: []bool{true}, steps: []string{"menu", "entry"}, want: []string{"no entry"}},
		"in French":       {answers: []bool{false}, lang: "fr-FR", steps: []string{"video " + relayed, "play", "menu", "entry"}, want: []string{"Augmenter le volume: Désactivé"}},
		"no Web Audio":    {answers: []bool{true}, steps: []string{"no audio", "video " + relayed, "play", "show"}, want: []string{alone}},
		"an element gone": {answers: []bool{true, true}, steps: []string{"video " + relayed, "play", "remove", "video " + relayed, "play", "show", "count"}, want: []string{boosted, "2 sources, 1 contexts"}},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  const observers = []
  class MutationObserver {
    constructor(callback) { this.callback = callback }
    observe(target) { observers.push({ target, observer: this }) }
    disconnect() {}
  }
  const classes = (element) => element.className.split(' ').filter(Boolean)
  const matches = (element, selector) => /^(\.[\w-]+)+$/.test(selector) && selector.slice(1).split('.').every((c) => classes(element).includes(c))
  const all = (root, selector) => root.children.flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
  class Element {
    constructor(tag, className) { this.tagName = tag.toUpperCase(); this.className = className || ''; this.children = []; this.parentNode = null; this.attributes = {}; this.listeners = {}; this.text = '' }
    get classList() {
      return {
        contains: (c) => classes(this).includes(c),
        add: (c) => { if (!classes(this).includes(c)) this.className = [...classes(this), c].join(' ') },
        remove: (c) => { this.className = classes(this).filter((x) => x !== c).join(' ') },
      }
    }
    set textContent(text) { this.text = text; this.children = [] }
    get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
    appendChild(child) {
      if (child.parentNode) child.parentNode.removeChild(child)
      this.children.push(child)
      child.parentNode = this
      for (const { target, observer } of observers) if (target === this) observer.callback([{ addedNodes: [child] }])
      return child
    }
    removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; return child }
    remove() { if (this.parentNode) this.parentNode.removeChild(this) }
    querySelector(selector) { return all(this, selector)[0] || null }
    querySelectorAll(selector) { return all(this, selector) }
    setAttribute(name, value) { this.attributes[name] = String(value) }
    getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
    addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener) }
    cloneNode() {
      const copy = new Element(this.tagName, this.className)
      copy.attributes = { ...this.attributes }
      copy.text = this.text
      for (const child of this.children) copy.appendChild(child.cloneNode())
      return copy
    }
    closest(selector) { for (let at = this; at; at = at.parentNode) if (at.tagName && matches(at, selector)) return at; return null }
  }
  const html = new Element('html')
  const body = html.appendChild(new Element('body'))
  const contains = (node) => { for (let at = node; at; at = at.parentNode) if (at === html) return true; return false }
  const document = {
    body,
    documentElement: { lang: scenario.lang || 'en-US', contains },
    head: { appendChild: () => {} },
    createElement: (tag) => new Element(tag),
    getElementById: () => null,
    getElementsByTagName: (tag) => tag === 'video' ? all(body, '.video') : [],
    querySelector: (selector) => body.querySelector(selector),
    querySelectorAll: (selector) => body.querySelectorAll(selector),
  }
  let href = 'http://polyfin.test/web/'
  const location = { protocol: 'http:', host: 'polyfin.test', hash: '', get href() { return href }, set href(url) {}, replace: () => {}, reload: () => {} }
  const history = { pushState: () => {}, replaceState: () => {}, back: () => { const open = body.querySelector('.dialogContainer'); if (open) open.remove() } }
  const answers = (scenario.answers || []).slice()
  const ApiClient = {
    serverId: () => 'server',
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => 'user',
    getCurrentUser: () => Promise.resolve({ Id: 'user', Policy: {} }),
    getJSON: (url) => {
      if (url !== 'http://polyfin.test/Polyfin/Sound') return Promise.reject(new Error('not asked here'))
      return answers.length ? Promise.resolve({ BoostVolume: answers.shift(), ConvertSound: false, FollowsGroup: false }) : Promise.reject(new Error('down'))
    },
    ajax: (request) => {
      if (request.type === 'POST' && request.url === 'http://polyfin.test/Polyfin/Sound') {
        const sent = JSON.parse(request.data)
        log.push('post ' + sent.BoostVolume)
        return Promise.resolve({ BoostVolume: sent.BoostVolume })
      }
      return Promise.reject(new Error('not asked here'))
    },
  }
  let contexts = 0
  let sources = 0
  let state = 'running'
  let resumes = true
  class Node {
    constructor(name) { this.name = name; this.outputs = [] }
    connect(other) { this.outputs = [other]; return other }
    disconnect() { this.outputs = [] }
  }
  class AudioContext {
    constructor() { contexts++; this.state = state; this.destination = new Node('speakers') }
    resume() { if (resumes) this.state = 'running'; return Promise.resolve() }
    createGain() { const n = new Node('gain'); n.gain = { value: 1 }; return n }
    createDynamicsCompressor() { const n = new Node('compressor'); for (const k of ['threshold', 'knee', 'ratio', 'attack', 'release']) n[k] = { value: 0 }; return n }
    createMediaElementSource(element) {
      if (element.source) throw new Error('InvalidStateError')
      sources++
      element.source = new Node('source')
      return element.source
    }
  }
  const window = { ApiClient }
  const listeners = {}
  const addEventListener = (type, listener) => { (listeners[type] ||= []).push(listener) }
  const fire = (type, event) => { for (const listener of listeners[type] || []) listener(event) }
  // The compressor's settings are the prototype's, the gain 2.
  const graph = (element) => {
    if (!element.source) return 'alone'
    const names = []
    for (let at = element.source; at; at = at.outputs[0]) {
      names.push(at.name)
      if (at.name === 'gain' && at.gain.value !== 2) names.push('(gain ' + at.gain.value + ')')
      if (at.name === 'compressor' && [at.threshold.value, at.knee.value, at.ratio.value, at.attack.value, at.release.value].join() !== '-6,6,12,0.003,0.25') names.push('(compressor)')
    }
    return names.join('>')
  }
  const steps = scenario.steps.slice()
  if (steps[0] === 'no audio') steps.shift()
  else window.AudioContext = AudioContext
  vm.runInNewContext(script, {
    location, history, document, MutationObserver, window, addEventListener,
    CustomEvent: class { constructor(type) { this.type = type } }, Event: class { constructor(type) { this.type = type } },
    setTimeout: (run, delay) => (delay < 1000 ? 0 : 0), clearTimeout: () => {},
    Date: { now: () => 100000 },
  })
  await flush()
  let video = null
  const sheet = (entries) => {
    const container = new Element('div', 'dialogContainer')
    const dialog = container.appendChild(new Element('div', 'dialog actionSheet'))
    dialog.setAttribute('data-history', 'true')
    const scroller = dialog.appendChild(new Element('div', 'actionSheetScroller'))
    for (const id of entries) {
      const item = scroller.appendChild(new Element('button', 'listItem actionSheetMenuItem'))
      item.setAttribute('data-id', id)
      const words = item.appendChild(new Element('div', 'listItemBody actionsheetListItemBody')).appendChild(new Element('div', 'listItemBodyText actionSheetItemText'))
      words.text = id
      item.appendChild(new Element('div', 'listItemAside actionSheetItemAsideText')).text = 'aside'
    }
    body.appendChild(container)
  }
  const settingsButton = body.appendChild(new Element('button', 'btnVideoOsdSettings'))
  for (const step of steps) {
    if (step === 'suspended') state = 'suspended'
    else if (step === 'stuck') { state = 'suspended'; resumes = false }
    else if (step.startsWith('video ')) {
      const words = step.split(' ')
      video = body.appendChild(new Element('video', 'video'))
      if (words.length === 3) video.setAttribute('crossorigin', 'anonymous')
      video.currentSrc = words[words.length - 1]
    } else if (step === 'remove') video.remove()
    else if (step === 'play') fire('play', { type: 'play', target: video })
    else if (step === 'show') log.push(graph(video))
    else if (step === 'count') log.push(sources + ' sources, ' + contexts + ' contexts')
    else if (step === 'menu') { fire('click', { type: 'click', target: settingsButton }); sheet(['aspectratio', 'playbackrate', 'quality', 'stats']) }
    else if (step === 'unasked menu') sheet(['aspectratio', 'playbackrate', 'quality', 'stats'])
    else if (step === 'item menu') { fire('click', { type: 'click', target: settingsButton }); sheet(['play', 'favorite', 'delete']) }
    else if (step === 'entry') {
      const entry = body.querySelector('.polyfinBoost')
      log.push(entry ? entry.querySelector('.actionSheetItemText').textContent + ': ' + entry.querySelector('.actionSheetItemAsideText').textContent : 'no entry')
    } else if (step === 'pick') {
      const entry = body.querySelector('.polyfinBoost')
      for (const listener of entry ? entry.listeners.click || [] : []) listener({ type: 'click', stopPropagation() {}, preventDefault() {} })
      const toast = body.querySelector('.toast')
      if (toast) { log.push('toast ' + toast.textContent); toast.remove() }
      if (body.querySelector('.dialogContainer')) log.push('menu still open')
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
		Answers []bool   `json:"answers"`
		Lang    string   `json:"lang,omitempty"`
		Steps   []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.answers, s.lang, s.steps}
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
