import { PlusIcon } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchAddons, fetchSources, queryKeys } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { useI18n } from '@/i18n'
import { Button } from '@/ui'
import { AddSourceModal, type AddKind } from './AddSource'
import type { Entry } from './model'
import { SourceBrowser } from './SourceBrowser'

/**
 * `/sources`, for administrators: the server's sources (Stremio and music addons, IPTV sources),
 * which they order and manage, then each user's own sources, marked with their owner. Their own
 * personal sources can be managed here too; other users' are shown without actions.
 * `?add=hdhomerun` (where Jellyfin's tuner setup leads) opens Add a source on its tuner tab.
 */
export default function SourcesRoute() {
  const { t } = useI18n()
  const user = useSessionUser()
  const [adding, setAdding] = useState(false)
  const [startKind, setStartKind] = useState<AddKind>()
  const [params, setParams] = useSearchParams()
  const addTuner = params.get('add') === 'hdhomerun'
  // The address opens the panel once, then leaves the URL: closing it does not open it again.
  useEffect(() => {
    if (!addTuner) return
    setStartKind('tuner')
    setAdding(true)
    setParams(
      (current) => {
        current.delete('add')
        return current
      },
      { replace: true },
    )
  }, [addTuner, setParams])
  const shared = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
  })
  const mine = useQuery({
    queryKey: queryKeys.addons('me'),
    queryFn: ({ signal }) => fetchAddons('me', signal),
  })
  const all = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
  })

  const entries: Entry[] = [
    ...(shared.data ?? []).map((addon) => ({
      key: `shared:${addon.id}`,
      addon,
      owner: null,
      scope: 'shared' as const,
    })),
    ...(mine.data ?? []).map((addon) => ({
      key: `me:${addon.id}`,
      addon,
      owner: { id: user.id, name: user.name },
      scope: 'me' as const,
    })),
    ...(all.data?.addons ?? [])
      .filter((addon) => addon.owner !== null && addon.owner.id !== user.id)
      .map((addon) => ({
        key: `user:${addon.owner?.id}:${addon.id}`,
        addon,
        owner: addon.owner,
        scope: null,
      })),
  ]

  return (
    <PageLayout
      title={t.sources.title}
      lede={t.sources.lede}
      actions={
        <Button variant="primary" icon={PlusIcon} onClick={() => setAdding(true)}>
          {t.sources.add}
        </Button>
      }
    >
      <SourceBrowser
        entries={entries}
        queries={[shared, mine, all]}
        selfId={user.id}
        emptyText={t.sources.emptyShared}
        ownerFilter
        listNote={t.sources.listNote}
        onAdd={() => setAdding(true)}
      />
      <AddSourceModal
        // A panel opened on another kind starts anew on it.
        key={startKind ?? 'any'}
        scope="shared"
        open={adding}
        initialKind={startKind}
        onClose={() => setAdding(false)}
        onAdded={(added) => setParams({ source: `shared:${added.id}` }, { replace: true })}
      />
    </PageLayout>
  )
}
