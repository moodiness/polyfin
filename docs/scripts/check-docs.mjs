// Checks the documentation against the style guide (CONTRIBUTING.md, "Writing the documentation"):
// - anchors: every link to a Markdown file's #anchor resolves with GitHub's heading slugs and with
//   VitePress's, an <a id="..."> alias counting for both;
// - wording: no em or en dashes, never "film", "remux" only in UI or code quotes (RemuxDB aside),
//   never "debrid", and no `Settings →`;
// - settings paths: each **Settings › X** names a section of the admin app's settings, unless the
//   words "web player" just before it make it jellyfin-web's own Settings.
// Usage: node docs/scripts/check-docs.mjs (from any folder). Exits 1 when something fails.
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoDir = resolve(fileURLToPath(new URL('../..', import.meta.url)))
const docsDir = resolve(repoDir, 'docs')
const files = [
  resolve(repoDir, 'README.md'),
  resolve(repoDir, 'CONTRIBUTING.md'),
  ...readdirSync(docsDir)
    .filter((name) => name.endsWith('.md'))
    .sort()
    .map((name) => resolve(docsDir, name)),
]
const problems = []
const report = (file, line, message) =>
  problems.push(`${relative(repoDir, file)}:${line}: ${message}`)

/** A file's lines, with fenced code blocks blanked so that line numbers stay right. */
const sources = new Map()
function linesOf(file) {
  if (!sources.has(file)) {
    let fence = null
    const lines = readFileSync(file, 'utf8')
      .split('\n')
      .map((line) => {
        const marker = /^ {0,3}(`{3,}|~{3,})/.exec(line)?.[1]
        if (fence) {
          if (marker && marker[0] === fence[0] && marker.length >= fence.length) fence = null
          return ''
        }
        if (marker) {
          fence = marker
          return ''
        }
        return line
      })
    sources.set(file, lines)
  }
  return sources.get(file)
}

const entities = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: '\u00a0' }

/** A heading's text as both renderers see it: no markup, code spans and link texts kept. */
function headingText(source) {
  return source
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]+>/g, '')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/(\*\*|__|\*|\b_|_\b)/g, '')
    .replace(/&(\w+);/g, (match, name) => entities[name] ?? match)
    .replace(/&#(\d+);/g, (_, code) => String.fromCodePoint(Number(code)))
    .trim()
}

/** GitHub's heading ids (github-slugger): punctuation and symbols dropped, each space a "-". */
const githubSlug = (text) =>
  text
    .toLowerCase()
    .replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, '')
    .replace(/ /g, '-')

/** VitePress's heading ids (@mdit-vue/shared slugify): each run of punctuation one "-". */
const vitepressSlug = (text) =>
  text
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u0000-\u001f]/g, '')
    .replace(/[\s~`!@#$%^&*()\-_+=[\]{}|\\;:"'“”‘’<>,.?/]+/g, '-')
    .replace(/-{2,}/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/^(\d)/, '_$1')
    .toLowerCase()

/** The anchors of a file in each style, its <a id> aliases in both. */
const anchorCache = new Map()
function anchorsOf(file) {
  if (!anchorCache.has(file)) {
    const github = new Set()
    const vitepress = new Set()
    const unique = (set, slug) => {
      let id = slug
      for (let n = 1; set.has(id); n++) id = `${slug}-${n}`
      set.add(id)
    }
    for (const line of linesOf(file)) {
      const heading = /^ {0,3}#{1,6}\s+(.*?)(?:\s+#+)?\s*$/.exec(line)
      if (heading) {
        const text = headingText(heading[1])
        unique(github, githubSlug(text))
        unique(vitepress, vitepressSlug(text))
      }
      for (const alias of line.matchAll(/<a\s[^>]*?\b(?:id|name)="([^"]+)"/g)) {
        github.add(alias[1])
        vitepress.add(alias[1])
      }
    }
    anchorCache.set(file, { github, vitepress })
  }
  return anchorCache.get(file)
}

/** Which renderers show a file: pages under docs/ are on both, docs/index.md only on the site. */
function stylesFor(file) {
  if (file === resolve(docsDir, 'index.md')) return ['vitepress']
  if (relative(docsDir, file).startsWith('..')) return ['github']
  return ['github', 'vitepress']
}

const styleNames = { github: 'GitHub', vitepress: 'the website' }

function checkLinks(file, lines) {
  lines.forEach((source, index) => {
    const line = source.replace(/`[^`]*`/g, '')
    const targets = [
      ...[...line.matchAll(/\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)/g)].map((m) => m[1]),
      ...[...line.matchAll(/^\s*\[[^\]]+\]:\s*<?(\S+?)>?(?:\s|$)/g)].map((m) => m[1]),
      ...[...line.matchAll(/<a\s[^>]*?\bhref="([^"]+)"/g)].map((m) => m[1]),
    ]
    for (const target of targets) {
      if (/^(?:[a-z][\w+.-]*:|\/)/i.test(target)) continue
      const hashAt = target.indexOf('#')
      const path = hashAt < 0 ? target : target.slice(0, hashAt)
      const anchor = hashAt < 0 ? '' : decodeURIComponent(target.slice(hashAt + 1))
      const destination = path ? resolve(dirname(file), decodeURIComponent(path)) : file
      if (!existsSync(destination)) {
        report(file, index + 1, `${target}: no such file`)
        continue
      }
      if (!anchor || !destination.endsWith('.md')) continue
      const anchors = anchorsOf(destination)
      const missing = stylesFor(destination).filter((style) => !anchors[style].has(anchor))
      if (missing.length > 0) {
        const where = missing.map((style) => styleNames[style]).join(' and ')
        report(file, index + 1, `${target}: no such anchor on ${where}`)
      }
    }
  })
}

const wordingRules = [
  { pattern: /[\u2014\u2013]/, message: 'an em or en dash: use a colon, a comma or two sentences' },
  { pattern: /\bfilms?\b/i, message: '"film": write "movie"' },
  { pattern: /\bdebrid/i, message: '"debrid": write "provider", or leave it out' },
  { pattern: /\bSettings\s*(?:→|->)/, message: '"Settings →": write **Settings › Section**' },
]

/** The admin app's settings sections, as its navigation names them. */
function settingsSections() {
  const nav = readFileSync(resolve(repoDir, 'web/src/i18n/en/nav.ts'), 'utf8')
  const block = /settingsSections:\s*\{([^}]*)\}/.exec(nav)?.[1]
  const names = [...(block ?? '').matchAll(/:\s*'([^']+)'/g)].map((m) => m[1])
  if (names.length === 0) throw new Error('web/src/i18n/en/nav.ts lists no settingsSections')
  return new Set(names)
}

function checkWording(file, lines, sections) {
  lines.forEach((source, index) => {
    // Prose only: no code spans, link targets, raw HTML or comments.
    const prose = source
      .replace(/`[^`]*`/g, '``')
      .replace(/\]\([^)]*\)/g, ']()')
      .replace(/<!--.*?-->|<[^>]+>/g, '')
      .replace(/https?:\/\/\S+/g, '')
    for (const { pattern, message } of wordingRules) {
      if (pattern.test(prose)) report(file, index + 1, message)
    }
    // UI labels are quoted in bold or in quotation marks, and may say Remux.
    const unquoted = prose.replace(/\*\*[^*]+\*\*|"[^"]*"|“[^”]*”/g, '').replace(/RemuxDB/g, '')
    if (/remux/i.test(unquoted)) {
      report(file, index + 1, '"remux": write "repackage", unless quoting the UI')
    }
    // "web player" or "jellyfin-web" just before a path, and after any other path, make it
    // jellyfin-web's own Settings.
    let after = 0
    for (const path of prose.matchAll(/\*\*Settings › ([^*›]+?)\s*(?:›[^*]*)?\*\*/g)) {
      const before = prose.slice(Math.max(after, path.index - 60), path.index)
      after = path.index + path[0].length
      if (sections.has(path[1]) || /web player|jellyfin-web/i.test(before)) continue
      report(
        file,
        index + 1,
        `**Settings › ${path[1]}** is no section of the admin app's settings (web/src/i18n/en/nav.ts); ` +
          `for the web player's own, write "the web player's **Settings › ${path[1]}**"`,
      )
    }
  })
}

const sections = settingsSections()
for (const file of files) {
  const lines = linesOf(file)
  checkLinks(file, lines)
  checkWording(file, lines, sections)
}

if (problems.length > 0) {
  console.error(problems.join('\n'))
  console.error(`\n${problems.length} problems in the documentation. See CONTRIBUTING.md.`)
  process.exit(1)
}
console.log(`${files.length} files checked: anchors, wording and settings paths are fine.`)
