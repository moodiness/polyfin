// The screens' requests as capture.mjs records them, and their replay. An identifier a request
// carries is kept as where it came from, an earlier answer of the same screen (or the signed-in
// user), so that a replay against another seed asks for the same things.

/** Identifiers: Jellyfin's 32 hex digits, or a GUID, alone or in a list (`,` or `%2C`). */
const idPattern =
  /(?<=^|[^0-9a-z]|%2c)(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?![0-9a-z])/gi

/** The path of the first value equal to token in json, such as `Items.3.Id`, or null. */
function locate(json, token) {
  const lower = token.toLowerCase()
  const plain = lower.replaceAll('-', '')
  const walk = (value, path) => {
    if (typeof value === 'string') {
      const text = value.toLowerCase()
      return text === lower || text.replaceAll('-', '') === plain ? path : null
    }
    if (value && typeof value === 'object') {
      for (const [key, child] of Object.entries(value)) {
        const found = walk(child, path ? `${path}.${key}` : key)
        if (found !== null) return found
      }
    }
    return null
  }
  return walk(json, '')
}

function valueAt(json, path) {
  let value = json
  for (const key of path.split('.')) {
    if (value === null || value === undefined) return undefined
    value = value[key]
  }
  return typeof value === 'string' ? value : undefined
}

/** Times, as the web player writes them: `2026-11-16T21:59:59.000Z`, plain or URL-encoded. */
const timePattern = /\d{4}-\d\d-\d\dT\d\d(?::|%3A)\d\d(?::|%3A)\d\d(?:\.\d+)?Z/gi

/**
 * Turns the requests captured in a screen, with their start and end times and answers, into
 * waves: a request is in the first wave when it started before any answer of the screen came,
 * else one wave after the latest wave whose answer came before it started. Identifiers become
 * references, `{user}` or `{<request>:<path>}`, and times `{time:<ms from the request>}`.
 */
export function toWaves(captured, user) {
  const sorted = [...captured].sort((a, b) => a.start - b.start)
  for (const request of sorted) {
    const before = sorted.filter((other) => other !== request && other.end <= request.start)
    request.wave = before.length ? Math.max(...before.map((other) => other.wave)) + 1 : 0
  }
  const waves = []
  sorted.forEach((request, index) => {
    request.index = index
    const reference = (token) => {
      if (token.replaceAll('-', '').toLowerCase() === user.replaceAll('-', '').toLowerCase()) {
        return '{user}'
      }
      for (const earlier of sorted.slice(0, index).reverse()) {
        if (earlier.end > request.start || earlier.json === undefined) continue
        const path = locate(earlier.json, token)
        if (path !== null) return `{${earlier.index}:${path}}`
      }
      return token
    }
    const time = (text) =>
      `{time:${Date.parse(decodeURIComponent(text)) - Math.round(request.epoch)}}`
    const template = (text) => text?.replace(idPattern, reference).replace(timePattern, time)
    ;(waves[request.wave] ??= []).push({
      index,
      method: request.method,
      path: template(request.path),
      body: template(request.body),
      at: Math.round(request.start - sorted[0].start),
      took: Math.round(request.end - request.start),
    })
  })
  return waves.filter(Boolean)
}

/** A request's path with its references resolved from answers, null when one cannot be. */
function resolve(text, answers, user, encode) {
  if (text === undefined || text === null) return text
  let missing = false
  const resolved = text.replace(/\{(user|time:-?\d+|\d+:[^}]+)\}/g, (_, ref) => {
    if (ref === 'user') return user
    if (ref.startsWith('time:')) {
      const iso = new Date(Date.now() + Number(ref.slice(5))).toISOString()
      return encode ? encodeURIComponent(iso) : iso
    }
    const [index, path] = [Number(ref.slice(0, ref.indexOf(':'))), ref.slice(ref.indexOf(':') + 1)]
    const value = answers.get(index) === undefined ? undefined : valueAt(answers.get(index), path)
    if (value === undefined) missing = true
    return value ?? ''
  })
  return missing ? null : resolved
}

/** The parameters that tell apart a screen's requests to one endpoint. */
const telling =
  /^(include|exclude)ItemTypes$|^mediaTypes$|^SortBy$|^StartIndex$|^limit$|^isAiring$/i

/** The name a request is shown under: its method, path and telling parameters. */
export function requestName(request) {
  const [path, query] = request.path.split('?')
  const shown = path.replace(/\{[^}]+\}/g, '{id}')
  const values = new Map()
  for (const [key, value] of new URLSearchParams(query ?? '')) {
    if (telling.test(key)) values.set(key, [...(values.get(key) ?? []), value])
  }
  const params = [...values].map(([key, list]) => `${key}=${list.join(',')}`).join('&')
  return `${request.method} ${shown}${params ? `?${params}` : ''}`
}

/**
 * Replays a screen's waves as jellyfin-web sends them: a wave's requests at once, the next wave
 * once all have answered. Returns the screen's time, each wave's, and each request's time and
 * status (0 for a request whose identifier no answer gave).
 */
export async function replayScreen(screen, client) {
  const answers = new Map()
  const requests = []
  const waves = []
  const start = performance.now()
  for (const wave of screen.waves) {
    const waveStart = performance.now()
    await Promise.all(
      wave.map(async (request) => {
        const path = resolve(request.path, answers, client.userId, true)
        const body = resolve(request.body, answers, client.userId, false)
        if (path === null || body === null) {
          requests.push({ index: request.index, ms: 0, status: 0 })
          return
        }
        const began = performance.now()
        const response = await fetch(client.base + path, {
          method: request.method,
          headers: {
            authorization: client.authorization,
            ...(body ? { 'content-type': 'application/json' } : {}),
          },
          body: body || undefined,
        })
        const text = await response.text()
        requests.push({
          index: request.index,
          ms: performance.now() - began,
          status: response.status,
        })
        if (text && response.headers.get('content-type')?.includes('json')) {
          try {
            answers.set(request.index, JSON.parse(text))
          } catch {
            // Not JSON after all: nothing to reference.
          }
        }
      }),
    )
    waves.push(performance.now() - waveStart)
  }
  return { ms: performance.now() - start, waves, requests }
}

/** The p-th percentile of numbers, by nearest rank. */
export function percentile(numbers, p) {
  if (!numbers.length) return NaN
  const sorted = [...numbers].sort((a, b) => a - b)
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1))]
}
