package webtheme

import (
	"encoding/json"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// studioLogosPage runs Polyfin's studio logos extension, read from the
// standard input, in Node.js, on a title's page whose Studios lines are
// made by line(names): a .detailsGroupItem holding a link per studio. The
// page logs each logo the extension adds and each it takes back; tick()
// runs its clock, and the steps' log is printed at the end as JSON.
const studioLogosPage = `
const vm = require('node:vm')
const script = require('node:fs').readFileSync(0, 'utf8')
const log = []
class Node {
  constructor(tag) {
    this.tagName = tag; this.children = []; this.parentElement = null; this.attributes = {}; this.textContent = ''
    this.style = { getPropertyValue: () => '', setProperty() {}, removeProperty() {} }
  }
  get isConnected() { return true }
  append(...nodes) {
    for (const node of nodes) {
      node.parentElement = this; this.children.push(node)
      if (node.tagName === 'img') log.push('logo ' + this.textContent + ': ' + node.src)
    }
  }
  remove() {
    const parent = this.parentElement
    if (!parent) return
    parent.children = parent.children.filter(child => child !== this)
    if (this.tagName === 'img') log.push('name ' + parent.textContent)
  }
  setAttribute(name, value) { this.attributes[name] = String(value) } getAttribute(name) { return name in this.attributes ? this.attributes[name] : null }
  hasAttribute(name) { return name in this.attributes } removeAttribute(name) { delete this.attributes[name] }
  toggleAttribute(name, on) { if (on) this.attributes[name] = ''; else delete this.attributes[name] }
  addEventListener() {} removeEventListener() {}
  contains(node) { for (let at = node; at; at = at.parentElement) if (at === this) return true; return false }
  closest(selector) {
    if (selector !== '.detailsGroupItem') return null
    for (let at = this; at; at = at.parentElement) if (at.studios) return at
    return null
  }
  querySelectorAll() { return this.studios || [] }
  querySelector() { return (this.studios || [])[0] || null }
}
// The Studios lines, as jellyfin-web draws them: group > line > item > link.
const groups = []
const line = (...names) => {
  const group = new Node('div'), content = new Node('div')
  group.studios = []
  group.append(content)
  const add = name => {
    const item = new Node('span'), link = new Node('a')
    link.setAttribute('href', '#/list?studioId=' + name.toLowerCase().replace(/ /g, '-'))
    link.textContent = name
    item.append(link); content.append(item); group.studios.push(link)
  }
  names.forEach(add)
  groups.push(group)
  return add
}
const clocks = []
let compact = false
const document = {
  documentElement: new Node('html'), head: new Node('head'), createElement: tag => new Node(tag),
  querySelectorAll: () => groups.flatMap(group => group.studios), addEventListener() {}, removeEventListener() {},
}
document.documentElement.classList = { contains: () => false }
const context = {
  document, location: { href: 'http://polyfin.test/web/#/details?id=title' }, URL, URLSearchParams, console,
  getComputedStyle: () => ({ getPropertyValue: name => name === '--lg-motion' ? ' 1' : '', fontSize: '16px' }),
  matchMedia: () => ({ get matches() { return compact } }),
  MutationObserver: class { observe() {} disconnect() {} },
  requestAnimationFrame: () => 0, cancelAnimationFrame() {},
  setInterval(fn) { clocks.push(fn); return clocks.length }, clearInterval() {},
  addEventListener() {}, removeEventListener() {}, devicePixelRatio: 2,
  ApiClient: { getUrl: (path, query) => path + '?' + new URLSearchParams(query) },
}
context.window = context
const start = () => vm.runInNewContext(script, context)
const tick = () => { for (const fn of clocks) fn() }
`

// runStudioLogos runs the studio logos extension in studioLogosPage, then
// steps, and returns what they logged.
func runStudioLogos(t *testing.T, steps string) []string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	script, err := fs.ReadFile(Files, "assets/extensions/studio-logos.js")
	if err != nil {
		t.Fatal(err)
	}
	harness := studioLogosPage + "\ntry {\n" + steps + "\n} catch (error) {\n  log.push('harness: ' + error.stack)\n}\nprocess.stdout.write(JSON.stringify(log))\n"
	command := exec.CommandContext(t.Context(), node, "-e", harness)
	command.Stdin = strings.NewReader(string(script))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var got []string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("node printed %q: %v", output, err)
	}
	return got
}

// A title's Studios line shows the logos of up to three studios, 40 pixels
// tall, 32 on a phone, asked at the screen's density. Past three, it keeps
// their names, links to the same pages, and asks for no logo; a line that
// grows past three gives its logos back (patches/0004-studio-logos.patch in
// third_party/lumaaglaass).
func TestStudioLogosAreFewAndSmall(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		steps string
		want  []string
	}{
		"three studios, then a fourth": {
			steps: `
const grow = line('North Studio', 'Lantern Pictures', 'Quiet Hill Media')
start()
grow('Harbor Network')
tick()
`,
			want: []string{
				"logo North Studio: Items/north-studio/Images/Thumb?maxHeight=80&margin=narrow",
				"logo Lantern Pictures: Items/lantern-pictures/Images/Thumb?maxHeight=80&margin=narrow",
				"logo Quiet Hill Media: Items/quiet-hill-media/Images/Thumb?maxHeight=80&margin=narrow",
				"name North Studio",
				"name Lantern Pictures",
				"name Quiet Hill Media",
			},
		},
		"four studios": {
			steps: `
line('North Studio', 'Lantern Pictures', 'Quiet Hill Media', 'Harbor Network')
start()
tick()
`,
			want: nil,
		},
		"a phone": {
			steps: `
compact = true
line('North Studio')
start()
`,
			want: []string{"logo North Studio: Items/north-studio/Images/Thumb?maxHeight=64&margin=narrow"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := runStudioLogos(t, test.steps); !slices.Equal(got, test.want) {
				t.Errorf("\n got  %q\n want %q", got, test.want)
			}
		})
	}
}
