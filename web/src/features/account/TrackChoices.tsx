import { EraserIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchOwnTrackChoices,
  forgetOwnTrackChoices,
  queryClient,
  queryKeys,
  type TrackChoices,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, ConfirmDialog, InlineError, SkeletonRows, useToast } from '@/ui'

/**
 * My account › Track choices: how many series and movies keep the audio and subtitle tracks the
 * user chose, and Forget my track choices, which asks first.
 */
export default function OwnTrackChoices() {
  const { t } = useI18n()
  const text = t.account.trackChoices
  const toast = useToast()
  const [asking, setAsking] = useState(false)
  const current = useQuery({
    queryKey: queryKeys.ownTrackChoices,
    queryFn: ({ signal }) => fetchOwnTrackChoices(signal),
  })
  const forget = useMutation({
    mutationFn: forgetOwnTrackChoices,
    onSuccess: () => {
      queryClient.setQueryData<TrackChoices>(queryKeys.ownTrackChoices, { titles: 0 })
      setAsking(false)
      toast(text.done)
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
  const { titles } = current.data
  return (
    <div className="flex flex-wrap items-center gap-4">
      <p className="text-sm text-ink-2">{titles === 0 ? text.none : text.kept(titles)}</p>
      <Button
        variant="secondary"
        icon={EraserIcon}
        disabled={titles === 0}
        onClick={() => {
          forget.reset()
          setAsking(true)
        }}
      >
        {text.forget}
      </Button>
      <ConfirmDialog
        open={asking}
        onClose={() => setAsking(false)}
        onConfirm={() => forget.mutate()}
        title={text.title}
        confirmLabel={forget.isPending ? text.forgetting : text.confirm}
        busy={forget.isPending}
        error={forget.isError ? errorMessage(t, forget.error) : undefined}
      >
        <p>{text.confirmHelp}</p>
      </ConfirmDialog>
    </div>
  )
}
