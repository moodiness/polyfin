package server

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// A movie's or an episode's page gets "Watch together" in its ⋮ menu, after
// Polyfin's other entries, for a user whose SyncPlay access is Create and
// join, while jellyfin-web shows its SyncPlay button; never on a card's
// menu, a series' page, for a user with another access, or without the
// button. On a short screen, the menu moves up as far as its whole height
// needs, though its opening animation scales it down when the entries
// come. Pressing it closes the menu, makes sure the title has a version,
// then, with Jellyfin's SyncPlay routes only, creates a group (or reuses
// the one the socket said the session is in, if it still exists), waits
// for jellyfin-web to ping a new group (10 seconds at most), sets the
// title as its queue from the start, checks the group took it and pauses
// it, and tells how the others join. A title without a version, a queue
// the group refused and SyncPlay refused each get their own toast. It runs
// in Node.js, in a context that stands for the browser and jellyfin-web.
func TestTitleMenusOfferWatchTogether(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const page = "0123456789abcdef0123456789abcdef"
	const queue = `POST SyncPlay/SetNewQueue {"PlayingQueue":["` + page + `"],"PlayingItemPosition":0,"StartPositionTicks":0}`
	const versions = "GET Polyfin/Items/" + page + "/Versions"
	// jellyfin-web pings a group it joined once its clock is set, a moment
	// after: the queue waits for it.
	const made = `POST SyncPlay/New {"GroupName":"Morgan’s group"}, POST SyncPlay/Ping`
	const created = versions + ", " + made + ", " + queue + ", GET SyncPlay/g1, POST SyncPlay/Pause"
	const started = "toast Your SyncPlay group is ready, paused. The others join it on their own device, from the SyncPlay button" +
		" (the people icon at the top). Press Play once they are in. (10s)"
	const entries = "Hide this title | Add to Watchlist | Watch together => back, "
	scenarios := map[string]string{
		// When it shows.
		"a movie's page":                               entries + created + ", " + started,
		"an episode's page":                            entries + created + ", " + started,
		"a movie's page in a legacy layout":            entries + created + ", " + started,
		"a movie's page in a legacy layout, no button": "Hide this title | Add to Watchlist",
		"a movie's page on a short screen":             "Hide this title | Add to Watchlist | Watch together | top 340px => back, " + created + ", " + started,
		"a series' page":                               "Hide this title | Add to Watchlist",
		"a movie's card":                               "Hide this title | Add to Watchlist",
		"a movie's page, join only":                    "Hide this title | Add to Watchlist",
		"a movie's page, SyncPlay off":                 "Hide this title | Add to Watchlist",
		"a movie's page, no button":                    "Hide this title | Add to Watchlist",
		"a movie's page, the user unknown":             "Hide this title | Add to Watchlist",
		"a movie's page, the user failing to load":     "Hide this title | Add to Watchlist",
		"a movie's page, in French": "Masquer ce titre | Ajouter à «\u202fÀ regarder\u202f» | Regarder ensemble => back, " +
			versions + `, POST SyncPlay/New {"GroupName":"Groupe de Morgan"}, POST SyncPlay/Ping, ` + queue + ", GET SyncPlay/g1, POST SyncPlay/Pause, " +
			"toast Votre groupe SyncPlay est prêt, en pause. Les autres le rejoignent sur leur propre appareil, depuis le bouton SyncPlay" +
			" (l’icône de personnes en haut). Appuyez sur Lire une fois qu’ils sont là. (10s)",
		// The group.
		"a movie's page, in a group": entries + versions + ", GET SyncPlay/g0, " + queue + ", GET SyncPlay/g0, POST SyncPlay/Pause, " + started,
		"a movie's page, in a group that ended": entries + versions + ", GET SyncPlay/g0 (404), " + made + ", " +
			queue + ", GET SyncPlay/g1, POST SyncPlay/Pause, " + started,
		"a movie's page, in a group then left":       entries + created + ", " + started,
		"a movie's page, in another user's group":    entries + created + ", " + started,
		"a movie's page, in a group then not in one": entries + created + ", " + started,
		"a movie's page, in a group that went":       entries + created + ", " + started,
		"a movie's page, in a group, others joining": entries + versions + ", GET SyncPlay/g0, " + queue + ", GET SyncPlay/g0, POST SyncPlay/Pause, " + started,
		// jellyfin-web may ping before Polyfin's answer comes, or never.
		"a movie's page, pinging before the answer": entries + created + ", " + started,
		"a movie's page, never pinging": entries + versions + `, POST SyncPlay/New {"GroupName":"Morgan’s group"}, 10 seconds, ` + queue +
			", GET SyncPlay/g1, POST SyncPlay/Pause, " + started,
		// What goes wrong.
		"a movie's page without a source": entries + versions + ", toast No source is available for this title.",
		"a movie's page still searching": entries + versions +
			", toast Sources for this title are still being looked for. Try again in a moment.",
		"a movie's page the group refuses": entries + versions + ", " + made + ", " + queue +
			", GET SyncPlay/g1, toast Not everyone in your SyncPlay group can play this title.",
		"a movie's page, SyncPlay turned off since": entries + versions + `, POST SyncPlay/New {"GroupName":"Morgan’s group"} (403), ` +
			"GET Users/u1, toast SyncPlay is off for your account.",
		"a movie's page, join only since": entries + versions + `, POST SyncPlay/New {"GroupName":"Morgan’s group"} (403), ` +
			"GET Users/u1, toast Your account can join SyncPlay groups, but not start one.",
		"a movie's page, refused otherwise": entries + versions + `, POST SyncPlay/New {"GroupName":"Morgan’s group"} (403), ` +
			"GET Users/u1, toast Watch together could not start. Try again.",
		"a movie's page, a failure": entries + versions + `, POST SyncPlay/New {"GroupName":"Morgan’s group"} (500), ` +
			"toast Watch together could not start. Try again.",
	}
	const harness = scriptRunner + `
const { script, scenarios, page } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const settle = () => new Promise((resolve) => setImmediate(resolve))
;(async () => {
  const results = {}
  for (const name of scenarios) {
    const did = []
    let recording = false
    const record = (line) => { if (recording) did.push(line) }
    const listeners = {}
    const observers = []
    class MutationObserver {
      constructor(callback) { this.callback = callback }
      observe(target, options) { if (!options.subtree) observers.push(this) }
      disconnect() {}
    }
    const classes = (initial) => {
      const set = new Set(initial)
      return { add: (...c) => c.forEach((x) => set.add(x)), remove: (...c) => c.forEach((x) => set.delete(x)), contains: (c) => set.has(c) }
    }
    const onCard = name.includes('card')
    const type = name.includes('episode') ? 'Episode' : name.includes('series') ? 'Series' : 'Movie'
    const item = { Type: type, Name: 'Amber Road', SeriesId: type === 'Episode' ? 's1' : undefined }
    const access = name.endsWith('join only') ? 'JoinGroups' : name.includes('SyncPlay off') ? 'None' : 'CreateAndJoinGroups'
    const later = name.includes('turned off since') ? 'None' : name.includes('join only since') ? 'JoinGroups' : 'CreateAndJoinGroups'
    const failure = (status) => Object.assign(new Error('HTTP ' + status), { status })
    // What the socket says, as jellyfin-web's ApiClient.subscribe gives it.
    const subscribers = {}
    const ApiClient = {
      getCurrentUserId: () => 'u1',
      serverId: () => 'srv',
      getUrl: (path) => path,
      getItem: (user, id) => Promise.resolve(item),
      getCurrentUser: name.includes('user unknown') ? undefined
        : () => (name.includes('failing to load') ? Promise.reject(failure(500)) : Promise.resolve({ Name: 'Morgan', Policy: { SyncPlayAccess: access } })),
      getUser: (id) => { record('GET Users/' + id); return Promise.resolve({ Policy: { SyncPlayAccess: later } }) },
      getJSON: (url) => {
        if (url.endsWith('/Versions')) {
          record('GET ' + url)
          return Promise.resolve(name.includes('without a source') ? { Pending: 0, Count: 1, Known: 0 }
            : name.includes('still searching') ? { Pending: 1, Count: 1, Known: 0 } : { Pending: 0, Count: 1, Known: 1 })
        }
        return Promise.resolve(url === 'Polyfin/UserItems/Hidden' || url === 'Polyfin/UserItems/Watchlist' ? { Items: [] } : {})
      },
      ajax: (request) => {
        const line = request.type + ' ' + request.url + (request.data ? ' ' + request.data : '')
        let status = 0
        if (request.url === 'SyncPlay/New') status = name.includes('since') || name.includes('refused otherwise') ? 403 : name.includes('a failure') ? 500 : 0
        if (request.url === 'SyncPlay/g0' && name.includes('ended')) status = 404
        record(line + (status ? ' (' + status + ')' : ''))
        if (status) return Promise.reject(failure(status))
        if (request.url === 'SyncPlay/New') {
          // jellyfin-web joins the group, and pings it once its clock is
          // set: a moment later, or before the answer comes.
          const joined = () => {
            tell('GroupJoined', 'g1')
            if (!name.includes('never pinging')) ApiClient.sendSyncPlayPing({ Ping: 12 })
          }
          if (name.includes('before the answer')) joined()
          else afterTicks(joined, 5)
          return Promise.resolve({ GroupId: 'g1', State: 'Idle' })
        }
        if (request.type === 'GET' && request.url.startsWith('SyncPlay/')) {
          return Promise.resolve({ GroupId: request.url.slice(9), State: name.includes('refuses') ? 'Idle' : 'Waiting' })
        }
        return Promise.resolve(undefined)
      },
      subscribe: (types, listener) => { for (const type of types) (subscribers[type] ||= []).push(listener) },
      sendSyncPlayPing: () => { record('POST SyncPlay/Ping'); return Promise.resolve() },
    }
    const tell = (Type, GroupId) => {
      for (const listener of subscribers.SyncPlayGroupUpdate || []) listener({ MessageType: 'SyncPlayGroupUpdate', Data: { GroupId, Type, Data: '' } })
    }
    const afterTicks = (run, ticks) => (ticks ? setImmediate(() => afterTicks(run, ticks - 1)) : run())
    // The menu jellyfin-web opens: a dialog with its own entries.
    const entries = []
    const model = { cloneNode: () => {
      const text = { textContent: 'Play' }
      const mark = { className: 'actionsheetMenuItemIcon material-icons play_arrow' }
      const entry = { text, mark, classList: classes([]), listeners: {}, attributes: {} }
      entry.setAttribute = (a, v) => { entry.attributes[a] = v }
      entry.querySelector = (s) => (s === '.actionSheetItemText' ? text : s === '.actionsheetMenuItemIcon' ? mark : null)
      entry.addEventListener = (type, listener) => { entry.listeners[type] = listener }
      return entry
    } }
    const scroller = { querySelector: (s) => (s === '.actionSheetMenuItem' ? model : null), appendChild: (e) => entries.push(e) }
    // On a short screen, the menu jellyfin-web placed for its own entries
    // runs past the bottom with Polyfin's, though it measures less while
    // its opening animation scales it down.
    const short = name.includes('short screen')
    const sheet = { style: { top: short ? '500px' : '10px', left: '' }, getAttribute: (a) => (a === 'data-history' ? 'true' : null),
      querySelector: (s) => (s === '.actionSheetScroller' ? scroller : null), offsetHeight: short ? 460 : 90,
      getBoundingClientRect: () => (short ? { bottom: 730, height: 230, right: 300, width: 218 } : { bottom: 100, height: 90, right: 300, width: 218 }) }
    const container = { isConnected: true, classList: classes(['dialogContainer']), querySelector: (s) => (s === '.actionSheet' || s === '.dialog' ? sheet : null) }
    const toasts = []
    const element = () => ({ classList: classes([]), setAttribute: () => {}, addEventListener: () => {},
      appendChild: (child) => { if (child.className === 'toast') toasts.push(child) }, remove: () => {} })
    // jellyfin-web's SyncPlay button: the default layout's, or the legacy
    // layouts' header button, which it hides rather than leaves out.
    const legacy = name.includes('legacy')
    const shown = {
      'button[aria-controls="app-sync-play-menu"]': !legacy && !name.includes('no button'),
      '.headerSyncButton:not(.hide)': legacy && !name.includes('no button'),
    }
    const document = {
      head: { appendChild: () => {} }, body: { appendChild: () => {} }, documentElement: { lang: name.includes('French') ? 'fr' : 'en' },
      createElement: element,
      querySelector: (s) => (s.split(', ').some((part) => shown[part]) ? {} : null),
      querySelectorAll: () => [],
    }
    const hash = onCard ? '#/home' : '#/details?id=' + page
    const location = { href: 'http://polyfin.test/web/' + hash, hash, replace: () => {} }
    const history = { pushState: () => {}, replaceState: () => {}, back: () => did.push('back') }
    const timers = []
    runScript(script, {
      location, history, URL, document, MutationObserver, Promise, WeakMap,
      window: { ApiClient, innerHeight: 800, innerWidth: 1280,
        sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
        localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} } },
      Date: { now: () => 0, parse: Date.parse }, setTimeout: (run, delay) => { timers.push({ run, delay }); return timers.length }, clearTimeout: () => {},
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    })
    // The session's group, as the socket told it.
    if (name.includes('in a group') || name.includes("another user's group")) {
      if (name.includes("another user's")) ApiClient.getCurrentUserId = () => 'u2'
      tell('GroupJoined', 'g0')
      ApiClient.getCurrentUserId = () => 'u1'
    }
    if (name.includes('then left')) tell('GroupLeft', 'g0')
    if (name.includes('then not in one')) tell('NotInGroup', '00000000000000000000000000000000')
    if (name.includes('that went')) tell('GroupDoesNotExist', '00000000000000000000000000000000')
    if (name.includes('others joining')) tell('UserJoined', 'g0')
    // The ⋮ button of the page, or of the card.
    const card = { getAttribute: (a) => (a === 'data-id' ? page : null), closest: (s) => (s === '.card[data-id], .listItem[data-id]' ? card : null) }
    const button = { closest: (s) => (s === 'button[data-action="menu"]' ? button : null) }
    if (onCard) button.closest = (s) => (s === 'button[data-action="menu"]' ? button : s === '.card[data-id], .listItem[data-id]' ? card : null)
    const target = onCard ? { closest: (s) => (s === 'button[data-action="menu"]' ? button : null) }
      : { closest: (s) => (s === '.btnMoreCommands' ? {} : null) }
    for (const listener of listeners.click || []) listener({ type: 'click', target })
    observers.forEach((observer) => observer.callback([{ addedNodes: [container] }]))
    for (let i = 0; i < 5; i++) await settle()
    const words = entries.map((e) => e.text.textContent)
    if (short) words.push('top ' + sheet.style.top)
    const together = entries.find((e) => e.mark.className.endsWith(' groups'))
    if (together) {
      recording = true
      timers.length = 0
      together.listeners.click({ stopPropagation: () => {}, preventDefault: () => {} })
      for (let i = 0; i < 20; i++) await settle()
      // 10 seconds pass.
      if (name.includes('never pinging')) {
        did.push('10 seconds')
        timers.filter((timer) => timer.delay === 10000).forEach((timer) => timer.run())
        for (let i = 0; i < 20; i++) await settle()
      }
    }
    // A toast lasts as long as the timer after its first, of 50 ms, says.
    const delays = timers.map((timer) => timer.delay)
    const lasts = delays[delays.lastIndexOf(50) + 1]
    const said = toasts.map((t) => 'toast ' + t.textContent + (lasts === 10000 ? ' (10s)' : ''))
    const pressed = did.concat(said).join(', ')
    results[name] = words.join(' | ') + (pressed ? ' => ' + pressed : '')
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	names := make([]string, 0, len(scenarios))
	for name := range scenarios {
		names = append(names, name)
	}
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody), "scenarios": names, "page": page})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results map[string]string
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	for name, want := range scenarios {
		if got := results[name]; got != want {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
	}
}
