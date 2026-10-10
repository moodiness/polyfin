import { PageLayout } from '@/app/PageLayout'
import { useI18n } from '@/i18n'
import StatisticsView from './StatisticsView'

/** `/system/statistics`: how the server is used, by everyone or one user, for a period. */
export default function StatisticsRoute() {
  const { t } = useI18n()
  return (
    <PageLayout title={t.statistics.title} lede={t.statistics.description}>
      <StatisticsView scope="server" />
    </PageLayout>
  )
}
