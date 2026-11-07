import { useMutation, useQuery } from '@tanstack/react-query'
import { fetchOwnSound, queryClient, queryKeys, saveOwnSound, type OwnSound } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { InlineError, Notice, SettingRow, SkeletonRows, useToast } from '@/ui'

/**
 * My account › Sound: Boost the volume in the web player and Convert the sound, saved as they
 * change; shown, not changed, while the user's group holds them.
 */
export function OwnSoundOptions() {
  const { t } = useI18n()
  const text = t.account.sound
  const toast = useToast()
  const current = useQuery({
    queryKey: queryKeys.ownSound,
    queryFn: ({ signal }) => fetchOwnSound(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnSound,
    onSuccess: (saved) => {
      queryClient.setQueryData<OwnSound>(queryKeys.ownSound, saved)
      // An administrator's own page under Users shows them.
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(text.saved)
    },
  })

  if (current.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (current.isError) {
    return (
      <InlineError onRetry={() => void current.refetch()} retrying={current.isFetching}>
        {errorMessage(t, current.error)}
      </InlineError>
    )
  }
  const { boostVolume, convertSound, followsGroup } = current.data
  const disabled = followsGroup || save.isPending
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
        <SettingRow
          kind="switch"
          label={text.boostVolume}
          help={text.boostVolumeHelp}
          checked={boostVolume}
          onChange={(on) => save.mutate({ boostVolume: on })}
          disabled={disabled}
        />
        <SettingRow
          kind="switch"
          label={text.convertSound}
          help={text.convertSoundHelp}
          checked={convertSound}
          onChange={(on) => save.mutate({ convertSound: on })}
          disabled={disabled}
        />
      </div>
      {save.isError && <InlineError>{errorMessage(t, save.error)}</InlineError>}
      {followsGroup && <Notice>{text.group}</Notice>}
    </div>
  )
}
