import { PageLayout } from '@/app/PageLayout'
import { useI18n } from '@/i18n'
import EpisodeOrders from './EpisodeOrders'
import LibraryEditor from './LibraryEditor'
import SmartLists from './SmartLists'

/**
 * `/libraries`: which catalogs of the server's addons are libraries in Jellyfin apps, in order, the
 * server's smart lists, and the series shown in an episode order of their own.
 */
export default function LibrariesRoute() {
  const { t } = useI18n()
  return (
    <PageLayout title={t.libraries.title} lede={t.libraries.description}>
      <LibraryEditor scope="shared" />
      <SmartLists scope="shared" />
      <EpisodeOrders />
    </PageLayout>
  )
}
