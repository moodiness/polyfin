import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { Language } from '@/i18n'
import { takeWebClientToken } from '@/webClientSession'

export type Status = {
  name: string
  version: string
  serverId: string
  database: 'ready' | 'unavailable'
  setupRequired: boolean
}

export type SessionUser = { id: string; name: string; isAdministrator: boolean }

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

export type Settings = {
  serverName: string
  quickConnectEnabled: boolean
  legacyAuthorization: boolean
  /** Language of the names the server generates for Jellyfin apps (seasons, library suffixes). */
  language: Language
  /** Sends apps the chapters of analyzed versions. */
  chapters: boolean
  /** Analyzes a title's version when its page opens, and the next episode near the end of one. */
  prepareAhead: boolean
  /** Whether the server converts (transcodes) video and audio for apps that need it. */
  transcoding: boolean
  /** Whether users allowed to download may do so. */
  downloads: boolean
  /** Items read at most from one movie or series catalog (any catalog but a Live TV one). */
  catalogLimit: number
  /** Items read at most from one Live TV catalog: its channels, or one day of its guide. */
  channelLimit: number
  /** Whether apps get skip intro, recap and credits buttons, from the segment databases. */
  skipButtons: boolean
  /** Whether apps get similar titles, from the addons' catalogs. */
  similarTitles: boolean
  /** Percent of a title's runtime past which a reported position marks it played. */
  playedPercent: number
  /** Percent of a title's runtime past which a reported position is kept to resume; below playedPercent. */
  resumePercent: number
  /** Minutes a title's version and subtitle lists from the addons are kept. */
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
  /** Makes scrubbing thumbnails of the versions played, from their keyframes, in the background. */
  trickplay: boolean
  /** Seconds between two scrubbing thumbnails. */
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
  /** Folder recordings are written to (read-only); empty when recording is off. */
  recordingsFolder: string
  /** Hours after which the XMLTV guides and IPTV channel lists are fetched again. */
  liveTvRefreshHours: number
}

/** The range the server accepts for Settings.liveTvRefreshHours. */
export const liveTvRefreshHoursRange = { min: 1, max: 168 }

/** The ranges the server accepts for the recording settings, padding in minutes here. */
export const recordingPaddingMinutesRange = { min: 0, max: 60 }
export const recordingRetentionDaysRange = { min: 0, max: 3650 }

/** The ranges the server accepts for Settings.catalogLimit and channelLimit. */
export const catalogLimitRange = { min: 100, max: 20000 }
export const channelLimitRange = { min: 100, max: 50000 }
/** The ranges the server accepts for Settings.loginAttempts (besides 0) and inactiveDeviceDays. */
export const loginAttemptsRange = { min: 3, max: 20 }
export const inactiveDeviceDaysRange = { min: 0, max: 365 }

/** The ranges the server accepts for the content settings. */
export const playedPercentRange = { min: 50, max: 100 }
export const resumePercentRange = { min: 0, max: 50 }
export const versionListMinutesRange = { min: 1, max: 360 }
export const catalogRefreshMinutesRange = { min: 1, max: 1440 }

/** The ranges the server accepts for Settings.analysisTimeout, versionAttempts and maxConversions. */
export const analysisTimeoutRange = { min: 5, max: 120 }
export const versionAttemptsRange = { min: 1, max: 10 }
export const maxConversionsRange = { min: 0, max: 32 }
/** The values the server accepts for Settings.maxConversionHeight, 0 keeping the original height. */
export const conversionHeights = [0, 480, 720, 1080, 1440, 2160]
/** The ranges and values the server accepts for the thumbnail settings. */
export const trickplayIntervalRange = { min: 5, max: 60 }
export const trickplayWidths = [240, 320, 480]
export const thumbnailStorageRange = { min: 1, max: 50 }

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
  /** 'stremio' for a Stremio addon; 'm3u' or 'xtream' for an IPTV source, which `source` describes. */
  kind: 'stremio' | 'm3u' | 'xtream'
  source: IptvSource | null
}

