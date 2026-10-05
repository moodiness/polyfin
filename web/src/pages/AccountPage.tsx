import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { changePassword, fetchMyDevices, queryClient, queryKeys, signOutMyDevice } from '@/api'
import DeviceList from '@/components/DeviceList'
import TrackingServices from '@/components/TrackingServices'
import { buttonPrimary, Card, Notice, PageHeader, TextField } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export default function AccountPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.account.title} description={t.account.description} />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card title={t.account.passwordTitle}>
          <PasswordForm />
        </Card>
        <Card title={t.account.devicesTitle}>
          <DeviceList
            queryKey={queryKeys.myDevices}
            load={fetchMyDevices}
            signOut={signOutMyDevice}
          />
        </Card>
        <div className="lg:col-span-2">
          <Card title={t.account.tracking.title}>
            <p className="-mt-2 mb-5 max-w-prose text-sm text-muted">
              {t.account.tracking.description}
            </p>
            <TrackingServices />
          </Card>
        </div>
      </div>
    </>
  )
}

function PasswordForm() {
  const { t } = useI18n()
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

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <p className="text-sm text-muted">{t.account.passwordHelp}</p>
      <TextField
        label={t.account.currentPassword}
        type="password"
        value={currentPassword}
        onValue={setCurrentPassword}
        autoComplete="current-password"
        required
      />
      <TextField
        label={t.account.newPassword}
        hint={t.common.passwordRule}
        type="password"
        value={newPassword}
        onValue={setNewPassword}
        autoComplete="new-password"
        required
      />
      <TextField
        label={t.account.confirmPassword}
        type="password"
        value={confirmation}
        onValue={setConfirmation}
        autoComplete="new-password"
        error={mismatch ? t.common.passwordMismatch : undefined}
        required
      />
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && <Notice kind="success">{t.account.passwordChanged}</Notice>}
      <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
        {mutation.isPending ? t.common.saving : t.common.save}
      </button>
    </form>
  )
}
