// The anchor tools. Every anchor a docs page ever had must keep working: on its page, or through
// the redirect map on the website and a stub on GitHub. See CONTRIBUTING.md, "Old anchors and
// redirects". Usage, from any folder:
//   node docs/scripts/anchors.mjs lost [page...]   the baseline anchors of the pages (all when
//                                                  none is named) that resolve nowhere, or on
//                                                  several pages; exits 1 when there are some
//   node docs/scripts/anchors.mjs add              adds every current anchor to the baseline
//   node docs/scripts/anchors.mjs redirects        writes .vitepress/redirects.json
//   node docs/scripts/anchors.mjs relink           rewrites the links to moved anchors and aliases
//   node docs/scripts/anchors.mjs stubs            keeps cited moved anchors on their old page
//   node docs/scripts/anchors.mjs baseline <commit>  creates the baseline from a commit, once
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, relative, resolve, sep } from 'node:path'
import {
  anchorsOfText,
  canonicalAnchor,
  baselineFile,
  citedFile,
  docsDir,
  loadState,
  locate,
  readJson,
  redirectsFile,
  repoDir,
  stubClass,
  titleOf,
  writeJson,
} from './anchors-lib.mjs'

const [command, ...args] = process.argv.slice(2)
const commands = { baseline, add, lost, redirects, relink, stubs }
if (!commands[command]) {
  console.error(readFileSync(new URL(import.meta.url), 'utf8').split('\nimport')[0])
  process.exit(2)
}
commands[command](args)

/** A page's anchors as baseline entries: every anchor, with its heading's text. */
function entriesOf(anchors) {
  const entries = {}
  for (const id of [...anchors.github, ...anchors.vitepress]) {
    entries[id] = anchors.titles.get(id) ?? null
  }
  return entries
}

function sorted(entries) {
  return Object.fromEntries(Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)))
}

function baseline([commit]) {
  if (!commit) throw new Error('usage: anchors.mjs baseline <commit>')
  if (existsSync(baselineFile)) {
    throw new Error(`${relative(repoDir, baselineFile)} exists: the baseline only grows (add)`)
  }
  const git = (...gitArgs) => execFileSync('git', gitArgs, { cwd: repoDir, encoding: 'utf8' })
  const pages = {}
  for (const path of git('ls-tree', '--name-only', commit, 'docs/').split('\n')) {
    const name = /^docs\/([^/]+)\.md$/.exec(path)?.[1]
    if (name) pages[name] = sorted(entriesOf(anchorsOfText(git('show', `${commit}:${path}`))))
  }
  writeJson(baselineFile, { from: git('rev-parse', '--short', commit).trim(), pages })
  console.log(`${count(pages)} anchors of ${Object.keys(pages).length} pages at ${commit}.`)
}

function count(pages) {
  return Object.values(pages).reduce((sum, entries) => sum + Object.keys(entries).length, 0)
}

function add() {
  const { pages } = loadState()
  const base = readJson(baselineFile, null)
  if (!base) throw new Error(`no ${relative(repoDir, baselineFile)}: create it with baseline`)
  const before = count(base.pages)
  for (const [name, page] of pages) {
    base.pages[name] = sorted({ ...entriesOf(page.anchors), ...base.pages[name] })
  }
  writeJson(baselineFile, base)
  console.log(`${count(base.pages) - before} anchors added to the baseline.`)
}

function lost(names) {
  const state = loadState()
  const unknown = names.map((n) => n.replace(/\.md$/, '')).filter((n) => !state.baseline.pages[n])
  if (unknown.length > 0) throw new Error(`not in the baseline: ${unknown.join(', ')}`)
  let bad = 0
  for (const page of names.length > 0
    ? names.map((n) => n.replace(/\.md$/, ''))
    : Object.keys(state.baseline.pages)) {
    for (const [anchor, title] of Object.entries(state.baseline.pages[page])) {
      const { status, targets } = locate(page, anchor, state)
      if (status === 'lost') {
        console.log(`${page}#${anchor}  (${title ?? 'alias'}): lost, on no page`)
      } else if (status === 'ambiguous') {
        console.log(
          `${page}#${anchor}  (${title ?? 'alias'}): on several pages, ${targets.join(', ')}; ` +
            'keep it on one, or ask MedCore for an override',
        )
      } else continue
      bad++
    }
  }
  if (bad > 0) process.exit(1)
  console.log('Every baseline anchor resolves.')
}

/** The map: every baseline anchor that left its page, to where it is now. Existing entries stay. */
function redirects() {
  const state = loadState()
  const map = {}
  const problems = []
  for (const [page, entries] of Object.entries(state.baseline.pages)) {
    for (const anchor of Object.keys(entries)) {
      const { status, targets } = locate(page, anchor, state)
      if (status === 'mapped' || status === 'moved') map[`${page}#${anchor}`] = targets[0]
      else if (status !== 'here')
        problems.push(`${page}#${anchor}: ${status} ${targets.join(', ')}`)
    }
  }
  writeJson(redirectsFile, sorted(map))
  console.log(
    `${Object.keys(map).length} redirects written to ${relative(repoDir, redirectsFile)}.`,
  )
  if (problems.length > 0) {
    console.error(
      `${problems.join('\n')}\n${problems.length} anchors unresolved: alias them, or add an ` +
        `override to ${relative(repoDir, 'scripts/redirect-rules.json')}.`,
    )
    process.exit(1)
  }
}

