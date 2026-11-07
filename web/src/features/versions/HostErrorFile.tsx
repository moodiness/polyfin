import { WarningIcon } from '@phosphor-icons/react'
import type { VersionStream } from '@/api'
import { hostErrorFileText } from '@/features/system/problems'
import { useI18n } from '@/i18n'

/**
 * On a version whose host answers an error file instead of the videos, the notice Health shows:
 * the host by its name, and since when. Nothing otherwise.
 */
export function HostErrorFile({ stream }: { stream: VersionStream }) {
  const { t, language } = useI18n()
  const errorFile = stream.hostErrorFile
  if (errorFile === null) return null
  return (
    <span className="flex items-start gap-1.5 text-warn">
      <WarningIcon size="1rem" aria-hidden="true" className="mt-0.5 shrink-0" />
      {hostErrorFileText(t, language, errorFile.host, errorFile.since)}
    </span>
  )
}
