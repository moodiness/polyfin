import { BroadcastIcon, MusicNotesIcon, PuzzlePieceIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchAddons, fetchSources, queryKeys, type Addon, type Owner } from '@/api'
import { registerPaletteSource, type PaletteEntry } from '@/app/palette/registry'
import { lineupPath } from '@/components/lineup/common'
import type { Messages } from '@/i18n'

/** One source as a palette entry, opening its page when it has one, else its list. */
function entry(t: Messages, selfId: string, addon: Addon, owner: Owner): PaletteEntry {
  const iptv = addon.kind === 'm3u' || addon.kind === 'xtream'
  const kind =
    addon.kind === 'm3u' || addon.kind === 'xtream'
      ? t.palette.kinds.iptv
      : t.palette.kinds[addon.kind]
  const mine = owner?.id === selfId
  // A source of another user's has no page an administrator can open: their list shows it.
  const to =
    owner === null
      ? iptv
        ? lineupPath('shared', addon.id)
        : '/sources'
      : mine
        ? iptv
          ? lineupPath('me', addon.id)
          : '/me/sources'
        : '/sources'
  return {
    id: `sources:${owner?.id ?? 'server'}:${addon.id}`,
    group: 'sources',
    label: addon.name,
    hint: owner === null || mine ? kind : t.palette.ownedBy(kind, owner.name),
    icon: iptv ? BroadcastIcon : addon.kind === 'eclipse' ? MusicNotesIcon : PuzzlePieceIcon,
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
