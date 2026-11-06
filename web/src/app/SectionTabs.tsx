import { useCallback, useMemo, useState } from 'react'
import { Outlet } from 'react-router'
import { useI18n } from '@/i18n'
import { Tabs } from '@/ui'
import { contentPages, systemPages } from './navigation'
import { SubAreaContext } from './PageLayout'
import { useSessionUser } from './session'

/**
 * The sub-area words of an area (Content: Sources, Libraries, Live TV, Versions; System: Health,
 * Statistics, Schedule, Logs, API keys), under the title of the route's page. A layout route: the
 * router nests the area's pages under it, and their PageLayout shows the words under its title;
 * a page without one yet (loading) gets them above it. Members, who reach a source page of theirs
 * here, see no words.
 */
export function SectionTabs({ section }: { section: 'content' | 'system' }) {
  const { t } = useI18n()
  const user = useSessionUser()
  const [claims, setClaims] = useState(0)
  const claim = useCallback(() => {
    setClaims((count) => count + 1)
    return () => setClaims((count) => count - 1)
  }, [])
  const pages = section === 'content' ? contentPages : systemPages
  const label = section === 'content' ? t.nav.contentTabs : t.nav.systemTabs
  const words = useMemo(
    () => (
      <Tabs
        label={label}
        items={pages.map((page) => ({ id: page.id, label: page.label(t), to: page.to }))}
      />
    ),
    [label, pages, t],
  )
  const subArea = useMemo(() => ({ words, claim }), [words, claim])

  if (!user.isAdministrator) return <Outlet />
  return (
    <SubAreaContext value={subArea}>
      {claims === 0 && <div className="mb-8 max-md:mb-6">{words}</div>}
      <Outlet />
    </SubAreaContext>
  )
}
