/**
 * jellyfin-web 12.2, which Polyfin serves at /web/ on this same origin, keeps its sign-ins in
 * localStorage under this key, as `{"Servers": [{"Id": serverId, "AccessToken": token, …}]}`.
 */
const credentialsKey = 'jellyfin_credentials'

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
