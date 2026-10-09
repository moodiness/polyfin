import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { Language } from '@/i18n'
import { takeWebClientToken } from '@/webClientSession'

export type Status = {
  name: string
  version: string
  serverId: string
  database: 'ready' | 'unavailable'
  setupRequired: boolean
  /** Whether the server serves the web player at /web/; absent from servers without it. */
  webClient?: boolean
}

export type SessionUser = {
  id: string
  name: string
  isAdministrator: boolean
  /** Identifies the user's profile picture, served at /UserImage; null without one. */
  imageTag: string | null
}

/** The address of a user's profile picture, undefined without one. */
export const userImageUrl = (user: { id: string; imageTag: string | null }) =>
  user.imageTag === null
    ? undefined
    : `/UserImage?userId=${encodeURIComponent(user.id)}&tag=${encodeURIComponent(user.imageTag)}`

export type User = {
  id: string
  name: string
  isAdministrator: boolean
  isHidden: boolean
  isDisabled: boolean
  createdAt: string
  lastLoginAt: string | null
  lastActivityAt: string | null
  parentalControl: ParentalControl
  /** Whether the user may have video and audio converted (transcoded). */
  transcoding: boolean
  /** Whether the user may download titles. */
  downloads: boolean
  /** Whether the user may add and use their own addons. */
  personalAddons: boolean
  /** When the block for wrong passwords ends; null when the account is not blocked. */
  blockedUntil: string | null
  /** How many of the user's other devices may be playing when one more starts; 0 for no limit. */
  maxPlaybacks: number
  /** Highest bitrate of the user's playback, in bits per second; 0 for no limit. */
  maxBitrate: number
  /** Whether the user can watch Live TV. */
  liveTv: boolean
  /** What the user may do in SyncPlay (watching together). */
  syncPlay: SyncPlayAccess
  /** Whether the user may control other users' apps. */
  remoteControl: boolean
  /** Whether the user may schedule and delete Live TV recordings. */
  liveTvManagement: boolean
  /** IDs of the server's libraries this user's apps do not show; libraries added later show. */
  hiddenLibraries: string[]
  /** Titles of any of these genres are hidden from the user, like parental control. */
  blockedGenres: string[]
  /** When not empty, the only hours the user may use the server in (server time). */
  accessSchedules: AccessSchedule[]
  /** Whether the user may create, change and delete the collections every user sees. */
  collectionManagement: boolean
  /** The PIN asked for from a Jellyfin app's "Forgot password" screen; null when none is active. */
  passwordResetPin: PasswordResetPin | null
  /** Whether the user may add subtitle files to titles from their apps. */
  subtitleManagement: boolean
  /** Identifies the user's profile picture, served at /UserImage; null without one. */
  imageTag: string | null
  /** The tallest video the user is offered, in lines (480 to 2160); 0 for Original, no limit. */
  qualityGroup: QualityGroup
}

/** Jellyfin's SyncPlayUserAccessType. */
export type SyncPlayAccess = 'CreateAndJoinGroups' | 'JoinGroups' | 'None'

/** The quality groups the server accepts for User.qualityGroup, tallest first; 0 is Original. */
export const qualityGroups = [0, 2160, 1440, 1080, 720, 480] as const
export type QualityGroup = (typeof qualityGroups)[number]

/** The range the server accepts for User.maxPlaybacks. */
export const maxPlaybacksRange = { min: 0, max: 20 }

/** Jellyfin's DynamicDayOfWeek names, in its order. */
export const scheduleDays = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
  'Everyday',
  'Weekday',
  'Weekend',
] as const

export type ScheduleDay = (typeof scheduleDays)[number]

/** Hours from `startHour` to `endHour` (0 to 24, fractions allowed) on `day`. */
export type AccessSchedule = { day: ScheduleDay; startHour: number; endHour: number }

/** What the admin app offers for a user's content: the server's libraries and the genres they offer. */
export type UserContentChoices = { libraries: { id: string; name: string }[]; genres: string[] }

/** Jellyfin parental control: `maxRating` null means no limit. */
export type ParentalControl = {
  maxRating: number | null
  maxSubRating: number | null
  /** Jellyfin UnratedItem names; Polyfin titles use 'Movie' and 'Series'. */
  blockUnrated: string[]
}

export type ParentalRating = { name: string; score: number; subScore: number | null }

export type Device = {
  id: string
  deviceName: string
  client: string
  clientVersion: string
  remoteAddress: string
  createdAt: string
  lastActivityAt: string
}

export type QuickConnectRequest = {
  deviceName: string
  appName: string
  appVersion: string
  requestedAt: string
}

/** The skip marker sources, by the names the server gives them. */
export type SegmentSource = 'theintrodb' | 'introdb' | 'publicmetadb'

/** What the server accepts for a setting, and its default. */
export type SettingBounds = {
  default: unknown
  /** The bounds of a number, or of a text's length. */
  min?: number
  max?: number
  /** Whether 0 is accepted too, below min, turning the setting off. */
  zero?: boolean
  /** The values the setting takes, or a list holds. */
  choices?: unknown[]
}

