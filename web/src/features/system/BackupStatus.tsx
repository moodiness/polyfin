import type { Backup } from '@/api'
import { StatusPill } from '@/ui'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import { Facts, RelativeTime } from './parts'

/**
 * How the database backups go: the last run, the last backup made (its file and size), and when
 * the next one is. A failed run shows its error under the facts. Takes the `backup` of the
 * health, which is null when backups are off: show `t.system.health.backupOff` then instead.
 */
export default function BackupStatus({ backup }: { backup: Backup }) {
  const { language, t } = useI18n()
  const text = t.system.health.backup
  const made = backup.madeAt
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
        ]}
      />
      {backup.error !== '' && (
        <p className="rounded-row border border-danger/25 bg-danger/8 px-3.5 py-3 text-small break-words text-ink">
          {backup.error}
        </p>
      )}
    </div>
  )
}
