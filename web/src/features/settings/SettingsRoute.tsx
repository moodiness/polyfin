import { useParams } from 'react-router'
import { isSettingsSection } from '@/app/navigation'
import NotFoundRoute from '@/features/home/NotFoundRoute'
import SettingsPage from '@/pages/SettingsPage'

/**
 * `/settings/:section`: one section of the server settings.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the settings area
 * replaces it.
 */
export default function SettingsRoute() {
  const { section } = useParams()
  if (!isSettingsSection(section)) return <NotFoundRoute />
  return <SettingsPage section={section} />
}
