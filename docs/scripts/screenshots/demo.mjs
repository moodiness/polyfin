// The demo's fixtures: a fictional Stremio addon (catalogs, metadata and streams of the titles the
// website shows), more addons with the shapes of a real server's, an IPTV list with its guide, the
// sources' logos over TLS, and a webhook and a WebDAV server for the notification targets and
// backup destinations, served on 127.0.0.1 for the demo server only.
import { createServer } from 'node:http'
import { createServer as createSecureServer } from 'node:https'
import { open, readFile } from 'node:fs/promises'
import { join } from 'node:path'

/** The website's fictional movies; their posters are in docs/public/posters/. */
const movies = [
  {
    slug: 'night-train',
    name: 'Night Train',
    year: 2021,
    runtime: 112,
    genres: ['Thriller', 'Mystery'],
    description:
      'Seven strangers share the last sleeper out of the city. By morning, one of them has vanished.',
  },
  {
    slug: 'amber-road',
    name: 'Amber Road',
    year: 2023,
    runtime: 104,
    genres: ['Drama', 'Adventure'],
    description:
      'Two sisters drive an old van across the desert to scatter their father’s ashes by the sea.',
  },
  {
    slug: 'ocean-life',
    name: 'Ocean Life',
    year: 2022,
    runtime: 88,
    genres: ['Documentary'],
    description: 'A year beneath the waves, from the kelp forests to the deep trenches.',
  },
  {
    slug: 'paper-moons',
    name: 'Paper Moons',
    year: 2024,
    runtime: 97,
    genres: ['Romance', 'Comedy'],
    description: 'A stage designer and a lighthouse keeper trade letters for one long winter.',
  },
]

/** The website's fictional series, each with one season; Wild Atlas's poster is drawn at run time. */
const series = [
  {
    slug: 'northwave',
    name: 'Northwave',
    year: 2024,
    genres: ['Science Fiction', 'Drama'],
    description:
      'A research station at the edge of the Arctic picks up a signal from under the ice.',
    episodes: ['Signal', 'Whiteout', 'The Long Night', 'Aurora', 'Fault Line', 'Thaw'],
  },
  {
    slug: 'echo-valley',
    name: 'Echo Valley',
    year: 2023,
    genres: ['Mystery'],
    description: 'A podcast host returns to her hometown to reopen a case everyone wants closed.',
    episodes: ['Homecoming', 'The Tape', 'Old Friends', 'Static', 'The Quarry', 'Echoes'],
  },
  {
    slug: 'glass-garden',
    name: 'Glass Garden',
    year: 2022,
    genres: ['Drama'],
    description: 'Three generations of a family run a greenhouse that is slowly being sold off.',
    episodes: ['Seedlings', 'Frost', 'Grafting', 'In Bloom', 'Harvest'],
  },
  {
    slug: 'long-field',
    name: 'The Long Field',
    year: 2021,
    genres: ['Western', 'Drama'],
    description: 'A ranch hand inherits land nobody wants, and the feud that comes with it.',
    episodes: ['The Fence', 'Dry Season', 'Branding', 'Stampede', 'Sundown', 'Deed'],
  },
  {
    slug: 'low-tide',
    name: 'Low Tide',
    year: 2024,
    genres: ['Crime'],
    description: 'In a fishing town, a harbor master finds a boat that should not exist.',
    episodes: ['Moorings', 'The Catch', 'Salt', 'Undertow', 'Ebb'],
  },
  {
    slug: 'silver-coast',
    name: 'Silver Coast',
    year: 2023,
    genres: ['Comedy'],
    description: 'The staff of a seaside hotel keep it open through its worst season yet.',
    episodes: ['Check-in', 'Room Service', 'The Wedding', 'Storm Week', 'Last Orders', 'Checkout'],
  },
  {
    slug: 'wild-atlas',
    name: 'Wild Atlas',
    year: 2025,
    genres: ['Documentary'],
    description: 'Six journeys across six continents, following the animals that make them.',
    episodes: ['Ice', 'Grassland', 'Desert', 'Forest', 'Islands', 'Rivers'],
  },
]

