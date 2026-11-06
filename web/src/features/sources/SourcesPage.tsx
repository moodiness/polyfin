import { ArrowLeftIcon, MagnifyingGlassIcon, PlusIcon, StackIcon } from '@phosphor-icons/react'
import { useEffect, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  fetchSources,
  orderAddons,
  queryClient,
  queryKeys,
  type Addon,
  type Scope,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { isLineupSection } from '@/features/iptv/IptvSource'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  ButtonLink,
  cx,
  EmptyState,
  InlineError,
  Modal,
  RelativeTime,
  Row,
  RowList,
  Select,
  Skeleton,
  SkeletonRows,
  TextInput,
} from '@/ui'
import { AddSourceModal, type AddKind } from './AddSource'
import {
  invalidateScope,
  isIptv,
  kindOf,
  lastTime,
  sourcePath,
  type Entry,
  type KindFilter,
} from './model'
import { SourceStatus, SourceTile } from './parts'
import { OwnerLabel, SourceDetail } from './SourceDetail'
import { UsedByChip } from './UsedBy'

/** Content's Sources (the server's, each user's), or a user's own My sources. */
export type SourcesArea = 'content' | 'mine'

const wideQuery = '(min-width: 1024px)'

/** Whether the list and the source's page fit side by side. */
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

type Filters = { kind: KindFilter | 'all'; owner: string; search: string }

/**
 * The list's filters, kept while moving from one source's page to another's (each page is drawn
 * anew), for the session.
 */
const remembered: Record<SourcesArea, Filters> = {
  content: { kind: 'all', owner: 'all', search: '' },
  mine: { kind: 'all', owner: 'all', search: '' },
}

/**
 * Moves a source of a scope. Moves are applied to the cache at once and sent one after another
 * (same mutation scope), so quick successive moves never race; the list is refetched once the last
 * one is done.
 */
