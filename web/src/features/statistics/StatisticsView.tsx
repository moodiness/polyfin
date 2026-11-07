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
import {
  dateTime,
  errorMessage,
  formatBytes,
  formatClock,
  formatEpisode,
  formatSpan,
} from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  cx,
  EmptyState,
  ExternalButtonLink,
  FigureTile,
  InlineError,
  Notice,
  Row,
  RowList,
  Segmented,
  Select,
  Skeleton,
  SkeletonRows,
} from '@/ui'
import { BarList, HourGrid, MethodShare, seriesColor, TimeChart, type TimeColumn } from './charts'

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
        <div role="status" aria-label={t.common.loading} className="grid gap-2.5 sm:grid-cols-3">
          {[0, 1, 2].map((index) => (
            <Skeleton key={index} className="h-[5.75rem] rounded-row" />
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
        key: `${app.shared}|${app.name}`,
        label: app.shared ? text.sharedLink : app.name || text.unknownApp,
        detail: `${formatSpan(app.played, language)} · ${text.plays(app.plays)}`,
        value: app.played,
      })),
    },
    {
      title: text.devices,
      items: statistics.devices.map((device) => ({
        key: `${device.shared}|${device.app}|${device.name}`,
        label: device.shared
          ? text.appOn(text.sharedLink, text.webBrowser)
          : text.appOn(device.app, device.name) || text.unknownDevice,
        detail: `${formatSpan(device.played, language)} · ${text.plays(device.plays)}`,
        value: device.played,
      })),
    },
  ]
  return (
    <>
      <div
        className={cx(
          'grid grid-cols-2 gap-2.5',
          scope === 'server' ? 'xl:grid-cols-4' : 'xl:grid-cols-3',
        )}
      >
        <FigureTile label={text.figures.played} value={formatSpan(statistics.played, language)} />
        <FigureTile label={text.figures.plays} value={number.format(statistics.plays)} />
        {scope === 'server' && (
          <FigureTile label={text.figures.users} value={number.format(statistics.users.length)} />
        )}
        <FigureTile label={text.figures.sent} value={formatBytes(statistics.sent, language)} />
      </div>
      <Block title={statistics.unit === 'month' ? text.perMonth : text.perDay}>
        <TimeChart
          label={statistics.unit === 'month' ? text.perMonth : text.perDay}
          columns={timeColumns(statistics, colors, language)}
        />
        {scope === 'server' && statistics.users.length > 1 && (
          <ul className="mt-4 flex flex-wrap gap-x-6 gap-y-2 text-small text-ink-2">
            {statistics.users.map((u, index) => (
              <li key={u.id} className="flex items-center gap-2">
                <span
                  aria-hidden="true"
                  className="size-3 rounded-full"
                  style={{ backgroundColor: seriesColor(index) }}
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
              <p className="text-small text-ink-2">{text.none}</p>
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
        <p className="mb-4 max-w-[66ch] text-small text-ink-2">{text.hoursHelp}</p>
        <HourGrid label={text.hours} hours={statistics.hours} />
      </Block>
    </>
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
        <SkeletonRows rows={3} />
      ) : history.isError ? (
        <InlineError onRetry={() => void history.refetch()} retrying={history.isFetching}>
          {errorMessage(t, history.error)}
        </InlineError>
      ) : entries.length === 0 ? (
        <p className="text-small text-ink-2">{text.historyEmpty}</p>
      ) : (
        <>
          <RowList compact aria-label={text.history}>
            {entries.map((entry) => {
              const [name, part] = entryTitle(entry)
              return (
                <Row
                  key={entry.id}
                  title={name}
                  titleAside={
                    entry.shared ? (
                      <span title={text.sharedHelp}>
                        <Badge tone="accent">{text.shared}</Badge>
                      </span>
                    ) : undefined
                  }
                  meta={
                    <span className="figures">
                      {[
                        part,
                        dateTime(entry.startedAt, language),
                        scope === 'server' ? entry.user.name : null,
                        entry.shared
                          ? text.sharedLink
                          : text.appOn(entry.app, entry.device) || text.unknownDevice,
                        text.methodNames[entry.method],
                      ]
                        .filter((said) => said !== null)
                        .join(' · ')}
                    </span>
                  }
                  trailing={
                    <>
                      <span className="figures text-ink" title={formatClock(entry.position)}>
                        <span className="sr-only">
                          {text.columns.played}
                          {t.common.colon}{' '}
                        </span>
                        {formatSpan(entry.played, language)}
                      </span>
                      {entry.sent > 0 && (
                        <span className="figures text-micro text-ink-2">
                          <span className="sr-only">
                            {text.columns.sent}
                            {t.common.colon}{' '}
                          </span>
                          {formatBytes(entry.sent, language)}
                        </span>
                      )}
                    </>
                  }
                />
              )
            })}
          </RowList>
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

/**
 * A playback's title as it was called, and what part of what it was: an episode under its series
 * name, a programme with its channel.
 */
function entryTitle(entry: HistoryEntry): [string, string | null] {
  const { item } = entry
  if (item.kind === 'episode' && item.seriesName !== null) {
    return [item.seriesName, `${formatEpisode(item.season ?? 0, item.episode ?? 0)} · ${item.name}`]
  }
  if (item.kind !== 'channel' && item.channelName) return [item.name, item.channelName]
  return [item.name, null]
}
