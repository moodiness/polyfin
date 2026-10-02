import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus, queryKeys } from '@/api'
import { useI18n } from '@/i18n'

export default function StatusPanel() {
  const { language, t } = useI18n()
  const { data, error, isFetching, dataUpdatedAt, refetch } = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })

  if (data === undefined && error === null) {
    return (
      <div
        role="status"
        className="rounded-2xl border border-line bg-surface p-5 motion-safe:animate-pulse"
      >
        <p className="text-muted">{t.status.loading}</p>
        <div aria-hidden="true" className="mt-5 space-y-3">
          <div className="h-4 w-1/3 rounded bg-line" />
          <div className="h-4 w-2/3 rounded bg-line" />
          <div className="h-4 w-1/4 rounded bg-line" />
        </div>
      </div>
    )
  }

  const retryButton = (
    <button
      type="button"
      onClick={() => void refetch()}
      disabled={isFetching}
      className="rounded-lg border border-line bg-ink px-4 py-2 text-sm font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-70"
    >
      {isFetching ? t.status.retrying : t.status.retry}
    </button>
  )

  if (data === undefined) {
    return (
      <div role="alert" className="rounded-2xl border border-rose-400/40 bg-rose-400/5 p-5">
        <h2 className="flex items-center gap-2 font-semibold text-rose-300">
          <span aria-hidden="true" className="size-2.5 shrink-0 rounded-full bg-rose-400" />
          {t.status.errorTitle}
        </h2>
        <p className="mt-2 text-zinc-200">{t.status.errorBody}</p>
        <div className="mt-4">{retryButton}</div>
      </div>
    )
  }

  const databaseReady = data.database === 'ready'
  const timeFormat = new Intl.DateTimeFormat(language, { timeStyle: 'medium' })

  return (
    <div className="space-y-4">
      {error !== null && (
        <div
          role="alert"
          className="flex flex-col gap-3 rounded-xl border border-amber-400/40 bg-amber-400/5 p-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <p className="text-sm text-amber-200">{t.status.staleWarning}</p>
          <div className="shrink-0">{retryButton}</div>
        </div>
      )}

      <section className="overflow-hidden rounded-2xl border border-line bg-surface">
        <dl className="divide-y divide-line">
          <StatusRow label={t.status.version}>
            <span className="font-medium text-white">{data.version}</span>
          </StatusRow>
          <StatusRow label={t.status.serverId}>
            <code className="font-mono text-sm break-all text-white select-all">
              {data.serverId}
            </code>
          </StatusRow>
          <StatusRow label={t.status.database}>
            <span
              className={`inline-flex items-center gap-2 font-medium ${
                databaseReady ? 'text-emerald-300' : 'text-amber-300'
              }`}
            >
              <span
                aria-hidden="true"
                className={`size-2.5 shrink-0 rounded-full ${
                  databaseReady ? 'bg-emerald-400' : 'bg-amber-400'
                }`}
              />
              {databaseReady ? t.status.databaseReady : t.status.databaseUnavailable}
            </span>
          </StatusRow>
        </dl>
        <div className="border-t border-line bg-ink/40 px-5 py-3 text-xs text-muted">
          <p>{t.status.updatedAt(timeFormat.format(dataUpdatedAt))}</p>
          <p className="mt-0.5">{t.status.autoRefresh}</p>
        </div>
      </section>
    </div>
  )
}

function StatusRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="min-w-0 sm:text-right">{children}</dd>
    </div>
  )
}
