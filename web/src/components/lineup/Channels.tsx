import { useId, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import {
  bulkLineupChannels,
  fetchLineupCategories,
  fetchLineupChannels,
  moveLineupChannel,
  queryClient,
  queryKeys,
  updateLineupChannel,
  type ChannelFilters,
  type LineupChannel,
  type Page,
  type Scope,
} from '@/api'
import { icons } from '@/components/icons'
import { invalidateLineup } from '@/components/lineup/common'
import ChannelEditor from '@/components/lineup/ChannelEditor'
import {
  ChannelLogo,
  Pager,
  SidePanel,
  smallField,
  Switch,
  useDebounced,
} from '@/components/lineup/shared'
import { Empty, Panel, Skeleton } from '@/components/panels'
import { Badge, buttonPrimary, buttonSecondary, Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Rows of one page: enough to scan, few enough to draw at once without delay. */
const pageSize = 100

type Tristate = 'any' | 'yes' | 'no'

const asFilter = (value: Tristate) => (value === 'any' ? undefined : value === 'yes')

/** A source's channels, a page at a time from the server, filtered, edited and reordered. */
export default function Channels({ scope, id }: { scope: Scope; id: string }) {
  const { t } = useI18n()
  const text = t.lineup.channels
  const [params, setParams] = useSearchParams()
  const category = params.get('category') ?? ''
  const [search, setSearch] = useState('')
  const [enabled, setEnabled] = useState<Tristate>('any')
  const [shown, setShown] = useState<Tristate>('any')
  const [mapped, setMapped] = useState<Tristate>('any')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [editing, setEditing] = useState<LineupChannel | null>(null)
  const [announcement, setAnnouncement] = useState('')
  const [dragged, setDragged] = useState<number | null>(null)
  const ids = { search: useId(), category: useId() }
  const q = useDebounced(search.trim())
  const filters: ChannelFilters = {
    category: category || undefined,
    enabled: asFilter(enabled),
    shown: asFilter(shown),
    mapped: asFilter(mapped),
    q: q || undefined,
  }
  const key = [...queryKeys.lineup(scope, id), 'channels', filters, offset]
  const categories = useQuery({
    queryKey: [...queryKeys.lineup(scope, id), 'categories'],
    queryFn: ({ signal }) => fetchLineupCategories(scope, id, signal),
  })
  const channels = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchLineupChannels(scope, id, filters, offset, pageSize, signal),
    placeholderData: keepPreviousData,
  })
  const items = channels.data?.items ?? []
  // Channels move within their category: only while it alone is listed, in its order.
  const reorder =
    category !== '' && q === '' && enabled === 'any' && shown === 'any' && mapped === 'any'

  function refilter(change: () => void) {
    change()
    setOffset(0)
    setSelected(new Set())
  }

  /** The id of the channel at a position of the category, or null past its end. */
  async function channelAt(position: number): Promise<string | null> {
    const inPage = position - offset
    if (inPage >= 0 && inPage < items.length) return items[inPage].id
    if (position >= (channels.data?.total ?? 0)) return null
    const page = await fetchLineupChannels(scope, id, { category }, position, 1)
    return page.items[0]?.id ?? null
  }

  const move = useMutation({
    mutationFn: async ({ from, to }: { from: number; to: number }) => {
      // Moving down puts the channel before the one after its new place.
      const before = await channelAt(to > from ? to + 1 : to)
      await moveLineupChannel(scope, id, items[from - offset].id, before)
      return { name: items[from - offset].name, to }
    },
    onSuccess: ({ name, to }) =>
      setAnnouncement(t.common.moved(name, to + 1, channels.data?.total ?? 0)),
    onSettled: () => invalidateLineup(scope, id),
  })
  const bulk = useMutation({
    mutationFn: (on: boolean) => bulkLineupChannels(scope, id, { ids: [...selected] }, on, false),
    onSuccess: (result) => {
      setAnnouncement(text.bulkDone(result.changed))
      setSelected(new Set())
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const allSelected = items.length > 0 && items.every((item) => selected.has(item.id))

  return (
    <div className="space-y-6">
      <Panel title={text.title} description={reorder ? text.reorderHelp : text.help}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(0,2fr)_minmax(0,1.5fr)_repeat(3,minmax(0,1fr))]">
          <div className="relative">
            <label htmlFor={ids.search} className="mb-1.5 block text-xs font-medium text-muted">
              {text.search}
            </label>
            <icons.search className="pointer-events-none absolute bottom-2.5 left-3 size-4 text-muted" />
            <input
              id={ids.search}
              type="search"
              value={search}
              autoComplete="off"
              onChange={(event) => refilter(() => setSearch(event.target.value))}
              className={`${smallField} pl-9`}
            />
          </div>
          <div>
            <label htmlFor={ids.category} className="mb-1.5 block text-xs font-medium text-muted">
              {text.category}
            </label>
            <select
              id={ids.category}
              value={category}
              onChange={(event) =>
                refilter(() =>
                  setParams(event.target.value ? { category: event.target.value } : {}, {
                    replace: true,
                  }),
                )
              }
              className={smallField}
            >
              <option value="">{text.allCategories}</option>
              {(categories.data?.items ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </div>
          <TristateSelect
            label={text.enabledFilter}
            value={enabled}
            yes={text.on}
            no={text.off}
            onChange={(value) => refilter(() => setEnabled(value))}
          />
          <TristateSelect
            label={text.shownFilter}
            value={shown}
            yes={text.shown}
            no={text.hidden}
            onChange={(value) => refilter(() => setShown(value))}
          />
          <TristateSelect
            label={text.mappedFilter}
            value={mapped}
            yes={text.mapped}
            no={text.unmapped}
            onChange={(value) => refilter(() => setMapped(value))}
          />
        </div>

        <div className="mt-4 flex min-h-10 flex-wrap items-center justify-between gap-3 border-y border-line py-2">
          <label className="inline-flex items-center gap-2 text-sm text-zinc-200">
            <input
              type="checkbox"
              checked={allSelected}
              disabled={items.length === 0}
              onChange={(event) =>
                setSelected((current) => {
                  const next = new Set(current)
                  for (const item of items) {
                    if (event.target.checked) next.add(item.id)
                    else next.delete(item.id)
                  }
                  return next
                })
              }
              className="size-4 accent-fin-3"
            />
            {text.selectPage}
          </label>
          {selected.size > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-zinc-100 tabular-nums">
                {text.selected(selected.size)}
              </span>
              <button
                type="button"
                className={rowButton}
                disabled={bulk.isPending}
                onClick={() => bulk.mutate(true)}
              >
                {text.turnOn}
              </button>
              <button
                type="button"
                className={rowButton}
                disabled={bulk.isPending}
                onClick={() => bulk.mutate(false)}
              >
                {text.turnOff}
              </button>
              <button type="button" className={rowButton} onClick={() => setSelected(new Set())}>
                {text.clearSelection}
              </button>
            </div>
          )}
        </div>
        <p aria-live="polite" className="sr-only">
          {announcement}
        </p>
        {(move.isError || bulk.isError) && (
          <div className="mt-3">
            <Notice kind="error">{errorMessage(t, move.error ?? bulk.error)}</Notice>
          </div>
        )}

        {channels.isPending ? (
          <div className="mt-4">
            <Skeleton rows={8} label={t.common.loading} />
          </div>
        ) : channels.isError ? (
          <div className="mt-4">
            <Notice kind="error">{errorMessage(t, channels.error)}</Notice>
          </div>
        ) : items.length === 0 ? (
          <div className="mt-4">
            <Empty>{text.noMatch}</Empty>
          </div>
        ) : (
          <ul
            aria-busy={channels.isFetching}
            className={`mt-2 transition-opacity ${channels.isPlaceholderData ? 'opacity-60' : ''}`}
          >
            {items.map((channel, index) => (
              <ChannelRow
                key={channel.id}
                scope={scope}
                id={id}
                pageKey={key}
                channel={channel}
                position={offset + index}
                total={channels.data.total}
                selected={selected.has(channel.id)}
                reorder={reorder}
                busy={move.isPending}
                dragging={dragged === offset + index}
                onSelect={(checked) =>
                  setSelected((current) => {
                    const next = new Set(current)
                    if (checked) next.add(channel.id)
                    else next.delete(channel.id)
                    return next
                  })
                }
                onEdit={() => setEditing(channel)}
                onMove={(to) => move.mutate({ from: offset + index, to })}
                onDragStart={() => setDragged(offset + index)}
                onDragEnd={() => setDragged(null)}
                onDrop={() => {
                  if (dragged !== null && dragged !== offset + index) {
                    move.mutate({ from: dragged, to: offset + index })
                  }
                  setDragged(null)
                }}
              />
            ))}
          </ul>
        )}
        {channels.data !== undefined && (
          <div className="mt-4">
            <Pager
              offset={offset}
              limit={pageSize}
              total={channels.data.total}
              busy={channels.isFetching}
              onOffset={(next) => {
                setOffset(next)
                setSelected(new Set())
              }}
            />
          </div>
        )}
      </Panel>
      <KeywordBulk scope={scope} id={id} category={category} />
      <SidePanel
        open={editing !== null}
        title={editing ? t.lineup.editor.title(editing.name) : ''}
        onClose={() => setEditing(null)}
      >
        {editing && <ChannelEditor scope={scope} id={id} channelId={editing.id} />}
      </SidePanel>
    </div>
  )
}

function TristateSelect({
  label,
  value,
  yes,
  no,
  onChange,
}: {
  label: string
  value: Tristate
  yes: string
  no: string
  onChange: (value: Tristate) => void
}) {
  const { t } = useI18n()
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-xs font-medium text-muted">
        {label}
      </label>
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value as Tristate)}
        className={smallField}
      >
        <option value="any">{t.lineup.channels.any}</option>
        <option value="yes">{yes}</option>
        <option value="no">{no}</option>
      </select>
    </div>
  )
}