export type Settings = {
  serverName: string
  quickConnectEnabled: boolean
  legacyAuthorization: boolean
  /** Language of the names the server generates for Jellyfin apps (seasons, library suffixes). */
  language: Language
  /** Analyzes a title's version when its page opens, and the next episode near the end of one. */
  prepareAhead: boolean
  /** Whether the server converts (transcodes) video and audio for apps that need it. */
  transcoding: boolean
  /** Items read at most from one movie or series catalog (any catalog but a Live TV one). */
  catalogLimit: number
  /** Items read at most from one Live TV catalog: its channels, or one day of its guide. */
  channelLimit: number
  /** Whether apps get skip intro, recap and credits buttons, from the segment databases. */
  skipButtons: boolean
  /** Whether a PublicMetaDB key is saved; it adds a third source of skip markers. */
  publicMetaDbKeySet: boolean
  /** Sent only to change the key: a new key, checked before it is saved, or "" to remove it. */
  publicMetaDbKey?: string
  /** Whether a TheIntroDB key is saved; optional, it raises TheIntroDB's daily limit. */
  theIntroDbKeySet: boolean
  /** Sent only to change the key: a new key, checked before it is saved, or "" to remove it. */
  theIntroDbKey?: string
  /** The order of preference of the skip marker sources, all three. */
  segmentOrder: SegmentSource[]
  /** The skip marker sources never asked, wherever they are in the order. */
  segmentSourcesOff: SegmentSource[]
  /** Whether apps get similar titles, from the addons' catalogs. */
  similarTitles: boolean
  /** Whether songs get lyrics from LRCLIB, which is sent their artist, title, album and length. */
  lyrics: boolean
  /** Percent of a title's runtime past which a reported position marks it played. */
  playedPercent: number
  /** Percent of a title's runtime past which a reported position is kept to resume; below playedPercent. */
  resumePercent: number
  /** Minutes a title's version and subtitle lists from the addons are used before the addons are asked again. */
  versionListMinutes: number
  /** Minutes catalog pages are kept. */
  catalogRefreshMinutes: number
  /** Whether users may add and use their own addons. */
  personalAddons: boolean
  /** Wrong passwords in a row that block an account for 15 minutes; 0 never blocks. */
  loginAttempts: number
  /** Days unused after which a Jellyfin app is signed out; 0 never signs it out. */
  inactiveDeviceDays: number
  /** Logs in detail (debug level), to diagnose a problem. */
  detailedLog: boolean
  /** Seconds one analysis of a version or a channel's stream may take before it is given up. */
  analysisTimeout: number
  /** Versions analyzed at most when an app plays a title without choosing a version. */
  versionAttempts: number
  /** Prefers the first version an app plays without conversion over the first that plays at all. */
  preferDirectPlay: boolean
  /** Playbacks whose video the server converts at once, 0 for no limit. */
  maxConversions: number
  /** Height converted video is scaled down to at most, 0 to keep the original's. */
  maxConversionHeight: number
  /** Encoder speed against quality: auto (Polyfin's choice), then veryslow to ultrafast. */
  encoderPreset: EncoderPreset
  /** Quality factor of H.264 and HEVC conversions (CRF, CQ or QVBR's), 0 aiming for the bitrate alone. */
  h264Quality: number
  hevcQuality: number
  /** Converts to HEVC for the apps that list it before H.264. */
  allowHevcEncoding: boolean
  /** GPU conversions run on. */
  hardwareAcceleration: HardwareAcceleration
  /** VAAPI render node conversions run on; empty tries each in turn. */
  vaapiDevice: string
  /** The render nodes found on the server (read-only). */
  renderNodes: string[]
  /** Disk space, in GB, kept for the parts of the files being read. */
  cacheSizeGb: number
  /** Codecs the GPU decodes. */
  hardwareDecodingCodecs: HardwareDecodingCodec[]
  /** Converts HDR to SDR with toneMappingAlgorithm; peak (nits, 0 the video's) and desaturation on the processor only. */
  toneMapping: boolean
  toneMappingAlgorithm: ToneMappingAlgorithm
  toneMappingPeak: number
  toneMappingDesat: number
  /** Deinterlacer, and whether it makes a frame of each field. */
  deinterlaceMethod: DeinterlaceMethod
  deinterlaceDoubleRate: boolean
  /** Stereo downmix, and the volume it is multiplied by. */
  downmixAlgorithm: DownmixAlgorithm
  downmixBoost: number
  /** Most channels of converted audio, 0 for no limit but the app's. */
  maxAudioChannels: number
  /** Bitrate of converted audio per channel, in kb/s; 0 keeps Polyfin's. */
  audioBitratePerChannel: number
  /** Threads FFmpeg converts with, 0 letting it choose. */
  encodingThreads: number
  /** Seconds of picture made ahead of the end of what the app asked for. */
  aheadSeconds: number
  /** What conversions run on (read-only). */
  conversionHardware: ConversionHardware
  /** Makes scrubbing thumbnails of the versions played, from their keyframes, in the background. */
  trickplay: boolean
  /** Shortest time between two scrubbing thumbnails, in seconds: longer titles get longer steps. */
  trickplayInterval: number
  /** Width of the scrubbing thumbnails, in pixels. */
  trickplayWidth: number
  /** Makes an image of each chapter of the versions played, from their keyframes. */
  chapterImages: boolean
  /** GB the thumbnails and chapter images take at most; past it, those used longest ago are dropped. */
  thumbnailStorageGB: number
  /** Seconds recordings start before their programme. */
  recordingPrePadding: number
  /** Seconds recordings go on after their programme. */
  recordingPostPadding: number
  /** Days after which recordings are deleted; 0 keeps them forever. */
  recordingRetentionDays: number
  /** Whether Live TV recording is on. */
  recording: boolean
  /** Folder recordings are written to; empty uses recordingsFolderDefault. */
  recordingsFolder: string
  /** Folder recordings are written to when recordingsFolder is empty (read-only). */
  recordingsFolderDefault: string
  /** Hours after which the XMLTV guides and IPTV channel lists are fetched again. */
  liveTvRefreshHours: number
  /** Hours after its last scan a local folder is scanned again; 0 for never on a schedule. */
  localScanHours: number
  /** CSS jellyfin-web applies to every page, unless a user turns it off (Jellyfin's branding). */
  customCss: string
  /** Script Polyfin adds to jellyfin-web's page; it runs in every user's browser. */
  customJs: string
  /** Text, Markdown or HTML jellyfin-web shows under its sign-in form (Jellyfin's branding). */
  loginDisclaimer: string
  /** Trakt app users connect through; both its client ID and secret are needed. */
  traktClientId: string
  /** Whether the Trakt client secret is saved. */
  traktClientSecretSet: boolean
  /** Sent only to change the secret: a new secret, or "" to remove it. */
  traktClientSecret?: string
  /** Simkl app users connect through. */
  simklClientId: string
  /** Last.fm API account users connect through; both its API key and shared secret are needed. */
  lastFmApiKey: string
  /** Whether the Last.fm shared secret is saved. */
  lastFmSecretSet: boolean
  /** Sent only to change the shared secret: a new one, or "" to remove it. */
  lastFmSecret?: string
  /** Hour of the server's time zone the database is backed up at every day, 0 to 23. */
  backupHour: number
  /** How many of the newest database backups are kept. */
  backupsKept: number
  /** Whether the database is backed up every day. */
  backups: boolean
  /** Folder backups are written to; empty uses backupFolderDefault. */
  backupFolder: string
  /** Folder backups are written to when backupFolder is empty (read-only). */
  backupFolderDefault: string
  /** Hour of the server's time zone every collection of the server's collection libraries is read at each day, 0 to 23; -1 never. */
  collectionReadHour: number
  /** Describes the versions not analyzed yet from RemuxDB, which the titles' IMDb ids are sent to. */
  remuxDb: boolean
  /** Address of the RemuxDB server asked: an http or https URL, without a trailing slash. */
  remuxDbUrl: string
  /** Address people open Polyfin at, which links in notifications start with; empty for no link. */
  publicAddress: string
  /** What each setting accepts, and its default, by its name here (read-only). */
  bounds: Record<string, SettingBounds>
}

/** The values of the conversion settings; the server lists them in Settings.bounds. */
export type EncoderPreset =
  | 'auto'
  | 'veryslow'
  | 'slower'
  | 'slow'
  | 'medium'
  | 'fast'
  | 'faster'
  | 'veryfast'
  | 'superfast'
  | 'ultrafast'
export type HardwareAcceleration = 'auto' | 'nvenc' | 'vaapi' | 'none'
export type HardwareDecodingCodec =
  'h264' | 'hevc' | 'hevc_10bit' | 'vp9' | 'av1' | 'mpeg2video' | 'vc1'
export type ToneMappingAlgorithm =
  'auto' | 'bt2390' | 'hable' | 'reinhard' | 'mobius' | 'clip' | 'linear'
export type DeinterlaceMethod = 'yadif' | 'bwdif'
export type DownmixAlgorithm = 'None' | 'Dave750' | 'NightmodeDialogue' | 'Rfc7845' | 'Ac4'

/** A GPU conversions run on. */
export type ConversionGPU = {
  /** cuda for NVIDIA, vaapi for AMD and Intel. */
  method: string
  device: string
  encoders: string[]
  toneMapping: boolean
  /** Whether a VAAPI GPU takes a quality factor. */
  qvbr: boolean
}

/** What conversions run on. */
export type ConversionHardware = {
  /** The GPU chosen, null for none. */
  gpu: ConversionGPU | null
  /** Software video encoders FFmpeg has. */
  encoders: string[]
  /** Whether FFmpeg tone maps HDR on the processor. */
  toneMapping: boolean
  /** Whether FFmpeg has the bwdif deinterlacer. */
  bwdif: boolean
}

export type NewUser = {
  name: string
  password: string
  isAdministrator: boolean
  isHidden: boolean
}

export type UserPatch = Partial<{
  name: string
  password: string
  isAdministrator: boolean
  isHidden: boolean
  isDisabled: boolean
  parentalControl: ParentalControl
  transcoding: boolean
  downloads: boolean
  personalAddons: boolean
  maxPlaybacks: number
  maxBitrate: number
  liveTv: boolean
  syncPlay: SyncPlayAccess
  remoteControl: boolean
  liveTvManagement: boolean
  hiddenLibraries: string[]
  blockedGenres: string[]
  accessSchedules: AccessSchedule[]
  collectionManagement: boolean
  subtitleManagement: boolean
  qualityGroup: QualityGroup
}>

/** Who owns addons and libraries: the server (administrators only) or the signed-in user. */
export type Scope = 'shared' | 'me'

export type Addon = {
  id: string
  name: string
  version: string
  description: string
  logo: string | null
  /** Redacted: the credentials it may embed are never returned. */
  manifestUrl: string
  enabled: boolean
  resources: string[]
  types: string[]
  catalogCount: number
  refreshedAt: string
  /**
   * 'stremio' for a Stremio addon; 'eclipse' for an Eclipse music addon, which `music` describes;
   * 'm3u' or 'xtream' for an IPTV source, which `source` describes; 'local' for a local folder,
   * which `folder` describes.
   */
  kind: 'stremio' | 'eclipse' | 'm3u' | 'xtream' | 'local'
  source: IptvSource | null
  music: AddonMusic | null
  folder: LocalFolder | null
}

/** What a local folder holds. */
export type FolderKind = 'movies' | 'shows'

/** A local folder: a folder mounted in the container, scanned for the titles of its files. */
export type LocalFolder = {
  kind: FolderKind
  /** Its path in the container. */
  path: string
  /** The last scan attempt, and the last scan that could read the folder; null before the first. */
  checkedAt: string | null
  scannedAt: string | null
  /** Why the last scan could not read the folder: 'missing', 'unreadable', 'not_folder'; else empty. */
  error: string
  /** Whether a scan is under way. */
  scanning: boolean
  files: number
  matched: number
  unmatched: number
  /** The files, or shows' folders, linked to an IMDb identifier by hand. */
  links: { unit: string; imdbId: string }[]
}

/** A file of a local folder no title was matched to, with why. */
export type UnmatchedFile = {
  path: string
  /** What a link would link: the file, or its show's folder. */
  unit: string
  title: string
  year: number | null
  size: number
  reason: 'unreadable_name' | 'not_found' | 'ambiguous' | 'other_year' | 'search_failed'
}

