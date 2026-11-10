package server

import (
	"encoding/json"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// Once "Watch together" started the group, a picker lists the people its
// group may invite, as Polyfin answers, with a box each, Invite and Skip;
// the entry used again in a group that plays the title opens it again,
// and nothing else. Invite invites each person ticked and says so; Skip,
// Escape, a click beside it, or nobody to invite say how the others join
// instead. The keyboard stays in the picker. Clicks pass through the
// toasts, which lie over the video's Play button. An invitation pushed on the
// socket shows a prompt, which Join answers with Jellyfin's
// POST /SyncPlay/Join, and Not now or Escape by declining it; an end of
// the invitation heard on the socket closes it, as does its expiry. It
// runs in Node.js, in a context that stands for the browser and
// jellyfin-web.
func TestWatchTogetherInvitesPeople(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const page = "0123456789abcdef0123456789abcdef"
	const started = `POST SyncPlay/New {"GroupName":"Morgan’s group"}, POST SyncPlay/SetNewQueue {"PlayingQueue":["` + page +
		`"],"PlayingItemPosition":0,"StartPositionTicks":0}, GET SyncPlay/g1, POST SyncPlay/Pause, GET Polyfin/SyncPlay/Invitees, `
	const picker = "picker dialog true | Watch together | Your SyncPlay group is ready, paused. Choose whom to invite: a prompt to join" +
		" shows in their web player. Press Play once they are in. | [ ] Sam, [ ] Robin | Skip, Invite (disabled) | focus Sam"
	const again = "picker dialog true | Watch together | Choose whom to invite to your SyncPlay group: a prompt to join shows in" +
		" their web player. | [ ] Sam, [ ] Robin | Skip, Invite (disabled) | focus Sam"
	const ready = "toast Your SyncPlay group is ready, paused. The others join it on their own device, from the SyncPlay button" +
		" (the people icon at the top). Press Play once they are in. (10s)"
	const invite = `POST Polyfin/SyncPlay/Invitations {"GroupId":"g1","UserId":"u2"}`
	const prompt = "prompt alertdialog | Watch together | Morgan invites you to watch together. | Show / Pilot | Not now, Join | focus Join"
	scenarios := map[string]string{
		// The host.
		"inviting Sam": started + picker + ", tick Sam, Invite, " + invite + ", nothing shown, focus Play, " +
			"toast Invitation sent to Sam. Press Play once they are in. (10s)",
		"inviting two, one too soon": started + picker + ", tick Sam, tick Robin, Invite, " + invite +
			`, POST Polyfin/SyncPlay/Invitations {"GroupId":"g1","UserId":"u3"} (429), nothing shown, focus Play, ` +
			"toast Invitation sent to Sam. Press Play once they are in. (10s), toast Invited less than a minute ago already: Robin.",
		"invitations failing": started + picker + ", tick Sam, Invite, " + invite + " (500), nothing shown, focus Play, " +
			"toast The invitation could not be sent. Try again.",
		"skipping":                     started + picker + ", Skip, nothing shown, focus Play, " + ready,
		"skipping with Escape":         started + picker + ", Escape, nothing shown, focus Play, " + ready,
		"skipping with a click beside": started + picker + ", click beside, nothing shown, focus Play, " + ready,
		"nobody to invite":             strings.TrimSuffix(started, ", ") + ", nothing shown, focus Play, " + ready,
		"invitees failing":             strings.TrimSuffix(started, ", ") + ", nothing shown, focus Play, " + ready,
		"by keyboard": started + picker + ", ArrowDown: focus Robin, Enter: focus Robin, Tab: focus Skip, Tab: focus Invite, " +
			"Tab: focus Sam, Shift+Tab: focus Invite, ArrowUp: focus Skip, " +
			"picker dialog true | Watch together | Your SyncPlay group is ready, paused. Choose whom to invite: a prompt to join" +
			" shows in their web player. Press Play once they are in. | [ ] Sam, [x] Robin | Skip, Invite | focus Skip",
		"again in a group playing the title": "GET Polyfin/SyncPlay/Invitees, " + again + `, tick Sam, Invite, ` +
			`POST Polyfin/SyncPlay/Invitations {"GroupId":"g0","UserId":"u2"}, nothing shown, focus Play, toast Invitation sent to Sam.`,
		"again in a group playing the title, skipping": "GET Polyfin/SyncPlay/Invitees, " + again + ", Skip, nothing shown, focus Play",
		"again in a group playing the title, nobody to invite": "GET Polyfin/SyncPlay/Invitees, nothing shown, focus Play, " +
			"toast Nobody else can be invited to this title right now.",
		"again in a group playing another title": `GET SyncPlay/g0, POST SyncPlay/SetNewQueue {"PlayingQueue":["` + page +
			`"],"PlayingItemPosition":0,"StartPositionTicks":0}, GET SyncPlay/g0, POST SyncPlay/Pause, GET Polyfin/SyncPlay/Invitees, ` +
			picker + ", Skip, nothing shown, focus Play, " + ready,
		"inviting Sam, in French": strings.Replace(started, "Morgan’s group", "Groupe de Morgan", 1) + "picker dialog true | Regarder ensemble | Votre groupe SyncPlay est prêt, en pause." +
			" Choisissez qui inviter\u202f: une invitation à le rejoindre s’affiche dans leur lecteur web. Appuyez sur Lire une fois" +
			" qu’ils sont là. | [ ] Sam, [ ] Robin | Passer, Inviter (disabled) | focus Sam, tick Sam, Inviter, " + invite +
			", nothing shown, focus Play, toast Invitation envoyée à Sam. Appuyez sur Lire une fois qu’ils sont là. (10s)",
		// The invitee.
		"a prompt, Join": prompt + `, Join, POST SyncPlay/Join {"GroupId":"g1"}, nothing shown, focus Play`,
		"a prompt, Join failing": prompt + `, Join, POST SyncPlay/Join {"GroupId":"g1"} (500), nothing shown, focus Play, ` +
			"toast The group could not be joined. Try again from the SyncPlay button (the people icon at the top).",
		"a prompt, Not now":                    prompt + ", Not now, POST Polyfin/SyncPlay/Invitations/i1/Decline, nothing shown, focus Play",
		"a prompt, Escape":                     prompt + ", Escape, POST Polyfin/SyncPlay/Invitations/i1/Decline, nothing shown, focus Play",
		"a prompt, by keyboard":                prompt + ", ArrowLeft: focus Not now, Tab: focus Join, Tab: focus Not now, Shift+Tab: focus Join",
		"a prompt, joined on another device":   prompt + ", told Joined i1, nothing shown, focus Play",
		"a prompt, declined on another device": prompt + ", told Declined i1, nothing shown, focus Play",
		"a prompt, its group ended":            prompt + ", told Ended i1, nothing shown, focus Play",
		"a prompt, another invitation ended":   prompt + ", told Expired i0, " + prompt,
		"a prompt, replaced": prompt + ", told Invited i2, " +
			"prompt alertdialog | Watch together | Robin invites you to watch together. | Amber Road | Not now, Join | focus Join",
		"a prompt, expiring":                       prompt + ", 5 minutes, nothing shown, focus Play",
		"a prompt, through the message event":      prompt + `, Join, POST SyncPlay/Join {"GroupId":"g1"}, nothing shown, focus Play`,
		"a prompt, the user elsewhere on the page": prompt + ", Not now, POST Polyfin/SyncPlay/Invitations/i1/Decline, nothing shown, focus Search",
		"a prompt, in German": "prompt alertdialog | Gemeinsam ansehen | Morgan lädt dich zum gemeinsamen Ansehen ein. | Show / Pilot |" +
			" Nicht jetzt, Beitreten | focus Beitreten",
	}
	const harness = scriptRunner + `
const { script, scenarios, page } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))
const settle = async (times) => { for (let i = 0; i < (times || 20); i++) await new Promise((resolve) => setImmediate(resolve)) }
;(async () => {
  const results = {}
  for (const name of scenarios) {
    const did = []
    let recording = name.startsWith('a prompt')
    const record = (line) => { if (recording) did.push(line) }
    const listeners = {}
    const observers = []
    class MutationObserver {
      constructor(callback) { this.callback = callback }
      observe(target, options) { if (!options || !options.subtree) observers.push(this) }
      disconnect() {}
    }
    // A document of elements that match tags and classes, keep the focus,
    // and pass events up to their parents.
    let active = null
    let pendingToast = null
    // The toasts, said last.
    const toasts = []
    const one = (element, selector) => {
      const match = /^([a-z0-9]*)((?:\.[\w-]+)*)$/i.exec(selector.trim())
      if (!match || (!match[1] && !match[2])) return false
      if (match[1] && element.tagName !== match[1].toUpperCase()) return false
      const own = String(element.className || '').split(/\s+/)
      return !match[2] || match[2].slice(1).split('.').every((c) => own.includes(c))
    }
    const matches = (element, selector) => !!element.tagName && selector.split(',').some((part) => one(element, part))
    const all = (root, selector) => (root.children || []).flatMap((child) => [...(matches(child, selector) ? [child] : []), ...all(child, selector)])
    class Element {
      constructor(tag) {
        Object.assign(this, { tagName: tag.toUpperCase(), className: '', id: '', children: [], parentNode: null, text: '', attributes: {},
          handlers: {}, disabled: false, checked: false, type: '', style: {} })
      }
      get classList() {
        const list = () => String(this.className || '').split(/\s+/).filter(Boolean)
        return { contains: (c) => list().includes(c), add: (c) => { if (!list().includes(c)) this.className = [...list(), c].join(' ') },
          remove: (c) => { this.className = list().filter((x) => x !== c).join(' ') }, toggle: () => {} }
      }
      set textContent(text) { this.text = String(text); this.children = [] }
      get textContent() { return this.text + this.children.map((child) => child.textContent).join('') }
      appendChild(child) { return this.insertBefore(child, null) }
      insertBefore(child, before) {
        if (child.parentNode) child.parentNode.removeChild(child)
        const at = before ? this.children.indexOf(before) : -1
        this.children.splice(at < 0 ? this.children.length : at, 0, child)
        child.parentNode = this
        if (child.className === 'toast') { pendingToast = { toast: child.textContent + (child.style.pointerEvents === 'none' ? '' : ' (blocks clicks)'), calls: 0 }; toasts.push(pendingToast) }
        return child
      }
      removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; return child }
      remove() { if (this.parentNode) this.parentNode.removeChild(this) }
      querySelector(selector) { return all(this, selector)[0] || null }
      querySelectorAll(selector) { return all(this, selector) }
      setAttribute(name, value) { this.attributes[name] = String(value); if (name === 'type') this.type = String(value) }
      getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
      removeAttribute(name) { delete this.attributes[name] }
      addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler) }
      removeEventListener() {}
      dispatch(type, fields) {
        const event = Object.assign({ type, target: this, stopped: false, preventDefault() {}, stopPropagation() { this.stopped = true } }, fields)
        for (let at = this; at && !event.stopped; at = at.parentNode) for (const handler of at.handlers[type] || []) handler.call(at, event)
      }
      click() {
        if (this.disabled) return
        if (this.type === 'checkbox') this.checked = !this.checked
        this.dispatch('click')
        if (this.type === 'checkbox') this.dispatch('change')
      }
      focus() { active = this }
      contains(other) { for (let at = other; at; at = at.parentNode) if (at === this) return true; return false }
      closest(selector) { for (let at = this; at && at.tagName; at = at.parentNode) if (matches(at, selector)) return at; return null }
      getBoundingClientRect() { return { top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 } }
    }
    const body = new Element('body')
    const head = new Element('head')
    const french = name.includes('French')
    const document = {
      body, head,
      documentElement: { lang: french ? 'fr' : name.includes('German') ? 'de' : 'en', classList: { contains: () => false, add() {}, remove() {} } },
      get activeElement() { return active || body },
      createElement: (tag) => new Element(tag),
      createTextNode: (text) => ({ textContent: text, parentNode: null }),
      getElementById: (id) => [...all(head, 'style'), ...all(body, 'div')].find((element) => element.id === id) || null,
      // jellyfin-web's SyncPlay button shows.
      querySelector: (selector) => (selector.includes('app-sync-play-menu') ? {} : body.querySelector(selector)),
      querySelectorAll: (selector) => body.querySelectorAll(selector),
    }
    // The page, with a control focused.
    const play = new Element('button')
    play.textContent = name.includes('elsewhere') ? 'Search' : 'Play'
    body.appendChild(play)
    play.focus()
    const failure = (status) => Object.assign(new Error('HTTP ' + status), { status })
    const subscribers = {}
    const again = name.startsWith('again')
    const ApiClient = {
      getCurrentUserId: () => 'u1',
      serverId: () => 'srv',
      getUrl: (path) => path,
      getItem: () => Promise.resolve({ Type: 'Movie', Name: 'Amber Road' }),
      getCurrentUser: () => Promise.resolve({ Name: 'Morgan', Policy: { SyncPlayAccess: 'CreateAndJoinGroups' } }),
      getJSON: (url) => {
        if (url === 'Polyfin/SyncPlay/Invitees') {
          record('GET ' + url)
          if (name.includes('invitees failing')) return Promise.reject(failure(500))
          return Promise.resolve({ GroupId: again ? 'g0' : 'g1',
            Items: name.includes('nobody') ? [] : [{ Id: 'u2', Name: 'Sam' }, { Id: 'u3', Name: 'Robin' }] })
        }
        if (url.endsWith('/Versions')) return Promise.resolve({ Pending: 0, Count: 1, Known: 1 })
        return Promise.resolve(url === 'Polyfin/UserItems/Hidden' || url === 'Polyfin/UserItems/Watchlist' ? { Items: [] } : {})
      },
      ajax: (request) => {
        let status = 0
        if (request.url === 'Polyfin/SyncPlay/Invitations') {
          status = name.includes('too soon') && JSON.parse(request.data).UserId === 'u3' ? 429 : name.includes('invitations failing') ? 500 : 0
        }
        if (request.url === 'SyncPlay/Join' && name.includes('failing')) status = 500
        record(request.type + ' ' + request.url + (request.data ? ' ' + request.data : '') + (status ? ' (' + status + ')' : ''))
        if (status) return Promise.reject(failure(status))
        if (request.url === 'SyncPlay/New') {
          setImmediate(() => { tell({ GroupId: 'g1', Type: 'GroupJoined', Data: '' }); ApiClient.sendSyncPlayPing() })
          return Promise.resolve({ GroupId: 'g1', State: 'Idle' })
        }
        if (request.type === 'GET') return Promise.resolve({ GroupId: request.url.slice(9), State: 'Waiting' })
        return Promise.resolve(undefined)
      },
      sendSyncPlayPing: () => Promise.resolve(),
    }
    const viaEvent = name.includes('message event')
    if (!viaEvent) ApiClient.subscribe = (types, listener) => { for (const type of types) (subscribers[type] ||= []).push(listener) }
    const send = (MessageType, Data) => {
      const message = { MessageType, Data }
      if (viaEvent) for (const listener of (ApiClient._callbacks || {}).message || []) listener({ type: 'message' }, message)
      else for (const listener of subscribers[MessageType] || []) listener(message)
    }
    const tell = (update) => send('SyncPlayGroupUpdate', update)
    const timers = []
    const hash = '#/details?id=' + page
    runScript(script, {
      location: { href: 'http://polyfin.test/web/' + hash, hash, replace: () => {} },
      history: { pushState: () => {}, replaceState: () => {}, back: () => {} },
      URL, document, MutationObserver, Promise, WeakMap,
      window: { ApiClient, innerHeight: 800, innerWidth: 1280,
        sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
        localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} } },
      Date: { now: () => 0, parse: Date.parse },
      setTimeout: (run, delay) => {
        timers.push({ run, delay })
        // A toast's second timer is how long it lasts.
        if (pendingToast && ++pendingToast.calls === 2) { pendingToast.lasts = delay; pendingToast = null }
        return timers.length
      },
      clearTimeout: (id) => { if (timers[id - 1]) timers[id - 1].run = () => {} },
      addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) },
    })
    // What shows: the picker or the prompt, with the control focused.
    const focusName = () => (active && active.type === 'checkbox' ? active.parentNode.textContent : active ? active.textContent : 'none')
    const describe = (dialog, kind) => [kind + ' ' + dialog.getAttribute('role') + (dialog.getAttribute('aria-modal') ? ' ' + dialog.getAttribute('aria-modal') : ''),
      ...dialog.children.map((part) => part.className === 'polyfinInviteList'
        ? part.children.map((label) => (label.children[0].checked ? '[x] ' : '[ ] ') + label.textContent).join(', ')
        : part.className === 'polyfinInviteButtons' ? part.children.map((b) => b.textContent + (b.disabled ? ' (disabled)' : '')).join(', ')
        : part.children.length ? part.children.map((child) => child.textContent).join(' / ') : part.textContent),
      'focus ' + focusName()].join(' | ')
    const shown = () => {
      const backdrop = body.querySelector('.polyfinInviteBackdrop')
      const prompts = body.querySelectorAll('.polyfinInvitePrompt')
      if (backdrop) did.push(describe(backdrop.children[0], 'picker'))
      prompts.forEach((prompt) => did.push(describe(prompt, 'prompt')))
      if (!backdrop && !prompts.length) did.push('nothing shown, focus ' + focusName())
    }
    const button = (text) => body.querySelectorAll('button').find((b) => b.textContent === text)
    const press = (text) => { did.push(text); button(text).click() }
    const key = (key, shiftKey) => {
      if (key === 'Escape') did.push(key)
      active.dispatch('keydown', { key, shiftKey })
      if (key !== 'Escape') did.push((shiftKey ? 'Shift+' : '') + key + ': focus ' + focusName())
    }
    const tick = (who) => { did.push('tick ' + who); body.querySelectorAll('label').find((l) => l.textContent === who).children[0].click() }
    await settle()
    if (name.startsWith('a prompt')) {
      const invite = { Id: 'i1', State: 'Invited', GroupId: 'g1', GroupName: 'Morgan’s group', HostName: 'Morgan', ItemId: 'e1',
        ItemName: 'Pilot', SeriesName: 'Show', ExpiresAt: '2026-11-10T20:05:00.0000000Z' }
      send('PolyfinSyncPlayInvite', invite)
      shown()
      const end = /(joined|declined) on another device|group ended|another invitation ended/.exec(name)
      if (end) {
        const state = { joined: 'Joined', declined: 'Declined', 'group ended': 'Ended', 'another invitation ended': 'Expired' }[end[1] || end[0]]
        const id = state === 'Expired' ? 'i0' : 'i1'
        did.push('told ' + state + ' ' + id)
        send('PolyfinSyncPlayInvite', { Id: id, State: state, GroupId: 'g1' })
      } else if (name.includes('replaced')) {
        did.push('told Invited i2')
        send('PolyfinSyncPlayInvite', { Id: 'i2', State: 'Invited', GroupId: 'g2', HostName: 'Robin', ItemId: 'm1', ItemName: 'Amber Road' })
      } else if (name.includes('expiring')) {
        did.push('5 minutes')
        timers.filter((timer) => timer.delay === 300000).forEach((timer) => timer.run())
      } else if (name.includes('keyboard')) {
        key('ArrowLeft'); key('Tab'); key('Tab'); key('Tab', true)
        results[name] = did.join(', ')
        continue
      } else if (name.includes('German')) {
        results[name] = did.join(', ')
        continue
      } else if (name.includes('Escape')) {
        key('Escape')
      } else {
        press(name.includes('Not now') || name.includes('elsewhere') ? 'Not now' : 'Join')
      }
      await settle()
      shown()
    } else {
      if (again) {
        tell({ GroupId: 'g0', Type: 'GroupJoined', Data: '' })
        tell({ GroupId: 'g0', Type: 'PlayQueue', Data: { Playlist: [{ ItemId: name.includes('another title') ? 'fedcba9876543210fedcba9876543210' : page.toUpperCase(),
          PlaylistItemId: 'p1' }], PlayingItemIndex: 0 } })
      }
      // The ⋮ of the title's page, its menu, and Watch together.
      for (const listener of listeners.click || []) listener({ type: 'click', target: { closest: (s) => (s === '.btnMoreCommands' ? {} : null) } })
      const entries = []
      const model = { cloneNode: () => {
        const text = { textContent: '' }
        const mark = { className: 'actionsheetMenuItemIcon material-icons play_arrow' }
        const entry = { text, mark, classList: { add() {} }, listeners: {}, setAttribute() {} }
        entry.querySelector = (s) => (s === '.actionSheetItemText' ? text : s === '.actionsheetMenuItemIcon' ? mark : null)
        entry.addEventListener = (type, listener) => { entry.listeners[type] = listener }
        return entry
      } }
      const scroller = { querySelector: (s) => (s === '.actionSheetMenuItem' ? model : null), appendChild: (e) => entries.push(e) }
      const sheet = { style: { top: '10px', left: '' }, getAttribute: () => 'true', querySelector: (s) => (s === '.actionSheetScroller' ? scroller : null),
        offsetHeight: 90, getBoundingClientRect: () => ({ bottom: 100, height: 90, right: 300, width: 218 }) }
      const container = { isConnected: true, classList: { contains: (c) => c === 'dialogContainer' }, querySelector: (s) => (s === '.actionSheet' || s === '.dialog' ? sheet : null) }
      observers.forEach((observer) => observer.callback([{ addedNodes: [container] }]))
      await settle(5)
      const together = entries.find((e) => e.mark.className.endsWith(' groups'))
      if (!together) { results[name] = 'no Watch together'; continue }
      recording = true
      together.listeners.click({ stopPropagation: () => {}, preventDefault: () => {} })
      await settle(40)
      shown()
      if (body.querySelector('.polyfinInviteBackdrop')) {
        if (name.includes('keyboard')) {
          key('ArrowDown'); key('Enter'); key('Tab'); key('Tab'); key('Tab'); key('Tab', true); key('ArrowUp')
          shown()
          results[name] = did.join(', ')
          continue
        }
        if (name.includes('skipping with Escape')) key('Escape')
        else if (name.includes('click beside')) { did.push('click beside'); body.querySelector('.polyfinInviteBackdrop').click() }
        else if (name.includes('skipping') || name.includes('another title')) press(french ? 'Passer' : 'Skip')
        else {
          tick('Sam')
          if (name.includes('two')) tick('Robin')
          press(french ? 'Inviter' : 'Invite')
        }
        await settle(40)
        shown()
      }
    }
    results[name] = [...did, ...toasts.map((line) => 'toast ' + line.toast + (line.lasts === 10000 ? ' (10s)' : ''))].join(', ')
  }
  process.stdout.write(JSON.stringify(results))
})()
`
	names := slices.Sorted(maps.Keys(scenarios))
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
	for _, name := range names {
		if got, want := results[name], scenarios[name]; got != want {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
	}
}