function ChannelRow({
  scope,
  id,
  pageKey,
  channel,
  position,
  total,
  selected,
  reorder,
  busy,
  dragging,
  onSelect,
  onEdit,
  onMove,
  onDragStart,
  onDragEnd,
  onDrop,
}: {
  scope: Scope
  id: string
  pageKey: readonly unknown[]
  channel: LineupChannel
  position: number
  total: number
  selected: boolean
  reorder: boolean
  busy: boolean
  dragging: boolean
  onSelect: (checked: boolean) => void
  onEdit: () => void
  onMove: (to: number) => void
  onDragStart: () => void
  onDragEnd: () => void
  onDrop: () => void
}) {
  const { t } = useI18n()
  const text = t.lineup.channels
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => updateLineupChannel(scope, id, channel.id, { enabled }),
    onSuccess: (updated) =>
      queryClient.setQueryData<Page<LineupChannel>>(pageKey, (page) =>
        page
          ? { ...page, items: page.items.map((c) => (c.id === updated.id ? updated : c)) }
          : page,
      ),
    onSettled: () => invalidateLineup(scope, id),
  })
  return (
    <li
      draggable={reorder}
      onDragStart={(event) => {
        event.dataTransfer.effectAllowed = 'move'
        event.dataTransfer.setData('text/plain', channel.id)
        onDragStart()
      }}
      onDragEnd={onDragEnd}
      onDragOver={(event) => reorder && event.preventDefault()}
      onDrop={(event) => {
        event.preventDefault()
        onDrop()
      }}
      className={`flex h-16 items-center gap-3 border-b border-line px-1 text-sm last:border-b-0 ${
        dragging ? 'opacity-40' : ''
      } ${selected ? 'bg-fin-2/10' : ''}`}
    >
      <input
        type="checkbox"
        checked={selected}
        onChange={(event) => onSelect(event.target.checked)}
        aria-label={text.select(channel.name)}
        className="size-4 shrink-0 accent-fin-3"
      />
      <span className="hidden w-12 shrink-0 text-right text-xs text-muted tabular-nums sm:inline">
        {channel.number ?? '–'}
      </span>
      <ChannelLogo id={channel.id} logo={channel.logo} name={channel.name} />
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-2">
          <span
            className={`truncate font-medium ${channel.enabled ? 'text-white' : 'text-zinc-400'}`}
            title={channel.name}
          >
            {channel.name}
          </span>
          <span className="hidden shrink-0 gap-1 md:flex">
            {channel.enabled && !channel.shown && (
              <span title={text.hiddenHelp}>
                <Badge tone="warning">{text.hidden}</Badge>
              </span>
            )}
            {channel.mapping === null || channel.mapping.guideChannelId === null ? (
              <Badge tone="muted">{text.unmapped}</Badge>
            ) : null}
            {channel.mapping?.manual && <Badge tone="fin">{text.manual}</Badge>}
            {channel.streams.length > 1 && (
              <Badge tone="muted">{text.streamCount(channel.streams.length)}</Badge>
            )}
          </span>
        </p>
        <p className="truncate text-xs text-muted" title={channel.providerName}>
          {channel.category.name}
          {channel.renamed && ` · ${channel.providerName}`}
        </p>
      </div>
      <Switch
        label={text.enabledLabel(channel.name)}
        checked={channel.enabled}
        disabled={toggle.isPending}
        onChange={(enabled) => toggle.mutate(enabled)}
      />
      {reorder && (
        <span className="hidden gap-1 sm:flex">
          <button
            type="button"
            className={iconButton}
            disabled={busy || position === 0}
            aria-label={t.common.moveUp(channel.name)}
            onClick={() => onMove(position - 1)}
          >
            <span aria-hidden="true">↑</span>
          </button>
          <button
            type="button"
            className={iconButton}
            disabled={busy || position === total - 1}
            aria-label={t.common.moveDown(channel.name)}
            onClick={() => onMove(position + 1)}
          >
            <span aria-hidden="true">↓</span>
          </button>
        </span>
      )}
      <button
        type="button"
        className={rowButton}
        aria-label={text.editLabel(channel.name)}
        onClick={onEdit}
      >
        {text.edit}
      </button>
    </li>
  )
}

