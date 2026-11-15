// Records the requests jellyfin-web sends for the bench's screens, in a local Chrome against the
// bench server, into screens.json: the home page, the live TV guide (its first screen, then
// scrolled), and search (a common word, then a rare one). See README.md.
import { existsSync } from 'node:fs'
import { readFile, writeFile } from 'node:fs/promises'
import puppeteer from 'puppeteer-core'
import { cleanup, log, options, run, screensFile, sleep, startStage, viewer } from './lib.mjs'
import { toWaves } from './replay.mjs'

/** The words searched: one in every series' name, and one in about one title in 250. */
const words = { common: 'show', rare: 'kepnor' }

function defaultChrome() {
  const candidates = {
    darwin: [
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
      '/Applications/Chromium.app/Contents/MacOS/Chromium',
    ],
    linux: ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'],
    win32: ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'],
  }[process.platform]
  return candidates?.find((path) => existsSync(path))
}

await run(async () => {
  const values = options({
    chrome: { type: 'string', default: process.env.CHROME_PATH ?? defaultChrome() },
    shots: { type: 'string' },
  })
  if (!values.web || !existsSync(`${values.web}/index.html`)) {
    throw new Error('give --web or POLYFIN_WEB_DIR: a folder of jellyfin-web, built')
  }
  if (!values.chrome || !existsSync(values.chrome)) {
    throw new Error('Chrome not found: give --chrome or CHROME_PATH')
  }
  const stage = await startStage(values)
  const base = stage.polyfin.url

  log('Starting Chrome')
  const browser = await puppeteer.launch({
    executablePath: values.chrome,
    headless: true,
    handleSIGINT: false,
    handleSIGTERM: false,
    handleSIGHUP: false,
    args: ['--lang=en-US'],
  })
  try {
    const page = await browser.newPage()
    await page.setViewport({ width: 1440, height: 900 })

    // Every request to the server, at whichever of its addresses the web player chose, but the
    // web player's own files, the pictures, which Polyfin fetches from the provider's
    // addresses, not reachable here, and the bitrate test the web player runs on its own
    // timer, whatever the screen.
    let recording = null
    const started = new Map()
    const port = new URL(base).port
    const api = (url) => {
      const { port: to, pathname } = new URL(url)
      return (
        to === port &&
        !pathname.startsWith('/web/') &&
        !/\/Images\/|\/socket|\/Branding\/Css|\/Playback\/BitrateTest|\/System\/Endpoint|\.(js|css|png|svg|woff2?|ico|json)$/.test(
          pathname,
        )
      )
    }
    page.on('request', (request) => {
      if (recording && api(request.url())) {
        started.set(request, { start: performance.now(), epoch: Date.now() })
      }
    })
    const finished = async (request) => {
      const { start, epoch } = started.get(request) ?? {}
      if (start === undefined || !recording) return
      started.delete(request)
      const end = performance.now()
      const url = new URL(request.url())
      let json
      try {
        const response = request.response()
        if (response?.headers()['content-type']?.includes('json')) json = await response.json()
      } catch {
        // No body, such as a 204.
      }
      recording.push({
        method: request.method(),
        path: url.pathname + url.search,
        body: request.postData(),
        start,
        end,
        epoch,
        json,
      })
    }
    page.on('requestfinished', finished)
    page.on('requestfailed', finished)

    /** Records what step sends, until no request has been under way for two seconds. */
    const record = async (name, step) => {
      recording = []
      await step()
      let quiet = 0
      while (quiet < 2000) {
        await sleep(100)
        quiet = started.size ? 0 : quiet + 100
      }
      const captured = recording
      recording = null
      log(`${name}: ${captured.length} requests`)
      if (values.shots) {
        const file = `${name.toLowerCase().replace(/[^a-z]+/g, '-')}.png`
        await page.screenshot({ path: `${values.shots}/${file}` })
      }
      return captured
    }

    // The home page as a signed-in web player opens it with nothing kept yet: from the sign-in
    // on, without the sign-in itself. Opened again, the web player shows what it kept, and
    // asks only for the rows of Polyfin's own.
    log('Opening jellyfin-web')
    await page.goto(`${base}/web/index.html#/login`, { waitUntil: 'networkidle0' })
    const manual = await page.$('.btnManual')
    if (manual) await manual.click()
    await page.waitForSelector('#txtManualName', { visible: true })
    await page.type('#txtManualName', viewer.name)
    await page.type('#txtManualPassword', viewer.password)
    const screens = []
    const home = await record('Home', async () => {
      await page.click('.manualLoginForm button[type=submit]')
    })
    const user = await page.evaluate(() => window.ApiClient?.getCurrentUserId())
    screens.push({
      name: 'Home',
      captured: home.filter((request) => !/\/Users\/authenticatebyname/i.test(request.path)),
    })

    const guide = await record('Guide', async () => {
      await page.goto(`${base}/web/index.html#/livetv?tab=1`)
    })
    screens.push({ name: 'Guide', captured: guide })
    const scrolled = await record('Guide, scrolled', async () => {
      await page.evaluate(() => {
        const scroller = document.querySelector('.guide-programsList, .programGrid, .guideScroller')
        const target = scroller ?? document.scrollingElement
        target.scrollBy(0, target.scrollHeight)
      })
      await sleep(500)
      await page.evaluate(() => {
        const next = [...document.querySelectorAll('.btnNextPage, .guide-nextPage')].find(
          (button) => !button.disabled && button.offsetParent,
        )
        next?.click()
      })
    })
    screens.push({ name: 'Guide, scrolled', captured: scrolled })

    for (const [kind, word] of Object.entries(words)) {
      await page.goto(`${base}/web/index.html#/search`)
      await page.waitForSelector('input[type=search], .searchfields-txtSearch', { visible: true })
      await sleep(1500)
      const captured = await record(`Search, ${kind} word`, async () => {
        await page.type('input[type=search], .searchfields-txtSearch', word, { delay: 120 })
      })
      screens.push({ name: `Search, ${kind} word`, captured })
    }

    const previous = existsSync(screensFile) ? JSON.parse(await readFile(screensFile, 'utf8')) : {}
    const budgets = Object.fromEntries((previous.screens ?? []).map((s) => [s.name, s.budget]))
    const file = {
      capturedAt: new Date().toISOString(),
      words,
      screens: screens.map(({ name, captured }) => ({
        name,
        budget: budgets[name] ?? null,
        waves: toWaves(captured, user),
      })),
    }
    await writeFile(screensFile, `${JSON.stringify(file, null, 2)}\n`)
    log(`Wrote ${screensFile}`)
  } finally {
    await browser.close()
    await cleanup()
  }
})
