// What the anchor scripts share: how GitHub and VitePress see a page's anchors, the anchor
// baseline, and the redirect map with its rules. See CONTRIBUTING.md, "Old anchors and redirects".
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const repoDir = resolve(fileURLToPath(new URL('../..', import.meta.url)))
export const docsDir = resolve(repoDir, 'docs')
/** Every anchor a docs page has ever had, by page: `{ pages: { users: { anchor: title } } }`. */
export const baselineFile = resolve(docsDir, '.vitepress/anchors-baseline.json')
/** Where moved anchors went: `{ "old-page#anchor": "new-page#anchor" }`, read by the theme. */
export const redirectsFile = resolve(docsDir, '.vitepress/redirects.json')
/** The pages split from each old page, and the moves the generator cannot guess. */
export const rulesFile = resolve(docsDir, 'scripts/redirect-rules.json')
/** The anchors cited outside the repository (issues, releases, discussions): they get stubs. */
export const citedFile = resolve(docsDir, 'scripts/cited-anchors.json')

/** The class of the block that keeps a moved section's old anchors on its old page for GitHub. */
export const stubClass = 'moved-sections'

export const readJson = (file, fallback) =>
  existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : fallback
export const writeJson = (file, value) => writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`)

/** A Markdown text's lines, with fenced code blocks blanked so that line numbers stay right. */
export function blankFences(text) {
  let fence = null
  return text.split('\n').map((line) => {
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
}

const entities = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: '\u00a0' }

/** A heading's text as both renderers see it: no markup, code spans and link texts kept. */
export function headingText(source) {
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
export const githubSlug = (text) =>
  text
    .toLowerCase()
    .replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, '')
    .replace(/ /g, '-')

/** VitePress's heading ids (@mdit-vue/shared slugify): each run of punctuation one "-". */
export const vitepressSlug = (text) =>
  text
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u0000-\u001f]/g, '')
    .replace(/[\s~`!@#$%^&*()\-_+=[\]{}|\\;:"'“”‘’<>,.?/]+/g, '-')
    .replace(/-{2,}/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/^(\d)/, '_$1')
    .toLowerCase()

/**
 * The anchors of a page's lines (fences blanked):
 * - `github` and `vitepress`: its heading ids in each style, its <a id> aliases in both;
 * - `stubs`: the aliases inside a moved-sections block, kept for GitHub only, in neither set;
 * - `titles`: each anchor's heading text (an alias takes the heading it stands above);
 * - `headingOf`: each alias standing above a heading, with that heading's ids in each style.
 * Both renderers number repeated heading slugs (-1, -2) among headings only, aliases aside.
 */
export function anchorsOfLines(lines) {
  const github = new Set()
  const vitepress = new Set()
  const stubs = new Set()
  const titles = new Map()
  const headingOf = new Map()
  const headings = { github: new Set(), vitepress: new Set() }
  const unique = (style, slug) => {
    let id = slug
    for (let n = 1; headings[style].has(id); n++) id = `${slug}-${n}`
    headings[style].add(id)
    return id
  }
  let inStub = false
  let pending = []
  for (const line of lines) {
    if (new RegExp(`^\\s*<div\\s+class="${stubClass}"`).test(line)) inStub = true
    const heading = /^ {0,3}#{1,6}\s+(.*?)(?:\s+#+)?\s*$/.exec(line)
    if (heading) {
      const text = headingText(heading[1])
      const ids = {
        github: unique('github', githubSlug(text)),
        vitepress: unique('vitepress', vitepressSlug(text)),
      }
      github.add(ids.github)
      vitepress.add(ids.vitepress)
      for (const id of [ids.github, ids.vitepress]) if (!titles.has(id)) titles.set(id, text)
      for (const id of pending) {
        if (!titles.has(id)) titles.set(id, text)
        headingOf.set(id, ids)
      }
      pending = []
    }
    const aliases = [...line.matchAll(/<a\s[^>]*?\b(?:id|name)="([^"]+)"/g)].map((m) => m[1])
    for (const id of aliases) {
      if (inStub) {
        stubs.add(id)
      } else {
        github.add(id)
        vitepress.add(id)
        pending.push(id)
      }
    }
    // An alias names the heading right below it; any other text in between ends the wait.
    if (!heading && aliases.length === 0 && line.trim() !== '') pending = []
    if (inStub && /<\/div>/.test(line)) inStub = false
  }
  return { github, vitepress, stubs, titles, headingOf }
}

