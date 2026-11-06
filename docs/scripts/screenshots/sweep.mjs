// Sweeps the admin app on the demo server of stage.mjs: every route of web/src/app/router.tsx, as
// the administrator, as a member and signed out, 1440 and 390 pixels wide, in English and German.
// It fails with a list of what it found: a page that scrolls sideways or an element past its
// edge, text running out of its box, and a console error but those expected. See README.md.
import { mkdir, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { parseArgs } from 'node:util'
import { client, defaultChrome, log, members, repoDir, run, seed, startStage } from './stage.mjs'

const { values: options } = parseArgs({
  options: {
    polyfin: { type: 'string', default: process.env.POLYFIN_BIN ?? join(repoDir, 'bin/polyfin') },
    'database-url': { type: 'string', default: process.env.SCREENSHOTS_DATABASE_URL },
    database: { type: 'string', default: 'polyfin_sweep' },
    chrome: { type: 'string', default: process.env.CHROME_PATH ?? defaultChrome() },
    jobs: { type: 'string', default: '4' },
    shots: { type: 'string' },
    help: { type: 'boolean', default: false },
  },
})

if (options.help || !options['database-url']) {
  console.log(`Usage: node sweep.mjs --database-url <postgresql://user:password@host:port/postgres> [options]

  --database-url  A PostgreSQL server the tool may create and drop a database on
                  (or SCREENSHOTS_DATABASE_URL).
  --database      Name of the scratch database, dropped at the end (polyfin_sweep).
  --polyfin       Polyfin executable built with the admin app (bin/polyfin, or POLYFIN_BIN).
  --chrome        Chrome or Chromium executable (CHROME_PATH, else the usual install).
  --jobs          Pages swept at once (4).
  --shots         A folder to save a screenshot of each page with a finding to, its
                  elements outlined.`)
  process.exit(options.help ? 0 : 2)
}

const widths = [1440, 390]
const languages = ['en', 'de']
/** The member the sweep signs in as: Alex, whose tracking services are connected. */
const member = members[2]
/** The most addresses of one route the sweep visits, of those the pages link to. */
const perRoute = 40

async function main() {
  const started = Date.now()
  const routes = await readRoutes()
  const stage = await startStage({
    polyfin: options.polyfin,
    chrome: options.chrome,
    databaseUrl: options['database-url'],
    database: options.database,
    logName: 'polyfin-sweep.log',
    rich: true,
  })
  log('Seeding the demo')
  const seeded = await seed(stage)
  const memberApi = client(stage.server.url)
  await memberApi.signIn(member)
  const roles = [
    { name: 'administrator', session: stage.api.session },
    { name: 'member', session: memberApi.session },
    { name: 'signed out', session: null },
  ]
  // The addresses of the routes without a parameter, or with optional ones left out, and those
  // the pages cannot link to: an open invite, a title's versions, which a search opens, and an
  // unknown page. Signed out, also an invite that does not exist, whose 404 is expected: only
  // then, as each lookup counts against the address's sign-in attempts, which a dozen exhaust.
  const badInvite = '/invite/no-such-invite'
  const start = [
    ...routes
      .filter((route) => route.params.every((param) => param.endsWith('?')))
      .map((route) => route.example.replace(/\/:[^/]+\?/g, '')),
    `/invite/${seeded.invite}`,
    `/versions/${seeded.title}`,
    '/no-such-page',
  ]

  const sweep = new Sweep(stage, routes, badInvite)
  if (options.shots) await mkdir(options.shots, { recursive: true })

  // First each signed-in role at 1440 in English, following the links of each page to the routes
  // with a parameter; then every other case, on the addresses each found.
  log(`Sweeping ${routes.length} routes`)
  const [found, foundMember] = await Promise.all(
    roles.slice(0, 2).map((role) => sweep.visitAll(role, widths[0], languages[0], start, true)),
  )
  const reached = new Set([...found, ...foundMember].map((url) => sweep.route(url)?.path))
  const missed = routes.filter((route) => route.params.length > 0 && !reached.has(route.path))
  for (const route of missed) {
    sweep.add({
      role: 'administrator',
      width: widths[0],
      language: languages[0],
      url: `/${route.path}`,
      kind: 'route',
      detail: 'no page of the demo links to this route: give the demo something it shows',
    })
  }
  // Signed out, every address leads to the sign-in, but the invites: one of each route.
  const firstOf = new Map()
  for (const url of found)
    firstOf.set(sweep.route(url)?.path, firstOf.get(sweep.route(url)?.path) ?? url)
  const lists = {
    administrator: found,
    member: foundMember,
    'signed out': [...new Set([...start, badInvite, ...firstOf.values()])],
  }
  const cases = []
  for (const role of roles) {
    for (const width of widths) {
      for (const language of languages) {
        const first = width === widths[0] && language === languages[0]
        if (first && role.session) continue
        cases.push(() => sweep.visitAll(role, width, language, lists[role.name], false))
      }
    }
  }
  await pool(cases, Number(options.jobs))

  const minutes = ((Date.now() - started) / 60000).toFixed(1)
  log(`${sweep.visits} pages in ${minutes} minutes`)
  if (sweep.findings.length === 0) {
    log('No finding')
    return
  }
  console.log(`\n${sweep.findings.length} findings:\n${sweep.report()}`)
  process.exitCode = 1
}

/** Runs the tasks, `jobs` at once. */
async function pool(tasks, jobs) {
  const queue = [...tasks]
  await Promise.all(
    Array.from({ length: jobs }, async () => {
      while (queue.length > 0) await queue.shift()()
    }),
  )
}

/**
 * The routes of the admin app, read from its router: each route's full path, from the paths of
 * the routes it is nested in, its parameters, and an address for one without any. The redirects
 * of addresses from before « Nuit » (`moved`) and the development gallery are left out.
 */
async function readRoutes() {
  const source = await readFile(join(repoDir, 'web/src/app/router.tsx'), 'utf8')
  const start = source.indexOf('createBrowserRouter(')
  const end = source.indexOf('{ basename:', start)
  if (start < 0 || end < 0) throw new Error('router.tsx: createBrowserRouter( not found')
  // The route objects, nested by their braces; comments and JSX hold none. An index route has
  // the path of the one it is in.
  const code = source.slice(start, end).replace(/\/\/.*$/gm, '')
  const stack = []
  const paths = new Set()
  const token = /[{}]|path:\s*'([^']*)'|index:\s*true/g
  for (const match of code.matchAll(token)) {
    if (match[0] === '{') stack.push(null)
    else if (match[0] === '}') stack.pop()
    else {
      if (match[1] !== undefined) stack[stack.length - 1] = match[1]
      paths.add(stack.filter((path) => path).join('/'))
    }
  }
  const routes = [...paths].map((path) => {
    const segments = path.split('/')
    const params = segments.filter((segment) => segment.startsWith(':') || segment === '*')
    const pattern = segments
      .map((segment) =>
        segment === '*'
          ? '(?:/.*)?'
          : segment.startsWith(':')
            ? segment.endsWith('?')
              ? '(?:/[^/]+)?'
              : '/[^/]+'
            : `/${segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`,
      )
      .join('')
    return {
      path,
      params,
      example: `/${path}`,
      regex: new RegExp(`^${pattern || '/'}$`),
      // The most specific route wins, as in the router: static segments first.
      rank: segments.filter((segment) => !segment.startsWith(':') && segment !== '*').length,
    }
  })
  if (routes.length < 20 || !routes.some((route) => route.path === 'settings/:section')) {
    throw new Error(`router.tsx: only ${routes.length} routes read; has its shape changed?`)
  }
  return routes.sort((a, b) => b.rank - a.rank || a.params.length - b.params.length)
}

