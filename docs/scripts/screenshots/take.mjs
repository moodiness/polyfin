// Retakes the admin app's screenshots of the documentation, in one pass, from the demo server of
// stage.mjs. See README.md.
import { writeFile, mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { parseArgs } from 'node:util'
import {
  admin,
  defaultChrome,
  docsDir,
  log,
  repoDir,
  run,
  seed,
  sleep,
  startStage,
} from './stage.mjs'

const { values: options } = parseArgs({
  options: {
    polyfin: { type: 'string', default: process.env.POLYFIN_BIN ?? join(repoDir, 'bin/polyfin') },
    'database-url': { type: 'string', default: process.env.SCREENSHOTS_DATABASE_URL },
    database: { type: 'string', default: 'polyfin_screenshots' },
    chrome: { type: 'string', default: process.env.CHROME_PATH ?? defaultChrome() },
    out: { type: 'string', default: join(docsDir, 'images') },
    only: { type: 'string' },
    keep: { type: 'boolean', default: false },
    rich: { type: 'boolean', default: false },
    help: { type: 'boolean', default: false },
  },
})

if (options.help || !options['database-url']) {
  console.log(`Usage: node take.mjs --database-url <postgresql://user:password@host:port/postgres> [options]

  --database-url  A PostgreSQL server the tool may create and drop a database on
                  (or SCREENSHOTS_DATABASE_URL).
  --database      Name of the scratch database, dropped at the end (polyfin_screenshots).
  --polyfin       Polyfin executable built with the admin app (bin/polyfin, or POLYFIN_BIN).
  --chrome        Chrome or Chromium executable (CHROME_PATH, else the usual install).
  --out           Folder the WebP files go to (docs/images).
  --only          Comma-separated shot names to retake, others are left as they are.
  --keep          Keep the demo running after the shots, until Ctrl-C, to look at it.
  --rich          Fill the demo as the sweep does, in a real server's shapes, rather than
                  as the documentation shows it (to look at it with --keep).`)
  process.exit(options.help ? 0 : 2)
}

/** Size a shot must stay under; the quality steps down until it does. */
const maxBytes = 150 * 1024
const viewport = { width: 1440, height: 900, deviceScaleFactor: 2 }

await run(main)

async function main() {
  const only = options.only ? new Set(options.only.split(',')) : null
  const stage = await startStage({
    polyfin: options.polyfin,
    chrome: options.chrome,
    databaseUrl: options['database-url'],
    database: options.database,
    logName: 'polyfin-screenshots.log',
    rich: options.rich,
  })
  const { browser, demo, server, api } = stage

  const page = await browser.newPage()
  await page.setViewport(viewport)
  await page.emulateTimezone('UTC')
  // The admin app's Content-Security-Policy would refuse the style and markers the tool adds.
  await page.setBypassCSP(true)
  await page.evaluateOnNewDocument(
    (demoUrl, shownUrl) => {
      localStorage.setItem('polyfin.language', 'en')
      // What the admin app reads, as a reader of the documentation would see it: the demo's
      // local address as a fictional one, and Polyfin in a container, as most run it.
      const fetchFirst = window.fetch.bind(window)
      window.fetch = async (...args) => {
        const response = await fetchFirst(...args)
        const url = new URL(response.url)
        const type = response.headers.get('content-type') ?? ''
        if (!url.pathname.startsWith('/admin/api/') || !type.includes('application/json')) {
          return response
        }
        let text = (await response.text()).replaceAll(demoUrl, shownUrl)
        if (url.pathname === '/admin/api/status') {
          text = JSON.stringify({ ...JSON.parse(text), system: 'container' })
        }
        return new Response(text, {
          status: response.status,
          statusText: response.statusText,
          headers: response.headers,
        })
      }
    },
    demo.url,
    'https://demo.example',
  )
  await page.setCookie({
    name: 'polyfin_session',
    value: api.session,
    domain: '127.0.0.1',
    path: '/admin',
    httpOnly: true,
    sameSite: 'Strict',
  })
  const shot = async (name, take) => {
    if (only && !only.has(name)) return
    log(`Shot ${name}`)
    const clip = await take()
    await save(page, clip, join(options.out, `${name}.webp`))
    await clearMarkers(page)
    await page.setViewport(viewport)
  }
  await mkdir(options.out, { recursive: true })

  // A fresh server: only the administrator exists.
  await shot('get-started', async () => {
    await open(page, server.url, '/')
    const card = await waitForSection(page, 'Get started')
    await clickText(page, 'button', 'All 7 steps')
    await restMouse(page)
    await sleep(300)
    return clipAround(page, [card], 24)
  })

  await shot('add-a-source-addon', async () => {
    await open(page, server.url, '/sources')
    await clickText(page, 'button', 'Add a source')
    const dialog = await page.waitForSelector('dialog[open]')
    const field = await dialog.waitForSelector('input[type="url"]')
    await sleep(300)
    await field.type('https://addon.example/manifest.json')
    await blur(page)
    return clipAround(page, [dialog], 24)
  })

  log('Seeding the demo')
  const seeded = await seed(stage)
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('polyfin.getStarted.dismissed', '1')
  })

  // First, while the two apps read their streams.
  await shot('home-now-playing', async () => {
    await open(page, server.url, '/')
    const playing = await waitForSection(page, 'Now playing')
    await page.waitForFunction(
      (section) => section.querySelectorAll('article').length >= 2,
      {},
      playing,
    )
    // The bandwidth is an average of the last 10 seconds.
    await sleep(Math.max(0, seeded.playingSince + 12000 - Date.now()))
    await page.click('article button[aria-label="Details"]')
    await sleep(3500)
    const header = await page.$('main header')
    return clipAround(page, [header, playing], 24)
  })

  await shot('top-bar-account-menu', async () => {
    await open(page, server.url, '/')
    await page.click(`button[aria-label*="${admin.name}"]`)
    const menu = await page.waitForSelector('[role="menu"]')
    await sleep(300)
    const bar = await page.$('header')
    await addMarkers(page, [
      { element: await page.$('header nav'), at: 'bottom' },
      { element: await page.$('header button[aria-label*="K"]'), at: 'left' },
      { element: menu, at: 'left-top' },
    ])
    return clipAround(page, [bar, menu], 0, { fullWidth: true, bottom: 24 })
  })

  await shot('add-a-source-folder', async () => {
    // Tall enough for the whole form, which scrolls inside the dialog otherwise.
    await page.setViewport({ ...viewport, height: 2100 })
    await open(page, server.url, '/sources')
    await clickText(page, 'button', 'Add a source')
    const dialog = await page.waitForSelector('dialog[open]')
    await clickText(page, 'dialog[open] button', 'Folder')
    await sleep(300)
    await typeInto(page, 'Name', 'Movies')
    await typeInto(page, 'Path in the container', '/media/movies')
    await blur(page)
    return clipAround(page, [dialog], 24)
  })

  await shot('user-page', async () => {
    await open(page, server.url, `/users/${seeded.users.Sam}`)
    const panel = await waitForSection(page, 'Name and password')
    const header = await page.$('main header')
    const nav = await page.$('main nav')
    return clipAround(page, [header, nav, panel], 32)
  })

  await shot('home-rows-editor', async () => {
    // Without the section's address, which would light it as a search jump does.
    await open(page, server.url, '/settings/web-player')
    const rows = await waitForSection(page, 'Home rows')
    await restMouse(page)
    return clipAround(page, [rows], 24)
  })

  await shot('iptv-source', async () => {
    await open(page, server.url, `/sources/shared/${seeded.iptv}`)
    await page.waitForNetworkIdle({ idleTime: 500 })
    return clipAround(page, [await contentBox(page, 'main')], 32)
  })

  if (options.keep) {
    console.log(
      `Demo running: ${server.url}/admin/ as ${admin.name} / ${admin.password}. Ctrl-C stops it.`,
    )
    await new Promise(() => {})
  }
}

