// Measures what a user feels in jellyfin-web served by Polyfin, and what a native app waits for:
// opening a title from a home row, pressing Play, and a native app's calls up to its stream's
// first bytes, against a fixture addon whose streams answer late, as an aggregator's do. Each
// is run warm and cold, and each run's requests are kept, to tell where the time goes. See
// README.md.
import { execFileSync } from 'node:child_process'
import { existsSync, readdirSync, statSync } from 'node:fs'
import { cp, mkdir, mkdtemp, open, readFile, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { parseArgs } from 'node:util'
import puppeteer from 'puppeteer-core'
import { adminClient, atExit, log, query, repoDir, run, sleep, startPolyfin } from './lib.mjs'

/** The users: one keeps the server's theme, LumaaGlaass, the other has jellyfin-web's own look. */
const users = {
  themed: { name: 'Riley', password: 'fluidity-password-1', webTheme: '' },
  plain: { name: 'Sam', password: 'fluidity-password-2', webTheme: 'none' },
}
const admin = { name: 'Morgan', password: 'fluidity-password-0' }

/** The versions' files: one that plays as it is, one repackaged, one converted. */
const kinds = {
  direct: { file: 'direct.mp4', type: 'video/mp4', label: 'MP4 H.264/AAC' },
  repack: { file: 'repack.mkv', type: 'video/x-matroska', label: 'MKV H.264/AAC' },
  convert: { file: 'convert.mkv', type: 'video/x-matroska', label: 'MKV HEVC/AAC' },
}

/**
 * The cases measured. Each has a catalog, a library, of its own, with a title per cold run
 * and one for the warm runs, so that a cold run opens a title never opened.
 */
const cells = [
  {
    id: 'open-themed',
    scenario: 'open',
    user: 'themed',
    kind: 'direct',
    name: 'Open, LumaaGlaass',
  },
  {
    id: 'open-plain',
    scenario: 'open',
    user: 'plain',
    kind: 'direct',
    name: "Open, jellyfin-web's look",
  },
  {
    id: 'play-now',
    scenario: 'play',
    user: 'themed',
    kind: 'direct',
    wait: 0,
    name: 'Play at once, MP4',
  },
  {
    id: 'play-ahead',
    scenario: 'play',
    user: 'themed',
    kind: 'direct',
    wait: 5000,
    name: 'Play 5 s after opening, MP4',
  },
  {
    id: 'repack-now',
    scenario: 'play',
    user: 'themed',
    kind: 'repack',
    wait: 0,
    name: 'Play at once, MKV',
  },
  {
    id: 'repack-ahead',
    scenario: 'play',
    user: 'themed',
    kind: 'repack',
    wait: 5000,
    name: 'Play 5 s after opening, MKV',
  },
  {
    id: 'convert-now',
    scenario: 'play',
    user: 'themed',
    kind: 'convert',
    wait: 0,
    name: 'Play at once, HEVC',
  },
  {
    id: 'convert-ahead',
    scenario: 'play',
    user: 'themed',
    kind: 'convert',
    wait: 5000,
    name: 'Play 5 s after opening, HEVC',
  },
  {
    id: 'native-now',
    scenario: 'native',
    kind: 'direct',
    wait: 0,
    name: 'Native app, Play at once',
  },
  {
    id: 'native-ahead',
    scenario: 'native',
    kind: 'direct',
    wait: 5000,
    name: 'Native app, Play 5 s after opening',
  },
]

const values = parseArgs({
  options: {
    'database-url': { type: 'string', default: process.env.BENCH_DATABASE_URL },
    database: { type: 'string', default: 'polyfin_fluidity' },
    polyfin: { type: 'string', default: process.env.POLYFIN_BIN ?? join(repoDir, 'bin/polyfin') },
    web: { type: 'string', default: process.env.POLYFIN_WEB_DIR },
    chrome: { type: 'string', default: process.env.CHROME_PATH ?? defaultChrome() },
    runs: { type: 'string', default: '10' },
    modes: { type: 'string', default: 'warm,cold' },
    only: { type: 'string', default: '' },
    'stream-delay': { type: 'string', default: '1500' },
    latency: { type: 'string', default: '50' },
    'source-rate': { type: 'string', default: '12' },
    hevc: { type: 'boolean', default: false },
    mkv: { type: 'boolean', default: false },
    json: { type: 'string' },
    report: { type: 'string' },
    log: { type: 'string' },
    shots: { type: 'string' },
    from: { type: 'string' },
    keep: { type: 'boolean', default: false },
  },
}).values

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

// ---------------------------------------------------------------------------------------------
// The fixture: a Stremio addon of a catalog per case, whose streams answer after
// --stream-delay, and whose every answer, pictures and files included, waits --latency first,
// as over the internet. Files are sent at --source-rate MB/s.

const ffmpeg = process.env.POLYFIN_FFMPEG ?? 'ffmpeg'

/**
 * The versions' files and the backdrops, made once with FFmpeg and kept in the system's
 * temporary folder: 3 minutes of 1080p at 24 pictures a second, H.264 at 8 Mbps and AAC in
 * MP4, the same in Matroska, and HEVC at 6 Mbps in Matroska. Titles say they last 20 minutes,
 * so that Polyfin takes the files for the title rather than for a clip.
 */
async function makeMedia() {
  const dir = join(tmpdir(), 'polyfin-fluidity-media')
  await mkdir(dir, { recursive: true })
  const make = (name, args) => {
    const path = join(dir, name)
    if (existsSync(path) && statSync(path).size > 0) return
    log(`Making ${name}`)
    execFileSync(ffmpeg, [
      '-y',
      '-loglevel',
      'error',
      ...args,
      `${path}.part.${name.split('.').pop()}`,
    ])
    execFileSync('mv', [`${path}.part.${name.split('.').pop()}`, path])
  }
  // prettier-ignore
  make('direct.mp4', [
    '-f', 'lavfi', '-i', 'testsrc2=size=1920x1080:rate=24',
    '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000', '-t', '180',
    '-c:v', 'libx264', '-preset', 'veryfast', '-profile:v', 'high',
    '-b:v', '8M', '-maxrate', '10M', '-bufsize', '16M',
    '-c:a', 'aac', '-b:a', '192k', '-ac', '2', '-movflags', '+faststart',
  ])
  make('repack.mkv', ['-i', join(dir, 'direct.mp4'), '-c', 'copy'])
  // prettier-ignore
  make('convert.mkv', [
    '-i', join(dir, 'direct.mp4'),
    '-c:v', 'libx265', '-preset', 'faster', '-b:v', '6M', '-tag:v', 'hvc1',
    '-x265-params', 'log-level=error', '-c:a', 'copy',
  ])
  for (let i = 0; i < 6; i++) {
    // prettier-ignore
    make(`backdrop-${i}.jpg`, [
      '-f', 'lavfi', '-i', `gradients=size=1920x1080:seed=${i + 1}:speed=0.01`,
      '-frames:v', '1', '-q:v', '3',
    ])
  }
  const files = {}
  for (const [kind, { file }] of Object.entries(kinds)) {
    files[kind] = { path: join(dir, file), size: statSync(join(dir, file)).size }
  }
  return { dir, files }
}

/** The titles of each case: one for the warm runs, then one per cold run. */
function titlesOf(cell, runs) {
  return Array.from({ length: runs + 1 }, (_, n) => ({
    id: `fluid:${cell.id}-${n}`,
    name: `${cell.id} ${String(n).padStart(2, '0')}`,
    n,
  }))
}

async function startFixture({ media, runs, latency, streamDelay, sourceRate }) {
  let base = ''
  const posterDir = join(repoDir, 'docs/public/posters')
  const posters = readdirSync(posterDir).filter(
    (name) => /^[a-z-]+[a-z]\.jpg$/.test(name) && !name.includes('-sq'),
  )
  const all = cells.flatMap((cell) => titlesOf(cell, runs).map((title) => ({ ...title, cell })))
  const byId = new Map(all.map((title, index) => [title.id, { ...title, index }]))
  /** What the fixture answered: each request's path, when it came and when its answer ended. */
  const events = []
  const meta = (title, full) => ({
    id: title.id,
    type: 'movie',
    name: title.name,
    poster: `${base}/posters/${title.id}.jpg`,
    background: `${base}/backdrops/${title.id}.jpg`,
    releaseInfo: '2024',
    year: '2024',
    genres: ['Drama'],
    ...(full
      ? {
          description: `A title of the fluidity bench, case ${title.cell.id}.`,
          runtime: '20 min',
          released: '2024-03-14T00:00:00.000Z',
        }
      : {}),
  })
  const json = (response, body) => {
    response.writeHead(200, { 'content-type': 'application/json' })
    response.end(JSON.stringify(body))
  }
  const send = (response, type, body) => {
    response.writeHead(200, { 'content-type': type, 'content-length': body.length })
    response.end(body)
  }
  const handle = async (request, response) => {
    const path = decodeURIComponent(new URL(request.url, 'http://x').pathname)
    const event = { path, method: request.method, range: request.headers.range, at: Date.now() }
    events.push(event)
    response.on('close', () => (event.end = Date.now()))
    try {
      await sleep(latency)
      if (path === '/manifest.json') {
        return json(response, {
          id: 'org.polyfin.fluidity',
          version: '1.0.0',
          name: 'Fluidity bench',
          description: 'Titles of the fluidity bench.',
          resources: ['catalog', 'meta', 'stream'],
          types: ['movie'],
          idPrefixes: ['fluid:'],
          catalogs: cells.map((cell) => ({ type: 'movie', id: cell.id, name: cell.id })),
        })
      }
      let match = path.match(/^\/catalog\/movie\/([a-z-]+)(\/[^/]*)?\.json$/)
      if (match) {
        const cell = cells.find((each) => each.id === match[1])
        const skip = Number(/skip=(\d+)/.exec(match[2] ?? '')?.[1] ?? 0)
        const metas =
          cell && skip === 0 ? titlesOf(cell, runs).map((t) => meta({ ...t, cell }, false)) : []
        return json(response, { metas })
      }
      match = path.match(/^\/meta\/movie\/(fluid:[a-z0-9-]+)\.json$/)
      if (match) {
        const title = byId.get(match[1])
        return json(response, { meta: title ? meta(title, true) : null })
      }
      match = path.match(/^\/stream\/movie\/(fluid:[a-z0-9-]+)\.json$/)
      if (match) {
        event.kind = 'stream'
        await sleep(Math.max(0, streamDelay - latency))
        const title = byId.get(match[1])
        if (!title) return json(response, { streams: [] })
        const kind = title.cell.kind
        const extension = kinds[kind].file.split('.').pop()
        return json(response, {
          streams: [
            {
              name: 'Fluidity\n1080p',
              title: `${title.name} · 1080p · ${kinds[kind].label}`,
              url: `${base}/video/${title.id}.${extension}`,
              behaviorHints: {
                filename: `${title.id}.${extension}`,
                videoSize: media.files[kind].size,
              },
            },
          ],
        })
      }
      match = path.match(/^\/(posters|backdrops)\/(fluid:[a-z0-9-]+)\.jpg$/)
      if (match && byId.has(match[2])) {
        event.kind = 'image'
        const { index } = byId.get(match[2])
        const file =
          match[1] === 'posters'
            ? join(posterDir, posters[index % posters.length])
            : join(media.dir, `backdrop-${index % 6}.jpg`)
        return send(response, 'image/jpeg', await readFile(file))
      }
      match = path.match(/^\/video\/(fluid:[a-z0-9-]+)\.(mp4|mkv)$/)
      if (match && byId.has(match[1])) {
        event.kind = 'video'
        const kind = byId.get(match[1]).cell.kind
        return await sendVideo(
          request,
          response,
          media.files[kind],
          kinds[kind].type,
          sourceRate,
          event,
        )
      }
      response.writeHead(404)
      response.end()
    } catch {
      if (!response.headersSent) response.writeHead(500)
      response.end()
    }
  }
  const server = createServer(handle)
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  base = `http://127.0.0.1:${server.address().port}`
  atExit(() => {
    server.closeAllConnections()
    return new Promise((resolve) => server.close(resolve))
  })
  return { url: base, manifestUrl: `${base}/manifest.json`, events, titles: all }
}

/** Sends a file with the range asked for, at rate bytes a second. */
async function sendVideo(request, response, file, type, rate, event) {
  const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range ?? '')
  const start = range ? Number(range[1]) : 0
  const end = range?.[2] ? Math.min(Number(range[2]), file.size - 1) : file.size - 1
  response.writeHead(range ? 206 : 200, {
    'content-type': type,
    'content-length': end - start + 1,
    'accept-ranges': 'bytes',
    'access-control-allow-origin': '*',
    ...(range ? { 'content-range': `bytes ${start}-${end}/${file.size}` } : {}),
  })
  event.bytes = 0
  if (request.method === 'HEAD') return response.end()
  const handle = await open(file.path)
  try {
    const chunk = Buffer.alloc(256 * 1024)
    const began = Date.now()
    for (let at = start; at <= end && !response.destroyed;) {
      const { bytesRead } = await handle.read(chunk, 0, Math.min(chunk.length, end + 1 - at), at)
      if (!bytesRead) break
      if (!response.write(Buffer.from(chunk.subarray(0, bytesRead))) && !response.destroyed) {
        await new Promise((resolve) => {
          const done = () => {
            response.off('drain', done)
            response.off('close', done)
            resolve()
          }
          response.on('drain', done)
          response.on('close', done)
        })
      }
      at += bytesRead
      event.bytes += bytesRead
      const due = began + (event.bytes / rate) * 1000
      if (due > Date.now()) await sleep(due - Date.now())
    }
  } finally {
    await handle.close()
  }
  response.end()
}