class Sweep {
  constructor(stage, routes, badInvite) {
    this.stage = stage
    this.routes = routes
    this.badInvite = badInvite
    this.findings = []
    this.visits = 0
  }

  route(url) {
    return this.routes.find((route) => route.regex.test(url))
  }

  add(finding) {
    this.findings.push(finding)
  }

  /**
   * Visits the addresses as the role, at the width and in the language. Following links, it adds
   * those of the routes with a parameter the pages link to, perRoute of each at most, and
   * returns every address visited.
   */
  async visitAll(role, width, language, urls, follow) {
    const context = await this.stage.browser.createBrowserContext()
    try {
      const page = await context.newPage()
      await page.setViewport({ width, height: width > 600 ? 900 : 844, deviceScaleFactor: 1 })
      await page.evaluateOnNewDocument((language) => {
        localStorage.setItem('polyfin.language', language)
        localStorage.setItem('polyfin.getStarted.dismissed', '1')
      }, language)
      if (role.session) {
        await page.setCookie({
          name: 'polyfin_session',
          value: role.session,
          domain: '127.0.0.1',
          path: '/admin',
          httpOnly: true,
          sameSite: 'Strict',
        })
      }
      const queue = [...urls]
      const seen = new Set(queue)
      const count = new Map()
      for (let index = 0; index < queue.length; index++) {
        const links = await this.visit(page, { role: role.name, width, language }, queue[index])
        if (!follow) continue
        for (const link of links) {
          const route = this.route(link)
          if (!route || route.params.length === 0 || seen.has(link)) continue
          if ((count.get(route.path) ?? 0) >= perRoute) continue
          count.set(route.path, (count.get(route.path) ?? 0) + 1)
          seen.add(link)
          queue.push(link)
        }
      }
      return queue
    } finally {
      await context.close()
    }
  }

