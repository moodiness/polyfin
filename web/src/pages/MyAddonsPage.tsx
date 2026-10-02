import { useMutation, useQuery } from '@tanstack/react-query'
import { fetchAddonPreferences, queryClient, queryKeys, saveAddonPreferences } from '@/api'
import AddonManager from '@/components/AddonManager'
import LibraryEditor from '@/components/LibraryEditor'
import { Card, Checkbox, Loading, Notice, PageHeader } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export default function MyAddonsPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.myAddons.title} description={t.myAddons.description} />
      <div className="space-y-6">
        <Card title={t.myAddons.preferenceTitle}>
          <SharedAddonsSwitch />
        </Card>
        <AddonManager scope="me" />
        <LibraryEditor scope="me" />
      </div>
    </>
  )
}

function SharedAddonsSwitch() {
  const { t } = useI18n()
  const preferences = useQuery({
    queryKey: queryKeys.addonPreferences,
    queryFn: ({ signal }) => fetchAddonPreferences(signal),
  })
  const mutation = useMutation({
    mutationFn: saveAddonPreferences,
    onSuccess: (saved) => queryClient.setQueryData(queryKeys.addonPreferences, saved),
  })

  if (preferences.isPending) return <Loading />
  if (preferences.isError) return <Notice kind="error">{errorMessage(t, preferences.error)}</Notice>

  return (
    <div className="space-y-3">
      <Checkbox
        label={t.myAddons.useShared}
        help={t.myAddons.useSharedHelp}
        checked={preferences.data.useSharedAddons}
        onChange={(useSharedAddons) => mutation.mutate({ useSharedAddons })}
      />
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && <Notice kind="success">{t.myAddons.preferenceSaved}</Notice>}
    </div>
  )
}