// ---------------------------------------------------------------------------------------------
// The stage: a scratch database, Polyfin with jellyfin-web, the users and the addon.

async function startStage(media, runs) {
  const name = values.database
  if (!/^[a-z_][a-z0-9_]*$/.test(name)) throw new Error(`invalid database name ${name}`)
  const server = values['database-url']
  await query(server, `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`)
  await query(server, `CREATE DATABASE ${name}`)
  atExit(() => query(server, `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`))
  const own = new URL(server)
  own.pathname = `/${name}`
  const dir = await mkdtemp(join(tmpdir(), 'polyfin-fluidity-'))
  atExit(() => rm(dir, { recursive: true, force: true }))
  // Polyfin's log, written as it stops, is kept with --log.
  if (values.log) atExit(() => cp(join(dir, 'polyfin.log'), values.log).catch(() => {}))

  const fixture = await startFixture({
    media,
    runs,
    latency: Number(values.latency),
    streamDelay: Number(values['stream-delay']),
    sourceRate: Number(values['source-rate']) * 1_000_000,
  })
  log(`Fixture addon at ${fixture.manifestUrl}`)
  const polyfin = await startPolyfin(values, own.toString(), dir)
  log(`Polyfin at ${polyfin.url}`)
  const api = adminClient(polyfin.url)
  let code
  for (let i = 0; i < 300 && !code; i++) {
    code = polyfin.log().match(/setup_code=([A-Z0-9]{4}-[A-Z0-9]{4})/)?.[1]
    if (!code) await sleep(100)
  }
  await api.post('/admin/api/setup', { setupCode: code, ...admin, language: 'en' })
  for (const user of Object.values(users)) {
    const made = await api.post('/admin/api/users', {
      name: user.name,
      password: user.password,
      isAdministrator: false,
    })
    if (user.webTheme) {
      await api.patch(`/admin/api/users/${made.id}`, { webTheme: user.webTheme })
    }
  }
  await api.post('/admin/api/scopes/shared/addons', { manifestUrl: fixture.manifestUrl })

  // The titles' identifiers, as each user's libraries list them: listing them stores them.
  const ids = new Map()
  const app = await nativeApp(polyfin.url, users.themed, 'fluidity-setup')
  for (let tries = 0; ids.size < fixture.titles.length; tries++) {
    if (tries > 150) throw new Error(`only ${ids.size} of ${fixture.titles.length} titles listed`)
    const views = (await app.get(`/UserViews?userId=${app.userId}`)).Items
    for (const view of views) {
      const items = await app.get(
        `/Items?userId=${app.userId}&ParentId=${view.Id}&Recursive=true&IncludeItemTypes=Movie&Limit=200`,
      )
      for (const item of items.Items) ids.set(item.Name, item.Id)
    }
    if (ids.size < fixture.titles.length) await sleep(200)
  }
  for (const title of fixture.titles) title.itemId = ids.get(title.name)
  await polyfin.conversionsDone()
  return { polyfin, fixture, api, dir }
}