  /** Visits one address, adds what it finds, and returns the addresses of the app it links to. */
  async visit(page, where, url) {
    this.visits++
    const errors = []
    const onConsole = (message) => {
      if (message.type() !== 'error') return
      errors.push({ text: message.text(), source: message.location()?.url ?? '' })
    }
    const onError = (error) => errors.push({ text: `Uncaught ${error.message}`, source: '' })
    page.on('console', onConsole)
    page.on('pageerror', onError)
    const pending = new Set()
    const onRequest = (request) => {
      if (!['eventsource', 'websocket'].includes(request.resourceType())) pending.add(request)
    }
    const onDone = (request) => pending.delete(request)
    page.on('request', onRequest)
    page.on('requestfinished', onDone)
    page.on('requestfailed', onDone)
    let found = []
    let links = []
    try {
      await page.goto(`${this.stage.server.url}/admin${url}`, { waitUntil: 'load', timeout: 30000 })
      // Settled: no request under way for half a second, 10 seconds at most.
      const deadline = Date.now() + 10000
      let quietSince = Date.now()
      while (Date.now() < deadline) {
        if (pending.size > 0) quietSince = Date.now()
        else if (Date.now() - quietSince >= 500) break
        await new Promise((resolve) => setTimeout(resolve, 50))
      }
      await page.evaluate(() => document.fonts.ready.then(() => undefined))
      await page.evaluate(
        () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
      )
      ;({ found, links } = await page.evaluate(inspect, where.language))
    } catch (error) {
      found = [{ kind: 'page', detail: `did not load: ${error.message}` }]
    } finally {
      page.off('console', onConsole)
      page.off('pageerror', onError)
      page.off('request', onRequest)
      page.off('requestfinished', onDone)
      page.off('requestfailed', onDone)
    }
    for (const error of errors) {
      if (this.expected(where, url, error)) continue
      found.push({
        kind: 'console',
        detail: `${error.text}${error.source ? ` (${error.source.replace(this.stage.server.url, '')})` : ''}`,
      })
    }
    for (const finding of found) this.add({ ...where, url, ...finding })
    if (found.length > 0 && options.shots) await this.shoot(page, where, url, found)
    return links
  }

