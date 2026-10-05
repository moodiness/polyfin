import { MagnifyingGlassIcon, PlusIcon, StackIcon } from '@phosphor-icons/react'
import { useState, useSyncExternalStore } from 'react'
import { useSearchParams } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { orderAddons, queryClient, queryKeys, type Addon, type Scope } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import {
  Button,
  cx,
  Modal,
  EmptyState,
  FieldError,
  InlineError,
  Row,
  RowList,
  Select,
  SkeletonRows,
  Skeleton,
  TextInput,
  RelativeTime,
} from '@/ui'
import { invalidateScope, isIptv, kindOf, lastTime, type Entry, type KindFilter } from './model'
import { SourceStatus, SourceTile } from './parts'
import { KindBadge, OwnerLabel, SourceDetail } from './SourceDetail'

const wideQuery = '(min-width: 1024px)'

/** Whether the list and the detail fit side by side. */
function useWide(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const media = window.matchMedia(wideQuery)
      media.addEventListener('change', onChange)
      return () => media.removeEventListener('change', onChange)
    },
    () => window.matchMedia(wideQuery).matches,
  )
}

/**
 * Moves a source of a scope. Moves are applied to the cache at once and sent one after another
 * (same mutation scope), so quick successive moves never race; the list is refetched once the last
 * one is done.
 */
export function useMoveSource(scope: Scope) {
  const { t } = useI18n()
  const [announcement, setAnnouncement] = useState('')
  const addonsKey = queryKeys.addons(scope)
  const orderKey = ['addon-order', scope] as const
  const order = useMutation({
    mutationKey: orderKey,
    scope: { id: `addon-order-${scope}` },
    mutationFn: (ids: string[]) => orderAddons(scope, ids),
    onSettled: () => {
      if (queryClient.isMutating({ mutationKey: orderKey }) === 1) invalidateScope(scope)
    },
  })
  function move(from: number, to: number) {
    const current = queryClient.getQueryData<Addon[]>(addonsKey)
    if (current === undefined || to < 0 || to >= current.length) return
    const next = [...current]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    void queryClient.cancelQueries({ queryKey: addonsKey })
    queryClient.setQueryData(addonsKey, next)
    setAnnouncement(t.sources.moved(moved.name, to + 1, next.length))
    order.mutate(next.map((addon) => addon.id))
  }
  return { move, announcement, error: order.isError ? order.error : null }
}

/** What a row says under its name: its owner, then what it brings. */
function rowMeta(t: Messages, language: string, addon: Addon): string {
  if (addon.source !== null) {
    return addon.source.fetchedAt === null
      ? t.sources.notDownloaded
      : t.sources.channelsShown(addon.source.lineup.shownChannels.toLocaleString(language))
  }
  return addon.description !== '' ? addon.description : t.sources.catalogCount(addon.catalogCount)
}

type Query = {
  isPending: boolean
  isError: boolean
  error: unknown
  refetch: () => unknown
  isRefetching: boolean
}

/**
 * The list of sources with its filters, and the detail of the one chosen beside it (in a floating panel
 * on narrower screens). The choice is kept in the address (`?source=`), so it survives a reload.
 */
