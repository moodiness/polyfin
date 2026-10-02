import AddonManager from '@/components/AddonManager'
import { PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'

export default function AddonsPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.addons.title} description={t.addons.description} />
      <AddonManager scope="shared" />
    </>
  )
}