export type NewLocalFolder = { name: string; path: string; kind: FolderKind }

export type LocalFolderPatch = Partial<{ name: string; path: string }>

/** What an Eclipse addon's tracks are; its catalog rows become music or books libraries. */
export type MusicContent = 'music' | 'audiobook' | 'podcast'

/** A setting an Eclipse addon declares; values travel as strings, toggles as "true" or "false". */
export type AddonSetting = {
  key: string
  type: 'select' | 'toggle' | 'text' | 'number'
  label: string
  help: string
  default: string
  options: { value: string; label: string }[]
  /** Text only; 0 for the server's own limit of 1,000 characters. */
  maxLength: number
  placeholder: string
  min: number | null
  max: number | null
  step: number | null
}

/** An Eclipse addon's content, settings, and the values chosen (a setting left out uses its default). */
export type AddonMusic = {
  contentType: MusicContent
  settings: AddonSetting[]
  values: Record<string, string>
}

/** An IPTV source's channel list, how it was last fetched, and its line-up. */
export type IptvSource = {
  /** Redacted: the credentials it holds are never returned. */
  address: string
  /** Entries of the provider's list, before exclusions and merging. */
  channels: number
  options: IptvOptions
  lineup: LineupCounts
  /** The movies and series of the provider's lists; zeros while neither is imported. */
  vod: VodCounts
  checkedAt: string | null
  fetchedAt: string | null
  nextAt: string | null
  /** Code of the last failure; empty after a success. */
  error: string
}

/** How a source's list becomes its line-up. */
export type IptvOptions = {
  /** Provider groups, or one category per country. */
  categories: 'original' | 'country'
  /** One channel per entry, or quality variants of the same name merged into one channel. */
  channels: 'original' | 'merged'
  /** Preview keys (`g:<group>`, `c:<country>`) whose entries are not imported. */
  excluded: string[]
  /** Whether channels appearing on a refresh arrive enabled. */
  newChannels: boolean
  /** A channel without a fixed number takes the provider's number, or its place. */
  numbering: 'provider' | 'sequential'
  /** What is imported: the live channels, the movies, the series; at least one. */
  liveTv: boolean
  movies: boolean
  series: boolean
  /** VOD preview keys (`movie:<category>`, `series:<category>`) whose titles are not imported. */
  vodExcluded: string[]
  /** One library per type, or one per provider category. */
  vodLibraries: 'type' | 'category'
  /** Whether titles with a TMDB or IMDb id are described by the server's metadata addons. */
  enrichment: boolean
}

export type VodCounts = {
  /** Titles in the provider's lists, before exclusions. */
  movies: number
  series: number
  /** Episodes known: for Xtream, those of the series opened so far. */
  episodes: number
  /** Titles imported: their type on, their category not left out. */
  shownMovies: number
  shownSeries: number
  movieCategories: number
  seriesCategories: number
}

export const defaultIptvOptions: IptvOptions = {
  categories: 'original',
  channels: 'original',
  excluded: [],
  newChannels: true,
  numbering: 'provider',
  liveTv: true,
  movies: false,
  series: false,
  vodExcluded: [],
  vodLibraries: 'type',
  enrichment: true,
}

/** The limits the server accepts for IptvOptions.excluded. */
export const excludedKeysLimit = 5000

export type LineupCounts = {
  categories: number
  enabledCategories: number
  channels: number
  enabledChannels: number
  /** Enabled, in an enabled category, with an enabled stream: what Jellyfin apps list. */
  shownChannels: number
  mapped: number
  unmapped: number
  /** Channels shown in apps whose provider keeps past programmes: the folders of Replay. */
  archived: number
}

/** What a preview counts: live entries by group or country, or the titles of a VOD type. */
export type PreviewBy = 'group' | 'country' | 'movie' | 'series'

/** A category of a source's list, before import: a group, a country, or a VOD category. */
export type PreviewCategory = { key: string; name: string; channels: number; excluded: boolean }

export type Preview = { total: number; categories: PreviewCategory[] }

/** The account a new source is previewed from. */
export type IptvAccount = {
  kind: 'm3u' | 'xtream'
  url?: string
  server?: string
  username?: string
  password?: string
}

/** A paged answer: `total` counts every row matching the filters. */
export type Page<T> = { total: number; offset: number; limit: number; items: T[] }

/** The most rows one page of the API returns. */
export const pageLimit = 500

export type LineupCategory = {
  id: string
  /** `g:`, `c:` or, for a custom category, `u:<id>`. */
  key: string
  /** The shown name: the admin's or the provider's. */
  name: string
  /** Empty for a custom category. */
  providerName: string
  custom: boolean
  enabled: boolean
  position: number
  channels: number
  enabledChannels: number
}

/** How a stream last answered when its channel was opened. */
export type StreamHealth = {
  okAt: string | null
  /** dead (no live stream), refused (not served now), timeout (nothing came), or empty. */
  failure: '' | 'dead' | 'refused' | 'timeout'
  failedAt: string | null
  failures: number
  /** Set while the stream is left out of its channel. */
  hiddenUntil: string | null
}

export type ChannelStream = {
  id: string
  label: string
  enabled: boolean
  custom: boolean
  /** Redacted address of a custom stream; null for the provider's. */
  address: string | null
  health: StreamHealth
}

/** The guide channel a channel takes; manual with null ids pins "no guide". */
export type GuideMapping = {
  guideId: string | null
  guideChannelId: string | null
  guideChannelName: string | null
  manual: boolean
}

export type LineupChannel = {
  /** The channel's Jellyfin item id; it never changes. */
  id: string
  name: string
  providerName: string
  renamed: boolean
  logo: string | null
  providerLogo: string | null
  description: string
  category: { id: string; name: string }
  providerCategoryId: string
  moved: boolean
  enabled: boolean
  shown: boolean
  number: number | null
  providerNumber: number | null
  fixedNumber: number | null
  /** The provider's tvg-id, empty without. */
  guideId: string
  mapping: GuideMapping | null
  streams: ChannelStream[]
  /** How many days back the provider keeps the channel's programmes, played from Replay; 0 for no archive. */
  archiveDays: number
}

export type ChannelFilters = {
  category?: string
  enabled?: boolean
  shown?: boolean
  mapped?: boolean
  archive?: boolean
  q?: string
}

export type ChannelPatch = Partial<{
  enabled: boolean
  name: string | null
  logo: string | null
  description: string
  category: string | null
  number: number | null
}>

/** Which channels a bulk change reaches: exactly one selector. */
export type BulkSelector =
  { ids: string[] } | { category: string } | { q: string; inCategory?: string }

/** A Live TV catalog: an IPTV source's is `{addonId, catalogType: 'tv', catalogId: 'channels'}`. */
export type CatalogTarget = { addonId: string; catalogType: string; catalogId: string }

export type CatalogGuide = {
  id: string
  position: number
  /** Redacted address. */
  url: string
  checkedAt: string | null
  fetchedAt: string | null
  nextAt: string | null
  channels: number
  programmes: number
  error: string
}

export type CatalogGuides = {
  guides: CatalogGuide[]
  channels: number
  mapped: number
  manual: number
}

/** A channel of one of a catalog's guides. */
export type GuideChannel = {
  guideId: string
  guidePosition: number
  id: string
  name: string
  names: string[]
  icon: string | null
  now: { title: string; start: string; end: string } | null
}

export type MappingState = 'all' | 'mapped' | 'unmapped' | 'manual'

export type MappingItem = {
  channelId: string
  name: string
  number: number | null
  guideId: string
  mapping: GuideMapping | null
}

/** An IPTV source to add. */
export type NewIptvSource = {
  name: string
  kind: 'm3u' | 'xtream'
  url?: string
  server?: string
  username?: string
  password?: string
  guideUrl?: string
  /** Xtream only: use the guide the server publishes for the account. */
  providerGuide?: boolean
  options?: Partial<IptvOptions>
}

/** What changes of an IPTV source; a password left empty keeps the current one. */
export type IptvSourcePatch = Partial<{
  name: string
  url: string
  server: string
  username: string
  password: string
  options: Partial<IptvOptions>
}>

export type AddonPatch = Partial<{ enabled: boolean; manifestUrl: string }>

