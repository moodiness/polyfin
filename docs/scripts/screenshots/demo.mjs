// The demo's fixtures: a fictional Stremio addon (catalogs, metadata and streams of the titles the
// website shows) and an IPTV list with its guide, served on 127.0.0.1 for the demo server only.
import { createServer } from 'node:http'
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

const addonPrefix = 'demo:'
const movieId = (slug) => `${addonPrefix}${slug}`
const seriesId = (slug) => `${addonPrefix}${slug}`

/**
 * Starts the fixtures' server on a free port of 127.0.0.1.
 * - posterDir: docs/public/posters;
 * - images: extra files drawn at run time, by name (`wild-atlas.jpg`, `logo-summit.png`…);
 * - video: `{ path, size }` of the MP4 the streams point to, which the server analyzes.
 */
export async function startDemo({ posterDir, images, video }) {
  let base = ''
  const posterUrl = (slug) => `${base}/posters/${slug}.jpg`

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

  const manifest = {
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
  }

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

  const server = createServer(async (request, response) => {
    const path = decodeURIComponent(new URL(request.url, 'http://x').pathname)
    try {
      if (path === '/manifest.json') return json(response, manifest)
      let match = path.match(/^\/catalog\/(movie|series)\/featured(?:\/[^/]*)?\.json$/)
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
  })
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  base = `http://127.0.0.1:${server.address().port}`
  return {
    url: base,
    manifestUrl: `${base}/manifest.json`,
    playlistUrl: `${base}/list.m3u`,
    guideUrl: `${base}/guide.xml`,
    close: () => {
      server.closeAllConnections()
      return new Promise((resolve) => server.close(resolve))
    },
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
