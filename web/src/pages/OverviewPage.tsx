import { useQuery } from '@tanstack/react-query'
import { fetchLiveSessions, queryKeys } from '@/api'
import LiveSessions from '@/components/LiveSessions'
import { Stat } from '@/components/panels'
import { findProblems, useHealthData } from '@/components/problems'
import RecentActivity from '@/components/RecentActivity'
import { useSessionUser } from '@/components/session'
import StatusPanel from '@/components/StatusPanel'
import { PageHeader } from '@/components/ui'
import { formatSpan } from '@/format'
import { useI18n } from '@/i18n'

/** The home page: the server's status, and for administrators what plays and what happened. */
export default function OverviewPage() {
  const { t } = useI18n()
  const user = useSessionUser()
  if (!user.isAdministrator) {
    return (
      <>
        <PageHeader title={t.status.title} description={t.status.description} />
        <StatusPanel />
      </>
    )
  }
  return (
    <>
      <PageHeader
        title={t.dashboard.overview.title}
        description={t.dashboard.overview.description}
      />
      <div className="space-y-6">
        <Figures />
        <LiveSessions />
        <RecentActivity />
        <section aria-labelledby="server-status">
          <h2 id="server-status" className="mb-3 text-base font-semibold text-white">
            {t.dashboard.overview.serverTitle}
          </h2>
          <StatusPanel />
        </section>
      </div>
    </>
  )
}

function Figures() {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  // The same query as the live sessions panel: one request serves both.
  const sessions = useQuery({
    queryKey: queryKeys.liveSessions,
    queryFn: ({ signal }) => fetchLiveSessions(signal),
    refetchInterval: 3_000,
  })
  const { health, sources, tasks } = useHealthData()
  const problems = findProblems(t, language, health.data, sources.data, tasks.data)
  const transcoder = health.data?.transcoder
  const startedAt = health.data?.process.startedAt
  const errors = problems.some((problem) => problem.tone === 'error')

  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Stat
        label={text.playing}
        value={sessions.data?.length ?? '–'}
        to="#now-playing"
        tone={sessions.data?.length ? 'active' : undefined}
      />
      <Stat
        label={text.conversions}
        value={transcoder ? transcoder.conversions : '–'}
        detail={
          transcoder ? text.conversionsOf(transcoder.conversions, transcoder.limit) : undefined
        }
        to="/health#transcoder"
      />
      <Stat
        label={text.uptime}
        value={
          startedAt && health.data
            ? formatSpan(
                (Date.parse(health.data.checkedAt) - Date.parse(startedAt)) / 1000,
                language,
              )
            : '–'
        }
        detail={health.data ? `Polyfin ${health.data.process.version}` : undefined}
        to="/health#process"
      />
      <Stat
        label={text.problems}
        value={health.data ? problems.length : '–'}
        detail={health.data ? text.problemCount(problems.length) : undefined}
        tone={
          problems.length === 0 ? (health.data ? 'ok' : undefined) : errors ? 'error' : 'warning'
        }
        to="/health"
      />
    </div>
  )
}
