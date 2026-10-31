import { ListNumbersIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  fetchEpisodeOrder,
  fetchEpisodeOrders,
  queryKeys,
  saveEpisodeOrder,
  searchOrderedSeries,
  type EpisodeOrder,
  type OrderedSeries,
  type SpecialsPlacement,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  Row,
  RowList,
  Segmented,
  Select,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'

/**
 * Under Content › Libraries: the series shown in an order of their own, as Jellyfin's DisplayOrder,
 * and a search for others. Each opens an editor choosing one of the series' TMDB episode groups,
 * or the aired order, and where its specials show.
 */
export default function EpisodeOrders() {
  const { t } = useI18n()
  const e = t.episodeOrders
  const [editing, setEditing] = useState<OrderedSeries | null>(null)
  const [query, setQuery] = useState('')
  const [asked, setAsked] = useState('')
  const orders = useQuery({
    queryKey: queryKeys.episodeOrders,
    queryFn: ({ signal }) => fetchEpisodeOrders(signal),
  })
  const found = useQuery({
    queryKey: queryKeys.orderedSeries(asked),
    queryFn: ({ signal }) => searchOrderedSeries(asked, signal),
    enabled: asked !== '',
    staleTime: 60_000,
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = query.trim()
    if (next === '') return
    if (next === asked) void found.refetch()
    else setAsked(next)
  }

  let list
  if (orders.isPending) list = <SkeletonRows rows={2} label={e.loading} />
  else if (orders.isError)
    list = (
      <InlineError onRetry={() => void orders.refetch()} retrying={orders.isFetching}>
        {errorMessage(t, orders.error)}
      </InlineError>
    )
  else if (orders.data.length === 0)
    list = (
      <EmptyState icon={ListNumbersIcon} title={e.emptyTitle}>
        {e.emptyHow}
      </EmptyState>
    )
  else
    list = (
      <RowList variant="boxed" aria-label={e.title}>
        {orders.data.map((order) => (
          <Row
            key={order.series.id}
            leading={ListNumbersIcon}
            onClick={() => setEditing(order.series)}
            title={order.series.name}
            titleAside={order.series.year > 0 && <Badge>{order.series.year}</Badge>}
            meta={summary(order)}
          />
        ))}
      </RowList>
    )

  function summary(order: EpisodeOrder): string {
    const parts = [
      Object.hasOwn(e.types, order.order) ? e.types[order.order as keyof typeof e.types] : e.aired,
    ]
    if (order.specials !== 'first') parts.push(e.specialsSummary[order.specials])
    return parts.join(' · ')
  }

  return (
    <Block title={e.title} count={orders.data?.length}>
      <p className="mb-4 max-w-[70ch] text-small text-ink-3">{e.help}</p>
      {list}
      <form
        onSubmit={submit}
        noValidate
        className="mt-5 flex max-w-[60ch] flex-wrap items-end gap-2"
      >
        <Field label={e.search} className="min-w-[220px] flex-1">
          <TextInput
            icon={MagnifyingGlassIcon}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={e.searchPlaceholder}
          />
        </Field>
        <Button type="submit" loading={found.isFetching}>
          {e.searchButton}
        </Button>
      </form>
      {asked !== '' && (
        <div className="mt-3 max-w-[60ch]">
          {found.isPending ? (
            <SkeletonRows rows={2} label={e.searching} />
          ) : found.isError ? (
            <InlineError onRetry={() => void found.refetch()} retrying={found.isFetching}>
              {errorMessage(t, found.error)}
            </InlineError>
          ) : found.data.length === 0 ? (
            <p className="text-small text-ink-3">{e.noSeries}</p>
          ) : (
            <RowList variant="boxed" aria-label={e.results}>
              {found.data.map((series) => (
                <Row
                  key={series.id}
                  onClick={() => setEditing(series)}
                  title={series.name}
                  titleAside={series.year > 0 && <Badge>{series.year}</Badge>}
                />
              ))}
            </RowList>
          )}
        </div>
      )}
      {editing && (
        <OrderEditor key={editing.id} series={editing} onClose={() => setEditing(null)} />
      )}
    </Block>
  )
}

/** The editor of one series' order, in a modal. */
function OrderEditor({ series, onClose }: { series: OrderedSeries; onClose: () => void }) {
  const { t } = useI18n()
  const e = t.episodeOrders
  const toast = useToast()
  const client = useQueryClient()
  const order = useQuery({
    queryKey: queryKeys.episodeOrder(series.id),
    queryFn: ({ signal }) => fetchEpisodeOrder(series.id, signal),
    staleTime: 0,
  })
  const [group, setGroup] = useState<string | null>(null)
  const [specials, setSpecials] = useState<SpecialsPlacement | null>(null)
  const save = useMutation({
    mutationFn: (choice: { group: string; specials: SpecialsPlacement }) =>
      saveEpisodeOrder(series.id, choice.group, choice.specials),
    onSuccess: (saved) => {
      client.setQueryData(queryKeys.episodeOrder(series.id), saved)
      void client.invalidateQueries({ queryKey: queryKeys.episodeOrders })
      toast(e.saved(series.name), { tone: 'ok' })
      onClose()
    },
  })

  const data = order.data
  // A DisplayOrder set from a Jellyfin app shows the first group of its type, as the server reads it.
  const current = data ? data.group || data.groups.find((g) => g.type === data.order)?.id || '' : ''
  const chosenGroup = group ?? current
  const chosenSpecials = specials ?? data?.specials ?? 'first'

  let body
  if (order.isPending) body = <SkeletonRows rows={2} label={e.loading} />
  else if (order.isError)
    body = (
      <InlineError onRetry={() => void order.refetch()} retrying={order.isFetching}>
        {errorMessage(t, order.error)}
      </InlineError>
    )
  else
    body = (
      <div className="flex flex-col gap-5">
        <Field label={e.order} help={e.orderHelp}>
          <Select
            value={chosenGroup}
            onValue={setGroup}
            options={[
              { value: '', label: e.aired },
              ...order.data.groups.map((g) => ({
                value: g.id,
                label: `${Object.hasOwn(e.types, g.type) ? e.types[g.type] : g.type} · ${g.name} · ${e.counts(g.seasons, g.episodes)}`,
              })),
            ]}
          />
        </Field>
        {order.data.lookup !== '' && <Notice tone="warn">{e.lookup[order.data.lookup]}</Notice>}
        <Field label={e.specials} help={e.specialsHelp}>
          <Segmented
            label={e.specials}
            value={chosenSpecials}
            onChange={setSpecials}
            options={[
              { value: 'first', label: e.specialsFirst },
              { value: 'last', label: e.specialsLast },
              { value: 'hidden', label: e.specialsHidden },
            ]}
          />
        </Field>
        {save.isError && <Notice tone="danger">{errorMessage(t, save.error)}</Notice>}
      </div>
    )

  return (
    <Modal
      open
      onClose={onClose}
      title={e.editTitle(series.name)}
      width={560}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            loading={save.isPending}
            disabled={!order.data}
            onClick={() => save.mutate({ group: chosenGroup, specials: chosenSpecials })}
          >
            {t.common.save}
          </Button>
        </>
      }
    >
      {body}
    </Modal>
  )
}
