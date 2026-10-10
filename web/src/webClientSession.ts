/**
 * jellyfin-web 12.2, which Polyfin serves at /web/ on this same origin, keeps its sign-ins in
 * localStorage under this key, as `{"Servers": [{"Id": serverId, "AccessToken": token, …}]}`.
 */
const credentialsKey = 'jellyfin_credentials'

/** jellyfin-web keeps the ID of the device it names itself as under this key. */
const deviceIdKey = '_deviceId2'

let taken = false

/**
 * The access token the web client keeps for this server, so that its Dashboard, and the Polyfin
 * entry members get instead, which lead to the admin app, need no second sign-in. Given once per
 * page load, so that signing out stays signed out; null when there is none.
 */
export function takeWebClientToken(serverId: string | undefined): string | null {
  if (taken || !serverId) return null
  taken = true
  let stored: unknown
  try {
    stored = JSON.parse(localStorage.getItem(credentialsKey) ?? '{}')
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
      return server.AccessToken
    }
  }
  return null
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
      Authorization: `MediaBrowser Client="Jellyfin Web", Device="${device}", DeviceId="${encodeURIComponent(deviceId)}", Version="12.2.0"`,
    },
    body: JSON.stringify({ Username: name, Pw: password }),
  })
  if (!response.ok) throw new Error(`Jellyfin sign-in answered ${response.status}`)
  const result = (await response.json()) as { AccessToken: string; User: { Id: string } }
  let stored: { Servers?: unknown }
  try {
    stored = JSON.parse(localStorage.getItem(credentialsKey) ?? '{}') as { Servers?: unknown }
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
    credentialsKey,
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