export type Library = {
  addonId: string
  addonName: string
  catalogType: string
  catalogId: string
  catalogName: string
  /** Custom name; null shows the catalog name. */
  name: string | null
  /** Name Jellyfin apps show, told apart from same-named libraries; null when apps do not show it. */
  appName: string | null
  enabled: boolean
  browsable: boolean
  /** The library's item in Jellyfin apps; null for a catalog that is not an enabled library. */
  itemId: string | null
  /** How it finds the image apps show on its tile; `custom` once one was uploaded for it. */
  image: LibraryImageChoice
  /** Tag of that image, served as the item's Primary image; null when it shows none. */
  imageTag: string | null
  /** First XMLTV guide of an enabled TV catalog; null for any other library. */
  guide: Guide | null
  /** Every guide of an enabled TV catalog, in order; null for any other library. */
  guides: CatalogGuide[] | null
  /** Genre the library is narrowed to; null lists the whole catalog. */
  genre: string | null
  /** Genres the catalog offers in its genre filter, in its order; empty when it offers none. */
  genres: string[]
  /** Most titles the library lists, its collections' included; null for no maximum but the server's catalog limit. */
  maxItems: number | null
  /** Whether the library takes a genre (when `genres` is not empty) and a maximum: false for live TV and music catalogs. */
  filterable: boolean
  /**
   * Whether the web player leaves the library out of its top bar, that bar's More menu and its side
   * menu; its home screen row and other Jellyfin apps keep it. False for a catalog that is not an
   * enabled library.
   */
  hideInMenus: boolean
}

/** The XMLTV guide of a TV catalog and how its last fetch went. */
export type Guide = {
  /** Redacted address: the credentials it may embed are never returned. Empty: no guide. */
  url: string
  checkedAt: string | null
  /** Last successful fetch. */
  fetchedAt: string | null
  /** Channels of the catalog at the last fetch, and those the guide covers. */
  channels: number
  matched: number
  /** Code of the last failure; empty after a success. */
  error: string
  /** When the guide is fetched again; null without a guide or before its first fetch. */
  nextAt: string | null
}

/** The most titles a library may be set to list; the least is 1. */
export const MAX_LIBRARY_ITEMS = 20000

/** One enabled library in the list sent to PUT /scopes/{scope}/libraries. */
export type LibrarySelection = Pick<Library, 'addonId' | 'catalogType' | 'catalogId' | 'name'> &
  Partial<Pick<Library, 'genre' | 'maxItems' | 'hideInMenus'>>

export type LibraryImageChoice = 'none' | 'automatic' | 'custom'

/**
 * Chooses the image of one enabled library: none, automatic, or custom with a picture sent in
 * base64 (`data`) or found at an address the server downloads once (`url`).
 */
export type LibraryImageRequest = Pick<Library, 'addonId' | 'catalogType' | 'catalogId'> &
  (
    | { image: 'none' | 'automatic' }
    | { image: 'custom'; data: string }
    | { image: 'custom'; url: string }
  )

/**
 * `parentalControl`: the user's parental control keeps them on the server's addons only.
 * `personalAddons`: false while the server or the user's permission turns their own addons off;
 * they are then kept but not used, and the server's addons are used.
 */
export type AddonPreferences = {
  useSharedAddons: boolean
  parentalControl: boolean
  personalAddons: boolean
}

/** Signing in with `pin` as the password, before `expiresAt`, makes it the user's new password. */
export type PasswordResetPin = { pin: string; expiresAt: string }

/** A key tools and apps use to call the Jellyfin API with administrator rights. */
export type ApiKey = { id: string; app: string; createdAt: string; lastUsedAt: string | null }

/** A key just created: `key` is the secret, returned this one time only. */
export type NewApiKey = ApiKey & { key: string }

export type ActivityEntry = {
  id: number
  date: string
  /** A readable sentence in the server language, such as "alice signed in". */
  name: string
  type: string
  overview: string | null
  shortOverview: string | null
  severity: 'Information' | 'Warning' | 'Error'
  userId: string | null
}

export type ActivityPage = { items: ActivityEntry[]; total: number }

/**
 * An HTTP error from the admin API. `code` is the machine code from `{"error": "..."}`;
 * `jellyfinId` names the Jellyfin user a Jellyfin import error is about, when the server adds one.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly jellyfinId: string | null

  constructor(status: number, code: string, jellyfinId: string | null = null) {
    super(`Admin API error ${status}: ${code}`)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.jellyfinId = jellyfinId
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

const apiBase = `${import.meta.env.BASE_URL}api`

export async function request<T>(
  method: Method,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
  extraHeaders?: Record<string, string>,
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json', ...extraHeaders }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const response = await fetch(`${apiBase}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  if (!response.ok) {
    let code = 'unknown'
    let jellyfinId: string | null = null
    try {
      const payload = (await response.json()) as { error?: unknown; jellyfinId?: unknown }
      if (typeof payload.error === 'string') code = payload.error
      if (typeof payload.jellyfinId === 'string') jellyfinId = payload.jellyfinId
    } catch {
      // Non-JSON error body (proxy page, empty response): keep the generic code.
    }
    throw new ApiError(response.status, code, jellyfinId)
  }
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

const seg = encodeURIComponent

export const fetchStatus = (signal?: AbortSignal) =>
  request<Status>('GET', '/status', undefined, signal)

/** `language` is the interface language; the server adopts it when it speaks it. */
export const setup = async (body: {
  setupCode: string
  name: string
  password: string
  language: Language
}) => (await request<{ user: SessionUser }>('POST', '/setup', body)).user

export const signIn = async (body: { name: string; password: string }) =>
  (await request<{ user: SessionUser }>('POST', '/session', body)).user

/**
 * Returns the signed-in user, or null when there is no valid session. Without one, the web client's
 * sign-in on this server is tried first: it opens a session for an administrator.
 */
export async function fetchSession(signal?: AbortSignal): Promise<SessionUser | null> {
  try {
    return (await request<{ user: SessionUser }>('GET', '/session', undefined, signal)).user
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401)) throw error
  }
  const token = takeWebClientToken(queryClient.getQueryData<Status>(queryKeys.status)?.serverId)
  if (token === null) return null
  try {
    const authorization = { Authorization: `MediaBrowser Token="${token}"` }
    return (
      await request<{ user: SessionUser }>(
        'POST',
        '/session/jellyfin',
        undefined,
        signal,
        authorization,
      )
    ).user
  } catch {
    // Refused or unreachable: the sign-in page it is.
    return null
  }
}

export const signOut = () => request<void>('DELETE', '/session')

export const changePassword = (body: { currentPassword: string; newPassword: string }) =>
  request<void>('PUT', '/account/password', body)

export const fetchMyDevices = (signal?: AbortSignal) =>
  request<Device[]>('GET', '/account/devices', undefined, signal)

export const signOutMyDevice = (id: string) =>
  request<void>('DELETE', `/account/devices/${seg(id)}`)

/** The tracking services, in the order the server lists them. */
export type TrackingServiceName =
  'trakt' | 'simkl' | 'mdblist' | 'publicmetadb' | 'lastfm' | 'listenbrainz'

/** The services users connect with an API key or user token, which they can show again. */
export type TrackingKeyServiceName = 'mdblist' | 'publicmetadb' | 'listenbrainz'

/** One of the signed-in user's tracking services, which Polyfin tells what the user watches. */
export type TrackingService = {
  service: TrackingServiceName
  /**
   * "code": the user enters a code on the service's site; "signin": the user signs in on the
   * service's site and allows Polyfin; "key": the user pastes an API key or user token.
   */
  connection: 'code' | 'signin' | 'key'
  /** The service is told the songs the user plays; it has no history to import. */
  music: boolean
  /** False only for a code or sign-in service whose app the server is not set up with. */
  available: boolean
  connected: boolean
  /** The account name the service reports. */
  account: string | null
  connectedAt: string | null
  /** The last time the service accepted something for this user. */
  lastSentAt: string | null
  /**
   * "reconnect": the service refused the connection; "unreachable": sends failed and are retried;
   * "app_refused": the service refused the server's app while the user's code waited.
   */
  problem: 'reconnect' | 'unreachable' | 'app_refused' | null
  /**
   * While a code connection waits for the user to enter the code on the service's site, or a
   * sign-in waits for the user to allow Polyfin on the page `verificationUrl` (`userCode` empty).
   */
  code: { userCode: string; verificationUrl: string; expiresAt: string } | null
  /** Whether the service's watch history is imported into Polyfin, every 6 hours. */
  importHistory: boolean
  /** An import runs now. */
  importing: boolean
  /** How the last import went; null before the first. */
  lastImport: TrackingImport | null
}

/** How an import of a watch history went. */
export type TrackingImport = {
  at: string
  /** Titles it newly marked played. */
  played: number
  /** Resume points it set. */
  resumed: number
  /** Titles of the history Polyfin could not identify. */
  unmapped: number
  /** Why the history could not be read whole; what was read is imported all the same. */
  problem: 'reconnect' | 'unreachable' | 'rate_limited' | null
}

export const fetchTracking = async (signal?: AbortSignal) =>
  (await request<{ services: TrackingService[] }>('GET', '/account/tracking', undefined, signal))
    .services

