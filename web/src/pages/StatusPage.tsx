import StatusPanel from '@/components/StatusPanel'
import { PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'

export default function StatusPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.status.title} description={t.status.description} />
      <StatusPanel />
    </>
  )
}
