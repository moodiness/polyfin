// Retakes the admin app's screenshots of the documentation, in one pass, from a demo server: a
// scratch database, a Polyfin built from this repository, the fictional addon and IPTV list of
// demo.mjs, and a local Chrome driven by puppeteer-core. See README.md.
import { spawn, execFileSync } from 'node:child_process'
import { existsSync, statSync } from 'node:fs'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import { randomBytes } from 'node:crypto'
import pg from 'pg'
import puppeteer from 'puppeteer-core'
import { channels, startDemo } from './demo.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const docsDir = resolve(here, '../..')
const repoDir = resolve(docsDir, '..')

const { values: options } = parseArgs({
  options: {
    polyfin: { type: 'string', default: process.env.POLYFIN_BIN ?? join(repoDir, 'bin/polyfin') },
    'database-url': { type: 'string', default: process.env.SCREENSHOTS_DATABASE_URL },
    database: { type: 'string', default: 'polyfin_screenshots' },
    chrome: { type: 'string', default: process.env.CHROME_PATH ?? defaultChrome() },
    out: { type: 'string', default: join(docsDir, 'images') },
    only: { type: 'string' },
    keep: { type: 'boolean', default: false },
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
  --keep          Keep the demo running after the shots, until Ctrl-C, to look at it.`)
  process.exit(options.help ? 0 : 2)
}

/** Size a shot must stay under; the quality steps down until it does. */
const maxBytes = 150 * 1024
const viewport = { width: 1440, height: 900, deviceScaleFactor: 2 }
/** The pace the demo apps read their streams at, in bits per second: a little above the file's. */
const readBitrate = 6_000_000
const admin = { name: 'Morgan', password: 'demo-password-1' }
const members = [
  { name: 'Sam', password: 'demo-password-2' },
  { name: 'Robin', password: 'demo-password-3' },
  { name: 'Alex', password: 'demo-password-4' },
]

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

// Everything started is stopped on the way out, in the reverse order, whatever happens.
const cleanups = []
let cleaning = null
function cleanup() {
  cleaning ??= (async () => {
    for (const step of cleanups.reverse()) {
      try {
        await step()
      } catch (error) {
        console.error(`cleanup: ${error.message}`)
      }
    }
  })()
  return cleaning
}
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => cleanup().then(() => process.exit(130)))
}

try {
  await main()
} catch (error) {
  console.error(error)
  process.exitCode = 1
} finally {
  await cleanup()
}

async function main() {
  if (!existsSync(options.polyfin)) {
    throw new Error(`${options.polyfin} not found: build Polyfin first (see README.md)`)
  }
  if (!options.chrome || !existsSync(options.chrome)) {
    throw new Error('Chrome not found: give --chrome or CHROME_PATH')
  }
  const only = options.only ? new Set(options.only.split(',')) : null
  const scratch = await mkdtemp(join(tmpdir(), 'polyfin-screenshots-'))
  cleanups.push(() => rm(scratch, { recursive: true, force: true }))

  log('Starting Chrome')
  const browser = await puppeteer.launch({
    executablePath: options.chrome,
    headless: true,
    defaultViewport: viewport,
    args: ['--lang=en-US', '--hide-scrollbars', '--font-render-hinting=none'],
  })
  cleanups.push(() => browser.close())

  log('Drawing the generated pictures')
  const images = await drawPictures(browser)
  const video = makeVideo(scratch)
  const demo = await startDemo({ posterDir: join(docsDir, 'public/posters'), images, video })
  cleanups.push(() => demo.close())
  log(`Demo addon at ${demo.manifestUrl}`)

  const databaseUrl = await createDatabase(options['database-url'], options.database)
  const server = await startPolyfin(scratch, databaseUrl)
  log(`Polyfin at ${server.url}`)

  const api = client(server.url)
  await api.post('/admin/api/setup', {
    setupCode: server.setupCode,
    name: admin.name,
    password: admin.password,
    language: 'en',
  })

  const page = await browser.newPage()
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
    return clipAround(page, [card], 24)
  })

  await shot('add-a-source-addon', async () => {
    await open(page, server.url, '/sources')
    await clickText(page, 'button', 'Add a source')
    const dialog = await page.waitForSelector('dialog[open]')
    const field = await dialog.waitForSelector('input[type="url"]')
    await sleep(300)
    await field.type('https://addon.example/manifest.json')
    return clipAround(page, [dialog], 24)
  })

  log('Seeding the demo')
  const seeded = await seed(api, demo)
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
    await clickText(page, 'article button', 'Details')
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
    await page.setViewport({ ...viewport, height: 1500 })
    await open(page, server.url, '/sources')
    await clickText(page, 'button', 'Add a source')
    const dialog = await page.waitForSelector('dialog[open]')
    await clickText(page, 'dialog[open] button', 'Folder')
    await sleep(300)
    await typeInto(page, 'Name', 'Movies')
    await typeInto(page, 'Path in the container', '/media/movies')
    await page.evaluate(() => document.activeElement?.blur())
    await sleep(300)
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
    await open(page, server.url, '/settings/web-player#home-rows')
    const rows = await waitForSection(page, 'Home rows')
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

function log(message) {
  console.log(`· ${message}`)
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** The date some days from now, as `YYYY-MM-DD`. */
function dayFromNow(days) {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 10)
}

/** Wild Atlas's poster, which the website lacks, and the channels' logos, drawn by the browser. */
async function drawPictures(browser) {
  const images = new Map()
  const page = await browser.newPage()
  await page.setViewport({ width: 400, height: 600, deviceScaleFactor: 1 })
  await page.setContent(`<body style="margin:0">
    <div style="width:400px;height:600px;display:flex;flex-direction:column;justify-content:flex-end;
      align-items:center;background:linear-gradient(160deg,#0f3d2e 0%,#1d6b4f 45%,#d9a441 100%);
      font:700 54px/1 Georgia,serif;color:#fff8e7;letter-spacing:6px;text-align:center">
      <svg width="400" height="260" viewBox="0 0 400 260"><path d="M0 260 L90 120 L150 190 L240 60 L330 170 L400 110 L400 260 Z" fill="#0b2b20" opacity=".85"/>
      <circle cx="300" cy="70" r="34" fill="#ffe3a3" opacity=".9"/></svg>
      <div style="padding:0 0 80px">WILD<br>ATLAS</div></div></body>`)
  images.set('wild-atlas.jpg', Buffer.from(await page.screenshot({ type: 'jpeg', quality: 88 })))
  await page.setViewport({ width: 256, height: 256, deviceScaleFactor: 1 })
  for (const channel of channels) {
    await page.setContent(`<body style="margin:0;background:transparent">
      <div style="width:256px;height:256px;border-radius:56px;background:${channel.color};display:flex;
        align-items:center;justify-content:center;font:700 64px/1 system-ui,sans-serif;color:#fff">
        ${channel.name
          .split(' ')
          .map((word) => word[0])
          .join('')}</div></body>`)
    images.set(
      `logo-${channel.id}.png`,
      Buffer.from(await page.screenshot({ type: 'png', omitBackground: true })),
    )
  }
  await page.close()
  return images
}

/**
 * The versions' file: a 1080p H.264 and AAC video the server analyzes. It lasts 12 minutes, as
 * Polyfin refuses a file under a tenth of the title's runtime as a trailer standing in for it,
 * at 2 pictures a second and a constant 3 Mbps, so that it is made in seconds yet has a bitrate
 * of a real one (about 280 MB, in the scratch folder).
 */
function makeVideo(scratch) {
  const path = join(scratch, 'demo.mp4')
  // prettier-ignore
  execFileSync(process.env.POLYFIN_FFMPEG ?? 'ffmpeg', [
    '-loglevel', 'error',
    '-f', 'lavfi', '-i', 'testsrc2=size=1920x1080:rate=2',
    '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000',
    '-t', '720',
    '-c:v', 'libx264', '-preset', 'ultrafast',
    '-b:v', '3M', '-minrate', '3M', '-maxrate', '3M', '-bufsize', '6M', '-x264-params', 'nal-hrd=cbr',
    '-c:a', 'aac', '-b:a', '128k', '-ac', '2',
    '-movflags', '+faststart', path,
  ])
  return { path, size: statSync(path).size }
}

/** Creates the scratch database afresh, and drops it on the way out. */
async function createDatabase(url, name) {
  if (!/^[a-z_][a-z0-9_]*$/.test(name)) throw new Error(`invalid database name ${name}`)
  const run = async (sql) => {
    const db = new pg.Client({ connectionString: url })
    await db.connect()
    try {
      await db.query(sql)
    } finally {
      await db.end()
    }
  }
  await run(`DROP DATABASE IF EXISTS ${name} WITH (FORCE)`)
  await run(`CREATE DATABASE ${name}`)
  cleanups.push(() => run(`DROP DATABASE IF EXISTS ${name} WITH (FORCE)`))
  const own = new URL(url)
  own.pathname = `/${name}`
  return own.toString()
}

/** Starts Polyfin on free ports, and reads its setup code from its log. */
async function startPolyfin(scratch, databaseUrl) {
  const [http, discovery, dlna, hdhomerun] = await freePorts(4)
  const data = join(scratch, 'data')
  // A web player folder with an index.html turns the web player on, so the top bar links to it.
  const web = join(scratch, 'web')
  await mkdir(web, { recursive: true })
  await writeFile(join(web, 'index.html'), '<!doctype html><title>Web player</title>')
  const child = spawn(options.polyfin, ['serve'], {
    env: {
      ...process.env,
      POLYFIN_DATABASE_URL: databaseUrl,
      POLYFIN_DATA_DIR: data,
      POLYFIN_LISTEN: `127.0.0.1:${http}`,
      POLYFIN_DISCOVERY_PORT: String(discovery),
      POLYFIN_DLNA_PORT: String(dlna),
      POLYFIN_HDHOMERUN_PORT: String(hdhomerun),
      POLYFIN_HDHOMERUN_ADDRESSES: '127.0.0.1',
      POLYFIN_WEB_DIR: web,
      // A key of this run only, so that the stored secrets are encrypted as advised.
      POLYFIN_SECRET_KEY: randomBytes(32).toString('base64'),
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const logPath = join(tmpdir(), 'polyfin-screenshots.log')
  let logText = ''
  const exited = new Promise((resolve) => child.on('exit', resolve))
  cleanups.push(async () => {
    if (child.exitCode === null) {
      child.kill('SIGTERM')
      await Promise.race([exited, sleep(10000)])
      if (child.exitCode === null) child.kill('SIGKILL')
    }
  })
  const setupCode = await new Promise((resolvePromise, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`no setup code in Polyfin's log:\n${logText}`)),
      60000,
    )
    const read = (chunk) => {
      logText += chunk
      const match = logText.match(/setup_code=([A-Z0-9]{4}-[A-Z0-9]{4})/)
      if (match) {
        clearTimeout(timer)
        resolvePromise(match[1])
      }
    }
    child.stdout.setEncoding('utf8').on('data', read)
    child.stderr.setEncoding('utf8').on('data', read)
    exited.then((code) => reject(new Error(`Polyfin exited with ${code}:\n${logText}`)))
  })
  // Polyfin's log stays after the run, to tell why a step failed.
  cleanups.push(async () => {
    await writeFile(logPath, logText)
    log(`Polyfin's log: ${logPath}`)
  })
  const url = `http://127.0.0.1:${http}`
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(`${url}/admin/api/status`)).ok) break
    } catch {
      // Not listening yet.
    }
    await sleep(200)
  }
  return { url, setupCode }
}

