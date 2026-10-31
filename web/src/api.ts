import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { Language } from '@/i18n'
import {
  arrivedAsAnotherUser,
  takeWebClientToken,
  webClientCredentialsKey,
} from '@/webClientSession'

export type Status = {
  name: string
  version: string
  serverId: string
  database: 'ready' | 'unavailable'
  setupRequired: boolean
  /** Whether the server serves the web player at /web/; absent from servers without it. */
  webClient?: boolean
  /** Where the server runs, which names its local folders; absent from servers without it. */
  system?: ServerSystem
  /**
   * The identity provider the sign-in pages offer: the name its button shows, and the address, at
   * the server's public address, that starts its sign-in, ?app=admin added. Null without one.
   */
  provider?: { name: string; start: string } | null
}

/** Where the server runs: in a container, as the Docker image does, else on its computer's system. */
export type ServerSystem = 'container' | 'linux' | 'macos' | 'windows'

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
  /**
   * Whether the user may give a movie or an episode to someone without an account, through a share
   * link, while the server allows share links (Settings.shareLinks).
   */
  canShare: boolean
  /**
   * Whether the user is offered the torrent streams addons list, while the server plays them
   * (Settings.torrentStreams). Off for every user, administrators included, until given.
   */
  torrentStreams: boolean
  /**
   * Whether the user may sign in and use the server from outside the local network, Jellyfin's
   * EnableRemoteAccess; without it, only from the local network, as Known proxies tells it.
   */
  remoteAccess: boolean
  /** Identifies the user's profile picture, served at /UserImage; null without one. */
  imageTag: string | null
  /** The tallest video the user is offered, in lines (480 to 2160); 0 for Original, no limit. */
  qualityGroup: QualityGroup
  /** The date the account ends, YYYY-MM-DD in the server's time zone; null for never. */
  endsOn: string | null
  /** Whether the end date has come: the account is refused as a disabled one is. */
  ended: boolean
  /** The group the user belongs to; null for none. Administrators never belong to one. */
  groupId: string | null
  groupName: string | null
  /**
   * Whether the user follows their group: its values apply and their page shows them read only.
   * False once detached (Own settings); true when the user has no group.
   */
  followGroup: boolean
  /** The language the user's titles are described in; '' for the server's. */
  metadataLanguage: string
  /** The user's web player theme; '' for the server's. */
  webTheme: UserWebTheme
  /** Whether the user is kept from changing their web player theme under My account. */
  webThemeLocked: boolean
  /** The regional variant of audio the user prefers, one of audioRegions; '' for none. */
  audioRegion: string
  /** The regional variants offered, BCP 47 tags such as 'fr-CA'. */
  audioRegions: string[]
  /** Whether the user signs in with a code from an authenticator app after their password. */
  twoStep: boolean
  /** Whether the user signs in through the identity provider only, their password refused. */
  providerOnly: boolean
}

/** The fields of a user a group holds, in its sections: Access, Playback and access, and the rest. */
export type GroupLimits = Pick<
  User,
  | 'transcoding'
  | 'downloads'
  | 'personalAddons'
  | 'collectionManagement'
  | 'subtitleManagement'
  | 'canShare'
  | 'torrentStreams'
  | 'remoteAccess'
  | 'maxPlaybacks'
  | 'maxBitrate'
  | 'qualityGroup'
  | 'syncPlay'
  | 'liveTv'
  | 'remoteControl'
  | 'liveTvManagement'
  | 'parentalControl'
  | 'hiddenLibraries'
  | 'blockedGenres'
  | 'accessSchedules'
>

/** A group as Users › Groups lists it. */
export type GroupSummary = { id: string; name: string; memberCount: number }

/** A group, with the values its following members take. */
export type Group = GroupSummary & GroupLimits

export type GroupPatch = Partial<GroupLimits & { name: string }>

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
export type SegmentSource = 'theintrodb' | 'introdb' | 'publicmetadb' | 'skipdb'

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

/** How Polyfin serves HTTPS (see Settings.httpsMode). */
export type HttpsMode = 'off' | 'files' | 'letsencrypt'

/** The challenges Let's Encrypt checks host names with (see Settings.acmeChallenge). */
export type AcmeChallenge = 'http-01' | 'tls-alpn-01'

/** What a saved WireGuard configuration says, without its keys (see Settings.torrentWireGuardSaved). */
export type WireGuardSaved = {
  /** The server, host:port. */
  endpoint: string
  /** The tunnel's addresses, with their prefix length. */
  addresses: string[]
  /** The DNS servers tracker names are looked up by, through the tunnel; empty for none. */
  dns: string[]
}

/**
 * Why a WireGuard configuration was refused (invalid_wireguard_config): the line, from 1, 0 for
 * the configuration as a whole, the key or section it is about, and the reason.
 */
export type WireGuardConfigProblem = {
  line: number
  key: string
  reason:
    | 'too_long'
    | 'syntax'
    | 'outside'
    | 'unknown'
    | 'repeated'
    | 'missing'
    | 'invalid'
    | 'no_ipv4'
    | 'not_all'
}

