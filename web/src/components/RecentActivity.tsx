import { useId, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { fetchActivity, queryKeys, type ActivityQuery } from '@/api'
import { icons } from '@/components/icons'
import { Empty, Panel, Skeleton } from '@/components/panels'
import { buttonSecondary, Notice } from '@/components/ui'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n } from '@/i18n'

const pageSize = 25

const severityDots = { Information: 'bg-zinc-600', Warning: 'bg-amber-400', Error: 'bg-rose-400' }

/** The activity log's entry types each filter keeps; empty keeps all. */
const filters = {
  all: {},
  problems: { severities: ['Warning', 'Error'] },
  signIns: { types: ['AuthenticationSucceeded', 'AuthenticationFailed'] },
  playback: { types: ['VideoPlayback', 'VideoPlaybackStopped'] },
  users: { types: ['UserCreated', 'UserDeleted', 'UserPolicyUpdated', 'UserPasswordChanged'] },
  server: {
    types: ['ServerConfigurationUpdated', 'AddonInstalled', 'AddonUninstalled', 'SecretRevealed'],
  },
  recordings: { types: ['RecordingScheduled', 'RecordingDeleted'] },
} satisfies Record<string, Omit<ActivityQuery, 'limit'>>

type Filter = keyof typeof filters

/** The server's latest activity (sign-ins, playback, changes), filterable, for administrators. */
export default function RecentActivity() {
  const { language, t } = useI18n()
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const searchId = useId()
  const query: ActivityQuery = { limit: pageSize, ...filters[filter] }
  const activity = useInfiniteQuery({
    queryKey: queryKeys.activity(query),
    queryFn: ({ pageParam, signal }) => fetchActivity({ ...query, start: pageParam }, signal),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((count, page) => count + page.items.length, 0)
      return loaded < last.total ? loaded : undefined
    },
    refetchInterval: 15_000,
  })
  const entries = activity.data?.pages.flatMap((page) => page.items) ?? []
  const total = activity.data?.pages.at(-1)?.total ?? 0
  const needle = search.trim().toLocaleLowerCase(language)
  const shown = needle
    ? entries.filter((entry) =>
        `${entry.name} ${entry.shortOverview ?? ''} ${entry.overview ?? ''}`
          .toLocaleLowerCase(language)
          .includes(needle),
      )
    : entries

  return (
    <Panel
      id="activity"
      title={t.dashboard.activity.title}
      description={t.dashboard.activity.autoRefresh}
    >
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div
          role="group"
          aria-label={t.dashboard.activity.filter}
          className="flex flex-wrap gap-1.5"
        >
          {(Object.keys(filters) as Filter[]).map((key) => (
            <button
              key={key}
              type="button"
              aria-pressed={filter === key}
              onClick={() => setFilter(key)}
              className={`min-h-9 rounded-lg border px-3 text-sm font-medium transition-colors ${
                filter === key
                  ? 'border-fin-3 bg-fin-2/25 text-white'
                  : 'border-line text-muted hover:border-fin-4 hover:text-white'
              }`}
            >
              {t.dashboard.activity.filters[key]}
            </button>
          ))}
        </div>
        <div className="relative lg:w-72">
          <label htmlFor={searchId} className="sr-only">
            {t.dashboard.activity.search}
          </label>
          <icons.search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" />
          <input
            id={searchId}
            type="search"
            value={search}
            placeholder={t.dashboard.activity.search}
            onChange={(event) => setSearch(event.target.value)}
            className="block min-h-9 w-full rounded-lg border border-line bg-bg py-1.5 pr-3 pl-9 text-sm text-white placeholder:text-zinc-500"
          />
        </div>
      </div>
      {activity.isPending ? (
        <Skeleton rows={4} label={t.common.loading} />
      ) : activity.isError ? (
        <Notice kind="error">{errorMessage(t, activity.error)}</Notice>
      ) : shown.length === 0 ? (
        <Empty>
          {entries.length === 0 && filter === 'all'
            ? t.activity.empty
            : t.dashboard.activity.noMatch}
        </Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {shown.map((entry) => (
            <li key={entry.id} className="flex gap-3 px-4 py-3 text-sm">
              <span
                aria-hidden="true"
                className={`mt-1.5 size-2 shrink-0 rounded-full ${severityDots[entry.severity]}`}
              />
              <div className="min-w-0 flex-1">
                <p className="break-words text-white">
                  {entry.severity !== 'Information' && (
                    <span className="sr-only">
                      {entry.severity === 'Error' ? t.activity.error : t.activity.warning}
                      {t.common.colon}{' '}
                    </span>
                  )}
                  {entry.name}
                </p>
                {entry.shortOverview !== null && (
                  <p className="break-words text-muted">{entry.shortOverview}</p>
                )}
              </div>
              <time
                dateTime={entry.date}
                title={dateTime(entry.date, language)}
                className="shrink-0 text-xs text-muted tabular-nums"
              >
                {relativeTime(entry.date, language, t.time.justNow)}
              </time>
            </li>
          ))}
        </ul>
      )}
      {activity.data !== undefined && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-muted">
          <p>{t.dashboard.activity.count(entries.length, total)}</p>
          {activity.hasNextPage && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={activity.isFetchingNextPage}
              onClick={() => void activity.fetchNextPage()}
            >
              {activity.isFetchingNextPage
                ? t.dashboard.activity.loadingMore
                : t.dashboard.activity.more}
            </button>
          )}
        </div>
      )}
    </Panel>
  )
}
