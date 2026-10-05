import { useQuery } from '@tanstack/react-query'
import { fetchAddons, fetchSources, queryKeys, type Addon, type Owner } from '@/api'
import { registerPaletteSource, type PaletteEntry } from '@/app/palette/registry'
import type { Messages } from '@/i18n'
import { isIptv, kindIcon, sourcePath } from './model'

/** One source as a palette entry, opening its page when it has one, else its list. */
function entry(t: Messages, selfId: string, addon: Addon, owner: Owner): PaletteEntry {
  const kind = isIptv(addon)
    ? t.palette.kinds.iptv
    : t.palette.kinds[addon.kind as 'stremio' | 'eclipse']
  const mine = owner?.id === selfId
  // Another user's source has no page an administrator can open: their row in the list shows it.
  const to =
    owner === null
      ? sourcePath('shared', addon.id)
      : mine
        ? sourcePath('me', addon.id)
        : `/sources?source=${encodeURIComponent(`user:${owner.id}:${addon.id}`)}`
  return {
    id: `sources:${owner?.id ?? 'server'}:${addon.id}`,
    group: 'sources',
    label: addon.name,
    hint: owner === null || mine ? kind : t.palette.ownedBy(kind, owner.name),
    icon: kindIcon(addon),
    to,
  }
}

// Administrators find every source, the server's and each user's (the query Health and Schedule
// use); members find their own.
registerPaletteSource('sources', ({ t, user }) => {
  const all = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
    enabled: user.isAdministrator,
  })
  const mine = useQuery({
    queryKey: queryKeys.addons('me'),
    queryFn: ({ signal }) => fetchAddons('me', signal),
    enabled: !user.isAdministrator,
  })
  if (user.isAdministrator) {
    return (all.data?.addons ?? []).map((addon) => entry(t, user.id, addon, addon.owner))
  }
  return (mine.data ?? []).map((addon) =>
    entry(t, user.id, addon, { id: user.id, name: user.name }),
  )
})
