import { MagnifyingGlassIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
import { useState } from 'react'
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
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { cx, EmptyState, InlineError, Panel, Segmented, SkeletonRows, TextInput } from '@/ui'
import { MappingControls, mappingWords, MappingText } from './MappingControls'
import { Pager, useDebounced } from './shared'

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
  const q = useDebounced(search.trim())
  const key = [...queryKeys.catalogGuides(scope, target), 'mappings', state, q, offset]
  const mappings = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchMappings(scope, target, { state, q }, offset, pageSize, signal),
    placeholderData: keepPreviousData,
  })
  const filtered = state !== 'all' || q !== ''

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

  const pager = mappings.data !== undefined && (
    <Pager
      offset={offset}
      limit={pageSize}
      total={mappings.data.total}
      busy={mappings.isFetching}
      onOffset={setOffset}
    />
  )

  return (
    <Panel title={text.title} description={text.help} flush>
      <div className="flex flex-wrap items-center gap-3 px-6 pt-5 pb-4 max-sm:px-4">
        <Segmented
          className="scrollbar-none max-w-full overflow-x-auto [&>button]:shrink-0 [&>button]:whitespace-nowrap"
          label={text.show}
          size="sm"
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
        <TextInput
          type="search"
          size="sm"
          icon={MagnifyingGlassIcon}
          value={search}
          placeholder={text.search}
          aria-label={text.search}
          autoComplete="off"
          onValue={(value) => {
            setSearch(value)
            setOffset(0)
          }}
          className="min-w-48 flex-1 md:ml-auto md:max-w-72"
        />
      </div>
      <p aria-live="polite" className="px-6 pb-3 text-small text-ok empty:hidden max-sm:px-4">
        {announcement}
      </p>
      {mappings.data !== undefined && mappings.data.total > 0 && (
        <div className="border-t border-line px-6 py-2.5 max-sm:px-4">{pager}</div>
      )}
      {mappings.isPending ? (
        <div className="border-t border-line">
          <SkeletonRows rows={6} boxed={false} />
        </div>
      ) : mappings.isError ? (
        <div className="px-6 pb-6 max-sm:px-4">
          <InlineError onRetry={() => void mappings.refetch()} retrying={mappings.isFetching}>
            {errorMessage(t, mappings.error)}
          </InlineError>
        </div>
      ) : mappings.data.items.length === 0 ? (
        <div className="px-6 pb-6 max-sm:px-4">
          <EmptyState
            icon={TelevisionSimpleIcon}
            title={filtered ? text.empty : t.lineup.channelsEmpty}
          />
        </div>
      ) : (
        <ul
          aria-label={text.title}
          aria-busy={mappings.isFetching}
          className={cx(
            'border-t border-line transition-opacity duration-160',
            mappings.isPlaceholderData && 'opacity-60',
          )}
        >
          {mappings.data.items.map((item) => (
            <li
              key={item.channelId}
              className="grid gap-x-6 gap-y-2 px-6 py-3.5 not-first:border-t not-first:border-line [contain-intrinsic-size:auto_4.5rem] [content-visibility:auto] max-sm:px-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]"
            >
              <div className="flex min-w-0 items-baseline gap-3">
                <span className="figures w-10 shrink-0 text-right text-[12.5px] text-ink-3">
                  {item.number ?? '–'}
                </span>
                <div className="min-w-0">
                  <p className="truncate text-[14px] font-medium text-ink" title={item.name}>
                    {item.name}
                  </p>
                  {item.guideId !== '' && (
                    <p className="figures text-small break-all text-ink-3">{item.guideId}</p>
                  )}
                </div>
              </div>
              <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 max-lg:pl-[52px]">
                <p className="min-w-0 text-control">
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
      {mappings.data !== undefined && mappings.data.total > 0 && (
        <div className="border-t border-line px-6 py-3 max-sm:px-4">{pager}</div>
      )}
    </Panel>
  )
}