// ---------------------------------------------------------------------------------------------
// A native-like app: the Jellyfin API over fetch, with the device profile of an app that plays
// MP4 and Matroska, H.264 and HEVC, as players built on FFmpeg do.

async function nativeApp(base, user, deviceId) {
  const header = (token) =>
    `MediaBrowser Client="Fluidity Native", Device="Bench", DeviceId="${deviceId}", Version="1.0.0"${token ? `, Token="${token}"` : ''}`
  let token = ''
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
    if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${text.slice(0, 300)}`)
    return { json: text ? JSON.parse(text) : null, bytes: text.length }
  }
  const signedIn = await call('POST', '/Users/AuthenticateByName', {
    Username: user.name,
    Pw: user.password,
  })
  token = signedIn.json.AccessToken
  return {
    userId: signedIn.json.User.Id,
    token,
    call,
    get: async (path) => (await call('GET', path)).json,
    authorization: () => header(token),
  }
}

const nativeProfile = {
  MaxStreamingBitrate: 120_000_000,
  MaxStaticBitrate: 120_000_000,
  DirectPlayProfiles: [
    {
      Container: 'mp4,m4v,mkv,mov,webm',
      Type: 'Video',
      VideoCodec: 'h264,hevc,av1,vp9',
      AudioCodec: 'aac,ac3,eac3,mp3,opus,flac',
    },
  ],
  TranscodingProfiles: [
    {
      Container: 'ts',
      Type: 'Video',
      VideoCodec: 'h264,hevc',
      AudioCodec: 'aac,ac3,eac3',
      Protocol: 'hls',
      Context: 'Streaming',
    },
  ],
  SubtitleProfiles: [{ Format: 'srt', Method: 'External' }],
}

/**
 * A native app opening a title, then playing it: its details, after the wait its PlaybackInfo,
 * then its stream's first MiB, as a player reads before it shows the first picture.
 */
async function nativeRun(app, base, title, wait) {
  const steps = []
  const step = async (name, task) => {
    const started = performance.now()
    const bytes = await task()
    steps.push({ name, ms: performance.now() - started, bytes })
  }
  const began = Date.now()
  await step(
    'Details',
    async () => (await app.call('GET', `/Items/${title.itemId}?userId=${app.userId}`)).bytes,
  )
  if (wait) await sleep(Math.max(0, wait - (Date.now() - began)))
  const playStart = Date.now()
  let info
  await step('PlaybackInfo', async () => {
    const answer = await app.call(
      'POST',
      `/Items/${title.itemId}/PlaybackInfo?userId=${app.userId}&autoOpenLiveStream=true`,
      { UserId: app.userId, AutoOpenLiveStream: true, DeviceProfile: nativeProfile },
    )
    info = answer.json
    return answer.bytes
  })
  const source = info.MediaSources?.[0]
  if (!source) throw new Error(`no version of ${title.name}`)
  const url = source.SupportsDirectPlay
    ? `${base}/Videos/${title.itemId}/stream?static=true&mediaSourceId=${source.Id}&playSessionId=${info.PlaySessionId}&api_key=${app.token}`
    : `${base}${source.TranscodingUrl}`
  await step('Stream, first MiB', async () => {
    const response = await fetch(url, { headers: { range: 'bytes=0-' } })
    let read = 0
    for await (const chunk of response.body) {
      read += chunk.length
      if (read >= 1 << 20) break
    }
    return read
  })
  const play = steps.slice(1).reduce((total, each) => total + each.ms, 0)
  return {
    ms: play + steps[0].ms,
    playMs: play,
    began,
    playStart,
    method: source.SupportsDirectPlay
      ? 'DirectPlay'
      : source.SupportsDirectStream
        ? 'DirectStream'
        : 'Transcode',
    steps,
  }
}

// ---------------------------------------------------------------------------------------------
// jellyfin-web in Chrome, with the requests of each run from the DevTools protocol.

/**
 * Installed in every page before its scripts: the times of the video's `playing` events. Without
 * mkv, the page's videos say they cannot play Matroska, as many browsers' do, so
 * that jellyfin-web asks for it repackaged: Chrome plays it.
 */
function pageHooks(mkv) {
  window.__fluidity = { playing: [] }
  document.addEventListener(
    'playing',
    (event) => {
      if (event.target instanceof HTMLVideoElement) {
        window.__fluidity.playing.push(performance.timeOrigin + performance.now())
      }
    },
    true,
  )
  if (!mkv) {
    const canPlayType = HTMLMediaElement.prototype.canPlayType
    HTMLMediaElement.prototype.canPlayType = function (type) {
      return /matroska|mkv/i.test(type) ? '' : canPlayType.call(this, type)
    }
  }
}

/** A browser tab whose requests are kept, with their timing. */
async function openTab(context) {
  const page = await context.newPage()
  await page.setViewport({ width: 1440, height: 900 })
  await page.evaluateOnNewDocument(pageHooks, values.mkv)
  await page.evaluateOnNewDocument(
    `window.detailsUsable = ${detailsUsable}; window.pressPlay = ${pressPlay}`,
  )
  const cdp = await page.createCDPSession()
  await cdp.send('Network.enable')
  const requests = new Map()
  const all = []
  cdp.on('Network.requestWillBeSent', (event) => {
    if (!/^https?:/.test(event.request.url)) return
    const previous = requests.get(event.requestId)
    if (previous && event.redirectResponse) previous.redirected = true
    const record = {
      id: event.requestId,
      url: event.request.url,
      method: event.request.method,
      type: event.type,
      start: event.wallTime * 1000,
      mono: event.timestamp,
      bytes: 0,
    }
    requests.set(event.requestId, record)
    all.push(record)
  })
  cdp.on('Network.requestServedFromCache', (event) => {
    const record = requests.get(event.requestId)
    if (record) record.cached = true
  })
  cdp.on('Network.responseReceived', (event) => {
    const record = requests.get(event.requestId)
    if (!record) return
    const { timing } = event.response
    record.status = event.response.status
    record.cached ||= event.response.fromDiskCache || event.response.fromServiceWorker
    record.size = Number(
      event.response.headers['content-length'] ?? event.response.headers['Content-Length'] ?? 0,
    )
    if (timing) {
      const at = (ms) => record.start + (timing.requestTime - record.mono) * 1000 + ms
      record.sent = at(timing.sendEnd)
      record.headers = at(timing.receiveHeadersEnd)
      record.wait = timing.receiveHeadersEnd - timing.sendEnd
    } else {
      record.headers = record.start + (event.timestamp - record.mono) * 1000
    }
  })
  cdp.on('Network.dataReceived', (event) => {
    const record = requests.get(event.requestId)
    if (!record) return
    record.bytes += event.encodedDataLength
    record.firstByte ??= record.start + (event.timestamp - record.mono) * 1000
  })
  const finish = (event, failed) => {
    const record = requests.get(event.requestId)
    if (!record) return
    record.end = record.start + (event.timestamp - record.mono) * 1000
    if (event.encodedDataLength !== undefined) record.bytes = event.encodedDataLength
    if (failed) record.failed = event.errorText || 'failed'
    requests.delete(event.requestId)
  }
  cdp.on('Network.loadingFinished', (event) => finish(event, false))
  cdp.on('Network.loadingFailed', (event) => finish(event, true))
  return {
    page,
    /** Requests from a time on. */
    since: (time) => all.filter((record) => record.start >= time),
    forget: () => all.splice(0, all.length),
    /** How the version played: the PlaybackInfo answer after a time, read back. */
    async playMethod(time) {
      const info = all.find(
        (record) => record.start >= time - 5 && /\/PlaybackInfo/i.test(record.url),
      )
      if (!info) return 'no PlaybackInfo'
      try {
        const { body } = await cdp.send('Network.getResponseBody', { requestId: info.id })
        const source = JSON.parse(body).MediaSources?.[0]
        if (!source) return 'no version'
        if (source.SupportsDirectPlay && !source.TranscodingUrl) return 'DirectPlay'
        const reasons = /TranscodeReasons=([^&]*)/i.exec(source.TranscodingUrl ?? '')?.[1] ?? ''
        const decoded = decodeURIComponent(reasons)
        const converted = decoded.split(',').some((reason) => reason && !/^Container/.test(reason))
        return `${converted ? 'Convert' : 'Repackage'}${decoded ? ` (${decoded})` : ''}`
      } catch (error) {
        return `unknown: ${error.message}`
      }
    },
    /**
     * Waits until no request but a socket or a video's has been under way for ms. A request
     * under way for 10 seconds is left out: the protocol never tells the end of some, such as
     * a worker's script, which its own target loads.
     */
    async quiet(ms = 1000, at = 20000) {
      let still = 0
      for (let waited = 0; still < ms && waited < at; waited += 50) {
        await sleep(50)
        const busy = [...requests.values()].some(
          (record) =>
            record.type !== 'WebSocket' &&
            record.type !== 'Media' &&
            record.type !== 'EventSource' &&
            Date.now() - record.start < 10000,
        )
        still = busy ? 0 : still + 50
      }
    },
  }
}

async function signIn(tab, base, user) {
  const { page } = tab
  await page.goto(`${base}/web/index.html#/login`, { waitUntil: 'networkidle0' })
  const manual = await page.$('.btnManual')
  if (manual && (await manual.isVisible())) await manual.click()
  await page.waitForSelector('#txtManualName', { visible: true })
  await page.type('#txtManualName', user.name)
  await page.type('#txtManualPassword', user.password)
  await page.click('.manualLoginForm button[type=submit]')
  await page.waitForFunction(() => /#\/home/.test(location.hash), { timeout: 30000 })
  await tab.quiet(1500)
}

/** Goes to the home page, and waits until it has settled and the title's card is on it. */
async function goHome(tab, title) {
  const { page } = tab
  if (!/#\/home/.test(await page.evaluate(() => location.hash))) {
    await page.evaluate(() => (location.hash = '#/home'))
  }
  await page.waitForFunction(
    (id) => document.querySelector(`.page:not(.hide) .card[data-id="${id}"]`),
    { timeout: 30000 },
    title.itemId,
  )
  await tab.quiet(1000)
}

/**
 * Whether the title's page is usable: its name, its backdrop drawn and its Play button, which
 * Polyfin's script holds, disabled, until a version is known. Runs in the page, each frame;
 * keeps when the page was first shown, Play held or not, and answers once Play can be pressed.
 */
function detailsUsable(name) {
  const shown = (element) => {
    if (!element) return false
    const box = element.getBoundingClientRect()
    const style = getComputedStyle(element)
    return box.width > 0 && box.height > 0 && style.visibility !== 'hidden' && style.opacity !== '0'
  }
  const page = document.querySelector('#itemDetailPage:not(.hide)')
  if (!page) return false
  const title = [...page.querySelectorAll('.itemName, h1, h3')].find(
    (element) => element.textContent.includes(name) && shown(element),
  )
  if (!title) return false
  const play = [...page.querySelectorAll('.btnPlay, .btnResume')].find(shown)
  if (!play) return false
  // The picture behind the page, drawn once it loaded: jellyfin-web's backdrop behind every
  // page, the page's own, or with LumaaGlaass the poster it shows there for a title without
  // one on the page.
  const urls = []
  const add = (image, kind) => {
    const url = /url\("?([^")]+)"?\)/.exec(image ?? '')?.[1]
    if (url) urls.push({ url: new URL(url, location.href).href, kind })
  }
  for (const element of document.querySelectorAll('.backdropImage, .itemBackdrop')) {
    if (!shown(element)) continue
    add(element.style.backgroundImage || getComputedStyle(element).backgroundImage, 'backdrop')
    if (element.classList.contains('lg-is-fallback')) {
      add(element.style.getPropertyValue('--lg-detail-fallback-backdrop'), 'poster')
    }
  }
  const drawn = urls.find(({ url }) =>
    performance.getEntriesByName(url).some((entry) => entry.responseEnd > 0),
  )
  if (!drawn) return false
  const now = performance.timeOrigin + performance.now()
  window.__fluidity.shownAt ||= now
  if (play.disabled || play.classList.contains('polyfinHeld')) return false
  return { at: now, shownAt: window.__fluidity.shownAt, backdrop: drawn.kind }
}

