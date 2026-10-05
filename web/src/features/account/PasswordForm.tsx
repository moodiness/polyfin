import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { ApiError, changePassword, queryClient, queryKeys } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, Notice, TextInput, useToast } from '@/ui'

/** Changes the signed-in user's password; the server then signs out their Jellyfin devices. */
export default function PasswordForm() {
  const { t } = useI18n()
  const text = t.account
  const toast = useToast()
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [mismatch, setMismatch] = useState(false)

  const mutation = useMutation({
    mutationFn: changePassword,
    onSuccess: () => {
      setCurrentPassword('')
      setNewPassword('')
      setConfirmation('')
      toast(text.passwordChanged, { tone: 'ok' })
      // The server signs out every Jellyfin device of this account.
      void queryClient.invalidateQueries({ queryKey: queryKeys.myDevices })
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const mismatched = newPassword !== confirmation
    setMismatch(mismatched)
    if (!mismatched) mutation.mutate({ currentPassword, newPassword })
  }

  // The server's errors go under the field they are about; others under the form.
  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const failure = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const currentError = code === 'wrong_password' ? failure : undefined
  const newError = code === 'invalid_password' ? failure : undefined
  const formError = currentError === undefined && newError === undefined ? failure : undefined

  return (
    <form onSubmit={submit} noValidate className="flex max-w-[360px] flex-col gap-5">
      <Field label={text.currentPassword} error={currentError}>
        <TextInput
          type="password"
          value={currentPassword}
          onValue={(value) => {
            setCurrentPassword(value)
            if (currentError) mutation.reset()
          }}
          autoComplete="current-password"
          required
        />
      </Field>
      <Field label={text.newPassword} help={t.common.passwordRule} error={newError}>
        <TextInput
          type="password"
          revealable
          value={newPassword}
          onValue={(value) => {
            setNewPassword(value)
            if (newError) mutation.reset()
          }}
          autoComplete="new-password"
          required
        />
      </Field>
      <Field label={text.confirmPassword} error={mismatch ? t.common.passwordMismatch : undefined}>
        <TextInput
          type="password"
          value={confirmation}
          onValue={setConfirmation}
          autoComplete="new-password"
          required
        />
      </Field>
      {formError && (
        <Notice tone="danger" live>
          {formError}
        </Notice>
      )}
      <div>
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {mutation.isPending ? text.changingPassword : text.changePassword}
        </Button>
      </div>
    </form>
  )
}
