// The demo server both tools run against, take.mjs for the screenshots and sweep.mjs for the
// admin app's sweep: a scratch database, a Polyfin built from this repository, the fixtures of
// demo.mjs, and a local Chrome driven by puppeteer-core. See README.md.
import { spawn, execFileSync } from 'node:child_process'
import { createHash, randomBytes, X509Certificate } from 'node:crypto'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import pg from 'pg'
import puppeteer from 'puppeteer-core'
import { channels, sources, startDemo } from './demo.mjs'

const here = dirname(fileURLToPath(import.meta.url))
export const docsDir = resolve(here, '../..')
export const repoDir = resolve(docsDir, '..')

/** The pace the demo apps read their streams at, in bits per second: a little above the file's. */
const readBitrate = 6_000_000
export const admin = { name: 'Morgan', password: 'demo-password-1' }
/** The members: Sam, Robin and Alex, whom the shots show, then those of a real server's size. */
export const members = [
  { name: 'Sam', password: 'demo-password-2' },
  { name: 'Robin', password: 'demo-password-3' },
  { name: 'Alex', password: 'demo-password-4' },
  { name: 'Jordan', password: 'demo-password-5' },
  { name: 'Casey', password: 'demo-password-6' },
  { name: 'Riley', password: 'demo-password-7' },
  { name: 'Taylor', password: 'demo-password-8' },
  { name: 'Quinn', password: 'demo-password-9' },
  { name: 'Avery', password: 'demo-password-10' },
]
/** The groups, of up to five members, every member in one. */
const groups = {
  Family: ['Sam', 'Robin', 'Jordan', 'Casey', 'Riley'],
  'Weekend movie club': ['Alex', 'Taylor', 'Quinn', 'Avery'],
}

export function defaultChrome() {
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
export function cleanup() {
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
// With --keep, Ctrl-C or SIGTERM is the way out: it stops everything too, and a second signal
// leaves at once. A closed terminal's SIGHUP stops everything as well.
for (const [signal, code] of [
  ['SIGINT', 130],
  ['SIGTERM', 143],
  ['SIGHUP', 129],
]) {
  process.on(signal, () => {
    if (cleaning) process.exit(code)
    cleanup().then(() => process.exit(code))
  })
}

/** Runs main, then stops everything, with exit code 1 on an error. */
export async function run(main) {
  try {
    await main()
  } catch (error) {
    console.error(error)
    process.exitCode = 1
  } finally {
    await cleanup()
  }
  // Everything is stopped, so node ends on its own; should a handle still hold it, it ends
  // anyway, and says what held it.
  setTimeout(() => {
    console.error(`exiting, still held by: ${process.getActiveResourcesInfo().join(', ')}`)
    process.exit()
  }, 5000).unref()
}

export function log(message) {
  console.log(`· ${message}`)
}

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** Waits for promise, but ms at most, and leaves no timer behind. */
async function within(promise, ms) {
  let timer
  try {
    await Promise.race([promise, new Promise((resolve) => (timer = setTimeout(resolve, ms)))])
  } finally {
    clearTimeout(timer)
  }
}

/** The date some days from now, as `YYYY-MM-DD`. */
function dayFromNow(days) {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 10)
}

/**
 * Starts the demo up to its fresh server: Chrome, the fixtures, the database and Polyfin, whose
 * setup makes the administrator. `api` is the admin API as the administrator. `rich` gives the
 * demo a real server's shapes, as the sweep wants: Demo Catalog's logo now, and the rest of
 * enrich() as seed() fills it; without it, the demo is the plain one the documentation shows.
 */
export async function startStage({ polyfin, chrome, databaseUrl, database, logName, rich }) {
  if (!existsSync(polyfin)) {
    throw new Error(`${polyfin} not found: build Polyfin first (see README.md)`)
  }
  if (!chrome || !existsSync(chrome)) {
    throw new Error('Chrome not found: give --chrome or CHROME_PATH')
  }
  const scratch = await mkdtemp(join(tmpdir(), 'polyfin-screenshots-'))
  cleanups.push(() => rm(scratch, { recursive: true, force: true }))
  const certificate = makeCertificate(scratch)

  log('Starting Chrome')
  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    // The signals are this tool's: Puppeteer's own would end node before the database is dropped.
    handleSIGINT: false,
    handleSIGTERM: false,
    handleSIGHUP: false,
    args: [
      '--lang=en-US',
      '--hide-scrollbars',
      '--font-render-hinting=none',
      // The logos' server's certificate, and it alone.
      `--ignore-certificate-errors-spki-list=${certificate.spki}`,
    ],
  })
  cleanups.push(async () => {
    const chromeProcess = browser.process()
    await within(browser.close(), 10000)
    // Chrome's helpers can outlive it holding its output, and node would wait on that output
    // forever: they go with Chrome's process group, and node lets go of the output.
    try {
      process.kill(-chromeProcess.pid, 'SIGKILL')
    } catch {
      // The group is gone already.
    }
    chromeProcess.stdout?.destroy()
    chromeProcess.stderr?.destroy()
  })

  log('Drawing the generated pictures')
  const images = await drawPictures(browser)
  const video = makeVideo(scratch)
  const demo = await startDemo({
    posterDir: join(docsDir, 'public/posters'),
    images,
    video,
    certificate,
    rich,
  })
  cleanups.push(() => demo.close())
  log(`Demo addon at ${demo.manifestUrl}, logos at ${demo.secureUrl}`)

  const ownUrl = await createDatabase(databaseUrl, database)
  const server = await startPolyfin(polyfin, scratch, ownUrl, logName)
  log(`Polyfin at ${server.url}`)

  const api = client(server.url)
  await api.post('/admin/api/setup', {
    setupCode: server.setupCode,
    name: admin.name,
    password: admin.password,
    language: 'en',
  })
  return { browser, scratch, demo, server, api, databaseUrl: ownUrl, rich }
}

