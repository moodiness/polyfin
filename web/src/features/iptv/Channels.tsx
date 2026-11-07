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
  Block,
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
  // The source page links here with ?archive=yes, to the channels Replay lists.
  const [archive, setArchive] = useState<Tristate>(params.get('archive') === 'yes' ? 'yes' : 'any')
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
    archive: asFilter(archive),
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
  const noFilter =
    q === '' && enabled === 'any' && shown === 'any' && mapped === 'any' && archive === 'any'
  const unfiltered = category === '' && noFilter
  // Channels move within their category: only while it alone is listed, in its order.
  const reorder = category !== '' && noFilter

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
  const paging = channels.data !== undefined && {
    offset,
    limit: pageSize,
    total: channels.data.total,
    busy: channels.isFetching,
    onOffset: (next: number) => {
      setOffset(next)
      setSelected(new Set())
    },
  }

  return (
    <div className="space-y-8 max-md:space-y-7">
      {keywordOpen && (
        <KeywordBulk
          scope={scope}
          id={id}
          category={category}
          onClose={() => setKeywordOpen(false)}
        />
      )}
      <Block title={text.title} count={channels.data?.total}>
        <div className="mb-3 flex flex-wrap items-start justify-between gap-x-5 gap-y-2">
          <p className="max-w-[66ch] text-small text-ink-2">
            {reorder ? text.reorderHelp : text.help}
          </p>
          <Button
            size="sm"
            aria-expanded={keywordOpen}
            onClick={() => setKeywordOpen((open) => !open)}
          >
            {t.lineup.keyword.title}
          </Button>
        </div>
        <div className="flex flex-wrap items-end gap-2.5">
          <Field label={text.search} className="min-w-56 flex-[2_1_16.625rem]">
            <TextInput
              type="search"
              size="compact"
              icon={MagnifyingGlassIcon}
              value={search}
              autoComplete="off"
              onValue={(value) => refilter(() => setSearch(value))}
            />
          </Field>
          <Field label={text.category} className="min-w-48 flex-[1_1_14.25rem]">
            <Select
              size="compact"
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
          <TristateSelect
            label={text.archiveFilter}
            value={archive}
            yes={text.archived}
            no={text.notArchived}
            onChange={(value) => refilter(() => setArchive(value))}
          />
        </div>

        <div className="mt-3.5 mb-1.5 flex min-h-9 flex-wrap items-center justify-between gap-x-4 gap-y-1.5 pl-3.5">
          <Checkbox
            checked={allSelected}
            indeterminate={someSelected}
            disabled={items.length === 0}
            label={text.selectPage}
            className="w-auto"
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
            paging && <Pager {...paging} />
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <span className="figures mr-1 text-control text-ink">
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
          <div className="mb-3">
            <FieldError>{errorMessage(t, move.error ?? bulk.error)}</FieldError>
          </div>
        )}

        {channels.isPending ? (
          <SkeletonRows rows={8} />
        ) : channels.isError ? (
          <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
            {errorMessage(t, channels.error)}
          </InlineError>
        ) : items.length === 0 ? (
          <EmptyState
            icon={TelevisionSimpleIcon}
            title={unfiltered ? t.lineup.channelsEmpty : text.noMatch}
          >
            {unfiltered ? t.lineup.channelsEmptyHelp : undefined}
          </EmptyState>
        ) : (
          <ul
            aria-label={text.title}
            aria-busy={channels.isFetching}
            className={cx(
              'grid grid-cols-[minmax(0,1fr)] gap-1.5 transition-opacity duration-160',
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
          <div className="mt-4 pl-4">{paging && <Pager {...paging} end />}</div>
        )}
      </Block>
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
    <Field label={label} className="min-w-36 flex-1 basis-36 max-sm:basis-[40%]">
      <Select
        size="compact"
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
        'flex min-h-11 min-w-0 items-center gap-2.5 rounded-[0.625rem] py-1 pr-2 pl-3.5 transition-colors duration-300 ease-nuit [contain-intrinsic-size:auto_3rem] [content-visibility:auto] max-sm:gap-2 max-sm:pr-1.5',
        dragging && 'opacity-40',
        selected ? 'bg-accent/14' : 'bg-s1 hover:bg-s2',
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
      <span className="figures hidden w-10 shrink-0 text-right text-small text-ink-3 sm:inline">
        {channel.number ?? '–'}
      </span>
      <ChannelLogo id={channel.id} logo={channel.logo} name={channel.name} />
      <div className="min-w-0 flex-1">
        <p
          className={cx(
            'truncate text-control font-semibold',
            channel.enabled ? 'text-ink' : 'text-ink-2',
          )}
          title={channel.name}
        >
          {channel.name}
        </p>
        <p className="truncate text-micro text-ink-3" title={channel.providerName}>
          {channel.number !== null && (
            <span className="sm:hidden">
              <span className="figures">{channel.number}</span> ·{' '}
            </span>
          )}
          {channel.category.name || t.lineup.exclusions.noGroup}
          {channel.renamed && ` · ${channel.providerName}`}
          {noGuide && <span className="md:hidden"> · {text.unmapped}</span>}
          {channel.archiveDays > 0 && (
            <span className="md:hidden"> · {text.archiveDays(channel.archiveDays)}</span>
          )}
          {channel.enabled && !channel.shown && (
            <span className="text-warn md:hidden"> · {text.hidden}</span>
          )}
        </p>
      </div>
      <span className="hidden max-w-[45%] shrink-0 flex-wrap justify-end gap-1.5 md:flex">
        {channel.enabled && !channel.shown && (
          <Tooltip content={text.hiddenHelp}>
            <span tabIndex={0} className="rounded-full focus-lit">
              <Badge tone="warn">{text.hidden}</Badge>
            </span>
          </Tooltip>
        )}
        {noGuide && <Badge>{text.unmapped}</Badge>}
        {channel.mapping?.manual && <Badge tone="accent">{text.manual}</Badge>}
        {channel.archiveDays > 0 && (
          <Tooltip content={text.archiveHelp}>
            <span tabIndex={0} className="rounded-full focus-lit">
              <Badge>
                <span className="figures">{text.archiveDays(channel.archiveDays)}</span>
              </Badge>
            </span>
          </Tooltip>
        )}
        {channel.streams.length > 1 && (
          <Badge>
            <span className="figures">{text.streamCount(channel.streams.length)}</span>
          </Badge>
        )}
      </span>
      <Switch
        label={text.enabledLabel(channel.name)}
        checked={channel.enabled}
        disabled={toggle.isPending}
        onChange={(on) => toggle.mutate(on)}
        className="max-sm:p-1 max-sm:[&>span:first-child]:hidden"
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
      <form onSubmit={submit} noValidate className="flex flex-wrap items-start gap-x-2 gap-y-2.5">
        <Field
          label={text.label}
          help={text.hint}
          className="min-w-48 flex-1 max-sm:basis-full sm:max-w-[36rem]"
        >
          <TextInput
            size="sm"
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
          className="mt-7 max-sm:mt-0"
          loading={count.isPending}
          disabled={tooShort}
        >
          {count.isPending ? text.counting : text.count}
        </Button>
      </form>
      {category !== '' && (
        <Checkbox
          className="mt-3"
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
          <div className="flex flex-wrap items-center gap-3 pt-2">
            <p className="figures mr-auto text-lead text-ink">
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