// Pages.

async function open(page, base, path) {
  await page.goto(`${base}/admin${path}`, { waitUntil: 'networkidle0' })
  await page.evaluate(() => document.fonts.ready)
  // No blinking caret in a typed field.
  await page.addStyleTag({ content: '* { caret-color: transparent !important; }' })
}

/** The section (a Block) whose heading starts with the text. */
async function waitForSection(page, heading) {
  const handle = await page.waitForFunction(
    (text) =>
      [...document.querySelectorAll('section h2, section h3')]
        .find((h) => h.textContent.trim().startsWith(text))
        ?.closest('section') ?? null,
    { timeout: 20000 },
    heading,
  )
  return handle.asElement()
}

async function clickText(page, selector, text) {
  const handle = await page.waitForFunction(
    (selector, text) =>
      [...document.querySelectorAll(selector)].find(
        (element) => element.textContent.trim() === text,
      ) ?? null,
    { timeout: 20000 },
    selector,
    text,
  )
  await handle.asElement().click()
}

/** Types into the field whose label reads the text. */
async function typeInto(page, label, text) {
  const handle = await page.waitForFunction(
    (label) => {
      const found = [...document.querySelectorAll('label')].find(
        (element) => element.textContent.trim() === label,
      )
      if (!found) return null
      return (
        (found.htmlFor && document.getElementById(found.htmlFor)) ||
        found.querySelector('input, textarea') ||
        null
      )
    },
    { timeout: 20000 },
    label,
  )
  await handle.asElement().type(text)
}

