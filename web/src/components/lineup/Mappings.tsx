import { useId, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import {
  fetchMappings,
  queryClient,
  queryKeys,
  type CatalogTarget,
  type MappingItem,
  type MappingState,
  type Page,
  type Scope,
} from '@/api'
import { icons } from '@/components/icons'
import { MappingControls, mappingWords, MappingText } from '@/components/lineup/MappingControls'
import { Pager, Segmented, smallField, useDebounced } from '@/components/lineup/shared'
import { Empty, Panel, Skeleton } from '@/components/panels'
import { Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

const pageSize = 100

/** A catalog's channels with the guide channel each takes, mapped by hand when needed. */
export default function Mappings({
  scope,
  target,
  onChanged,
}: {
  scope: Scope
  target: CatalogTarget
  onChanged?: () => void
}) {
  const { t } = useI18n()
  const text = t.lineup.mapping
  const [state, setState] = useState<MappingState>('all')
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [announcement, setAnnouncement] = useState('')
  const searchId = useId()
  const q = useDebounced(search.trim())
  const key = [...queryKeys.catalogGuides(scope, target), 'mappings', state, q, offset]
  const mappings = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchMappings(scope, target, { state, q }, offset, pageSize, signal),
    placeholderData: keepPreviousData,
  })

  function changed(item: MappingItem) {
    setAnnouncement(text.changed(item.name, mappingWords(t, item)))
    queryClient.setQueryData<Page<MappingItem>>(key, (page) =>
      page
        ? { ...page, items: page.items.map((i) => (i.channelId === item.channelId ? item : i)) }
        : page,
    )
    // The counts and the line-up follow; this page keeps the row until the next read.
    void queryClient.invalidateQueries({
      queryKey: queryKeys.catalogGuides(scope, target),
      predicate: (query) => !query.queryKey.includes('mappings'),
    })
    onChanged?.()
  }

  return (
    <Panel title={text.title} description={text.help}>
      <div className="flex flex-col gap-3 md:flex-row md:items-end">
        <Segmented
          label={text.show}
          value={state}
          options={[
            { value: 'all', label: text.states.all },
            { value: 'mapped', label: text.states.mapped },
            { value: 'unmapped', label: text.states.unmapped },
            { value: 'manual', label: text.states.manual },
          ]}
          onChange={(value) => {
            setState(value)
            setOffset(0)
          }}
        />
        <div className="relative flex-1 md:max-w-sm">
          <label htmlFor={searchId} className="mb-1.5 block text-xs font-medium text-muted">
            {text.search}
          </label>
          <icons.search className="pointer-events-none absolute bottom-2.5 left-3 size-4 text-muted" />
          <input
            id={searchId}
            type="search"
            value={search}
            autoComplete="off"
            onChange={(event) => {
              setSearch(event.target.value)
              setOffset(0)
            }}
            className={`${smallField} pl-9`}
          />
        </div>
      </div>
      <p aria-live="polite" className="mt-3 text-xs text-emerald-300 empty:hidden">
        {announcement}
      </p>
      <div className="mt-4">
        {mappings.isPending ? (
          <Skeleton rows={6} label={t.common.loading} />
        ) : mappings.isError ? (
          <Notice kind="error">{errorMessage(t, mappings.error)}</Notice>
        ) : mappings.data.items.length === 0 ? (
          <Empty>{text.empty}</Empty>
        ) : (
          <ul
            aria-busy={mappings.isFetching}
            className={`divide-y divide-line rounded-xl border border-line transition-opacity ${
              mappings.isPlaceholderData ? 'opacity-60' : ''
            }`}
          >
            {mappings.data.items.map((item) => (
              <li
                key={item.channelId}
                className="grid gap-2 p-3 text-sm lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]"
              >
                <div className="min-w-0">
                  <p className="flex items-baseline gap-2">
                    <span className="w-10 shrink-0 text-right text-xs text-muted tabular-nums">
                      {item.number ?? '–'}
                    </span>
                    <span className="truncate font-medium text-white">{item.name}</span>
                  </p>
                  {item.guideId !== '' && (
                    <p className="pl-12 font-mono text-xs break-all text-muted">{item.guideId}</p>
                  )}
                </div>
                <div className="min-w-0 space-y-2">
                  <p className="text-sm">
                    <MappingText mapping={item.mapping} />
                  </p>
                  <MappingControls
                    scope={scope}
                    target={target}
                    channelId={item.channelId}
                    channelName={item.name}
                    mapping={item.mapping}
                    onChanged={changed}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
      {mappings.data !== undefined && (
        <div className="mt-4">
          <Pager
            offset={offset}
            limit={pageSize}
            total={mappings.data.total}
            busy={mappings.isFetching}
            onOffset={setOffset}
          />
        </div>
      )}
    </Panel>
  )
}
