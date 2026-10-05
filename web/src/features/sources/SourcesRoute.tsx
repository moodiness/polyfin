import AddonsPage from '@/pages/AddonsPage'

/**
 * `/sources`: the server's sources (Stremio and music addons, IPTV sources).
 * Temporary: renders the page from before « Nuit » inside the new shell, until the sources area
 * replaces it.
 */
export default function SourcesRoute() {
  return <AddonsPage />
}
