import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, setup, type Status } from '@/api'
import { buttonPrimary, Card, Notice, PageHeader, TextField } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export default function SetupPage() {
  const { t } = useI18n()
  const [setupCode, setSetupCode] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [mismatch, setMismatch] = useState(false)

  const mutation = useMutation({
    mutationFn: setup,
    meta: { public: true },
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.session, user)
      queryClient.setQueryData<Status>(queryKeys.status, (old) =>
        old ? { ...old, setupRequired: false } : old,
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.status })
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const mismatched = password !== confirmation
    setMismatch(mismatched)
    if (!mismatched) mutation.mutate({ setupCode: setupCode.trim(), name: name.trim(), password })
  }

  return (
    <div className="mx-auto max-w-lg">
      <PageHeader title={t.setup.title} description={t.setup.description} />
      <Card>
        <p className="mb-5 rounded-lg border border-fin-4/40 bg-fin-4/5 p-3 text-sm text-zinc-200">
          {t.setup.codeHelp}
        </p>
        <form onSubmit={submit} noValidate className="space-y-4">
          <TextField
            label={t.setup.setupCode}
            hint={t.setup.setupCodeHint}
            value={setupCode}
            onValue={setSetupCode}
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            required
          />
          <TextField
            label={t.setup.name}
            hint={t.common.nameRule}
            value={name}
            onValue={setName}
            autoComplete="username"
            maxLength={64}
            required
          />
          <TextField
            label={t.setup.password}
            hint={t.common.passwordRule}
            type="password"
            value={password}
            onValue={setPassword}
            autoComplete="new-password"
            required
          />
          <TextField
            label={t.setup.confirmPassword}
            type="password"
            value={confirmation}
            onValue={setConfirmation}
            autoComplete="new-password"
            error={mismatch ? t.common.passwordMismatch : undefined}
            required
          />
          {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
          <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
            {mutation.isPending ? t.setup.submitting : t.setup.submit}
          </button>
        </form>
      </Card>
    </div>
  )
}
