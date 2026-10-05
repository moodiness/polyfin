import { useState, type FormEvent } from 'react'
import { SignInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, signIn } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, Notice, TextInput } from '@/ui'
import { AuthFrame } from './AuthFrame'

/**
 * Sign-in, shown by the session gate in place of any page while no one is signed in, so the URL
 * still names the requested page.
 */
export default function LoginRoute() {
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
    <AuthFrame title={t.login.title} lede={t.login.description}>
      <form onSubmit={submit} className="flex flex-col gap-5">
        <Field label={t.login.name}>
          <TextInput
            value={name}
            onValue={setName}
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            autoFocus
            required
          />
        </Field>
        <Field label={t.login.password}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={setPassword}
            autoComplete="current-password"
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
          icon={SignInIcon}
          loading={mutation.isPending}
          className="w-full"
        >
          {mutation.isPending ? t.login.submitting : t.login.submit}
        </Button>
      </form>
    </AuthFrame>
  )
}
