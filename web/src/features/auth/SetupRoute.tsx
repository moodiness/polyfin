import { useState, type FormEvent } from 'react'
import { UserPlusIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, setup, type Status } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, Notice, TextInput } from '@/ui'
import { AuthFrame } from './AuthFrame'

/** The first administrator, shown by the session gate at any address until one exists. */
export default function SetupRoute() {
  const { t, language } = useI18n()
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
    // The server adopts the interface language for the names it generates.
    if (!mismatched)
      mutation.mutate({ setupCode: setupCode.trim(), name: name.trim(), password, language })
  }

  return (
    <AuthFrame title={t.setup.title} lede={t.setup.description} wide>
      <Notice className="mb-6">{t.setup.codeHelp}</Notice>
      <form onSubmit={submit} noValidate className="flex flex-col gap-5">
        <Field label={t.setup.setupCode} help={t.setup.setupCodeHint}>
          <TextInput
            value={setupCode}
            onValue={setSetupCode}
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            placeholder="XXXX-XXXX"
            mono
            autoFocus
            required
          />
        </Field>
        <Field label={t.setup.name} help={t.common.nameRule}>
          <TextInput
            value={name}
            onValue={setName}
            autoComplete="username"
            maxLength={64}
            required
          />
        </Field>
        <Field label={t.setup.password} help={t.common.passwordRule}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={setPassword}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field
          label={t.setup.confirmPassword}
          error={mismatch ? t.common.passwordMismatch : undefined}
        >
          <TextInput
            type="password"
            revealable
            value={confirmation}
            onValue={setConfirmation}
            autoComplete="new-password"
            required
          />
        </Field>
        {mutation.isError && (
          <Notice tone="danger" live>
            {errorMessage(t, mutation.error)}
          </Notice>
        )}
        <Button
          type="submit"
          variant="primary"
          icon={UserPlusIcon}
          loading={mutation.isPending}
          className="w-full"
        >
          {mutation.isPending ? t.setup.submitting : t.setup.submit}
        </Button>
      </form>
    </AuthFrame>
  )
}
