import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { queryClient, queryKeys, resetOwnPreferences } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, ConfirmDialog, useToast } from '@/ui'

/**
 * Reset my preferences, among the page's actions: asks first, listing what goes back to the
 * server's defaults and what stays, then resets the metadata language, the audio region, the web
 * player theme unless the administrator chose it, the home rows turned off, the track choices and
 * the preferences Jellyfin apps saved in the account.
 */
export default function ResetPreferences() {
  const { t } = useI18n()
  const text = t.resetPreferences
  const toast = useToast()
  const [asking, setAsking] = useState(false)
  const reset = useMutation({
    mutationFn: resetOwnPreferences,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownMetadataLanguage })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownWebTheme })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownAudioRegion })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownHomeRows })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownTrackChoices })
      void queryClient.invalidateQueries({ queryKey: queryKeys.session })
      // An administrator's own page under Users shows their metadata language, theme and audio region.
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      setAsking(false)
      toast(text.done)
    },
  })

  return (
    <>
      <Button
        variant="secondary"
        icon={ArrowCounterClockwiseIcon}
        onClick={() => {
          reset.reset()
          setAsking(true)
        }}
      >
        {text.button}
      </Button>
      <ConfirmDialog
        open={asking}
        onClose={() => setAsking(false)}
        onConfirm={() => reset.mutate()}
        title={text.title}
        confirmLabel={reset.isPending ? text.resetting : text.confirm}
        busy={reset.isPending}
        error={reset.isError ? errorMessage(t, reset.error) : undefined}
      >
        <div className="space-y-3">
          <div>
            <p>{text.help}</p>
            <ul className="mt-1 list-disc space-y-0.5 ps-5">
              <li>{text.metadataLanguage}</li>
              <li>{text.webTheme}</li>
              <li>{text.audioRegion}</li>
              <li>{text.homeRows}</li>
              <li>{text.nextUpWithResume}</li>
              <li>{text.trackChoices}</li>
              <li>{text.appPreferences}</li>
            </ul>
          </div>
          <p>{text.kept}</p>
        </div>
      </ConfirmDialog>
    </>
  )
}
