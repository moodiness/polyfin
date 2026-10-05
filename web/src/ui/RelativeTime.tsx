import { dateTime, relativeTime } from '@/format'
import { useI18n } from '@/i18n'

/** A time as "3 hours ago" or "in 9 hours", with the full date and time on hover. */
export function RelativeTime({ iso, className }: { iso: string; className?: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)} className={className}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}
