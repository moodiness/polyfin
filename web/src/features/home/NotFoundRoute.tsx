import { DeadEnd } from '@/app/DeadEnd'
import { useI18n } from '@/i18n'

/** Any address the app does not know. */
export default function NotFoundRoute() {
  const { t } = useI18n()
  return (
    <DeadEnd title={t.notFound.title} description={t.notFound.description} home={t.notFound.home} />
  )
}
