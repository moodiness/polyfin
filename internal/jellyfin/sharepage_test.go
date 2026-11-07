package jellyfin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// The page of a share link keeps where the guest stopped in their browser,
// under the link's ID: reopened, it offers to resume there or to start
// from the beginning, and a playback the server ended starts again there.
// It forgets a position near the end, one older than 31 days, and the
// position of a link expired or revoked. It runs in Node.js, in a context
// that stands for the browser, the server and hls.js.
func TestSharePageKeepsWhereTheGuestStopped(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	script, err := fs.ReadFile(shareFiles, "share/assets/share.js")
	if err != nil {
		t.Fatal(err)
	}
	type scenario struct {
		// Stored are the positions kept as the first page loads, by key:
		// a position in seconds and its age in days.
		Stored map[string][2]float64 `json:"stored"`
		// Runtime is the title's, as the page says it; Duration the
		// video's, once loaded; both 600 seconds when unset.
		Runtime  float64 `json:"runtime"`
		Duration float64 `json:"duration"`
		// Native plays HLS without hls.js, and has no Media Source
		// Extensions; Gone loads the page of a link expired or revoked.
		Native bool `json:"native"`
		Gone   bool `json:"gone"`
		// Steps: "load" and "reload" the page, "close" it; "click play",
		// "click resume", or "press video" for the video's own play
		// button; "playing" once the video plays, after it sought where
		// it started; "metadata" as the video's loads; "at N" as it
		// plays to N seconds; "pause"; "finish" as it reaches its end;
		// "tick" for the 10-second report; "server <code>" has the server
		// answer the next starts and reports with that refusal; "days N"
		// passes the time. "show" logs the buttons and the message,
		// "kept" the positions stored, "time" the video's position,
		// "loaded" how many times the page loaded hls.js.
		Steps []string `json:"steps"`
		// Want logs, besides, "start N" for each start asked of the
		// server, "hls N" for each hls.js start position (-1 for the
		// start), and "stop" for each stop told to the server.
		Want []string `json:"want"`
	}
	scenarios := map[string]scenario{
		"a reload offers to resume where the guest paused": {
			Steps: []string{"load", "show", "click play", "playing", "at 124", "pause", "reload", "show", "kept", "click resume", "playing", "time"},
			Want: []string{"play(Play)", "start 0", "hls -1", "stop", "resume(Resume at 0:02:04) play(From the beginning)",
				"kept polyfin.share.L1=124", "start 124", "hls 124", "time 124"}},
		"a position past an hour shows its hours": {Runtime: 7200, Duration: 7200, Stored: map[string][2]float64{"polyfin.share.L1": {3723.6, 1}},
			Steps: []string{"load", "show"},
			Want:  []string{"resume(Resume at 1:02:03) play(From the beginning)"}},
		"from the beginning starts again at the start": {Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}},
			Steps: []string{"load", "click play", "playing", "at 30", "pause", "kept"},
			Want:  []string{"start 0", "hls -1", "kept polyfin.share.L1=30"}},
		"after the server ended the playback, Play resumes where the guest was": {
			Steps: []string{"load", "click play", "playing", "at 124", "pause", "server ended", "tick", "show", "kept", "server ok", "click play", "playing", "time"},
			Want: []string{"start 0", "hls -1", "play(Play) message(ended-text)", "kept polyfin.share.L1=124", "start 124", "hls 124",
				"time 124"}},
		"after the server ended the playback, the video's own play button resumes too": {
			Steps: []string{"load", "click play", "playing", "at 124", "server ended", "tick", "server ok", "press video"},
			Want:  []string{"start 0", "hls -1", "start 124", "hls 124"}},
		"the last 2 minutes are forgotten": {
			Steps: []string{"load", "click play", "playing", "at 470", "pause", "kept", "at 490", "pause", "kept", "reload", "show"},
			Want:  []string{"start 0", "hls -1", "kept polyfin.share.L1=470", "kept none", "stop", "play(Play)"}},
		"the last 5 percent of a long title are forgotten": {Runtime: 7200, Duration: 7200,
			Steps: []string{"load", "click play", "playing", "at 6830", "pause", "kept", "at 6850", "pause", "kept"},
			Want:  []string{"start 0", "hls -1", "kept polyfin.share.L1=6830", "kept none"}},
		"a position kept near the end is not offered": {Stored: map[string][2]float64{"polyfin.share.L1": {590, 0}},
			Steps: []string{"load", "show", "kept"},
			Want:  []string{"play(Play)", "kept none"}},
		"the end of the title forgets": {
			Steps: []string{"load", "click play", "playing", "at 300", "pause", "kept", "finish", "kept", "show", "click play"},
			Want:  []string{"start 0", "hls -1", "kept polyfin.share.L1=300", "stop", "kept none", "play(Play)", "start 0", "hls -1"}},
		"positions older than 31 days are forgotten": {
			Stored: map[string][2]float64{"polyfin.share.L1": {124, 32}, "polyfin.share.L2": {50, 30}, "other": {1, 90}},
			Steps:  []string{"load", "show", "kept", "days 2", "reload", "kept"},
			Want:   []string{"play(Play)", "kept other polyfin.share.L2=50", "kept other"}},
		"the page of a link expired or revoked forgets it": {Gone: true,
			Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}, "polyfin.share.L2": {50, 0}},
			Steps:  []string{"load", "kept"},
			Want:   []string{"kept polyfin.share.L2=50"}},
		"a link revoked as the guest resumes forgets it": {Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}},
			Steps: []string{"load", "server revoked", "click resume", "show", "kept"},
			Want:  []string{"start 124", "play(Play) message(revoked-text)", "kept none"}},
		"a link that expires while the guest watches forgets it": {
			Steps: []string{"load", "click play", "playing", "at 200", "pause", "kept", "server expired", "tick", "show", "kept"},
			Want:  []string{"start 0", "hls -1", "kept polyfin.share.L1=200", "play(Play) message(expired-text)", "kept none"}},
		"a refusal that may pass keeps the position": {Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}},
			Steps: []string{"load", "server viewers", "click resume", "kept", "server ok", "click resume"},
			Want:  []string{"start 124", "kept polyfin.share.L1=124", "start 124", "hls 124"}},
		"closing the page keeps where it was": {
			Steps: []string{"load", "click play", "playing", "at 200", "close", "kept"},
			Want:  []string{"start 0", "hls -1", "stop", "kept polyfin.share.L1=200"}},
		"a playback closed before it played keeps the position kept": {Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}},
			Steps: []string{"load", "click resume", "pause", "close", "kept"},
			Want:  []string{"start 124", "hls 124", "stop", "kept polyfin.share.L1=124"}},
		"native HLS starts at the position kept": {Native: true, Stored: map[string][2]float64{"polyfin.share.L1": {124, 0}},
			Steps: []string{"load", "click resume", "metadata", "playing", "time", "at 130", "pause", "kept"},
			Want:  []string{"start 124", "time 124", "kept polyfin.share.L1=130"}},
		"hls.js loads at the first press of Play, once": {
			Steps: []string{"load", "loaded", "click play", "loaded", "playing", "at 124", "pause", "server ended", "tick", "server ok",
				"click play", "loaded"},
			Want: []string{"hls.js 0", "start 0", "hls -1", "hls.js 1", "start 124", "hls 124", "hls.js 1"}},
		"a browser that plays HLS itself never loads hls.js": {Native: true,
			Steps: []string{"load", "click play", "metadata", "playing", "loaded"},
			Want:  []string{"start 0", "hls.js 0"}},
		"the page of a link gone never loads hls.js": {Gone: true,
			Steps: []string{"load", "loaded"},
			Want:  []string{"hls.js 0"}},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = async () => { for (let i = 0; i < 10; i++) await new Promise((resolve) => setImmediate(resolve)) }