/** An IPTV source's channel list and how it was last fetched. */
export type IptvSource = {
  /** Redacted: the credentials it holds are never returned. */
  address: string
  channels: number
  groups: { name: string; channels: number }[]
  /** Groups shown; null shows them all, those added later included. */
  includedGroups: string[] | null
  checkedAt: string | null
  fetchedAt: string | null
  nextAt: string | null
  /** Code of the last failure; empty after a success. */
  error: string
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
}

/** What changes of an IPTV source; a password left empty keeps the current one. */
export type IptvSourcePatch = Partial<{
  name: string
  url: string
  server: string
  username: string
  password: string
  groups: string[] | null
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
  /** XMLTV guide of an enabled TV catalog; null for any other library. */
  guide: Guide | null
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

/** Which TV catalog a guide request is for. */
export type GuideTarget = Pick<Library, 'addonId' | 'catalogType' | 'catalogId'>

/** One enabled library in the list sent to PUT /scopes/{scope}/libraries. */
export type LibrarySelection = Pick<Library, 'addonId' | 'catalogType' | 'catalogId' | 'name'>

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

/** An HTTP error from the admin API. `code` is the machine code from `{"error": "..."}`. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string) {
    super(`Admin API error ${status}: ${code}`)
    this.name = 'ApiError'
    this.status = status
    this.code = code
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
    try {
      const payload = (await response.json()) as { error?: unknown }
      if (typeof payload.error === 'string') code = payload.error
    } catch {
      // Non-JSON error body (proxy page, empty response): keep the generic code.
    }
    throw new ApiError(response.status, code)
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

export const fetchParentalRatings = (signal?: AbortSignal) =>
  request<ParentalRating[]>('GET', '/parental-ratings', undefined, signal)

export const fetchUserContentChoices = (signal?: AbortSignal) =>
  request<UserContentChoices>('GET', '/user-content-choices', undefined, signal)

export const fetchSettings = (signal?: AbortSignal) =>
  request<Settings>('GET', '/settings', undefined, signal)

export const saveSettings = (settings: Settings) => request<Settings>('PUT', '/settings', settings)

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

/** Sets the XMLTV guide address of a TV catalog (empty removes it) and fetches it at once. */
export const saveGuide = (scope: Scope, target: GuideTarget, url: string) =>
  request<Library[]>('PUT', `${scopePath(scope)}/guides`, { ...target, url })

/** Fetches the XMLTV guide of a TV catalog now. */
export const refreshGuide = (scope: Scope, target: GuideTarget) =>
  request<Library[]>('POST', `${scopePath(scope)}/guides/refresh`, target)

export const fetchAddonPreferences = (signal?: AbortSignal) =>
  request<AddonPreferences>('GET', '/account/addon-preferences', undefined, signal)

export const saveAddonPreferences = (preferences: Pick<AddonPreferences, 'useSharedAddons'>) =>
  request<AddonPreferences>('PUT', '/account/addon-preferences', preferences)

export const fetchApiKeys = (signal?: AbortSignal) =>
  request<ApiKey[]>('GET', '/api-keys', undefined, signal)

export const createApiKey = (app: string) => request<NewApiKey>('POST', '/api-keys', { app })

export const deleteApiKey = (id: string) => request<void>('DELETE', `/api-keys/${seg(id)}`)

/** The latest `limit` (1 to 100) entries of the server's activity, newest first. */
export const fetchActivity = (limit: number, signal?: AbortSignal) =>
  request<ActivityPage>('GET', `/activity?limit=${limit}`, undefined, signal)

export const queryKeys = {
  status: ['status'] as const,
  session: ['session'] as const,
  myDevices: ['account', 'devices'] as const,
  users: ['users'] as const,
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
  activity: (limit: number) => ['activity', limit] as const,
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
