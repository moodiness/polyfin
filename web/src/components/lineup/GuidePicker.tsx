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
import { icons } from '@/components/icons'
import { Pager, smallField, useDebounced } from '@/components/lineup/shared'
import { Empty, Skeleton } from '@/components/panels'
import { buttonSecondary, Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

const pageSize = 50

/**
 * Searches the channels of a catalog's guides, to map a channel to one by hand. The search starts
 * from the channel's name; each result shows its names, id, guide, icon and what airs now.
 */
export default function GuidePicker({
  scope,
  target,
  channelName,
  onPick,
  onCancel,
}: {
  scope: Scope
  target: CatalogTarget
  channelName: string
  onPick: (channel: GuideChannel) => void
  onCancel: () => void
}) {
  const { language, t } = useI18n()
  const text = t.lineup.picker
  const [search, setSearch] = useState(channelName)
  const [guide, setGuide] = useState('')
  const [offset, setOffset] = useState(0)
  const ids = { search: useId(), guide: useId(), title: useId() }
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
      aria-labelledby={ids.title}
      className="space-y-3 rounded-xl border border-fin-4/40 bg-bg/60 p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h3 id={ids.title} className="text-sm font-semibold text-white">
          {text.title(channelName)}
        </h3>
        <button type="button" className={buttonSecondary} onClick={onCancel}>
          {t.common.cancel}
        </button>
      </div>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="relative">
          <label htmlFor={ids.search} className="mb-1.5 block text-xs font-medium text-muted">
            {text.search}
          </label>
          <icons.search className="pointer-events-none absolute bottom-2.5 left-3 size-4 text-muted" />
          <input
            id={ids.search}
            type="search"
            value={search}
            autoFocus
            autoComplete="off"
            onChange={(event) => {
              setSearch(event.target.value)
              setOffset(0)
            }}
            className={`${smallField} pl-9`}
          />
        </div>
        <div>
          <label htmlFor={ids.guide} className="mb-1.5 block text-xs font-medium text-muted">
            {text.guide}
          </label>
          <select
            id={ids.guide}
            value={guide}
            onChange={(event) => {
              setGuide(event.target.value)
              setOffset(0)
            }}
            className={smallField}
          >
            <option value="">{text.allGuides}</option>
            {(guides.data?.guides ?? []).map((g) => (
              <option key={g.id} value={g.id}>
                {text.guideN(g.position)} · {g.url}
              </option>
            ))}
          </select>
        </div>
      </div>
      {channels.isPending ? (
        <Skeleton rows={4} label={t.common.loading} />
      ) : channels.isError ? (
        <Notice kind="error">{errorMessage(t, channels.error)}</Notice>
      ) : channels.data.items.length === 0 ? (
        <Empty hint={guides.data?.guides.length === 0 ? text.noGuides : undefined}>
          {text.empty}
        </Empty>
      ) : (
        <ul
          aria-busy={channels.isFetching}
          className="max-h-[50dvh] divide-y divide-line overflow-y-auto rounded-lg border border-line"
        >
          {channels.data.items.map((channel) => {
            const others = channel.names.filter((name) => name !== channel.name)
            return (
              <li
                key={`${channel.guideId}:${channel.id}`}
                className="flex items-start gap-3 p-3 text-sm"
              >
                <GuideIcon icon={channel.icon} name={channel.name} />
                <div className="min-w-0 flex-1">
                  <p className="font-medium break-words text-white">{channel.name}</p>
                  {others.length > 0 && (
                    <p className="text-xs break-words text-muted">
                      {text.alsoNamed(others.join(' · '))}
                    </p>
                  )}
                  <p className="font-mono text-xs break-all text-zinc-400">
                    {channel.id} · {text.guideN(channel.guidePosition)}
                  </p>
                  <p className="text-xs text-zinc-300">
                    {channel.now
                      ? text.now(
                          channel.now.title,
                          time.format(new Date(channel.now.start)),
                          time.format(new Date(channel.now.end)),
                        )
                      : text.nothingNow}
                  </p>
                </div>
                <button
                  type="button"
                  className={buttonSecondary}
                  aria-label={text.chooseLabel(channel.name)}
                  onClick={() => onPick(channel)}
                >
                  {text.choose}
                </button>
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
  const frame = 'size-10 shrink-0 overflow-hidden rounded-md bg-surface-2'
  if (!icon || failed) {
    return (
      <span
        aria-hidden="true"
        className={`${frame} flex items-center justify-center text-xs font-semibold text-muted`}
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
      className={`${frame} object-contain p-0.5`}
    />
  )
}
