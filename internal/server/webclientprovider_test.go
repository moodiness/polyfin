package server

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// jellyfin-web's sign-in page offers the identity provider the server
// names: a button first among the page's buttons, which opens the
// provider's sign-in for the web client, in jellyfin-web's language. Back
// from the provider, the script takes its parameter out of the address,
// says what went wrong under the button, or, once jellyfin-web drew its
// first page, trades the ticket as jellyfin-web's device and keeps the
// sign-in in this server's entry of jellyfin-web's credentials, keeping
// the rest, then opens /web/ again. A trade refused says so and keeps
// nothing. It runs in Node.js, in a context that stands for the browser.
func TestTheSignInPageOffersTheProvider(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	const harness = `
const vm = require('node:vm')
const { script } = JSON.parse(require('node:fs').readFileSync(0, 'utf8'))

// Element stands for the DOM's elements, with selectors of tags, classes
// and IDs, and descendants.
class Element {
  constructor(tag, className = '', id = '') {
    this.tagName = tag.toUpperCase(); this.className = className; this.id = id
    this.children = []; this.parentNode = null; this.attributes = {}; this.listeners = {}; this.text = ''
    this.style = {}; this.dataset = {}
    this.classList = { contains: (c) => this.className.split(/\s+/).includes(c), add: () => {}, remove: () => {}, toggle: () => {} }
  }
  get textContent() { return this.text + this.children.map((c) => c.textContent).join('') }
  set textContent(value) { this.children = []; this.text = String(value) }
  get firstChild() { return this.children[0] || null }
  get nextSibling() { const s = this.parentNode ? this.parentNode.children : []; return s[s.indexOf(this) + 1] || null }
  appendChild(child) { return this.insertBefore(child, null) }
  insertBefore(child, ref) {
    child.remove(); child.parentNode = this
    const at = ref ? this.children.indexOf(ref) : -1
    if (at < 0) this.children.push(child); else this.children.splice(at, 0, child)
    return child
  }
  remove() { if (this.parentNode) { const s = this.parentNode.children; s.splice(s.indexOf(this), 1); this.parentNode = null } }
  setAttribute(name, value) { this.attributes[name] = String(value) }
  getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
  removeAttribute(name) { delete this.attributes[name] }
  addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener) }
  removeEventListener() {}
  click() { (this.listeners.click || []).forEach((listener) => listener({ type: 'click', target: this, preventDefault() {} })) }
  matches(selector) {
    const m = /^([a-z]*)((?:[.#][\w-]+)*)$/i.exec(selector.trim())
    if (!m) return false
    if (m[1] && m[1].toUpperCase() !== this.tagName) return false
    return (m[2].match(/[.#][\w-]+/g) || []).every((part) => part[0] === '#' ? this.id === part.slice(1) : this.classList.contains(part.slice(1)))
  }
  closest(selector) { for (let e = this; e; e = e.parentNode) if (e.matches && e.matches(selector)) return e; return null }
  all() { return this.children.flatMap((c) => [c, ...c.all()]) }
  querySelectorAll(selector) {
    return selector.split(',').flatMap((one) => {
      const parts = one.trim().split(/\s+/)
      return this.all().filter((e) => {
        if (!e.matches(parts[parts.length - 1])) return false
        let i = parts.length - 2
        for (let p = e.parentNode; p && i >= 0; p = p.parentNode) if (p.matches(parts[i])) i--
        return i < 0
      })
    })
  }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null }
}

// run loads the script on a scenario's page and tells what it did.
async function run(s) {
  const results = { requests: [], replaced: '', drawnAt: 0 }
  const listeners = {}
  const location = { replace: (url) => { results.replaced = new URL(url, location.href).pathname } }
  const go = (url) => { const u = new URL(url, location.href); Object.assign(location, { href: u.href, hash: u.hash, pathname: u.pathname, search: u.search, origin: u.origin }) }
  go(s.address)
  const history = { state: null, pushState: (state, title, url) => go(url), replaceState: (state, title, url) => go(url) }
  const storage = new Map(Object.entries(s.storage || {}))
  const localStorage = { getItem: (k) => (storage.has(k) ? storage.get(k) : null), setItem: (k, v) => storage.set(k, String(v)), removeItem: (k) => storage.delete(k) }
  const documentElement = new Element('html'); documentElement.lang = s.lang || 'en'
  const body = new Element('body')
  const document = { documentElement, body, head: new Element('head'), createElement: (tag) => new Element(tag),
    querySelectorAll: (selector) => body.querySelectorAll(selector), querySelector: (selector) => body.querySelector(selector),
    getElementById: (id) => body.all().find((e) => e.id === id) || null, addEventListener: () => {}, removeEventListener: () => {} }
  // drawLogin draws jellyfin-web's sign-in page and routes to it.
  const drawLogin = () => {
    const page = body.appendChild(new Element('div', 'page standalonePage', 'loginPage'))
    const buttons = page.appendChild(new Element('div', 'readOnlyContent'))
    buttons.appendChild(new Element('button', 'raised cancel block btnManual'))
    buttons.appendChild(new Element('button', 'raised cancel block btnQuick'))
    go('#/login'); results.drawnAt = Date.now()
    ;(listeners.hashchange || []).forEach((listener) => listener())
  }
  const fetch = async (url, options = {}) => {
    results.requests.push({ url, method: options.method || 'GET', authorization: (options.headers || {}).Authorization || '', at: Date.now() })
    let status = 404, answer = {}
    if (url === '/admin/api/status') { status = 200; answer = { name: 'Polyfin', provider: s.provider || null } }
    if (url === '/admin/api/provider/web') { status = s.trade.status; answer = s.trade.body }
    return { ok: status >= 200 && status < 300, status, json: async () => answer }
  }
  // jellyfin-web 12.4's client gives its app's name escaped already.
  const ApiClient = { deviceId: () => 'device-1', deviceName: () => 'Firefox', appName: () => 'Jellyfin%20Web', appVersion: () => '12.4.0' }
  const context = { location, history, URL, URLSearchParams, document, localStorage, fetch, setTimeout, clearTimeout, setInterval, clearInterval,
    btoa, navigator: { userAgent: 'Mozilla/5.0 Firefox/130.0' }, ApiClient, Promise, console: { log() {}, error() {}, debug() {} },
    MutationObserver: class { observe() {} disconnect() {} }, requestAnimationFrame: (f) => setTimeout(f, 0), getComputedStyle: () => ({}),
    addEventListener: (type, listener) => { (listeners[type] ||= []).push(listener) }, removeEventListener: () => {} }
  context.window = context
  vm.createContext(context)
  if (s.drawn) drawLogin()
  vm.runInContext(script, context)
  if (!s.drawn) setTimeout(drawLogin, 600)
  await new Promise((resolve) => setTimeout(resolve, 1500))
  const buttons = body.querySelector('#loginPage .readOnlyContent')
  results.address = location.pathname + location.search + location.hash
  results.buttons = buttons ? buttons.children.map((b) => b.className.split(' ').pop() + (b.textContent ? ': ' + b.textContent : '')) : []
  const button = body.querySelector('.polyfinProviderButton')
  if (button) { button.click(); results.clicked = location.href }
  results.credentials = storage.has('jellyfin_credentials') ? JSON.parse(storage.get('jellyfin_credentials')) : null
  results.traded = results.requests.filter((r) => r.url === '/admin/api/provider/web')
  results.tradedAfterDrawing = results.traded.every((r) => results.drawnAt && r.at >= results.drawnAt)
  for (const r of results.traded) delete r.at
  delete results.requests
  delete results.drawnAt
  return results
}

const provider = { name: 'Example ID', start: 'https://media.example.org/admin/api/provider/start' }
const signedIn = { status: 200, body: { accessToken: 'token-1', userId: 'user-1', serverId: 'server-1', serverName: 'Polyfin' } }
const saved = { jellyfin_credentials: JSON.stringify({ Servers: [{ Id: 'other', AccessToken: 'other-token' },
  { Id: 'server-1', Name: 'Living room', ManualAddress: 'http://polyfin.test', LocalAddress: 'http://10.0.0.2:8096' }] }) }
const scenarios = {
  offered: { address: 'http://polyfin.test/web/', provider, drawn: true },
  none: { address: 'http://polyfin.test/web/', drawn: true },
  french: { address: 'http://polyfin.test/web/', provider, drawn: true, lang: 'fr' },
  refused: { address: 'http://polyfin.test/web/?polyfinProvider=provider_no_account', provider, drawn: true },
  traded: { address: 'http://polyfin.test/web/?polyfinProvider=signed-in', provider, storage: saved, trade: signedIn },
  tradeRefused: { address: 'http://polyfin.test/web/?polyfinProvider=signed-in', provider, storage: saved,
    trade: { status: 401, body: { error: 'invalid_credentials' } } },
}
Promise.all(Object.entries(scenarios).map(async ([name, s]) => [name, await run(s)])).then((entries) => {
  process.stdout.write(JSON.stringify(Object.fromEntries(entries)))
})
`
	input, _ := json.Marshal(map[string]any{"script": string(webScriptBody)})
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	type traded struct {
		URL           string `json:"url"`
		Method        string `json:"method"`
		Authorization string `json:"authorization"`
	}
	var results map[string]struct {
		Address            string         `json:"address"`
		Buttons            []string       `json:"buttons"`
		Clicked            string         `json:"clicked"`
		Replaced           string         `json:"replaced"`
		Credentials        map[string]any `json:"credentials"`
		Traded             []traded       `json:"traded"`
		TradedAfterDrawing bool           `json:"tradedAfterDrawing"`
	}
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	start := "https://media.example.org/admin/api/provider/start?app=web"
	offered := []string{"polyfinProviderButton: Sign in with Example ID", "btnManual", "btnQuick"}
	if got := results["offered"]; !reflect.DeepEqual(got.Buttons, offered) || got.Clicked != start || len(got.Traded) != 0 {
		t.Errorf("offered: %+v", got)
	}
	if got := results["none"]; !reflect.DeepEqual(got.Buttons, []string{"btnManual", "btnQuick"}) {
		t.Errorf("no provider: %+v", got)
	}
	if got := results["french"]; len(got.Buttons) == 0 || got.Buttons[0] != "polyfinProviderButton: Se connecter avec Example ID" {
		t.Errorf("in French: %+v", got)
	}
	refused := []string{"polyfinProviderButton: Sign in with Example ID",
		"polyfinProviderMessage: No account on this server matches your account at the identity provider.", "btnManual", "btnQuick"}
	if got := results["refused"]; !reflect.DeepEqual(got.Buttons, refused) || got.Address != "/web/#/login" || len(got.Traded) != 0 || got.Replaced != "" {
		t.Errorf("refused: %+v", got)
	}
	got := results["traded"]
	wantTrade := []traded{{URL: "/admin/api/provider/web", Method: "POST",
		Authorization: `MediaBrowser Client="Jellyfin%20Web", Device="Firefox", DeviceId="device-1", Version="12.4.0"`}}
	wantCredentials := map[string]any{"Servers": []any{
		map[string]any{"Id": "server-1", "Name": "Living room", "ManualAddress": "http://polyfin.test", "LocalAddress": "http://10.0.0.2:8096",
			"UserId": "user-1", "AccessToken": "token-1"},
		map[string]any{"Id": "other", "AccessToken": "other-token"},
	}}
	if servers, ok := got.Credentials["Servers"].([]any); ok && len(servers) > 0 {
		if first, ok := servers[0].(map[string]any); ok {
			if _, dated := first["DateLastAccessed"].(float64); !dated {
				t.Errorf("traded: no DateLastAccessed in %v", first)
			}
			delete(first, "DateLastAccessed")
		}
	}
	if !reflect.DeepEqual(got.Traded, wantTrade) || !got.TradedAfterDrawing || got.Replaced != "/web/" ||
		!reflect.DeepEqual(got.Credentials, wantCredentials) || got.Address != "/web/#/login" {
		t.Errorf("traded: %+v", got)
	}
	got = results["tradeRefused"]
	failed := []string{"polyfinProviderButton: Sign in with Example ID",
		"polyfinProviderMessage: The sign-in with the identity provider failed. Try again.", "btnManual", "btnQuick"}
	if len(got.Traded) != 1 || got.Replaced != "" || !reflect.DeepEqual(got.Buttons, failed) ||
		got.Credentials["Servers"].([]any)[1].(map[string]any)["AccessToken"] != nil {
		t.Errorf("trade refused: %+v", got)
	}
}
