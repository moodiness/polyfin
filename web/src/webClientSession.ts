/**
 * jellyfin-web 12.4, which Polyfin serves at /web/ on this same origin, keeps its sign-ins in
 * localStorage under this key, as `{"Servers": [{"Id": serverId, "AccessToken": token, …}]}`.
 */
export const webClientCredentialsKey = 'jellyfin_credentials'

/** jellyfin-web keeps the ID of the device it names itself as under this key. */
const deviceIdKey = '_deviceId2'

/**
 * The web client's links to the admin app, its Dashboard and dashboard pages, the Polyfin entry,
 * Share and the reminders' link, carry `?from=web` (see internal/server/webclient.js).
 */
const fromParameter = 'from'
const fromWebClient = 'web'

/**
 * Whether this page load came from the web client. The parameter leaves the address before the
 * router reads it, so that a reload is not taken for another arrival.
 */
let arrived = takeArrival()

function takeArrival(): boolean {
  const address = new URL(window.location.href)
  if (address.searchParams.get(fromParameter) !== fromWebClient) return false
  address.searchParams.delete(fromParameter)
  window.history.replaceState(
    window.history.state,
    '',
    address.pathname + address.search + address.hash,
  )
  return true
}

/** The web client's sign-in on this server: its access token and its user's ID. */
function webClientSignIn(serverId: string | undefined): { token: string; userId: string } | null {
  if (!serverId) return null
  let stored: unknown
  try {
    stored = JSON.parse(localStorage.getItem(webClientCredentialsKey) ?? '{}')
  } catch {
    return null
  }
  if (!stored || typeof stored !== 'object' || !('Servers' in stored)) return null
  if (!Array.isArray(stored.Servers)) return null
  for (const server of stored.Servers) {
    if (
      server &&
      typeof server === 'object' &&
      server.Id === serverId &&
      typeof server.AccessToken === 'string' &&
      server.AccessToken !== ''
    ) {
      const userId = typeof server.UserId === 'string' ? server.UserId : ''
      return { token: server.AccessToken, userId: userId.replaceAll('-', '').toLowerCase() }
    }
  }
  return null
}

/** The web client's token last handed out or turned down, which is not handed out again. */
let tried: string | null = null

/**
 * The access token the web client keeps for this server, so that its Dashboard, and the Polyfin
 * entry members get instead, which lead to the admin app, need no second sign-in. Each token is
 * given once per page load, so that signing out stays signed out until the web client signs in
 * again; null when there is none.
 */
export function takeWebClientToken(serverId: string | undefined): string | null {
  const signIn = webClientSignIn(serverId)
  if (signIn === null || signIn.token === tried) return null
  tried = signIn.token
  return signIn.token
}

/** Keeps the web client's current token from signing the admin app in again: it signed out. */
export function declineWebClientToken(serverId: string | undefined) {
  tried = webClientSignIn(serverId)?.token ?? tried
}

/**
 * Whether this page load came from the web client signed in as another user than `userId`, whose
 * session then gives way to the web client's: the browser is that user's now, as Jellyfin's
 * dashboard, part of jellyfin-web, is. Asked once per page load: false afterwards.
 */
export function arrivedAsAnotherUser(serverId: string | undefined, userId: string): boolean {
  if (!arrived) return false
  arrived = false
  const signIn = webClientSignIn(serverId)
  return signIn !== null && signIn.userId !== '' && signIn.userId !== userId.toLowerCase()
}

/** The browser jellyfin-web names the device after, as it shows in a user's devices. */
function browserName(): string {
  const agent = navigator.userAgent
  if (agent.includes('Firefox/')) return 'Firefox'
  if (agent.includes('Edg/')) return 'Edge Chromium'
  if (agent.includes('OPR/')) return 'Opera'
  if (agent.includes('Chrome/')) return 'Chrome'
  if (agent.includes('Safari/')) return 'Safari'
  return 'Web Browser'
}

/**
 * Signs `name` in to the web client, as jellyfin-web does: through Polyfin's Jellyfin API, as the
 * device jellyfin-web is in this browser (making its ID as it does when it has none yet), then
 * keeps the sign-in where jellyfin-web reads it, in place of an earlier one on this server. Opening
 * /web/ then needs no sign-in.
 */
export async function signInToWebClient(
  server: { id: string; name: string },
  name: string,
  password: string,
): Promise<void> {
  let deviceId = localStorage.getItem(deviceIdKey)
  if (!deviceId) {
    deviceId = btoa(`${navigator.userAgent}|${Date.now()}`).replaceAll('=', '1')
    localStorage.setItem(deviceIdKey, deviceId)
  }
  const device = encodeURIComponent(browserName())
  const response = await fetch('/Users/AuthenticateByName', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `MediaBrowser Client="Jellyfin Web", Device="${device}", DeviceId="${encodeURIComponent(deviceId)}", Version="12.4.0"`,
    },
    body: JSON.stringify({ Username: name, Pw: password }),
  })
  if (!response.ok) throw new Error(`Jellyfin sign-in answered ${response.status}`)
  const result = (await response.json()) as { AccessToken: string; User: { Id: string } }
  let stored: { Servers?: unknown }
  try {
    stored = JSON.parse(localStorage.getItem(webClientCredentialsKey) ?? '{}') as {
      Servers?: unknown
    }
  } catch {
    stored = {}
  }
  const others = Array.isArray(stored.Servers)
    ? stored.Servers.filter(
        (entry: unknown) =>
          !(entry && typeof entry === 'object' && 'Id' in entry && entry.Id === server.id),
      )
    : []
  localStorage.setItem(
    webClientCredentialsKey,
    JSON.stringify({
      ...stored,
      Servers: [
        {
          ManualAddress: window.location.origin,
          LastConnectionMode: 2,
          Name: server.name,
          Id: server.id,
          DateLastAccessed: Date.now(),
          UserId: result.User.Id,
          AccessToken: result.AccessToken,
        },
        ...others,
      ],
    }),
  )
}
