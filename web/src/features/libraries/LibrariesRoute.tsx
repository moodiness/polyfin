import LibrariesPage from '@/pages/LibrariesPage'

/**
 * `/libraries`: which catalogs are libraries in Jellyfin apps, and in which order.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the libraries area
 * replaces it.
 */
export default function LibrariesRoute() {
  return <LibrariesPage />
}
