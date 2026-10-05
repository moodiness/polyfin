import { HouseIcon } from '@phosphor-icons/react'
import { PageLayout } from '@/app/PageLayout'
import { useI18n } from '@/i18n'
import { ButtonLink } from '@/ui'

/** Any address the app does not know. */
export default function NotFoundRoute() {
  const { t } = useI18n()
  return (
    <PageLayout title={t.notFound.title} lede={t.notFound.description}>
      <div>
        <ButtonLink to="/" icon={HouseIcon}>
          {t.notFound.home}
        </ButtonLink>
      </div>
    </PageLayout>
  )
}