/** Turns on or off every channel matching a keyword, after counting them. */
function KeywordBulk({ scope, id, category }: { scope: Scope; id: string; category: string }) {
  const { language, t } = useI18n()
  const text = t.lineup.keyword
  const inputId = useId()
  const [keyword, setKeyword] = useState('')
  const [inCategory, setInCategory] = useState(true)
  const selector = {
    q: keyword.trim(),
    ...(category !== '' && inCategory ? { inCategory: category } : {}),
  }
  const count = useMutation({
    mutationFn: () => bulkLineupChannels(scope, id, selector, true, true),
  })
  const apply = useMutation({
    mutationFn: (enabled: boolean) => bulkLineupChannels(scope, id, selector, enabled, false),
    onSuccess: () => count.reset(),
    onSettled: () => invalidateLineup(scope, id),
  })
  const tooShort = keyword.trim().length < 2

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    apply.reset()
    if (!tooShort) count.mutate()
  }

  return (
    <Panel title={text.title} description={text.help}>
      <form onSubmit={submit} noValidate className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1">
          <label htmlFor={inputId} className="mb-1.5 block text-xs font-medium text-muted">
            {text.label}
          </label>
          <input
            id={inputId}
            value={keyword}
            autoComplete="off"
            onChange={(event) => {
              count.reset()
              apply.reset()
              setKeyword(event.target.value)
            }}
            aria-describedby={`${inputId}-hint`}
            className={smallField}
          />
          <p id={`${inputId}-hint`} className="mt-1 text-xs text-muted">
            {text.hint}
          </p>
        </div>
        <button type="submit" className={buttonSecondary} disabled={tooShort || count.isPending}>
          {count.isPending ? text.counting : text.count}
        </button>
      </form>
      {category !== '' && (
        <label className="mt-3 inline-flex items-center gap-2 text-sm text-zinc-200">
          <input
            type="checkbox"
            checked={inCategory}
            onChange={(event) => {
              count.reset()
              setInCategory(event.target.checked)
            }}
            className="size-4 accent-fin-3"
          />
          {text.inCategory}
        </label>
      )}
      <div aria-live="polite" className="mt-3 space-y-3 empty:hidden">
        {count.isError && <Notice kind="error">{errorMessage(t, count.error)}</Notice>}
        {apply.isError && <Notice kind="error">{errorMessage(t, apply.error)}</Notice>}
        {apply.isSuccess && <Notice kind="success">{text.done(apply.data.changed)}</Notice>}
        {count.isSuccess && (
          <div className="flex flex-wrap items-center gap-3 rounded-lg border border-line bg-ink/50 p-3">
            <p className="text-sm text-zinc-100">
              {text.matched(count.data.matched.toLocaleString(language))}
            </p>
            {count.data.matched > 0 && (
              <div className="flex gap-2">
                <button
                  type="button"
                  className={buttonPrimary}
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(true)}
                >
                  {text.turnOn(count.data.matched.toLocaleString(language))}
                </button>
                <button
                  type="button"
                  className={buttonSecondary}
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(false)}
                >
                  {text.turnOff(count.data.matched.toLocaleString(language))}
                </button>
              </div>
            )}
          </div>
        )}
      </div>
    </Panel>
  )
}

const rowButton =
  'inline-flex min-h-9 shrink-0 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-60'
const iconButton =
  'inline-flex size-9 items-center justify-center rounded-lg border border-line bg-ink text-white transition-colors hover:border-fin-4 disabled:cursor-not-allowed disabled:opacity-40'