  /**
   * The console errors expected: signed out, the 401 of the session check; and the 404 of an
   * invite that does not exist.
   */
  expected(where, url, error) {
    const source = error.source.replace(this.stage.server.url, '')
    if (
      where.role === 'signed out' &&
      source === '/admin/api/session' &&
      /\b401\b/.test(error.text)
    ) {
      return true
    }
    return (
      url === this.badInvite &&
      source.startsWith('/admin/api/invite/') &&
      /\b404\b/.test(error.text)
    )
  }

  async shoot(page, where, url, found) {
    const name =
      `${where.role}-${where.width}-${where.language}${url.replaceAll('/', '_')}`.replace(
        /[^a-zA-Z0-9_.-]/g,
        '-',
      )
    await page
      .evaluate(
        (paths) => {
          for (const path of paths) {
            const element = path && document.querySelector(path)
            if (element) element.style.outline = '2px solid #ff2d55'
          }
        },
        found.map((finding) => finding.path),
      )
      .catch(() => {})
    await page
      .screenshot({ path: join(options.shots, `${name}.png`), fullPage: true })
      .catch(() => {})
  }

  /** The findings, by page, then case. */
  report() {
    const byPage = new Map()
    for (const finding of this.findings) {
      const key = `${finding.url}  [${finding.role}, ${finding.width} px, ${finding.language}]`
      byPage.set(key, [...(byPage.get(key) ?? []), finding])
    }
    return [...byPage.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(
        ([key, findings]) =>
          `${key}\n${findings.map((finding) => `  - ${finding.kind}: ${finding.detail}`).join('\n')}`,
      )
      .join('\n')
  }
}

/**
 * Runs in the page: what overflows, and the app's addresses the page links to. An element is
 * named by its tag, its text and the heading of its section; `path` is a selector for the shot.
 */
function inspect(language) {
  const found = []
  const root = document.documentElement
  const width = root.clientWidth
  if (root.lang !== language) {
    found.push({ kind: 'page', detail: `shown in ${root.lang || 'no language'}, not ${language}` })
  }
  const pathOf = (element) => {
    const parts = []
    for (let node = element; node && node !== document.body; node = node.parentElement) {
      const index = [...node.parentElement.children].indexOf(node) + 1
      parts.unshift(`${node.tagName.toLowerCase()}:nth-child(${index})`)
    }
    return `body > ${parts.join(' > ')}`
  }
  const nameOf = (element) => {
    const text = (element.innerText || element.getAttribute('aria-label') || '')
      .trim()
      .replace(/\s+/g, ' ')
    const section = element.closest('section, dialog, header, nav, aside')
    const heading = section?.querySelector('h1, h2, h3')?.textContent.trim()
    const tag = element.tagName.toLowerCase()
    return `<${tag}>${text ? ` "${text.length > 60 ? `${text.slice(0, 57)}…` : text}"` : ''}${heading ? ` in « ${heading} »` : ''}`
  }
  // Seen: neither hidden nor left to screen readers (a box of a pixel, as `sr-only` makes).
  const unseen = new Map()
  const forReaders = (element) => {
    if (!element || element === document.body) return false
    if (!unseen.has(element)) {
      const box = element.getBoundingClientRect()
      const absolute = getComputedStyle(element).position === 'absolute'
      unseen.set(
        element,
        (absolute && box.width <= 1 && box.height <= 1) || forReaders(element.parentElement),
      )
    }
    return unseen.get(element)
  }
  const shown = (element) =>
    element.checkVisibility({ opacityProperty: true, visibilityProperty: true }) &&
    !element.closest('[inert]') &&
    !forReaders(element)
  const clips = (style) =>
    ['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowX) ||
    ['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowY)

  if (root.scrollWidth > width + 1) {
    found.push({
      kind: 'overflow',
      detail: `the page scrolls sideways: ${root.scrollWidth} px for ${width}`,
    })
  }

  // Elements past the page's edge, but those a scrolling or clipping box keeps in.
  const past = []
  for (const element of document.body.querySelectorAll('*')) {
    const box = element.getBoundingClientRect()
    if (box.width === 0 || box.height === 0) continue
    if (box.right <= width + 1 && box.left >= -1) continue
    if (past.some((outer) => outer.contains(element)) || !shown(element)) continue
    let kept = false
    for (
      let parent = element.parentElement;
      parent && parent !== document.body;
      parent = parent.parentElement
    ) {
      const frame = parent.getBoundingClientRect()
      if (clips(getComputedStyle(parent)) && frame.right <= width + 1 && frame.left >= -1) {
        kept = true
        break
      }
    }
    if (kept) continue
    past.push(element)
    found.push({
      kind: 'edge',
      detail: `${nameOf(element)} goes past the page's edge: ${Math.round(box.left)} to ${Math.round(box.right)} px of ${width}`,
      path: pathOf(element),
    })
  }

  // Text out of its box: the nearest box that frames it, with a border, a background or its own
  // overflow, but text cut on purpose (an ellipsis, a line clamp) or in a box that scrolls.
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  const reported = new Set()
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (!node.textContent.trim()) continue
    const element = node.parentElement
    if (!element || element.closest('script, style, noscript, template, select, textarea, svg'))
      continue
    if (!shown(element)) continue
    const range = document.createRange()
    range.selectNodeContents(node)
    const rects = [...range.getClientRects()].filter((rect) => rect.width > 0 && rect.height > 0)
    if (rects.length === 0) continue
    const text = {
      left: Math.min(...rects.map((rect) => rect.left)),
      right: Math.max(...rects.map((rect) => rect.right)),
      top: Math.min(...rects.map((rect) => rect.top)),
      bottom: Math.max(...rects.map((rect) => rect.bottom)),
    }
    let cut = false
    let frame = null
    for (let parent = element; parent && parent !== document.body; parent = parent.parentElement) {
      const style = getComputedStyle(parent)
      if (style.textOverflow === 'ellipsis' || style.webkitLineClamp !== 'none') cut = true
      const framed =
        parseFloat(style.borderTopWidth) > 0 ||
        parseFloat(style.borderLeftWidth) > 0 ||
        style.backgroundColor !== 'rgba(0, 0, 0, 0)' ||
        clips(style)
      if (framed) {
        frame = { element: parent, style }
        break
      }
    }
    if (!frame) continue
    const box = frame.element.getBoundingClientRect()
    const out =
      text.left < box.left - 1 ||
      text.right > box.right + 1 ||
      text.top < box.top - 1 ||
      text.bottom > box.bottom + 1
    if (!out) continue
    if (
      ['auto', 'scroll'].includes(frame.style.overflowX) ||
      ['auto', 'scroll'].includes(frame.style.overflowY)
    ) {
      continue
    }
    if (cut && clips(frame.style)) continue
    if (reported.has(frame.element)) continue
    reported.add(frame.element)
    const how = clips(frame.style) ? 'is cut off by' : 'runs out of'
    const snippet = node.textContent.trim().replace(/\s+/g, ' ')
    found.push({
      kind: 'text',
      detail: `"${snippet.length > 60 ? `${snippet.slice(0, 57)}…` : snippet}" ${how} ${nameOf(frame.element)}`,
      path: pathOf(frame.element),
    })
  }

  // The pages it links to, not the API's files it offers to download.
  const links = [...document.querySelectorAll('a[href]')]
    .map((link) => new URL(link.href, location.href))
    .filter((link) => link.origin === location.origin && link.pathname.startsWith('/admin/'))
    .filter((link) => !link.pathname.startsWith('/admin/api/'))
    .map((link) => link.pathname.replace(/^\/admin/, '').replace(/\/$/, '') || '/')
  return { found, links: [...new Set(links)] }
}

// Last, once the class above is defined.
await run(main)