/** Presses Play on the title's page, in the page; answers the time it did. */
function pressPlay() {
  const page = document.querySelector('#itemDetailPage:not(.hide)')
  const button = [...page.querySelectorAll('.btnPlay, .btnResume')].find(
    (each) => each.offsetParent,
  )
  const at = performance.timeOrigin + performance.now()
  button.click()
  return at
}

/** Clicks the title's card on the home page, and answers the time of the click. */
async function clickCard(tab, title) {
  return tab.page.evaluate((id) => {
    const card = document.querySelector(`.page:not(.hide) .card[data-id="${id}"]`)
    card.scrollIntoView({ block: 'center', inline: 'center' })
    const target = card.querySelector('.cardImageContainer, .itemAction, a, button') ?? card
    const at = performance.timeOrigin + performance.now()
    window.__fluidity.shownAt = 0
    target.click()
    return at
  }, title.itemId)
}

/**
 * Opens a title from the home page: the time until its page is usable, and its requests. With
 * press, Play is pressed in the frame the page became usable.
 */
async function openRun(tab, base, title, press = false) {
  await goHome(tab, title)
  await tab.page.evaluate(() => performance.setResourceTimingBufferSize(100000))
  const clicked = await clickCard(tab, title)
  const handle = await tab.page.waitForFunction(
    (name, press) => {
      const usable = detailsUsable(name)
      if (usable && press) usable.pressed = pressPlay()
      return usable
    },
    { polling: 'raf', timeout: 60000 },
    title.name,
    press,
  )
  const usable = await handle.jsonValue()
  if (!press) await tab.quiet(1000)
  return {
    clicked,
    usable: usable.at,
    pressed: usable.pressed,
    backdrop: usable.backdrop,
    shownMs: usable.shownAt - clicked,
    ms: usable.at - clicked,
    requests: describe(tab.since(clicked - 5), clicked, base),
  }
}

