import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { Language } from '@/i18n'

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
}

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
}

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
}

/** One enabled library in the list sent to PUT /scopes/{scope}/libraries. */
export type LibrarySelection = Pick<Library, 'addonId' | 'catalogType' | 'catalogId' | 'name'>

export type AddonPreferences = { useSharedAddons: boolean }

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
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
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

/** Returns the signed-in user, or null when there is no valid session. */
export async function fetchSession(signal?: AbortSignal): Promise<SessionUser | null> {
  try {
    return (await request<{ user: SessionUser }>('GET', '/session', undefined, signal)).user
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return null
    throw error
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

export const fetchAddonPreferences = (signal?: AbortSignal) =>
  request<AddonPreferences>('GET', '/account/addon-preferences', undefined, signal)

export const saveAddonPreferences = (preferences: AddonPreferences) =>
  request<AddonPreferences>('PUT', '/account/addon-preferences', preferences)

export const queryKeys = {
  status: ['status'] as const,
  session: ['session'] as const,
  myDevices: ['account', 'devices'] as const,
  users: ['users'] as const,
  userDevices: (id: string) => ['users', id, 'devices'] as const,
  settings: ['settings'] as const,
  /** Prefix of every scope: invalidating it refreshes all addons and libraries. */
  scopes: ['scopes'] as const,
  /** Prefix of everything a scope owns: invalidating it refreshes its addons and libraries. */
  scope: (scope: Scope) => ['scopes', scope] as const,
  addons: (scope: Scope) => ['scopes', scope, 'addons'] as const,
  libraries: (scope: Scope) => ['scopes', scope, 'libraries'] as const,
  addonPreferences: ['account', 'addon-preferences'] as const,
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
