import { CheckIcon, CopyIcon, LinkIcon, ProhibitIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  ApiError,
  createInvite,
  fetchInvites,
  queryClient,
  queryKeys,
  revokeInvite,
  type Invite,
  type InviteState,
  type NewInvite,
  type User,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  NumberInput,
  RelativeTime,
  Row,
  RowList,
  Select,
  SkeletonRows,
  StatusPill,
  useToast,
  type StatusTone,
} from '@/ui'

const stateTones: Record<InviteState, StatusTone> = {
  active: 'ok',
  used_up: 'muted',
  expired: 'muted',
  revoked: 'muted',
}

/** The expiries offered, in days; 0 never expires. */
const expiryDays = [1, 7, 30, 90, 0]

/**
 * The invite links of `/users`: each with its uses, expiry, model, creator and state, and a
 * button revoking an active one. `onCreate` opens the form creating one.
 */
export function InviteLinks({ onCreate }: { onCreate: () => void }) {
  const { t } = useI18n()
  const text = t.invites
  const toast = useToast()
  const invites = useQuery({
    queryKey: queryKeys.invites,
    queryFn: ({ signal }) => fetchInvites(signal),
  })
  const [asked, setAsked] = useState<Invite | null>(null)
  const revoke = useMutation({
    mutationFn: (invite: Invite) => revokeInvite(invite.id),
    onSuccess: () => {
      setAsked(null)
      toast(text.revoked, { tone: 'ok' })
    },
    onSettled: (_data, error) => {
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.invites })
      }
    },
  })

  return (
    <Block title={text.title} count={invites.data?.length}>
      {invites.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : invites.isError ? (
        <InlineError onRetry={() => void invites.refetch()} retrying={invites.isFetching}>
          {errorMessage(t, invites.error)}
        </InlineError>
      ) : invites.data.length === 0 ? (
        <EmptyState
          icon={LinkIcon}
          title={text.empty}
          action={
            <Button icon={LinkIcon} onClick={onCreate}>
              {text.create}
            </Button>
          }
        >
          {text.emptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={text.title}>
          {invites.data.map((invite) => (
            <Row
              key={invite.id}
              leading={LinkIcon}
              title={text.uses(invite.uses, invite.maxUses)}
              muted={invite.state !== 'active'}
              meta={
                <span className="truncate">
                  {invite.model === null
                    ? text.newUserSettings
                    : text.settingsOf(invite.model.name)}
                  {' · '}
                  {invite.expiresAt === null ? (
                    text.neverExpires
                  ) : (
                    <>
                      {invite.state === 'expired' ? text.expiredLabel : text.expiresLabel}{' '}
                      <RelativeTime iso={invite.expiresAt} />
                    </>
                  )}
                  {' · '}
                  {invite.createdBy === null
                    ? text.createdByDeleted
                    : text.createdBy(invite.createdBy.name)}{' '}
                  <RelativeTime iso={invite.createdAt} />
                </span>
              }
              trailing={
                <span className="flex items-center gap-3">
                  <StatusPill tone={stateTones[invite.state]}>
                    {text.states[invite.state]}
                  </StatusPill>
                  {invite.state === 'active' && (
                    <Button
                      size="sm"
                      variant="danger"
                      icon={ProhibitIcon}
                      loading={revoke.isPending && revoke.variables.id === invite.id}
                      onClick={() => {
                        revoke.reset()
                        setAsked(invite)
                      }}
                    >
                      {text.revoke}
                    </Button>
                  )}
                </span>
              }
            />
          ))}
        </RowList>
      )}
      <ConfirmDialog
        open={asked !== null}
        onClose={() => setAsked(null)}
        onConfirm={() => asked && revoke.mutate(asked)}
        title={text.revokeTitle}
        confirmLabel={revoke.isPending ? text.revoking : text.revoke}
        busy={revoke.isPending}
        error={revoke.isError ? errorMessage(t, revoke.error) : undefined}
      >
        {text.revokeConfirm}
      </ConfirmDialog>
    </Block>
  )
}