/** Connects a key service with `key`, or starts a code connection without it. */
export const connectTracking = (service: TrackingServiceName, key?: string) =>
  request<TrackingService>(
    'POST',
    `/account/tracking/${seg(service)}`,
    key === undefined ? {} : { key },
  )

/** Turns the import of a connected service's watch history on (importing it at once) or off. */
export const setTrackingImport = (service: TrackingServiceName, importHistory: boolean) =>
  request<TrackingService>('PATCH', `/account/tracking/${seg(service)}`, { importHistory })

/** Imports a service's watch history at once. */
export const importTracking = (service: TrackingServiceName) =>
  request<TrackingService>('POST', `/account/tracking/${seg(service)}/import`)

/** Disconnects a service, or cancels the code it waits for. */
export const disconnectTracking = (service: TrackingServiceName) =>
  request<void>('DELETE', `/account/tracking/${seg(service)}`)

/** Reads the signed-in user's saved API key or user token again; never cached. */
export const revealTrackingKey = async (service: TrackingKeyServiceName) =>
  (await request<{ value: string }>('POST', `/account/tracking/${seg(service)}/key/reveal`)).value

/** The kinds of notification targets. */
export type NotificationKind = 'webhook' | 'discord' | 'ntfy'

/** The events targets choose from. */
export type NotificationEvent =
  'new_episode' | 'recording_finished' | 'recording_failed' | 'health_problem' | 'health_solved'

/**
 * Where notifications go. Its secret address and token are never sent back: `address` is only the
 * scheme and host of a webhook's or Discord target's address, and an ntfy target's server.
 */
export type NotificationTarget = {
  id: string
  kind: NotificationKind
  name: string
  address: string
  /** An ntfy target's topic, empty for the others. */
  topic: string
  /** Whether an ntfy target has an access token. */
  tokenSet: boolean
  events: NotificationEvent[]
  enabled: boolean
  createdAt: string
  /** The last time the target accepted a message. */
  lastSentAt: string | null
  /**
   * "refused": it answered 401, 403, 404 or 410; "rejected": it refused a message (another 4xx);
   * "unreachable": messages could not be delivered; "unreadable": its address or token cannot be
   * decrypted with POLYFIN_SECRET_KEY.
   */
  problem: 'refused' | 'rejected' | 'unreachable' | 'unreadable' | null
  /** The HTTP status the target answered with its problem, when it answered. */
  problemStatus: number | null
  problemAt: string | null
}

/** The targets of the server, or of the signed-in user, and what they may choose. */
export type Notifications = {
  targets: NotificationTarget[]
  /** The events these targets may receive: health events only for the server and administrators. */
  events: NotificationEvent[]
  kinds: NotificationKind[]
}

/** A target to add, or the changes to one: fields left out keep their values. */
export type NotificationDraft = {
  kind?: NotificationKind
  name?: string
  /** A webhook's or Discord target's address, or an ntfy target's server (empty: the public one). */
  address?: string
  topic?: string
  /** An ntfy target's access token; empty removes it. */
  token?: string
  events?: NotificationEvent[]
  enabled?: boolean
}

/** Whose targets: the server's (Settings › Notifications) or the signed-in user's own. */
export type NotificationScope = 'server' | 'own'

const notificationsPath = (scope: NotificationScope) =>
  scope === 'server' ? '/notifications' : '/account/notifications'

export const fetchNotifications = (scope: NotificationScope, signal?: AbortSignal) =>
  request<Notifications>('GET', notificationsPath(scope), undefined, signal)

export const createNotificationTarget = (scope: NotificationScope, draft: NotificationDraft) =>
  request<NotificationTarget>('POST', `${notificationsPath(scope)}/targets`, draft)

export const updateNotificationTarget = (
  scope: NotificationScope,
  id: string,
  draft: NotificationDraft,
) => request<NotificationTarget>('PATCH', `${notificationsPath(scope)}/targets/${seg(id)}`, draft)

export const deleteNotificationTarget = (scope: NotificationScope, id: string) =>
  request<void>('DELETE', `${notificationsPath(scope)}/targets/${seg(id)}`)

/** Sends a test message at once: whether the target accepted it, and how it stands after. */
export const testNotificationTarget = (scope: NotificationScope, id: string) =>
  request<{ delivered: boolean; status: number | null; target: NotificationTarget }>(
    'POST',
    `${notificationsPath(scope)}/targets/${seg(id)}/test`,
  )

export const lookupQuickConnect = (code: string, signal?: AbortSignal) =>
  request<QuickConnectRequest>('GET', `/quick-connect/${seg(code)}`, undefined, signal)

export const approveQuickConnect = (code: string) =>
  request<void>('POST', '/quick-connect', { code })

export const fetchUsers = (signal?: AbortSignal) =>
  request<User[]>('GET', '/users', undefined, signal)

export const createUser = (body: NewUser) => request<User>('POST', '/users', body)

export const updateUser = (id: string, patch: UserPatch) =>
  request<User>('PATCH', `/users/${seg(id)}`, patch)

export const deleteUser = (id: string) => request<void>('DELETE', `/users/${seg(id)}`)

export const fetchUserDevices = (id: string, signal?: AbortSignal) =>
  request<Device[]>('GET', `/users/${seg(id)}/devices`, undefined, signal)

export const signOutUserDevice = (id: string, deviceId: string) =>
  request<void>('DELETE', `/users/${seg(id)}/devices/${seg(deviceId)}`)

/** Ends the block of a user's account for wrong passwords. */
export const unblockUser = (id: string) => request<User>('POST', `/users/${seg(id)}/unblock`)

/** Takes the permission to download away from every user; answers with those who had it. */
export const turnOffDownloads = () => request<User[]>('POST', '/users/downloads/off')

/** A Jellyfin server as the import reads it; `address` is the normalized one to send back. */
export type JellyfinServer = { name: string; version: string; address: string }

/** A user of the Jellyfin server, with the Polyfin user of the same name as a suggestion. */
export type JellyfinUser = {
  id: string
  name: string
  isAdministrator: boolean
  isDisabled: boolean
  isHidden: boolean
  lastActivityAt: string | null
  /** The Polyfin user with the same name (case ignored), else null. */
  userId: string | null
}

/**
 * One Jellyfin user to import, with exactly one of `userId` (an existing Polyfin user) or
 * `create` (a new one). An existing user without `watchData` gets nothing.
 */
export type JellyfinImportEntry = {
  jellyfinId: string
  userId?: string
  create?: NewUser
  watchData: boolean
}

/** Why an import, or one of its users, failed. */
export type JellyfinImportProblem =
  'jellyfin_unreachable' | 'jellyfin_key_refused' | 'not_jellyfin' | 'internal'

/** A title of a Jellyfin user's watch data that no Polyfin title matches. */
export type JellyfinUnmatched = {
  name: string
  type: 'movie' | 'episode' | 'series'
  year: number | null
  /** For an episode: its series, season and number, when Jellyfin knows them. */
  series: string | null
  season: number | null
  episode: number | null
  /** `no_identifier`: no IMDb, TMDB or TVDB identifier; `not_found`: no Polyfin title has it. */
  reason: 'no_identifier' | 'not_found'
}

/** How the import of one Jellyfin user's watch data goes. */
export type JellyfinUserImport = {
  jellyfinId: string
  jellyfinName: string
  userId: string
  userName: string
  /** A finished import's `waiting` users were not reached: it stopped or failed first. */
  state: 'waiting' | 'reading' | 'saving' | 'done' | 'failed'
  /** Items read from Jellyfin so far. */
  read: number
  /** Titles newly marked played, resume points set, titles newly marked favorite. */
  played: number
  resumed: number
  favorites: number
  /** Every title not matched; `unmatched` lists the first 500 only. */
  unmatchedCount: number
  unmatched: JellyfinUnmatched[]
  problem: JellyfinImportProblem | null
}

/** The running import, else the last one since Polyfin started: a restart forgets it. */
export type JellyfinImportStatus = {
  id: string
  server: { name: string; address: string }
  state: 'running' | 'done' | 'stopped' | 'failed'
  problem: JellyfinImportProblem | null
  startedAt: string
  endedAt: string | null
  users: JellyfinUserImport[]
}

/** What connecting to a Jellyfin server reads: the server and its users. */
export type JellyfinConnection = { server: JellyfinServer; users: JellyfinUser[] }

/** Reads a Jellyfin server's users with its API key, which the server never keeps. */
export const connectJellyfin = (body: { address: string; apiKey: string }) =>
  request<JellyfinConnection>('POST', '/jellyfin-import/users', body)

/**
 * Creates the new users (all or none), then starts importing the watch data in the background;
 * `import` is null when no entry asked for watch data.
 */
