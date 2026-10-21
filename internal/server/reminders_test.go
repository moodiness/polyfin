package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// A programme's page gets Polyfin's reminders after its record buttons,
// once jellyfin-web drew them: "Remind me", with a menu offering "Remind
// me of every airing", and "Reminder set" for a programme with one, which
// removes it, or the series reminder that made it, told under the button.
// A line links to My account › Notifications when no target receives
// reminders. A programme started, an item that is not a programme, or a
// route that leaves the page shows nothing; the words follow the page's
// language. It runs in Node.js, in a context that stands for the browser
// and jellyfin-web.
func TestProgrammePagesGetReminders(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const (
		id    = "0123456789abcdef0123456789abcdef"
		title = "#/details?id=" + id + "&serverId=fedcba9876543210fedcba9876543210"
		ask   = "ask http://polyfin.test/Polyfin/Reminders/Programs/" + id
	)
	coming := map[string]any{"ProgramId": id, "Started": false, "ReminderId": "", "SeriesReminderId": "", "Reaches": true,
		"NotificationsUrl": "/admin/me/account#notifications"}
	with := func(changes map[string]any) map[string]any {
		answer := map[string]any{}
		for k, v := range coming {
			answer[k] = v
		}
		for k, v := range changes {
			answer[k] = v
		}
		return answer
	}
	set := with(map[string]any{"ReminderId": "r1"})
	series := with(map[string]any{"ReminderId": "r1", "SeriesReminderId": "s1", "SeriesStartDate": "2026-10-14T18:30:00.0000000Z"})
	type scenario struct {
		answer  map[string]any
		changes []map[string]any
		lang    string
		steps   []string
		want    []string
	}
	scenarios := map[string]scenario{
		"a programme to come": {answer: coming, steps: []string{title, "show"}, want: []string{ask, "[Remind me] [more]"}},
		"setting a reminder": {
			answer: coming, changes: []map[string]any{set}, steps: []string{title, "click main", "show", "wait 4000", "show"},
			want: []string{ask, `POST http://polyfin.test/Polyfin/Reminders {"ProgramId":"` + id + `","Series":false,"Language":"en"}`,
				"[Reminder set] [more] toast(Reminder set: you will be told before it starts.)", "[Reminder set] [more]"},
		},
		"every airing": {
			answer: coming, changes: []map[string]any{series}, steps: []string{title, "click more", "show", "click every", "show"},
			want: []string{ask, "[Remind me] [more] menu(Remind me of every airing)",
				`POST http://polyfin.test/Polyfin/Reminders {"ProgramId":"` + id + `","Series":true,"Language":"en"}`,
				"[Reminder set] note(Reminds you of every airing at about 06:30 PM.) toast(Reminder set: you will be told before it starts.)"},
		},
		"removing a reminder": {
			answer: set, changes: []map[string]any{nil}, steps: []string{title, "show", "click main", "show"},
			want: []string{ask, "[Reminder set] [more]", "DELETE http://polyfin.test/Polyfin/Reminders/r1", "[Remind me] [more] toast(Reminder removed.)"},
		},
		"removing a series reminder": {
			answer: series, changes: []map[string]any{nil}, steps: []string{title, "click main", "show"},
			want: []string{ask, "DELETE http://polyfin.test/Polyfin/Reminders/s1", "[Remind me] [more] toast(Reminder removed.)"},
		},
		"no target": {
			answer: with(map[string]any{"Reaches": false}), steps: []string{title, "show"},
			want: []string{ask, "[Remind me] [more] note(No target of yours receives reminders yet: choose Programme starting for one in " +
				"My account › Notifications <http://polyfin.test/admin/me/account#notifications>.)"},
		},
		"started":          {answer: with(map[string]any{"Started": true}), steps: []string{title, "show"}, want: []string{ask, "none"}},
		"not a programme":  {steps: []string{title, "show"}, want: []string{ask, "none"}},
		"before the page":  {answer: coming, steps: []string{"blank", title, "show", "draw", "show"}, want: []string{"none", ask, "[Remind me] [more]"}},
		"rendered anew":    {answer: coming, steps: []string{title, "show", "draw", "show"}, want: []string{ask, "[Remind me] [more]", "[Remind me] [more]"}},
		"leaving the page": {answer: coming, steps: []string{title, "show", "#/home", "show"}, want: []string{ask, "[Remind me] [more]", "none"}},
		"during a video":   {answer: coming, steps: []string{"video", title, "show", "end video", "show"}, want: []string{"none", ask, "[Remind me] [more]"}},
		"in French": {
			answer: series, lang: "fr-FR", steps: []string{title, "show"},
			want: []string{ask, "[Rappel prévu] note(Vous rappelle chaque diffusion vers 18:30.)"},
		},
	}
	const harness = `
const vm = require('node:vm')
const { script, scenarios } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const flush = () => new Promise((resolve) => setImmediate(resolve))
async function run(scenario) {
  const log = []
  let now = 0
  const timers = []
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
  const all = (root, selector) => (root.children || []).flatMap((child) => (child.children ? [...(matches(child, selector) ? [child] : []), ...all(child, selector)] : []))
  class Element {
    constructor(tag) { this.tagName = tag; this.className = ''; this.children = []; this.parentNode = null; this.title = ''; this.text = ''; this.attributes = {}; this.handlers = {}; this.disabled = false }
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
    focus() {}
    closest(selector) { for (let at = this; at && at.children; at = at.parentNode) if (matches(at, selector)) return at; return null }
  }
  const body = new Element('body')
  const page = body.appendChild(new Element('div'))
  page.className = 'page itemDetailPage'
  // jellyfin-web draws the page anew: its recording fields, then the
  // section of its overview.
  const draw = () => {
    for (const child of [...page.children]) page.removeChild(child)
    for (const name of ['recordingFields', 'detailSectionContent']) page.appendChild(new Element('div')).className = name
  }
  const sheets = []
  const document = {
    body,
    documentElement: { lang: scenario.lang || 'en-US' },
    head: { appendChild: (sheet) => sheets.push(sheet) },
    createElement: (tag) => new Element(tag),
    createTextNode: (text) => ({ textContent: text, parentNode: null }),
    getElementById: (id) => sheets.find((sheet) => sheet.id === id) || null,
    querySelector: (selector) => {
      if (selector === '.page.itemDetailPage:not(.hide)') return body.querySelector('.page.itemDetailPage')
      return body.querySelector(selector)
    },
    querySelectorAll: (selector) => body.querySelectorAll(selector),
  }
  // The buttons after the recording fields, their menu when open, the
  // lines under them, and the toasts shown.
  function show() {
    const block = page.querySelector('.polyfinReminders')
    const words = []
    if (block) {
      const fields = page.querySelector('.recordingFields')
      if (block.previousSibling !== fields) words.push('misplaced')
      const row = block.querySelector('.polyfinReminderRow')
      const main = row.querySelector('.polyfinRemind')
      words.push('[' + main.textContent + ']' + (main.disabled ? ' (busy)' : ''))
      if (row.querySelector('.polyfinReminderMore')) words.push('[more]')
      const menu = row.querySelector('.polyfinReminderMenu')
      if (menu && !menu.classList.contains('hide')) words.push('menu(' + menu.textContent + ')')
      for (const note of block.querySelectorAll('.polyfinReminderNote')) {
        words.push('note(' + note.children.map((child) => child.href ? child.textContent + ' <' + child.href + '>' : child.textContent).join('').replace(/^/, note.text) + ')')
      }
    } else words.push('none')
    for (const toast of body.querySelectorAll('.toast')) if (!toast.classList.contains('toastHide')) words.push('toast(' + toast.textContent + ')')
    log.push(words.join(' '))
  }
  const location = { href: 'http://polyfin.test/web/', hash: '' }
  const go = (hash) => { location.hash = hash; location.href = 'http://polyfin.test/web/' + hash }
  const history = { pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  const changes = (scenario.changes || []).slice()
  const ApiClient = {
    getUrl: (path) => 'http://polyfin.test/' + path,
    getCurrentUserId: () => 'user',
    getCurrentUser: () => Promise.resolve({ Id: 'user', Policy: { IsAdministrator: true } }),
    getJSON: (url) => {
      // The versions, scores and views are other parts'.
      if (!url.includes('/Polyfin/Reminders/')) return Promise.reject(new Error('not asked here'))
      log.push('ask ' + url)
      // An empty 204, as for an item that is no programme, fails to read.
      return scenario.answer ? Promise.resolve({ ...scenario.answer }) : Promise.reject(new SyntaxError('Unexpected end of JSON input'))
    },
    ajax: (request) => {
      log.push(request.type + ' ' + request.url + (request.data ? ' ' + request.data : ''))
      const answer = changes.shift()
      return Promise.resolve(answer ? { ...answer } : undefined)
    },
  }
  vm.runInNewContext(script, {
    location, history, URL, document, MutationObserver, window: { ApiClient }, Object, JSON, String, Date,
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init && init.detail } },
    setTimeout: (run, delay) => timers.push({ run, at: now + delay }),
    clearTimeout: (id) => { if (id) timers[id - 1] = null },
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
  })
  await flush()
  let video = null
  const click = (element) => {
    for (const listener of listeners.click || []) listener({ target: element })
    for (const handler of element.handlers.click || []) handler({ target: element })
  }
  for (const step of scenario.steps) {
    if (step === 'show') show()
    else if (step === 'blank') { for (const child of [...page.children]) page.removeChild(child) }
    else if (step === 'draw') draw()
    else if (step === 'video') video = body.appendChild(new Element('div')), video.className = 'videoPlayerContainer'
    else if (step === 'end video') body.removeChild(video)
    else if (step === 'click main') click(page.querySelector('.polyfinRemind'))
    else if (step === 'click more') click(page.querySelector('.polyfinReminderMore'))
    else if (step === 'click every') click(page.querySelector('.polyfinReminderEvery'))
    else if (step.startsWith('wait ')) {
      const until = now + Number(step.slice(5))
      for (const timer of timers.slice().sort((a, b) => a.at - b.at)) if (timer.at <= until) { now = timer.at; timer.run(); await flush() }
      timers.splice(0, timers.length, ...timers.filter((timer) => timer.at > until))
      now = until
    } else {
      if (step.startsWith('#/details') && page.children.length === 0 && !scenario.steps.includes('blank')) draw()
      history.pushState(null, '', step)
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
		Answer  map[string]any   `json:"answer,omitempty"`
		Changes []map[string]any `json:"changes"`
		Lang    string           `json:"lang,omitempty"`
		Steps   []string         `json:"steps"`
	}
	inputs := map[string]input{}
	for name, s := range scenarios {
		inputs[name] = input{s.answer, s.changes, s.lang, s.steps}
	}
	data, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": inputs})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(data))
	// The series' time of day is told in the browser's time zone.
	command.Env = append(os.Environ(), "TZ=UTC")
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
