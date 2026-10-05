import HealthPage from '@/pages/HealthPage'

/**
 * `/system/health`: problems first, then the state of each part of the server.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the system area
 * replaces it.
 */
export default function HealthRoute() {
  return <HealthPage />
}
