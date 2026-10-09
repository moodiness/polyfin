import type { ReactNode } from 'react'
import {
  ArrowRightIcon,
  CheckCircleIcon,
  CpuIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { fetchStatus, queryKeys, type HealthProblem } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { problemText, useHealth, useHealthProblems } from '@/features/system/problems'
import { errorMessage, formatBytes, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  ButtonLink,
  cx,
  EmptyState,
  InlineError,
  ProgressBar,
  Skeleton,
  StatusPill,
  TextLink,
} from '@/ui'
import { NowPlaying, useLiveSessions } from './NowPlaying'
import { RecentActivity } from './RecentActivity'
import { ServerStatus } from './ServerStatus'

/** `/`: the administrator overview, or a member's view of the server's status. */
export default function HomeRoute() {
  const { t } = useI18n()
  const user = useSessionUser()
  if (!user.isAdministrator) {
    return (
      <PageLayout title={t.status.title} lede={t.status.description}>
        <div className="max-w-[784px]">
          <ServerStatus />
        </div>
      </PageLayout>
    )
  }
  return <Overview name={user.name} />
}

function Overview({ name }: { name: string }) {
  const { t } = useI18n()
  const text = t.dashboard.overview
  const sessions = useLiveSessions()
  const health = useHealth()
  const problems = useHealthProblems()
  const transcoder = health.data?.transcoder

  return (
    <PageLayout
      title={text.greeting(new Date().getHours(), name)}
      lede={
        sessions.data !== undefined && problems.data !== undefined
          ? text.summary(sessions.data.length, problems.data.length)
          : text.description
      }
    >
      <NowPlaying
        aside={
          transcoder ? (
            <Link
              to="/system/health#transcoder"
              className="flex items-center gap-2 rounded-md transition-colors duration-160 hover:text-ink"
            >
              <CpuIcon size={16} aria-hidden="true" />
              {text.conversions}
              <span className="figures text-ink-2">
                {text.conversionsOf(transcoder.conversions, transcoder.limit)}
              </span>
            </Link>
          ) : undefined
        }
      />
      <Problems
        problems={problems.data ?? []}
        loading={problems.isPending}
        error={problems.error}
        retrying={problems.isFetching}
        onRetry={() => void problems.refetch()}
      />
      <ServerFigures />
      <RecentActivity />
    </PageLayout>
  )
}

function Problems({
  problems,
  loading,
  error,
  retrying,
  onRetry,
}: {
  problems: HealthProblem[]
  loading: boolean
  error: Error | null
  retrying: boolean
  onRetry: () => void
}) {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  return (
    <Block title={text.problems} count={loading ? undefined : problems.length}>
      {loading ? (
        <div
          role="status"
          aria-label={t.common.loading}
          className="flex items-center gap-5 rounded-panel border border-line-2 bg-s1 px-5 py-[18px]"
        >
          <Skeleton className="size-10 rounded-row" />
          <Skeleton className="h-3.5 w-1/2" />
        </div>
      ) : error !== null && problems.length === 0 ? (
        <InlineError onRetry={onRetry} retrying={retrying}>
          {errorMessage(t, error)}
        </InlineError>
      ) : problems.length === 0 ? (
        <EmptyState icon={CheckCircleIcon} title={text.problemsEmpty}>
          {text.problemsEmptyHelp}
        </EmptyState>
      ) : (
        <ul className="flex flex-col gap-3">
          {problems.map((problem) => (
            <li
              key={problem.key}
              className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-5 rounded-panel border border-line-2 bg-s1 px-5 py-[18px] max-sm:grid-cols-[auto_minmax(0,1fr)] max-sm:gap-4 max-sm:p-4"
            >
              <span
                aria-hidden="true"
                className={cx(
                  'grid size-10 place-items-center rounded-row',
                  problem.tone === 'error' ? 'bg-danger/10 text-danger' : 'bg-warn/10 text-warn',
                )}
              >
                {problem.tone === 'error' ? (
                  <WarningCircleIcon size={18} />
                ) : (
                  <WarningIcon size={18} />
                )}
              </span>
              <div className="min-w-0">
                <StatusPill tone={problem.tone === 'error' ? 'danger' : 'warn'}>
                  {problem.tone === 'error' ? text.problemError : text.problemWarning}
                </StatusPill>
                <p className="mt-1 text-[15px] font-medium tracking-[-0.01em] text-ink">
                  {problemText(t, language, problem)}
                </p>
              </div>
              <ButtonLink
                to={problem.to}
                size="sm"
                iconEnd={ArrowRightIcon}
                className="max-sm:col-span-full max-sm:justify-self-start"
              >
                {text.open}
              </ButtonLink>
            </li>
          ))}
        </ul>
      )}
    </Block>
  )
}

/** The server in a row of figures between hairlines, as Health tells it in full. */
function ServerFigures() {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  const health = useHealth()
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })
  const data = health.data
  const timeFormat = new Intl.DateTimeFormat(language, { timeStyle: 'medium' })

  return (
    <Block
      title={text.serverTitle}
      aside={<TextLink to="/system/health">{text.openHealth}</TextLink>}
    >
      {health.isError && data === undefined ? (
        <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
          {errorMessage(t, health.error)}
        </InlineError>
      ) : (
        <>
          <dl className="grid grid-cols-5 border-t border-line pt-[22px] pb-1 max-lg:grid-cols-3 max-lg:gap-y-6 max-sm:grid-cols-2">
            <Figure label={text.graphics} loading={data === undefined}>
              {data &&
                (data.transcoder?.hardware ? (
                  <>
                    <dd className="mt-2 text-[24px] leading-tight font-medium tracking-[-0.02em] text-ink">
                      {data.transcoder.hardware.method === 'cuda' ? 'NVIDIA' : 'VAAPI'}
                    </dd>
                    <dd className="mt-1.5 text-[13px] text-ink-2">
                      {[
                        data.transcoder.hardware.encoders.join(', '),
                        data.transcoder.hardware.toneMapping ? text.toneMapping : '',
                      ]
                        .filter(Boolean)
                        .join(', ')}
                    </dd>
                  </>
                ) : (
                  <>
                    <dd className="mt-2 text-[24px] leading-tight font-medium tracking-[-0.02em] text-ink">
                      {text.graphicsNone}
                    </dd>
                    <dd className="mt-1.5 text-[13px] text-ink-2">{text.graphicsCpu}</dd>
                  </>
                ))}
            </Figure>
            <Figure label={text.cache} loading={data === undefined}>
              {data &&
                (data.cache === null ? (
                  <FigureValue>{text.cacheOff}</FigureValue>
                ) : (
                  <>
                    <FigureValue mono>{formatBytes(data.cache.used, language)}</FigureValue>
                    <dd className="mt-1.5 flex items-center gap-2.5 text-[13px] text-ink-2">
                      {data.cache.limit > 0
                        ? text.cacheOf(formatBytes(data.cache.limit, language))
                        : text.cacheNoLimit}
                      {data.cache.limit > 0 && (
                        <ProgressBar
                          size="sm"
                          variant={data.cache.used / data.cache.limit > 0.9 ? 'warn' : 'neutral'}
                          value={data.cache.used / data.cache.limit}
                          label={text.cacheUnit}
                          className="w-14"
                        />
                      )}
                    </dd>
                  </>
                ))}
            </Figure>
            <Figure label={text.database} loading={data === undefined}>
              {data && (
                <>
                  <dd className="mt-2">
                    <StatusPill
                      tone={data.database.reachable ? 'ok' : 'danger'}
                      className="text-[15px]"
                    >
                      {data.database.reachable
                        ? t.status.databaseReady
                        : t.status.databaseUnavailable}
                    </StatusPill>
                  </dd>
                  {data.database.size !== null && (
                    <dd className="figures mt-1.5 text-[13px] text-ink-2">
                      {text.databaseSize(formatBytes(data.database.size, language))}
                    </dd>
                  )}
                </>
              )}
            </Figure>
            <Figure label={text.uptime} loading={data === undefined}>
              {data && (
                <FigureValue mono>
                  {formatSpan(
                    (Date.parse(data.checkedAt) - Date.parse(data.process.startedAt)) / 1000,
                    language,
                  )}
                </FigureValue>
              )}
            </Figure>
            <Figure label={text.version} loading={data === undefined}>
              {data && <FigureValue mono>{data.process.version}</FigureValue>}
            </Figure>
          </dl>
          {status.data !== undefined && (
            <p className="mt-6 flex flex-wrap items-center gap-x-5 gap-y-1 text-small text-ink-3">
              <span>
                {t.status.serverId}
                {t.common.colon}{' '}
                <code className="font-mono break-all text-ink-2 select-all">
                  {status.data.serverId}
                </code>
              </span>
              <span>{t.status.updatedAt(timeFormat.format(status.dataUpdatedAt))}</span>
            </p>
          )}
        </>
      )}
    </Block>
  )
}

function Figure({
  label,
  loading,
  children,
}: {
  label: string
  loading: boolean
  children: ReactNode
}) {
  return (
    <div className="min-w-0 border-line px-6 first:border-l-0 first:pl-0 not-first:border-l max-lg:[&:nth-child(3n+1)]:border-l-0 max-lg:[&:nth-child(3n+1)]:pl-0 max-sm:px-4 max-sm:[&:nth-child(odd)]:border-l-0 max-sm:[&:nth-child(odd)]:pl-0 max-sm:[&:nth-child(even)]:border-l max-sm:[&:nth-child(even)]:pl-4">
      <dt className="text-[12.5px] text-ink-3">{label}</dt>
      {loading ? (
        <dd className="mt-2.5 flex flex-col gap-2.5">
          <Skeleton className="h-6 w-20" />
          <Skeleton className="h-3 w-24" />
        </dd>
      ) : (
        children
      )}
    </div>
  )
}

function FigureValue({ mono = false, children }: { mono?: boolean; children: ReactNode }) {
  return (
    <dd
      className={cx(
        'mt-2 text-[24px] leading-tight font-medium tracking-[-0.02em] text-ink max-sm:text-[20px]',
        mono && 'font-mono',
      )}
    >
      {children}
    </dd>
  )
}