/** The IPTV list's channels, with their category and the programmes of their guide. */
export const channels = [
  {
    id: 'summit',
    name: 'Summit',
    group: 'Nature',
    color: '#3E8EF7',
    shows: ['Peaks at Dawn', 'Trail Notes', 'High Country'],
  },
  {
    id: 'deep-blue',
    name: 'Deep Blue',
    group: 'Nature',
    color: '#22D3EE',
    shows: ['Reef Watch', 'Ocean Life', 'Tides'],
  },
  {
    id: 'arena',
    name: 'Arena',
    group: 'Sports',
    color: '#F97316',
    shows: ['Match Day', 'Highlights', 'The Final Whistle'],
  },
  {
    id: 'cinema-one',
    name: 'Cinema One',
    group: 'Movies',
    color: '#6D4AFF',
    shows: ['Amber Road', 'Paper Moons', 'Night Train'],
  },
  {
    id: 'harbor',
    name: 'Harbor',
    group: 'News',
    color: '#10B981',
    shows: ['Harbor News', 'The Morning Desk', 'Weather Watch'],
  },
]

/**
 * The demo's other sources, Stremio addons with the shapes of a real server's: logos over https
 * with transparent parts, of several forms, one over http that the admin app does not try, and
 * none; a name too long for a row and one without a space; one with as many catalogs, as long a
 * description and as long an address as the largest seen; one whose server fails once it is
 * added; and enough of them for the list to scroll. `logo` is how its logo is drawn (see
 * drawPictures in stage.mjs), `catalogs` how many it has.
 */
export const sources = [
  {
    slug: 'archive-shelf',
    name: 'Archive Shelf',
    logo: 'wordmark',
    color: '#E8B04B',
    catalogs: 161,
    description:
      'Every movie and series the volunteers of the Archive Shelf have restored, catalogued by ' +
      'genre, decade, country and award, with the newest additions first. Lists are refreshed ' +
      'each night; titles that leave a list stay in its history for a month. Choose the ' +
      'languages, the minimum rating and the lists on its setup page.',
    // The length of its address, with a configuration as addons set up on a page of their own give.
    addressLength: 207,
  },
  {
    slug: 'grand-municipal',
    name: 'The Grand Municipal Movie and Television Lending Library',
    logo: 'dark',
    color: '#1F2937',
    catalogs: 1,
  },
  {
    slug: 'kinoklassiker',
    name: 'Kinoklassikersammlungsarchiv',
    logo: 'svg',
    color: '#EF4444',
    catalogs: 1,
  },
  { slug: 'matinee-club', name: 'Matinee Club', logo: 'round', color: '#8B5CF6', catalogs: 1 },
  {
    slug: 'short-clips-box',
    name: 'Short Clips Box',
    logo: 'square',
    color: '#0EA5E9',
    catalogs: 0,
  },
  // With a Live TV catalog, whose guides page the sweep visits.
  {
    slug: 'night-owl',
    name: 'Night Owl Picks',
    logo: 'http',
    color: '#64748B',
    catalogs: 1,
    tv: true,
  },
  { slug: 'festival-reels', name: 'Festival Reels', logo: null, catalogs: 0 },
  {
    slug: 'family-sundays',
    name: 'Family Sundays',
    logo: 'round',
    color: '#F59E0B',
    catalogs: 1,
  },
  {
    slug: 'harbor-classics',
    name: 'Harbor Classics',
    logo: 'round',
    color: '#14B8A6',
    catalogs: 1,
    fails: true,
  },
]

/** Archive Shelf's catalogs: 161, of names up to 30 characters, movies and series in turn. */
function manyCatalogs(count) {
  const kinds = ['Popular', 'New', 'Top rated', 'Hidden gems', 'Awarded', 'Classics', 'Trending']
  const genres = [
    'Action',
    'Adventure',
    'Animation',
    'Biography',
    'Comedy',
    'Crime',
    'Documentary',
    'Drama',
    'Family',
    'Fantasy',
    'History',
    'Horror',
    'Music',
    'Musical',
    'Mystery',
    'Romance',
    'Science Fiction',
    'Sport',
    'Thriller',
    'War',
    'Western',
    'Short',
    'Talk and variety',
  ]
  const catalogs = []
  for (const genre of genres) {
    for (const kind of kinds) {
      if (catalogs.length === count) return catalogs
      const index = catalogs.length
      catalogs.push({
        type: index % 2 === 0 ? 'movie' : 'series',
        id: `list-${index + 1}`,
        name: `${kind} · ${genre}`,
      })
    }
  }
  return catalogs
}