/** The files whose links follow the map: the docs, the root's Markdown and the theme. */
function linkFiles() {
  const { pages } = loadState()
  return [
    resolve(repoDir, 'README.md'),
    resolve(repoDir, 'CONTRIBUTING.md'),
    ...[...pages.keys()].map((name) => resolve(docsDir, `${name}.md`)),
    resolve(docsDir, '.vitepress/config.mts'),
    ...['home.ts', 'Home.vue', 'FeatureViewer.vue', 'TourScreen.vue', 'NavInstall.vue']
      .map((name) => resolve(docsDir, '.vitepress/theme', name))
      .filter(existsSync),
  ]
}

/**
 * Rewrites `x.md#a` (and the theme's `/docs/x#a`) to where the map sends `x#a`, and a link to an
 * alias to the heading it stands above (canonicalAnchor): aliases are for old addresses only.
 */
function relink() {
  const { pages, redirects: map } = loadState()
  const newAddress = (page, anchor) => {
    if (map[`${page}#${anchor}`]) return map[`${page}#${anchor}`]
    const canonical = pages.has(page) && canonicalAnchor(pages.get(page).anchors, anchor)
    return canonical && canonical !== anchor ? `${page}#${canonical}` : null
  }
  let changed = 0
  let links = 0
  for (const file of linkFiles()) {
    const source = readFileSync(file, 'utf8')
    const pageOf = (path) => {
      const target = path ? resolve(dirname(file), decodeURIComponent(path)) : file
      const fromDocs = relative(docsDir, target)
      return fromDocs.includes(sep) || !fromDocs.endsWith('.md') ? null : fromDocs.slice(0, -3)
    }
    const hrefTo = (target) => {
      const [page, anchor] = target.split('#')
      const destination = resolve(docsDir, `${page}.md`)
      const path = relative(dirname(file), destination).split(sep).join('/')
      return `${destination === file ? '' : path}#${anchor}`
    }
    let text = source
    if (file.endsWith('.md')) {
      // Markdown links, reference definitions and <a href>, outside code.
      const lines = text.split('\n')
      let fence = null
      text = lines
        .map((line) => {
          const marker = /^ {0,3}(`{3,}|~{3,})/.exec(line)?.[1]
          if (fence || marker) {
            if (fence && marker && marker[0] === fence[0] && marker.length >= fence.length)
              fence = null
            else if (!fence && marker) fence = marker
            return line
          }
          if (new RegExp(`class="${stubClass}"`).test(line)) return line
          return line.replace(
            /(\]\(\s*<?|^\s*\[[^\]]+\]:\s*<?|<a\s[^>]*?\bhref=")([^)\s>"#]*)#([^)\s>"]+)/g,
            (whole, lead, path, anchor) => {
              if (/^(?:[a-z][\w+.-]*:|\/)/i.test(path)) return whole
              const page = pageOf(path)
              const target = page && newAddress(page, decodeURIComponent(anchor))
              if (!target) return whole
              links++
              return `${lead}${hrefTo(target)}`
            },
          )
        })
        .join('\n')
    } else {
      text = text.replace(/(['"`])\/docs\/([\w-]*)#([\w-]+)/g, (whole, quote, page, anchor) => {
        const target = newAddress(page || 'README', anchor)
        if (!target) return whole
        links++
        const [newPage, newAnchor] = target.split('#')
        return `${quote}/docs/${newPage === 'README' ? '' : newPage}#${newAnchor}`
      })
    }
    if (text !== source) {
      writeFileSync(file, text)
      changed++
      console.log(`relinked ${relative(repoDir, file)}`)
    }
  }
  console.log(`${links} links in ${changed} files relinked.`)
}

/**
 * For each cited anchor that moved, a line in a block at the end of its old page: GitHub shows
 * the block and keeps its <a id>, the website hides it (style.css) and redirects instead.
 */
function stubs() {
  const state = loadState()
  const map = state.redirects
  const cited = readJson(citedFile, {})
  let written = 0
  for (const [page, anchors] of Object.entries(cited)) {
    const file = resolve(docsDir, `${page}.md`)
    if (!existsSync(file)) continue
    // One line per new place, with every old anchor that leads there.
    const lines = new Map()
    for (const anchor of anchors) {
      const target = map[`${page}#${anchor}`]
      if (!target) continue
      if (!lines.has(target)) lines.set(target, [])
      lines.get(target).push(anchor)
    }
    const source = readFileSync(file, 'utf8')
    const block = new RegExp(`\\n*<div class="${stubClass}">[\\s\\S]*?</div>\\n*$`)
    let text = source.replace(block, '\n')
    if (lines.size > 0) {
      const body = [...lines]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([target, ids]) => {
          const [newPage, newAnchor] = target.split('#')
          const title = state.baseline.pages[page][ids[0]] ?? ids[0]
          const pageTitle = titleOf(state.pages.get(newPage).text)
          const href = `${newPage === page ? '' : `${newPage}.md`}#${newAnchor}`
          const ids_ = ids.map((id) => `<a id="${id}"></a>`).join('')
          return `${ids_}*${title}* moved to [${pageTitle}](${href}).`
        })
      text = `${text.replace(/\n*$/, '')}\n\n<div class="${stubClass}">\n\n${body.join('\n\n')}\n\n</div>\n`
    }
    if (text !== source) {
      writeFileSync(file, text)
      written++
      console.log(`${relative(repoDir, file)}: ${lines.size} moved sections`)
    }
  }
  console.log(`${written} pages given stubs.`)
}