/**
 * Presses Play on the title's page, wait ms after its card was clicked, else as soon as Play
 * can be pressed: the time until the video plays.
 */
async function playRun(tab, base, title, wait) {
  const before = await tab.page.evaluate(() => window.__fluidity.playing.length)
  const opened = await openRun(tab, base, title, !wait)
  let pressed = opened.pressed
  if (wait) {
    await sleep(Math.max(0, wait - (Date.now() - opened.clicked)))
    pressed = await tab.page.evaluate(() => pressPlay())
  }
  const handle = await tab.page.waitForFunction(
    (count) => window.__fluidity.playing.length > count && window.__fluidity.playing[count],
    { polling: 'raf', timeout: 90000 },
    before,
  )
  const playing = await handle.jsonValue()
  const method = await tab.playMethod(pressed)
  await sleep(300)
  const requests = describe(tab.since(pressed - 5), pressed, base)
  // Stops the video as Back does, and waits for the stop to be told.
  await tab.page.evaluate(() => history.back())
  await tab.page
    .waitForFunction(() => !document.querySelector('video'), { timeout: 20000 })
    .catch(() => {})
  await tab.quiet(1000)
  return {
    opened: opened.ms,
    openedShown: opened.shownMs,
    clicked: pressed,
    ms: playing - pressed,
    fromCard: playing - opened.clicked,
    method,
    requests,
  }
}

