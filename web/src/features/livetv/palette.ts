import { BroadcastIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchAddons, fetchLibraries, queryKeys } from '@/api'
import { registerPaletteSource } from '@/app/palette/registry'
import { guidesLink, isIptv, isTvCatalog } from './catalogs'

// The guides of each of the server's TV catalogs in Live TV, for administrators (the Live TV
// page's queries, so the cache is shared).
registerPaletteSource('livetv', ({ t, user }) => {
  const libraries = useQuery({
    queryKey: queryKeys.libraries('shared'),
    queryFn: ({ signal }) => fetchLibraries('shared', signal),
    enabled: user.isAdministrator,
  })
  const addons = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
    enabled: user.isAdministrator,
  })
  if (!user.isAdministrator) return []
  const iptv = new Set((addons.data ?? []).filter(isIptv).map((addon) => addon.id))
  return (libraries.data ?? [])
    .filter((library) => isTvCatalog(library) && library.enabled && library.browsable)
    .map((library) => {
      const name = library.name ?? library.catalogName
      return {
        id: `livetv:${library.addonId}:${library.catalogId}`,
        group: 'pages' as const,
        label: t.livetv.guidesOf(name),
        hint: t.nav.trail(t.nav.content, t.nav.liveTv),
        keywords: [library.addonName, t.livetv.manage],
        icon: iptv.has(library.addonId) ? BroadcastIcon : TelevisionSimpleIcon,
        to: guidesLink('shared', library, iptv.has(library.addonId)),
      }
    })
})
