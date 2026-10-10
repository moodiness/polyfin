import { useState } from 'react'
import { ChartBarIcon, DownloadSimpleIcon } from '@phosphor-icons/react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import {
  fetchHistory,
  fetchStatistics,
  fetchUsers,
  historyExportUrl,
  queryKeys,
  statisticsPeriods,
  type HistoryEntry,
  type PlaybacksQuery,
  type RankedTitle,
  type Statistics,
  type StatisticsPeriod,
  type StatisticsScope,
} from '@/api'
import { settingsPath } from '@/app/navigation'
import { dateTime, errorMessage, formatClock, formatEpisode, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  ButtonLink,
  cx,
  EmptyState,
  ExternalButtonLink,
  InlineError,
  Notice,
  Segmented,
  Select,
  Skeleton,
  Table,
} from '@/ui'
import { BarList, HourGrid, MethodShare, TimeChart, userColor, type TimeColumn } from './charts'

const historyPage = 20

/**
 * The statistics of a period, its playbacks, and its CSV: everyone's, or one user's, for an
 * administrator (`server`), the signed-in user's (`own`).
 */
export default function StatisticsView({ scope }: { scope: StatisticsScope }) {
  const { t } = useI18n()
  const text = t.statistics
  const [period, setPeriod] = useState<StatisticsPeriod>('30d')
  const [user, setUser] = useState('')
  const query: PlaybacksQuery = { period, ...(user ? { user } : {}) }
  const statistics = useQuery({
    queryKey: queryKeys.statistics(scope, query),
    queryFn: ({ signal }) => fetchStatistics(scope, query, signal),
    placeholderData: (previous) => previous,
  })
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
    enabled: scope === 'server',
  })

  return (
    <>
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
        <Segmented
          label={text.period}
          value={period}
          onChange={setPeriod}
          options={statisticsPeriods.map((value) => ({ value, label: text.periods[value] }))}
        />
        {scope === 'server' && (
          <Select
            aria-label={text.user}
            value={user}
            onValue={setUser}
            options={[
              { value: '', label: text.allUsers },
              ...(users.data ?? []).map((u) => ({ value: u.id, label: u.name })),
            ]}
            className="sm:w-56"
          />
        )}
        <ExternalButtonLink
          href={historyExportUrl(scope, query)}
          download="playback-history.csv"
          icon={DownloadSimpleIcon}
          className="sm:ml-auto"
        >
          {text.exportCsv}
        </ExternalButtonLink>
      </div>
      {statistics.data && !statistics.data.historyEnabled && (
        <Notice
          tone="warn"
          action={
            scope === 'server' ? (
              <ButtonLink size="sm" to={settingsPath('playback', 'playback-history')}>
                {text.openSettings}
              </ButtonLink>
            ) : undefined
          }
        >
          {text.historyOff[scope]}
        </Notice>
      )}
      {statistics.isPending ? (
        <div role="status" aria-label={t.common.loading} className="grid gap-4 sm:grid-cols-3">
          {[0, 1, 2].map((index) => (
            <Skeleton key={index} className="h-20" />
          ))}
        </div>
      ) : statistics.isError ? (
        <InlineError onRetry={() => void statistics.refetch()} retrying={statistics.isFetching}>
          {errorMessage(t, statistics.error)}
        </InlineError>
      ) : statistics.data.plays === 0 ? (
        <EmptyState icon={ChartBarIcon} title={text.empty}>
          {text.emptyHint}
        </EmptyState>
      ) : (
        <Figures scope={scope} statistics={statistics.data} />
      )}
      <History scope={scope} query={query} />
    </>
  )
}

