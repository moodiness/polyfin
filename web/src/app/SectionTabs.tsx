import { Outlet } from 'react-router'
import { useI18n } from '@/i18n'
import { Tabs } from '@/ui'
import { contentPages, systemPages } from './navigation'
import { useSessionUser } from './session'

/**
 * The tabs of a TopBar section (Content: Sources, Libraries, Live TV, Streamyfin home; System:
 * Health, Schedule, Logs, API keys) above the page of the route. A layout route: the router nests
 * the section's pages under it. Members, who reach a source page of theirs here, see no tabs.
 */
export function SectionTabs({ section }: { section: 'content' | 'system' }) {
  const { t } = useI18n()
  const user = useSessionUser()
  const pages = section === 'content' ? contentPages : systemPages
  return (
    <>
      {user.isAdministrator && (
        <Tabs
          label={section === 'content' ? t.nav.contentTabs : t.nav.systemTabs}
          items={pages.map((page) => ({ id: page.id, label: page.label(t), to: page.to }))}
          className="mb-8 max-md:mb-6"
        />
      )}
      <Outlet />
    </>
  )
}
