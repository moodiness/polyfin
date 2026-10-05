import type { ReactNode } from 'react'
import { ArrowClockwiseIcon, PlugsIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus, queryKeys } from '@/api'
import { useI18n } from '@/i18n'
import { Button, Notice, Panel, PanelFooter, Skeleton, StatusPill } from '@/ui'

/**
 * The server's version, ID and database, refreshed every 10 seconds: a member's home, and what the
 * session gate shows when the server does not answer.
 */
export function ServerStatus() {
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
        aria-label={t.status.loading}
        className="rounded-panel border border-line-2 bg-s1"
      >
        {[0, 1, 2].map((row) => (
          <div
            key={row}
            className="flex items-center justify-between gap-6 px-6 py-[18px] not-first:border-t not-first:border-line max-sm:px-4"
          >
            <Skeleton className="h-3.5 w-28" />
            <Skeleton className="h-3.5 w-44" />
          </div>
        ))}
      </div>
    )
  }

  const retry = (
    <Button size="sm" icon={ArrowClockwiseIcon} loading={isFetching} onClick={() => void refetch()}>
      {isFetching ? t.status.retrying : t.status.retry}
    </Button>
  )

  if (data === undefined) {
    return (
      <div
        role="alert"
        className="flex flex-col items-start gap-4 rounded-panel border border-danger/25 bg-danger/8 p-6 max-sm:p-4"
      >
        <span className="inline-grid size-10 place-items-center rounded-row bg-danger/10 text-danger">
          <PlugsIcon size={18} aria-hidden="true" />
        </span>
        <div>
          <h2 className="text-h3 text-ink">{t.status.errorTitle}</h2>
          <p className="mt-1 max-w-[60ch] text-control text-ink-2">{t.status.errorBody}</p>
        </div>
        {retry}
      </div>
    )
  }

  const databaseReady = data.database === 'ready'
  const timeFormat = new Intl.DateTimeFormat(language, { timeStyle: 'medium' })

  return (
    <div className="flex flex-col gap-4">
      {error !== null && (
        <Notice tone="warn" live action={retry}>
          {t.status.staleWarning}
        </Notice>
      )}
      <Panel
        as="div"
        flush
        footer={
          <PanelFooter note={t.status.updatedAt(timeFormat.format(dataUpdatedAt))}>
            <span className="text-small text-ink-3">{t.status.autoRefresh}</span>
          </PanelFooter>
        }
      >
        <dl>
          <StatusRow label={t.status.version}>
            <span className="figures text-ink">{data.version}</span>
          </StatusRow>
          <StatusRow label={t.status.serverId}>
            <code className="font-mono text-small break-all text-ink select-all">
              {data.serverId}
            </code>
          </StatusRow>
          <StatusRow label={t.status.database}>
            <StatusPill tone={databaseReady ? 'ok' : 'warn'}>
              {databaseReady ? t.status.databaseReady : t.status.databaseUnavailable}
            </StatusPill>
          </StatusRow>
        </dl>
      </Panel>
    </div>
  )
}

function StatusRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 px-6 py-4 not-first:border-t not-first:border-line sm:flex-row sm:items-center sm:justify-between sm:gap-6 max-sm:px-4">
      <dt className="text-control text-ink-2">{label}</dt>
      <dd className="min-w-0 sm:text-right">{children}</dd>
    </div>
  )
}