async function freePorts(count) {
  const { createServer } = await import('node:net')
  const servers = await Promise.all(
    Array.from({ length: count }, () => {
      const server = createServer()
      return new Promise((resolvePromise) =>
        server.listen(0, '127.0.0.1', () => resolvePromise(server)),
      )
    }),
  )
  const ports = servers.map((server) => server.address().port)
  await Promise.all(servers.map((server) => new Promise((done) => server.close(done))))
  return ports
}

/** The admin API, as the administrator: the session cookie of setup or sign-in is kept. */
function client(base) {
  const api = {
    base,
    session: '',
    async request(method, path, body) {
      const response = await fetch(base + path, {
        method,
        headers: {
          ...(body === undefined ? {} : { 'content-type': 'application/json' }),
          ...(api.session ? { cookie: `polyfin_session=${api.session}` } : {}),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const cookie = response.headers.get('set-cookie')?.match(/polyfin_session=([^;]+)/)
      if (cookie) api.session = cookie[1]
      const text = await response.text()
      if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${text}`)
      return text ? JSON.parse(text) : null
    },
    get: (path) => api.request('GET', path),
    post: (path, body) => api.request('POST', path, body ?? {}),
    put: (path, body) => api.request('PUT', path, body),
    patch: (path, body) => api.request('PATCH', path, body),
  }
  return api
}

/** A Jellyfin app's client: signs in as a user, then speaks for one device. */
function jellyfinApp(base, { client: name, device, deviceId }) {
  // Values are escaped, as apps do for names with spaces or punctuation.
  const header = (token) =>
    `MediaBrowser Client="${encodeURIComponent(name)}", Device="${encodeURIComponent(device)}", DeviceId="${deviceId}", Version="10.11.0"${token ? `, Token="${token}"` : ''}`
  let token = ''
  let userId = ''
  const call = async (method, path, body) => {
    const response = await fetch(base + path, {
      method,
      headers: {
        authorization: header(token),
        ...(body === undefined ? {} : { 'content-type': 'application/json' }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await response.text()
    if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${text}`)
    return text ? JSON.parse(text) : null
  }
  return {
    async signIn(user) {
      const answer = await call('POST', '/Users/AuthenticateByName', {
        Username: user.name,
        Pw: user.password,
      })
      token = answer.AccessToken
      userId = answer.User.Id
    },
    get userId() {
      return userId
    },
    get token() {
      return token
    },
    call,
    /** A request of the app's own, such as its stream, with its authorization. */
    fetch: (url, init) => fetch(url, { ...init, headers: { authorization: header(token) } }),
  }
}

/** Fills the demo server: the addon and its libraries, users, IPTV, home rows and two playbacks. */
async function seed(api, demo) {
  const addon = await api.post('/admin/api/scopes/shared/addons', { manifestUrl: demo.manifestUrl })

  const users = {}
  for (const member of members) {
    const user = await api.post('/admin/api/users', { ...member, isAdministrator: false })
    users[member.name] = user.id
  }

  const iptv = await api.post('/admin/api/scopes/shared/iptv', {
    name: 'Demo TV',
    kind: 'm3u',
    url: demo.playlistUrl,
    guideUrl: demo.guideUrl,
  })

  const choices = await api.get('/admin/api/home-rows/choices')
  const catalog = (type) =>
    choices.catalogs.find((item) => item.addonId === addon.id && item.catalogType === type)
  const row = (fields) => ({
    kind: 'catalog',
    place: 'top',
    addonId: '',
    catalogType: '',
    catalogId: '',
    collectionId: '',
    count: 3,
    everyone: true,
    users: [],
    library: false,
    schedule: { startDate: '', endDate: '', days: [], startHour: null, endHour: null },
    ...fields,
  })
  const movieCatalog = catalog('movie')
  const seriesCatalog = catalog('series')
  if (movieCatalog) {
    await api.post(
      '/admin/api/home-rows',
      row({
        name: 'Featured movies',
        addonId: addon.id,
        catalogType: 'movie',
        catalogId: movieCatalog.catalogId,
      }),
    )
  }
  await api.post(
    '/admin/api/home-rows',
    row({ name: 'Because you watched', kind: 'because', place: 'resume' }),
  )
  if (seriesCatalog) {
    await api.post(
      '/admin/api/home-rows',
      row({
        name: 'Festival picks',
        addonId: addon.id,
        catalogType: 'series',
        catalogId: seriesCatalog.catalogId,
        place: 'bottom',
        everyone: false,
        users: [users.Sam, users.Robin],
        // From next week, so that the list shows it as Not shown now whatever the day.
        schedule: {
          startDate: dayFromNow(7),
          endDate: dayFromNow(21),
          days: [],
          startHour: null,
          endHour: null,
        },
      }),
    )
  }
  await api.post(
    '/admin/api/home-rows',
    row({ name: 'New on Polyfin', kind: 'new', place: 'bottom' }),
  )

  await play(api, {
    user: members[0],
    app: { client: 'Jellyfin Android TV', device: 'Living room TV', deviceId: 'demo-tv-1' },
    title: 'Night Train',
    position: 41 * 60,
    method: 'DirectPlay',
  })
  await play(api, {
    user: members[1],
    app: { client: 'Jellyfin iOS', device: 'Robin’s phone', deviceId: 'demo-phone-1' },
    title: 'Amber Road',
    position: 12 * 60,
    method: 'DirectStream',
  })
  return { addon: addon.id, iptv: iptv.id, users, playingSince: Date.now() }
}

/**
 * Reports a playback as a Jellyfin app does: it finds the title in its library, asks for its
 * version (the server reads the demo file then), and reports the play, then its progress every
 * minute so that it stays on Home.
 */
async function play(api, { user, app, title, position, method }) {
  const jellyfin = jellyfinApp(api.base, app)
  await jellyfin.signIn(user)
  // The app takes commands, as most do: Home offers to send it a message or stop its play.
  await jellyfin.call('POST', '/Sessions/Capabilities/Full', {
    PlayableMediaTypes: ['Video'],
    SupportedCommands: ['DisplayMessage'],
    SupportsMediaControl: true,
  })
  const socket = new WebSocket(
    `${api.base.replace('http', 'ws')}/socket?ApiKey=${jellyfin.token}&deviceId=${app.deviceId}`,
  )
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = () => reject(new Error(`${app.device}: the socket did not open`))
  })
  const keepAlive = setInterval(
    () => socket.send(JSON.stringify({ MessageType: 'KeepAlive' })),
    30000,
  )
  cleanups.push(() => {
    clearInterval(keepAlive)
    socket.close()
  })
  const views = await jellyfin.call('GET', `/UserViews?userId=${jellyfin.userId}`)
  let item = null
  for (const view of views.Items) {
    const found = await jellyfin.call(
      'GET',
      `/Items?userId=${jellyfin.userId}&parentId=${view.Id}&limit=50`,
    )
    item = found.Items.find((candidate) => candidate.Name === title) ?? item
  }
  if (!item) throw new Error(`${title} is not in ${user.name}'s libraries`)
  const info = await jellyfin.call(
    'POST',
    `/Items/${item.Id}/PlaybackInfo?userId=${jellyfin.userId}`,
    {
      UserId: jellyfin.userId,
      AutoOpenLiveStream: true,
      DeviceProfile: {
        MaxStreamingBitrate: 120000000,
        DirectPlayProfiles: [
          { Container: 'mp4,mkv', Type: 'Video', VideoCodec: 'h264,hevc', AudioCodec: 'aac,ac3' },
        ],
        TranscodingProfiles: [
          {
            Container: 'ts',
            Type: 'Video',
            VideoCodec: 'h264',
            AudioCodec: 'aac',
            Protocol: 'hls',
            Context: 'Streaming',
          },
        ],
      },
    },
  )
  const source = info.MediaSources?.[0]
  if (!source) throw new Error(`no version of ${title}: ${JSON.stringify(info)}`)
  const report = (paused) => ({
    ItemId: item.Id,
    MediaSourceId: source.Id,
    PlaySessionId: info.PlaySessionId,
    PositionTicks: position * 10_000_000,
    PlayMethod: method,
    IsPaused: paused,
    CanSeek: true,
  })
  await jellyfin.call('POST', '/Sessions/Playing', report(false))
  const timer = setInterval(() => {
    position += 60
    jellyfin.call('POST', '/Sessions/Playing/Progress', report(false)).catch(() => {})
  }, 60000)
  cleanups.push(() => clearInterval(timer))

  // The app reads the file at its bitrate, again from the start at its end, so that Home shows
  // what the play moves.
  const reading = new AbortController()
  cleanups.push(() => reading.abort())
  const address = `${api.base}/Videos/${item.Id}/stream?static=true&mediaSourceId=${source.Id}&playSessionId=${info.PlaySessionId}`
  ;(async () => {
    while (!reading.signal.aborted) {
      const response = await jellyfin.fetch(address, { signal: reading.signal })
      for await (const chunk of response.body) {
        await sleep((chunk.length * 8 * 1000) / readBitrate)
      }
    }
  })().catch(() => {})
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
 * width of its layout's wrappers.
 */
async function contentBox(page, selector) {
  return page.$eval(selector, (root) => {
    const box = { x: Infinity, y: Infinity, right: -Infinity, bottom: -Infinity }
    for (const element of root.querySelectorAll('*')) {
      const rect = element.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) continue
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
          font: '600 12px/22px "Geist Variable", system-ui, sans-serif',
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
