import { dateTime, relativeTime } from '@/format'
import { useI18n } from '@/i18n'

/** "3 hours ago", "in 9 hours", with the full date and time on hover. */
export function RelativeTime({ iso }: { iso: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}
