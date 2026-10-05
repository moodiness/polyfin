import SetupPage from '@/pages/SetupPage'

/**
 * The first administrator, shown by the session gate at any address until one exists.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the auth area
 * replaces it.
 */
export default function SetupRoute() {
  return <SetupPage />
}
