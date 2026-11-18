import { KeyIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { fetchServerActions, queryKeys } from '@/api'
import { ActionDialog } from '@/features/home/ServerActions'
import { useI18n } from '@/i18n'
import { Button } from '@/ui'
import { PlainRow } from './parts'

/**
 * Settings › Security › Signing secret: what the secret signs and what protects it, and a button
 * that makes a new one. Its confirmation is that of the server's actions, as Polyfin restarts to
 * sign with it: it says what stops working, and asks for the password or a two-step code.
 */
export default function SigningSecret() {
  const { t } = useI18n()
  const s = t.settings
  const actions = useQuery({
    queryKey: queryKeys.serverActions,
    queryFn: ({ signal }) => fetchServerActions(signal),
  }).data
  const [confirming, setConfirming] = useState(false)
  return (
    <PlainRow anchor="signing-secret">
      <div
        role="group"
        aria-labelledby="signing-secret-title"
        aria-describedby="signing-secret-help"
      >
        <p id="signing-secret-title" className="text-h3 text-ink max-md:text-[1.125rem]">
          {s.signingSecret}
        </p>
        <p id="signing-secret-help" className="mt-2 max-w-[66ch] text-small text-ink-2">
          {s.signingSecretHelp}
        </p>
        <Button
          className="mt-3.5"
          icon={KeyIcon}
          disabled={actions === undefined}
          onClick={() => setConfirming(true)}
        >
          {s.newSigningSecret}
        </Button>
      </div>
      {confirming && actions !== undefined && (
        <ActionDialog action="secret" actions={actions} onClose={() => setConfirming(false)} />
      )}
    </PlainRow>
  )
}
