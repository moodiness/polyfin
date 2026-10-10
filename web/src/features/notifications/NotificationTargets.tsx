import { BellIcon, PlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  ApiError,
  createNotificationTarget,
  deleteNotificationTarget,
  fetchNotifications,
  queryClient,
  queryKeys,
  testNotificationTarget,
  updateNotificationTarget,
  type NotificationDraft,
  type NotificationEvent,
  type NotificationKind,
  type NotificationScope,
  type NotificationTarget,
} from '@/api'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  IconTile,
  InlineError,
  Modal,
  Notice,
  SecretField,
  Select,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
  type StatusTone,
} from '@/ui'

/** The letters in each kind's tile. */
const monograms: Record<NotificationKind, string> = {
  webhook: 'W',
  discord: 'D',
  ntfy: 'N',
  email: 'E',
  telegram: 'T',
  gotify: 'G',
  pushover: 'P',
}

/**
 * The notification targets of the server (Settings › Notifications) or of the signed-in user
 * (My account › Notifications): each with its state, Send a test, Edit and Delete, and Add.
 */
export default function NotificationTargets({ scope }: { scope: NotificationScope }) {
  const { t } = useI18n()
  const text = t.notifications
  const [editing, setEditing] = useState<NotificationTarget | 'new' | null>(null)
  const notifications = useQuery({
    queryKey: queryKeys.notifications(scope),
    queryFn: ({ signal }) => fetchNotifications(scope, signal),
  })

  if (notifications.isPending) return <SkeletonRows rows={2} label={text.loading} />
  if (notifications.isError) {
    return (
      <InlineError onRetry={() => void notifications.refetch()} retrying={notifications.isFetching}>
        {errorMessage(t, notifications.error)}
      </InlineError>
    )
  }
  const { targets, events, kinds, emailAvailable } = notifications.data
  return (
    <div className="flex flex-col gap-4">
      {targets.length === 0 ? (
        <EmptyState icon={BellIcon} title={text.empty}>
          {text.emptyHint[scope]}
        </EmptyState>
      ) : (
        <ul className="overflow-hidden rounded-panel border border-line-2 bg-s1">
          {targets.map((target) => (
            <TargetRow
              key={target.id}
              scope={scope}
              target={target}
              onEdit={() => setEditing(target)}
            />
          ))}
        </ul>
      )}
      <div>
        <Button icon={PlusIcon} onClick={() => setEditing('new')}>
          {text.add}
        </Button>
      </div>
      {editing !== null && (
        <TargetModal
          scope={scope}
          target={editing === 'new' ? null : editing}
          events={events}
          kinds={kinds}
          emailAvailable={emailAvailable}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

/** Puts a target as the server answered it into the list. */
function store(scope: NotificationScope, target: NotificationTarget) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.notifications(scope) })
  queryClient.setQueryData(
    queryKeys.notifications(scope),
    (old: { targets: NotificationTarget[] } | undefined) =>
      old && {
        ...old,
        targets: old.targets.some((current) => current.id === target.id)
          ? old.targets.map((current) => (current.id === target.id ? target : current))
          : [...old.targets, target],
      },
  )
}

function TargetRow({
  scope,
  target,
  onEdit,
}: {
  scope: NotificationScope
  target: NotificationTarget
  onEdit: () => void
}) {
  const { language, t } = useI18n()
  const text = t.notifications
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const test = useMutation({
    mutationFn: () => testNotificationTarget(scope, target.id),
    onSuccess: (result) => {
      store(scope, result.target)
      toast(result.delivered ? text.testDelivered(target.name) : text.testFailed(target.name), {
        tone: result.delivered ? 'ok' : 'danger',
      })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  const remove = useMutation({
    mutationFn: () => deleteNotificationTarget(scope, target.id),
    onSuccess: () => {
      setConfirming(false)
      toast(text.deleted(target.name))
      void queryClient.invalidateQueries({ queryKey: queryKeys.notifications(scope) })
    },
  })

  const status: { tone: StatusTone; label: string } = !target.enabled
    ? { tone: 'muted', label: text.status.off }
    : target.problem === 'refused' || target.problem === 'unreadable'
      ? { tone: 'danger', label: text.status[target.problem] }
      : target.problem !== null
        ? { tone: 'warn', label: text.status[target.problem] }
        : target.lastSentAt !== null
          ? { tone: 'ok', label: text.status.working }
          : { tone: 'muted', label: text.status.waiting }
  const answer = text.answer(String(target.problemStatus ?? '?'), target.kind === 'email')
  const problem =
    target.problem === 'refused'
      ? target.kind === 'email'
        ? text.problem.refusedEmail(answer)
        : text.problem.refused(answer)
      : target.problem === 'rejected'
        ? text.problem.rejected(answer)
        : target.problem === null
          ? null
          : text.problem[target.problem]
  // What the target is: its address, or a Telegram target's chat; a Pushover target shows none.
  const detail = target.kind === 'telegram' ? target.chat : target.address
  const headingId = useId()
  return (
    <li
      aria-labelledby={headingId}
      className="grid grid-cols-[40px_minmax(0,1fr)_auto] items-start gap-x-4 px-[22px] py-5 max-sm:grid-cols-[40px_minmax(0,1fr)] max-sm:gap-x-3.5 max-sm:px-4 [&+&]:border-t [&+&]:border-line"
    >
      <IconTile letters={monograms[target.kind]} />
      <div className="min-w-0">
        <h3
          id={headingId}
          className="flex h-10 items-center text-[15px] font-semibold tracking-[-0.01em] text-ink"
        >
          {target.name}
        </h3>
        <p className="-mt-1.5 text-control break-words text-ink-2">
          {text.kinds[target.kind]}
          {detail !== '' && (
            <>
              {' · '}
              <span className="font-mono">{detail}</span>
            </>
          )}
          {target.topic !== '' && <span className="font-mono">/{target.topic}</span>}
        </p>
        <p className="mt-1 text-small text-ink-3">
          {target.events.length === 0
            ? text.noEvents
            : target.events.map((event) => text.eventNames[event]).join(', ')}
          {' · '}
          {target.lastSentAt === null ? (
            text.nothingSent
          ) : (
            <>
              {text.lastSent}
              <time dateTime={target.lastSentAt} title={dateTime(target.lastSentAt, language)}>
                {relativeTime(target.lastSentAt, language, t.time.justNow)}
              </time>
            </>
          )}
        </p>
      </div>
      <div className="flex h-10 items-center justify-end gap-1.5 max-sm:col-span-full max-sm:mt-3 max-sm:h-auto max-sm:flex-wrap">
        <StatusPill tone={status.tone} className="max-sm:mr-auto">
          {status.label}
        </StatusPill>
        <Button
          variant="secondary"
          loading={test.isPending}
          disabled={target.problem === 'unreadable'}
          onClick={() => test.mutate()}
        >
          {test.isPending ? text.testing : text.test}
        </Button>
        <Button variant="ghost" onClick={onEdit}>
          {text.edit}
        </Button>
        <Button variant="ghost" onClick={() => setConfirming(true)}>
          {text.delete}
        </Button>
      </div>
      {problem !== null && (
        <div className="col-start-2 -col-end-1 mt-4 max-sm:col-span-full">
          <Notice tone={status.tone === 'danger' ? 'danger' : 'warn'}>{problem}</Notice>
        </div>
      )}
      <ConfirmDialog
        open={confirming}
        onClose={() => {
          setConfirming(false)
          remove.reset()
        }}
        onConfirm={() => remove.mutate()}
        title={text.deleteTitle(target.name)}
        confirmLabel={text.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {text.deleteBody}
      </ConfirmDialog>
    </li>
  )
}

/** The codes about the address, shown under it. */
const addressCodes = ['invalid_target_address', 'private_target_address', 'invalid_email_address']

/** The form to add a target, or to change one, in a floating panel. */
function TargetModal({
  scope,
  target,
  events,
  kinds,
  emailAvailable,
  onClose,
}: {
  scope: NotificationScope
  target: NotificationTarget | null
  events: NotificationEvent[]
  kinds: NotificationKind[]
  emailAvailable: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.notifications
  const toast = useToast()
  const formId = useId()
  const [kind, setKind] = useState<NotificationKind>(target?.kind ?? 'webhook')
  const [name, setName] = useState(target?.name ?? '')
  // A webhook's or Discord target's secret address: undefined keeps the saved one.
  const [secretAddress, setSecretAddress] = useState<string | undefined>(undefined)
  // An ntfy or Gotify target's server, and an email target's recipient.
  const [server, setServer] = useState(
    target?.kind === 'ntfy' || target?.kind === 'gotify' ? target.address : '',
  )
  const [recipient, setRecipient] = useState(target?.kind === 'email' ? target.address : '')
  const [topic, setTopic] = useState(target?.topic ?? '')
  const [chat, setChat] = useState(target?.chat ?? '')
  // Tokens and keys: undefined keeps the saved one.
  const [token, setToken] = useState<string | undefined>(undefined)
  const [userKey, setUserKey] = useState<string | undefined>(undefined)
  const [chosen, setChosen] = useState<NotificationEvent[]>(target?.events ?? events)
  const [enabled, setEnabled] = useState(target?.enabled ?? true)

  const mutation = useMutation({
    mutationFn: (draft: NotificationDraft) =>
      target === null
        ? createNotificationTarget(scope, draft)
        : updateNotificationTarget(scope, target.id, draft),
    onSuccess: (saved) => {
      store(scope, saved)
      toast(target === null ? text.created(saved.name) : text.saved(saved.name))
      onClose()
    },
  })
  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const fieldError = (codes: string[]) =>
    code !== null && codes.includes(code) ? message : undefined
  const nameError = fieldError(['invalid_target_name'])
  const addressError = fieldError(addressCodes)
  const topicError = fieldError(['invalid_topic'])
  const chatError = fieldError(['invalid_chat'])
  const tokenError = fieldError(['invalid_token'])
  const userKeyError = fieldError(['invalid_user_key'])
  const otherError =
    mutation.isError &&
    !nameError &&
    !addressError &&
    !topicError &&
    !chatError &&
    !tokenError &&
    !userKeyError
      ? message
      : undefined
  // An email target cannot be added without the server's SMTP server.
  const blocked = target === null && kind === 'email' && !emailAvailable

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const draft: NotificationDraft = { name: name.trim(), events: chosen, enabled }
    if (target === null) draft.kind = kind
    if (token !== undefined) draft.token = token.trim()
    switch (kind) {
      case 'ntfy':
        draft.address = server.trim()
        draft.topic = topic.trim()
        break
      case 'gotify':
        draft.address = server.trim()
        break
      case 'email':
        draft.address = recipient.trim()
        break
      case 'telegram':
        draft.chat = chat.trim()
        break
      case 'pushover':
        if (userKey !== undefined) draft.userKey = userKey.trim()
        break
      default:
        if (secretAddress !== undefined) draft.address = secretAddress.trim()
    }
    mutation.mutate(draft)
  }

  const change =
    <T,>(set: (value: T) => void) =>
    (value: T) => {
      mutation.reset()
      set(value)
    }

  return (
    <Modal
      open
      onClose={onClose}
      title={target === null ? text.addTitle : text.editTitle(target.name)}
      width={540}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            loading={mutation.isPending}
            disabled={blocked}
          >
            {target === null
              ? mutation.isPending
                ? text.creating
                : text.create
              : mutation.isPending
                ? text.saving
                : text.save}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        {target === null && (
          <Field label={text.kind} help={text.kindHelp[kind]}>
            <Select
              value={kind}
              options={kinds.map((value) => ({ value, label: text.kinds[value] }))}
              onValue={change(setKind)}
              className="max-w-xs"
            />
          </Field>
        )}
        <Field label={text.name} help={text.nameHelp} error={nameError}>
          <TextInput
            value={name}
            onValue={change(setName)}
            autoComplete="off"
            maxLength={64}
            invalid={nameError !== undefined}
          />
        </Field>
        {blocked ? (
          <Notice tone="warn">{text.emailUnavailable[scope]}</Notice>
        ) : (
          <KindFields
            kind={kind}
            target={target}
            values={{ secretAddress, server, recipient, topic, chat, token, userKey }}
            set={{
              secretAddress: change(setSecretAddress),
              server: change(setServer),
              recipient: change(setRecipient),
              topic: change(setTopic),
              chat: change(setChat),
              token: change(setToken),
              userKey: change(setUserKey),
            }}
            errors={{ addressError, topicError, chatError, tokenError, userKeyError }}
          />
        )}
        <fieldset className="space-y-3">
          <legend className="mb-1 text-[15px] font-medium text-ink">{text.events}</legend>
          <p className="text-small text-ink-3">{text.eventsHelp[scope]}</p>
          {events.map((event) => (
            <Checkbox
              key={event}
              label={text.eventNames[event]}
              help={text.eventHelp[event]}
              checked={chosen.includes(event)}
              onChange={(checked) => {
                mutation.reset()
                setChosen((current) =>
                  checked ? [...current, event] : current.filter((value) => value !== event),
                )
              }}
            />
          ))}
        </fieldset>
        <Checkbox label={text.enabled} checked={enabled} onChange={change(setEnabled)} />
        {otherError && <Notice tone="danger">{otherError}</Notice>}
      </form>
    </Modal>
  )
}

/** What a target's own fields hold, by field. */
type KindValues = {
  secretAddress: string | undefined
  server: string
  recipient: string
  topic: string
  chat: string
  token: string | undefined
  userKey: string | undefined
}

/** The fields of a target of kind: its address, server, topic, chat, tokens and keys. */
function KindFields({
  kind,
  target,
  values,
  set,
  errors,
}: {
  kind: NotificationKind
  target: NotificationTarget | null
  values: KindValues
  set: { [field in keyof KindValues]: (value: KindValues[field]) => void }
  errors: {
    addressError: string | undefined
    topicError: string | undefined
    chatError: string | undefined
    tokenError: string | undefined
    userKeyError: string | undefined
  }
}) {
  const { t } = useI18n()
  const text = t.notifications
  const { addressError, topicError, chatError, tokenError, userKeyError } = errors
  // A token that is required: it may be replaced, not removed.
  const requiredToken = (label: string, help: string) => (
    <SecretField
      label={label}
      help={help}
      saved={target?.tokenSet ?? false}
      value={values.token}
      onValue={set.token}
      removable={false}
      error={tokenError}
    />
  )
  switch (kind) {
    case 'ntfy':
      return (
        <>
          <Field label={text.ntfyServer} help={text.ntfyServerHelp} error={addressError}>
            <TextInput
              value={values.server}
              onValue={set.server}
              placeholder="https://ntfy.sh"
              autoComplete="off"
              spellCheck={false}
              mono
              invalid={addressError !== undefined}
            />
          </Field>
          <Field label={text.topic} help={text.topicHelp} error={topicError}>
            <TextInput
              value={values.topic}
              onValue={set.topic}
              autoComplete="off"
              spellCheck={false}
              mono
              maxLength={64}
              invalid={topicError !== undefined}
            />
          </Field>
          <SecretField
            label={text.token}
            help={text.tokenHelp}
            saved={target?.tokenSet ?? false}
            value={values.token}
            onValue={set.token}
            error={tokenError}
          />
        </>
      )
    case 'email':
      return (
        <Field label={text.emailAddress} help={text.emailAddressHelp} error={addressError}>
          <TextInput
            type="email"
            value={values.recipient}
            onValue={set.recipient}
            placeholder="sam@example.org"
            autoComplete="off"
            spellCheck={false}
            mono
            maxLength={254}
            invalid={addressError !== undefined}
          />
        </Field>
      )
    case 'telegram':
      return (
        <>
          <Field label={text.chat} help={text.chatHelp} error={chatError}>
            <TextInput
              value={values.chat}
              onValue={set.chat}
              placeholder="-1001234567890"
              autoComplete="off"
              spellCheck={false}
              mono
              maxLength={33}
              invalid={chatError !== undefined}
            />
          </Field>
          {requiredToken(text.botToken, text.botTokenHelp)}
        </>
      )
    case 'gotify':
      return (
        <>
          <Field label={text.gotifyServer} help={text.gotifyServerHelp} error={addressError}>
            <TextInput
              value={values.server}
              onValue={set.server}
              placeholder="https://gotify.example.org"
              autoComplete="off"
              spellCheck={false}
              mono
              invalid={addressError !== undefined}
            />
          </Field>
          {requiredToken(text.appToken, text.appTokenHelp.gotify)}
        </>
      )
    case 'pushover':
      return (
        <>
          <SecretField
            label={text.userKey}
            help={text.userKeyHelp}
            saved={target !== null && target.problem !== 'unreadable'}
            value={values.userKey}
            onValue={set.userKey}
            removable={false}
            error={userKeyError}
          />
          {requiredToken(text.appToken, text.appTokenHelp.pushover)}
        </>
      )
    default:
      return (
        <SecretField
          label={kind === 'discord' ? text.discordAddress : text.webhookAddress}
          help={text.addressHelp[kind]}
          saved={target !== null && target.problem !== 'unreadable'}
          value={values.secretAddress}
          onValue={set.secretAddress}
          removable={false}
          status={target === null ? null : text.savedAddress(target.address)}
          error={addressError}
        />
      )
  }
}
