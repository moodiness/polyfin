import { MagnifyingGlassIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
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
import { Button, cx, EmptyState, Field, InlineError, Select, SkeletonRows, TextInput } from '@/ui'
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
    <section
      aria-labelledby={titleId}
      className="mt-3 space-y-4 rounded-row border border-accent/35 bg-bg p-4 max-sm:p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h3 id={titleId} className="min-w-0 truncate text-[14px] font-semibold text-ink">
          {text.title(channelName)}
        </h3>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          {t.common.cancel}
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Field label={text.search} hideLabel>
          <TextInput
            type="search"
            size="sm"
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
        <Field label={text.guide} hideLabel>
          <Select
            className="h-9"
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
        <SkeletonRows rows={3} />
      ) : channels.isError ? (
        <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
          {errorMessage(t, channels.error)}
        </InlineError>
      ) : channels.data.items.length === 0 ? (
        <EmptyState icon={TelevisionSimpleIcon} title={text.empty}>
          {guides.data?.guides.length === 0 ? text.noGuides : undefined}
        </EmptyState>
      ) : (
        <ul
          aria-busy={channels.isFetching}
          className={cx(
            'max-h-[50dvh] overflow-y-auto rounded-row border border-line-2 bg-s1 transition-opacity',
            channels.isPlaceholderData && 'opacity-60',
          )}
        >
          {channels.data.items.map((channel) => {
            const others = channel.names.filter((name) => name !== channel.name)
            return (
              <li
                key={`${channel.guideId}:${channel.id}`}
                className="flex items-start gap-3 px-3.5 py-3 not-first:border-t not-first:border-line"
              >
                <GuideIcon icon={channel.icon} name={channel.name} />
                <div className="min-w-0 flex-1 text-small">
                  <p className="text-[14px] font-medium break-words text-ink">{channel.name}</p>
                  {others.length > 0 && (
                    <p className="break-words text-ink-3">{text.alsoNamed(others.join(' · '))}</p>
                  )}
                  <p className="figures break-all text-ink-3">
                    {channel.id} · {text.guideN(channel.guidePosition)}
                  </p>
                  <p className="text-ink-2">
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
  const frame = 'size-10 shrink-0 overflow-hidden rounded-field border border-line bg-s3'
  if (!icon || failed) {
    return (
      <span
        aria-hidden="true"
        className={cx(frame, 'inline-grid place-items-center text-small font-semibold text-ink-3')}
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
      className={cx(frame, 'object-contain p-0.5')}
    />
  )
}