export const startJellyfinImport = (body: {
  address: string
  apiKey: string
  users: JellyfinImportEntry[]
}) =>
  request<{ created: User[]; import: JellyfinImportStatus | null }>(
    'POST',
    '/jellyfin-import',
    body,
  )

/** The answer of the status and stop routes. */
type JellyfinImportAnswer = { import: JellyfinImportStatus | null }

/** The running import, else the last one, else null. */
export const fetchJellyfinImport = async (signal?: AbortSignal) =>
  (await request<JellyfinImportAnswer>('GET', '/jellyfin-import', undefined, signal)).import

/** Stops the running import; what it imported stays. */
export const stopJellyfinImport = async () =>
  (await request<JellyfinImportAnswer>('POST', '/jellyfin-import/stop')).import

export const fetchParentalRatings = (signal?: AbortSignal) =>
  request<ParentalRating[]>('GET', '/parental-ratings', undefined, signal)

export const fetchUserContentChoices = (signal?: AbortSignal) =>
  request<UserContentChoices>('GET', '/user-content-choices', undefined, signal)

export const fetchSettings = (signal?: AbortSignal) =>
  request<Settings>('GET', '/settings', undefined, signal)

/** Saves the settings; their bounds are the server's, and are not sent. */
export const saveSettings = (settings: Settings) =>
  request<Settings>('PUT', '/settings', { ...settings, bounds: undefined })

/** The server's secrets an administrator can read again. */
export type ServerSecretName =
  'publicMetaDbKey' | 'theIntroDbKey' | 'traktClientSecret' | 'lastFmSecret'

/** Reads one of the server's saved secrets again, for an administrator; never cached. */
export const revealServerSecret = async (name: ServerSecretName) =>
  (await request<{ value: string }>('POST', `/settings/secrets/${seg(name)}/reveal`)).value

const scopePath = (scope: Scope) => `/scopes/${seg(scope)}`

export const fetchAddons = (scope: Scope, signal?: AbortSignal) =>
  request<Addon[]>('GET', `${scopePath(scope)}/addons`, undefined, signal)

export const installAddon = (scope: Scope, manifestUrl: string) =>
  request<Addon>('POST', `${scopePath(scope)}/addons`, { manifestUrl })

export const updateAddon = (scope: Scope, id: string, patch: AddonPatch) =>
  request<Addon>('PATCH', `${scopePath(scope)}/addons/${seg(id)}`, patch)

export const refreshAddon = (scope: Scope, id: string) =>
  request<Addon>('POST', `${scopePath(scope)}/addons/${seg(id)}/refresh`)

export const addIptvSource = (scope: Scope, source: NewIptvSource) =>
  request<Addon>('POST', `${scopePath(scope)}/iptv`, source)

export const updateIptvSource = (scope: Scope, id: string, patch: IptvSourcePatch) =>
  request<Addon>('PATCH', `${scopePath(scope)}/iptv/${seg(id)}`, patch)

/** Adds a local folder to the server's sources; it is scanned at once, in the background. */
export const addLocalFolder = (folder: NewLocalFolder) =>
  request<Addon>('POST', `${scopePath('shared')}/folders`, folder)

export const updateLocalFolder = (id: string, patch: LocalFolderPatch) =>
  request<Addon>('PATCH', `${scopePath('shared')}/folders/${seg(id)}`, patch)

/** Starts a scan of a local folder; the answer tells the scan under way. */
export const scanLocalFolder = (id: string) =>
  request<Addon>('POST', `${scopePath('shared')}/folders/${seg(id)}/scan`)

export const fetchUnmatchedFiles = (id: string, signal?: AbortSignal) =>
  request<{ total: number; files: UnmatchedFile[] }>(
    'GET',
    `${scopePath('shared')}/folders/${seg(id)}/unmatched`,
    undefined,
    signal,
  )

/** Links a file, or a show's folder, to an IMDb identifier; the link survives rescans. */
export const linkLocalFile = (id: string, path: string, imdbId: string) =>
  request<Addon>('PUT', `${scopePath('shared')}/folders/${seg(id)}/links`, { path, imdbId })

export const unlinkLocalFile = (id: string, path: string) =>
  request<Addon>(
    'DELETE',
    `${scopePath('shared')}/folders/${seg(id)}/links?path=${encodeURIComponent(path)}`,
  )

/** Replaces the values of an Eclipse addon's settings; the addon receives them on every request. */
export const saveAddonSettings = (scope: Scope, id: string, values: Record<string, string>) =>
  request<Addon>('PUT', `${scopePath(scope)}/addons/${seg(id)}/settings`, { values })

export const deleteAddon = (scope: Scope, id: string) =>
  request<void>('DELETE', `${scopePath(scope)}/addons/${seg(id)}`)

/** Sets the order of the scope's addons; `ids` must list every addon of the scope. */
export const orderAddons = (scope: Scope, ids: string[]) =>
  request<void>('PUT', `${scopePath(scope)}/addons/order`, { ids })

export const fetchLibraries = (scope: Scope, signal?: AbortSignal) =>
  request<Library[]>('GET', `${scopePath(scope)}/libraries`, undefined, signal)

/** Replaces the scope's enabled libraries with `libraries`, in this order. */
export const saveLibraries = (scope: Scope, libraries: LibrarySelection[]) =>
  request<Library[]>('PUT', `${scopePath(scope)}/libraries`, { libraries })

/** Chooses one library's image at once, answering the scope's libraries. */
export const saveLibraryImage = (scope: Scope, body: LibraryImageRequest) =>
  request<Library[]>('PUT', `${scopePath(scope)}/libraries/image`, body)

/** Query parameters, without those left undefined. */
function params(values: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  return search.toString()
}

const sourcePath = (scope: Scope, id: string) => `${scopePath(scope)}/iptv/${seg(id)}`

/** Downloads a new account's list and counts its categories; nothing is stored. */
export const previewNewSource = (
  scope: Scope,
  account: IptvAccount,
  by: PreviewBy,
  signal?: AbortSignal,
) => request<Preview>('POST', `${scopePath(scope)}/iptv/preview`, { ...account, by, q: '' }, signal)

/** Counts a source's categories from its stored list; `excluded` follows its options. */
export const previewSource = (scope: Scope, id: string, by: PreviewBy, signal?: AbortSignal) =>
  request<Preview>('GET', `${sourcePath(scope, id)}/preview?${params({ by })}`, undefined, signal)

export const fetchLineupCategories = (scope: Scope, id: string, signal?: AbortSignal) =>
  request<{ items: LineupCategory[] }>(
    'GET',
    `${sourcePath(scope, id)}/categories`,
    undefined,
    signal,
  )

export const createLineupCategory = (scope: Scope, id: string, name: string) =>
  request<LineupCategory>('POST', `${sourcePath(scope, id)}/categories`, { name })

/** `name: null` goes back to the provider's name. */
export const updateLineupCategory = (
  scope: Scope,
  id: string,
  categoryId: string,
  patch: Partial<{ name: string | null; enabled: boolean }>,
) =>
  request<LineupCategory>('PATCH', `${sourcePath(scope, id)}/categories/${seg(categoryId)}`, patch)

export const deleteLineupCategory = (scope: Scope, id: string, categoryId: string) =>
  request<void>('DELETE', `${sourcePath(scope, id)}/categories/${seg(categoryId)}`)

/** Sets the order of the categories; `ids` must list every category once. */
export const orderLineupCategories = (scope: Scope, id: string, ids: string[]) =>
  request<void>('PUT', `${sourcePath(scope, id)}/categories/order`, { ids })

/** Turns categories on or off; without `ids`, every category. */
export const bulkLineupCategories = (scope: Scope, id: string, enabled: boolean, ids?: string[]) =>
  request<{ changed: number }>('POST', `${sourcePath(scope, id)}/categories/bulk`, { enabled, ids })

export const fetchLineupChannels = (
  scope: Scope,
  id: string,
  filters: ChannelFilters,
  offset: number,
  limit: number,
  signal?: AbortSignal,
) =>
  request<Page<LineupChannel>>(
    'GET',
    `${sourcePath(scope, id)}/channels?${params({ ...filters, offset, limit })}`,
    undefined,
    signal,
  )

export const fetchLineupChannel = (
  scope: Scope,
  id: string,
  channelId: string,
  signal?: AbortSignal,
) =>
  request<LineupChannel>(
    'GET',
    `${sourcePath(scope, id)}/channels/${seg(channelId)}`,
    undefined,
    signal,
  )

export const updateLineupChannel = (
  scope: Scope,
  id: string,
  channelId: string,
  patch: ChannelPatch,
) => request<LineupChannel>('PATCH', `${sourcePath(scope, id)}/channels/${seg(channelId)}`, patch)

/** Puts a channel before another of its category, or last with null. */
export const moveLineupChannel = (
  scope: Scope,
  id: string,
  channelId: string,
  before: string | null,
) => request<void>('POST', `${sourcePath(scope, id)}/channels/${seg(channelId)}/move`, { before })

