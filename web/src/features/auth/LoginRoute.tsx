import { useState, type FormEvent } from 'react'
import { IdentificationCardIcon, SignInIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  fetchStatus,
  queryClient,
  queryKeys,
  signIn,
  signInSecondStep,
  type SessionUser,
  type TwoStepSetup,
} from '@/api'
import { RecoveryCodes, SetupKey } from '@/features/account/TwoStep'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type en from '@/i18n/en/auth'
import { Button, ExternalButtonLink, Field, Notice, TextInput } from '@/ui'
import { AuthFrame } from './AuthFrame'

/** What the identity provider's sign-in can come back with, as the server names it. */
type ProviderOutcome = keyof typeof en.login.providerOutcomes

/**
 * What went wrong at the identity provider, as its sign-in sends the browser back to the admin
 * app with ?provider=: taken out of the address as the app loads, so that a reload does not show
 * it again. An outcome the app does not know reads as a failure.
 */
const providerOutcome = takeProviderOutcome()

function takeProviderOutcome(): ProviderOutcome | null {
  const address = new URL(window.location.href)
  const outcome = address.searchParams.get('provider')
  if (outcome === null) return null
  address.searchParams.delete('provider')
  window.history.replaceState(
    window.history.state,
    '',
    address.pathname + address.search + address.hash,
  )
  const known: readonly string[] = [
    'provider_refused',
    'provider_no_account',
    'account_disabled',
    'outside_allowed_hours',
    'outside_local_network',
    'too_many_attempts',
  ]
  return known.includes(outcome) ? (outcome as ProviderOutcome) : 'provider_failed'
}

/** Where a sign-in stands: its second step, then, after a setup, the recovery codes. */
type Step =
  | { kind: 'password' }
  | { kind: 'code' }
  | { kind: 'setup'; setup: TwoStepSetup }
  | { kind: 'recovery'; user: SessionUser; codes: string[] }

/**
 * Sign-in, shown by the session gate in place of any page while no one is signed in, so the URL
 * still names the requested page. An account with two-step sign-in then enters a code of its app or
 * a recovery code; an administrator the server requires it of sets it up first. No session exists
 * before. With an identity provider set up, a button signs in through it instead, its second step
 * being the provider's; the server sends the browser back here, signed in or with what went wrong.
 */
export default function LoginRoute() {
  const { t } = useI18n()
  const text = t.login
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [step, setStep] = useState<Step>({ kind: 'password' })
  const [code, setCode] = useState('')
  const provider = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  }).data?.provider

  const mutation = useMutation({
    mutationFn: signIn,
    meta: { public: true },
    onSuccess: (answer) => {
      if (answer.twoStep === undefined) {
        queryClient.setQueryData(queryKeys.session, answer.user)
        return
      }
      setCode('')
      setStep(answer.twoStep === 'code' ? { kind: 'code' } : { kind: 'setup', setup: answer.setup })
    },
  })
  const second = useMutation({
    mutationFn: signInSecondStep,
    meta: { public: true },
    onSuccess: (answer) => {
      if (answer.recoveryCodes !== undefined) {
        setStep({ kind: 'recovery', user: answer.user, codes: answer.recoveryCodes })
        return
      }
      queryClient.setQueryData(queryKeys.session, answer.user)
    },
    onError: (error) => {
      // The step expired: the password is asked again.
      if (error instanceof ApiError && error.code === 'two_step_expired') {
        setPassword('')
        setStep({ kind: 'password' })
        mutation.reset()
      }
    },
  })

  function restart() {
    second.reset()
    mutation.reset()
    setPassword('')
    setStep({ kind: 'password' })
  }

  if (step.kind === 'recovery') {
    return (
      <AuthFrame title={text.recoveryTitle} lede={t.account.twoStep.turnedOn} wide>
        <RecoveryCodes
          codes={step.codes}
          doneLabel={text.continue}
          onDone={() => queryClient.setQueryData(queryKeys.session, step.user)}
        />
      </AuthFrame>
    )
  }

  if (step.kind !== 'password') {
    const codeError = second.isError ? errorMessage(t, second.error) : undefined
    const setup = step.kind === 'setup'
    return (
      <AuthFrame
        wide={setup}
        title={setup ? text.setupTitle : text.codeTitle}
        lede={setup ? text.setupDescription : text.codeDescription}
      >
        <form
          noValidate
          className="flex flex-col gap-5"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault()
            second.mutate(code.trim())
          }}
        >
          {setup && <SetupKey setup={step.setup} />}
          <Field
            label={setup ? t.account.twoStep.code : text.code}
            help={setup ? t.account.twoStep.codeHelp : text.codeHelp}
            error={codeError}
          >
            <TextInput
              value={code}
              onValue={(value) => {
                second.reset()
                setCode(value)
              }}
              autoComplete="one-time-code"
              autoCapitalize="none"
              spellCheck={false}
              mono
              autoFocus
              required
              invalid={codeError !== undefined}
            />
          </Field>
          <Button
            type="submit"
            variant="primary"
            icon={SignInIcon}
            loading={second.isPending}
            disabled={code.trim() === ''}
            className="w-full"
          >
            {second.isPending ? text.verifying : text.verify}
          </Button>
          <Button variant="ghost" onClick={restart} className="w-full">
            {text.restart}
          </Button>
        </form>
      </AuthFrame>
    )
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ name: name.trim(), password })
  }

  return (
    <AuthFrame title={text.title} lede={text.description}>
      <form onSubmit={submit} className="flex flex-col gap-5">
        <Field label={text.name}>
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
        <Field label={text.password}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={setPassword}
            autoComplete="current-password"
            required
          />
        </Field>
        {(mutation.isError || second.isError) && (
          <Notice tone="danger" live>
            {errorMessage(t, mutation.isError ? mutation.error : second.error)}
          </Notice>
        )}
        {providerOutcome !== null && !mutation.isError && (
          <Notice tone="danger" live>
            {text.providerOutcomes[providerOutcome]}
          </Notice>
        )}
        <Button
          type="submit"
          variant="primary"
          icon={SignInIcon}
          loading={mutation.isPending}
          className="w-full"
        >
          {mutation.isPending ? text.submitting : text.submit}
        </Button>
        {provider && (
          <ExternalButtonLink
            href={`${provider.start}?app=admin`}
            icon={IdentificationCardIcon}
            className="w-full"
          >
            {text.providerSignIn(provider.name)}
          </ExternalButtonLink>
        )}
      </form>
    </AuthFrame>
  )
}
