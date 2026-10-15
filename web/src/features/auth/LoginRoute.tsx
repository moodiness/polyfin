import { useState, type FormEvent } from 'react'
import { SignInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import {
  ApiError,
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
import { Button, Field, Notice, TextInput } from '@/ui'
import { AuthFrame } from './AuthFrame'

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
 * before.
 */
export default function LoginRoute() {
  const { t } = useI18n()
  const text = t.login
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [step, setStep] = useState<Step>({ kind: 'password' })
  const [code, setCode] = useState('')

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
        <Button
          type="submit"
          variant="primary"
          icon={SignInIcon}
          loading={mutation.isPending}
          className="w-full"
        >
          {mutation.isPending ? text.submitting : text.submit}
        </Button>
      </form>
    </AuthFrame>
  )
}