/** Every figure of a period with playbacks. */
function Figures({ scope, statistics }: { scope: StatisticsScope; statistics: Statistics }) {
  const { language, t } = useI18n()
  const text = t.statistics
  const number = new Intl.NumberFormat(language)
  const colors = new Map(statistics.users.map((u, index) => [u.id, index]))
  const ranked = (titles: RankedTitle[]) =>
    titles.map((title) => ({
      key: title.id,
      label: title.name,
      detail: `${formatSpan(title.played, language)} · ${text.plays(title.plays)}`,
      value: title.played,
    }))
  const lists = [
    { title: text.movies, items: ranked(statistics.movies) },
    { title: text.series, items: ranked(statistics.series) },
    { title: text.channels, items: ranked(statistics.channels) },
    {
      title: text.apps,
      items: statistics.apps.map((app) => ({
        key: app.name,
        label: app.name || text.unknownApp,
        detail: `${formatSpan(app.played, language)} · ${text.plays(app.plays)}`,
        value: app.played,
      })),
    },
    {
      title: text.devices,
      items: statistics.devices.map((device) => ({
        key: `${device.app}|${device.name}`,
        label: text.appOn(device.app, device.name) || text.unknownDevice,
        detail: `${formatSpan(device.played, language)} · ${text.plays(device.plays)}`,
        value: device.played,
      })),
    },
  ]
  return (
    <>
      <dl className={cx('grid gap-4', scope === 'server' ? 'sm:grid-cols-3' : 'sm:grid-cols-2')}>
        <Figure label={text.figures.played} value={formatSpan(statistics.played, language)} />
        <Figure label={text.figures.plays} value={number.format(statistics.plays)} />
        {scope === 'server' && (
          <Figure label={text.figures.users} value={number.format(statistics.users.length)} />
        )}
      </dl>
      <Block title={statistics.unit === 'month' ? text.perMonth : text.perDay}>
        <TimeChart
          label={statistics.unit === 'month' ? text.perMonth : text.perDay}
          columns={timeColumns(statistics, colors, language)}
        />
        {scope === 'server' && statistics.users.length > 1 && (
          <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-small text-ink-2">
            {statistics.users.map((u, index) => (
              <li key={u.id} className="flex items-center gap-1.5">
                <span
                  aria-hidden="true"
                  className={cx('size-2.5 rounded-full', userColor(index))}
                />
                {u.name}
              </li>
            ))}
          </ul>
        )}
      </Block>
      {scope === 'server' && (
        <Block title={text.perUser}>
          <BarList
            label={text.perUser}
            items={statistics.users.map((u) => ({
              key: u.id,
              label: u.name,
              detail: `${formatSpan(u.played, language)} · ${text.plays(u.plays)}`,
              value: u.played,
            }))}
          />
        </Block>
      )}
      <div className="grid gap-x-12 gap-y-block md:grid-cols-2">
        {lists.map((list) => (
          <Block key={list.title} title={list.title}>
            {list.items.length === 0 ? (
              <p className="text-small text-ink-3">{text.none}</p>
            ) : (
              <BarList label={list.title} items={list.items} />
            )}
          </Block>
        ))}
      </div>
      <Block title={text.methods}>
        <MethodShare methods={statistics.methods} />
      </Block>
      <Block title={text.hours}>
        <p className="mb-3 text-small text-ink-3">{text.hoursHelp}</p>
        <HourGrid label={text.hours} hours={statistics.hours} />
      </Block>
    </>
  )
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-panel border border-line-2 bg-s1 px-4 py-3.5">
      <dt className="text-small text-ink-3">{label}</dt>
      <dd className="figures mt-1 text-h2 text-ink">{value}</dd>
    </div>
  )
}

