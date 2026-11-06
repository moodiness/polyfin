import { ArrowClockwiseIcon, PlugsIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus, queryKeys } from '@/api'
import { useI18n } from '@/i18n'
import {
  AlertTile,
  Button,
  Notice,
  Panel,
  PanelFooter,
  Row,
  RowList,
  SkeletonRows,
  StatusPill,
} from '@/ui'

/**
 * The server's version, ID and database, refreshed every 10 seconds: what the session gate shows
 * when the server does not answer.
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
      <div role="status" aria-label={t.status.loading}>
        <SkeletonRows rows={3} />
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
      <div role="alert">
        <AlertTile
          tone="danger"
          icon={PlugsIcon}
          title={t.status.errorTitle}
          titleAs="h2"
          actions={retry}
        >
          {t.status.errorBody}
        </AlertTile>
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
        footer={
          <PanelFooter note={t.status.updatedAt(timeFormat.format(dataUpdatedAt))}>
            <span className="text-small text-ink-3">{t.status.autoRefresh}</span>
          </PanelFooter>
        }
      >
        <RowList aria-label={t.status.title}>
          <Row
            title={t.status.version}
            trailing={<span className="figures text-ink">{data.version}</span>}
          />
          <Row
            title={t.status.serverId}
            trailing={
              <code className="font-mono text-small break-all text-ink select-all">
                {data.serverId}
              </code>
            }
          />
          <Row
            title={t.status.database}
            trailing={
              <StatusPill tone={databaseReady ? 'ok' : 'warn'}>
                {databaseReady ? t.status.databaseReady : t.status.databaseUnavailable}
              </StatusPill>
            }
          />
        </RowList>
      </Panel>
    </div>
  )
}
