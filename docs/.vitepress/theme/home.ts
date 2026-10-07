import { icons } from './icons'

export const repository = 'https://github.com/moodiness/polyfin'

/** Apps that sign in to Polyfin, from docs/jellyfin-compatibility.md. */
export const apps = [
  'Infuse',
  'Swiftfin',
  'Findroid',
  'Streamyfin',
  'Strand',
  'Odin',
  'VidHub',
  'Nuvio',
  'Kodi',
]

export interface Feature {
  /** Names the video and its poster: videos/polyfin-<slug>.mp4 and .jpg. */
  slug: string
  name: string
  pitch: string
  text: string
  docs: { title: string; link: string }
  /** SVG markup, from icons.ts. */
  icon: string
  /** The feature's color in its video. */
  accent: string
}

/** The features with a video, in the order of the tour. */
export const features: Feature[] = [
  {
    slug: 'stremio',
    name: 'Stremio addons',
    pitch: 'Your addons, as Jellyfin libraries.',
    text: 'Catalogs become libraries and collections, streams become versions of a title, and addon subtitles become subtitle tracks.',
    docs: { title: 'Addons and libraries', link: '/docs/addons-and-libraries' },
    icon: icons.squaresFour,
    accent: '#c084fc',
  },
  {
    slug: 'multi-user',
    name: 'Multi-user',
    pitch: 'Real accounts for everyone at home.',
    text: 'Each user has their own watched state, resume points, favorites and Next Up, with parental control and per-user limits.',
    docs: { title: 'Users and permissions', link: '/docs/users' },
    icon: icons.users,
    accent: '#8b6bff',
  },
  {
    slug: 'transcoding',
    name: 'Transcoding',
    pitch: 'The right stream for every screen.',
    text: 'Direct play when the app supports the file. Otherwise a remux or a conversion to HLS, on an NVIDIA, AMD or Intel GPU when there is one, with HDR tone mapping.',
    docs: { title: 'Transcoding', link: '/docs/transcoding' },
    icon: icons.arrowsLeftRight,
    accent: '#6a6bf8',
  },
  {
    slug: 'live-tv',
    name: 'Live TV',
    pitch: 'Your IPTV, with a real guide.',
    text: 'TV catalogs, M3U playlists and Xtream Codes accounts become Live TV, with a programme guide and recordings.',
    docs: { title: 'Live TV', link: '/docs/live-tv' },
    icon: icons.televisionSimple,
    accent: '#3e8ef7',
  },
  {
    slug: 'music',
    name: 'Music',
    pitch: 'Music, audiobooks and podcasts.',
    text: 'Eclipse music addons install like Stremio addons and become music and books libraries, with audiobook chapters and podcasts.',
    docs: { title: 'Music addons', link: '/docs/addons-and-libraries#music-addons' },
    icon: icons.musicNotes,
    accent: '#22d3ee',
  },
]

export interface Extra {
  name: string
  text: string
  link: string
  /** SVG markup, from icons.ts. */
  icon: string
}

/** Everything else, each with the page that covers it. */
export const extras: Extra[] = [
  {
    name: 'IPTV',
    text: 'M3U playlists and Xtream Codes accounts, with XMLTV guides. Their movies and series become libraries.',
    link: '/docs/iptv',
    icon: icons.broadcast,
  },
  {
    name: 'Subtitles',
    text: 'Addon subtitles, text tracks inside files, and ASS styles with their fonts. Image subtitles are burned in when an app cannot show them.',
    link: '/docs/subtitles',
    icon: icons.subtitles,
  },
  {
    name: 'Skip buttons',
    text: 'Intros, recaps, credits and previews to skip, from three community databases.',
    link: '/docs/skip-segments',
    icon: icons.skipForward,
  },
  {
    name: 'Tracking',
    text: 'Each user can send what they watch to Trakt, Simkl, MDBList and PublicMetaDB, and import what they watched there.',
    link: '/docs/tracking',
    icon: icons.listChecks,
  },
  {
    name: 'Watching together',
    text: 'SyncPlay groups play the same titles in step across devices and users.',
    link: '/docs/playback#watching-together',
    icon: icons.usersThree,
  },
  {
    name: 'Quick Connect',
    text: 'Sign in with a password, or approve the 6-digit code an app shows.',
    link: '/docs/getting-started#sign-in-from-jellyfin-apps',
    icon: icons.key,
  },
  {
    name: 'Web client and admin app',
    text: "Jellyfin's own web client at /web/, and an admin app at /admin/ that administrators open from it, already signed in.",
    link: '/docs/web-client',
    icon: icons.browser,
  },
  {
    name: 'Backups',
    text: 'A backup of the database every day, into a folder where Polyfin keeps the newest ones.',
    link: '/docs/backups',
    icon: icons.database,
  },
]
