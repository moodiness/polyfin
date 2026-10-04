import RecentActivity from '@/components/RecentActivity'
import { useSessionUser } from '@/components/session'
import StatusPanel from '@/components/StatusPanel'
import { PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'

export default function StatusPage() {
  const { t } = useI18n()
  const user = useSessionUser()
  return (
    <>
      <PageHeader title={t.status.title} description={t.status.description} />
      <StatusPanel />
      {user.isAdministrator && (
        <div className="mt-6">
          <RecentActivity />
        </div>
      )}
    </>
  )
}
