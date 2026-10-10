import { useState, type FormEvent } from 'react'
import { UserPlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router'
import {
  acceptInvite,
  ApiError,
  fetchPublicInvite,
  fetchStatus,
  queryClient,
  queryKeys,
  type Status,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, InlineError, Notice, Spinner, TextInput } from '@/ui'
import { signInToWebClient } from '@/webClientSession'
import { AuthFrame } from './AuthFrame'

/** The codes of a link that can no longer create an account, each with a page of its own. */
const goneCodes = ['invite_unknown', 'invite_used_up', 'invite_expired', 'invite_revoked'] as const
type GoneCode = (typeof goneCodes)[number]

function goneCode(error: unknown): GoneCode | null {
  if (!(error instanceof ApiError)) return null
  return goneCodes.find((code) => code === error.code) ?? null
}

/** Codes about one field, shown under it rather than above the button. */
const nameCodes = ['invalid_name', 'name_taken']
const passwordCodes = ['invalid_password']

/**
 * `/invite/:token`: the page an invite link opens, with no session. The guest picks a name and a
 * password; once their account exists, they land signed in on the web client, or on the admin
 * app when the server has no web client. A link that can no longer create an account says why.
 */
export default function InviteRoute() {
  const { t } = useI18n()
  const { token = '' } = useParams()
  const navigate = useNavigate()
  const invite = useQuery({
    queryKey: ['invite', token],
    queryFn: ({ signal }) => fetchPublicInvite(token, signal),
    retry: false,
  })
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [mismatch, setMismatch] = useState(false)

  const mutation = useMutation({
    mutationFn: async (body: { name: string; password: string }) => {
      const joined = await acceptInvite(token, body)
      const server = queryClient.getQueryData<Status>(queryKeys.status)
      if (joined.webClient && server !== undefined) {
        try {
          await signInToWebClient(
            { id: server.serverId, name: invite.data?.serverName ?? '' },
            body.name,
            body.password,
          )
        } catch {
          // The account exists: the web client asks for the name and password instead.
        }
      }
      return joined
    },
    meta: { public: true },
    onSuccess: (joined) => {
      if (joined.webClient) {
        window.location.assign('/web/')
        return
      }
      queryClient.setQueryData(queryKeys.session, joined.user)
      void navigate('/', { replace: true })
    },
  })

  const gone = goneCode(invite.error) ?? goneCode(mutation.error)
  if (gone !== null) {
    const page = t.invite.gone[gone]
    return <AuthFrame title={page.title} lede={page.description} />
  }
  if (invite.isPending || status.isPending) {
    return (
      <p role="status" className="flex items-center gap-2.5 text-control text-ink-3">
        <Spinner />
        {t.common.loading}
      </p>
    )
  }
  if (invite.isError || status.isError) {
    const failed = invite.isError ? invite : status
    return (
      <InlineError onRetry={() => void failed.refetch()} retrying={failed.isFetching}>
        {errorMessage(t, failed.error)}
      </InlineError>
    )
  }

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const nameError = code !== null && nameCodes.includes(code) ? message : undefined
  const passwordError = code !== null && passwordCodes.includes(code) ? message : undefined
  const otherError = mutation.isError && !nameError && !passwordError ? message : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const mismatched = password !== confirmation
    setMismatch(mismatched)
    if (!mismatched) mutation.mutate({ name: name.trim(), password })
  }

  return (
    <AuthFrame title={t.invite.title(invite.data.serverName)} lede={t.invite.description}>
      <form onSubmit={submit} noValidate className="flex flex-col gap-5">
        <Field label={t.invite.name} help={t.common.nameRule} error={nameError}>
          <TextInput
            value={name}
            onValue={(value) => {
              mutation.reset()
              setName(value)
            }}
            autoComplete="username"
            maxLength={64}
            autoFocus
            required
            invalid={nameError !== undefined}
          />
        </Field>
        <Field label={t.invite.password} help={t.common.passwordRule} error={passwordError}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => {
              mutation.reset()
              setPassword(value)
            }}
            autoComplete="new-password"
            required
            invalid={passwordError !== undefined}
          />
        </Field>
        <Field
          label={t.invite.confirmPassword}
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
        {otherError && (
          <Notice tone="danger" live>
            {otherError}
          </Notice>
        )}
        <Button
          type="submit"
          variant="primary"
          icon={UserPlusIcon}
          loading={mutation.isPending || mutation.isSuccess}
          className="w-full"
        >
          {mutation.isPending || mutation.isSuccess ? t.invite.submitting : t.invite.submit}
        </Button>
      </form>
    </AuthFrame>
  )
}
