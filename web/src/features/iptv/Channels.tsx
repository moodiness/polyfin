import {
  MagnifyingGlassIcon,
  PencilSimpleIcon,
  TelevisionSimpleIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
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
import { invalidateLineup } from '@/features/iptv/lineup'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  cx,
  Modal,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Panel,
  Select,
  SkeletonRows,
  Switch,
  TextInput,
  Tooltip,
  useToast,
  MoveButtons,
} from '@/ui'
import ChannelEditor from './ChannelEditor'
import { ChannelLogo, DragGrip, Pager, useDebounced, useNumber } from './shared'

/** Rows of one page: enough to scan, few enough to draw at once without delay. */
const pageSize = 100

type Tristate = 'any' | 'yes' | 'no'

const asFilter = (value: Tristate) => (value === 'any' ? undefined : value === 'yes')

/** A source's channels, a page at a time from the server, filtered, edited and reordered. */
export default function Channels({ scope, id }: { scope: Scope; id: string }) {
  const { t } = useI18n()
  const toast = useToast()
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
  const [keywordOpen, setKeywordOpen] = useState(false)
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
  const unfiltered =
    category === '' && q === '' && enabled === 'any' && shown === 'any' && mapped === 'any'
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
      toast(text.bulkDone(result.changed))
      setSelected(new Set())
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const allSelected = items.length > 0 && items.every((item) => selected.has(item.id))
  const someSelected = !allSelected && items.some((item) => selected.has(item.id))
  const pager = channels.data !== undefined && (
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
  )

  return (
    <div className="space-y-8">
      {keywordOpen && (
        <KeywordBulk
          scope={scope}
          id={id}
          category={category}
          onClose={() => setKeywordOpen(false)}
        />
      )}
      <Panel
        title={text.title}
        description={reorder ? text.reorderHelp : text.help}
        flush
        actions={
          <Button
            size="sm"
            className="max-sm:hidden"
            aria-expanded={keywordOpen}
            onClick={() => setKeywordOpen((open) => !open)}
          >
            {t.lineup.keyword.title}
          </Button>
        }
      >
        <div className="px-4 pt-4 sm:hidden">
          <Button
            size="sm"
            aria-expanded={keywordOpen}
            onClick={() => setKeywordOpen((open) => !open)}
          >
            {t.lineup.keyword.title}
          </Button>
        </div>
        <div className="grid gap-3 px-6 pt-5 pb-4 max-sm:px-4 md:grid-cols-2 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1.2fr)_repeat(3,minmax(0,0.8fr))]">
          <Field label={text.search}>
            <TextInput
              type="search"
              size="sm"
              icon={MagnifyingGlassIcon}
              value={search}
              autoComplete="off"
              onValue={(value) => refilter(() => setSearch(value))}
            />
          </Field>
          <Field label={text.category}>
            <Select
              className="h-9"
              value={category}
              options={[
                { value: '', label: text.allCategories },
                ...(categories.data?.items ?? []).map((c) => ({
                  value: c.id,
                  label: c.name || t.lineup.exclusions.noGroup,
                })),
              ]}
              onValue={(value) =>
                refilter(() => setParams(value ? { category: value } : {}, { replace: true }))
              }
            />
          </Field>
          <div className="grid grid-cols-3 gap-3 md:col-span-2 xl:col-span-3">
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
        </div>

        <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-y border-line bg-bg/40 px-6 py-2 max-sm:px-4">
          <Checkbox
            checked={allSelected}
            indeterminate={someSelected}
            disabled={items.length === 0}
            label={text.selectPage}
            className="text-control"
            onChange={(checked) =>
              setSelected((current) => {
                const next = new Set(current)
                for (const item of items) {
                  if (checked) next.add(item.id)
                  else next.delete(item.id)
                }
                return next
              })
            }
          />
          {selected.size === 0 ? (
            pager
          ) : (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="mr-1 text-control text-ink tabular-nums">
                {text.selected(selected.size)}
              </span>
              <Button size="sm" disabled={bulk.isPending} onClick={() => bulk.mutate(true)}>
                {text.turnOn}
              </Button>
              <Button size="sm" disabled={bulk.isPending} onClick={() => bulk.mutate(false)}>
                {text.turnOff}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
                {text.clearSelection}
              </Button>
            </div>
          )}
        </div>
        <p aria-live="polite" className="sr-only">
          {announcement}
        </p>
        {(move.isError || bulk.isError) && (
          <div className="px-6 pt-3 max-sm:px-4">
            <FieldError>{errorMessage(t, move.error ?? bulk.error)}</FieldError>
          </div>
        )}

        {channels.isPending ? (
          <SkeletonRows rows={8} boxed={false} />
        ) : channels.isError ? (
          <div className="p-6 max-sm:p-4">
            <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
              {errorMessage(t, channels.error)}
            </InlineError>
          </div>
        ) : items.length === 0 ? (
          <div className="p-6 max-sm:p-4">
            <EmptyState
              icon={TelevisionSimpleIcon}
              title={unfiltered ? t.lineup.channelsEmpty : text.noMatch}
            >
              {unfiltered ? t.lineup.channelsEmptyHelp : undefined}
            </EmptyState>
          </div>
        ) : (
          <ul
            aria-label={text.title}
            aria-busy={channels.isFetching}
            className={cx(
              'transition-opacity duration-160',
              channels.isPlaceholderData && 'opacity-60',
            )}
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
        {channels.data !== undefined && channels.data.total > 0 && (
          <div className="border-t border-line px-6 py-3 max-sm:px-4">{pager}</div>
        )}
      </Panel>
      <Modal
        open={editing !== null}
        onClose={() => setEditing(null)}
        title={editing ? t.lineup.editor.title(editing.name) : ''}
        width={640}
      >
        {editing && <ChannelEditor key={editing.id} scope={scope} id={id} channelId={editing.id} />}
      </Modal>
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
  return (
    <Field label={label}>
      <Select
        className="h-9"
        value={value}
        options={[
          { value: 'any', label: t.lineup.channels.any },
          { value: 'yes', label: yes },
          { value: 'no', label: no },
        ]}
        onValue={onChange}
      />
    </Field>
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
  const noGuide = channel.mapping === null || channel.mapping.guideChannelId === null
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
      className={cx(
        'flex min-h-16 items-center gap-3 px-6 py-2.5 not-first:border-t not-first:border-line transition-colors duration-160 max-sm:gap-2.5 max-sm:px-4',
        dragging && 'opacity-40',
        selected ? 'bg-accent/8' : 'hover:bg-s2/60',
      )}
    >
      <Checkbox
        checked={selected}
        onChange={onSelect}
        label={text.select(channel.name)}
        hideLabel
        className="w-auto flex-none"
      />
      {reorder && <DragGrip name={channel.name} />}
      <span className="figures hidden w-12 shrink-0 text-right text-[12.5px] text-ink-3 sm:inline">
        {channel.number ?? '–'}
      </span>
      <ChannelLogo id={channel.id} logo={channel.logo} name={channel.name} />
      <div className="min-w-0 flex-1">
        <p className="flex min-w-0 items-center gap-2">
          <span
            className={cx(
              'truncate text-[14px] font-medium',
              channel.enabled ? 'text-ink' : 'text-ink-2',
            )}
            title={channel.name}
          >
            {channel.name}
          </span>
          <span className="hidden shrink-0 gap-1 md:flex">
            {channel.enabled && !channel.shown && (
              <Tooltip content={text.hiddenHelp}>
                <span tabIndex={0} className="rounded-full">
                  <Badge tone="warn">{text.hidden}</Badge>
                </span>
              </Tooltip>
            )}
            {noGuide && <Badge>{text.unmapped}</Badge>}
            {channel.mapping?.manual && <Badge tone="accent">{text.manual}</Badge>}
            {channel.streams.length > 1 && (
              <Badge>
                <span className="tabular-nums">{text.streamCount(channel.streams.length)}</span>
              </Badge>
            )}
          </span>
        </p>
        <p className="truncate text-[12.5px] text-ink-3" title={channel.providerName}>
          {channel.number !== null && (
            <span className="sm:hidden">
              <span className="figures">{channel.number}</span> ·{' '}
            </span>
          )}
          {channel.category.name || t.lineup.exclusions.noGroup}
          {channel.renamed && ` · ${channel.providerName}`}
          {noGuide && <span className="md:hidden"> · {text.unmapped}</span>}
          {channel.enabled && !channel.shown && (
            <span className="text-warn md:hidden"> · {text.hidden}</span>
          )}
        </p>
      </div>
      <Switch
        label={text.enabledLabel(channel.name)}
        checked={channel.enabled}
        disabled={toggle.isPending}
        onChange={(on) => toggle.mutate(on)}
      />
      {reorder && (
        <span className="hidden sm:flex">
          <MoveButtons
            name={channel.name}
            index={position}
            count={total}
            disabled={busy}
            onMove={onMove}
          />
        </span>
      )}
      <IconButton
        size="sm"
        icon={PencilSimpleIcon}
        label={text.editLabel(channel.name)}
        onClick={onEdit}
      />
    </li>
  )
}

/** Turns on or off every channel matching a keyword, after counting them. */
function KeywordBulk({
  scope,
  id,
  category,
  onClose,
}: {
  scope: Scope
  id: string
  category: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const toast = useToast()
  const number = useNumber()
  const text = t.lineup.keyword
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
    onSuccess: (result) => {
      count.reset()
      toast(text.done(result.changed))
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const tooShort = keyword.trim().length < 2

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    apply.reset()
    if (!tooShort) count.mutate()
  }

  return (
    <Panel
      title={text.title}
      description={text.help}
      actions={<IconButton icon={XIcon} label={t.lineup.close} onClick={onClose} />}
    >
      <form onSubmit={submit} noValidate className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <Field label={text.label} help={text.hint} className="flex-1">
          <TextInput
            value={keyword}
            autoComplete="off"
            autoFocus
            onValue={(value) => {
              count.reset()
              apply.reset()
              setKeyword(value)
            }}
          />
        </Field>
        <Button
          type="submit"
          className="h-10 sm:mt-[30px]"
          loading={count.isPending}
          disabled={tooShort}
        >
          {count.isPending ? text.counting : text.count}
        </Button>
      </form>
      {category !== '' && (
        <Checkbox
          className="mt-4"
          checked={inCategory}
          label={text.inCategory}
          onChange={(checked) => {
            count.reset()
            setInCategory(checked)
          }}
        />
      )}
      <div aria-live="polite" className="mt-4 space-y-3 empty:hidden">
        {count.isError && <FieldError>{errorMessage(t, count.error)}</FieldError>}
        {apply.isError && <FieldError>{errorMessage(t, apply.error)}</FieldError>}
        {apply.isSuccess && <p className="text-small text-ok">{text.done(apply.data.changed)}</p>}
        {count.isSuccess && (
          <div className="flex flex-wrap items-center gap-3 rounded-row border border-line-2 bg-bg px-4 py-3">
            <p className="mr-auto text-control text-ink tabular-nums">
              {text.matched(number(count.data.matched))}
            </p>
            {count.data.matched > 0 && (
              <div className="flex gap-2">
                <Button
                  variant="primary"
                  loading={apply.isPending && apply.variables === true}
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(true)}
                >
                  {text.turnOn(number(count.data.matched))}
                </Button>
                <Button
                  loading={apply.isPending && apply.variables === false}
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(false)}
                >
                  {text.turnOff(number(count.data.matched))}
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </Panel>
  )
}
