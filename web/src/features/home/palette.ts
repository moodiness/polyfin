import { ArrowClockwiseIcon, CloudArrowDownIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchServerActions, queryKeys } from '@/api'
import { registerPaletteSource, type PaletteEntry } from '@/app/palette/registry'
import enHome from '@/i18n/en/home'
import frHome from '@/i18n/fr/home'

// Restart, and Update now while an update tool is set, for administrators: each opens Home with
// its confirmation (the same query as Home, so its cache serves both). The words of both
// languages find them, whichever the interface uses.
registerPaletteSource('server-actions', ({ t, user }) => {
  const actions = useQuery({
    queryKey: queryKeys.serverActions,
    queryFn: ({ signal }) => fetchServerActions(signal),
    enabled: user.isAdministrator,
  })
  if (!user.isAdministrator) return []
  const text = t.dashboard.server
  const entries: PaletteEntry[] = [
    {
      id: 'server:restart',
      group: 'pages',
      label: text.restartTitle,
      hint: t.nav.home,
      keywords: [enHome.dashboard.server.restartTitle, frHome.dashboard.server.restartTitle],
      icon: ArrowClockwiseIcon,
      to: '/?action=restart',
    },
  ]
  if (actions.data?.updateTool) {
    entries.push({
      id: 'server:update',
      group: 'pages',
      label: text.updateTitle,
      hint: t.nav.home,
      keywords: [enHome.dashboard.server.updateTitle, frHome.dashboard.server.updateTitle],
      icon: CloudArrowDownIcon,
      to: '/?action=update',
    })
  }
  return entries
})
