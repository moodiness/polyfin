import { ArrowRightIcon, MagnifyingGlassIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
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
import { Block, cx, EmptyState, InlineError, Segmented, SkeletonRows, TextInput } from '@/ui'
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

  const paging = mappings.data !== undefined && {
    offset,
    limit: pageSize,
    total: mappings.data.total,
    busy: mappings.isFetching,
    onOffset: setOffset,
  }

  return (
    <Block title={text.title} count={mappings.data?.total}>
      <p className="mb-3 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      <div className="flex flex-wrap items-center gap-2.5">
        <Segmented
          label={text.show}
          size="compact"
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
          size="compact"
          icon={MagnifyingGlassIcon}
          value={search}
          placeholder={text.search}
          aria-label={text.search}
          autoComplete="off"
          onValue={(value) => {
            setSearch(value)
            setOffset(0)
          }}
          className="min-w-48 flex-1 md:ml-auto md:max-w-80"
        />
      </div>
      <p aria-live="polite" className="mt-4 text-small text-ok empty:hidden">
        {announcement}
      </p>
      {mappings.data !== undefined && mappings.data.total > 0 && (
        <div className="mt-4 mb-2 pl-5 max-sm:pl-4">{paging && <Pager {...paging} />}</div>
      )}
      {mappings.isPending ? (
        <div className="mt-4">
          <SkeletonRows rows={6} />
        </div>
      ) : mappings.isError ? (
        <InlineError
          className="mt-4"
          onRetry={() => void mappings.refetch()}
          retrying={mappings.isFetching}
        >
          {errorMessage(t, mappings.error)}
        </InlineError>
      ) : mappings.data.items.length === 0 ? (
        <EmptyState
          className="mt-4"
          icon={TelevisionSimpleIcon}
          title={filtered ? text.empty : t.lineup.channelsEmpty}
        />
      ) : (
        <ul
          aria-label={text.title}
          aria-busy={mappings.isFetching}
          className={cx(
            'grid grid-cols-[minmax(0,1fr)] gap-1.5 transition-opacity duration-160',
            mappings.isPlaceholderData && 'opacity-60',
          )}
        >
          {mappings.data.items.map((item) => (
            <li
              key={item.channelId}
              className="min-w-0 rounded-[0.625rem] bg-s1 py-1 pr-2 pl-3.5 transition-colors duration-300 ease-nuit hover:bg-s2 [contain-intrinsic-size:auto_3rem] [content-visibility:auto] max-sm:pl-3"
            >
              <div className="flex min-h-10 flex-wrap items-center gap-x-2.5 gap-y-1">
                <span className="figures w-8 shrink-0 text-right text-small text-ink-3">
                  {item.number}
                </span>
                <div className="min-w-0 flex-1 basis-40">
                  <p className="truncate text-control font-semibold text-ink" title={item.name}>
                    {item.name}
                  </p>
                  <p className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-micro text-ink-3">
                    {item.guideId !== '' && item.guideId !== item.mapping?.guideChannelId && (
                      <>
                        <span className="figures break-all">{item.guideId}</span>
                        <ArrowRightIcon size="0.75rem" aria-hidden="true" className="shrink-0" />
                      </>
                    )}
                    <MappingText mapping={item.mapping} />
                  </p>
                </div>
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
        <div className="mt-4 pl-5 max-sm:pl-4">{paging && <Pager {...paging} end />}</div>
      )}
    </Block>
  )
}