/** A local date as YYYY-MM-DD, as the server names days in the browser's time zone. */
function dayKey(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

/**
 * Every day of the period, or every month since the first playback, with what each user played
 * in it: days without playbacks show as empty columns.
 */
function timeColumns(
  statistics: Statistics,
  colors: Map<string, number>,
  language: string,
): TimeColumn[] {
  const monthly = statistics.unit === 'month'
  const format = new Intl.DateTimeFormat(
    language,
    monthly ? { month: 'short', year: 'numeric' } : { day: 'numeric', month: 'short' },
  )
  const byKey = new Map(statistics.buckets.map((bucket) => [bucket.start, bucket]))
  const first = statistics.since
    ? new Date(statistics.since)
    : new Date(`${statistics.buckets[0]?.start ?? dayKey(new Date())}T00:00:00`)
  const day = new Date(first.getFullYear(), first.getMonth(), monthly ? 1 : first.getDate())
  const columns: TimeColumn[] = []
  for (const end = new Date(); day <= end;) {
    const key = dayKey(day)
    const bucket = byKey.get(key)
    columns.push({
      key,
      label: format.format(day),
      played: bucket?.played ?? 0,
      parts: (bucket?.users ?? []).map((u) => ({ index: colors.get(u.id) ?? 0, played: u.played })),
    })
    if (monthly) day.setMonth(day.getMonth() + 1)
    else day.setDate(day.getDate() + 1)
  }
  return columns
}

/** The playbacks of the period, the latest first, a page at a time. */
function History({ scope, query }: { scope: StatisticsScope; query: PlaybacksQuery }) {
  const { language, t } = useI18n()
  const text = t.statistics
  const history = useInfiniteQuery({
    queryKey: queryKeys.history(scope, { ...query, start: 0 }),
    queryFn: ({ pageParam, signal }) =>
      fetchHistory(scope, { ...query, limit: historyPage, start: pageParam }, signal),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((count, page) => count + page.items.length, 0)
      return loaded < last.total ? loaded : undefined
    },
  })
  const entries = history.data?.pages.flatMap((page) => page.items) ?? []
  return (
    <Block title={text.history} count={history.data?.pages.at(-1)?.total}>
      {history.isPending ? (
        <Skeleton className="h-40" />
      ) : history.isError ? (
        <InlineError onRetry={() => void history.refetch()} retrying={history.isFetching}>
          {errorMessage(t, history.error)}
        </InlineError>
      ) : entries.length === 0 ? (
        <p className="text-small text-ink-3">{text.historyEmpty}</p>
      ) : (
        <>
          <Table label={text.history} minWidth={scope === 'server' ? 760 : 640}>
            <thead>
              <tr>
                <th scope="col">{text.columns.started}</th>
                {scope === 'server' && <th scope="col">{text.columns.user}</th>}
                <th scope="col">{text.columns.title}</th>
                <th scope="col">{text.columns.device}</th>
                <th scope="col" className="text-right">
                  {text.columns.played}
                </th>
                <th scope="col">{text.columns.method}</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id}>
                  <td className="figures whitespace-nowrap">
                    {dateTime(entry.startedAt, language)}
                  </td>
                  {scope === 'server' && <td className="text-ink">{entry.user.name}</td>}
                  <td className="text-ink">
                    <EntryTitle entry={entry} />
                  </td>
                  <td>{text.appOn(entry.app, entry.device) || text.unknownDevice}</td>
                  <td
                    className="figures text-right whitespace-nowrap"
                    title={formatClock(entry.position)}
                  >
                    {formatSpan(entry.played, language)}
                  </td>
                  <td className="whitespace-nowrap">{text.methodNames[entry.method]}</td>
                </tr>
              ))}
            </tbody>
          </Table>
          {history.hasNextPage && (
            <Button
              variant="ghost"
              className="mt-3"
              disabled={history.isFetchingNextPage}
              onClick={() => void history.fetchNextPage()}
            >
              {history.isFetchingNextPage ? text.loadingMore : text.showMore}
            </Button>
          )}
        </>
      )}
    </Block>
  )
}

/** A playback's title as it was called: an episode with its series, a programme with its channel. */
function EntryTitle({ entry }: { entry: HistoryEntry }) {
  const { item } = entry
  if (item.kind === 'episode' && item.seriesName !== null) {
    return (
      <>
        {item.seriesName}
        <span className="block text-small text-ink-3">
          {formatEpisode(item.season ?? 0, item.episode ?? 0)} · {item.name}
        </span>
      </>
    )
  }
  if (item.kind !== 'channel' && item.channelName) {
    return (
      <>
        {item.name}
        <span className="block text-small text-ink-3">{item.channelName}</span>
      </>
    )
  }
  return <>{item.name}</>
}
