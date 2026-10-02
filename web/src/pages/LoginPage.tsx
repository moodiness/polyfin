import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, signIn } from '@/api'
import { buttonPrimary, Card, Notice, PageHeader, TextField } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Rendered in place of any page while signed out, so the URL still names the requested page. */
export default function LoginPage() {
  const { t } = useI18n()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')

  const mutation = useMutation({
    mutationFn: signIn,
    meta: { public: true },
    onSuccess: (user) => queryClient.setQueryData(queryKeys.session, user),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ name: name.trim(), password })
  }

  return (
    <div className="mx-auto max-w-md">
      <PageHeader title={t.login.title} description={t.login.description} />
      <Card>
        <form onSubmit={submit} className="space-y-4">
          <TextField
            label={t.login.name}
            value={name}
            onValue={setName}
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            required
          />
          <TextField
            label={t.login.password}
            type="password"
            value={password}
            onValue={setPassword}
            autoComplete="current-password"
            required
          />
          {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
          <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
            {mutation.isPending ? t.login.submitting : t.login.submit}
          </button>
        </form>
      </Card>
    </div>
  )
}
