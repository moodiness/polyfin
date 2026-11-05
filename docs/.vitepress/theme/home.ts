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

/**
 * The logo's five facets, in its order. The site follows it from your sources (violet) to the
 * apps (cyan), and each feature owns one of them.
 */
export type Facet = 'violet' | 'indigo' | 'periwinkle' | 'blue' | 'cyan'

export interface Feature {
  /** Names the video and its poster: videos/polyfin-<slug>.mp4 and .jpg. */
  slug: string
  name: string
  pitch: string
  text: string
  docs: { title: string; link: string }
  /** SVG markup, from icons.ts. */
  icon: string
  facet: Facet
}

/** The features with a video, from your sources to your apps. */
export const features: Feature[] = [
  {
    slug: 'stremio',
    name: 'Addons and libraries',
    pitch: 'Your addons, as Jellyfin libraries.',
    text: 'Catalogs become libraries and collections, streams become versions of a title, and addon subtitles become subtitle tracks.',
    docs: { title: 'Addons', link: '/docs/addons' },
    icon: icons.squaresFour,
    facet: 'violet',
  },
  {
    slug: 'live-tv',
    name: 'Live TV',
    pitch: 'Your IPTV, with a real guide.',
    text: 'TV catalogs, M3U playlists and Xtream Codes accounts become Live TV, with a programme guide and recordings.',
    docs: { title: 'Live TV', link: '/docs/live-tv' },
    icon: icons.televisionSimple,
    facet: 'indigo',
  },
  {
    slug: 'music',
    name: 'Music',
    pitch: 'Music, audiobooks and podcasts.',
    text: 'Music addons install like Stremio addons and become music and books libraries, with audiobook chapters and podcasts.',
    docs: { title: 'Music, audiobooks and podcasts', link: '/docs/music' },
    icon: icons.musicNotes,
    facet: 'periwinkle',
  },
  {
    slug: 'multi-user',
    name: 'Multi-user',
    pitch: 'Real accounts for everyone at home.',
    text: 'Each user has their own watched state, resume points, favorites and Next Up, with parental control and per-user limits.',
    docs: { title: 'Users and permissions', link: '/docs/users' },
    icon: icons.users,
    facet: 'blue',
  },
  {
    slug: 'transcoding',
    name: 'Transcoding',
    pitch: 'The right stream for every screen.',
    text: 'Direct play when the app supports the file. Otherwise Polyfin repackages it or converts it to HLS, on an NVIDIA, AMD or Intel GPU when there is one, with HDR tone mapped on NVIDIA and Intel GPUs, else on the processor.',
    docs: { title: 'Transcoding', link: '/docs/transcoding' },
    icon: icons.arrowsLeftRight,
    facet: 'blue',
  },
  {
    slug: 'ad',
    name: 'Compatibility',
    pitch: 'Plays in the apps you already use.',
    text: `${apps.join(', ')} and the official Jellyfin apps sign in to Polyfin as they would to Jellyfin.`,
    docs: { title: 'Jellyfin apps', link: '/docs/jellyfin-compatibility' },
    icon: icons.devices,
    facet: 'cyan',
  },
]

export interface Extra {
  name: string
  text: string
  link: string
  /** SVG markup, from icons.ts. */
  icon: string
  facet: Facet
  /** A frame from the tour, in public/stills/, cropped so the corner facet sits on the picture. */
  still?: { src: string; alt: string; scale: number; origin: string; position?: string }
  /** The paths it lives at, shown as address bars. */
  paths?: { path: string; label: string; facet: Facet }[]
}

/** Everything else, each with the page that covers it, from your sources to your apps. */
export const extras: Extra[] = [
  {
    name: 'IPTV',
    text: 'M3U playlists and Xtream Codes accounts, with XMLTV guides. Their movies and series become libraries.',
    link: '/docs/iptv',
    icon: icons.broadcast,
    facet: 'violet',
    still: {
      src: '/stills/guide.jpg',
      alt: 'A programme guide with channels and their shows',
      scale: 1.16,
      origin: '0 100%',
      position: '50% 100%',
    },
  },
  {
    name: 'Subtitles',
    text: 'Addon subtitles, text tracks inside files, and ASS styles with their fonts. Image subtitles are burned in when an app cannot show them.',
    link: '/docs/subtitles',
    icon: icons.subtitles,
    facet: 'indigo',
    still: {
      src: '/stills/subtitles.jpg',
      alt: 'A subtitle menu with addon tracks in four languages',
      scale: 1.1,
      origin: '100% 50%',
    },
  },
  {
    name: 'Skip buttons',
    text: 'Intros, recaps, credits and previews to skip, from four community databases.',
    link: '/docs/skip-segments',
    icon: icons.skipForward,
    facet: 'periwinkle',
  },
  {
    name: 'Tracking',
    text: 'Each user can send what they watch to their tracking services, and import what they watched there.',
    link: '/docs/tracking',
    icon: icons.listChecks,
    facet: 'periwinkle',
  },
  {
    name: 'Watching together',
    text: 'SyncPlay groups play the same titles in step across devices and users.',
    link: '/docs/playback#watching-together',
    icon: icons.usersThree,
    facet: 'blue',
  },
  {
    name: 'Backups',
    text: 'A backup of the database every day, into a folder where Polyfin keeps the newest ones.',
    link: '/docs/backups',
    icon: icons.database,
    facet: 'blue',
  },
  {
    name: 'Web client and admin app',
    text: "Jellyfin's own web client at /web/, and an admin app at /admin/ that administrators open from it, already signed in.",
    link: '/docs/web-client',
    icon: icons.browser,
    facet: 'cyan',
    paths: [
      { path: '/web/', label: 'Web client', facet: 'cyan' },
      { path: '/admin/', label: 'Admin app', facet: 'blue' },
    ],
  },
  {
    name: 'Quick Connect',
    text: 'Sign in with a password, or approve the code an app shows, from a Jellyfin app or a Silo TV.',
    link: '/docs/getting-started#sign-in-from-jellyfin-apps',
    icon: icons.key,
    facet: 'cyan',
    still: {
      src: '/stills/quick-connect.jpg',
      alt: 'A six-digit Quick Connect code on a TV',
      scale: 1.1,
      origin: '100% 100%',
    },
  },
]
