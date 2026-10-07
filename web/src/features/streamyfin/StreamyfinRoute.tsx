import { PageLayout } from '@/app/PageLayout'
import { useI18n } from '@/i18n'
import StreamyfinEditor from './StreamyfinEditor'

/** `/streamyfin`: the rows of the home screen of Streamyfin, a Jellyfin app, in order. */
export default function StreamyfinRoute() {
  const { t } = useI18n()
  return (
    <PageLayout title={t.streamyfin.title} lede={t.streamyfin.description}>
      <StreamyfinEditor />
    </PageLayout>
  )
}
