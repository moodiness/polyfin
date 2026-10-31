import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchOwnAudioRegion,
  queryClient,
  queryKeys,
  saveOwnAudioRegion,
  type AudioRegion,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Field, InlineError, Select, SkeletonRows, useToast } from '@/ui'

/** A choice of audio region: None, then the regions offered, by name, in the server's order. */
export function AudioRegionSelect({
  value,
  regions,
  onValue,
  disabled,
}: {
  value: string
  regions: readonly string[]
  onValue: (region: string) => void
  disabled: boolean
}) {
  const { t } = useI18n()
  const text = t.account.audioRegion
  return (
    <Select
      value={value}
      options={[
        { value: '', label: text.none },
        ...regions.map((region) => ({ value: region, label: text.regions[region] ?? region })),
      ]}
      onValue={onValue}
      disabled={disabled}
      className="max-w-xs"
    />
  )
}

/** My account › Audio region: the regional dub the user prefers, saved as it changes. */
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
    <Field
      label={text.label}
      hideLabel
      error={save.isError ? errorMessage(t, save.error) : undefined}
    >
      <AudioRegionSelect
        value={current.data.region}
        regions={current.data.regions}
        onValue={(chosen) => save.mutate(chosen)}
        disabled={save.isPending}
      />
    </Field>
  )
}
