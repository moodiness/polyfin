// The large-library bench's stage, which bench.mjs and capture.mjs share: the synthetic Xtream
// provider of internal/iptv at the size of a large library, a PostgreSQL database kept between
// runs, and Polyfin built from this repository, filled as a user would. See README.md.
import { execFileSync, spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import pg from 'pg'

const here = dirname(fileURLToPath(import.meta.url))
export const repoDir = resolve(here, '../../..')
export const screensFile = join(here, 'screens.json')

/** The seed's version: a database seeded by another is seeded again. */
const seedVersion = 1
export const admin = { name: 'Morgan', password: 'bench-password-1' }
/** Riley watches a lot; the others a little. */
export const viewer = { name: 'Riley', password: 'bench-password-2' }
const others = [
  { name: 'Sam', password: 'bench-password-3' },
  { name: 'Robin', password: 'bench-password-4' },
  { name: 'Alex', password: 'bench-password-5' },
  { name: 'Jordan', password: 'bench-password-6' },
]
/** What jellyfin-web says it is, as the requests replayed are its own. */
export const webApp = {
  client: 'Jellyfin Web',
  device: 'Chrome',
  deviceId: 'bench-web-1',
  version: '10.11.0',
}

/** The options both tools take, and theirs. */
export function options(own = {}) {
  const { values } = parseArgs({
    options: {
      'database-url': { type: 'string', default: process.env.BENCH_DATABASE_URL },
      database: { type: 'string', default: 'polyfin_bench' },
      polyfin: { type: 'string', default: process.env.POLYFIN_BIN ?? join(repoDir, 'bin/polyfin') },
      'provider-port': { type: 'string', default: '47391' },
      fresh: { type: 'boolean', default: false },
      drop: { type: 'boolean', default: false },
      web: { type: 'string', default: process.env.POLYFIN_WEB_DIR },
      ...own,
    },
  })
  if (!values['database-url']) {
    throw new Error('give --database-url or BENCH_DATABASE_URL: a PostgreSQL address')
  }
  if (!existsSync(values.polyfin)) {
    throw new Error(`${values.polyfin} not found: build Polyfin first (see README.md)`)
  }
  return values
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

/** Runs main, then stops everything; exit code 1 on an error, else main's. */
export async function run(main) {
  try {
    process.exitCode = (await main()) ?? 0
  } catch (error) {
    console.error(error)
    process.exitCode = 1
  } finally {
    await cleanup()
  }
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

async function within(promise, ms) {
  let timer
  try {
    await Promise.race([promise, new Promise((resolve) => (timer = setTimeout(resolve, ms)))])
  } finally {
    clearTimeout(timer)
  }
}

/** A random number generator of its own seed, so that each run picks the same titles. */
export function random(seed) {
  let a = seed >>> 0
  const next = () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  return {
    next,
    int: (n) => Math.floor(next() * n),
    /** count items of list, without repeats. */
    pick(list, count) {
      const copy = [...list]
      for (let i = copy.length - 1; i > 0; i--) {
        const j = Math.floor(next() * (i + 1))
        ;[copy[i], copy[j]] = [copy[j], copy[i]]
      }
      return copy.slice(0, count)
    },
  }
}

/** Runs SQL on a database, with parameters. */
export async function query(url, sql, values) {
  const db = new pg.Client({ connectionString: url })
  await db.connect()
  try {
    return await db.query(sql, values)
  } finally {
    await db.end()
  }
}

function databaseUrl(url, name) {
  const own = new URL(url)
  own.pathname = `/${name}`
  return own.toString()
}

/**
 * The bench's folder: Polyfin's data folder and what the seed made, kept with the database
 * between runs.
 */
function benchDir(database) {
  return join(tmpdir(), `polyfin-bench-${database}`)
}

/**
 * Starts the stage: the provider, then Polyfin on the bench database, seeded first when it is
 * missing, seeded by another version or --fresh is given. Returns the stage and the seed's
 * state; onImporting, when given, runs while the first import runs, before the users' history.
 */
export async function startStage(values, { onImporting } = {}) {
  const name = values.database
  if (!/^[a-z_][a-z0-9_]*$/.test(name)) throw new Error(`invalid database name ${name}`)
  const dir = benchDir(name)
  const stateFile = join(dir, 'state.json')
  const own = databaseUrl(values['database-url'], name)
  if (values.drop) {
    cleanups.push(async () => {
      await query(values['database-url'], `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`)
      await rm(dir, { recursive: true, force: true })
      log(`Dropped ${name}`)
    })
  }
  let state = null
  const exists = (
    await query(values['database-url'], 'SELECT 1 FROM pg_database WHERE datname = $1', [name])
  ).rowCount
  if (exists && !values.fresh && existsSync(stateFile)) {
    state = JSON.parse(await readFile(stateFile, 'utf8'))
    if (state.version !== seedVersion) state = null
  }
  const provider = await startProvider(values)
  if (state) {
    log(`Reusing ${name}, seeded ${state.seededAt}`)
    const polyfin = await startPolyfin(values, own, dir, state.port)
    return { values, provider, polyfin, state, databaseUrl: own, dir }
  }
  log(`Seeding ${name} afresh`)
  await query(values['database-url'], `DROP DATABASE IF EXISTS ${name} WITH (FORCE)`)
  await rm(dir, { recursive: true, force: true })
  await mkdir(dir, { recursive: true })
  await query(values['database-url'], `CREATE DATABASE ${name}`)
  const polyfin = await startPolyfin(values, own, dir)
  state = await seed({ values, provider, polyfin, databaseUrl: own, onImporting })
  state.version = seedVersion
  state.port = polyfin.port
  await writeFile(stateFile, JSON.stringify(state, null, 2))
  return { values, provider, polyfin, state, databaseUrl: own, dir }
}

/**
 * Serves the synthetic provider of internal/iptv (TestServeSyntheticXtream) with the large
 * library, built as a test binary, on a fixed port: the source Polyfin keeps is its address.
 */
async function startProvider(values) {
  const binary = join(tmpdir(), 'polyfin-bench-provider.test')
  log('Building the synthetic provider')
  execFileSync('go', ['test', '-c', '-o', binary, './internal/iptv'], {
    cwd: repoDir,
    stdio: ['ignore', 'ignore', 'inherit'],
  })
  const address = `127.0.0.1:${values['provider-port']}`
  const child = spawn(binary, ['-test.run', '^TestServeSyntheticXtream$', '-test.timeout', '0'], {
    env: {
      ...process.env,
      POLYFIN_SYNTHETIC_XTREAM: 'library',
      POLYFIN_SYNTHETIC_XTREAM_ADDR: address,
      POLYFIN_TEST_FFMPEG: '',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const exited = new Promise((resolve) => child.on('exit', resolve))
  cleanups.push(async () => {
    if (child.exitCode === null) {
      child.kill('SIGINT')
      await within(exited, 5000)
      if (child.exitCode === null) child.kill('SIGKILL')
    }
  })
  const summary = await new Promise((resolvePromise, reject) => {
    let text = ''
    child.stdout.setEncoding('utf8').on('data', (chunk) => {
      text += chunk
      const line = text.match(/^Synthetic Xtream provider: .*$/m)
      if (line && text.includes('Interrupt')) resolvePromise(line[0])
    })
    child.stderr.setEncoding('utf8').on('data', (chunk) => (text += chunk))
    exited.then((code) => reject(new Error(`the provider exited with ${code}:\n${text}`)))
  })
  log(summary)
  return { url: `http://${address}`, summary }
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

/**
 * Starts Polyfin on the bench database and folder, on port when given (a restart keeps it), and
 * waits until it answers. stop() and restart() come with it, and its CPU and memory in usage().
 */
async function startPolyfin(values, databaseUrl, dir, port) {
  const [http, discovery, dlna, hdhomerun] = await freePorts(4)
  const listen = port ?? http
  const logPath = join(dir, 'polyfin.log')
  const start = async () => {
    const env = {
      ...process.env,
      POLYFIN_DATABASE_URL: databaseUrl,
      POLYFIN_DATA_DIR: join(dir, 'data'),
      POLYFIN_LISTEN: `127.0.0.1:${listen}`,
      POLYFIN_DISCOVERY_PORT: String(discovery),
      POLYFIN_DLNA_PORT: String(dlna),
      POLYFIN_HDHOMERUN_PORT: String(hdhomerun),
      POLYFIN_HDHOMERUN_ADDRESSES: '127.0.0.1',
    }
    if (values.web) env.POLYFIN_WEB_DIR = values.web
    else delete env.POLYFIN_WEB_DIR
    const child = spawn(values.polyfin, ['serve'], { env, stdio: ['ignore', 'pipe', 'pipe'] })
    const exited = new Promise((resolve) => child.on('exit', resolve))
    let text = ''
    const read = (chunk) => {
      text += chunk
      if (text.length > 4_000_000) text = text.slice(-2_000_000)
    }
    child.stdout.setEncoding('utf8').on('data', read)
    child.stderr.setEncoding('utf8').on('data', read)
    const url = `http://127.0.0.1:${listen}`
    for (let i = 0; ; i++) {
      if (child.exitCode !== null) throw new Error(`Polyfin exited:\n${text}`)
      try {
        if ((await fetch(`${url}/admin/api/status`)).ok) break
      } catch {
        // Not listening yet.
      }
      if (i > 600) throw new Error(`Polyfin did not answer:\n${text}`)
      await sleep(100)
    }
    return { child, exited, log: () => text }
  }
  const stop = async (process) => {
    if (process.child.exitCode === null) {
      process.child.kill('SIGTERM')
      await within(process.exited, 15000)
      if (process.child.exitCode === null) process.child.kill('SIGKILL')
    }
    await writeFile(logPath, process.log(), { flag: 'a' })
  }
  let current = await start()
  const polyfin = {
    url: `http://127.0.0.1:${listen}`,
    port: listen,
    get pid() {
      return current.child.pid
    },
    log: () => current.log(),
    async restart() {
      await stop(current)
      current = await start()
    },
    /** Polyfin's CPU time so far, in seconds, and its resident memory, in MB. */
    usage() {
      const [cpu, rss] = execFileSync('ps', ['-o', 'time=,rss=', '-p', String(current.child.pid)], {
        encoding: 'utf8',
      })
        .trim()
        .split(/\s+/)
      const parts = cpu.split(':').map(Number)
      const seconds = parts.reduce((total, part) => total * 60 + part, 0)
      return { cpu: seconds, rss: Number(rss) / 1024 }
    },
  }
  cleanups.push(() => stop(current))
  return polyfin
}

/** The admin API, as the user signed in: the session cookie of setup or sign-in is kept. */
export function adminClient(base) {
  let session = ''
  const request = async (method, path, body) => {
    const response = await fetch(base + path, {
      method,
      headers: {
        ...(body === undefined ? {} : { 'content-type': 'application/json' }),
        ...(session ? { cookie: `polyfin_session=${session}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const cookie = response.headers.get('set-cookie')?.match(/polyfin_session=([^;]+)/)
    if (cookie) session = cookie[1]
    const text = await response.text()
    if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${text}`)
    return text ? JSON.parse(text) : null
  }
  return {
    get: (path) => request('GET', path),
    post: (path, body) => request('POST', path, body ?? {}),
    signIn: (user) => request('POST', '/admin/api/session', user),
  }
}

/** The Jellyfin API as jellyfin-web, signed in as a user. */
export async function jellyfinClient(base, user, deviceId = webApp.deviceId) {
  const header = (token) =>
    `MediaBrowser Client="${webApp.client}", Device="${webApp.device}", DeviceId="${deviceId}", Version="${webApp.version}"${token ? `, Token="${token}"` : ''}`
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
    return text ? JSON.parse(text) : null
  }
  const answer = await call('POST', '/Users/AuthenticateByName', {
    Username: user.name,
    Pw: user.password,
  })
  token = answer.AccessToken
  return {
    userId: answer.User.Id,
    token,
    authorization: header(token),
    get: (path) => call('GET', path),
    post: (path, body) => call('POST', path, body ?? {}),
  }
}

/** Runs task on each item, at most `at` at once. */
export async function each(items, at, task) {
  let next = 0
  await Promise.all(
    Array.from({ length: Math.min(at, items.length) }, async () => {
      while (next < items.length) {
        const i = next++
        await task(items[i], i)
      }
    }),
  )
}

/** What the first import stored so far: rows by table and items by kind. */
export async function stored(databaseUrl) {
  const counts = await query(
    databaseUrl,
    `SELECT (SELECT count(*) FROM iptv_titles WHERE type = 'movie')::int AS "titles.movie",
       (SELECT count(*) FROM iptv_titles WHERE type = 'series')::int AS "titles.series",
       (SELECT count(*) FROM iptv_streams)::int AS streams,
       (SELECT count(*) FROM iptv_lineup)::int AS channels,
       (SELECT count(*) FROM live_guide_programmes)::int AS programmes,
       (SELECT count(*) FROM iptv_details)::int AS details,
       (SELECT count(*) FROM user_data)::int AS user_data`,
  )
  const kinds = await query(
    databaseUrl,
    'SELECT kind, count(*)::int AS n FROM items GROUP BY kind ORDER BY kind',
  )
  const items = Object.fromEntries(kinds.rows.map((row) => [`items.${row.kind}`, row.n]))
  return { ...counts.rows[0], ...items }
}

/** The largest tables, with their indexes and TOAST, in MB. */
export async function tableSizes(databaseUrl) {
  const result = await query(
    databaseUrl,
    `SELECT relname AS table, round(pg_total_relation_size(oid) / 1048576.0, 1)::float AS mb,
       round(pg_relation_size(oid) / 1048576.0, 1)::float AS heap
     FROM pg_class WHERE relkind = 'r' AND relnamespace = 'public'::regnamespace
     ORDER BY pg_total_relation_size(oid) DESC LIMIT 12`,
  )
  const total = await query(
    databaseUrl,
    'SELECT round(pg_database_size(current_database()) / 1048576.0)::int AS mb',
  )
  return { tables: result.rows, totalMb: total.rows[0].mb }
}

/** The counts the import itself moves: the users' browsing and history move the others. */
const importCounts = ['titles.movie', 'titles.series', 'streams', 'channels', 'programmes']

/**
 * Waits for the first import to settle: what it stores unchanged, and, when quiet, Polyfin
 * under 5 % of a core, for half a minute. Reports what is stored as it goes, and when each
 * count first moved.
 */
async function settle(databaseUrl, since, polyfin, quiet) {
  let last = ''
  let still = 0
  let used = polyfin.usage().cpu
  const phases = {}
  for (;;) {
    await sleep(2000)
    const now = await stored(databaseUrl)
    const seconds = Math.round((Date.now() - since) / 1000)
    for (const [key, value] of Object.entries(now)) {
      if (value > 0 && phases[key] === undefined) phases[key] = seconds
    }
    const cpu = polyfin.usage().cpu
    const busy = (cpu - used) / 2
    used = cpu
    const text = JSON.stringify(importCounts.map((key) => now[key]))
    if (text === last && (!quiet || busy < 0.05)) {
      still += 2
      if (still >= 30) return { seconds: seconds - still, phases, stored: now }
    } else {
      still = 0
      log(`${seconds} s, ${Math.round(busy * 100)} % CPU: ${JSON.stringify(now)}`)
    }
    last = text
  }
}

/**
 * Fills the bench as users would: the administrator's setup, five users, the provider added as
 * an IPTV source with Movies and Series on (its libraries), the first import, then the users'
 * history. Riley played 300 movies, has 200 series under way (a few of the longest among them),
 * favorites and a Watchlist; the others a little of each.
 */
async function seed({ values, provider, polyfin, databaseUrl, onImporting }) {
  const api = adminClient(polyfin.url)
  const setupCode = await waitFor(() => polyfin.log().match(/setup_code=([A-Z0-9]{4}-[A-Z0-9]{4})/))
  await api.post('/admin/api/setup', {
    setupCode: setupCode[1],
    name: admin.name,
    password: admin.password,
    language: 'en',
  })
  for (const user of [viewer, ...others]) {
    await api.post('/admin/api/users', { ...user, isAdministrator: false })
  }

  // As the admin app does: the account's preview, then the source with its import options.
  const account = { kind: 'xtream', server: provider.url, username: 'user', password: 'secret' }
  const since = Date.now()
  for (const by of ['group', 'movie', 'series']) {
    await api.post('/admin/api/scopes/shared/iptv/preview', { ...account, by, q: '' })
  }
  const adding = api.post('/admin/api/scopes/shared/iptv', {
    name: 'Bench TV',
    ...account,
    userAgent: '',
    providerGuide: true,
    guideUrl: '',
    options: { liveTv: true, movies: true, series: true },
  })
  let addSeconds = 0
  const settled = adding.then(() => {
    addSeconds = (Date.now() - since) / 1000
    log(`Source added in ${addSeconds.toFixed(1)} s`)
    return settle(databaseUrl, since, polyfin, !onImporting)
  })
  const importing = onImporting?.({ polyfin, settled, since }) ?? Promise.resolve()
  const added = await adding
  const imported = await settled
  await importing
  imported.addSeconds = addSeconds
  log(`First import settled after ${imported.seconds} s`)

  const history = await fillHistory(polyfin, provider)
  const after = await stored(databaseUrl)
  await query(databaseUrl, 'VACUUM ANALYZE')
  return {
    seededAt: new Date().toISOString(),
    source: added.id,
    provider: provider.summary,
    imported,
    history,
    stored: after,
    sizes: await tableSizes(databaseUrl),
  }
}

async function waitFor(check, ms = 60000) {
  for (let waited = 0; waited < ms; waited += 200) {
    const result = check()
    if (result) return result
    await sleep(200)
  }
  throw new Error('timed out')
}

/** The provider's series lengths, which pick a few of the longest for Riley. */
async function seriesLengths(provider) {
  const api = `${provider.url}/player_api.php?username=user&password=secret`
  const series = await (await fetch(`${api}&action=get_series`)).json()
  const lengths = new Map()
  await each(series, 8, async (show) => {
    const info = await (
      await fetch(`${api}&action=get_series_info&series_id=${show.series_id}`)
    ).json()
    lengths.set(
      show.name,
      Object.values(info.episodes).reduce((n, season) => n + season.length, 0),
    )
  })
  return lengths
}

const day = 86_400_000

/** A user's history: movies played and started, series under way, favorites, Watchlist. */
async function fillHistory(polyfin, provider) {
  const lengths = await seriesLengths(provider)
  const users = [
    { user: viewer, movies: 300, started: 12, series: 200, long: 8, favorites: 60, watchlist: 40 },
    ...others.map((user) => ({
      user,
      movies: 25,
      started: 3,
      series: 8,
      long: 0,
      favorites: 5,
      watchlist: 6,
    })),
  ]
  const totals = { played: 0, episodes: 0, series: 0, favorites: 0, watchlist: 0, started: 0 }
  for (const [n, plan] of users.entries()) {
    const started = Date.now()
    const jf = await jellyfinClient(polyfin.url, plan.user, `bench-seed-${n}`)
    const pick = random(1217 + n)
    const views = (await jf.get(`/UserViews?userId=${jf.userId}`)).Items
    const view = (type) => views.find((v) => v.CollectionType === type)?.Id
    // The whole library, a page at a time, as scrolling it does: Polyfin stores a title as an
    // item once a page lists it.
    const list = async (type, parent) => {
      const items = []
      for (let total = Infinity; items.length < total;) {
        const page = await jf.get(
          `/Items?userId=${jf.userId}&ParentId=${parent}&IncludeItemTypes=${type}&Recursive=true&Fields=&EnableImages=false&EnableUserData=false&SortBy=SortName&StartIndex=${items.length}&Limit=200`,
        )
        if (!page.Items.length) break
        items.push(...page.Items)
        total = page.TotalRecordCount
      }
      return items
    }
    const movies = await list('Movie', view('movies'))
    const series = await list('Series', view('tvshows'))
    const ago = (days) => new Date(Date.now() - days * day).toISOString()

    const played = pick.pick(movies, plan.movies + plan.started)
    await each(played.slice(0, plan.movies), 4, async (movie) => {
      await jf.post(
        `/Users/${jf.userId}/PlayedItems/${movie.Id}?datePlayed=${ago(1 + pick.int(365))}`,
      )
      totals.played++
    })
    await each(played.slice(plan.movies), 2, async (movie) => {
      await jf.post(`/UserItems/${movie.Id}/UserData?userId=${jf.userId}`, {
        PlaybackPositionTicks: (10 + pick.int(60)) * 60 * 10_000_000,
        LastPlayedDate: ago(pick.int(20)),
      })
      totals.started++
    })

    // Series under way: a few of the longest, the rest at random; each opened (its episodes
    // asked from the provider, paced), its first seasons played, then part of the next one.
    const longest = [...series]
      .sort((a, b) => (lengths.get(b.Name) ?? 0) - (lengths.get(a.Name) ?? 0))
      .slice(0, 30)
    const underWay = [
      ...pick.pick(longest, plan.long),
      ...pick.pick(
        series.filter((s) => !longest.includes(s)),
        plan.series - plan.long,
      ),
    ]
    await each(underWay, 2, async (show, i) => {
      await jf.get(`/Users/${jf.userId}/Items/${show.Id}`)
      const seasons = (await jf.get(`/Shows/${show.Id}/Seasons?userId=${jf.userId}`)).Items
      if (!seasons.length) return
      const current = pick.int(seasons.length)
      const when = ago(i % 3 === 0 ? pick.int(14) : 14 + pick.int(150))
      for (const season of seasons.slice(0, current)) {
        await jf.post(`/Users/${jf.userId}/PlayedItems/${season.Id}?datePlayed=${when}`)
      }
      const episodes = (
        await jf.get(
          `/Shows/${show.Id}/Episodes?userId=${jf.userId}&seasonId=${seasons[current].Id}&Fields=&EnableImages=false`,
        )
      ).Items
      const watched = pick.int(Math.max(1, episodes.length - 1))
      for (const episode of episodes.slice(0, watched)) {
        await jf.post(`/Users/${jf.userId}/PlayedItems/${episode.Id}?datePlayed=${when}`)
        totals.episodes++
      }
      if (i % 10 === 0 && episodes[watched]) {
        await jf.post(`/UserItems/${episodes[watched].Id}/UserData?userId=${jf.userId}`, {
          PlaybackPositionTicks: (5 + pick.int(30)) * 60 * 10_000_000,
          LastPlayedDate: when,
        })
        totals.started++
      }
      totals.series++
    })

    const channels = (await jf.get(`/LiveTv/Channels?userId=${jf.userId}&Limit=400`)).Items
    const favorites = [
      ...pick.pick(movies, plan.favorites),
      ...pick.pick(underWay, Math.ceil(plan.favorites / 4)),
      ...pick.pick(channels, Math.ceil(plan.favorites / 6)),
    ]
    await each(favorites, 4, async (item) => {
      await jf.post(`/Users/${jf.userId}/FavoriteItems/${item.Id}`)
      totals.favorites++
    })
    const wanted = [
      ...pick.pick(movies, Math.ceil(plan.watchlist / 2)),
      ...pick.pick(series, Math.floor(plan.watchlist / 2)),
    ]
    await each(wanted, 4, async (item) => {
      await jf.post(`/Polyfin/UserItems/${item.Id}/Watchlist`)
      totals.watchlist++
    })
    log(`${plan.user.name}'s history in ${Math.round((Date.now() - started) / 1000)} s`)
  }
  return totals
}
