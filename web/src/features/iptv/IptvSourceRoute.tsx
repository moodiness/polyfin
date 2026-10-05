import type { Scope } from '@/api'
import LineupPage from '@/pages/LineupPage'

/**
 * `/sources/:scope/:id/:section?` and `/me/sources/:id/:section?` (with `scope="me"`): an IPTV
 * source's page. Other addons have no page there yet: the sources area adds their details.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the IPTV area
 * replaces it.
 */
export default function IptvSourceRoute({ scope }: { scope?: Scope }) {
  return <LineupPage scope={scope} />
}