const day = 24 * 3600 * 1000
const messages = { ended: 'ended-text', expired: 'expired-text', revoked: 'revoked-text', viewers: 'viewers-text', error: 'error-text' }
class Element {
  constructor(id, attributes) { this.id = id; this.attributes = attributes || {}; this.hidden = false; this.disabled = false; this.textContent = ''; this.listeners = {}; this.classes = new Set() }
  getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
  hasAttribute(name) { return name in this.attributes }
  setAttribute(name, value) { this.attributes[name] = String(value) }
  removeAttribute(name) { delete this.attributes[name] }
  addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener) }
  removeEventListener(type, listener) { this.listeners[type] = (this.listeners[type] || []).filter((other) => other !== listener) }
  dispatch(type) { for (const listener of [...(this.listeners[type] || [])]) listener.call(this, { type }) }
  get classList() { return { add: (c) => this.classes.add(c), remove: (c) => this.classes.delete(c), contains: (c) => this.classes.has(c) } }
}
async function run(scenario) {
  const log = []
  let now = Date.UTC(2026, 10, 5)
  const stored = new Map()
  for (const [key, [position, age]] of Object.entries(scenario.stored || {})) stored.set(key, JSON.stringify({ position, savedAt: now - age * day }))
  const localStorage = {
    get length() { return stored.size },
    key: (i) => [...stored.keys()][i] ?? null,
    getItem: (key) => stored.has(key) ? stored.get(key) : null,
    setItem: (key, value) => { stored.set(key, String(value)) },
    removeItem: (key) => { stored.delete(key) },
  }
  const runtime = scenario.runtime || 600
  const duration = scenario.duration || 600
  let answer = 'ok'
  let page = null
  function load() {
    const window = {}
    const listeners = {}
    const main = new Element('', Object.assign({ 'data-link': 'L1' }, scenario.gone ? { 'data-gone': '' } : { 'data-runtime': String(runtime), 'data-hls': '/share/assets/hls.min.js?v=0' }))
    const elements = { 'share-messages': Object.assign(new Element('share-messages'), { textContent: JSON.stringify(messages) }) }
    let video = null
    let hls = null
    let interval = null
    let loads = 0
    if (!scenario.gone) {
      video = new Element('video')
      Object.assign(video, { currentTime: 0, duration: NaN, paused: true, seeking: false, src: '' })
      video.play = () => { video.paused = false; video.dispatch('play'); return Promise.resolve() }
      video.load = () => { video.currentTime = 0; video.duration = NaN; video.paused = true }
      video.canPlayType = (type) => (scenario.native && type === 'application/vnd.apple.mpegurl' ? 'maybe' : '')
      video.removeAttribute = (name) => { if (name === 'src') video.src = '' }
      elements.video = video
      elements.play = new Element('play', { 'data-again': 'From the beginning' })
      elements.play.textContent = 'Play'
      elements.resume = new Element('resume', { 'data-label': 'Resume at {time}' })
      elements.resume.hidden = true
      elements.message = new Element('message')
      elements.message.hidden = true
      elements.poster = new Element('poster')
    }
    class Hls {
      static isSupported() { return !scenario.native }
      constructor(config) { this.start = config.startPosition; log.push('hls ' + config.startPosition); hls = this }
      on() {}
      loadSource() {}
      attachMedia() { video.duration = duration; video.currentTime = 0 }
      destroy() { if (hls === this) hls = null }
    }
    Hls.Events = { ERROR: 'hlsError' }
    const respond = (status, body) => Promise.resolve({ ok: status < 300, status, json: () => Promise.resolve(body) })
    Object.assign(window, {
      window,
      localStorage,
      MediaSource: scenario.native ? undefined : class {},
      location: { pathname: '/share/TOKEN/' },
      navigator: { sendBeacon: (url) => { if (url.endsWith('/stop')) log.push('stop'); return true } },
      document: {
        documentElement: { lang: 'en' },
        getElementById: (id) => elements[id] || null,
        querySelector: (selector) => (selector === 'main.share' ? main : selector === 'img.poster' ? elements.poster || null : null),
        createElement: (tag) => new Element(tag),
        // hls.js defines Hls once loaded.
        head: { appendChild: (element) => { loads++; setImmediate(() => { window.Hls = Hls; element.onload() }) } },
      },
      Image: class { addEventListener() {} },
      Date: class extends Date { static now() { return now } },
      setInterval: (fn) => { interval = fn; return 1 },
      clearInterval: () => { interval = null },
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
      fetch: (url, options) => {
        if (url === '/share/TOKEN/play') {
          log.push('start ' + (options.body ? JSON.parse(options.body).position : 0))
          return answer === 'ok' ? respond(200, { session: 's1' }) : respond(answer === 'viewers' ? 429 : 410, { error: answer })
        }
        if (url.endsWith('/progress')) return answer === 'ok' ? respond(204, {}) : respond(answer === 'ended' ? 404 : 410, { error: answer })
        if (url.endsWith('/stop')) log.push('stop')
        return respond(204, {})
      },
      JSON, Math, Number, Object, Promise, isFinite, console,
    })
    vm.createContext(window)
    vm.runInContext(script, window)
    return {
      video, elements, stored,
      close: () => { for (const listener of listeners.pagehide || []) listener({ type: 'pagehide' }) },
      tick: () => interval && interval(),
      hls: () => hls,
      loads: () => loads,
    }
  }
  for (const step of scenario.steps) {
    const [verb, ...rest] = step.split(' ')
    const arg = rest.join(' ')
    const video = page && page.video
    switch (verb) {
      case 'load': page = load(); break
      case 'reload': page.close(); page = load(); break
      case 'close': page.close(); break
      case 'click': page.elements[arg].dispatch('click'); break
      case 'press': video.play(); break
      case 'playing':
        if (page.hls() && page.hls().start > 0 && !video.currentTime) video.currentTime = page.hls().start
        video.paused = false
        video.dispatch('playing')
        break
      case 'metadata': video.duration = duration; video.dispatch('loadedmetadata'); break
      case 'at': video.currentTime = Number(arg); break
      case 'pause': video.paused = true; video.dispatch('pause'); break
      case 'finish': video.currentTime = video.duration; video.paused = true; video.dispatch('pause'); video.dispatch('ended'); break
      case 'tick': page.tick(); break
      case 'server': answer = arg; break
      case 'days': now += Number(arg) * day; break
      case 'show': {
        const { play, resume, message } = page.elements
        const parts = []
        if (!resume.hidden) parts.push('resume(' + resume.textContent + ')')
        if (!play.hidden) parts.push('play(' + play.textContent + ')')
        if (!message.hidden) parts.push('message(' + message.textContent + ')')
        log.push(parts.join(' '))
        break
      }
      case 'kept': {
        const keys = [...stored.keys()].sort()
        log.push('kept ' + (keys.length ? keys.map((key) => key.startsWith('polyfin.share.') ? key + '=' + JSON.parse(stored.get(key)).position : key).join(' ') : 'none'))
        break
      }
      case 'time': log.push('time ' + video.currentTime); break
      case 'loaded': log.push('hls.js ' + page.loads()); break
      default: throw new Error('unknown step ' + step)
    }
    await flush()
  }
  return log
}
;(async () => {
  const results = {}
  for (const [name, scenario] of Object.entries(scenarios)) {
    try { results[name] = await run(scenario) } catch (error) { results[name] = ['error: ' + (error.stack || error)] }
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	input, err := json.Marshal(map[string]any{"script": string(script), "scenarios": scenarios})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string][]string
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	for _, name := range slices.Sorted(maps.Keys(scenarios)) {
		if got, want := results[name], scenarios[name].Want; !slices.Equal(got, want) {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
	}
}

// renderedSharePage is the page of a link that opens its title.
func renderedSharePage(t *testing.T) string {
	t.Helper()
	text := shareTexts["en"]
	var page strings.Builder
	if err := sharePage.Execute(&page, sharePageData{Language: "en", Text: text, Server: "Polyfin", Link: "L1", Messages: text.messages(),
		Assets: shareLinks, Title: &shareTitleData{Name: "A movie", Minutes: 100, Seconds: 6000, Expires: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	return page.String()
}

// Every file the page and its style link to is one of /share/assets/:
// the logo and the style, and the font and hls.js. The page links its
// own files and hls.js with their version, the hash package server tags
// them with, which lets the browser keep them for good.
func TestSharePageLinksItsAssets(t *testing.T) {
	t.Parallel()
	page := renderedSharePage(t)
	style, err := fs.ReadFile(ShareAssets, "share.css")
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for _, match := range regexp.MustCompile(`/share/assets/([\w.-]+)(?:\?v=(\w+))?`).FindAllStringSubmatch(page, -1) {
		versions[match[1]] = match[2]
	}
	for _, match := range regexp.MustCompile(`url\('([\w.-]+)'\)`).FindAllStringSubmatch(string(style), -1) {
		versions[match[1]] = ""
	}
	for _, want := range []string{"share.css", "share.js", "hls.min.js", "polyfin.svg",
		"figtree-latin-wght-normal.woff2", "figtree-latin-ext-wght-normal.woff2"} {
		if _, ok := versions[want]; !ok {
			t.Errorf("%s is not linked: %q", want, slices.Sorted(maps.Keys(versions)))
		}
	}
	for name, version := range versions {
		data, err := fs.ReadFile(ShareAssets, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		sum := sha256.Sum256(data)
		versioned := !strings.HasPrefix(name, "figtree")
		if want := hex.EncodeToString(sum[:8]); versioned && version != want {
			t.Errorf("%s: linked with version %q, want %q", name, version, want)
		}
	}
}

// Before Play, the page loads its own script only, within a budget:
// hls.js loads once Play is pressed, and only where it plays.
func TestSharePageScriptsBeforePlayStayWithinBudget(t *testing.T) {
	t.Parallel()
	const budget = 16 << 10
	page := renderedSharePage(t)
	total := 0
	for _, match := range regexp.MustCompile(`<script src="/share/assets/([\w.-]+)`).FindAllStringSubmatch(page, -1) {
		data, err := fs.ReadFile(ShareAssets, match[1])
		if err != nil {
			t.Fatal(err)
		}
		total += len(data)
	}
	if total == 0 || total > budget {
		t.Errorf("the page loads %d bytes of script before Play, want 1 to %d", total, budget)
	}
}