/** Turns the selected channels on or off; a dry run only counts them. */
export const bulkLineupChannels = (
  scope: Scope,
  id: string,
  selector: BulkSelector,
  enabled: boolean,
  dryRun: boolean,
) =>
  request<{ matched: number; changed: number }>('POST', `${sourcePath(scope, id)}/channels/bulk`, {
    ...selector,
    enabled,
    dryRun,
  })

/** Sets the order and enabled flags of a channel's streams; every stream once. */
export const saveChannelStreams = (
  scope: Scope,
  id: string,
  channelId: string,
  streams: { id: string; enabled: boolean }[],
) =>
  request<LineupChannel>('PUT', `${sourcePath(scope, id)}/channels/${seg(channelId)}/streams`, {
    streams,
  })

export const addChannelStream = (
  scope: Scope,
  id: string,
  channelId: string,
  stream: { url: string; label: string },
) =>
  request<LineupChannel>(
    'POST',
    `${sourcePath(scope, id)}/channels/${seg(channelId)}/streams`,
    stream,
  )

/** Forgets how a channel's streams answered: its next start tries them all. */
export const retryChannelStreams = (scope: Scope, id: string, channelId: string) =>
  request<LineupChannel>(
    'POST',
    `${sourcePath(scope, id)}/channels/${seg(channelId)}/streams/retry`,
  )

export const deleteChannelStream = (
  scope: Scope,
  id: string,
  channelId: string,
  streamId: string,
) =>
  request<LineupChannel>(
    'DELETE',
    `${sourcePath(scope, id)}/channels/${seg(channelId)}/streams/${seg(streamId)}`,
  )

const guidesPath = (scope: Scope) => `${scopePath(scope)}/catalog-guides`

export const fetchCatalogGuides = (scope: Scope, target: CatalogTarget, signal?: AbortSignal) =>
  request<CatalogGuides>('GET', `${guidesPath(scope)}?${params(target)}`, undefined, signal)

/** Replaces a catalog's guides, in order; a kept guide is sent as its id, its address being redacted. */
export const saveCatalogGuides = (
  scope: Scope,
  target: CatalogTarget,
  urls: (string | { id: string })[],
) => request<CatalogGuides>('PUT', guidesPath(scope), { ...target, urls })

export const refreshCatalogGuides = (scope: Scope, target: CatalogTarget) =>
  request<CatalogGuides>('POST', `${guidesPath(scope)}/refresh`, target)

/** Maps the channels without a mapping, or with `remap` every channel, manual ones dropped. */
export const automapCatalog = (scope: Scope, target: CatalogTarget, mode: 'unmapped' | 'remap') =>
  request<{ channels: number; mapped: number; changed: number }>(
    'POST',
    `${guidesPath(scope)}/automap`,
    { ...target, mode },
  )

export const fetchGuideChannels = (
  scope: Scope,
  target: CatalogTarget,
  query: { q?: string; guide?: string },
  offset: number,
  limit: number,
  signal?: AbortSignal,
) =>
  request<Page<GuideChannel>>(
    'GET',
    `${guidesPath(scope)}/channels?${params({ ...target, ...query, offset, limit })}`,
    undefined,
    signal,
  )

export const fetchMappings = (
  scope: Scope,
  target: CatalogTarget,
  query: { state: MappingState; q?: string },
  offset: number,
  limit: number,
  signal?: AbortSignal,
) =>
  request<Page<MappingItem>>(
    'GET',
    `${guidesPath(scope)}/mappings?${params({ ...target, ...query, offset, limit })}`,
    undefined,
    signal,
  )

/** Maps a channel by hand; both ids null pins "no guide". */
export const setMapping = (
  scope: Scope,
  target: CatalogTarget,
  channelId: string,
  guide: { guideId: string; guideChannelId: string } | null,
) =>
  request<MappingItem>('PUT', `${guidesPath(scope)}/mappings`, {
    ...target,
    channelId,
    guideId: guide?.guideId ?? null,
    guideChannelId: guide?.guideChannelId ?? null,
  })

/** Drops a manual mapping: the channel is mapped automatically again. */
export const clearMapping = (scope: Scope, target: CatalogTarget, channelId: string) =>
  request<MappingItem>(
    'DELETE',
    `${guidesPath(scope)}/mappings?${params({ ...target, channelId })}`,
  )

export const fetchAddonPreferences = (signal?: AbortSignal) =>
  request<AddonPreferences>('GET', '/account/addon-preferences', undefined, signal)

export const saveAddonPreferences = (preferences: Pick<AddonPreferences, 'useSharedAddons'>) =>
  request<AddonPreferences>('PUT', '/account/addon-preferences', preferences)

export const fetchApiKeys = (signal?: AbortSignal) =>
  request<ApiKey[]>('GET', '/api-keys', undefined, signal)

export const createApiKey = (app: string) => request<NewApiKey>('POST', '/api-keys', { app })

export const deleteApiKey = (id: string) => request<void>('DELETE', `/api-keys/${seg(id)}`)

/** Which entries of the activity log to list: `limit` (1 to 100) from `start`, newest first. */
export type ActivityQuery = {
  limit: number
  start?: number
  /** Entry types kept; empty keeps all. */
  types?: readonly string[]
  /** Severities kept; empty keeps all. */
  severities?: readonly ActivityEntry['severity'][]
}

export function fetchActivity(query: ActivityQuery, signal?: AbortSignal) {
  const params = new URLSearchParams({ limit: String(query.limit) })
  if (query.start) params.set('start', String(query.start))
  if (query.types?.length) params.set('type', query.types.join(','))
  if (query.severities?.length) params.set('severity', query.severities.join(','))
  return request<ActivityPage>('GET', `/activity?${params}`, undefined, signal)
}

/** A video's size, codecs and bitrate (bits per second); 0 or empty when not known. */
export type StreamInfo = {
  width: number
  height: number
  videoCodec: string
  audioCodec: string
  bitrate: number
}

/** How a playback reaches its app; `stream` is HLS whose encoding is idle, so details are unknown. */
export type Delivery = 'directPlay' | 'remux' | 'conversion' | 'stream'

/** A device playing, as the dashboard shows it. */
export type LiveSession = {
  id: string
  user: { id: string; name: string; imageTag: string | null; qualityGroup: number }
  device: { name: string; app: string; appVersion: string; address: string }
  /** Whether the app takes commands: stopping and messages. */
  controllable: boolean
  item: {
    id: string
    kind: 'movie' | 'episode' | 'channel' | 'recording' | string
    name: string
    seriesName: string
    season: number
    episode: number
    year: number
    /** Seconds, 0 when unknown. */
    runtime: number
    posterId: string
  } | null
  /** Seconds. */
  position: number
  paused: boolean
  startedAt: string
  playMethod: string
  delivery: Delivery
  /** Jellyfin's TranscodeReasons. */
  reasons: string[]
  source: StreamInfo | null
  sent: StreamInfo | null
  video: {
    encoder: string
    /** GPU method (cuda, vaapi); empty for the CPU. */
    hardware: string
    width: number
    height: number
    bitrate: number
    toneMap: boolean
    burnSubtitles: boolean
  } | null
  audio: { codec: string; channels: number; bitrate: number } | null
}

export const fetchLiveSessions = (signal?: AbortSignal) =>
  request<LiveSession[]>('GET', '/sessions', undefined, signal)

export const stopLiveSession = (id: string) => request<void>('POST', `/sessions/${seg(id)}/stop`)

/** Shows a message on the session's app, for `timeout` seconds or until dismissed when 0. */
export const messageLiveSession = (id: string, message: { text: string; timeout: number }) =>
  request<void>('POST', `/sessions/${seg(id)}/message`, message)

export type TaskResult = {
  start: string
  end: string
  status: 'Completed' | 'Failed' | 'Cancelled'
  error: string
}

/**
 * A scheduled task; `interval` is in seconds, 0 for a task run by hand only or daily, and `daily`
 * the hour of the server's time zone a daily task runs at.
 */
export type Task = {
  id: string
  key: string
  name: string
  description: string
  category: string
  interval: number
  daily: number | null
  state: 'Idle' | 'Running' | 'Cancelling'
  last: TaskResult | null
  next: string | null
}

export const fetchTasks = (language: Language, signal?: AbortSignal) =>
  request<Task[]>('GET', `/tasks?language=${seg(language)}`, undefined, signal)

export const runTask = (id: string) => request<void>('POST', `/tasks/${seg(id)}/run`)

export const stopTask = (id: string) => request<void>('POST', `/tasks/${seg(id)}/stop`)

