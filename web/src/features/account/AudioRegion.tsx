import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchOwnAudioRegion,
  queryClient,
  queryKeys,
  saveOwnAudioRegion,
  type AudioRegion,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import { InlineError, SettingRow, SkeletonRows, useToast } from '@/ui'

/** The choices of audio region: None, then the regions offered, by name, in the server's order. */
function audioRegionOptions(regions: readonly string[], t: Messages) {
  const text = t.account.audioRegion
  return [
    { value: '', label: text.none },
    ...regions.map((region) => ({ value: region, label: text.regions[region] ?? region })),
  ]
}

/** My account › Audio region: the regional dub the user prefers, a setting row saved as it changes. */
export function OwnAudioRegion() {
  const { t } = useI18n()
  const text = t.account.audioRegion
  const toast = useToast()
  const current = useQuery({
    queryKey: queryKeys.ownAudioRegion,
    queryFn: ({ signal }) => fetchOwnAudioRegion(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnAudioRegion,
    onSuccess: (saved) => {
      queryClient.setQueryData<AudioRegion>(queryKeys.ownAudioRegion, saved)
      // An administrator's own page under Users shows their audio region.
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(text.saved)
    },
  })

  if (current.isPending) return <SkeletonRows rows={1} boxed label={t.common.loading} />
  if (current.isError) {
    return (
      <InlineError onRetry={() => void current.refetch()} retrying={current.isFetching}>
        {errorMessage(t, current.error)}
      </InlineError>
    )
  }
  return (
    <SettingRow
      kind="choice"
      label={text.label}
      help={text.help}
      value={current.data.region}
      options={audioRegionOptions(current.data.regions, t)}
      onChange={(chosen) => save.mutate(chosen)}
      disabled={save.isPending}
      error={save.isError ? errorMessage(t, save.error) : undefined}
    />
  )
}