/** A logo drawn as SVG, with transparent parts, as some addons have. */
function svgLogo(color) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle cx="32" cy="32" r="26" fill="none" stroke="${color}" stroke-width="8"/><path d="M26 20 L46 32 L26 44 Z" fill="${color}"/></svg>`
}

const addonPrefix = 'demo:'
const movieId = (slug) => `${addonPrefix}${slug}`
const seriesId = (slug) => `${addonPrefix}${slug}`

/**
 * Starts the fixtures' server on a free port of 127.0.0.1, and the logos' server over TLS on
 * another.
 * - posterDir: docs/public/posters;
 * - images: extra files drawn at run time, by name (`wild-atlas.jpg`, `logo-summit.png`,
 *   `source-matinee-club.png`…);
 * - video: `{ path, size }` of the MP4 the streams point to, which the server analyzes;
 * - certificate: `{ key, cert }` in PEM, of 127.0.0.1, for the logos' server;
 * - rich: whether Demo Catalog has a logo, as the sweep's rich demo wants; the shots show none.
 */
export async function startDemo({ posterDir, images, video, certificate, rich }) {
  let base = ''
  let secureBase = ''
  const posterUrl = (slug) => `${base}/posters/${slug}.jpg`
  /** The sources whose server fails, from when fail() was called. */
  const failing = new Set()

  const movieMeta = (movie, full) => ({
    id: movieId(movie.slug),
    type: 'movie',
    name: movie.name,
    poster: posterUrl(movie.slug),
    year: String(movie.year),
    releaseInfo: String(movie.year),
    genres: movie.genres,
    ...(full
      ? {
          description: movie.description,
          runtime: `${movie.runtime} min`,
          released: `${movie.year}-03-14T00:00:00.000Z`,
        }
      : {}),
  })
  const seriesMeta = (show, full) => ({
    id: seriesId(show.slug),
    type: 'series',
    name: show.name,
    poster: posterUrl(show.slug),
    year: `${show.year}-`,
    releaseInfo: `${show.year}-`,
    genres: show.genres,
    ...(full
      ? {
          description: show.description,
          runtime: '48 min',
          videos: show.episodes.map((title, index) => ({
            id: `${seriesId(show.slug)}:1:${index + 1}`,
            title,
            season: 1,
            episode: index + 1,
            runtime: '48 min',
            released: new Date(Date.UTC(show.year, 0, 7 * (index + 1))).toISOString(),
          })),
        }
      : {}),
  })

  const manifest = () => ({
    id: 'org.polyfin.demo',
    version: '1.0.0',
    name: 'Demo Catalog',
    description: 'Fictional titles for the documentation’s screenshots.',
    resources: ['catalog', 'meta', 'stream'],
    types: ['movie', 'series'],
    idPrefixes: [addonPrefix],
    catalogs: [
      { type: 'movie', id: 'featured', name: 'Featured movies' },
      { type: 'series', id: 'featured', name: 'Featured series' },
    ],
    ...(rich ? { logo: `${secureBase}/logos/sources/demo-catalog.png` } : {}),
  })

  /** The address of a source of `sources`: Archive Shelf's padded to its length. */
  const sourceUrl = (source) => {
    const start = `${base}/addons/${source.slug}`
    const end = '/manifest.json'
    if (!source.addressLength) return start + end
    const padding = source.addressLength - start.length - end.length - 1
    const config = Buffer.from(
      JSON.stringify({
        languages: ['en', 'de', 'fr'],
        minRating: 6,
        lists: 'all',
        sort: 'new',
      }).repeat(4),
    )
      .toString('base64url')
      .slice(0, padding)
    return `${start}/${config}${end}`
  }
  const sourceLogo = (source) => {
    switch (source.logo) {
      case null:
        return undefined
      case 'http':
        return `${base}/logos/sources/${source.slug}.png`
      case 'svg':
        return `${secureBase}/logos/sources/${source.slug}.svg`
      default:
        return `${secureBase}/logos/sources/${source.slug}.png`
    }
  }
  const sourceManifest = (source) => ({
    id: `org.polyfin.demo.${source.slug}`,
    version: '1.0.0',
    name: source.name,
    description: source.description ?? `${source.name}: fictional titles of the demo.`,
    resources: source.catalogs > 0 ? ['catalog', 'stream'] : ['stream'],
    types: source.tv ? ['movie', 'series', 'tv'] : ['movie', 'series'],
    idPrefixes: [addonPrefix],
    catalogs: [
      ...(source.catalogs > 1
        ? manyCatalogs(source.catalogs)
        : source.catalogs === 1
          ? [{ type: 'movie', id: 'picks', name: `${source.name} picks` }]
          : []),
      ...(source.tv ? [{ type: 'tv', id: 'channels', name: `${source.name} channels` }] : []),
    ],
    ...(source.logo === null ? {} : { logo: sourceLogo(source) }),
  })

  const now = Date.now()
  const hour = 3600 * 1000
  const xmltvTime = (ms) => new Date(ms).toISOString().replace(/[-:T]/g, '').slice(0, 14) + ' +0000'
  const guide = () => {
    const start = Math.floor(now / hour) * hour - hour
    const lines = ['<?xml version="1.0" encoding="UTF-8"?>', '<tv>']
    for (const channel of channels) {
      lines.push(
        `<channel id="${channel.id}"><display-name>${channel.name}</display-name></channel>`,
      )
    }
    for (const channel of channels) {
      for (let slot = 0; slot < 12; slot++) {
        const title = channel.shows[slot % channel.shows.length]
        lines.push(
          `<programme start="${xmltvTime(start + slot * hour)}" stop="${xmltvTime(start + (slot + 1) * hour)}" channel="${channel.id}"><title>${title}</title></programme>`,
        )
      }
    }
    lines.push('</tv>')
    return lines.join('\n')
  }
  const playlist = () =>
    [
      '#EXTM3U',
      ...channels.flatMap((channel, index) => [
        `#EXTINF:-1 tvg-id="${channel.id}" tvg-chno="${index + 1}" tvg-logo="${base}/logos/${channel.id}.png" group-title="${channel.group}",${channel.name}`,
        `${base}/live/${channel.id}.mp4`,
      ]),
    ].join('\n')

  const json = (response, body) => {
    response.writeHead(200, {
      'content-type': 'application/json',
      'access-control-allow-origin': '*',
    })
    response.end(JSON.stringify(body))
  }
  const send = (response, type, body) => {
    response.writeHead(200, { 'content-type': type, 'content-length': body.length })
    response.end(body)
  }

  const handle = async (request, response) => {
    const path = decodeURIComponent(new URL(request.url, 'http://x').pathname)
    try {
      if (path === '/manifest.json') return json(response, manifest())
      // The other sources: their manifest, then empty catalogs. A failing one answers 503.
      let match = path.match(
        /^\/addons\/([a-z-]+)(?:\/[A-Za-z0-9_-]+)?\/(manifest\.json|catalog\/.+)$/,
      )
      const source = match && sources.find((item) => item.slug === match[1])
      if (source && failing.has(source.slug)) {
        response.writeHead(503)
        return response.end()
      }
      if (source) {
        return json(response, match[2] === 'manifest.json' ? sourceManifest(source) : { metas: [] })
      }
      match = path.match(/^\/catalog\/(movie|series)\/featured(?:\/[^/]*)?\.json$/)
      if (match) {
        const metas =
          match[1] === 'movie'
            ? movies.map((movie) => movieMeta(movie, false))
            : series.map((show) => seriesMeta(show, false))
        return json(response, { metas })
      }
      match = path.match(/^\/meta\/(movie|series)\/demo:([a-z-]+)\.json$/)
      if (match) {
        const [, type, slug] = match
        const movie = movies.find((item) => item.slug === slug)
        const show = series.find((item) => item.slug === slug)
        if (type === 'movie' && movie) return json(response, { meta: movieMeta(movie, true) })
        if (type === 'series' && show) return json(response, { meta: seriesMeta(show, true) })
        return json(response, { meta: null })
      }
      match = path.match(/^\/stream\/(movie|series)\/demo:[a-z0-9:-]+\.json$/)
      if (match) {
        return json(response, {
          streams: [
            {
              name: 'Demo\n1080p',
              title: 'Demo stream · 1080p · H.264 · AAC',
              url: `${base}/video/demo.mp4`,
              behaviorHints: { filename: 'demo-1080p.mp4', videoSize: video.size },
            },
          ],
        })
      }
      match = path.match(/^\/posters\/([a-z-]+\.jpg)$/)
      if (match) {
        const name = match[1]
        const body = images.get(name) ?? (await readFile(join(posterDir, name)))
        return send(response, 'image/jpeg', body)
      }
      match = path.match(/^\/logos\/([a-z-]+)\.png$/)
      if (match && images.has(`logo-${match[1]}.png`)) {
        return send(response, 'image/png', images.get(`logo-${match[1]}.png`))
      }
      match = path.match(/^\/logos\/sources\/([a-z-]+)\.(png|svg)$/)
      if (match && match[2] === 'svg') {
        const color = sources.find((item) => item.slug === match[1])?.color ?? '#888888'
        return send(response, 'image/svg+xml', Buffer.from(svgLogo(color)))
      }
      if (match && images.has(`source-${match[1]}.png`)) {
        return send(response, 'image/png', images.get(`source-${match[1]}.png`))
      }
      // Notification targets: one that takes messages, others refused.
      if (request.method === 'POST' && path === '/hooks/ok') {
        response.writeHead(204)
        return response.end()
      }
      // A backup destination whose WebDAV server is full.
      if (path.startsWith('/dav/')) {
        response.writeHead(507)
        return response.end()
      }
      if (path === '/list.m3u') return send(response, 'audio/x-mpegurl', Buffer.from(playlist()))
      if (path === '/guide.xml') return send(response, 'application/xml', Buffer.from(guide()))
      if (path === '/video/demo.mp4' || path.startsWith('/live/')) {
        return await sendVideo(request, response, video)
      }
      response.writeHead(404)
      response.end()
    } catch {
      response.writeHead(404)
      response.end()
    }
  }
  const server = createServer(handle)
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  base = `http://127.0.0.1:${server.address().port}`
  // The logos, over https as the admin app's security policy wants third-party images.
  const secure = createSecureServer(certificate, handle)
  await new Promise((resolve) => secure.listen(0, '127.0.0.1', resolve))
  secureBase = `https://127.0.0.1:${secure.address().port}`
  return {
    url: base,
    secureUrl: secureBase,
    manifestUrl: `${base}/manifest.json`,
    playlistUrl: `${base}/list.m3u`,
    guideUrl: `${base}/guide.xml`,
    sourceUrl,
    /** Makes a source's server fail from now on. */
    fail: (slug) => failing.add(slug),
    close: () =>
      Promise.all(
        [server, secure].map((each) => {
          each.closeAllConnections()
          return new Promise((resolve) => each.close(resolve))
        }),
      ),
  }
}

/** The pace the demo file is sent at, in bytes per second, as from a source on the internet. */
const sourceRate = 2_000_000

/** Serves the demo video from its file, with the ranges asked for, at the pace of sourceRate. */
async function sendVideo(request, response, video) {
  const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range ?? '')
  const start = range ? Number(range[1]) : 0
  const end = range?.[2] ? Math.min(Number(range[2]), video.size - 1) : video.size - 1
  response.writeHead(range ? 206 : 200, {
    'content-type': 'video/mp4',
    'content-length': end - start + 1,
    'accept-ranges': 'bytes',
    ...(range ? { 'content-range': `bytes ${start}-${end}/${video.size}` } : {}),
  })
  if (request.method === 'HEAD') return response.end()
  const file = await open(video.path)
  try {
    const chunk = Buffer.alloc(256 * 1024)
    for (let at = start; at <= end && !response.destroyed; at += chunk.length) {
      const { bytesRead } = await file.read(chunk, 0, Math.min(chunk.length, end + 1 - at), at)
      response.write(Buffer.from(chunk.subarray(0, bytesRead)))
      await new Promise((resolve) => setTimeout(resolve, (chunk.length / sourceRate) * 1000))
    }
  } finally {
    await file.close()
  }
  response.end()
}