/**
 * The box of what an element shows: its texts, controls and framed panels, without the empty
 * width of its layout's wrappers, nor what is laid out but not seen: the hidden dock, and the
 * help that only screen readers read.
 */
async function contentBox(page, selector) {
  return page.$eval(selector, (root) => {
    const box = { x: Infinity, y: Infinity, right: -Infinity, bottom: -Infinity }
    for (const element of root.querySelectorAll('*')) {
      const rect = element.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) continue
      if (element.closest('[inert], .sr-only')) continue
      if (!element.checkVisibility({ opacityProperty: true, visibilityProperty: true })) continue
      const style = getComputedStyle(element)
      const framed =
        parseFloat(style.borderTopWidth) > 0 || style.backgroundColor !== 'rgba(0, 0, 0, 0)'
      if (element.children.length > 0 && !framed) continue
      box.x = Math.min(box.x, rect.left + scrollX)
      box.y = Math.min(box.y, rect.top + scrollY)
      box.right = Math.max(box.right, rect.right + scrollX)
      box.bottom = Math.max(box.bottom, rect.bottom + scrollY)
    }
    return box
  })
}

/** The box around the elements (handles, or boxes from contentBox), in page coordinates, with a margin. */
async function clipAround(page, elements, margin, { fullWidth = false, bottom, maxHeight } = {}) {
  const boxes = await Promise.all(
    elements.map((element) =>
      typeof element.evaluate !== 'function'
        ? element
        : element.evaluate((node) => {
            const box = node.getBoundingClientRect()
            return {
              x: box.x + scrollX,
              y: box.y + scrollY,
              right: box.right + scrollX,
              bottom: box.bottom + scrollY,
            }
          }),
    ),
  )
  const width = await page.evaluate(() => document.documentElement.scrollWidth)
  let x = Math.min(...boxes.map((box) => box.x)) - margin
  let y = Math.min(...boxes.map((box) => box.y)) - margin
  let right = Math.max(...boxes.map((box) => box.right)) + margin
  let end = Math.max(...boxes.map((box) => box.bottom)) + (bottom ?? margin)
  if (fullWidth) {
    x = 0
    right = width
  }
  x = Math.max(0, x)
  y = Math.max(0, y)
  if (maxHeight) end = Math.min(end, y + maxHeight)
  return {
    x: Math.round(x),
    y: Math.round(y),
    width: Math.round(right - x),
    height: Math.round(end - y),
  }
}

/** Small numbered markers, explained in the text of the page the shot is on. */
async function addMarkers(page, marks) {
  let number = 0
  for (const { element, at } of marks) {
    number++
    if (!element) throw new Error(`marker ${number}: element not found`)
    await element.evaluate(
      (node, number, at) => {
        const box = node.getBoundingClientRect()
        const marker = document.createElement('div')
        marker.dataset.screenshotMarker = ''
        marker.textContent = String(number)
        const size = 22
        // Beside the element on the left, at its middle or its top, or under it.
        const [left, top] =
          at === 'bottom'
            ? [box.left + box.width / 2 - size / 2, box.bottom + 8]
            : [
                box.left - size - 8,
                at === 'left-top' ? box.top + 12 : box.top + box.height / 2 - size / 2,
              ]
        Object.assign(marker.style, {
          position: 'absolute',
          left: `${left + scrollX}px`,
          top: `${top + scrollY}px`,
          width: `${size}px`,
          height: `${size}px`,
          borderRadius: '50%',
          background: '#5b63f6',
          boxShadow: '0 0 0 2px #08080e',
          color: '#fff',
          font: '600 12px/22px "Figtree Variable", system-ui, sans-serif',
          textAlign: 'center',
          zIndex: '2147483647',
          pointerEvents: 'none',
        })
        document.body.append(marker)
      },
      number,
      at,
    )
  }
}

async function clearMarkers(page) {
  await page.evaluate(() => {
    for (const marker of document.querySelectorAll('[data-screenshot-marker]')) marker.remove()
  })
}

/** No focused field, so that none shows lit. */
async function blur(page) {
  await page.evaluate(() => document.activeElement?.blur())
  await sleep(300)
}

/** The mouse off the page's controls, so that none shows lit under it. */
async function restMouse(page) {
  await page.mouse.move(1, 1)
}

/** Saves the clip as WebP, at the best quality that keeps it under maxBytes. */
async function save(page, clip, path) {
  let picture
  for (const quality of [90, 85, 80, 75, 70, 65, 60, 55, 50]) {
    picture = await page.screenshot({ type: 'webp', quality, clip, captureBeyondViewport: true })
    if (picture.length <= maxBytes) break
  }
  await writeFile(path, picture)
  log(`  ${path}: ${clip.width * 2}×${clip.height * 2}, ${Math.round(picture.length / 1024)} KB`)
}