/**
 * The anchor to link to for `anchor` on a page: the heading an alias stands above, when that
 * heading's id is the same on GitHub and on the website; else the anchor itself.
 */
export function canonicalAnchor(anchors, anchor) {
  const ids = anchors.headingOf.get(anchor)
  return ids && ids.github === ids.vitepress ? ids.github : anchor
}

export const anchorsOfText = (text) => anchorsOfLines(blankFences(text))

/** Every anchor of an anchor set, in either style. */
export const allOf = (anchors) => new Set([...anchors.github, ...anchors.vitepress])

/** The docs pages of the working tree, by name (users for docs/users.md), with their anchors. */
export function currentPages() {
  const pages = new Map()
  for (const name of readdirSync(docsDir).sort()) {
    if (!name.endsWith('.md')) continue
    const text = readFileSync(resolve(docsDir, name), 'utf8')
    pages.set(name.slice(0, -3), { text, anchors: anchorsOfText(text) })
  }
  return pages
}

/** A page's title: its first H1. */
export const titleOf = (text) =>
  headingText(/^#\s+(.+)$/m.exec(blankFences(text).join('\n'))?.[1] ?? '')

/**
 * Where a baseline anchor of `page` lives now, as `{ status, targets }`:
 * - `here`: still on its page (not as a stub);
 * - `mapped`: the redirect map or an override sends it to `targets[0]`;
 * - `moved`: on one page, found among the pages split from its page first, then anywhere;
 * - `ambiguous`: on several pages (`targets`), so it needs an override;
 * - `lost`: nowhere.
 * A page whose baseline already had the anchor holds a section of its own: it is no candidate.
 */
export function locate(page, anchor, { pages, baseline, rules, redirects }) {
  const key = `${page}#${anchor}`
  if (pages.get(page) && allOf(pages.get(page).anchors).has(anchor)) {
    return { status: 'here', targets: [key] }
  }
  for (const target of [rules.overrides?.[key], redirects?.[key]]) {
    if (target && resolves(target, pages)) return { status: 'mapped', targets: [target] }
  }
  const candidates = (names) =>
    names.filter(
      (name) =>
        name !== page &&
        pages.has(name) &&
        !(anchor in (baseline.pages[name] ?? {})) &&
        allOf(pages.get(name).anchors).has(anchor),
    )
  const family = candidates(rules.splits?.[page] ?? [])
  const found = family.length > 0 ? family : candidates([...pages.keys()])
  if (found.length === 1) {
    const target = canonicalAnchor(pages.get(found[0]).anchors, anchor)
    return { status: 'moved', targets: [`${found[0]}#${target}`] }
  }
  if (found.length > 1) return { status: 'ambiguous', targets: found.map((n) => `${n}#${anchor}`) }
  return { status: 'lost', targets: [] }
}

/** Whether `page#anchor` exists in the working tree (stubs excluded). */
export function resolves(target, pages) {
  const at = target.indexOf('#')
  const page = pages.get(at < 0 ? target : target.slice(0, at))
  return Boolean(page) && (at < 0 || allOf(page.anchors).has(target.slice(at + 1)))
}

export function loadState() {
  return {
    pages: currentPages(),
    baseline: readJson(baselineFile, { pages: {} }),
    rules: readJson(rulesFile, { splits: {}, overrides: {} }),
    redirects: readJson(redirectsFile, {}),
  }
}

/**
 * The problems of the anchor baseline and the redirect map, for `npm run check`:
 * - a baseline anchor neither on its page nor redirected to an anchor that exists;
 * - a redirect from an anchor still on its page, whose section the redirect would hide;
 * - a redirect to an anchor that does not exist.
 */
export function anchorProblems(state = loadState()) {
  const { pages, baseline, redirects } = state
  const problems = []
  for (const [page, anchors] of Object.entries(baseline.pages)) {
    for (const anchor of Object.keys(anchors)) {
      const key = `${page}#${anchor}`
      const here = pages.get(page) && allOf(pages.get(page).anchors).has(anchor)
      if (here && redirects[key]) {
        problems.push(`${key}: on its page, yet .vitepress/redirects.json sends it elsewhere`)
      } else if (!here && !redirects[key]) {
        problems.push(
          `${key}: no such anchor any more, and no redirect: alias it (<a id="${anchor}"></a>) ` +
            'or run npm run redirects',
        )
      }
    }
  }
  for (const [key, target] of Object.entries(redirects)) {
    if (!resolves(target, pages))
      problems.push(`${key}: redirects to ${target}, which does not exist`)
  }
  return problems
}