/**
 * The form creating an invite link, in a floating panel over the list: how many accounts it may
 * create, when it expires, and whose settings the accounts copy. Once created, it shows the link,
 * which the server gives this one time only.
 */
export function CreateInviteModal({
  open,
  onClose,
  users,
}: {
  open: boolean
  onClose: () => void
  users: readonly User[]
}) {
  const { t } = useI18n()
  const text = t.invites
  const formId = useId()
  const [maxUses, setMaxUses] = useState<number | null>(1)
  const [days, setDays] = useState(7)
  const [model, setModel] = useState('')
  const mutation = useMutation({
    mutationFn: createInvite,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.invites }),
  })

  function close() {
    mutation.reset()
    setMaxUses(1)
    setDays(7)
    setModel('')
    onClose()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({
      maxUses: maxUses ?? 0,
      expiresInDays: days === 0 ? null : days,
      modelUserId: model === '' ? null : model,
    })
  }

  if (mutation.isSuccess) {
    return <NewLink invite={mutation.data} open={open} onClose={close} key={mutation.data.id} />
  }

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const usesError = code === 'invalid_invite_uses' ? message : undefined
  const otherError = mutation.isError && usesError === undefined ? message : undefined

  return (
    <Modal
      open={open}
      onClose={close}
      title={text.create}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={mutation.isPending}>
            {mutation.isPending ? text.creating : text.submit}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <Field label={text.maxUses} help={text.maxUsesHelp} error={usesError}>
          <NumberInput
            value={maxUses}
            onValue={(value) => {
              mutation.reset()
              setMaxUses(value)
            }}
            min={1}
            max={100}
            step={1}
            invalid={usesError !== undefined}
          />
        </Field>
        <Field label={text.expires}>
          <Select
            value={days}
            onValue={setDays}
            options={expiryDays.map((value) => ({
              value,
              label: value === 0 ? text.never : text.after(value),
            }))}
          />
        </Field>
        <Field label={text.model} help={text.modelHelp}>
          <Select
            value={model}
            onValue={setModel}
            options={[
              { value: '', label: text.newUser },
              ...users.map((user) => ({ value: user.id, label: user.name })),
            ]}
          />
        </Field>
        {otherError && <Notice tone="danger">{otherError}</Notice>}
      </form>
    </Modal>
  )
}

/** The link of an invite just created, to copy: the server gives it this one time only. */
function NewLink({
  invite,
  open,
  onClose,
}: {
  invite: NewInvite
  open: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.invites
  const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle')
  const link =
    invite.url ?? `${window.location.origin}${import.meta.env.BASE_URL}invite/${invite.token}`

  async function copyLink() {
    try {
      // The clipboard is missing outside secure contexts, such as plain http on a local network.
      await navigator.clipboard.writeText(link)
      setCopy('copied')
    } catch {
      setCopy('failed')
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={text.createdTitle}
      width={520}
      footer={
        <>
          <Button onClick={onClose}>{text.done}</Button>
          <Button
            variant="primary"
            icon={copy === 'copied' ? CheckIcon : CopyIcon}
            onClick={() => void copyLink()}
          >
            {text.copy}
          </Button>
        </>
      }
    >
      <div className="space-y-4 p-5">
        <Notice tone="warn" live>
          {text.createdNotice}
        </Notice>
        <code className="block rounded-field border border-line-2 bg-bg px-3.5 py-2.5 font-mono text-control break-all text-ink select-all">
          {link}
        </code>
        <div aria-live="polite">
          {copy === 'copied' && <Notice tone="ok">{text.copied}</Notice>}
          {copy === 'failed' && <Notice tone="danger">{text.copyFailed}</Notice>}
        </div>
      </div>
    </Modal>
  )
}
