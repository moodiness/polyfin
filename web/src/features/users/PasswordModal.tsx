import { KeyIcon } from '@phosphor-icons/react'
import { useId, useState, type FormEvent } from 'react'
import type { User } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, Modal, TextInput, useToast } from '@/ui'
import { useUserPatch } from './shared'

/**
 * A new password for the user, set at once; it signs out their Jellyfin devices. Opened from the
 * user's page and from the "…" menu of their row in the list.
 */
export default function PasswordModal({
  user,
  open,
  onClose,
}: {
  user: User
  open: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const toast = useToast()
  const formId = useId()
  const [password, setPassword] = useState('')
  const resetPassword = useUserPatch(user)
  const error = resetPassword.isError ? errorMessage(t, resetPassword.error) : undefined

  function close() {
    resetPassword.reset()
    setPassword('')
    onClose()
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title={t.users.resetPassword}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            icon={KeyIcon}
            loading={resetPassword.isPending}
          >
            {resetPassword.isPending ? t.common.saving : t.users.resetPassword}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="p-5"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          resetPassword.mutate(
            { password },
            {
              onSuccess: () => {
                toast(t.users.passwordReset)
                close()
              },
            },
          )
        }}
      >
        <Field
          label={t.users.newPassword}
          help={`${t.common.passwordRule} ${t.users.resetPasswordHelp}`}
          error={error}
        >
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => {
              resetPassword.reset()
              setPassword(value)
            }}
            autoComplete="new-password"
            required
            invalid={error !== undefined}
          />
        </Field>
      </form>
    </Modal>
  )
}
