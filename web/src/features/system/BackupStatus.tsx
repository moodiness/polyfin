import type { Backup, BackupDestination } from '@/api'
import { StatusPill, RelativeTime, type StatusTone } from '@/ui'
import { formatBytes } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import { Facts } from './parts'

/** The state of a destination: its key unreadable, its last upload, or none yet. */
export function destinationStatus(
  t: Messages,
  destination: BackupDestination,
): { tone: StatusTone; label: string } {
  const status = t.backups.destinations.status
  if (destination.unreadable) return { tone: 'danger', label: status.unreadable }
  if (destination.problem === 'failed') return { tone: 'danger', label: status.failed }
  if (destination.problem === 'stale') return { tone: 'warn', label: status.stale }
  if (destination.ranAt === null) return { tone: 'muted', label: status.waiting }
  return { tone: 'ok', label: status.working }
}

/**
 * How the database backups go: the last run, the last backup made (its file and size), when the
 * next one is, each destination's last upload, and the last restore. A failed run shows its error
 * under the facts. Takes the `backup` of the health, which is null when backups are off: show
 * `t.system.health.backupOff` then instead.
 */
export default function BackupStatus({ backup }: { backup: Backup }) {
  const { language, t } = useI18n()
  const text = t.system.health.backup
  const made = backup.madeAt
  const last = backup.lastRestore
  return (
    <div className="space-y-4">
      <Facts
        items={[
          {
            label: text.result,
            value:
              backup.ranAt === null ? (
                <span className="text-ink-3">{text.none}</span>
              ) : (
                <span className="flex flex-wrap items-baseline gap-x-2">
                  {backup.error === '' ? (
                    <StatusPill tone="ok">{text.succeeded}</StatusPill>
                  ) : (
                    <StatusPill tone="danger">{text.failed}</StatusPill>
                  )}
                  <span className="text-small text-ink-3">
                    <RelativeTime iso={backup.ranAt} />
                  </span>
                </span>
              ),
          },
          { label: text.made, value: made === null ? '–' : <RelativeTime iso={made} /> },
          {
            label: text.file,
            value:
              made === null ? '–' : <code className="font-mono text-small">{backup.file}</code>,
          },
          { label: text.size, value: made === null ? '–' : formatBytes(backup.size, language) },
          {
            label: text.next,
            value: backup.next === null ? '–' : <RelativeTime iso={backup.next} />,
          },
          ...(last === null
            ? []
            : [
                {
                  label: t.backups.restore.lastRestore,
                  value: (
                    <span className="flex flex-wrap items-baseline gap-x-2">
                      <StatusPill tone={last.error === '' ? 'ok' : 'danger'}>
                        {last.error === '' ? text.succeeded : text.failed}
                      </StatusPill>
                      <span className="text-small break-all text-ink-3">
                        {last.error === ''
                          ? t.backups.restore.lastRestoreOk(last.file, last.administrator)
                          : t.backups.restore.lastRestoreFailed(last.file)}{' '}
                        <RelativeTime iso={last.at} />
                      </span>
                    </span>
                  ),
                },
              ]),
        ]}
      />
      {backup.error !== '' && (
        <p className="rounded-row border border-danger/25 bg-danger/8 px-3.5 py-3 text-small break-words text-ink">
          {backup.error}
        </p>
      )}
      {backup.destinations.length > 0 && (
        <div>
          <h4 className="mb-2 text-[14px] font-medium text-ink">{t.backups.destinations.title}</h4>
          <ul className="overflow-hidden rounded-row border border-line-2">
            {backup.destinations.map((destination) => {
              const status = destinationStatus(t, destination)
              return (
                <li key={destination.id} className="px-3.5 py-2.5 [&+&]:border-t [&+&]:border-line">
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <code className="min-w-0 flex-1 font-mono text-small break-all text-ink">
                      {destination.name}
                    </code>
                    <StatusPill tone={status.tone}>{status.label}</StatusPill>
                  </div>
                  <p className="mt-0.5 text-small text-ink-3">
                    {t.backups.destinations.lastUpload}{' '}
                    {destination.ranAt === null ? (
                      t.backups.destinations.none
                    ) : (
                      <RelativeTime iso={destination.ranAt} />
                    )}
                    {destination.newestFile !== '' && (
                      <>
                        {' · '}
                        {t.backups.destinations.newest}{' '}
                        <code className="font-mono">{destination.newestFile}</code>
                      </>
                    )}
                  </p>
                  {destination.error !== '' && (
                    <p className="mt-1 text-small break-words text-danger">{destination.error}</p>
                  )}
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}
