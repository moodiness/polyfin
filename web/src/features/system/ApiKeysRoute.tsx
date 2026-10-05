import ApiKeysPage from '@/pages/ApiKeysPage'

/**
 * `/system/api-keys`: keys for other tools.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the system area
 * replaces it.
 */
export default function ApiKeysRoute() {
  return <ApiKeysPage />
}
