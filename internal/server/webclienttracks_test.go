package server

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// A title page's track menus follow the analyses of its versions (the
// second part): once Polyfin pushes that the tracks of the title's versions
// changed, the script checks, a second later, the page's menus against the
// title's details, and when the version picked lists other tracks there,
// reloads the page's details and tells jellyfin-web the version was picked,
// as picking it does: jellyfin-web then fills the track menus from those
// details. It keeps the version picked, and the tracks the user picked where
// the menus still list them, through jellyfin-web's renders. It never
// changes the page while a video plays or while a menu has focus, checks
// once for the pushes that come within the second, and checks the page
// shown again after a video when the push came during it. It runs in
// Node.js, in a context that stands for the browser: a title page with its
// version, video, audio and subtitle menus, which a small jellyfin-web fills
// as jellyfin-web 12.4 does (itemDetails/index.js, renderTrackSelections and
// the version menu's change), a clock and timers the harness moves on.
func TestTitlePagesRefreshTheirTrackMenusOnceAVersionIsAnalyzed(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	type scenario struct {
		// steps: "route" opens the title's page; "home" and "video" move the
		// router there; "back" shows the title's page again, as restored;
		// "analyze <version>" has Polyfin analyze a version, its details
		// listing its tracks from then on, "push" push that the tracks
		// changed, "push other" for another title; "playing" and "stopped"
		// start and end a video over the page; "focus <menu>" and "blur";
		// "pick <menu> <value>" picks as a user does; "tick" runs the next
		// timers due, logging "idle" when there is none; "menus" logs the menus:
		// the version picked, then the tracks listed by type, "*" before
		// those picked.
		picked string
		steps  []string
		want   []string
	}
	const (
		details = "details t"
		bare    = "menus t | video - | audio - | subtitles *-1 ext"
		tracked = "menus t | video *0 | audio *1 2 | subtitles *-1 3 ext"
	)
	// Each scenario opens the page first: the script asks for the versions
	// at once.
	opened := []string{"ask"}
	scenarios := map[string]scenario{
		"an analysis fills the menus": {
			steps: []string{"route", "menus", "analyze t", "push", "tick", "menus"},
			want:  slices.Concat(opened, []string{bare, details, "reload", "change t", "tracks t", tracked})},
		"menus that list the tracks stay": {
			steps: []string{"analyze t", "route", "push", "tick", "tick"},
			want:  slices.Concat(opened, []string{details, "idle"})},
		// Several versions analyzed together push within the second: the
		// script checks once.
		"pushes within a second check once": {
			steps: []string{"route", "analyze t", "push", "push", "push", "tick", "tick"},
			want:  slices.Concat(opened, []string{details, "reload", "change t", "tracks t", "idle"})},
		"a push for another title changes nothing": {
			steps: []string{"route", "analyze t", "push other", "tick", "tick", "menus"},
			want:  slices.Concat(opened, []string{"idle", bare})},
		// The analysis lands during the play: the push comes while the
		// router shows the video, and the page is checked once shown again.
		"a version analyzed during a play shows once back": {
			steps: []string{"route", "video", "analyze t", "push", "tick", "back", "tick", "menus"},
			want:  slices.Concat(opened, []string{"idle", "ask", details, "reload", "change t", "tracks t", tracked})},
		// A video plays over the page: it waits for the video to end, and
		// the next push.
		"never during a video": {
			steps: []string{"route", "playing", "analyze t", "push", "tick", "stopped", "push", "tick", "menus"},
			want:  slices.Concat(opened, []string{details, "reload", "change t", "tracks t", tracked})},
		"not while a menu has focus": {
			steps: []string{"route", "focus audio", "analyze t", "push", "tick", "menus", "blur", "tick", "menus"},
			want:  slices.Concat(opened, []string{bare, details, "reload", "change t", "tracks t", tracked})},
		// The version picked stays picked: jellyfin-web fills the track menus
		// for it, then again once it has that version's own details.
		"the version picked stays picked": {picked: "v1",
			steps: []string{"route", "analyze v1", "push", "tick", "menus"},
			want: slices.Concat(opened, []string{details, "reload", "change v1", "tracks v1", "details v1", "tracks v1",
				"menus v1 | video *0 | audio *1 2 | subtitles *-1 3 ext"})},
		// The subtitle file the user picked is still listed, under another
		// index: it stays picked, through both of jellyfin-web's renders.
		"the tracks the user picked stay picked": {picked: "v1",
			steps: []string{"route", "pick subtitles ext", "analyze v1", "push", "tick", "menus"},
			want: slices.Concat(opened, []string{details, "reload", "change v1", "tracks v1", "details v1", "tracks v1",
				"menus v1 | video *0 | audio *1 2 | subtitles -1 3 *ext"})},
		// Picks made before a play are not the page's after it: jellyfin-web
		// shows the page again with its default tracks, which stay.
		"picks before a play are forgotten": {
			steps: []string{"route", "pick subtitles ext", "video", "analyze t", "push", "back", "tick", "menus"},
			want:  slices.Concat(opened, []string{"ask", details, "reload", "change t", "tracks t", tracked})},
		"no check while the page is left": {
			steps: []string{"route", "analyze t", "push", "home", "tick", "menus"},
			want:  slices.Concat(opened, []string{"idle", bare})},
	}

	const harness = scriptRunner + `
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
const TITLE = '0123456789abcdef0123456789abcdef'
const short = (id) => (id === TITLE ? 't' : id)
async function run(scenario) {
  const log = []
  let now = 0
  const timers = []
  const listeners = {}
  const location = { href: 'http://polyfin.test/web/', hash: '', replace: () => {} }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  class CustomEvent { constructor(type, init) { this.type = type; this.detail = init && init.detail; this.bubbles = !!(init && init.bubbles) } }
  class Event { constructor(type, init) { this.type = type; this.bubbles = !!(init && init.bubbles) } stopPropagation() { this.stopped = true } }
  const observers = new Set()
  const queued = new Set()
  const changed = (element) => {
    for (const observer of observers) if (observer.targets.has(element)) queued.add(observer)
    queueMicrotask(() => {
      const ready = [...queued]
      queued.clear()
      for (const observer of ready) if (observers.has(observer)) observer.callback([])
    })
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback; this.targets = new Set() }
    observe(target) { this.targets.add(target); observers.add(this) }
    disconnect() { this.targets.clear(); observers.delete(this) }
  }
  // The tracks of the versions, by version, once analyzed: a video, two
  // audio tracks, the second the default, and an embedded subtitle; each
  // version's subtitle file is listed before, its index after them.
  const analyzed = new Set()
  const streams = (id) => analyzed.has(id)
    ? [{ Type: 'Video', Index: 0, DisplayTitle: '1080p H264' }, { Type: 'Audio', Index: 1, DisplayTitle: 'English AC3' },
      { Type: 'Audio', Index: 2, DisplayTitle: 'French AAC' }, { Type: 'Subtitle', Index: 3, DisplayTitle: 'English' },
      { Type: 'Subtitle', Index: 4, DisplayTitle: 'File' }]
    : [{ Type: 'Subtitle', Index: 0, DisplayTitle: 'File' }]
  const source = (id) => ({ Id: id, Name: 'Version ' + short(id), MediaStreams: streams(id), DefaultAudioStreamIndex: analyzed.has(id) ? 1 : undefined })
  const sources = () => [source(TITLE), source('v1')]
  function select(name) {
    const element = {
      name,
      children: [],
      get options() { return this.children },
      get selectedIndex() { return this.children.findIndex((option) => option.selected) },
      get value() { const option = this.children.find((option) => option.selected); return option ? option.value : '' },
      set value(value) { for (const option of this.children) option.selected = option.value === value },
      matches: (selector) => selector === '.' + name,
      closest: (selector) => (selector === '.itemDetailPage' ? page : null),
      dispatchEvent: (event) => dispatch(element, event),
      fill(list, picked) {
        this.children = list.map(([value, textContent]) => ({ value: String(value), textContent, selected: false }))
        this.value = String(picked)
        changed(this)
      },
    }
    return element
  }
  const menus = { source: select('selectSource'), video: select('selectVideo'), audio: select('selectAudio'), subtitles: select('selectSubtitles') }
  const selections = { classList: { contains: () => false } }
  // jellyfin-web: its copy of the versions, the item it shows, and the
  // track menus filled for the version picked from that copy.
  let copy = []
  let current = null
  function renderTracks() {
    const picked = copy.find((s) => s.Id === menus.source.value)
    const of = (type) => picked.MediaStreams.filter((s) => s.Type === type)
    menus.video.fill(of('Video').map((s) => [s.Index, s.DisplayTitle]), of('Video').length ? of('Video')[0].Index : -1)
    menus.audio.fill(of('Audio').map((s) => [s.Index, s.DisplayTitle]), picked.DefaultAudioStreamIndex)
    menus.subtitles.fill([[-1, 'Off'], ...of('Subtitle').map((s) => [s.Index, s.DisplayTitle])], -1)
    log.push('tracks ' + short(menus.source.value))
  }
  function renderTrackSelections(item, force) {
    copy = item.MediaSources
    const before = menus.source.value
    const id = copy.some((s) => s.Id === before) ? before : copy[0].Id
    menus.source.fill(copy.map((s) => [s.Id, s.Name]), id)
    if (menus.source.value !== before || force) renderTracks()
  }
  // A version picked: jellyfin-web fills the track menus from its copy,
  // then, for a version other than the item shown, asks for its details
  // and fills them again.
  function picked() {
    renderTracks()
    const id = menus.source.value
    if (id === current.Id) return
    log.push('details ' + short(id))
    Promise.resolve().then(() => {
      current = { Id: id, MediaSources: sources() }
      renderTrackSelections(current)
      renderTracks()
    })
  }
  function dispatch(target, event) {
    event.target = target
    for (const listener of listeners[event.type] || []) listener(event)
    if (event.type !== 'change' || event.stopped) return
    if (target === menus.source) {
      log.push('change ' + short(target.value))
      picked()
    }
  }
  let shown = false
  const page = {
    querySelector: (selector) => {
      const name = { '.selectSource': 'source', '.selectVideo': 'video', '.selectAudio': 'audio', '.selectSubtitles': 'subtitles' }[selector]
      if (name) return menus[name]
      return selector === '.trackSelections' ? selections : null
    },
    // The script's reload: viewshow, not restored, asks for the details,
    // and the page lists them. It does not bubble.
    dispatchEvent: (event) => {
      if (event.type !== 'viewshow') return
      log.push('reload')
      Promise.resolve().then(() => {
        current = { Id: TITLE, MediaSources: sources() }
        renderTrackSelections(current)
      })
    },
  }
  let video = false
  let focused = null
  const document = {
    get activeElement() { return focused },
    head: { appendChild: () => {} },
    body: null,
    createElement: () => ({}),
    getElementById: () => null,
    querySelector: (selector) => {
      if (selector === '.videoPlayerContainer') return video ? {} : null
      return selector === '.page.itemDetailPage:not(.hide)' && shown ? page : null
    },
  }
  // The page as jellyfin-web opens it: its details, the version picked.
  function open() {
    current = { Id: TITLE, MediaSources: sources() }
    copy = current.MediaSources
    menus.source.fill(copy.map((s) => [s.Id, s.Name]), scenario.picked || TITLE)
    renderTracks()
    log.pop()
    shown = true
  }
  const subscribers = {}
  let asked = false
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => 'user',
    // The versions are answered once each time the page opens: later
    // asks never answer.
    getJSON: (url) => {
      if (url.endsWith('/Polyfin/UserViews')) return Promise.resolve({ Items: [] })
      if (asked) return new Promise(() => {})
      asked = true
      log.push('ask')
      return Promise.resolve({ Pending: 0, Count: 2, Known: 2, AddVersions: true })
    },
    getItem: (user, id) => {
      log.push('details ' + short(id))
      return Promise.resolve({ Id: id, Name: 'Movie', MediaSources: sources() })
    },
    subscribe: (types, callback) => {
      for (const type of types) (subscribers[type] ||= []).push(callback)
      return () => {}
    },
  }
  const raise = (type) => { for (const listener of listeners[type] || []) listener(new CustomEvent(type, { bubbles: true })) }
  runScript(script, {
    location, history, URL, document, CustomEvent, Event, MutationObserver, window: { ApiClient },
    Date: { now: () => now },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: (id) => { if (id) timers[id - 1] = null },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  const route = '#/details?id=' + TITLE + '&serverId=s'
  for (const step of scenario.steps) {
    const [verb, ...args] = step.split(' ')
    if (verb === 'route') {
      asked = false
      open()
      history.pushState(null, '', route)
    } else if (verb === 'home' || verb === 'video') {
      if (verb === 'video') video = true
      raise('viewbeforehide')
      shown = verb === 'video'
      history.pushState(null, '', '#/' + verb)
    } else if (verb === 'back') {
      // jellyfin-web shows the page as it was, its tracks the defaults of
      // its own copy.
      video = false
      asked = false
      history.pushState(null, '', route)
      renderTrackSelections(current, true)
      log.pop()
      raise('viewshow')
    } else if (verb === 'playing') video = true
    else if (verb === 'stopped') video = false
    else if (verb === 'analyze') analyzed.add(args[0] === 't' ? TITLE : args[0])
    else if (verb === 'push') {
      const message = { MessageType: 'PolyfinVersions', MessageId: 'm',
        Data: { ItemId: args[0] === 'other' ? 'f'.repeat(32) : TITLE.toUpperCase(), Pending: 0, Count: 2, Known: 2, AddVersions: true, Tracks: true } }
      for (const callback of (subscribers.PolyfinVersions || []).slice()) callback(message)
    } else if (verb === 'focus') focused = menus[args[0]]
    else if (verb === 'blur') {
      const menu = focused
      focused = null
      dispatch(menu, new Event('blur'))
    } else if (verb === 'pick') {
      menus[args[0]].value = args[1] === 'ext' ? String(menus[args[0]].options.find((o) => o.textContent === 'File').value) : args[1]
      dispatch(menus[args[0]], new Event('change', { bubbles: true }))
    } else if (verb === 'menus') {
      const list = (menu) => menu.children.length
        ? menu.children.map((o) => (o.selected ? '*' : '') + (o.textContent === 'File' ? 'ext' : o.value)).join(' ')
        : '-'
      log.push('menus ' + short(menus.source.value) + ' | video ' + list(menus.video) + ' | audio ' + list(menus.audio) + ' | subtitles ' + list(menus.subtitles))
    } else if (verb === 'tick') {
      // The timers due first all run.
      const due = timers.filter(Boolean).reduce((at, timer) => Math.min(at, timer.at), Infinity)
      if (due === Infinity) log.push('idle')
      else {
        now = due
        timers.forEach((timer, i) => {
          if (!timer || timer.at !== due) return
          timers[i] = null
          timer.run()
        })
      }
    }
    await flush()
  }
  return log
}
;(async () => {
  const results = {}
  for (const [name, scenario] of Object.entries(scenarios)) results[name] = await run(scenario)
  process.stdout.write(JSON.stringify(results))
})()
`
	type input struct {
		Picked string   `json:"picked,omitempty"`
		Steps  []string `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.picked, s.steps}
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