/** A run's requests, times from t0, with what they are of. */
function describe(records, t0, base) {
  const origin = new URL(base).origin
  return records.map((record) => {
    const url = new URL(record.url)
    const path = url.pathname + url.search
    const own = url.origin === origin
    const kind = !own
      ? record.type === 'Media' || /\/video\//.test(url.pathname)
        ? 'source'
        : 'other'
      : /^\/web\//.test(url.pathname) && !/\/web\/(polyfin|lumaaglaass)/i.test(url.pathname)
        ? 'asset'
        : /\/Images\//i.test(url.pathname)
          ? 'image'
          : /\.m3u8|\/hls\d?\/|\.ts$|\.mp4$|\/stream/i.test(url.pathname)
            ? 'media'
            : /^\/web\//.test(url.pathname) || /Branding|polyfin\.js|\.css$/i.test(url.pathname)
              ? 'theme'
              : 'api'
    const at = (time) => (time === undefined ? undefined : Math.round(time - t0))
    return {
      path: own ? path : `${url.host}${path}`,
      kind,
      method: record.method,
      status: record.status,
      start: at(record.start),
      headers: at(record.headers),
      end: at(record.end),
      wait: record.wait === undefined ? undefined : Math.round(record.wait),
      bytes: record.bytes,
      cached: Boolean(record.cached),
      failed: record.failed,
    }
  })
}

// ---------------------------------------------------------------------------------------------

/** The fixture's answers between two times, from t0. */
function fixtureEvents(fixture, from, to, t0) {
  return fixture.events
    .filter((event) => event.at >= from - 5 && event.at <= to)
    .map((event) => ({
      path: event.path,
      kind: event.kind ?? 'meta',
      method: event.method,
      range: event.range,
      start: Math.round(event.at - t0),
      end: event.end === undefined ? undefined : Math.round(event.end - t0),
      bytes: event.bytes,
    }))
}

