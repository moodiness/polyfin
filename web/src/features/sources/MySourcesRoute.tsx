import MyAddonsPage from '@/pages/MyAddonsPage'

/**
 * `/me/sources`: the signed-in user's own sources and libraries.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the sources area
 * replaces it.
 */
export default function MySourcesRoute() {
  return <MyAddonsPage />
}