export type Settings = {
  serverName: string
  quickConnectEnabled: boolean
  legacyAuthorization: boolean
  /** Asks GitHub once a day for Polyfin's latest release, to tell administrators of a new version. */
  updateCheck: boolean
  /** Answers the discovery messages Jellyfin apps broadcast on the local network (UDP port 7359). */
  localDiscovery: boolean
  /**
   * Address Polyfin is reached at on the local network, such as http://192.168.1.10:8096, which
   * discovery announces; empty announces the address of the interface that reaches the app.
   */
  localAddress: string
  /** Shares the libraries with the DLNA devices of the local network (SSDP on UDP port 1900). */
  dlna: boolean
  /** The name DLNA devices list Polyfin under; empty, the server's name. */
  dlnaName: string
  /** The user whose libraries a DLNA device seen for the first time shows; null for no one. */
  dlnaNewDevicesUser: string | null
  /** Marks played what DLNA devices read past the played percentage of. */
  dlnaMarkPlayed: boolean
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
  /** Whether a PublicMetaDB key is saved; it adds PublicMetaDB as a source of skip markers. */
  publicMetaDbKeySet: boolean
  /** Sent only to change the key: a new key, checked before it is saved, or "" to remove it. */
  publicMetaDbKey?: string
  /** Whether a TheIntroDB key is saved; optional, it raises TheIntroDB's daily limit. */
  theIntroDbKeySet: boolean
  /** Sent only to change the key: a new key, checked before it is saved, or "" to remove it. */
  theIntroDbKey?: string
  /** The order of preference of the skip marker sources, all four. */
  segmentOrder: SegmentSource[]
  /** The skip marker sources never asked, wherever they are in the order. */
  segmentSourcesOff: SegmentSource[]
  /** Whether local episodes without an intro or credits have them looked for in their season's audio; needs skipButtons. */
  audioIntros: boolean
  /** Whether apps get similar titles, from the addons' catalogs. */
  similarTitles: boolean
  /** Whether addons' collections without a poster, and smart collections, get an image Polyfin draws. */
  generateImages: boolean
  /** Whether the server's MDBList key for ratings is saved; titles then show MDBList's ratings. */
  mdblistRatingsKeySet: boolean
  /** Sent only to change the key: a new key, checked with MDBList before it is saved, or "" to remove it. */
  mdblistRatingsKey?: string
  /** The score CriticRating carries: the critics' score or the audience score. */
  criticRatingShows: 'critics' | 'audience'
  /** Whether the server's TMDB key is saved; Edit images then searches TMDB's images too. */
  tmdbKeySet: boolean
  /** Sent only to change the key: a new key, checked with TMDB before it is saved, or "" to remove it. */
  tmdbKey?: string
  /** Whether songs get lyrics from LRCLIB, which is sent their artist, title, album and length. */
  lyrics: boolean
  /** Whether apps get the gains of songs and albums known, which even the volume between songs. */
  evenVolume: boolean
  /** Which songs without a gain have their loudness measured: none, local songs, or addons' songs too. */
  measureLoudness: 'off' | 'local' | 'all'
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
  /** Whether users may import their own watch history from another server, under My account. */
  serverImports: boolean
  /** Whether users with the permission may share a movie or an episode through a link that expires. */
  shareLinks: boolean
  /** Wrong passwords in a row that block an account for 15 minutes; 0 never blocks. */
  loginAttempts: number
  /**
   * Whether administrators must use two-step sign-in: one without it sets it up right after their
   * password, and their password alone signs in to no Jellyfin app.
   */
  twoStepAdministrators: boolean
  /** Days unused after which a Jellyfin app is signed out; 0 never signs it out. */
  inactiveDeviceDays: number
  /**
   * Addresses and networks of the reverse proxies whose forwarded headers tell a request's client,
   * such as 172.18.0.0/16; empty trusts no forwarded header.
   */
  knownProxies: string[]
  /**
   * How Polyfin serves HTTPS itself, next to HTTP: not at all, with certificate files, or with a
   * certificate it obtains from Let's Encrypt.
   */
  httpsMode: HttpsMode
  /** The TCP port HTTPS listens on; HTTP keeps POLYFIN_LISTEN. 8920 by default, as Jellyfin. */
  httpsPort: number
  /** Sends the requests of clients on the internet that come over HTTP to HTTPS (308). */
  httpsRedirect: boolean
  /** The certificate file, with its chain, and its RSA or ECDSA key file: PEM paths in the container. */
  httpsCertificate: string
  httpsPrivateKey: string
  /** The public host names the certificate of Let's Encrypt is for. */
  acmeHosts: string[]
  /** The Let's Encrypt account's contact; empty for none. */
  acmeEmail: string
  /** Whether the administrator agreed to Let's Encrypt's terms of service. */
  acmeTerms: boolean
  /** How Let's Encrypt checks the host names: on the HTTP port, or on the HTTPS port. */
  acmeChallenge: AcmeChallenge
  /** Logs in detail (debug level), to diagnose a problem. */
  detailedLog: boolean
  /** Seconds one analysis of a version or a channel's stream may take before it is given up. */
  analysisTimeout: number
  /** Versions analyzed at most when an app plays a title without choosing a version. */
  versionAttempts: number
  /** Prefers the first version an app plays without conversion over the first that plays at all. */
  preferDirectPlay: boolean
  /**
   * Up to four criteria ordering a title's versions, each grouping them and the next sorting within
   * them; none keeps the addons' order.
   */
  versionOrder: VersionCriterion[]
  /** The order of the video codecs for the codec criterion, every codec once. */
  versionCodecOrder: VersionCodec[]
  /**
   * The IDs of the server's sources the source criterion puts first, in order, some of
   * `versionSources`; the others come after them, in the order of Content › Sources.
   */
  versionSourceOrder: string[]
  /** The server's sources listing versions of movies and episodes, folders included (read-only). */
  versionSources: VersionSource[]
  /** Hides the versions known to be larger, in GB; 0 hides none. */
  hideVersionsLargerGb: number
  /** Hides the versions known to be in these video codecs. */
  hiddenVersionCodecs: VersionCodec[]
  /** Hides the versions whose labels hold one of these words, in any letter case. */
  hiddenVersionWords: string[]
  /**
   * Hides the versions whose labels tag them as low-quality releases of these kinds, by whole
   * words after the title's own.
   */
  hiddenReleases: ReleaseKind[]
  /** Ends version names with how the app asking plays each, as its last playback decides it. */
  nameHowVersionsPlay: boolean
  /** Times the addons' subtitles to the version played, against when it has speech. */
  syncAddonSubtitles: boolean
  /** Playbacks whose video the server converts at once, 0 for no limit. */
  maxConversions: number
  /** Height converted video is scaled down to at most, 0 to keep the original's. */
  maxConversionHeight: number
  /** Streams a title as one file, copied or converted within the user's quality, to apps saving it offline. */
  convertedDownloads: boolean
  /**
   * Reads picture subtitles (PGS, VobSub) into text tracks with Tesseract: never, when an app that
   * cannot draw them plays a version, or also after the scans of local folders.
   */
  subtitleOcr: SubtitleOcr
  /** Picture subtitle tracks read at once. */
  subtitleOcrTracks: number
  /**
   * The audio and subtitle languages of the users who chose none, ISO 639-2/B codes as their own
   * preferences take them, or '' for none.
   */
  defaultAudioLanguage: string
  defaultSubtitleLanguage: string
  /** The languages they offer, those of users' own preferences, by English name (read-only). */
  trackLanguages: TrackLanguage[]
  /** Encoder speed against quality: auto (Polyfin's choice), then veryslow to ultrafast. */
  encoderPreset: EncoderPreset
  /** Quality factor of H.264 and HEVC conversions (CRF, CQ or QVBR's), 0 aiming for the bitrate alone. */
  h264Quality: number
  hevcQuality: number
  /** Converts to HEVC for the apps that list it before H.264. */
  allowHevcEncoding: boolean
  /** Whether converted video offers lower qualities apps step down to: off, outside the local network, or always. */
  adaptiveQuality: AdaptiveQuality
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
  /** Tone maps HDR on a GPU that can (NVIDIA, Intel through VAAPI or QSV, VideoToolbox); off, the processor does. */
  gpuToneMapping: boolean
  /** Most height of HDR the processor tone maps: 720 to 2160, 0 for Automatic (measured at startup). */
  processorToneMappingHeight: number
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
  /** Converts video apart from its audio, and keeps it when an app changes the audio track. */
  keepConvertedVideo: boolean
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
  /** Whether finished recordings are read for their commercial breaks, served as segments with skipButtons. */
  recordingCommercials: boolean
  /** Whether finished recordings are converted to HEVC. */
  convertRecordings: boolean
  /** Hours, in the server's time zone, between which recordings are converted. */
  convertRecordingsFrom: number
  convertRecordingsUntil: number
  /** Whether recordings are converted on the processor when no graphics card encodes HEVC. */
  convertRecordingsOnProcessor: boolean
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
  /** Hours after its last check a watchlist title is checked again for versions; 0 for never. */
  watchlistCheckHours: number
  /** Whether the local folders, in the container and network shares, are watched for changes. */
  watchLocalFolders: boolean
  /**
   * The web player's look: the bundled LumaaGlaass theme, jellyfin-web's own look, or the
   * Custom texts below.
   */
  webTheme: WebTheme
  /**
   * Custom CSS jellyfin-web applies to every page while the theme is Custom, unless a user turns
   * it off (Jellyfin's branding); kept while another theme is chosen.
   */
  customCss: string
  /**
   * Custom script Polyfin adds to jellyfin-web's page while the theme is Custom; it runs in every
   * user's browser. Kept while another theme is chosen.
   */
  customJs: string
  /** The CSS and script Polyfin serves for the LumaaGlaass theme (read-only). */
  lumaaGlaass: { css: string; js: string }
  /** Text, Markdown or HTML jellyfin-web shows under its sign-in form (Jellyfin's branding). */
  loginDisclaimer: string
  /** Whether the web player adds the versions that come to a title's page while it is open. */
  addVersionsToOpenPage: boolean
  /** Whether the web player shows titles' ratings on their posters, as the pointer or focus comes. */
  posterRatings: boolean
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
  /** AniList app users connect through; both its client ID and secret are needed. */
  aniListClientId: string
  /** Whether the AniList client secret is saved. */
  aniListClientSecretSet: boolean
  /** Sent only to change the secret: a new secret, or "" to remove it. */
  aniListClientSecret?: string
  /** MyAnimeList app users connect through; both its client ID and secret are needed. */
  myAnimeListClientId: string
  /** Whether the MyAnimeList client secret is saved. */
  myAnimeListClientSecretSet: boolean
  /** Sent only to change the secret: a new secret, or "" to remove it. */
  myAnimeListClientSecret?: string
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
  /** Whether a passphrase encrypts the backups. */
  backupPassphraseSet: boolean
  /** Sent only to change the backup passphrase: a new one, or "" to stop encrypting. */
  backupPassphrase?: string
  /** Hour of the server's time zone every collection of the server's collection libraries is read at each day, 0 to 23; -1 never. */
  collectionReadHour: number
  /** Apps, by the name they sign in with, told an addon's collection library holds collections (boxsets) rather than mixed content; none by default. */
  collectionLibraryApps: string[]
  /** Describes the versions not analyzed yet from RemuxDB, which the titles' IMDb ids are sent to. */
  remuxDb: boolean
  /** Address of the RemuxDB server asked: an http or https URL, without a trailing slash. */
  remuxDbUrl: string
  /** Address people open Polyfin at, which links in notifications start with; empty for no link. */
  publicAddress: string
  /** Host of the SMTP server email notifications go through; empty, email targets cannot be added. */
  smtpHost: string
  smtpPort: number
  /** How the SMTP connection is encrypted: STARTTLS, TLS from the start, or not at all. */
  smtpSecurity: 'starttls' | 'tls' | 'none'
  /** User the SMTP server is signed in with; empty, it is not. */
  smtpUser: string
  /** Whether the SMTP password is saved. */
  smtpPasswordSet: boolean
  /** Sent only to change the SMTP password: a new one, or "" to remove it. */
  smtpPassword?: string
  /** Address and name email notifications come from; empty name, the server name. */
  smtpFrom: string
  smtpFromName: string
  /** Whether the videos played are kept, for the statistics, and for how many days. */
  playbackHistory: boolean
  playbackHistoryDays: number
  /**
   * When the weekly summary is sent, in the server's time zone: the day of the week, 0 for Sunday
   * to 6 for Saturday, and the hour, 0 to 23.
   */
  weeklySummaryDay: number
  weeklySummaryHour: number
  /**
   * How often connected accounts are imported again, in minutes: the app connections and the
   * watch histories of tracking services. One of its bounds' choices.
   */
  importEveryMinutes: number
  /** Whether /metrics answers Prometheus. */
  metrics: boolean
  /** Whether Polyfin made the metrics token (read-only): never sent, revealed on demand. */
  metricsTokenSet: boolean
  /**
   * Whether an administrator confirmed that Polyfin is started again when it exits, by its
   * container's restart policy or its service manager: Restart then warns no more.
   */
  restartConfirmed: boolean
  /** The update API of an update tool run beside Polyfin, which Update now asks; '' for none. */
  updateToolUrl: string
  /** Whether the update tool's token is saved (read-only): never sent, revealed on demand. */
  updateToolTokenSet: boolean
  /** The update tool's token, sent only to change it: '' removes it. */
  updateToolToken?: string
  /**
   * Whether torrent streams play, over one route alone: the network interface every torrent
   * socket is bound to (Linux only), a SOCKS5 proxy, host:port, or a WireGuard tunnel Polyfin runs;
   * '' for none.
   */
  torrentStreams: boolean
  torrentInterface: string
  torrentProxy: string
  torrentProxyUser: string
  /** Whether the proxy's password is saved. */
  torrentProxyPasswordSet: boolean
  /** Sent only to change the proxy's password: a new one, or "" to remove it. */
  torrentProxyPassword?: string
  /** What the saved WireGuard configuration says, without its keys (read-only); null for none. */
  torrentWireGuardSaved: WireGuardSaved | null
  /** Sent only to change the WireGuard configuration: a provider's .conf, or "" to remove it. */
  torrentWireGuard?: string
  /** Download limit in Mbps, 0 for none; upload limit, 0 for no sharing. */
  torrentDownloadMbps: number
  torrentUploadMbps: number
  /** How many torrents may run at once, server-wide, 0 for no limit. */
  torrentsAtOnce: number
  /** Announce URLs of trackers asked for every torrent besides those its addon names. */
  torrentTrackers: string[]
  /**
   * Folders of the segments of conversions, of the files being read, and of the cache; empty uses
   * cacheFolderDefault, the cache in the data folder. Each applies at the next start.
   */
  conversionFolder: string
  readingFolder: string
  cacheFolder: string
  /** The cache in the data folder, where the three folders above are when empty (read-only). */
  cacheFolderDefault: string
  /** Whether this system binds sockets to a network interface, Linux alone (read-only). */
  interfaceRoutes: boolean
  /** Whether Silo's apps may sign in and play through Silo's API under /api/v2. */
  siloApps: boolean
  /**
   * The OpenID Connect provider people may sign in with: its issuer, the name its button shows
   * (empty for the issuer's host), and the server's client at it. The client secret is never sent
   * back: providerClientSecretSet tells whether one is saved; providerClientSecret, sent only to
   * replace it, empty to remove it. providerUserClaim names the claim compared with account names,
   * the verified email after it; providerCreateAccounts creates an account for someone none
   * matches; providerAdminClaim and providerAdminValue, both set, make administrators.
   * providerRedirectUri is the address to register at the provider, empty without a public
   * address (read-only).
   */
  providerIssuer: string
  providerName: string
  providerClientId: string
  providerClientSecretSet: boolean
  providerClientSecret?: string
  providerUserClaim: string
  providerCreateAccounts: boolean
  providerAdminClaim: string
  providerAdminValue: string
  providerRedirectUri: string
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
/** Each system offers its own GPU methods (the settings' bounds list them). */
export type HardwareAcceleration =
  'auto' | 'nvenc' | 'vaapi' | 'videotoolbox' | 'qsv' | 'amf' | 'none'
export type HardwareDecodingCodec =
  'h264' | 'hevc' | 'hevc_10bit' | 'vp9' | 'av1' | 'mpeg2video' | 'vc1'
export type VersionCriterion =
  | 'language'
  | 'resolution-high'
  | 'resolution-low'
  | 'size-large'
  | 'size-small'
  | 'codec'
  | 'hdr-first'
  | 'hdr-last'
  | 'source'
export type VersionCodec = 'av1' | 'hevc' | 'vp9' | 'h264' | 'vc1' | 'mpeg4' | 'mpeg2video'
/** Kinds of low-quality releases: camera copies, telesyncs, telecines, screeners, early copies. */
export type ReleaseKind = 'cam' | 'telesync' | 'telecine' | 'screener' | 'early'
/** A source of the server listing versions of movies and episodes, in Content › Sources order. */
export type VersionSource = { id: string; name: string }
/** A language of the default track languages: its ISO 639-2/B code and its English name. */
export type TrackLanguage = { code: string; name: string }
export type ToneMappingAlgorithm =
  'auto' | 'bt2390' | 'hable' | 'reinhard' | 'mobius' | 'clip' | 'linear'
export type DeinterlaceMethod = 'yadif' | 'bwdif'
export type AdaptiveQuality = 'off' | 'remote' | 'always'
export type DownmixAlgorithm = 'None' | 'Dave750' | 'NightmodeDialogue' | 'Rfc7845' | 'Ac4'
export type SubtitleOcr = 'off' | 'played' | 'scans'

/** A GPU conversions run on. */
export type ConversionGPU = {
  /** cuda for NVIDIA, vaapi for AMD and Intel on Linux, videotoolbox on macOS, qsv for Intel and amf for AMD on Windows. */
  method: string
  device: string
  encoders: string[]
  toneMapping: boolean
  /** Whether a VAAPI GPU takes a quality factor. */
  qvbr: boolean
  /** Whether it encodes with the FFmpeg for NVIDIA drivers older than 610. */
  olderDrivers: boolean
}

/**
 * The FFmpeg for NVIDIA drivers older than 610 while NVENC waits for it, or when it could not
 * download it (`failed`) or NVENC failed with it too (`unusable`), `error` telling why.
 */
export type OlderDriverFFmpeg = {
  state: 'downloading' | 'failed' | 'unusable'
  error?: string
}

/** What conversions run on. */
export type ConversionHardware = {
  /** The GPU chosen, null for none. */
  gpu: ConversionGPU | null
  /** Software video encoders FFmpeg has. */
  encoders: string[]
  /** Whether FFmpeg tone maps HDR on the processor. */
  toneMapping: boolean
  /** Height Automatic caps HDR tone mapped on the processor at, as measured at startup (720 until then); 0 when it cannot. */
  toneMappingHeight: number
  /** Whether FFmpeg has the bwdif deinterlacer. */
  bwdif: boolean
  /** The FFmpeg for older NVIDIA drivers, null when it is used or not needed. */
  olderDriverFFmpeg: OlderDriverFFmpeg | null
}

export type NewUser = {
  name: string
  password: string
  isAdministrator: boolean
  isHidden: boolean
  /** The group the account joins and follows; absent or null for none. */
  groupId?: string | null
  /** Whether the account signs in through the identity provider only; it then needs no password. */
  providerOnly?: boolean
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
  canShare: boolean
  torrentStreams: boolean
  remoteAccess: boolean
  qualityGroup: QualityGroup
  /** YYYY-MM-DD, or null for never. */
  endsOn: string | null
  /** Joins this group and takes its values; '' leaves the group, keeping them as own settings. */
  groupId: string
  /** False detaches the member, keeping the group's values; true takes them back. */
  followGroup: boolean
  /** One of the languages the server's addons were given, or '' for the server's. */
  metadataLanguage: string
  /** One of the user's audioRegions, or '' for none. */
  audioRegion: string
  /** Whether the user signs in through the identity provider only, their password refused. */
  providerOnly: boolean
  /** One of the themes offered (see WebThemes), or '' for the server's. */
  webTheme: UserWebTheme
  /** True keeps the user from changing their web player theme under My account. */
  webThemeLocked: boolean
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
  /** In the answer to adding the addon only: how many of its catalogs became libraries. */
  enabledLibraries?: number
  /**
   * In the answer to adding the addon only: the number of libraries the server, or the user, gets
   * automatically, when none of its catalogs became a library because there were that many already.
   */
  libraryLimit?: number
  refreshedAt: string
  /**
   * 'stremio' for a Stremio addon; 'eclipse' for an Eclipse music addon, which `music` describes;
   * 'm3u', 'xtream' or 'hdhomerun' (an HDHomeRun tuner, the server's only) for an IPTV source,
   * which `source` describes; 'local' for a local folder, which `folder` describes.
   */
  kind: 'stremio' | 'eclipse' | 'm3u' | 'xtream' | 'hdhomerun' | 'local'
  source: IptvSource | null
  music: AddonMusic | null
  folder: LocalFolder | null
  /**
   * The language of the descriptions of a server's metadata addon, a language code; '' for none.
   * Users who chose that metadata language see its descriptions.
   */
  language: string
}

/** What a local folder holds. */
export type FolderKind = 'movies' | 'shows' | 'music' | 'homevideos'

/**
 * Where a folder is: '' in the container, 'smb' an SMB share, 'webdav' a WebDAV folder, 's3' an
 * S3 bucket, 'sftp' a folder of an SFTP server, 'rclone' a folder of an rclone remote.
 */
export type FolderShare = '' | 'smb' | 'webdav' | 's3' | 'sftp' | 'rclone'

/**
 * How a local file was matched to its title. A search keeps a title by a looser rule when none
 * matches exactly: 'year' for a title of the year before or after the name's, 'numerals' for one
 * named alike once numerals are read, 'yearNumerals' for both. 'tmdb' is a title TMDB's search
 * found when every metadata addon's search failed.
 */
export type MatchMethod =
  'name' | 'nfo' | 'search' | 'year' | 'numerals' | 'yearNumerals' | 'link' | 'tmdb'

/** The looser rule a search kept a title by, as a looser match tells it. */
export type LooserRule = 'year' | 'numerals' | 'year_numerals'

/**
 * A local folder: a folder mounted in the container, or a network share Polyfin reads itself,
 * scanned for the titles of its files.
 */
export type LocalFolder = {
  kind: FolderKind
  /** Its path in the container, or the share's address. */
  path: string
  share: FolderShare
  /** The share's user, and whether a password is stored; the server never sends the password. */
  user: string
  passwordSet: boolean
  /** The last scan attempt, and the last scan that could read the folder; null before the first. */
  checkedAt: string | null
  scannedAt: string | null
  /**
   * Why the last scan could not read the folder: 'missing', 'unreadable', 'not_folder', or for a
   * share 'unreachable', 'refused', and for an SFTP folder 'host_key_changed'; else empty.
   */
  error: string
  /** Whether a scan is under way. */
  scanning: boolean
  /** Its video files, or a music folder's tracks, matched those with a title and an album. */
  files: number
  matched: number
  /**
   * The matched files by how they were matched: an identifier in their name, their NFO file, a
   * search, a search by a looser rule, or a link made by hand.
   */
  matchedBy: Record<MatchMethod, number>
  unmatched: number
  /** A music folder's albums, and their artists. */
  albums: number
  artists: number
  /** A music folder's tracks with a gain, from their tags or measured, and those measured. */
  gained: number
  measured: number
  /**
   * A home videos folder's videos by where their dates come from: an NFO file, the recording's
   * creation time, or the file's modification time.
   */
  datedBy: { nfo: number; created: number; file: number }
  /** The files, or shows' folders, linked to an IMDb identifier by hand. */
  /**
   * The units linked by hand, and whether a metadata addon of the server described the title, as
   * the folder's kind, when their files were last matched to it.
   */
  links: { unit: string; imdbId: string; described: boolean }[]
  /** The episodes given by hand to the titles of shows' discs. */
  episodes: GivenEpisode[]
  /** Whether its titles show the artwork beside their files rather than the addons'. */
  useArtwork: boolean
  /** Whether its scans read the extras, local trailers and theme media beside its titles. */
  readExtras: boolean
  /** The extras, local trailers and theme media its last scan found. */
  extras: number
  /** An S3 bucket's endpoint and region, '' for the default one. */
  endpoint: string
  region: string
  /** An rclone folder's remote. */
  rcloneRemote: string
  /** Whether an SFTP folder has a private key and a passphrase stored; never sent. */
  privateKeySet: boolean
  passphraseSet: boolean
  /**
   * The SHA-256 fingerprint of the SFTP server's host key saved at the first connection, and of
   * another one it offers since, which Polyfin refuses until it is accepted; '' for none.
   */
  hostKey: string
  offeredHostKey: string
  /**
   * How often an S3 bucket, an SFTP folder or an rclone remote is listed again for changes, in
   * minutes, 0 only at its scans; null for the other folders.
   */
  pollMinutes: number | null
  /**
   * The files left unmatched as their titles' searches failed, the addon that failed most of them
   * and why (null when none did), and when the folder is scanned again to search them again (null
   * when it is not to be).
   */
  searchFailed: number
  searchFailure: SearchFailure | null
  searchRetryAt: string | null
  /** The files left unmatched as no addon of the server describes and searches titles of its kind. */
  noSearch: number
  /**
   * The episodes its files hold that their series' metadata addon does not list, as the series were
   * last described; they are listed all the same, named after their files.
   */
  unlistedEpisodes: number
}

/**
 * A failed search of a title: the name of the addon that failed it, '' for another failure, and
 * why, as System › Health names an addon's failures, or 'failed'.
 */
export type SearchFailure = {
  addon: string
  failure: string
}

/** The kind of a disc: a Blu-ray disc's or a DVD's folder, or a disc image. */
export type DiscKind = 'bluray' | 'dvd' | 'bluray_image' | 'dvd_image'

/**
 * The episodes given by hand to a title of a show's disc: the title's path, its disc's kind (empty
 * when the disc is gone), folder or image, and number, and its episodes, such as `S01E05` or
 * `S01E05-E06`.
 */
export type GivenEpisode = {
  path: string
  disc: DiscKind | ''
  discPath: string
  number: number
  episode: string
}

/** A file of a local folder no title was matched to, with why. */
export type UnmatchedFile = {
  path: string
  /**
   * What a link would link: the file, its show's folder, its disc's folder (Blu-ray disc or DVD),
   * or a Blu-ray disc's title other than its main one, as `<disc folder>#<playlist number>`.
   */
  unit: string
  title: string
  year: number | null
  size: number
  reason:
    | 'unreadable_name'
    | 'not_found'
    | 'ambiguous'
    | 'other_year'
    | 'search_failed'
    | 'no_search'
    | 'disc_title'
    | 'disc_episode'
    | 'lone_extra'
  /** For a failed search, the addon that failed it and why; null for another reason. */
  search: SearchFailure | null
  /**
   * The kind of disc the file is read as, its unit the disc's folder, or the image itself for a
   * disc image; null for another file.
   */
  disc: DiscKind | null
  /**
   * A title of a show's disc, its unit the show's folder: its disc's folder or image, its number,
   * how long it plays in seconds, its episodes and those suggested, such as `S01E05`; null for
   * another file.
   */
  discTitle: {
    discPath: string
    number: number
    length: number | null
    episode: string | null
    suggested: string | null
  } | null
}

export type NewLocalFolder = {
  name: string
  path: string
  kind: FolderKind
  user?: string
  password?: string
  /** Whether its titles show the artwork beside their files; yes when left out. */
  useArtwork?: boolean
  /** Whether its scans read the extras beside its titles; yes when left out. */
  readExtras?: boolean
  /** An S3 bucket's endpoint and region; its user and password are its access and secret keys. */
  endpoint?: string
  region?: string
  /** An SFTP folder's private key, OpenSSH or PEM, and its passphrase. */
  privateKey?: string
  passphrase?: string
  /** An rclone folder's remote. */
  rcloneRemote?: string
  /** Minutes between the listings for changes, 0 only at scans; the kind's default when left out. */
  pollMinutes?: number
}

/**
 * A change of a folder; a password, private key or passphrase left out keeps the stored one, an
 * empty one removes it. A new kind scans the folder again as that kind. `acceptHostKey` accepts
 * the host key an SFTP server offers in place of the one saved.
 */
export type LocalFolderPatch = Partial<{
  name: string
  path: string
  user: string
  password: string
  useArtwork: boolean
  readExtras: boolean
  kind: FolderKind
  endpoint: string
  region: string
  privateKey: string
  passphrase: string
  rcloneRemote: string
  pollMinutes: number
  acceptHostKey: boolean
}>

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

/**
 * An Eclipse addon's content, how many catalog rows of its own it lists (none leaves My music as its
 * library), settings, and the values chosen (a setting left out uses its default).
 */
export type AddonMusic = {
  contentType: MusicContent
  rows: number
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
  /** The user agent its administrator set for its list and streams; empty for the default. */
  userAgent: string
  /** An HDHomeRun tuner; null for another source. */
  tuner: IptvTuner | null
}

/** An HDHomeRun tuner, as a source of the server. */
export type IptvTuner = {
  /** Its device identifier. */
  device: string
  model: string
  /** How many channels it plays at once. */
  tuners: number
  /**
   * Why its last stream failed, other than its tuners being in use: 'unreachable', or a stream
   * failure code; empty once a stream plays.
   */
  failure: string
  failedAt: string | null
  /** Its channel scan under way: its progress in percent and the channels found; null for none. */
  scan: { progress: number; found: number } | null
}

/** A tuner that answered on the local network, with the source it is already, null for none. */
export type FoundTuner = {
  deviceId: string
  name: string
  model: string
  tuners: number
  address: string
  source: string | null
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
  /** A tuner only: whether only the channels it marks favorite are kept. */
  favoritesOnly: boolean
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
  favoritesOnly: false,
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
  userAgent?: string
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
  /** dead (no live stream), refused (not served now), timeout (nothing came), drm (needs a DRM Polyfin does not support), keys (the list's keys do not decrypt it), or empty. */
  failure: '' | 'dead' | 'refused' | 'timeout' | 'drm' | 'keys'
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

/** An IPTV source to add; a tuner named nothing takes the name it gives itself. */
export type NewIptvSource = {
  name: string
  kind: 'm3u' | 'xtream' | 'hdhomerun'
  url?: string
  server?: string
  username?: string
  password?: string
  /** M3U and Xtream only: the user agent of its list and streams; empty for the default. */
  userAgent?: string
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
  userAgent: string
  options: Partial<IptvOptions>
}>

export type AddonPatch = Partial<{ enabled: boolean; manifestUrl: string; language: string }>

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
  /**
   * Whether an enabled library of the server's has its descriptions read again on request: a movie
   * or series library of a Stremio addon or a local folder. False in the users' own scopes.
   */
  refreshable: boolean
  /** How the last refresh of its descriptions since the server started stands; null for none. */
  refresh: LibraryRefresh | null
}

/**
 * The refresh of a library's descriptions: `titles` to describe again, `done` described,
 * `failed` that no addon described; `startedAt` and `endedAt` null until it starts and ends.
 */
export type LibraryRefresh = {
  state: 'queued' | 'running' | 'done' | 'stopped' | 'failed'
  titles: number
  done: number
  failed: number
  startedAt: string | null
  endedAt: string | null
}

/** A library of the server's, by its addon and catalog. */
export type LibraryKey = { addonId: string; catalogType: string; catalogId: string }

/** Has the descriptions of one of the server's libraries read again, paced, in the background. */
export const refreshLibrary = (library: LibraryKey) =>
  request<LibraryRefresh>('POST', '/scopes/shared/libraries/refresh', library)

/** Stops the refresh of one of the server's libraries, running or waiting. */
export const stopLibraryRefresh = (library: LibraryKey) =>
  request<void>('POST', '/scopes/shared/libraries/refresh/stop', library)

/** How many users keep an order of libraries of their own, saved from a Jellyfin app. */
export type LibraryOrders = { users: number }

/** Counts the users who keep an order of libraries of their own. */
export const libraryOrders = () =>
  request<LibraryOrders>('GET', '/scopes/shared/libraries/user-orders')

/** Apply this order to every user: forgets the orders of libraries users saved. */
export const applyLibraryOrder = () =>
  request<LibraryOrders>('DELETE', '/scopes/shared/libraries/user-orders')

/** A series as Episode orders lists and finds it. */
export type OrderedSeries = { id: string; name: string; year: number }

/** The name Jellyfin gives a TMDB episode group's type, a series' DisplayOrder. */
export type EpisodeGroupType =
  'originalAirDate' | 'absolute' | 'dvd' | 'digital' | 'storyArc' | 'production' | 'tv'

/** Where a series' specials show. */
export type SpecialsPlacement = 'first' | 'last' | 'hidden'

/** One of a series' TMDB episode groups. */
export type EpisodeGroup = {
  id: string
  name: string
  type: EpisodeGroupType
  episodes: number
  seasons: number
}

/**
 * A series' order of episodes: `order` is its DisplayOrder, empty as aired (another value than a
 * group type is set from a Jellyfin app, and shows as aired); `group` the group chosen here, empty
 * for the first of its type. `lookup` tells why `groups` is empty when TMDB could not tell.
 */
export type EpisodeOrder = {
  series: OrderedSeries
  order: string
  group: string
  specials: SpecialsPlacement
  groups: EpisodeGroup[]
  lookup: '' | 'no_key' | 'unknown' | 'failed'
}

/** The series shown in an order of their own, or with their specials elsewhere than first. */
export const fetchEpisodeOrders = (signal?: AbortSignal) =>
  request<EpisodeOrder[]>('GET', '/episode-orders', undefined, signal)

/** Searches the series to choose an order for. */
export const searchOrderedSeries = (search: string, signal?: AbortSignal) =>
  request<OrderedSeries[]>(
    'GET',
    `/episode-orders/series?${new URLSearchParams({ search })}`,
    undefined,
    signal,
  )

/** A series' order of episodes, with the TMDB episode groups it may show in. */
export const fetchEpisodeOrder = (id: string, signal?: AbortSignal) =>
  request<EpisodeOrder>('GET', `/titles/${encodeURIComponent(id)}/episode-order`, undefined, signal)

/** Shows a series in a TMDB episode group, as aired when `group` is empty. */
export const saveEpisodeOrder = (id: string, group: string, specials: SpecialsPlacement) =>
  request<EpisodeOrder>('PUT', `/titles/${encodeURIComponent(id)}/episode-order`, {
    group,
    specials,
  })

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

export type LibraryImageChoice = 'none' | 'automatic' | 'generated' | 'custom'

/**
 * Chooses the image of one enabled library: none, automatic, generated, or custom with a picture
 * sent in base64 (`data`) or found at an address the server downloads once (`url`).
 */
export type LibraryImageRequest = Pick<Library, 'addonId' | 'catalogType' | 'catalogId'> &
  (
    | { image: 'none' | 'automatic' | 'generated' }
    | { image: 'custom'; data: string }
    | { image: 'custom'; url: string }
  )

/** What a rule of a smart list looks at; `group` holds rules of its own. */
export type SmartField =
  | 'group'
  | 'genre'
  | 'year'
  | 'rating'
  | 'votes'
  | 'language'
  | 'runtime'
  | 'contentRating'
  | 'person'
  | 'studio'
  | 'released'
  | 'type'
  | 'rank'
  | 'state'

/** What a `state` rule matches, for the user who opens the list. */
export type SmartState = 'played' | 'unplayed' | 'started' | 'favorite'

/** How a smart list orders its titles. */
export type SmartSort =
  'catalog' | 'premiere' | 'latestEpisode' | 'seasonPremiere' | 'rating' | 'name' | 'random'

/**
 * A rule of a smart list, or a group of them. `not` negates it. `value` is a genre, an ISO 639
 * language code, the highest content rating, a person's or a studio's name, `movie` or `series`,
 * or a SmartState; `min` and `max` bound a year, a rating, its votes, a runtime in minutes, or a
 * release in days from today (negative: past), and `max` is the place in its catalog. A group
 * matches `all` or `any` of its `rules`.
 */
export type SmartRule = {
  field: SmartField
  not?: boolean
  value?: string
  min?: number
  max?: number
  match?: 'all' | 'any'
  rules?: SmartRule[]
}

/** A catalog a smart list draws from. */
export type SmartCatalogRef = { addonId: string; type: string; id: string }

/** A smart list as it is saved: `itemId` is its library or collection in Jellyfin apps. */
export type SmartList = {
  id: string
  itemId: string
  name: string
  shownAs: 'library' | 'collection'
  catalogs: SmartCatalogRef[]
  rules: SmartRule
  /** The group of rules whose titles it leaves out; one without rules leaves none out. */
  exclude: SmartRule
  sort: SmartSort
  /** Most titles it lists; null for no maximum. */
  maxItems: number | null
  updatedAt: string
}

/** A smart list being made or changed. */
export type SmartListDraft = Omit<SmartList, 'id' | 'itemId' | 'updatedAt'>

/**
 * A catalog a scope's smart lists may draw from. `shared`: one of the server's addons, which a
 * user's own list may use while they use the server's sources.
 */
export type SmartCatalogChoice = {
  addonId: string
  addonName: string
  shared: boolean
  catalogType: string
  catalogId: string
  catalogName: string
  /** Genres its genre filter offers; empty when it offers none. */
  genres: string[]
}

/**
 * What a smart list may draw from, the ISO 639-1 codes of the languages its rules may name, and the
 * content ratings a rule may set as the highest, from the youngest.
 */
export type SmartChoices = {
  catalogs: SmartCatalogChoice[]
  languages: string[]
  contentRatings: string[]
}

/**
 * A title of a smart list's preview. `languages`: those its description gives, by their ISO 639-1
 * code when they have one; empty while it was not described.
 */
export type SmartTitle = {
  id: string
  name: string
  type: 'movie' | 'series'
  year: number | null
  rating: number | null
  genres: string[]
  languages: string[]
}

/**
 * A smart list as it stands now: its first titles, how many it lists, how many may match once their
 * description is read, and whether every catalog answered in time.
 */
export type SmartPreview = {
  titles: SmartTitle[]
  listed: number
  unknown: number
  complete: boolean
}

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
  /** What a remote server or a tool answered, as the server tells it, when it tells it. */
  readonly detail: string | null
  /** Why a WireGuard configuration was refused, for invalid_wireguard_config. */
  readonly wireGuard: WireGuardConfigProblem | null
  /** The addresses no copy of the theme could save, for theme_copy_failed. */
  readonly unsaved: ThemeCopyUnsaved[] | null

  constructor(
    status: number,
    code: string,
    jellyfinId: string | null = null,
    detail: string | null = null,
    wireGuard: WireGuardConfigProblem | null = null,
    unsaved: ThemeCopyUnsaved[] | null = null,
  ) {
    super(`Admin API error ${status}: ${code}`)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.jellyfinId = jellyfinId
    this.detail = detail
    this.wireGuard = wireGuard
    this.unsaved = unsaved
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
  if (!response.ok) throw await apiError(response)
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

/** The error of an answer that is not ok, with its code, from its JSON body when it has one. */
async function apiError(response: Response): Promise<ApiError> {
  let code = 'unknown'
  let jellyfinId: string | null = null
  let detail: string | null = null
  let wireGuard: WireGuardConfigProblem | null = null
  let unsaved: ThemeCopyUnsaved[] | null = null
  try {
    const payload = (await response.json()) as {
      error?: unknown
      jellyfinId?: unknown
      message?: unknown
      line?: unknown
      key?: unknown
      reason?: unknown
      unsaved?: unknown
    }
    if (typeof payload.error === 'string') code = payload.error
    if (typeof payload.jellyfinId === 'string') jellyfinId = payload.jellyfinId
    if (typeof payload.message === 'string') detail = payload.message
    if (
      code === 'invalid_wireguard_config' &&
      typeof payload.line === 'number' &&
      typeof payload.key === 'string' &&
      typeof payload.reason === 'string'
    ) {
      wireGuard = {
        line: payload.line,
        key: payload.key,
        reason: payload.reason as WireGuardConfigProblem['reason'],
      }
    }
    if (code === 'theme_copy_failed' && Array.isArray(payload.unsaved)) {
      unsaved = payload.unsaved as ThemeCopyUnsaved[]
    }
  } catch {
    // Non-JSON error body (proxy page, empty response): keep the generic code.
  }
  return new ApiError(response.status, code, jellyfinId, detail, wireGuard, unsaved)
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

/**
 * What an authenticator app reads: the key, the otpauth:// address that holds it, and that address
 * as a QR code, a row a line, '1' for a dark module.
 */
export type TwoStepSetup = { secret: string; uri: string; qr: string[] }

/**
 * The answer to a right password: the session, or, without one yet, the second step it needs: a
 * code, or, for an administrator required to use two-step sign-in, its setup.
 */
export type SignInAnswer =
  | { user: SessionUser; twoStep?: undefined }
  | { twoStep: 'code' }
  | { twoStep: 'setup'; setup: TwoStepSetup }

export const signIn = (body: { name: string; password: string }) =>
  request<SignInAnswer>('POST', '/session', body)

/**
 * Completes a sign-in with a code of the app or a recovery code; after a setup, the answer holds
 * the recovery codes, shown this once.
 */
export const signInSecondStep = (code: string) =>
  request<{ user: SessionUser; recoveryCodes?: string[] }>('POST', '/session/two-step', { code })

/** How the signed-in user's two-step sign-in stands. */
export type TwoStepState = {
  on: boolean
  /** Recovery codes left. */
  recoveryCodes: number
  /** POLYFIN_SECRET_KEY can no longer read the key: only recovery codes sign in. */
  unreadable: boolean
  /** The server requires it of this user, an administrator. */
  required: boolean
}

/** A change to two-step sign-in is confirmed with a code or the password. */
export type TwoStepConfirmation = { code: string } | { password: string }

export const fetchOwnTwoStep = (signal?: AbortSignal) =>
  request<TwoStepState>('GET', '/account/two-step', undefined, signal)

export const startOwnTwoStep = () => request<TwoStepSetup>('POST', '/account/two-step/setup')

export const confirmOwnTwoStep = (code: string) =>
  request<{ recoveryCodes: string[] }>('POST', '/account/two-step/confirm', { code })

export const newOwnRecoveryCodes = (confirmation: TwoStepConfirmation) =>
  request<{ recoveryCodes: string[] }>('POST', '/account/two-step/recovery-codes', confirmation)

export const turnOffOwnTwoStep = (confirmation: TwoStepConfirmation) =>
  request<void>('POST', '/account/two-step/off', confirmation)

/**
 * Returns the signed-in user, or null when there is no valid session. Without one, the web client's
 * sign-in on this server is tried first: it opens a session for its user, administrator or member.
 * Opened from the web client signed in as someone else, the admin app signs its session out and
 * takes the web client's, so that it never shows another user's pages than the web client's.
 */
export async function fetchSession(signal?: AbortSignal): Promise<SessionUser | null> {
  const serverId = queryClient.getQueryData<Status>(queryKeys.status)?.serverId
  try {
    const user = (await request<{ user: SessionUser }>('GET', '/session', undefined, signal)).user
    if (!arrivedAsAnotherUser(serverId, user.id)) return user
    await request<void>('DELETE', '/session', undefined, signal)
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401)) throw error
  }
  const token = takeWebClientToken(serverId)
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

/** The signed-in user's metadata language ('' for the server's) and the ones they may choose. */
export type MetadataLanguage = { language: string; languages: string[] }

export const fetchOwnMetadataLanguage = (signal?: AbortSignal) =>
  request<MetadataLanguage>('GET', '/account/metadata-language', undefined, signal)

export const saveOwnMetadataLanguage = (language: string) =>
  request<MetadataLanguage>('PUT', '/account/metadata-language', { language })

/** The signed-in user's web player theme ('' for the server's), whether the administrator keeps them from changing it, and the themes they choose among. */
export type OwnWebTheme = WebThemes & { webTheme: UserWebTheme; locked: boolean }

export const fetchOwnWebTheme = (signal?: AbortSignal) =>
  request<OwnWebTheme>('GET', '/account/web-theme', undefined, signal)

export const saveOwnWebTheme = (webTheme: UserWebTheme) =>
  request<OwnWebTheme>('PUT', '/account/web-theme', { webTheme })

/** The signed-in user's preferred audio region ('' for none) and the ones they may choose. */
export type AudioRegion = { region: string; regions: string[] }

export const fetchOwnAudioRegion = (signal?: AbortSignal) =>
  request<AudioRegion>('GET', '/account/audio-region', undefined, signal)

export const saveOwnAudioRegion = (region: string) =>
  request<AudioRegion>('PUT', '/account/audio-region', { region })

/**
 * Puts back the server's defaults for what the signed-in user chose: their metadata language, their
 * audio region, the home rows they turned off, the tracks they chose on titles, the preferences
 * Jellyfin apps saved in their account, and their web player theme unless the administrator chose
 * it.
 */
export const resetOwnPreferences = () => request<void>('POST', '/account/preferences/reset')

/** How many series and movies the signed-in user chose audio or subtitle tracks on. */
export type TrackChoices = { titles: number }

export const fetchOwnTrackChoices = (signal?: AbortSignal) =>
  request<TrackChoices>('GET', '/account/track-choices', undefined, signal)

/** Forgets the tracks the signed-in user chose on every series and movie. */
export const forgetOwnTrackChoices = () => request<void>('DELETE', '/account/track-choices')

export const fetchMyDevices = (signal?: AbortSignal) =>
  request<Device[]>('GET', '/account/devices', undefined, signal)

export const signOutMyDevice = (id: string) =>
  request<void>('DELETE', `/account/devices/${seg(id)}`)

/** A title of the user's lists on My account. */
export type AccountTitle = {
  id: string
  kind: 'movie' | 'series' | 'episode' | 'video' | 'replay' | string
  name: string
  seriesName: string
  season: number
  episode: number
  year: number
  /** Seconds, 0 when unknown. */
  runtime: number
  /** The item whose Primary image is the poster: the series of an episode. */
  posterId: string
}

/** What a home row of the web player lists. */
export type HomeRowKind = 'catalog' | 'collection' | 'because' | 'new' | 'available' | 'waiting'

/** Where the web player shows a home row: at the top, after Continue Watching, after Next Up, at the bottom. */
export type HomeRowPlace = 'top' | 'resume' | 'nextup' | 'bottom'

/**
 * A row of the web player's home page (Settings › Web player › Home rows). A catalog row names its
 * catalog, a collection row its collection; `count` is how many rows Because you watched makes;
 * `users` are those a row not shown to everyone is shown to; `library` also makes it a library in
 * other apps.
 */
export type HomeRow = {
  id: string
  name: string
  kind: HomeRowKind
  place: HomeRowPlace
  addonId: string
  catalogType: string
  catalogId: string
  collectionId: string
  count: number
  everyone: boolean
  users: string[]
  library: boolean
}

export type HomeRowDraft = Omit<HomeRow, 'id'>

/** A catalog of the server's addons a home row may list. */
export type HomeRowCatalog = {
  addonId: string
  addonName: string
  catalogType: string
  catalogId: string
  catalogName: string
}

/** A collection a home row may list: made by users, a smart list or an addon's. */
export type HomeRowCollection = { id: string; name: string; source: 'users' | 'smart' | 'addon' }

export const fetchHomeRows = (signal?: AbortSignal) =>
  request<{ rows: HomeRow[] }>('GET', '/home-rows', undefined, signal)

export const fetchHomeRowChoices = (signal?: AbortSignal) =>
  request<{ catalogs: HomeRowCatalog[]; collections: HomeRowCollection[] }>(
    'GET',
    '/home-rows/choices',
    undefined,
    signal,
  )

export const createHomeRow = (draft: HomeRowDraft) => request<HomeRow>('POST', '/home-rows', draft)

export const updateHomeRow = (id: string, draft: HomeRowDraft) =>
  request<HomeRow>('PATCH', `/home-rows/${seg(id)}`, draft)

export const deleteHomeRow = (id: string) => request<void>('DELETE', `/home-rows/${seg(id)}`)

/** Puts the rows in this order, and answers them. */
export const orderHomeRows = (ids: string[]) =>
  request<{ rows: HomeRow[] }>('PUT', '/home-rows/order', { ids })

/** A home row shown to the signed-in user, and whether they turned it off. */
export type OwnHomeRow = {
  id: string
  name: string
  kind: HomeRowKind
  place: HomeRowPlace
  off: boolean
}

export const fetchOwnHomeRows = (signal?: AbortSignal) =>
  request<{ rows: OwnHomeRow[] }>('GET', '/account/home-rows', undefined, signal)

export const setOwnHomeRow = (id: string, off: boolean) =>
  request<void>('PUT', `/account/home-rows/${seg(id)}`, { off })

/** A title of Continue Watching: its resume point and runtime in seconds, 0 when unknown. */
export type ResumeTitle = {
  item: AccountTitle
  position: number
  runtime: number
  played: boolean
  lastPlayedAt: string | null
}

/** A title hidden from the user's lists, and when. */
export type HiddenTitle = { item: AccountTitle; hiddenAt: string }

export const fetchOwnResume = (signal?: AbortSignal) =>
  request<ResumeTitle[]>('GET', '/account/resume', undefined, signal)

export const removeOwnResume = (id: string) => request<void>('DELETE', `/account/resume/${seg(id)}`)

export const fetchOwnHidden = (signal?: AbortSignal) =>
  request<HiddenTitle[]>('GET', '/account/hidden', undefined, signal)

export const showOwnHidden = (id: string) => request<void>('DELETE', `/account/hidden/${seg(id)}`)

/** A reminder of one programme, set from the web player. */
export type ProgrammeReminder = {
  id: string
  programId: string
  name: string
  channelId: string
  channelName: string
  start: string
  end: string
}

/** A reminder of every airing of a title on a channel, at the time of day of `start`. */
export type SeriesReminder = {
  id: string
  name: string
  channelId: string
  channelName: string
  start: string
  /** The next airing in the guide, null while none is. */
  next: string | null
}

/** My account › Reminders: `available` is false without the Live TV permission. */
export type OwnReminders = {
  available: boolean
  reminders: ProgrammeReminder[]
  series: SeriesReminder[]
}

/**
 * How long before a programme its reminder comes, one of `choices`, and whether it also shows on
 * the user's apps; `available` is false without the Live TV permission.
 */
export type ReminderSettings = {
  available: boolean
  beforeMinutes: number
  onApps: boolean
  choices: number[]
}

export const fetchOwnReminders = (signal?: AbortSignal) =>
  request<OwnReminders>('GET', '/account/reminders', undefined, signal)

export const deleteOwnReminder = (id: string) =>
  request<void>('DELETE', `/account/reminders/${seg(id)}`)

export const fetchReminderSettings = (signal?: AbortSignal) =>
  request<ReminderSettings>('GET', '/account/reminders/settings', undefined, signal)

export const updateReminderSettings = (settings: { beforeMinutes: number; onApps: boolean }) =>
  request<ReminderSettings>('PUT', '/account/reminders/settings', settings)

/** The tracking services, in the order the server lists them. */
export type TrackingServiceName =
  | 'trakt'
  | 'simkl'
  | 'mdblist'
  | 'publicmetadb'
  | 'anilist'
  | 'myanimelist'
  | 'lastfm'
  | 'listenbrainz'

/** The services users connect with an API key or user token, which they can show again. */
export type TrackingKeyServiceName = 'mdblist' | 'publicmetadb' | 'listenbrainz'

/** One of the signed-in user's tracking services, which Polyfin tells what the user watches. */
export type TrackingService = {
  service: TrackingServiceName
  /**
   * "code": the user enters a code on the service's site; "signin": the user signs in on the
   * service's site and allows Polyfin; "pin": the user allows Polyfin on the service's site and
   * pastes the code it shows (AniList); "redirect": the user allows Polyfin on the service's site,
   * which sends the browser back to the admin app, or pastes the address it sent it to
   * (MyAnimeList); "key": the user pastes an API key or user token.
   */
  connection: 'code' | 'signin' | 'pin' | 'redirect' | 'key'
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
  /** When a token that cannot be refreshed (AniList's) expires, which stops the connection. */
  expiresAt: string | null
  /** Whether the service's watch history is imported into Polyfin, at the server's interval. */
  importHistory: boolean
  /** An import runs now. */
  importing: boolean
  /** How the last import went; null before the first. */
  lastImport: TrackingImport | null
  /** When the next import runs; null while none is planned. */
  nextImportAt: string | null
  /**
   * The service has a watchlist Polyfin can import: Trakt's watchlist, Simkl's, AniList's and
   * MyAnimeList's Plan to Watch.
   */
  watchlist: boolean
  /** Whether that watchlist is imported, with the history's imports, into the Watchlist view. */
  importWatchlist: boolean
  /** The service keeps ratings (AniList, MyAnimeList). */
  ratings: boolean
  /** Whether the user's ratings are sent there, and its scores imported. */
  syncRatings: boolean
}

/** How an import of a watch history, and of a watchlist, went. */
export type TrackingImport = {
  at: string
  /** Titles it newly marked played. */
  played: number
  /** Resume points it set. */
  resumed: number
  /** Titles it added to the Watchlist view. */
  watchlist: number
  /** Titles of the history and the watchlist Polyfin could not identify. */
  unmapped: number
  /** Titles it gave the service's score. */
  rated: number
  /** Ratings of series spread over several entries, skipped by it and by the ratings sent since. */
  ratingsSkipped: number
  /** Why the history could not be read whole; what was read is imported all the same. */
  problem: 'reconnect' | 'unreachable' | 'rate_limited' | null
}

/** The signed-in user's tracking services, and how often their histories are imported, in minutes. */
export type TrackingList = { services: TrackingService[]; importEveryMinutes: number }

export const fetchTracking = (signal?: AbortSignal) =>
  request<TrackingList>('GET', '/account/tracking', undefined, signal)

/** Connects a key service with `key`, or starts a code connection without it. */
export const connectTracking = (service: TrackingServiceName, key?: string) =>
  request<TrackingService>(
    'POST',
    `/account/tracking/${seg(service)}`,
    key === undefined ? {} : { key },
  )

/**
 * Connects AniList with the code it showed the user, or MyAnimeList with the address it sent the
 * browser back to, or its code.
 */
export const exchangeTrackingCode = (service: 'anilist' | 'myanimelist', code: string) =>
  request<TrackingService>('POST', `/account/tracking/${seg(service)}/code`, { code })

/**
 * Turns the import of a connected service's watch history, or of its watchlist, or the sync of the
 * user's ratings with it, on (importing at once) or off.
 */
export const setTrackingImport = (
  service: TrackingServiceName,
  change: { importHistory?: boolean; importWatchlist?: boolean; syncRatings?: boolean },
) => request<TrackingService>('PATCH', `/account/tracking/${seg(service)}`, change)

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
export type NotificationKind =
  'webhook' | 'discord' | 'ntfy' | 'email' | 'telegram' | 'gotify' | 'pushover'

/** The events targets choose from. */
export type NotificationEvent =
  | 'new_episode'
  | 'recording_finished'
  | 'recording_failed'
  | 'health_problem'
  | 'health_solved'
  | 'user_joined'
  | 'new_version'
  | 'playback_started'
  | 'playback_paused'
  | 'playback_resumed'
  | 'playback_stopped'
  | 'weekly_summary'
  | 'monthly_summary'
  | 'yearly_summary'
  | 'connection_stopped'
  | 'account_blocked'
  | 'new_device'
  | 'share_played'
  | 'programme_reminder'
  | 'title_available'

/**
 * Where notifications go. Its secret address, tokens and keys are never sent back: `address` is
 * only the scheme and host of a webhook's or Discord target's address, an ntfy or Gotify target's
 * server, an email target's recipient, and empty for Telegram and Pushover targets.
 */
export type NotificationTarget = {
  id: string
  kind: NotificationKind
  name: string
  address: string
  /** An ntfy target's topic, empty for the others. */
  topic: string
  /** A Telegram target's chat, empty for the others. */
  chat: string
  /** Whether an ntfy, Telegram, Gotify or Pushover target has a token. */
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
  /** The HTTP status, or an email target's SMTP code, answered with its problem, when it answered. */
  problemStatus: number | null
  problemAt: string | null
}

/** The targets of the server, or of the signed-in user, and what they may choose. */
export type Notifications = {
  targets: NotificationTarget[]
  /** The events these targets may receive: health events only for the server and administrators. */
  events: NotificationEvent[]
  kinds: NotificationKind[]
  /** Whether email targets can be added: the server has an SMTP server. */
  emailAvailable: boolean
}

/** A target to add, or the changes to one: fields left out keep their values. */
export type NotificationDraft = {
  kind?: NotificationKind
  name?: string
  /**
   * A webhook's or Discord target's address, an ntfy target's server (empty: the public one), a
   * Gotify target's server, or an email target's recipient.
   */
  address?: string
  topic?: string
  /** A Telegram target's chat. */
  chat?: string
  /**
   * An ntfy target's access token, empty removes it; a Telegram bot's token; a Gotify or Pushover
   * application's token.
   */
  token?: string
  /** A Pushover target's user key. */
  userKey?: string
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
  request<{
    delivered: boolean
    status: number | null
    /** Why it did not accept it: the start of its answer, without its address or tokens. */
    answer: string | null
    target: NotificationTarget
  }>('POST', `${notificationsPath(scope)}/targets/${seg(id)}/test`)

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

/** Turns off the two-step sign-in of a user who lost their device; the activity log records it. */
export const turnOffUserTwoStep = (id: string) =>
  request<User>('POST', `/users/${seg(id)}/two-step/off`)

/** Takes the permission to download away from every user; answers with those who had it. */
export const turnOffDownloads = () => request<User[]>('POST', '/users/downloads/off')

export const fetchGroups = (signal?: AbortSignal) =>
  request<GroupSummary[]>('GET', '/groups', undefined, signal)

/** A new group, with the values of a user made with Create a user, or those of `copyFrom`. */
export const createGroup = (body: { name: string; copyFrom?: string }) =>
  request<Group>('POST', '/groups', body)

export const fetchGroup = (id: string, signal?: AbortSignal) =>
  request<Group>('GET', `/groups/${seg(id)}`, undefined, signal)

/** Changes a group; its following members take the new values at once. */
export const updateGroup = (id: string, patch: GroupPatch) =>
  request<Group>('PATCH', `/groups/${seg(id)}`, patch)

/** Deletes a group: its members keep its values as their own settings. */
export const deleteGroup = (id: string) => request<void>('DELETE', `/groups/${seg(id)}`)

/** Puts users in a group, following it; an administrator among them refuses them all. */
export const addGroupMembers = (id: string, userIds: string[]) =>
  request<unknown>('POST', `/groups/${seg(id)}/members`, { userIds })

/** The limits Reset to defaults would give a user or a group: a new user's. */
export const fetchUserDefaults = (id: string, signal?: AbortSignal) =>
  request<GroupLimits>('GET', `/users/${seg(id)}/defaults`, undefined, signal)

export const fetchGroupDefaults = (id: string, signal?: AbortSignal) =>
  request<GroupLimits>('GET', `/groups/${seg(id)}/defaults`, undefined, signal)

/** Puts a user's limits back to a new user's; the rest of the account stays. */
export const resetUser = (id: string) => request<User>('POST', `/users/${seg(id)}/reset`)

/** Puts a group's limits back to a new user's; its following members take them at once. */
export const resetGroup = (id: string) => request<Group>('POST', `/groups/${seg(id)}/reset`)

export type InviteState = 'active' | 'used_up' | 'expired' | 'revoked'

/**
 * An invite link: how many accounts it may create and has created, when it expires (null for
 * never), the user whose settings new accounts copy (null for a new user's), the group they join
 * (null for none), who created it (null once deleted), and how many days after its creation each
 * account it creates ends (null for never).
 */
export type Invite = {
  id: string
  maxUses: number
  uses: number
  expiresAt: string | null
  createdAt: string
  revokedAt: string | null
  model: { id: string; name: string } | null
  group: { id: string; name: string } | null
  createdBy: { id: string; name: string } | null
  state: InviteState
  accountsEndInDays: number | null
}

/**
 * An invite just created, with the token of its link, given this once, and its address when the
 * server's public address is set.
 */
export type NewInvite = Invite & { token: string; url: string | null }

export const fetchInvites = (signal?: AbortSignal) =>
  request<Invite[]>('GET', '/invites', undefined, signal)

/**
 * `expiresInDays` null never expires; `accountsEndInDays` null never ends the accounts; `groupId`
 * null joins no group.
 */
export const createInvite = (body: {
  maxUses: number
  expiresInDays: number | null
  modelUserId: string | null
  accountsEndInDays: number | null
  groupId: string | null
}) => request<NewInvite>('POST', '/invites', body)

export const revokeInvite = (id: string) => request<Invite>('POST', `/invites/${seg(id)}/revoke`)

export type ShareLinkState = 'active' | 'expired' | 'revoked'

/**
 * A share link: the movie or episode it gives, named as it was when the link was made, who made it,
 * when it expires, how many guests may watch at once, and how many times it was played.
 */
export type ShareLink = {
  id: string
  user: { id: string; name: string }
  item: { id: string; kind: 'movie' | 'episode'; name: string }
  maxViewers: number
  createdAt: string
  expiresAt: string
  revokedAt: string | null
  plays: number
  state: ShareLinkState
}

/**
 * A share link just made, with the token of its address, given this once, and that address when
 * the server's public address is set (null otherwise: the admin app completes it).
 */
export type NewShareLink = ShareLink & { token: string; url: string | null }

/** The expiries the server accepts for a share link, in hours: 1 hour, 1 day, 7 and 30 days. */
export const shareLinkHours = [1, 24, 168, 720] as const
export type ShareLinkHours = (typeof shareLinkHours)[number]

/** The most guests a share link lets watch at once. */
export const maxShareViewers = 5

/**
 * The signed-in user's share links, with whether they may make one now: the server allows share
 * links, and their account may share.
 */
export const fetchOwnShareLinks = (signal?: AbortSignal) =>
  request<{ allowed: boolean; links: ShareLink[] }>(
    'GET',
    '/account/share-links',
    undefined,
    signal,
  )

export const createShareLink = (body: {
  itemId: string
  expiresInHours: ShareLinkHours
  maxViewers: number
}) => request<NewShareLink>('POST', '/account/share-links', body)

export const revokeOwnShareLink = (id: string) =>
  request<ShareLink>('POST', `/account/share-links/${seg(id)}/revoke`)

/** Movies and series the signed-in user may share, up to 20; a series gives one episode at a time. */
export const searchShareTitles = (term: string, signal?: AbortSignal) =>
  request<AccountTitle[]>(
    'GET',
    `/account/share-titles?${new URLSearchParams({ search: term })}`,
    undefined,
    signal,
  )

/** A movie or an episode the signed-in user may share, as the web client's Share opens it. */
export const fetchShareTitle = (id: string, signal?: AbortSignal) =>
  request<AccountTitle>('GET', `/account/share-titles/${seg(id)}`, undefined, signal)

/** The episodes of a series the signed-in user may share, in order. */
export const fetchShareEpisodes = (seriesId: string, signal?: AbortSignal) =>
  request<AccountTitle[]>(
    'GET',
    `/account/share-titles/${seg(seriesId)}/episodes`,
    undefined,
    signal,
  )

/** A user's share links, for an administrator. */
export const fetchUserShareLinks = (userId: string, signal?: AbortSignal) =>
  request<ShareLink[]>('GET', `/users/${seg(userId)}/share-links`, undefined, signal)

/** Revokes any user's share link, for an administrator. */
export const revokeShareLink = (id: string) =>
  request<ShareLink>('POST', `/share-links/${seg(id)}/revoke`)

/** Whether an invite's link may still create an account, with the server's name; no session. */
export const fetchPublicInvite = (token: string, signal?: AbortSignal) =>
  request<{ serverName: string }>('GET', `/invite/${seg(token)}`, undefined, signal)

/**
 * Creates the guest's account through an invite's link. With the web client (`webClient`), the
 * guest still has to sign in to it; without it, they are signed in to the admin app.
 */
export const acceptInvite = (token: string, body: { name: string; password: string }) =>
  request<{ user: SessionUser; webClient: boolean }>('POST', `/invite/${seg(token)}`, body)

/** The kind of server an import reads: Jellyfin (or a server that speaks its API), Emby or Plex. */
export type ServerKind = 'jellyfin' | 'emby' | 'plex'

/** A server as the import reads it; `address` is the normalized one to send back. */
export type JellyfinServer = { kind: ServerKind; name: string; version: string; address: string }

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
 * `create` (a new one). An existing user without `watchData` gets nothing. `jellyfinPassword`,
 * the user's password on the server, has the import sign in as them to read their watch data,
 * which a user's key or account cannot read of another user.
 */
export type JellyfinImportEntry = {
  jellyfinId: string
  userId?: string
  create?: NewUser
  watchData: boolean
  jellyfinPassword?: string
  /** A Plex Home profile's PIN, for an import the server's owner signed in to Plex for. */
  plexPin?: string
}

/**
 * Why an import, or one of its users, failed. `jellyfin_user_forbidden` fails that user only: the
 * server does not let the key read their data.
 */
export type JellyfinImportProblem =
  | 'jellyfin_unreachable'
  | 'jellyfin_key_refused'
  | 'not_jellyfin'
  | 'internal'
  | 'jellyfin_user_forbidden'

/** A title of a Jellyfin user's watch data that no Polyfin title matches. */
export type JellyfinUnmatched = {
  name: string
  type: 'movie' | 'episode' | 'series'
  year: number | null
  /** For an episode: its series, season and number, when Jellyfin knows them. */
  series: string | null
  season: number | null
  episode: number | null
  /**
   * `no_identifier`: no IMDb, TMDB or TVDB identifier; `not_found`: no Polyfin title has it;
   * `not_in_library`: a Plex history names it, but no library section lists it any longer;
   * `other_episode_order`: Plex numbers the episode in another of TheTVDB's episode orders.
   */
  reason: 'no_identifier' | 'not_found' | 'not_in_library' | 'other_episode_order'
}

/** How the import of one Jellyfin user's watch data goes. */
export type JellyfinUserImport = {
  jellyfinId: string
  jellyfinName: string
  userId: string
  userName: string
  /** A finished import's `waiting` users were not reached: it stopped or failed first. */
  state: 'waiting' | 'reading' | 'saving' | 'done' | 'failed'
  /** Items read from Jellyfin so far, with watch data. */
  read: number
  /**
   * What was read of a Plex server: its library sections of movies and shows, the items they list,
   * and the rows of the account's playback history; null for Jellyfin and Emby.
   */
  sections: number | null
  listed: number | null
  history: number | null
  /** Items read that a Polyfin title matched. */
  matched: number
  /**
   * Titles newly marked played, resume points set, titles newly marked favorite, titles rated, and
   * titles a Plex Watchlist added to the user's.
   */
  played: number
  resumed: number
  favorites: number
  ratings: number
  watchlist: number
  /** Marks, resume points and favorites Polyfin already had. */
  upToDate: number
  /** Resume points without a date, which are not added. */
  undated: number
  /** Every title not matched; `unmatched` lists the first 500 only. */
  unmatchedCount: number
  unmatched: JellyfinUnmatched[]
  problem: JellyfinImportProblem | null
  /** Why a user's import that ended without a problem added nothing; null otherwise. */
  nothingAdded: JellyfinNothingAdded | null
}

/**
 * Why a user's import added nothing: everything read was already in Polyfin; Jellyfin or Emby
 * lists nothing played, in progress or favorite for the user; the Plex server has no movie or show
 * library; the Plex owner, or the account signed in to Plex, has nothing played, in progress or
 * rated; Plex lists no play of the account.
 */
export type JellyfinNothingAdded =
  | 'up_to_date'
  | 'nothing_played'
  | 'no_sections'
  | 'plex_owner_nothing_watched'
  | 'plex_nothing_watched'
  | 'plex_no_history'

/**
 * The running import, else the last one since Polyfin started: a restart forgets it. `startedBy`
 * is the Polyfin user who started it; `own` tells an import of their own watch data, made under My
 * account.
 */
export type JellyfinImportStatus = {
  id: string
  server: { kind: ServerKind; name: string; address: string }
  startedBy: { id: string; name: string }
  own: boolean
  state: 'running' | 'done' | 'stopped' | 'failed'
  problem: JellyfinImportProblem | null
  startedAt: string
  endedAt: string | null
  users: JellyfinUserImport[]
}

/**
 * What connecting to a server reads: the server, its users, and the user the key belongs to or
 * who signed in, null for an API key of the server's dashboard or Plex's owner's token: another
 * user's watch data is then read signed in as them.
 */
export type JellyfinConnection = {
  server: JellyfinServer
  users: JellyfinUser[]
  keyOwner: string | null
}

/**
 * What connects to a server: an API key (Plex's owner's token for Plex), or a user's name and
 * password, which Plex does not take.
 */
export type JellyfinCredentials =
  { apiKey: string } | { account: { name: string; password: string } }

/** Reads a server's users with credentials, which the server never keeps. */
export const connectJellyfin = (
  body: { kind: ServerKind; address: string } & JellyfinCredentials,
) => request<JellyfinConnection>('POST', '/jellyfin-import/users', body)

/**
 * Creates the new users (all or none), then starts importing the watch data in the background;
 * `import` is null when no entry asked for watch data. `plexOwner` reads each account of a Plex
 * server with the token Plex gives its owner, whom the administrator signed in to Plex as.
 */
export const startJellyfinImport = (
  body: {
    kind: ServerKind
    address: string
    plexOwner?: boolean
    users: JellyfinImportEntry[]
  } & JellyfinCredentials,
) =>
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

/**
 * Whether the signed-in user may import their own watch history from another server, whether they
 * may import a Letterboxd export and the server's metadata addons search movies (without one, no
 * movie of an export is found), and their latest import, from a server or Letterboxd, running or
 * ended, else null.
 */
export type OwnImport = {
  enabled: boolean
  letterboxd: boolean
  searchable: boolean
  import: JellyfinImportStatus | LetterboxdImportStatus | null
}

export const fetchOwnImport = (signal?: AbortSignal) =>
  request<OwnImport>('GET', '/account/server-import', undefined, signal)

/**
 * Imports the signed-in user's own watch data in the background: from a Jellyfin or Emby server,
 * signing in there as `name`, with a password used for this import only; or from the Plex server
 * `server` their sign-in to Plex reaches, with its Watchlist unless `watchlist` is false.
 */
export const startOwnImport = async (
  body:
    | { kind: Exclude<ServerKind, 'plex'>; address: string; name: string; password: string }
    | { kind: 'plex'; server: string; watchlist: boolean },
) =>
  (await request<{ import: JellyfinImportStatus }>('POST', '/account/server-import', body)).import

/** Where the user signs in to Plex: Plex's own sign-in page, until `expiresAt`. */
export type PlexSignIn = { link: string; expiresAt: string }

/**
 * A Plex account signed in: its name, its Plex Home's profiles (empty outside one), the profile
 * read, and the servers that profile reaches, by machine identifier; `preselected` is the server
 * administrators import from.
 */
export type PlexAccount = {
  name: string
  profiles: { id: string; name: string; managed: boolean; protected: boolean }[]
  profile: string
  servers: { id: string; name: string; owned: boolean; preselected: boolean }[]
}

/**
 * Whose sign-in to Plex: a user's own, under My account, or the server owner's, in the
 * administrators' import.
 */
export type PlexSignInFor = 'own' | 'owner'

const plexBases: Record<PlexSignInFor, string> = {
  own: '/account/server-import/plex',
  owner: '/jellyfin-import/plex',
}

/** Starts the user's sign-in to Plex, dropping the one held before. */
export const startPlexSignIn = (of: PlexSignInFor) =>
  request<PlexSignIn>('POST', `${plexBases[of]}/sign-in`)

/** The account once Plex signed the user in, else pending. */
export const pollPlexSignIn = (of: PlexSignInFor) =>
  request<PlexAccount | { pending: true }>('POST', `${plexBases[of]}/sign-in/poll`)

/** Reads the profile of the Plex Home chosen, with its PIN when it has one. */
export const choosePlexProfile = (profile: string, pin: string) =>
  request<PlexAccount>('POST', `${plexBases.own}/profile`, { profile, pin })

/** Drops the user's sign-in to Plex, signing it out. */
export const dropPlexSignIn = (of: PlexSignInFor) =>
  request<void>('DELETE', `${plexBases[of]}/sign-in`)

/**
 * What an administrator's import reads of a Plex account once the owner signed in to Plex: all its
 * watch data, all of it once its Plex Home profile's PIN is given, or its plays on the server only.
 */
export type PlexAccess = 'all' | 'pin' | 'plays'

/** What the import reads of each account of the Plex server but the owner, by its identifier. */
export const fetchPlexOwnerAccess = async (
  body: { kind: 'plex'; address: string } & JellyfinCredentials,
) =>
  (
    await request<{ accounts: Record<string, PlexAccess> }>(
      'POST',
      `${plexBases.owner}/owner`,
      body,
    )
  ).accounts

/** Why a movie of a Letterboxd export was not found. */
export type LetterboxdReason = 'not_found' | 'ambiguous' | 'search_failed'

/** A movie of a Letterboxd export no title matched, with its Letterboxd link when it has one. */
export type LetterboxdUnmatched = {
  name: string
  year: number | null
  link: string | null
  reason: LetterboxdReason
}

/**
 * An import of a Letterboxd export into a user's account, running or the last one since Polyfin
 * started: the switches it was started with, how many of its movies were looked up, then what it
 * added, and the movies not found (the first 500 in `unmatched`).
 */
export type LetterboxdImportStatus = {
  id: string
  kind: 'letterboxd'
  user: { id: string; name: string }
  startedBy: { id: string; name: string }
  watchlist: boolean
  ratings: boolean
  likesAsFavorites: boolean
  state: 'running' | 'done' | 'failed'
  problem: 'internal' | null
  startedAt: string
  endedAt: string | null
  movies: number
  searched: number
  added: { played: number; watchlist: number; ratings: number; favorites: number }
  unmatchedCount: number
  unmatched: LetterboxdUnmatched[]
}

/** Whether an import of the user's account is one of a Letterboxd export. */
export const isLetterboxdImport = (
  current: JellyfinImportStatus | LetterboxdImportStatus,
): current is LetterboxdImportStatus => 'kind' in current && current.kind === 'letterboxd'

/** What a Letterboxd import takes: the export's ZIP, and what to import besides the movies watched. */
export type LetterboxdUpload = {
  file: File
  watchlist: boolean
  ratings: boolean
  likesAsFavorites: boolean
}

/** The largest export the server takes, in bytes. */
export const maxLetterboxdExportBytes = 20 * 1024 * 1024

/** Sends a Letterboxd export as a multipart form to `path`, which imports it in the background. */
async function uploadLetterboxd(path: string, upload: LetterboxdUpload) {
  const form = new FormData()
  form.append('kind', 'letterboxd')
  form.append('watchlist', String(upload.watchlist))
  form.append('ratings', String(upload.ratings))
  form.append('likesAsFavorites', String(upload.likesAsFavorites))
  form.append('file', upload.file)
  // The browser writes the multipart Content-Type, with its boundary.
  const response = await fetch(`${apiBase}${path}`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    body: form,
  })
  if (!response.ok) throw await apiError(response)
  // The server answers the import started, as for the other kinds.
  const answer: { import: LetterboxdImportStatus } = await response.json()
  return answer.import
}

/** Imports a Letterboxd export into the signed-in user's account. */
export const startOwnLetterboxdImport = (upload: LetterboxdUpload) =>
  uploadLetterboxd('/account/server-import', upload)

/**
 * For an administrator: whether the server's metadata addons search movies, and the Letterboxd
 * import into a user's account, running or the last one, else null.
 */
export type UserLetterboxdImport = { searchable: boolean; import: LetterboxdImportStatus | null }

export const fetchUserLetterboxdImport = (id: string, signal?: AbortSignal) =>
  request<UserLetterboxdImport>('GET', `/users/${seg(id)}/server-import`, undefined, signal)

/** Imports a Letterboxd export into a user's account, for an administrator. */
export const startUserLetterboxdImport = (id: string, upload: LetterboxdUpload) =>
  uploadLetterboxd(`/users/${seg(id)}/server-import`, upload)

/** An app whose accounts Polyfin imports. */
export type AppKind = 'nuvio' | 'stremio'

/** An addon a profile of an app offers to install, by its index; its address is redacted. */
export type AppAddon = { index: number; name: string; address: string }

/**
 * A profile of an app account: the titles it holds, how many it played, resumed and keeps in its
 * library, and its addons. `name` is empty when the app gives none.
 */
export type AppProfile = {
  id: string
  name: string
  /** Asks for its PIN before it is read (`AppReaders.stremio.unlock`): it holds nothing until then. */
  locked: boolean
  entries: number
  played: number
  resumes: number
  library: number
  addons: AppAddon[]
}

/**
 * An app account read: the server holds it a few minutes under `key`, while a profile is chosen.
 * `stayConnected` tells it was read to stay connected: importing it keeps the connection.
 */
export type AppAccount = {
  key: string
  app: AppKind
  stayConnected: boolean
  profiles: AppProfile[]
}

/**
 * A title of the app no Polyfin title matched, by its identifier in the app, or a series whose
 * episodes played could not be read (`episode_list`).
 */
export type AppUnmatched = {
  id: string
  name: string
  type: 'movie' | 'series' | 'episode'
  season: number | null
  episode: number | null
  reason: 'other_identifier' | 'not_found' | 'episode_list'
}

/** An addon an import installs as the user's own, and why it was not. */
export type AppAddonInstalled = {
  name: string
  address: string
  installed: boolean
  problem:
    | 'not_allowed'
    | 'exists'
    | 'invalid_address'
    | 'private_network'
    | 'unreachable'
    | 'invalid_manifest'
    | 'internal'
    | null
}

/** An import from an app into a user's account, running or the last one since Polyfin started. */
export type AppImportStatus = {
  id: string
  app: AppKind
  profile: string
  user: { id: string; name: string }
  startedBy: { id: string; name: string }
  state: 'running' | 'done' | 'failed'
  problem: 'internal' | null
  startedAt: string
  endedAt: string | null
  /**
   * Titles of the profile; then titles newly marked played, resume points set, and titles of the
   * app's library newly added to the Watchlist view.
   */
  read: number
  played: number
  resumed: number
  watchlist: number
  /** Every title not matched; `unmatched` lists the first 500 only. */
  unmatchedCount: number
  unmatched: AppUnmatched[]
  addons: AppAddonInstalled[]
}

/**
 * What signs in to a Nuvio account; the password is used for this sign-in only. With
 * `stayConnected`, the server keeps the session, never the password.
 */
export type NuvioSignIn = {
  backend: string
  email: string
  password: string
  stayConnected: boolean
}

/** A Stremio link code: the user opens `link` where they are signed in to Stremio, and approves `code`. */
export type StremioLink = { code: string; link: string }

/** A Stremio link code polled: the account read once the code is approved, else pending. */
export type StremioLinkPoll = AppAccount | { pending: true }

/**
 * What signs in to a Stremio account; the password is used for this sign-in only. With
 * `stayConnected`, the server keeps the auth key the sign-in gives, never the password.
 */
export type StremioSignIn = { email: string; password: string; stayConnected: boolean }

/** A profile of a Stremio account held under `key`, and the PIN that opens it. */
export type StremioUnlock = { key: string; profile: string; pin: string }

/**
 * How each app's account is read, for the Polyfin account it is imported into. Passwords are used
 * for the sign-in only; the auth keys and sessions apps give are kept by the server, never answered,
 * when `stayConnected` asks for it.
 */
export type AppReaders = {
  nuvio: (body: NuvioSignIn) => Promise<AppAccount>
  stremio: {
    startLink: () => Promise<StremioLink>
    pollLink: (stayConnected: boolean) => Promise<StremioLinkPoll>
    password: (body: StremioSignIn) => Promise<AppAccount>
    key: (key: string, stayConnected: boolean) => Promise<AppAccount>
    /**
     * Opens, with its PIN, a profile of the account held under `key` that asks for it, and answers
     * the account with that profile read. The PIN goes to Stremio only, and is never kept.
     */
    unlock: (body: StremioUnlock) => Promise<AppAccount>
  }
}

/** The readers of the app import routes under `base`. */
const appReaders = (base: string): AppReaders => ({
  nuvio: (body) => request<AppAccount>('POST', `${base}/nuvio`, body),
  stremio: {
    startLink: () => request<StremioLink>('POST', `${base}/stremio/link`),
    pollLink: (stayConnected) =>
      request<StremioLinkPoll>('POST', `${base}/stremio/link/poll`, { stayConnected }),
    password: (body) => request<AppAccount>('POST', `${base}/stremio/password`, body),
    key: (key, stayConnected) =>
      request<AppAccount>('POST', `${base}/stremio/key`, { key, stayConnected }),
    unlock: (body) => request<AppAccount>('POST', `${base}/stremio/unlock`, body),
  },
})

/**
 * How an import of a connection went: when it ended, what it read and added, the titles it did not
 * find, and what kept it from reading the account.
 */
export type AppConnectionImport = {
  at: string
  read: number
  played: number
  resumed: number
  watchlist: number
  unmatched: number
  problem:
    | 'unreachable'
    | 'rate_limited'
    | 'unexpected'
    | 'profile_missing'
    | 'profile_locked'
    | 'sign_in_refused'
    | 'internal'
    | null
}

/**
 * A user's connection to an app, imported again at the server's interval: the account as the app
 * names it, who connected it, `problem` once the app refused the sign-in kept (the user signs in
 * again), `paused` while the server does not let the user import, the last import and when the
 * next one runs.
 */
export type AppConnection = {
  app: AppKind
  account: string | null
  connectedAt: string
  connectedBy: { id: string; name: string } | null
  problem: 'sign_in_refused' | null
  paused: boolean
  importing: boolean
  lastImport: AppConnectionImport | null
  nextImportAt: string | null
}

/** Imports a connection at once, or disconnects it, for the account the routes under `base` name. */
export type AppConnectionActions = {
  importNow: (app: AppKind) => Promise<AppConnection>
  disconnect: (app: AppKind) => Promise<void>
}

const appConnectionActions = (base: string): AppConnectionActions => ({
  importNow: async (app) =>
    (
      await request<{ connection: AppConnection }>(
        'POST',
        `${base}/app-connections/${seg(app)}/import`,
      )
    ).connection,
  disconnect: (app) => request<void>('DELETE', `${base}/app-connections/${seg(app)}`),
})

/** The profile to import of an account read, and the indexes of its addons to install. */
export type AppImportChoice = { key: string; profile: string; addons: number[] }

/**
 * Whether the signed-in user may import from an app, whether they may add their own addons, the
 * import into their account running, else their last one, else null, their connections, and how
 * often connections are imported, in minutes.
 */
export type OwnAppImport = {
  enabled: boolean
  addonsAllowed: boolean
  import: AppImportStatus | null
  connections: AppConnection[]
  importEveryMinutes: number
}

export const fetchOwnAppImport = (signal?: AbortSignal) =>
  request<OwnAppImport>('GET', '/account/app-import', undefined, signal)

/** Reads the user's own app accounts: each signs in, reads the account, and signs out. */
export const ownAppReaders = appReaders('/account/app-import')

/** The user's own connections to apps. */
export const ownAppConnections = appConnectionActions('/account')

/** Imports the chosen profile of the account read into the user's account, in the background. */
export const startOwnAppImport = async (body: AppImportChoice) =>
  (await request<{ import: AppImportStatus }>('POST', '/account/app-import', body)).import

/**
 * For an administrator: whether a user may add their own addons, the import into their account,
 * their connections, and how often connections are imported, in minutes.
 */
export type UserAppImport = {
  addonsAllowed: boolean
  import: AppImportStatus | null
  connections: AppConnection[]
  importEveryMinutes: number
}

export const fetchUserAppImport = (id: string, signal?: AbortSignal) =>
  request<UserAppImport>('GET', `/users/${seg(id)}/app-import`, undefined, signal)

/** Reads app accounts for an administrator, to import into the user's account. */
export const userAppReaders = (id: string) => appReaders(`/users/${seg(id)}/app-import`)

/** A user's connections to apps, for an administrator. */
export const userAppConnections = (id: string) => appConnectionActions(`/users/${seg(id)}`)

export const startUserAppImport = async (id: string, body: AppImportChoice) =>
  (await request<{ import: AppImportStatus }>('POST', `/users/${seg(id)}/app-import`, body)).import

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
  | 'publicMetaDbKey'
  | 'theIntroDbKey'
  | 'mdblistRatingsKey'
  | 'tmdbKey'
  | 'traktClientSecret'
  | 'lastFmSecret'
  | 'aniListClientSecret'
  | 'myAnimeListClientSecret'
  | 'metricsToken'
  | 'updateToolToken'

/** Reads one of the server's saved secrets again, for an administrator; never cached. */
export const revealServerSecret = async (name: ServerSecretName) =>
  (await request<{ value: string }>('POST', `/settings/secrets/${seg(name)}/reveal`)).value

/** Makes a new metrics token, which ends the former one; answers the settings. */
export const newMetricsToken = () => request<Settings>('POST', '/settings/metrics-token')

/** The web player's looks: the bundled LumaaGlaass, jellyfin-web's own, or the Custom texts. */
export const webThemes = ['lumaaglaass', 'none', 'custom'] as const
export type WebTheme = (typeof webThemes)[number]
/** A user's web player theme: one of the server's themes, or '' for the server's own. */
export type UserWebTheme = WebTheme | ''

/** The server's theme, which users without their own get, and the themes a user may be given. */
export type WebThemes = { serverTheme: WebTheme; themes: WebTheme[] }

/** A file of the copy Polyfin keeps of a theme's files. */
export type ThemeCopyFile = {
  /** The address it came from. */
  address: string
  /** How the Custom texts named it, for the files they name. */
  text?: string
  name: string
  kind: 'stylesheet' | 'script' | 'font' | 'image'
  /** Size in bytes. */
  size: number
}

/** Why an address could not be saved in the copy. */
export type ThemeCopyReason =
  | 'private_address'
  | 'not_http'
  | 'unavailable'
  | 'timeout'
  | 'wrong_type'
  | 'too_large'
  | 'total_too_large'
  | 'too_many_files'
  | 'too_deep'
  | 'built_at_run_time'

/** An address the copy could not save; `detail` is an HTTP status or the type a file came as. */
export type ThemeCopyUnsaved = { address: string; reason: ThemeCopyReason; detail?: string }

/** The copy Polyfin keeps of the files the Custom texts load from elsewhere. */
export type ThemeCopy = {
  /** The copy kept, null for none. */
  copy: {
    savedAt: string
    /** Size of its files, in bytes. */
    size: number
    files: ThemeCopyFile[]
    unsaved: ThemeCopyUnsaved[]
  } | null
  /** Names of the copy's files missing from its folder, such as after a restore elsewhere. */
  missing: string[]
  /** Addresses the Custom texts still load from elsewhere. */
  outside: string[]
  /** The limits of a copy: bytes of a file and in all, files, and levels followed. */
  limits: { fileBytes: number; totalBytes: number; files: number; depth: number }
}

export const fetchThemeCopy = (signal?: AbortSignal) =>
  request<ThemeCopy>('GET', '/settings/theme-copy', undefined, signal)
/** Keeps a copy of the theme's files, or makes the copy again from where its files came from. */
export const keepThemeCopy = () => request<ThemeCopy>('POST', '/settings/theme-copy')
/** Removes the copy: the Custom texts name again the addresses its files came from. */
export const removeThemeCopy = () => request<ThemeCopy>('DELETE', '/settings/theme-copy')

/** The apps users are signed in with, by the name they send, each once whatever its case, sorted. */
export const fetchSettingsApps = async (signal?: AbortSignal) =>
  (await request<{ apps: string[] }>('GET', '/settings/apps', undefined, signal)).apps

/** The languages the server's addons were given, which users may choose as metadata language. */
export const fetchMetadataLanguages = async (signal?: AbortSignal) =>
  (await request<{ languages: string[] }>('GET', '/metadata-languages', undefined, signal))
    .languages

/** The web player themes users choose among: Custom while it has a text. */
export const fetchWebThemes = (signal?: AbortSignal) =>
  request<WebThemes>('GET', '/web-themes', undefined, signal)

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

/** Asks the local network for HDHomeRun tuners; administrators only. Takes a few seconds. */
export const findTuners = (signal?: AbortSignal) =>
  request<{ items: FoundTuner[] }>('GET', '/tuners', undefined, signal)

/** Has a tuner of the server scan for channels; its line-up is read again once the scan ends. */
export const scanTuner = (id: string) =>
  request<Addon>('POST', `${scopePath('shared')}/iptv/${seg(id)}/scan`)

/** Adds a local folder to the server's sources; it is scanned at once, in the background. */
export const addLocalFolder = (folder: NewLocalFolder) =>
  request<Addon>('POST', `${scopePath('shared')}/folders`, folder)

export const updateLocalFolder = (id: string, patch: LocalFolderPatch) =>
  request<Addon>('PATCH', `${scopePath('shared')}/folders/${seg(id)}`, patch)

/**
 * The remotes of the rclone instance at an address, signed in as user with password; a password
 * left out is the one stored for the folder named. `error` tells why rclone could not list them:
 * 'unreachable' or 'refused'.
 */
export const fetchRcloneRemotes = (query: {
  address: string
  user: string
  password?: string
  folder?: string
}) =>
  request<{ remotes: string[]; error: '' | 'unreachable' | 'refused' }>(
    'POST',
    `${scopePath('shared')}/folders/rclone-remotes`,
    query,
  )

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

/**
 * A unit of a local folder a looser rule matched: a file, a show's folder or a disc's folder, its
 * first file and how many it has, what its name was read as, and the title it was matched to.
 */
export type LooserMatch = {
  unit: string
  path: string
  files: number
  title: string
  year: number | null
  imdbId: string
  name: string
  titleYear: number | null
  rule: LooserRule
}

export const fetchLooserMatches = (id: string, signal?: AbortSignal) =>
  request<{ total: number; units: LooserMatch[] }>(
    'GET',
    `${scopePath('shared')}/folders/${seg(id)}/looser`,
    undefined,
    signal,
  )

/**
 * A title the metadata addons found for a local folder's unit, that a link can name: `posterId` is
 * the item whose Primary image is its poster, null without one.
 */
export type FolderTitle = {
  imdbId: string
  name: string
  year: number | null
  kind: 'movie' | 'show'
  posterId: string | null
}

/**
 * Searches the metadata addons for titles of a local folder's kind, those of `year` first, at most
 * 10; only titles with an IMDb identifier are answered.
 */
export const searchFolderTitles = (
  id: string,
  query: string,
  year: number | null,
  signal?: AbortSignal,
) =>
  request<{ titles: FolderTitle[] }>(
    'GET',
    `${scopePath('shared')}/folders/${seg(id)}/titles?query=${encodeURIComponent(query)}${year === null ? '' : `&year=${year}`}`,
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

/**
 * Gives titles of shows' discs their episodes, such as `S01E05` or `S01E05-E06`, for good; an
 * empty episode removes those given.
 */
export const setLocalEpisodes = (id: string, episodes: { path: string; episode: string }[]) =>
  request<Addon>('PUT', `${scopePath('shared')}/folders/${seg(id)}/episodes`, { episodes })

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

/** The scope's smart lists, in the order they were made. */
export const fetchSmartLists = (scope: Scope, signal?: AbortSignal) =>
  request<SmartList[]>('GET', `${scopePath(scope)}/smart-lists`, undefined, signal)

export const fetchSmartList = (scope: Scope, id: string, signal?: AbortSignal) =>
  request<SmartList>('GET', `${scopePath(scope)}/smart-lists/${seg(id)}`, undefined, signal)

/** The catalogs the scope's smart lists may draw from, and the languages their rules may name. */
export const fetchSmartChoices = (scope: Scope, signal?: AbortSignal) =>
  request<SmartChoices>('GET', `${scopePath(scope)}/smart-lists/catalogs`, undefined, signal)

export const createSmartList = (scope: Scope, list: SmartListDraft) =>
  request<SmartList>('POST', `${scopePath(scope)}/smart-lists`, list)

export const saveSmartList = (scope: Scope, id: string, list: SmartListDraft) =>
  request<SmartList>('PUT', `${scopePath(scope)}/smart-lists/${seg(id)}`, list)

export const deleteSmartList = (scope: Scope, id: string) =>
  request<void>('DELETE', `${scopePath(scope)}/smart-lists/${seg(id)}`)

/** The titles a smart list, saved or not, lists for the signed-in user now. */
export const previewSmartList = (scope: Scope, list: SmartListDraft, signal?: AbortSignal) =>
  request<SmartPreview>('POST', `${scopePath(scope)}/smart-lists/preview`, list, signal)

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

/**
 * What set the size of converted video: nothing below the source's, the GPU's 4K, the
 * processor's 1080p, HDR converted by the processor, the user's quality group, the settings'
 * maximum quality of converted video, the bitrate limit, or what the app's profile allows.
 */
export type SizeLimit =
  'source' | 'gpu' | 'processor' | 'toneMapping' | 'qualityGroup' | 'serverMax' | 'bitrate' | 'app'

/** A device playing, as the dashboard shows it. */
export type LiveSession = {
  id: string
  user: { id: string; name: string; imageTag: string | null; qualityGroup: number }
  device: { name: string; app: string; appVersion: string; address: string }
  /** Whether the app takes commands: stopping and messages. */
  controllable: boolean
  /**
   * A guest playing through a share link: it shows under the sharer, with "Shared link" as its
   * app, and takes no command.
   */
  shared: boolean
  /**
   * A converted download: its position is how far the file is made, and it takes no command. Its
   * device is empty when the download named none.
   */
  download: boolean
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
    /** GPU method (cuda, vaapi, videotoolbox, qsv, amf); empty for the CPU. */
    hardware: string
    width: number
    height: number
    bitrate: number
    toneMap: boolean
    burnSubtitles: boolean
    /** What set the size; empty when not known. */
    limitedBy: SizeLimit | ''
    /** The height that capped the size, 0 for `source`. */
    maxHeight: number
    /** For `bitrate`: the limit in bits per second, and whose it is. */
    bitrateLimit: number
    bitrateLimitOf: 'app' | 'user' | ''
    /** Heights of the qualities the app may switch between, first the one chosen; empty for one. */
    qualities: number[]
    /** The first quality's height when the app stepped down to a lower one, else 0. */
    steppedDownFrom: number
  } | null
  audio: { codec: string; channels: number; bitrate: number } | null
  /** The version the play went on from once its version's source failed; empty when it did not. */
  switchedVersion: string
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
  /** A tuner's channel: the timers recording at the same time need more tuners than it has. */
  tunersShort: boolean
}

/** The recordings converted to HEVC, and the bytes their conversion saved. */
export type ConvertedRecordings = { recordings: number; saved: number }

export const fetchTimers = (signal?: AbortSignal) =>
  request<{ available: boolean; timers: Timer[]; converted?: ConvertedRecordings }>(
    'GET',
    '/timers',
    undefined,
    signal,
  )

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

/**
 * How a network share is watched for changes: `watch` is `notified` when its server tells of
 * them, `polled` when its folders are listed again every few minutes, with why in `polled`
 * (`webdav`, `unsupported`, `silent`, `failed`, empty while the watch is set up); empty when it
 * is not watched, with why in `unwatched` when it should be.
 */
export type ShareHealth = {
  id: string
  name: string
  share: Exclude<FolderShare, ''>
  watch: '' | 'notified' | 'polled'
  polled: string
  unwatched: string
  /** Minutes between the listings of an S3 bucket, SFTP folder or rclone remote; 0 only at scans. */
  pollMinutes: number | null
}

/** The folders the settings choose for conversion, files being read and the cache. */
export type WorkFolderName = 'conversion' | 'reading' | 'cache'

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
  /** `torrents` is the part of `used` the pieces of torrents being played take. */
  cache: { used: number; limit: number; sources: number; torrents: number } | null
  /** The image cache: images and bytes on disk against its limit, and in memory; null when unknown. */
  images: ImageCacheUsage | null
  /**
   * How torrent streams play, null while they are off: their route ('' for none) and its name,
   * the WireGuard server for a tunnel, the torrents playing, their peers, those of them reached
   * over uTP, and the bytes shared since the server started. Over a tunnel, `addresses` are its
   * addresses and `lastHandshake` when its last handshake was, absent for none yet.
   */
  torrents: {
    route: 'interface' | 'proxy' | 'wireguard' | ''
    name: string
    addresses?: string[]
    lastHandshake?: string
    playing: number
    /** The bound on `playing`, Torrents at once, 0 for none. */
    atOnce: number
    peers: number
    utpPeers: number
    uploaded: number
  } | null
  disks: {
    folder: WorkFolderName | 'recordings' | 'backups'
    path: string
    free: number
    used: number
    mount: string
    /** Whether the disk is short of room, which Health shows as a problem. */
    low: boolean
  }[]
  transcoder: {
    hardware: {
      method: string
      device: string
      encoders: string[]
      toneMapping: boolean
      olderDrivers: boolean
    } | null
    encoders: string[]
    conversions: number
    limit: number
    remuxes: number
    maxHeight: number
    enabled: boolean
    olderDriverFFmpeg: OlderDriverFFmpeg | null
  } | null
  thumbnails: {
    enabled: boolean
    waiting: number
    queueLength: number
    working: boolean
    pausedHosts: { host: string; until: string }[]
  } | null
  addons: AddonHealth[]
  /** How the enabled network shares are watched for changes; `watchFolders` tells whether they are. */
  shares: ShareHealth[]
  watchFolders: boolean
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
  /**
   * The new version of Polyfin the daily check found, with its release notes; null when there is
   * none, or while Check for new versions is off.
   */
  update: { version: string; url: string } | null
  /**
   * The host names of Known proxies, with the addresses each resolved to when last looked up,
   * sorted: none when it resolves to nothing, which trusts nothing.
   */
  proxyNames: { name: string; addresses: string[]; resolvedAt: string }[]
  /**
   * How the lookups of titles' ratings on MDBList stand, null while no key is saved: the requests
   * MDBList allows a day and those left until `resetsAt`, null until MDBList tells them; until when
   * it is left alone, and why; the titles waiting to be asked about, and those with ratings kept.
   */
  ratings: {
    limit: number | null
    remaining: number | null
    resetsAt: string | null
    heldUntil: string | null
    held: '' | 'limit' | 'refused' | 'failed'
    waiting: number
    kept: number
  } | null
}

/**
 * A stored secret POLYFIN_SECRET_KEY cannot decrypt: a server setting (the SMTP password is one,
 * though it is never read again), a user's tracking connection, a user's connection to an app, or a
 * notification target, by its name, of a user or of the server (`user` null).
 */
export type UnreadableSecret = {
  setting:
    | ServerSecretName
    | 'smtpPassword'
    | 'torrentProxyPassword'
    | 'torrentWireGuard'
    | 'backupPassphrase'
    | null
  service: TrackingServiceName | null
  app: AppKind | null
  target: string | null
  user: string | null
  /** A network share's password, by its folder's name. */
  folder: string | null
  /** A backup destination's password or key, by what the destination shows. */
  destination: string | null
  /** The account key or private key of the certificate of Let's Encrypt. */
  certificate: boolean
  /** The two-step sign-in of `user`. */
  twoStep: boolean
}

/**
 * The peers a torrent that did not start found: those connected, and the others by how their last
 * dial or connection ended, each count left out at 0.
 */
export type TorrentPeerCounts = {
  found: number
  connected: number
  timedOut?: number
  refused?: number
  unreachable?: number
  proxy?: number
  closed?: number
  left?: number
  other?: number
  untried?: number
}

/**
 * Something Health shows as needing attention, as the server finds it: `key` names it while it
 * lasts, `code` tells what it is, with its details; `to` is the page that describes it, with its
 * anchor. `transient` ones are not told as health problems: they come and go with the load, or
 * their own event told them. An addon, source or guide has an `owner` when it is a user's own; a
 * stopped connection has its user as `owner`, its `kind` and its app or service as `name`.
 */
export type HealthProblem = {
  key: string
  tone: 'error' | 'warning'
  to: string
  transient: boolean
} & (
  | { code: 'database' }
  | { code: 'disk'; folder: WorkFolderName | 'recordings' | 'backups'; free: number }
  | { code: 'addon'; name: string; owner?: NonNullable<Owner>; failure: string }
  /** Searches leave out the addon's search catalog for a few minutes: it did not answer in time. */
  | { code: 'search_late'; name: string; owner?: NonNullable<Owner> }
  | { code: 'conversions_full' }
  | { code: 'thumbnails_paused'; host: string }
  | { code: 'unknown_proxy'; address: string }
  | { code: 'discovery_port'; port: number }
  | { code: 'discovery_address'; address: string }
  | { code: 'dlna_port'; port: number }
  /**
   * Torrent streams do not play: their route cannot carry them, `failure` tells why, `name` names
   * the interface or the proxy; or the proxy offers no UDP ASSOCIATE.
   */
  | {
      code: 'torrent_route'
      failure:
        | 'interface_missing'
        | 'interface_down'
        | 'interface_no_address'
        | 'interface_unsupported'
        | 'proxy_unreachable'
        | 'proxy_refused'
        | 'wireguard_endpoint'
        | 'wireguard_down'
        | 'wireguard_handshake'
        | 'no_route'
      name?: string
    }
  | { code: 'torrent_udp'; name: string }
  /**
   * A torrent did not start in time lately: `failure` tells why, and `peers`, when some were found,
   * why they did not connect.
   */
  | {
      code: 'torrent_start'
      failure: 'no_udp' | 'refused' | 'no_trackers' | 'no_peers' | 'silent'
      peers?: TorrentPeerCounts
    }
  | { code: 'proxy_name_unresolved'; name: string }
  | { code: 'secrets_unreadable'; secrets: UnreadableSecret[] }
  | { code: 'secrets_plaintext' }
  | { code: 'backup_failed' }
  | { code: 'backup_stale' }
  | { code: 'backup_destination_failed' | 'backup_destination_stale'; name: string }
  | { code: 'iptv' | 'guide'; name: string; owner?: NonNullable<Owner> }
  /** A tuner of the server: 'unreachable' when it does not answer, else a stream failure code. */
  | { code: 'tuner'; name: string; failure: string }
  | { code: 'folder'; name: string; failure: string; share?: Exclude<FolderShare, ''> }
  | { code: 'folder_unwatched'; name: string; failure: string; share?: Exclude<FolderShare, ''> }
  /** A local folder's searches failed: the addon that failed most of them ('' for another failure) and why. */
  | { code: 'folder_search'; name: string; addon?: string; failure: string }
  /** No addon of the server describes and searches the titles of a local folder's files. */
  | { code: 'folder_no_search'; name: string }
  | { code: 'task'; task: string }
  | { code: 'https_port'; port: number }
  | { code: 'https_files'; failure: 'missing' | 'unreadable' | 'mismatch'; file: string }
  | { code: 'https_renewal'; error: string }
  | { code: 'https_expiry'; expires: string }
  | { code: 'subtitle_ocr_language'; name: string; file: string }
  /** Every metadata addon of the server has a language: `name` is the first one's. */
  | { code: 'metadata_languages'; name: string }
  /** NVENC needs a newer NVIDIA driver than the GPU's: `name` is the version it needs. */
  | { code: 'nvidia_driver'; name: string }
  /** NVENC cannot use the FFmpeg for older NVIDIA drivers: not downloaded, or failing too. */
  | { code: 'nvidia_older_ffmpeg'; failure: 'download' | 'encode'; error: string }
  /** The copy kept of the Custom theme's files misses `missing` of its `files`. */
  | { code: 'theme_copy_missing'; missing: number; files: number }
  /**
   * A folder the settings chose could not be written into at start: `path` is the folder, `error`
   * why; the cache in the data folder keeps its files until the next start.
   */
  | { code: 'folder_unwritable'; folder: WorkFolderName; path: string; error: string }
  | { code: 'connection_stopped'; kind: 'app'; name: AppKind; owner: NonNullable<Owner> }
  | {
      code: 'connection_stopped'
      kind: 'tracking'
      name: TrackingServiceName
      owner: NonNullable<Owner>
    }
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
  /** Where the backups are sent besides the folder, with their last upload. */
  destinations: BackupDestination[]
  /** Why a backup cannot be restored, empty when it can. */
  restore: '' | 'pg_restore_missing'
  /** How the last restore went; `error` is empty after a success. */
  lastRestore: { at: string; file: string; administrator: string; error: string } | null
}

export const fetchBackup = (signal?: AbortSignal) =>
  request<Backup>('GET', '/backup', undefined, signal)

export type BackupDestinationKind = 's3' | 'sftp' | 'webdav'

/**
 * A place backups are sent to. Its password, secret key and private key are never sent:
 * `secretSet` and `privateKeySet` tell whether they are set. `address` is an S3 endpoint, an SFTP
 * server's host or a WebDAV folder's address; `folder` an S3 prefix or an SFTP folder. `hostKey` is
 * the fingerprint of the SFTP server's key saved at the first connection. `ranAt` and `error` are
 * the last upload, `newestFile` and `newestAt` the newest backup there.
 */
export type BackupDestination = {
  id: string
  kind: BackupDestinationKind
  name: string
  address: string
  port: number
  bucket: string
  folder: string
  region: string
  username: string
  secretSet: boolean
  privateKeySet: boolean
  hostKey: string
  kept: number
  createdAt: string
  ranAt: string | null
  error: string
  newestFile: string
  newestAt: string | null
  problem: 'failed' | 'stale' | null
  unreadable: boolean
}

/** A destination to add, or the fields to change: those left out keep their values. */
export type BackupDestinationDraft = {
  kind?: BackupDestinationKind
  address?: string
  port?: number
  bucket?: string
  folder?: string
  region?: string
  username?: string
  secret?: string
  privateKey?: string
  hostKey?: ''
  kept?: number
}

export const fetchBackupDestinations = (signal?: AbortSignal) =>
  request<{ destinations: BackupDestination[]; kinds: BackupDestinationKind[] }>(
    'GET',
    '/backup/destinations',
    undefined,
    signal,
  )

export const createBackupDestination = (draft: BackupDestinationDraft) =>
  request<BackupDestination>('POST', '/backup/destinations', draft)

export const updateBackupDestination = (id: string, draft: BackupDestinationDraft) =>
  request<BackupDestination>('PATCH', `/backup/destinations/${seg(id)}`, draft)

export const deleteBackupDestination = (id: string) =>
  request<void>('DELETE', `/backup/destinations/${seg(id)}`)

/** Writes a small file, reads it back and deletes it: `error` tells why it failed. */
export const testBackupDestination = (id: string) =>
  request<{ ok: boolean; error: string; destination: BackupDestination }>(
    'POST',
    `/backup/destinations/${seg(id)}/test`,
  )

/**
 * A backup that can be restored, `madeAt` read from its name; `size` is -1 when its destination
 * does not tell it; `encrypted` tells one encrypted with a passphrase.
 */
export type BackupFile = {
  name: string
  size: number
  madeAt: string
  beforeRestore: boolean
  encrypted: boolean
}

/** The backups of the folder and of each destination, newest first. */
export type BackupFiles = {
  folder: BackupFile[]
  destinations: { destination: BackupDestination; files: BackupFile[]; error: string }[]
}

export const fetchBackupFiles = (signal?: AbortSignal) =>
  request<BackupFiles>('GET', '/backup/files', undefined, signal)

/**
 * A backup checked to be restored: `unreadable` counts by kind the stored keys in it that
 * POLYFIN_SECRET_KEY cannot decrypt; `serverName` is what the confirmation asks to type. A
 * `locked` backup is encrypted and waits for its passphrase (see `unlockRestore`): it is checked
 * once opened, and `unreadable` is empty until then.
 */
export type RestorePlan = {
  id: string
  file: string
  destination: string | null
  uploaded: boolean
  madeAt: string | null
  size: number
  unreadable: Partial<
    Record<
      | 'settings'
      | 'tracking'
      | 'apps'
      | 'notifications'
      | 'shares'
      | 'destinations'
      | 'certificates'
      | 'twoStep',
      number
    >
  >
  encrypted: boolean
  locked: boolean
  serverName: string
}

/** Checks a backup of the folder (`destination` null) or of a destination, to restore it. */
export const prepareRestore = (destination: string | null, file: string) =>
  request<RestorePlan>('POST', '/backup/restore/prepare', { destination, file })

/** Sends a file as the body of a POST to `path`, and answers the plan the server checked. */
async function sendBackup(
  path: string,
  file: File,
  headers: Record<string, string> = {},
): Promise<RestorePlan> {
  const response = await fetch(`${apiBase}${path}?name=${seg(file.name)}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/octet-stream', ...headers },
    body: file,
  })
  if (!response.ok) throw await apiError(response)
  return (await response.json()) as RestorePlan
}

/** Sends a backup file and checks it, to restore it. */
export const uploadRestore = (file: File) => sendBackup('/backup/restore/upload', file)

/** Opens the encrypted backup prepared with its passphrase, then checks it. */
export const unlockRestore = (id: string, passphrase: string) =>
  request<RestorePlan>('POST', '/backup/restore/unlock', { id, passphrase })

/** Restores the backup checked: Polyfin stops, restores it, and starts again. */
export const startRestore = (id: string, serverName: string, anyway: boolean) =>
  request<{ restoring: boolean }>('POST', '/backup/restore', { id, serverName, anyway })

/** Drops a backup checked and not restored, with the file downloaded or sent for it. */
export const discardRestore = (id: string) =>
  request<void>('POST', '/backup/restore/discard', { id })

/** The address that downloads a backup of the folder as it is. */
export const backupDownloadUrl = (name: string) => `${apiBase}/backup/files/${seg(name)}`

/**
 * The address that downloads the dump an encrypted backup of the folder holds, with the ticket
 * `checkBackupPassphrase` gave: a plain link, which the browser saves as it comes.
 */
export const backupDecryptedUrl = (name: string, ticket: string) =>
  `${apiBase}/backup/files/${seg(name)}/decrypted?ticket=${seg(ticket)}`

/**
 * Checks that a passphrase opens an encrypted backup of the folder, and answers the ticket that
 * downloads it decrypted, once, within a minute.
 */
export const checkBackupPassphrase = (name: string, passphrase: string) =>
  request<{ ticket: string }>('POST', `/backup/files/${seg(name)}/check`, { passphrase })

/**
 * Sends a backup from the setup page, before any administrator exists, with the setup code; then
 * `setupRestoreUnlock`, `setupRestoreStart` or `setupRestoreDiscard` with the plan's id.
 */
export const setupRestoreUpload = (setupCode: string, file: File) =>
  sendBackup('/setup/restore/upload', file, { 'X-Polyfin-Setup-Code': setupCode })

export const setupRestoreUnlock = (setupCode: string, id: string, passphrase: string) =>
  request<RestorePlan>('POST', '/setup/restore/unlock', { setupCode, id, passphrase })

/** Restores the backup sent from the setup page: Polyfin stops, restores it, and starts again. */
export const setupRestoreStart = (setupCode: string, id: string, anyway: boolean) =>
  request<{ restoring: boolean }>('POST', '/setup/restore', { setupCode, id, anyway })

export const setupRestoreDiscard = (setupCode: string, id: string) =>
  request<void>('POST', '/setup/restore/discard', { setupCode, id })

/**
 * A device that asked Polyfin for something over DLNA, known by its address. `userId` is the user
 * whose libraries it shows (null for no one); `profile` the profile forced for it: empty for
 * automatic, `builtin:` and a built-in profile's name, or an added profile's ID. `matchedProfile`
 * is the name of the profile it gets now, forced or chosen from what it last sent.
 */
export type DlnaDevice = {
  id: string
  name: string
  manufacturer: string
  model: string
  address: string
  userAgent: string
  clientInfo: string
  userId: string | null
  userName?: string
  blocked: boolean
  profile: string
  matchedProfile: string
  firstSeen: string
  lastSeen: string
}

/** What to change of a DLNA device: the fields left out keep their values. */
export type DlnaDeviceChanges = { userId?: string | null; blocked?: boolean; profile?: string }

/** A DLNA profile: a built-in one (`builtin:` and its name), or one an administrator added. */
export type DlnaProfile = { id: string; name: string; builtin: boolean; createdAt?: string }

/** The DLNA devices seen, the latest first. */
export const fetchDlnaDevices = (signal?: AbortSignal) =>
  request<DlnaDevice[]>('GET', '/dlna/devices', undefined, signal)

export const updateDlnaDevice = (id: string, changes: DlnaDeviceChanges) =>
  request<DlnaDevice>('PATCH', `/dlna/devices/${seg(id)}`, changes)

/** Forgets a device: it is new when it asks again. */
export const forgetDlnaDevice = (id: string) => request<void>('DELETE', `/dlna/devices/${seg(id)}`)

/** The DLNA profiles, the built-in ones first. */
export const fetchDlnaProfiles = (signal?: AbortSignal) =>
  request<DlnaProfile[]>('GET', '/dlna/profiles', undefined, signal)

/** A DLNA profile's XML. */
export async function fetchDlnaProfileXml(id: string, signal?: AbortSignal): Promise<string> {
  const response = await fetch(`${apiBase}/dlna/profiles/${seg(id)}`, {
    headers: { Accept: 'application/xml' },
    signal,
  })
  if (!response.ok) throw await apiError(response)
  return response.text()
}

/**
 * Adds a DLNA profile from the XML of Jellyfin's DLNA plugin, or replaces the added profile `id`.
 * XML that is not a profile is refused with invalid_dlna_profile, and why in the error's detail.
 */
export async function saveDlnaProfile(xml: string, id?: string): Promise<DlnaProfile> {
  const path = id === undefined ? '/dlna/profiles' : `/dlna/profiles/${seg(id)}`
  const response = await fetch(`${apiBase}${path}`, {
    method: id === undefined ? 'POST' : 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/xml' },
    body: xml,
  })
  if (!response.ok) throw await apiError(response)
  return (await response.json()) as DlnaProfile
}

/** Removes an added DLNA profile: the devices forced to it get theirs chosen again. */
export const removeDlnaProfile = (id: string) =>
  request<void>('DELETE', `/dlna/profiles/${seg(id)}`)

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
/**
 * What the image cache keeps, or what clearing it freed: `files` images on disk of `used` bytes,
 * which `limit` bounds, and `memoryImages` in memory, of `memoryBytes` bytes.
 */
export type ImageCacheUsage = {
  files: number
  used: number
  limit: number
  memoryImages: number
  memoryBytes: number
}

/** Empties the image cache, answering what it freed. */
export const clearImageCache = () => request<ImageCacheUsage>('POST', '/health/images/clear')

export const checkAddon = (id: string) =>
  request<AddonHealth>('POST', `/health/addons/${seg(id)}/check`)

/**
 * What starts Polyfin again once it exits: Polyfin Desktop, a restart policy or service manager an
 * administrator confirmed, or nothing Polyfin knows of.
 */
export type RestartKind = 'desktop' | 'confirmed' | 'unknown'

/** The update Polyfin Desktop offers: its download's progress, and whether its last install failed. */
export type DesktopUpdate = {
  version: string
  notes: string
  installing: boolean
  percent: number
  failed: boolean
}

/** How the server's actions stand; `startedAt` changes once a restart has happened. */
export type ServerActions = {
  version: string
  startedAt: string
  restart: RestartKind
  system: ServerSystem
  desktopUpdate: DesktopUpdate | null
  updateTool: boolean
}

export const fetchServerActions = (signal?: AbortSignal) =>
  request<ServerActions>('GET', '/server', undefined, signal)

/**
 * Restarts Polyfin, confirmed with the password or a two-step code; `anyway` once the warning that
 * nothing may start it again was seen. Answers when the stopping Polyfin started.
 */
export const restartServer = (confirmation: TwoStepConfirmation, anyway: boolean) =>
  request<{ startedAt: string }>('POST', '/server/restart', { ...confirmation, anyway })

/** Has Polyfin Desktop install the update it offers, as its tray's Install Polyfin does. */
export const installDesktopUpdate = (confirmation: TwoStepConfirmation, version: string) =>
  request<{ version: string }>('POST', '/server/desktop-update', { ...confirmation, version })

/** What the update tool answered Update now: its HTTP status and the start of its body. */
export type UpdateToolAnswer = { status: number; body: string }

export const askUpdateTool = (confirmation: TwoStepConfirmation) =>
  request<UpdateToolAnswer>('POST', '/server/update-tool', confirmation)

/** The log lines written after the first `after`, and the `after` of the next call. */
export const fetchLogLines = (after: number, limit: number, signal?: AbortSignal) =>
  request<{ lines: string[]; next: number }>(
    'GET',
    `/logs?after=${after}&limit=${limit}`,
    undefined,
    signal,
  )

export const logDownloadUrl = `${apiBase}/logs/download`

/** What every goroutine of the server is doing, as text, for a request that never answers. */
export const goroutinesDownloadUrl = `${apiBase}/health/goroutines`

/** Whose playbacks the statistics cover: everyone's (administrators) or the signed-in user's. */
export type StatisticsScope = 'server' | 'own'

/** The periods of the statistics: the last 7 or 30 days, the last year, or the whole history. */
export const statisticsPeriods = ['7d', '30d', 'year', 'all'] as const
export type StatisticsPeriod = (typeof statisticsPeriods)[number]

/** How a playback reached its app; empty when no report told. */
export type PlayMethod = 'direct_play' | 'direct_stream' | 'conversion' | ''

/** A movie, a series or a channel of the statistics, by its latest name. Times are in seconds. */
export type RankedTitle = { id: string; name: string; plays: number; users: number; played: number }

/**
 * The playbacks of a period, summed. Times are played times in seconds, pauses left out. Buckets
 * are days, or months for the whole history (`unit`), starting YYYY-MM-DD; `hours` are the 168
 * hours of the week from Monday 0:00, in the time zone asked.
 */
export type Statistics = {
  period: StatisticsPeriod
  since: string | null
  unit: 'day' | 'month'
  plays: number
  played: number
  users: { id: string; name: string; plays: number; played: number }[]
  buckets: { start: string; played: number; users: { id: string; played: number }[] }[]
  movies: RankedTitle[]
  series: RankedTitle[]
  channels: RankedTitle[]
  /** shared marks the app and device of guests' playbacks through share links. */
  apps: { name: string; shared: boolean; plays: number; played: number }[]
  devices: { name: string; app: string; shared: boolean; plays: number; played: number }[]
  methods: { method: PlayMethod; plays: number; played: number }[]
  hours: number[]
  /** Whether the history is kept, and for how many days. */
  historyEnabled: boolean
  historyDays: number
}

/** A playback of the history, as its title was called then. Times are in seconds. */
export type HistoryEntry = {
  id: number
  user: { id: string; name: string }
  item: {
    id: string
    kind: 'movie' | 'episode' | 'channel' | 'recording' | 'replay' | 'video'
    name: string
    seriesName: string | null
    season: number | null
    episode: number | null
    channelName: string | null
  }
  app: string
  device: string
  startedAt: string
  endedAt: string
  played: number
  position: number
  method: PlayMethod
  /** Whether a guest played it through the user's share link. */
  shared: boolean
}

export type HistoryPage = { items: HistoryEntry[]; total: number }

/** Which playbacks to read: a period, and, for administrators, one user. */
export type PlaybacksQuery = { period: StatisticsPeriod; user?: string }

const playbacksBase = (scope: StatisticsScope) => (scope === 'own' ? '/account' : '')

function playbacksParams(query: PlaybacksQuery) {
  const params = new URLSearchParams({ period: query.period })
  if (query.user) params.set('user', query.user)
  return params
}

/** The statistics of a period, by the browser's time zone. */
export function fetchStatistics(
  scope: StatisticsScope,
  query: PlaybacksQuery,
  signal?: AbortSignal,
) {
  const params = playbacksParams(query)
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  if (timeZone) params.set('timeZone', timeZone)
  return request<Statistics>(
    'GET',
    `${playbacksBase(scope)}/statistics?${params}`,
    undefined,
    signal,
  )
}

/** The playbacks of a period, the latest first: `limit` (1 to 100) from `start`. */
export function fetchHistory(
  scope: StatisticsScope,
  query: PlaybacksQuery & { limit: number; start?: number },
  signal?: AbortSignal,
) {
  const params = playbacksParams(query)
  params.set('limit', String(query.limit))
  if (query.start) params.set('start', String(query.start))
  return request<HistoryPage>('GET', `${playbacksBase(scope)}/history?${params}`, undefined, signal)
}

/** The address of the CSV of the playbacks of a period. */
export const historyExportUrl = (scope: StatisticsScope, query: PlaybacksQuery) =>
  `${apiBase}${playbacksBase(scope)}/history/export?${playbacksParams(query)}`

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
  themeCopy: ['settings', 'theme-copy'] as const,
  session: ['session'] as const,
  myDevices: ['account', 'devices'] as const,
  ownTwoStep: ['account', 'two-step'] as const,
  ownResume: ['account', 'resume'] as const,
  ownHidden: ['account', 'hidden'] as const,
  ownHomeRows: ['account', 'home-rows'] as const,
  ownTrackChoices: ['account', 'track-choices'] as const,
  homeRows: ['home-rows'] as const,
  homeRowChoices: ['home-rows', 'choices'] as const,
  ownReminders: ['account', 'reminders'] as const,
  reminderSettings: ['account', 'reminders', 'settings'] as const,
  ownMetadataLanguage: ['account', 'metadata-language'] as const,
  ownAudioRegion: ['account', 'audio-region'] as const,
  /** The languages the server's addons were given: refreshed with the server's addons. */
  metadataLanguages: ['scopes', 'shared', 'metadata-languages'] as const,
  ownWebTheme: ['account', 'web-theme'] as const,
  /** The server's theme and the themes offered: refreshed as the settings are saved. */
  webThemes: ['web-themes'] as const,
  tracking: ['account', 'tracking'] as const,
  users: ['users'] as const,
  /** Prefix of every group query: invalidating it refreshes the list and each group. */
  groups: ['groups'] as const,
  group: (id: string) => ['groups', id] as const,
  /** What Reset to defaults would give a user or a group. */
  userDefaults: (id: string) => ['users', id, 'defaults'] as const,
  groupDefaults: (id: string) => ['groups', id, 'defaults'] as const,
  jellyfinImport: ['jellyfin-import'] as const,
  ownImport: ['account', 'server-import'] as const,
  ownAppImport: ['account', 'app-import'] as const,
  userAppImport: (id: string) => ['users', id, 'app-import'] as const,
  userLetterboxdImport: (id: string) => ['users', id, 'server-import'] as const,
  userDevices: (id: string) => ['users', id, 'devices'] as const,
  settings: ['settings'] as const,
  settingsApps: ['settings', 'apps'] as const,
  parentalRatings: ['parental-ratings'] as const,
  /** Prefix of every scope: invalidating it refreshes all addons and libraries. */
  scopes: ['scopes'] as const,
  /** Prefix of everything a scope owns: invalidating it refreshes its addons and libraries. */
  scope: (scope: Scope) => ['scopes', scope] as const,
  addons: (scope: Scope) => ['scopes', scope, 'addons'] as const,
  libraries: (scope: Scope) => ['scopes', scope, 'libraries'] as const,
  smartLists: (scope: Scope) => ['scopes', scope, 'smart-lists'] as const,
  episodeOrders: ['episode-orders'] as const,
  episodeOrder: (id: string) => ['episode-orders', id] as const,
  orderedSeries: (search: string) => ['episode-orders', 'series', search] as const,
  smartList: (scope: Scope, id: string) => ['scopes', scope, 'smart-lists', id] as const,
  smartChoices: (scope: Scope) => ['scopes', scope, 'smart-choices'] as const,
  smartPreview: (scope: Scope, list: SmartListDraft) =>
    ['scopes', scope, 'smart-preview', JSON.stringify(list)] as const,
  addonPreferences: ['account', 'addon-preferences'] as const,
  userContentChoices: ['user-content-choices'] as const,
  apiKeys: ['api-keys'] as const,
  invites: ['invites'] as const,
  ownShareLinks: ['account', 'share-links'] as const,
  shareTitles: (term: string) => ['account', 'share-titles', term] as const,
  shareTitle: (id: string) => ['account', 'share-title', id] as const,
  shareEpisodes: (seriesId: string) => ['account', 'share-episodes', seriesId] as const,
  userShareLinks: (id: string) => ['users', id, 'share-links'] as const,
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
  serverActions: ['server', 'actions'] as const,
  /** Under the health's key: refreshing the health refreshes them too. */
  healthProblems: (language: Language) => ['health', 'problems', language] as const,
  backup: ['backup'] as const,
  backupDestinations: ['backup', 'destinations'] as const,
  backupFiles: ['backup', 'files'] as const,
  sources: ['sources'] as const,
  /** The unmatched files of a local folder. */
  unmatched: (id: string) => ['unmatched', id] as const,
  /** The units of a local folder a looser rule matched. */
  looser: (id: string) => ['looser', id] as const,
  /** A title search for a local folder's unit. */
  folderTitles: (id: string, query: string, year: number | null) =>
    ['folder-titles', id, query, year] as const,
  variables: ['variables'] as const,
  /** Prefix of the DLNA devices and profiles: forcing or removing a profile changes devices. */
  dlna: ['dlna'] as const,
  dlnaDevices: ['dlna', 'devices'] as const,
  dlnaProfiles: ['dlna', 'profiles'] as const,
  dlnaProfileXml: (id: string) => ['dlna', 'profiles', id] as const,
  notifications: (scope: NotificationScope) => ['notifications', scope] as const,
  statistics: (scope: StatisticsScope, query: PlaybacksQuery) =>
    ['statistics', scope, query] as const,
  history: (scope: StatisticsScope, query: PlaybacksQuery & { start: number }) =>
    ['history', scope, query] as const,
}

/**
 * Asks the session again: this browser may be signed in as another user now, as when another tab
 * of the admin app, which shares its session, or the web client changed it. Joins a session request
 * under way rather than cancelling it, which could leave the web client's token taken without the
 * session it opened.
 */
export function askSessionAgain() {
  void queryClient.refetchQueries({ queryKey: queryKeys.session }, { cancelRefetch: false })
}

/**
 * The session ended, as when the web client it was taken from signed out or changed user: the
 * sign-in page shows, and the session is asked again, which takes the web client's sign-in if it is
 * a new one.
 */
export function sessionEnded() {
  if (queryClient.getQueryData(queryKeys.session) === null) return
  queryClient.setQueryData(queryKeys.session, null)
  askSessionAgain()
}

/**
 * A 401 means the session is gone. A 403 "forbidden" may mean that another user signed in to this
 * browser meanwhile: the session is asked again.
 */
function handleSessionErrors(error: unknown) {
  if (!(error instanceof ApiError)) return
  if (error.status === 401) sessionEnded()
  else if (error.status === 403 && error.code === 'forbidden') askSessionAgain()
}

export const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({ onError: handleSessionErrors }),
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      // Sign-in and setup failures are credential errors, not an expired session.
      if (mutation.options.meta?.public === true) return
      handleSessionErrors(error)
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

/** The admin app's tabs in this browser, which share one session, tell each other it changed. */
const sessionChannel =
  typeof BroadcastChannel === 'function' ? new BroadcastChannel('polyfin-admin-session') : null
sessionChannel?.addEventListener('message', askSessionAgain)

/**
 * The user the answers kept are for. When the session's user changes, signed out or another, they
 * are forgotten before any page draws them, but for the public status. The other tabs are told
 * whenever this one learns who is signed in, the first time included: the browser's session may
 * have changed under them.
 */
let answersFor: string | null | undefined
queryClient.getQueryCache().subscribe((event) => {
  if (event.type !== 'updated' || event.action.type !== 'success') return
  if (event.query.queryKey[0] !== queryKeys.session[0]) return
  const user = (event.query.state.data as SessionUser | null | undefined)?.id ?? null
  if (user === answersFor) return
  if (answersFor !== undefined) {
    queryClient.removeQueries({
      predicate: (query) => query.queryKey[0] !== 'status' && query.queryKey[0] !== 'session',
    })
  }
  answersFor = user
  sessionChannel?.postMessage(null)
})

// The web client signing in or out in another tab changes the session this one may take.
window.addEventListener('storage', (event) => {
  if (event.key === null || event.key === webClientCredentialsKey) askSessionAgain()
})
