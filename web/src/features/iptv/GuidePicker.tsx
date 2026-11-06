import { MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useId, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import {
  fetchCatalogGuides,
  fetchGuideChannels,
  queryKeys,
  type CatalogTarget,
  type GuideChannel,
  type Scope,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, cx, Field, InlineError, Select, SkeletonRows, TextInput } from '@/ui'
import { Pager, useDebounced } from './shared'

const pageSize = 50

/**
 * Searches the channels of a catalog's guides, to map a channel to one by hand. The search starts
 * from the channel's name; each result shows its names, id, guide, icon and what airs now.
 */
export default function GuidePicker({
  scope,
  target,
  channelName,
  busy = false,
  onPick,
  onCancel,
}: {
  scope: Scope
  target: CatalogTarget
  channelName: string
  busy?: boolean
  onPick: (channel: GuideChannel) => void
  onCancel: () => void
}) {
  const { language, t } = useI18n()
  const text = t.lineup.picker
  const [search, setSearch] = useState(channelName)
  const [guide, setGuide] = useState('')
  const [offset, setOffset] = useState(0)
  const titleId = useId()
  const q = useDebounced(search.trim())
  const guides = useQuery({
    queryKey: queryKeys.catalogGuides(scope, target),
    queryFn: ({ signal }) => fetchCatalogGuides(scope, target, signal),
  })
  const channels = useQuery({
    queryKey: [...queryKeys.catalogGuides(scope, target), 'channels', q, guide, offset],
    queryFn: ({ signal }) =>
      fetchGuideChannels(scope, target, { q, guide: guide || undefined }, offset, pageSize, signal),
    placeholderData: keepPreviousData,
  })
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })

  return (
    <section aria-labelledby={titleId} className="mt-2 mb-3 min-w-0 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h3 id={titleId} className="min-w-0 text-control font-semibold break-words text-ink">
          {text.title(channelName)}
        </h3>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          {t.common.cancel}
        </Button>
      </div>
      <div className="flex flex-wrap gap-2.5">
        <Field label={text.search} hideLabel className="min-w-48 flex-[2_1_19rem]">
          <TextInput
            type="search"
            size="compact"
            icon={MagnifyingGlassIcon}
            value={search}
            autoFocus
            autoComplete="off"
            placeholder={text.search}
            onValue={(value) => {
              setSearch(value)
              setOffset(0)
            }}
          />
        </Field>
        <Field label={text.guide} hideLabel className="min-w-48 flex-[1_1_14.25rem]">
          <Select
            size="compact"
            value={guide}
            options={[
              { value: '', label: text.allGuides },
              ...(guides.data?.guides ?? []).map((g) => ({
                value: g.id,
                label: `${text.guideN(g.position)} · ${g.url}`,
              })),
            ]}
            onValue={(value) => {
              setGuide(value)
              setOffset(0)
            }}
          />
        </Field>
      </div>
      {channels.isPending ? (
        <SkeletonRows rows={3} boxed={false} />
      ) : channels.isError ? (
        <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
          {errorMessage(t, channels.error)}
        </InlineError>
      ) : channels.data.items.length === 0 ? (
        <p className="py-4 text-small text-ink-2">
          {text.empty}
          {guides.data?.guides.length === 0 && ` ${text.noGuides}`}
        </p>
      ) : (
        <ul
          aria-busy={channels.isFetching}
          className={cx(
            '-mx-3 grid max-h-[50dvh] grid-cols-[minmax(0,1fr)] gap-1 overflow-y-auto overscroll-contain transition-opacity',
            channels.isPlaceholderData && 'opacity-60',
          )}
        >
          {channels.data.items.map((channel) => {
            const others = channel.names.filter((name) => name !== channel.name)
            return (
              <li
                key={`${channel.guideId}:${channel.id}`}
                className="flex min-w-0 items-start gap-3 rounded-[1rem] px-3 py-2.5 transition-colors duration-300 ease-nuit hover:bg-s2"
              >
                <GuideIcon icon={channel.icon} name={channel.name} />
                <div className="min-w-0 flex-1">
                  <p className="text-control font-semibold break-words text-ink">{channel.name}</p>
                  {others.length > 0 && (
                    <p className="text-micro break-words text-ink-3">
                      {text.alsoNamed(others.join(' · '))}
                    </p>
                  )}
                  <p className="figures text-micro break-all text-ink-3">
                    {channel.id} · {text.guideN(channel.guidePosition)}
                  </p>
                  <p className="mt-0.5 text-small text-ink-2">
                    {channel.now
                      ? text.now(
                          channel.now.title,
                          time.format(new Date(channel.now.start)),
                          time.format(new Date(channel.now.end)),
                        )
                      : text.nothingNow}
                  </p>
                </div>
                <Button
                  size="sm"
                  disabled={busy}
                  aria-label={text.chooseLabel(channel.name)}
                  onClick={() => onPick(channel)}
                >
                  {text.choose}
                </Button>
              </li>
            )
          })}
        </ul>
      )}
      {channels.data !== undefined && (
        <Pager
          offset={offset}
          limit={pageSize}
          total={channels.data.total}
          busy={channels.isFetching}
          onOffset={setOffset}
        />
      )}
    </section>
  )
}

function GuideIcon({ icon, name }: { icon: string | null; name: string }) {
  const [failed, setFailed] = useState(false)
  const frame = 'size-9 shrink-0 overflow-hidden rounded-[0.75rem] bg-s2'
  if (!icon || failed) {
    return (
      <span
        aria-hidden="true"
        className={cx(frame, 'inline-grid place-items-center text-small font-bold text-ink')}
      >
        {name.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return (
    <img
      src={icon}
      alt=""
      loading="lazy"
      referrerPolicy="no-referrer"
      onError={() => setFailed(true)}
      className={cx(frame, 'object-contain p-1')}
    />
  )
}