async function main() {
  if (values.from) {
    // The report of earlier runs, from their --json files, measuring nothing.
    const results = []
    for (const file of values.from.split(',')) {
      results.push(...JSON.parse(await readFile(file, 'utf8')).results)
    }
    const summary = summarize(results)
    console.log(summary.text)
    if (values.report) await writeFile(values.report, summary.markdown)
    return 0
  }
  if (!values['database-url']) throw new Error('give --database-url or BENCH_DATABASE_URL')
  if (!existsSync(values.polyfin))
    throw new Error(`${values.polyfin} not found: build Polyfin first`)
  if (!values.web || !existsSync(join(values.web, 'index.html'))) {
    throw new Error('give --web or POLYFIN_WEB_DIR: a folder of jellyfin-web, built')
  }
  const runs = Number(values.runs)
  const modes = values.modes.split(',')
  const only = values.only ? values.only.split(',') : null
  const chosen = cells.filter(
    (cell) => !only || only.includes(cell.id) || only.includes(cell.scenario),
  )
  const media = await makeMedia()
  const { polyfin, fixture } = await startStage(media, runs)
  const base = polyfin.url
  const titleOf = (cell, n) =>
    fixture.titles.find((title) => title.cell.id === cell.id && title.n === n)

  log('Starting Chrome')
  const browser = await puppeteer.launch({
    executablePath: values.chrome,
    headless: true,
    handleSIGINT: false,
    handleSIGTERM: false,
    handleSIGHUP: false,
    args: [
      '--lang=en-US',
      '--autoplay-policy=no-user-gesture-required',
      // Chrome on macOS plays HEVC; most browsers do not. Without --hevc, it says it cannot,
      // so that jellyfin-web asks for a conversion.
      ...(values.hevc ? [] : ['--disable-features=PlatformHEVCDecoderSupport']),
    ],
  })
  atExit(async () => {
    const chrome = browser.process()
    await Promise.race([browser.close(), sleep(10000)])
    try {
      process.kill(-chrome.pid, 'SIGKILL')
    } catch {
      // Gone already.
    }
  })
  if (values.keep) {
    log(
      `Keeping the stage: ${base}/web/, ${users.themed.name} / ${users.themed.password}. Ctrl-C ends it.`,
    )
    await new Promise(() => {})
  }

  const results = []
  const record = (cell, mode, n, result, from, to) => {
    const fixtureLog = fixtureEvents(fixture, from, to, result.clicked ?? result.began)
    results.push({
      cell: cell.id,
      name: cell.name,
      scenario: cell.scenario,
      mode,
      n,
      ...result,
      fixture: fixtureLog,
    })
    log(
      `${cell.name}, ${mode} ${n}: ${Math.round(result.ms)} ms${result.method ? ` (${result.method})` : ''}`,
    )
  }
  const failures = []
  const once = async (cell, mode, n, tab) => {
    const title = titleOf(cell, n)
    const from = Date.now()
    try {
      if (cell.scenario === 'native') {
        const app = await nativeApp(base, users.themed, `fluidity-native-${mode}`)
        const result = await nativeRun(app, base, title, cell.wait)
        record(cell, mode, n, { ...result, clicked: result.began }, from, Date.now())
        return
      }
      const result =
        cell.scenario === 'open'
          ? await openRun(tab, base, title)
          : await playRun(tab, base, title, cell.wait)
      record(cell, mode, n, result, from, Date.now())
    } catch (error) {
      // A run that fails is told and left out; the page starts again from the home page.
      failures.push({ cell: cell.id, mode, n, error: error.message })
      log(`${cell.name}, ${mode} ${n}: failed, ${error.message.split('\n')[0]}`)
      if (values.shots && tab) {
        await tab.page
          .screenshot({ path: join(values.shots, `${cell.id}-${mode}-${n}.png`) })
          .catch(() => {})
      }
      if (tab) await tab.page.goto(`${base}/web/index.html#/home`).catch(() => {})
    }
  }

  if (modes.includes('warm')) {
    const tabs = {}
    for (const cell of chosen) {
      let tab = null
      if (cell.scenario !== 'native') {
        if (!tabs[cell.user]) {
          tabs[cell.user] = await openTab(await browser.createBrowserContext())
          await signIn(tabs[cell.user], base, users[cell.user])
        }
        tab = tabs[cell.user]
      }
      // A first run warms the title up: its page, pictures, versions and files.
      await once(cell, 'warm-up', 0, tab)
      for (let i = 1; i <= runs; i++) await once(cell, 'warm', 0, tab)
    }
    for (const tab of Object.values(tabs)) await tab.page.browserContext().close()
  }
  if (modes.includes('cold')) {
    for (const cell of chosen) {
      for (let n = 1; n <= runs; n++) {
        // A start of Polyfin, a browser that kept nothing, and a title never opened.
        await polyfin.restart()
        await polyfin.conversionsDone()
        let tab = null
        let context = null
        if (cell.scenario !== 'native') {
          context = await browser.createBrowserContext()
          tab = await openTab(context)
          await signIn(tab, base, users[cell.user])
        }
        await once(cell, 'cold', n, tab)
        await context?.close()
      }
    }
  }

  const summary = summarize(results)
  console.log(summary.text)
  if (values.json)
    await writeFile(values.json, JSON.stringify({ values, results, failures }, null, 2))
  if (values.report) await writeFile(values.report, summary.markdown)
  if (failures.length) log(`${failures.length} runs failed`)
  return failures.length ? 1 : 0
}

// ---------------------------------------------------------------------------------------------
// The summary: p50 and p95 of each case, its requests, and a waterfall of its median run.

function percentile(list, p) {
  if (!list.length) return NaN
  const sorted = [...list].sort((a, b) => a - b)
  return sorted[Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)]
}

/**
 * Where a run's time went, each millisecond given to the first of these under way then:
 * Polyfin answering the API, Polyfin's media until its first byte, Polyfin waiting for the
 * addon's lists, Polyfin reading the source file, pictures, jellyfin-web's files, the theme and
 * script; the rest is the page's own work, or nothing under way.
 */
const parts = ['api', 'media', 'addon', 'source', 'image', 'asset', 'theme']
function split(run) {
  const spent = Object.fromEntries([...parts, 'client'].map((key) => [key, 0]))
  if (run.steps) {
    const steps = Object.fromEntries(run.steps.map((step) => [step.name, Math.round(step.ms)]))
    return { ...spent, api: steps.Details + steps.PlaybackInfo, media: steps['Stream, first MiB'] }
  }
  const total = Math.round(run.ms)
  const ranges = [
    ...(run.requests ?? [])
      .filter((request) => !request.cached && request.start !== undefined)
      .map((request) => ({
        kind: parts.includes(request.kind)
          ? request.kind
          : request.kind === 'source'
            ? 'source'
            : 'asset',
        from: request.start,
        // Until the first byte for a stream, which then plays as it comes.
        to:
          request.kind === 'media' || request.kind === 'source'
            ? (request.headers ?? request.end ?? total)
            : (request.end ?? total),
      })),
    ...(run.fixture ?? [])
      .filter((event) => event.kind !== 'image')
      .map((event) => ({
        kind: event.kind === 'video' ? 'source' : 'addon',
        from: event.start,
        to: event.end ?? total,
      })),
  ]
  for (let t = 0; t < total; t++) {
    const kind = parts.find((each) =>
      ranges.some((range) => range.kind === each && range.from <= t && t < range.to),
    )
    spent[kind ?? 'client']++
  }
  return spent
}