/** A Live TV recording scheduled or under way; it runs from `from` to `until`, padding included. */
export type Timer = {
  id: string
  name: string
  channel: string
  userId: string
  userName: string
  start: string
  end: string
  from: string
  until: string
  status: 'New' | 'InProgress' | 'Error' | string
  series: boolean
}

export const fetchTimers = (signal?: AbortSignal) =>
  request<{ available: boolean; timers: Timer[] }>('GET', '/timers', undefined, signal)

/** How the requests made to one of the server's addons went since the server started. */
/** The user whose own addon, source or guide a dashboard row is; null for the server's. */
export type Owner = { id: string; name: string } | null

export type AddonHealth = {
  id: string
  owner: Owner
  name: string
  enabled: boolean
  refreshedAt: string
  lastSuccessAt: string | null
  lastFailureAt: string | null
  /** Code of the last failure; empty after a success. */
  failure: string
  /** Milliseconds, of the last answer. */
  responseTime: number
  requests: number
  failures: number
}

export type Health = {
  checkedAt: string
  process: {
    version: string
    goVersion: string
    startedAt: string
    memory: number
    heap: number
    goroutines: number
  }
  database: { reachable: boolean; size: number | null }
  cache: { used: number; limit: number; sources: number } | null
  disks: {
    folder: 'cache' | 'recordings' | 'backups'
    path: string
    free: number
    used: number
    mount: string
    /** Whether the disk is short of room, which Health shows as a problem. */
    low: boolean
  }[]
  transcoder: {
    hardware: { method: string; device: string; encoders: string[]; toneMapping: boolean } | null
    encoders: string[]
    conversions: number
    limit: number
    remuxes: number
    maxHeight: number
    enabled: boolean
  } | null
  thumbnails: {
    enabled: boolean
    waiting: number
    queueLength: number
    working: boolean
    pausedHosts: { host: string; until: string }[]
  } | null
  addons: AddonHealth[]
  /** How the stored keys and tokens stand; null when the server could not read them. */
  secrets: {
    /** Whether POLYFIN_SECRET_KEY is set, which encrypts them. */
    encrypted: boolean
    /** How many are stored unencrypted. */
    plaintext: number
    /** Those POLYFIN_SECRET_KEY cannot decrypt. */
    unreadable: UnreadableSecret[]
  } | null
  /** How the database backups go; null when they are off. */
  backup: Backup | null
}

/**
 * A stored secret POLYFIN_SECRET_KEY cannot decrypt: a server setting, a user's connection, or a
 * notification target, by its name, of a user or of the server (`user` null).
 */
export type UnreadableSecret = {
  setting: ServerSecretName | null
  service: TrackingServiceName | null
  target: string | null
  user: string | null
}

/**
 * Something Health shows as needing attention, as the server finds it: `key` names it while it
 * lasts, `code` tells what it is, with its details; `to` is the page that describes it, with its
 * anchor. `transient` ones come and go with the load. An addon, source or guide has an `owner`
 * when it is a user's own.
 */
export type HealthProblem = {
  key: string
  tone: 'error' | 'warning'
  to: string
  transient: boolean
} & (
  | { code: 'database' }
  | { code: 'disk'; folder: 'cache' | 'recordings' | 'backups'; free: number }
  | { code: 'addon'; name: string; owner?: NonNullable<Owner>; failure: string }
  | { code: 'conversions_full' }
  | { code: 'thumbnails_paused'; host: string }
  | { code: 'secrets_unreadable'; secrets: UnreadableSecret[] }
  | { code: 'secrets_plaintext' }
  | { code: 'backup_failed' }
  | { code: 'backup_stale' }
  | { code: 'iptv' | 'guide'; name: string; owner?: NonNullable<Owner> }
  | { code: 'folder'; name: string }
  | { code: 'task'; task: string }
)

/** The problems Health shows, errors first, with the tasks named in `language`. */
export const fetchHealthProblems = (language: Language, signal?: AbortSignal) =>
  request<{ problems: HealthProblem[] }>(
    'GET',
    `/health/problems?language=${seg(language)}`,
    undefined,
    signal,
  )

/** How the database backups go; `folder` is empty when they are off. */
export type Backup = {
  folder: string
  /** When the next backup is made. */
  next: string | null
  /** When the last run started; `error` is why it failed, empty after a success. */
  ranAt: string | null
  error: string
  /** When the last backup made started, its file in the folder and its size in bytes. */
  madeAt: string | null
  file: string
  size: number
  /** Whether the last run failed or the last backup is older than two days. */
  problem: boolean
}

export const fetchBackup = (signal?: AbortSignal) =>
  request<Backup>('GET', '/backup', undefined, signal)

/** The addons, IPTV sources and XMLTV guides of the server, then of every user, with their owner. */
export type Sources = {
  addons: (Addon & { owner: Owner })[]
  /** Live TV catalogs that have a guide address. */
  guides: (Library & { owner: Owner })[]
}

export const fetchSources = (signal?: AbortSignal) =>
  request<Sources>('GET', '/sources', undefined, signal)

export const fetchHealth = (signal?: AbortSignal) =>
  request<Health>('GET', '/health', undefined, signal)

/** Asks an addon for its manifest once; allowed once a minute per addon. */
export const checkAddon = (id: string) =>
  request<AddonHealth>('POST', `/health/addons/${seg(id)}/check`)

/** The log lines written after the first `after`, and the `after` of the next call. */
export const fetchLogLines = (after: number, limit: number, signal?: AbortSignal) =>
  request<{ lines: string[]; next: number }>(
    'GET',
    `/logs?after=${after}&limit=${limit}`,
    undefined,
    signal,
  )

export const logDownloadUrl = `${apiBase}/logs/download`

/** A POLYFIN_ environment variable; `value` is empty when `hidden`, and the default when not `set`. */
export type Variable = {
  name: string
  value: string
  set: boolean
  hidden: boolean
  known: boolean
}

export const fetchVariables = (signal?: AbortSignal) =>
  request<Variable[]>('GET', '/variables', undefined, signal)

export const queryKeys = {
  status: ['status'] as const,
  session: ['session'] as const,
  myDevices: ['account', 'devices'] as const,
  tracking: ['account', 'tracking'] as const,
  users: ['users'] as const,
  jellyfinImport: ['jellyfin-import'] as const,
  userDevices: (id: string) => ['users', id, 'devices'] as const,
  settings: ['settings'] as const,
  parentalRatings: ['parental-ratings'] as const,
  /** Prefix of every scope: invalidating it refreshes all addons and libraries. */
  scopes: ['scopes'] as const,
  /** Prefix of everything a scope owns: invalidating it refreshes its addons and libraries. */
  scope: (scope: Scope) => ['scopes', scope] as const,
  addons: (scope: Scope) => ['scopes', scope, 'addons'] as const,
  libraries: (scope: Scope) => ['scopes', scope, 'libraries'] as const,
  addonPreferences: ['account', 'addon-preferences'] as const,
  userContentChoices: ['user-content-choices'] as const,
  apiKeys: ['api-keys'] as const,
  activity: (query: ActivityQuery) => ['activity', query] as const,
  /** Everything a source's line-up shows: invalidated after any change to it. */
  lineup: (scope: Scope, id: string) => ['lineup', scope, id] as const,
  /** Everything a Live TV catalog's guides show. */
  catalogGuides: (scope: Scope, target: CatalogTarget) =>
    ['catalog-guides', scope, target.addonId, target.catalogType, target.catalogId] as const,
  liveSessions: ['live-sessions'] as const,
  tasks: (language: Language) => ['tasks', language] as const,
  timers: ['timers'] as const,
  health: ['health'] as const,
  /** Under the health's key: refreshing the health refreshes them too. */
  healthProblems: (language: Language) => ['health', 'problems', language] as const,
  backup: ['backup'] as const,
  sources: ['sources'] as const,
  /** The unmatched files of a local folder. */
  unmatched: (id: string) => ['unmatched', id] as const,
  variables: ['variables'] as const,
  notifications: (scope: NotificationScope) => ['notifications', scope] as const,
}

/** Any 401 means the session is gone: drop back to the sign-in page. */
function handleUnauthenticated(error: unknown) {
  if (error instanceof ApiError && error.status === 401) {
    queryClient.setQueryData(queryKeys.session, null)
  }
}

export const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({ onError: handleUnauthenticated }),
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      // Sign-in and setup failures are credential errors, not an expired session.
      if (mutation.options.meta?.public === true) return
      handleUnauthenticated(error)
    },
  }),
  defaultOptions: {
    queries: {
      // Fail fast so an unreachable server is reported within seconds, not after long backoff.
      // Client errors (401, 403, 404) are answers, not transient failures.
      retry: (failureCount, error) =>
        !(error instanceof ApiError && error.status < 500) && failureCount < 1,
    },
  },
})
