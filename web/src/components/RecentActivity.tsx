import { useQuery } from '@tanstack/react-query'
import { fetchActivity, queryKeys } from '@/api'
import { Card, Loading, Notice } from '@/components/ui'
import { dateTime, errorMessage } from '@/format'
import { useI18n } from '@/i18n'

const limit = 20

const severityDots = { Information: 'bg-line', Warning: 'bg-amber-400', Error: 'bg-rose-400' }

/** The server's latest activity (sign-ins, playback, changes), for administrators. */
export default function RecentActivity() {
  const { language, t } = useI18n()
  const activity = useQuery({
    queryKey: queryKeys.activity(limit),
    queryFn: ({ signal }) => fetchActivity(limit, signal),
    refetchInterval: 30_000,
  })

  return (
    <Card title={t.activity.title}>
      {activity.isPending ? (
        <Loading />
      ) : activity.isError ? (
        <Notice kind="error">{errorMessage(t, activity.error)}</Notice>
      ) : activity.data.items.length === 0 ? (
        <p className="text-sm text-muted">{t.activity.empty}</p>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {activity.data.items.map((entry) => (
            <li key={entry.id} className="flex gap-3 p-4 text-sm">
              <span
                aria-hidden="true"
                className={`mt-1.5 size-2 shrink-0 rounded-full ${severityDots[entry.severity]}`}
              />
              <div className="min-w-0">
                <p className="break-words text-white">
                  {entry.severity !== 'Information' && (
                    <span className="sr-only">
                      {entry.severity === 'Error' ? t.activity.error : t.activity.warning}:{' '}
                    </span>
                  )}
                  {entry.name}
                </p>
                {entry.shortOverview !== null && (
                  <p className="break-words text-muted">{entry.shortOverview}</p>
                )}
                <p className="mt-0.5 text-xs text-muted">
                  <time dateTime={entry.date}>{dateTime(entry.date, language)}</time>
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
      {activity.data !== undefined && (
        <div className="mt-3 space-y-0.5 text-xs text-muted">
          {activity.data.total > activity.data.items.length && (
            <p>{t.activity.count(activity.data.items.length, activity.data.total)}</p>
          )}
          <p>{t.activity.autoRefresh}</p>
        </div>
      )}
    </Card>
  )
}
