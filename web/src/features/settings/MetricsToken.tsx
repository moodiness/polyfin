import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { newMetricsToken, revealServerSecret } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, ConfirmDialog, SecretField, useToast } from '@/ui'

/**
 * The metrics token Polyfin made, shown as saved keys are, with a button that makes a new one
 * once confirmed: the former one stops working at once.
 */
export default function MetricsToken({ saved }: { saved: boolean }) {
  const { t } = useI18n()
  const s = t.settings
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  // Each new token draws the field anew, hiding the one revealed before.
  const [made, setMade] = useState(0)
  const remake = useMutation({
    mutationFn: newMetricsToken,
    onSuccess: () => {
      setConfirming(false)
      setMade((count) => count + 1)
      toast(s.newMetricsTokenMade, { tone: 'ok' })
    },
  })
  if (!saved) {
    return <p className="text-small text-ink-3">{s.metricsTokenPending}</p>
  }
  return (
    <>
      <SecretField
        key={made}
        label={s.metricsToken}
        help={s.metricsTokenHelp}
        saved
        value={undefined}
        reveal={() => revealServerSecret('metricsToken')}
        replaceable={false}
        removable={false}
        actions={<Button onClick={() => setConfirming(true)}>{s.newMetricsToken}</Button>}
      />
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={() => remake.mutate()}
        title={s.newMetricsTokenTitle}
        confirmLabel={s.newMetricsToken}
        tone="danger"
        busy={remake.isPending}
        error={remake.isError ? errorMessage(t, remake.error) : undefined}
      >
        {s.newMetricsTokenConfirm}
      </ConfirmDialog>
    </>
  )
}
