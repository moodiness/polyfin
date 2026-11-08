import { WarningIcon } from '@phosphor-icons/react'
import type { VersionStream } from '@/api'
import { hostErrorFileText } from '@/features/system/problems'
import { useI18n } from '@/i18n'

/**
 * On a version whose host answers an error file instead of the videos, the notice Health shows:
 * the host by its name, and since when, then what to check on a line of its own. Nothing
 * otherwise.
 */
export function HostErrorFile({ stream }: { stream: VersionStream }) {
  const { t, language } = useI18n()
  const errorFile = stream.hostErrorFile
  if (errorFile === null) return null
  return (
    <span className="flex items-start gap-1.5">
      <WarningIcon size="1rem" aria-hidden="true" className="mt-0.5 shrink-0 text-warn" />
      <span className="flex min-w-0 flex-col">
        <span className="text-warn">
          {hostErrorFileText(t, language, errorFile.host, errorFile.since)}
        </span>
        <span className="text-ink-2">{t.system.health.problems.hostErrorFileHint}</span>
      </span>
    </span>
  )
}