export function SourceBrowser({
  entries,
  queries,
  selfId,
  emptyText,
  ownerFilter,
  listNote,
  onAdd,
}: {
  entries: Entry[]
  /** The loads the list depends on: their first failure is shown with a retry. */
  queries: Query[]
  selfId: string
  emptyText: string
  /** Shows the owner filter (the administrator's list of every user's sources). */
  ownerFilter: boolean
  listNote?: string
  onAdd: () => void
}) {
  const { language, t } = useI18n()
  const text = t.sources
  const wide = useWide()
  const [params, setParams] = useSearchParams()
  const [kind, setKind] = useState<KindFilter | 'all'>('all')
  const [owner, setOwner] = useState('all')
  const [search, setSearch] = useState('')
  const shared = useMoveSource('shared')
  const mine = useMoveSource('me')

  const failed = queries.find((query) => query.isError)
  if (queries.some((query) => query.isPending)) {
    return (
      <div className="grid items-start gap-6 lg:grid-cols-[432px_minmax(0,1fr)]">
        <div>
          <Skeleton className="mb-3 h-[30px] w-64" />
          <SkeletonRows rows={4} label={text.loading} />
        </div>
        <Skeleton className="h-[420px] rounded-panel max-lg:hidden" />
      </div>
    )
  }
  if (failed) {
    return (
      <InlineError
        onRetry={() => queries.forEach((query) => void query.refetch())}
        retrying={queries.some((query) => query.isRefetching)}
      >
        {errorMessage(t, failed.error)}
      </InlineError>
    )
  }
  if (entries.length === 0) {
    return (
      <EmptyState
        icon={StackIcon}
        title={text.emptyTitle}
        action={
          <Button variant="primary" icon={PlusIcon} onClick={onAdd}>
            {text.add}
          </Button>
        }
      >
        {emptyText}
      </EmptyState>
    )
  }

  const owners = new Map<string, string>()
  for (const entry of entries) if (entry.owner) owners.set(entry.owner.id, entry.owner.name)
  const needle = search.trim().toLocaleLowerCase(language)
  const byOwner = entries.filter((entry) =>
    owner === 'all' ? true : owner === 'server' ? entry.owner === null : entry.owner?.id === owner,
  )
  const visible = byOwner.filter(
    (entry) =>
      (kind === 'all' || kindOf(entry.addon) === kind) &&
      (needle === '' ||
        [
          entry.addon.name,
          entry.addon.description,
          entry.owner?.name ?? '',
          text.tag[entry.addon.kind],
        ]
          .join(' ')
          .toLocaleLowerCase(language)
          .includes(needle)),
  )
  const counts = {
    all: byOwner.length,
    stremio: byOwner.filter((e) => kindOf(e.addon) === 'stremio').length,
    eclipse: byOwner.filter((e) => kindOf(e.addon) === 'eclipse').length,
    iptv: byOwner.filter((e) => isIptv(e.addon)).length,
  }

  const chosenKey = params.get('source')
  const chosen = entries.find((entry) => entry.key === chosenKey) ?? (wide ? visible[0] : undefined)

  function choose(key: string | null) {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        if (key === null) next.delete('source')
        else next.set('source', key)
        return next
      },
      { replace: true },
    )
  }

  const scopeEntries = (scope: Entry['scope']) => entries.filter((e) => e.scope === scope)
  const detail = chosen && (
    <SourceDetail
      key={chosen.key}
      entry={chosen}
      selfId={selfId}
      mode="panel"
      index={scopeEntries(chosen.scope).indexOf(chosen)}
      count={scopeEntries(chosen.scope).length}
      onMove={
        chosen.scope === null
          ? undefined
          : (to) =>
              (chosen.scope === 'shared' ? shared : mine).move(
                scopeEntries(chosen.scope).indexOf(chosen),
                to,
              )
      }
      onRemoved={() => choose(null)}
    />
  )
  const orderError = shared.error ?? mine.error

  return (
    <div className="grid items-start gap-6 lg:grid-cols-[432px_minmax(0,1fr)]">
      <div className="min-w-0 lg:sticky lg:top-[calc(var(--spacing-topbar)+24px)]">
        <div className="mb-3 flex flex-col gap-2.5">
          <div
            role="group"
            aria-label={text.kindFilter}
            className="scrollbar-none -mx-1 flex gap-1 overflow-x-auto px-1"
          >
            {(['all', 'stremio', 'eclipse', 'iptv'] as const).map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={kind === value}
                onClick={() => setKind(value)}
                className={cx(
                  'inline-flex h-[30px] shrink-0 items-center gap-1.5 rounded-field border px-2.5 text-control font-medium transition-colors duration-160 ease-nuit',
                  kind === value
                    ? 'border-line-2 bg-s3 text-ink'
                    : 'border-transparent text-ink-2 hover:bg-s2 hover:text-ink',
                )}
              >
                {value === 'all' ? text.all : text.kinds[value]}
                <span className="font-mono text-micro text-ink-3">{counts[value]}</span>
              </button>
            ))}
          </div>
          <div className="flex gap-2 max-sm:flex-col">
            <TextInput
              icon={MagnifyingGlassIcon}
              value={search}
              onValue={setSearch}
              placeholder={text.searchPlaceholder}
              aria-label={text.search}
              type="search"
              className="min-w-0 flex-1"
            />
            {ownerFilter && owners.size > 0 && (
              <Select
                aria-label={text.owner}
                value={owner}
                onValue={setOwner}
                options={[
                  { value: 'all', label: text.everyOwner },
                  { value: 'server', label: text.server },
                  ...[...owners].map(([id, name]) => ({
                    value: id,
                    label: id === selfId ? text.yours : text.ownedBy(name),
                  })),
                ]}
                className="sm:w-[190px]"
              />
            )}
          </div>
        </div>
        <p className="sr-only" role="status">
          {shared.announcement || mine.announcement}
        </p>
        {orderError !== null && <FieldError>{errorMessage(t, orderError)}</FieldError>}
        {visible.length === 0 ? (
          <EmptyState
            icon={MagnifyingGlassIcon}
            title={text.noMatchTitle}
            action={
              <Button
                size="sm"
                onClick={() => {
                  setKind('all')
                  setOwner('all')
                  setSearch('')
                }}
              >
                {text.clearFilters}
              </Button>
            }
          >
            {text.noMatch}
          </EmptyState>
        ) : (
          <RowList variant="separate" aria-label={text.listLabel}>
            {visible.map((entry) => (
              <Row
                key={entry.key}
                leading={<SourceTile addon={entry.addon} selected={entry === chosen} />}
                title={entry.addon.name}
                titleAside={<KindBadge addon={entry.addon} />}
                muted={!entry.addon.enabled}
                meta={
                  <>
                    {entry.owner !== null && (
                      <>
                        <OwnerLabel entry={entry} selfId={selfId} />
                        <span aria-hidden="true">·</span>
                      </>
                    )}
                    <span className="truncate">{rowMeta(t, language, entry.addon)}</span>
                  </>
                }
                trailing={
                  <>
                    <SourceStatus addon={entry.addon} />
                    {lastTime(entry.addon) !== null && (
                      <span className="text-[12px] text-ink-3">
                        <RelativeTime iso={lastTime(entry.addon) as string} />
                      </span>
                    )}
                  </>
                }
                selected={entry === chosen}
                onClick={() => choose(entry.key)}
              />
            ))}
          </RowList>
        )}
        {listNote && <p className="mt-4 px-3.5 text-[12.5px] text-ink-3">{listNote}</p>}
      </div>
      {wide ? (
        <div className="min-w-0">{detail}</div>
      ) : (
        <Modal
          open={chosen !== undefined}
          onClose={() => choose(null)}
          title={chosen?.addon.name ?? ''}
          hideTitle
          width={640}
        >
          <div className="p-3">{detail}</div>
        </Modal>
      )}
    </div>
  )
}
