import { PageLayout } from '@/app/PageLayout'
import { useI18n } from '@/i18n'
import LibraryEditor from './LibraryEditor'

/** `/libraries`: which catalogs of the server's addons are libraries in Jellyfin apps, in order. */
export default function LibrariesRoute() {
  const { t } = useI18n()
  return (
    <PageLayout title={t.libraries.title} lede={t.libraries.description}>
      <LibraryEditor scope="shared" />
    </PageLayout>
  )
}