function waterfall(run, width = 48) {
  const total = Math.max(1, Math.round(run.ms))
  const scale = (t) => Math.max(0, Math.min(width, Math.round((t / total) * width)))
  const lines = []
  const rows = [
    ...(run.requests ?? []).filter(
      (request) => request.start !== undefined && request.start <= total,
    ),
    ...(run.fixture ?? [])
      .filter(
        (event) => event.start <= total && (event.end ?? total) >= 0 && event.kind !== 'image',
      )
      .map((event) => ({
        ...event,
        path: `addon ${event.path}${event.range ? ` ${event.range}` : ''}`,
        kind: `addon:${event.kind}`,
      })),
  ].sort((a, b) => a.start - b.start)
  for (const row of rows) {
    const end = Math.min(total, row.end ?? total)
    const from = scale(row.start)
    const bar = ' '.repeat(from) + '█'.repeat(Math.max(1, scale(end) - from))
    const label = row.path.length > 58 ? `${row.path.slice(0, 57)}…` : row.path
    lines.push(
      `${String(row.start).padStart(6)} ${String(row.end === undefined ? '…' : row.end - row.start).padStart(6)} ${String(row.wait ?? '').padStart(5)} ${String(row.cached ? 'cache' : (row.bytes ?? '')).padStart(8)} ${row.kind.padEnd(12)} |${bar.padEnd(width)}| ${label}`,
    )
  }
  return [
    `${'start'.padStart(6)} ${'ms'.padStart(6)} ${'srv'.padStart(5)} ${'bytes'.padStart(8)} ${'kind'.padEnd(12)} |${`0 … ${total} ms`.padEnd(width)}|`,
    ...lines,
  ].join('\n')
}

function summarize(results) {
  const rows = []
  const sections = []
  const byCase = new Map()
  for (const result of results.filter((each) => each.mode !== 'warm-up')) {
    const key = `${result.cell}|${result.mode}`
    if (!byCase.has(key)) byCase.set(key, [])
    byCase.get(key).push(result)
  }
  for (const [key, list] of byCase) {
    const [, mode] = key.split('|')
    const times = list.map((each) => each.ms)
    const until = (each) => (each.requests ?? []).filter((request) => request.start <= each.ms)
    const counts = list.map((each) => until(each).filter((request) => !request.cached).length)
    const bytes = list.map((each) =>
      until(each).reduce(
        (total, request) => total + (request.cached ? 0 : (request.bytes ?? 0)),
        0,
      ),
    )
    const median = [...list].sort((a, b) => a.ms - b.ms)[Math.floor((list.length - 1) / 2)]
    const splits = list.map(split)
    const part = (name) =>
      Math.round(
        percentile(
          splits.map((each) => each[name]),
          50,
        ),
      )
    rows.push({
      name: list[0].name,
      mode,
      runs: list.length,
      p50: Math.round(percentile(times, 50)),
      p95: Math.round(percentile(times, 95)),
      requests: percentile(counts, 50),
      kb: Math.round(percentile(bytes, 50) / 1024),
      method: [...new Set(list.map((each) => each.method ?? each.backdrop ?? ''))].join(', '),
      extra: (() => {
        const pair = (key) => {
          const values = list.map((each) => each[key]).filter((value) => value !== undefined)
          return `${Math.round(percentile(values, 50))} / ${Math.round(percentile(values, 95))}`
        }
        if (list[0].scenario === 'open') return `shown ${pair('shownMs')}`
        if (list[0].scenario === 'play') return `from card ${pair('fromCard')}`
        return `play ${pair('playMs')}`
      })(),
      split: [...parts, 'client'].map((name) => `${name} ${part(name)}`).join(', '),
    })
    // The slowest requests of the case, by their median time over the runs.
    const byPath = new Map()
    for (const each of list) {
      for (const request of until(each)) {
        if (request.cached || request.end === undefined) continue
        const path = request.path
          .replace(/[0-9a-f]{32}/g, '{id}')
          .replace(/fluid:[a-z0-9-]+/g, '{title}')
          .replace(
            /[?&](api_key|ApiKey|PlaySessionId|playSessionId|tag|DeviceId|deviceId)=[^&]+/g,
            '',
          )
        if (!byPath.has(path)) byPath.set(path, [])
        byPath.get(path).push({ ms: request.end - request.start, wait: request.wait })
      }
    }
    const slowest = [...byPath]
      .map(([path, times]) => ({
        path,
        n: times.length,
        ms: Math.round(
          percentile(
            times.map((each) => each.ms),
            50,
          ),
        ),
        wait: Math.round(
          percentile(
            times.map((each) => each.wait ?? 0),
            50,
          ),
        ),
      }))
      .sort((a, b) => b.ms - a.ms)
      .slice(0, 8)
    sections.push(
      [
        `### ${list[0].name}, ${mode}`,
        '',
        `p50 ${rows.at(-1).p50} ms, p95 ${rows.at(-1).p95} ms over ${list.length} runs; median run ${Math.round(median.ms)} ms.`,
        '',
        '| Slowest requests (median) | Seen | Total ms | Server ms |',
        '| --- | ---: | ---: | ---: |',
        ...slowest.map(
          (each) => `| \`${each.path.slice(0, 110)}\` | ${each.n} | ${each.ms} | ${each.wait} |`,
        ),
        '',
        ...(median.steps
          ? [
              '| Step | ms | Bytes |',
              '| --- | ---: | ---: |',
              ...median.steps.map(
                (step) => `| ${step.name} | ${Math.round(step.ms)} | ${step.bytes} |`,
              ),
              '',
            ]
          : []),
        'Waterfall of the median run (start and length in ms from the click, `srv` the server time before the first byte):',
        '',
        '```text',
        waterfall(median),
        '```',
        '',
      ].join('\n'),
    )
  }
  const table = [
    '| Case | Mode | Runs | p50 ms | p95 ms | Other p50 / p95 ms | Requests | KB | Play or backdrop | Split of p50 runs (ms) |',
    '| --- | --- | ---: | ---: | ---: | --- | ---: | ---: | --- | --- |',
    ...rows.map(
      (row) =>
        `| ${row.name} | ${row.mode} | ${row.runs} | ${row.p50} | ${row.p95} | ${row.extra} | ${row.requests} | ${row.kb} | ${row.method} | ${row.split} |`,
    ),
  ].join('\n')
  return { text: table, markdown: `${table}\n\n${sections.join('\n')}` }
}

await run(main)
