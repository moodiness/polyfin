import OverviewPage from '@/pages/OverviewPage'

/**
 * `/`: the administrator overview, or a member's status.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the home area
 * replaces it.
 */
export default function HomeRoute() {
  return <OverviewPage />
}