/**
 * A certificate of 127.0.0.1 for the logos' server, made by OpenSSL, and the hash of its public
 * key that Chrome is told to accept.
 */
function makeCertificate(scratch) {
  const key = join(scratch, 'logos-key.pem')
  const cert = join(scratch, 'logos-cert.pem')
  // prettier-ignore
  execFileSync('openssl', [
    'req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:prime256v1', '-nodes',
    '-keyout', key, '-out', cert, '-days', '2', '-subj', '/CN=127.0.0.1',
    '-addext', 'subjectAltName=IP:127.0.0.1',
  ], { stdio: ['ignore', 'ignore', 'pipe'] })
  const pem = { key: readFileSync(key), cert: readFileSync(cert) }
  const spki = new X509Certificate(pem.cert).publicKey.export({ type: 'spki', format: 'der' })
  return { ...pem, spki: createHash('sha256').update(spki).digest('base64') }
}

/**
 * Wild Atlas's poster, which the website lacks, the channels' logos and the sources' logos, drawn
 * by the browser. The logos have transparent parts, as most have: round ones, a wide wordmark, a
 * dark one that the dark theme hardly shows, a white one that a lit row hardly shows, and a
 * square one without any.
 */
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
  const initials = (name) =>
    name
      .split(' ')
      .filter((word) => /^[A-Z]/.test(word))
      .slice(0, 2)
      .map((word) => word[0])
      .join('')
  const draw = async (width, height, html) => {
    await page.setViewport({ width, height, deviceScaleFactor: 1 })
    await page.setContent(
      `<body style="margin:0;background:transparent;font:700 64px/1 system-ui,sans-serif">${html}</body>`,
    )
    return Buffer.from(await page.screenshot({ type: 'png', omitBackground: true }))
  }
  const round = (color, name) =>
    draw(
      256,
      256,
      `<div style="width:256px;height:256px;border-radius:56px;background:${color};display:flex;
        align-items:center;justify-content:center;color:#fff">${initials(name)}</div>`,
    )
  for (const channel of channels) {
    images.set(`logo-${channel.id}.png`, await round(channel.color, channel.name))
  }
  images.set(
    'source-demo-catalog.png',
    await draw(
      256,
      256,
      `<div style="width:256px;height:256px;border-radius:50%;background:#5b63f6;display:flex;
        align-items:center;justify-content:center;color:#fff">DC</div>`,
    ),
  )
  for (const source of sources) {
    const name = `source-${source.slug}.png`
    switch (source.logo) {
      case 'round':
      case 'http':
        images.set(
          name,
          await draw(
            256,
            256,
            `<div style="width:256px;height:256px;border-radius:50%;background:${source.color};
              display:flex;align-items:center;justify-content:center;color:#fff">${initials(source.name)}</div>`,
          ),
        )
        break
      case 'wordmark':
        images.set(
          name,
          await draw(
            600,
            200,
            `<div style="width:600px;height:200px;display:flex;align-items:center;justify-content:center;
              gap:24px;color:${source.color};font-size:88px;letter-spacing:-2px">
              <svg width="120" height="120" viewBox="0 0 10 10"><rect x="1" y="2" width="2" height="7" fill="currentColor"/>
              <rect x="4" y="1" width="2" height="8" fill="currentColor"/><rect x="7" y="3" width="2" height="6" fill="currentColor"/></svg>
              ${source.name.split(' ')[0]}</div>`,
          ),
        )
        break
      case 'dark':
        images.set(
          name,
          await draw(
            256,
            256,
            `<svg width="256" height="256" viewBox="0 0 16 16"><path d="M8 1 L15 5 L1 5 Z M2 6 H14 V7 H2 Z M3 8 H5 V13 H3 Z M7 8 H9 V13 H7 Z M11 8 H13 V13 H11 Z M1 14 H15 V15 H1 Z" fill="${source.color}"/></svg>`,
          ),
        )
        break
      case 'light':
        images.set(
          name,
          await draw(
            256,
            256,
            `<svg width="256" height="256" viewBox="0 0 16 16"><path d="M8 2 L14 7 H12 V14 H9 V10 H7 V14 H4 V7 H2 Z" fill="${source.color}"/></svg>`,
          ),
        )
        break
      case 'square':
        images.set(
          name,
          await draw(
            256,
            256,
            `<div style="width:256px;height:256px;background:${source.color};display:flex;
              align-items:center;justify-content:center;color:#fff">${initials(source.name)}</div>`,
          ),
        )
        break
    }
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

/** Runs SQL on a database, with parameters. */
async function query(url, sql, values) {
  const db = new pg.Client({ connectionString: url })
  await db.connect()
  try {
    return await db.query(sql, values)
  } finally {
    await db.end()
  }
}

/** Creates the scratch database afresh, and drops it on the way out. */
async function createDatabase(url, name) {
  if (!/^[a-z_][a-z0-9_]*$/.test(name)) throw new Error(`invalid database name ${name}`)
  await query(url, `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`)
  await query(url, `CREATE DATABASE ${name}`)
  cleanups.push(() => query(url, `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`))
  const own = new URL(url)
  own.pathname = `/${name}`
  return own.toString()
}

/** Starts Polyfin on free ports, and reads its setup code from its log. */
async function startPolyfin(polyfin, scratch, databaseUrl, logName) {
  const [http, discovery, dlna, hdhomerun] = await freePorts(4)
  const data = join(scratch, 'data')
  // A web player folder with an index.html turns the web player on, so the top bar links to it.
  const web = join(scratch, 'web')
  await mkdir(web, { recursive: true })
  await writeFile(join(web, 'index.html'), '<!doctype html><title>Web player</title>')
  const child = spawn(polyfin, ['serve'], {
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
  const logPath = join(tmpdir(), logName)
  let logText = ''
  const exited = new Promise((resolve) => child.on('exit', resolve))
  cleanups.push(async () => {
    if (child.exitCode === null) {
      child.kill('SIGTERM')
      await within(exited, 10000)
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

/** The admin API, as the user signed in: the session cookie of setup or sign-in is kept. */
export function client(base) {
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
    /** Signs in as a user, whose session the client keeps from then on. */
    signIn: (user) => api.post('/admin/api/session', { name: user.name, password: user.password }),
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

/**
 * Fills the demo server: the addon and its libraries, users, IPTV, home rows and two playbacks,
 * which the shots show; then, for a rich stage, what a real server has besides (see enrich).
 */
export async function seed(stage) {
  const { api, demo } = stage
  const addon = await api.post('/admin/api/scopes/shared/addons', { manifestUrl: demo.manifestUrl })

  const users = {}
  for (const member of members.slice(0, 3)) {
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

  const title = await play(api, {
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
  const playingSince = Date.now()
  const more = stage.rich ? await enrich(stage, { addon: addon.id, iptv: iptv.id, users }) : {}
  return { addon: addon.id, iptv: iptv.id, users, playingSince, title, ...more }
}

/**
 * What a real server has besides, in its shapes (counts and lengths of a real one), and the cases
 * others have: ten users in two groups, one with 23 devices; more sources, with logos, long names
 * and one that fails; six libraries hidden in menus; a folder source and a missing one; a failing
 * notification target and backup destination; tracking services connected; expired invites; a
 * long activity log; a smart list, an own channel and a member's own source.
 *
 * Tracking connections, the backup destination's failure, the invites' expiry and the activity
 * log are written in the database: the services are not reached from the demo, no backup runs
 * without pg_dump, and none of these can be made old through the API.
 */
async function enrich(stage, { addon, iptv, users }) {
  const { api, demo, scratch, databaseUrl } = stage

  for (const member of members.slice(3)) {
    const user = await api.post('/admin/api/users', { ...member, isAdministrator: false })
    users[member.name] = user.id
  }
  const groupIds = {}
  for (const [name, names] of Object.entries(groups)) {
    const group = await api.post('/admin/api/groups', { name })
    await api.post(`/admin/api/groups/${group.id}/members`, {
      userIds: names.map((each) => users[each]),
    })
    groupIds[name] = group.id
  }

  // 45 devices with the two playing, 23 of them Riley's, their names and apps as long as a real
  // server's.
  const apps = ['Jellyfin Android', 'Jellyfin Media Player', 'Jellyfin Web', 'Jellyfin iOS']
  // prettier-ignore
  const devices = [
    'Living room TV', 'Kitchen tablet', 'Bedroom TV', 'Office laptop', 'Upstairs bedroom TV',
    'Garage tablet', 'Spare phone', 'Guest room TV', 'Basement projector', 'Kids’ tablet',
    'Study laptop', 'Hallway display', 'Patio TV', 'Car tablet', 'Travel phone', 'Gym TV',
    'Attic laptop', 'Workshop tablet', 'Loft TV', 'Dining room display', 'Backup phone',
    'Old tablet', 'Porch TV',
  ]
  // prettier-ignore
  const deviceCounts = {
    Riley: 23, Sam: 2, Robin: 2, Alex: 3, Jordan: 3, Casey: 3, Taylor: 3, Quinn: 2, Avery: 2,
  }
  for (const [name, count] of Object.entries(deviceCounts)) {
    for (let index = 0; index < count; index++) {
      const app = jellyfinApp(api.base, {
        client: apps[index % apps.length],
        device: devices[index],
        deviceId: `demo-${name.toLowerCase()}-${index}`,
      })
      await app.signIn(members.find((member) => member.name === name))
    }
  }

  // More sources, then the one whose server fails is read again.
  const added = {}
  for (const source of sources) {
    const installed = await api.post('/admin/api/scopes/shared/addons', {
      manifestUrl: demo.sourceUrl(source),
    })
    added[source.slug] = installed.id
  }
  for (const source of sources.filter((each) => each.fails)) {
    demo.fail(source.slug)
    await api.post(`/admin/api/scopes/shared/addons/${added[source.slug]}/refresh`).catch(() => {})
  }

  // A folder of home videos, and one of movies whose folder is gone since.
  const homeVideos = join(scratch, 'media/home-videos')
  await mkdir(join(homeVideos, 'Summer by the lake'), { recursive: true })
  for (const file of ['Summer by the lake/Arrival.mp4', 'Summer by the lake/Fireworks.mp4']) {
    await writeFile(join(homeVideos, file), randomBytes(2048))
  }
  const folder = await api.post('/admin/api/scopes/shared/folders', {
    name: 'Home videos',
    path: homeVideos,
    kind: 'homevideos',
  })
  const gone = join(scratch, 'media/movies')
  await mkdir(gone, { recursive: true })
  const missing = await api.post('/admin/api/scopes/shared/folders', {
    name: 'Movies on the old disk',
    path: gone,
    kind: 'movies',
  })
  await rm(gone, { recursive: true, force: true })
  await api.post(`/admin/api/scopes/shared/folders/${missing.id}/scan`).catch(() => {})

  // Six libraries, hidden in Jellyfin apps' menus but the live TV ones, which cannot be: the Demo
  // Catalog's, Demo TV's, the folders' and Night Owl Picks' channels. Once the playbacks have
  // found their titles.
  const libraries = await api.get('/admin/api/scopes/shared/libraries')
  const kept = new Set([addon, iptv, folder.id, missing.id])
  const chosen = [
    ...libraries.filter((library) => kept.has(library.addonId)),
    ...libraries.filter(
      (library) => library.addonId === added['night-owl'] && library.catalogType === 'tv',
    ),
  ]
  await api.put('/admin/api/scopes/shared/libraries', {
    libraries: chosen.map((library) => ({
      addonId: library.addonId,
      catalogType: library.catalogType,
      catalogId: library.catalogId,
      name: library.name,
      hideInMenus: library.catalogType !== 'tv',
    })),
  })

  // A notification target that works, and one whose address is refused.
  const working = await api.post('/admin/api/notifications/targets', {
    kind: 'webhook',
    name: 'Server room',
    address: `${demo.url}/hooks/ok`,
    events: ['health_problem', 'health_solved', 'new_version'],
  })
  await api.post(`/admin/api/notifications/targets/${working.id}/test`)
  const refused = await api.post('/admin/api/notifications/targets', {
    kind: 'webhook',
    name: 'Weekend movie night reminders for the whole household',
    address: `${demo.url}/hooks/household`,
    events: ['new_episode', 'user_joined'],
  })
  await api.post(`/admin/api/notifications/targets/${refused.id}/test`)

  // A backup destination whose server is full: the test's answer, as a backup would record it.
  const destination = await api.post('/admin/api/backup/destinations', {
    kind: 'webdav',
    address: `${demo.url}/dav/polyfin-backups`,
    username: 'demo',
    secret: 'demo-secret',
    kept: 7,
  })
  const test = await api.post(`/admin/api/backup/destinations/${destination.id}/test`)
  await query(
    databaseUrl,
    `UPDATE backup_destinations SET ran_at = now() - interval '3 hours', error = $2 WHERE id = $1`,
    [destination.id, test.error || 'The WebDAV server answered 507 Insufficient Storage'],
  )

  // Five tracking services connected for Alex, each importing: every one that is not for music
  // but the first, as on a real server. Its last import is recent, so that none runs.
  const alex = client(api.base)
  await alex.signIn(members[2])
  const tracking = await alex.get('/admin/api/account/tracking')
  const connected = tracking.services
    .filter((service) => !service.music)
    .slice(1, 6)
    .map((service) => service.service)
  for (const service of connected) {
    await query(
      databaseUrl,
      `INSERT INTO tracking_connections (user_id, service, token, account, connected_at, last_sent_at)
        VALUES ($1, $2, 'demo-token', 'alex', now() - interval '40 days', now() - interval '2 hours')`,
      [users.Alex, service],
    )
    await query(
      databaseUrl,
      `INSERT INTO tracking_imports (user_id, service, enabled, last_import_at) VALUES ($1, $2, true, now())`,
      [users.Alex, service],
    )
  }

  // Five invites expired, and one open, which the sweep follows.
  for (let index = 0; index < 5; index++) {
    const invite = await api.post('/admin/api/invites', {
      maxUses: 1,
      expiresInDays: 7,
      modelUserId: null,
      accountsEndInDays: null,
      groupId: groupIds.Family,
    })
    await query(
      databaseUrl,
      `UPDATE invites SET created_at = now() - make_interval(days => $2 + 7),
        expires_at = now() - make_interval(days => $2) WHERE id = $1`,
      [invite.id, 10 + index * 9],
    )
  }
  const invite = await api.post('/admin/api/invites', {
    maxUses: 3,
    expiresInDays: 14,
    modelUserId: null,
    accountsEndInDays: null,
    groupId: groupIds['Weekend movie club'],
  })

  // An activity log as long as a real one's, its names up to 108 characters.
  await query(
    databaseUrl,
    `INSERT INTO activity_log (name, short_overview, type, user_id, date, severity)
      SELECT CASE n % 3
          WHEN 0 THEN u.name || ' is playing Night Train on Living room TV'
          WHEN 1 THEN u.name || ' signed in'
          ELSE u.name || ' stopped playing Glass Garden, season 1, episode 4, In Bloom (extended version) on Upstairs bedroom TV'
        END,
        CASE n % 3 WHEN 1 THEN 'Address: 127.0.0.1' END,
        CASE n % 3 WHEN 0 THEN 'VideoPlayback' WHEN 1 THEN 'AuthenticationSucceeded' ELSE 'VideoPlaybackStopped' END,
        u.id, now() - make_interval(mins => n * 50), 'Information'
      FROM generate_series(1, 1150) n
      JOIN LATERAL (SELECT id, name FROM users ORDER BY name OFFSET (n % 10) LIMIT 1) u ON true`,
  )

  // A smart list of the server's, an own channel airing it, the administrator's own source (a
  // member's may not be on the local network, as the demo's are), and a member's own list.
  const smart = await api.post('/admin/api/scopes/shared/smart-lists', {
    name: 'Rainy day movies',
    shownAs: 'library',
    catalogs: [{ addonId: addon, type: 'movie', id: 'featured' }],
    rules: { field: 'group', match: 'all', rules: [] },
    exclude: { field: 'group', match: 'any', rules: [] },
    sort: 'catalog',
    maxItems: null,
  })
  const { smartLists } = await api.get('/admin/api/live-tv/channels/choices')
  const choice = smartLists.find((each) => each.name === 'Rainy day movies')
  const channel = await api.post('/admin/api/live-tv/channels', {
    name: 'Rainy Day Cinema',
    number: null,
    enabled: true,
    source: { kind: 'smartlist', items: [{ id: choice.id, name: choice.name, kind: 'smartlist' }] },
    shuffle: false,
    blocks: [],
  })
  const settings = await api.get('/admin/api/settings')
  await api.put('/admin/api/settings', { ...settings, personalAddons: true, bounds: undefined })
  // Alex follows a group, whose permission is theirs: My sources offers them to add one.
  await api.patch(`/admin/api/groups/${groupIds['Weekend movie club']}`, { personalAddons: true })
  const own = await api.post('/admin/api/scopes/me/addons', {
    manifestUrl: demo.sourceUrl(sources.find((source) => source.slug === 'festival-reels')),
  })
  const ownSmart = await alex.post('/admin/api/scopes/me/smart-lists', {
    name: 'Alex’s favourites',
    shownAs: 'collection',
    catalogs: [{ addonId: addon, type: 'series', id: 'featured' }],
    rules: { field: 'group', match: 'all', rules: [] },
    exclude: { field: 'group', match: 'any', rules: [] },
    sort: 'name',
    maxItems: 50,
  })

  return {
    sources: added,
    folder: folder.id,
    groups: groupIds,
    invite: invite.token,
    smartList: smart.id,
    channel: channel.id,
    own: own.id,
    ownSmartList: ownSmart.id,
  }
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
  return item.Id
}
