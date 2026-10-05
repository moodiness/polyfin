import CatalogGuidesPage from '@/pages/CatalogGuidesPage'

/**
 * `/live-tv/guides/:scope/:addonId/:catalogId/:section?`: the guides and guide mapping of a
 * Stremio addon's TV catalog.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the Live TV area
 * replaces it.
 */
export default function CatalogGuidesRoute() {
  return <CatalogGuidesPage />
}