function useMoveSource(scope: Scope) {
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

/**
 * What a source says it is: a folder's location, in the admin app's language, else the addon's
 * description, which a folder's addon gives in English. Searched, not shown in the list.
 */
function describe(t: Messages, addon: Addon): string {
  if (addon.folder !== null) {
    return t.localFolders.locations[addon.folder.share === '' ? 'local' : addon.folder.share]
  }
  return addon.description
}

/** What a row says under its name: its kind, then what it brings. */
function rowMeta(t: Messages, language: string, addon: Addon): string {
  const brings =
    addon.source !== null
      ? addon.source.fetchedAt === null
        ? t.sources.notDownloaded
        : t.sources.channelsShown(addon.source.lineup.shownChannels.toLocaleString(language))
      : addon.folder !== null
        ? t.localFolders.kinds[addon.folder.kind]
        : t.sources.catalogCount(addon.catalogCount)
  const kind =
    addon.folder !== null && addon.folder.share !== ''
      ? t.localFolders.locations[addon.folder.share]
      : t.sources.kindName[addon.kind]
  return `${kind} · ${brings}`
}

/**
 * Sources and My sources, with a source's page: the list on the left, sticky, with the open source
 * lit, and the source's page beside it; Add a source at the top right. Without a source in the
 * address, the wide page opens the one chosen with `?source=`, else the first of the list. On a
 * phone (and under 1024 px) the list and a source's page are each a page of their own.
 * `?add=hdhomerun` (where Jellyfin's tuner setup leads) opens Add a source on its tuner tab.
 */
export function SourcesPage({
  area,
  open,
  before,
  after,
}: {
  area: SourcesArea
  /** The source the address names, and the part of its page. */
  open?: { scope: Scope; id: string; section?: string }
  /** Above the list, on the list's own page: My sources' choice of the server's sources. */
  before?: ReactNode
  /** Under the list and the source, on the list's own page: My sources' libraries. */
  after?: ReactNode
}) {
  const { language, t } = useI18n()
  const text = t.sources
  const user = useSessionUser()
  const navigate = useNavigate()
  const wide = useWide()
  const [params, setParams] = useSearchParams()
  const content = area === 'content'
  const addScope: Scope = content ? 'shared' : 'me'
  const listPath = content ? '/sources' : '/me/sources'

  const [adding, setAdding] = useState(false)
  const [startKind, setStartKind] = useState<AddKind>()
  const addTuner = params.get('add') === 'hdhomerun'
  // The address opens the panel once, then leaves the URL: closing it does not open it again.
  useEffect(() => {
    if (!addTuner || !content) return
    setStartKind('tuner')
    setAdding(true)
    setParams(
      (current) => {
        current.delete('add')
        return current
      },
      { replace: true },
    )
  }, [addTuner, content, setParams])

  const [filters, setFiltersState] = useState<Filters>(() => remembered[area])
  function setFilters(patch: Partial<Filters>) {
    setFiltersState((current) => {
      const next = { ...current, ...patch }
      remembered[area] = next
      return next
    })
  }

  const shared = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
    enabled: content,
  })
  const mine = useQuery({
    queryKey: queryKeys.addons('me'),
    queryFn: ({ signal }) => fetchAddons('me', signal),
  })
  const all = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
    enabled: content,
  })
  const queries = content ? [shared, mine, all] : [mine]
  const movers = { shared: useMoveSource('shared'), me: useMoveSource('me') }

  const entries: Entry[] = content
    ? [
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
    : (mine.data ?? []).map((addon) => ({
        key: `me:${addon.id}`,
        addon,
        owner: null,
        scope: 'me' as const,
      }))

  const owners = new Map<string, string>()
  for (const entry of entries) if (entry.owner) owners.set(entry.owner.id, entry.owner.name)
  const needle = filters.search.trim().toLocaleLowerCase(language)
  const byOwner = entries.filter((entry) =>
    filters.owner === 'all'
      ? true
      : filters.owner === 'server'
        ? entry.owner === null
        : entry.owner?.id === filters.owner,
  )
  const visible = byOwner.filter(
    (entry) =>
      (filters.kind === 'all' || kindOf(entry.addon) === filters.kind) &&
      (needle === '' ||
        [
          entry.addon.name,
          describe(t, entry.addon),
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
    local: byOwner.filter((e) => kindOf(e.addon) === 'local').length,
  }
  // Only the server has local folders, and Eclipse addons are rarer: their filters show once
  // there is one.
  const kindFilters = (['all', 'stremio', 'eclipse', 'iptv', 'local'] as const).filter(
    (value) => value === 'all' || value === 'stremio' || value === 'iptv' || counts[value] > 0,
  )

  const chosenKey = params.get('source')
  const chosen = open
    ? entries.find((entry) => entry.scope === open.scope && entry.addon.id === open.id)
    : (entries.find((entry) => entry.key === chosenKey) ??
      (wide ? (visible[0] ?? entries[0]) : undefined))
  const missing =
    open !== undefined &&
    (chosen === undefined || (isIptv(chosen.addon) ? !isLineupSection(open.section) : false))

  const loading = queries.some((query) => query.isPending)
  const failed = queries.find((query) => query.isError)
  const back = { to: listPath, label: content ? text.back : text.backMine }
  const narrowPage = !wide && open !== undefined

  function clearChoice() {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        next.delete('source')
        return next
      },
      { replace: true },
    )
  }

  const scopeEntries = (scope: Entry['scope']) => entries.filter((e) => e.scope === scope)
  const detail = loading ? (
    <div aria-busy="true">
      <Skeleton className="h-[72px] w-80 rounded-row" />
      <Skeleton className="mt-10 h-[420px] rounded-panel" />
    </div>
  ) : missing ? (
    <EmptyState
      icon={StackIcon}
      title={text.notFoundTitle}
      action={
        <ButtonLink to={listPath} icon={ArrowLeftIcon}>
          {back.label}
        </ButtonLink>
      }
    >
      {text.notFound}
    </EmptyState>
  ) : (
    chosen && (
      <SourceDetail
        key={chosen.key}
        entry={chosen}
        selfId={user.id}
        section={open?.section}
        index={scopeEntries(chosen.scope).indexOf(chosen)}
        count={scopeEntries(chosen.scope).length}
        onMove={
          chosen.scope === null
            ? undefined
            : (to) =>
                movers[chosen.scope as Scope].move(scopeEntries(chosen.scope).indexOf(chosen), to)
        }
        moveError={
          chosen.scope !== null && movers[chosen.scope].error !== null
            ? errorMessage(t, movers[chosen.scope].error)
            : undefined
        }
        onRemoved={() => {
          if (open) void navigate(listPath, { replace: true })
          else clearChoice()
        }}
      />
    )
  )

  const list = (
    <section aria-label={text.listLabel} className="min-w-0">
      <div role="group" aria-label={text.kindFilter} className="mb-3.5 -ml-3 flex flex-wrap gap-1">
        {kindFilters.map((value) => (
          <button
            key={value}
            type="button"
            aria-pressed={filters.kind === value}
            onClick={() => setFilters({ kind: value })}
            className={cx(
              'focus-lit inline-flex items-baseline gap-2 rounded-[14px] px-3 py-2 text-control font-semibold transition-colors duration-300 ease-nuit',
              filters.kind === value ? 'bg-s2 text-ink' : 'text-ink-3 hover:text-ink',
            )}
          >
            {value === 'all' ? text.all : text.kinds[value]}
            <span className="figures text-[16px] font-semibold text-ink-3">{counts[value]}</span>
          </button>
        ))}
      </div>
      {(entries.length > 4 || needle !== '' || (content && owners.size > 0)) && (
        <div className="mb-3.5 flex flex-col gap-2">
          <TextInput
            size="sm"
            icon={MagnifyingGlassIcon}
            value={filters.search}
            onValue={(search) => setFilters({ search })}
            placeholder={text.searchPlaceholder}
            aria-label={text.search}
            type="search"
          />
          {content && owners.size > 0 && (
            <Select
              size="sm"
              aria-label={text.owner}
              value={filters.owner}
              onValue={(owner) => setFilters({ owner })}
              options={[
                { value: 'all', label: text.everyOwner },
                { value: 'server', label: text.server },
                ...[...owners].map(([id, name]) => ({
                  value: id,
                  label: id === user.id ? text.yours : text.ownedBy(name),
                })),
              ]}
            />
          )}
        </div>
      )}
      <p className="sr-only" role="status">
        {movers.shared.announcement || movers.me.announcement}
      </p>
      {visible.length === 0 ? (
        <EmptyState
          icon={MagnifyingGlassIcon}
          title={text.noMatchTitle}
          action={
            <Button size="sm" onClick={() => setFilters({ kind: 'all', owner: 'all', search: '' })}>
              {text.clearFilters}
            </Button>
          }
        >
          {text.noMatch}
        </EmptyState>
      ) : (
        <RowList variant="separate" aria-label={text.listLabel}>
          {visible.map((entry) => {
            const when = lastTime(entry.addon)
            return (
              <Row
                key={entry.key}
                className="py-[18px]"
                leading={<SourceTile addon={entry.addon} selected={entry === chosen} />}
                title={entry.addon.name}
                muted={!entry.addon.enabled}
                meta={
                  <>
                    <span className="basis-full">{rowMeta(t, language, entry.addon)}</span>
                    {entry.owner !== null && content && (
                      <span className="basis-full">
                        <OwnerLabel entry={entry} selfId={user.id} />
                      </span>
                    )}
                    {entry.owner === null && entry.addon.usedBy.mode === 'listed' && (
                      <span className="flex min-w-0 basis-full">
                        <UsedByChip usedBy={entry.addon.usedBy} />
                      </span>
                    )}
                    <span className="mt-2 flex basis-full flex-wrap items-center gap-x-3 gap-y-1">
                      <SourceStatus addon={entry.addon} className="text-[16.5px]" />
                      {when !== null && (
                        <RelativeTime iso={when} className="text-micro text-ink-3" />
                      )}
                    </span>
                  </>
                }
                selected={entry === chosen && !missing}
                // Another user's source has no page of its own: the list opens it beside itself.
                to={
                  entry.scope !== null
                    ? sourcePath(entry.scope, entry.addon.id)
                    : `/sources?source=${encodeURIComponent(entry.key)}`
                }
              />
            )
          })}
        </RowList>
      )}
      {content && <p className="mt-[18px] px-1.5 text-[16.5px] text-ink-3">{text.listNote}</p>}
    </section>
  )

  let body: ReactNode
  if (failed) {
    body = (
      <InlineError
        onRetry={() => queries.forEach((query) => void query.refetch())}
        retrying={queries.some((query) => query.isRefetching)}
      >
        {errorMessage(t, failed.error)}
      </InlineError>
    )
  } else if (loading && !narrowPage) {
    body = (
      <div className="grid items-start gap-14 lg:grid-cols-[420px_minmax(0,1fr)] max-[1180px]:gap-10 max-[1180px]:lg:grid-cols-[340px_minmax(0,1fr)]">
        <div>
          <Skeleton className="mb-3.5 h-10 w-64" />
          <SkeletonRows rows={3} label={text.loading} />
        </div>
        {wide && detail}
      </div>
    )
  } else if (!loading && entries.length === 0 && !open) {
    body = (
      <EmptyState
        icon={StackIcon}
        title={text.emptyTitle}
        action={
          <Button variant="primary" icon={PlusIcon} onClick={() => setAdding(true)}>
            {text.add}
          </Button>
        }
      >
        {content ? text.emptyShared : text.emptyMine}
      </EmptyState>
    )
  } else if (narrowPage) {
    body = detail
  } else {
    body = (
      <div className="grid items-start gap-14 lg:grid-cols-[420px_minmax(0,1fr)] max-[1180px]:gap-10 max-[1180px]:lg:grid-cols-[340px_minmax(0,1fr)]">
        {/* The list stays in view beside a long page; a list taller than the screen scrolls. */}
        <div className="min-w-0 lg:sticky lg:top-6 lg:-m-3 lg:max-h-[calc(100dvh-48px)] lg:overflow-y-auto lg:p-3">
          {list}
        </div>
        {wide ? (
          <div className="min-w-0">{detail}</div>
        ) : (
          <Modal
            open={chosen !== undefined}
            onClose={clearChoice}
            title={chosen?.addon.name ?? ''}
            hideTitle
            width={640}
          >
            <div className="p-5">{detail}</div>
          </Modal>
        )}
      </div>
    )
  }

  return (
    <PageLayout
      title={content ? text.title : t.nav.mySources}
      lede={content ? undefined : t.mySources.lede}
      back={narrowPage ? back : undefined}
      actions={
        <Button icon={PlusIcon} onClick={() => setAdding(true)}>
          {text.add}
        </Button>
      }
    >
      {!open && before}
      {body}
      {!open && after}
      <AddSourceModal
        // A panel opened on another kind starts anew on it.
        key={startKind ?? 'any'}
        scope={addScope}
        open={adding}
        initialKind={startKind}
        onClose={() => setAdding(false)}
        onAdded={(added) => void navigate(sourcePath(addScope, added.id))}
      />
    </PageLayout>
  )
}
