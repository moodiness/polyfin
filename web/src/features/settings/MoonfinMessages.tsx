import {
  ChatCenteredTextIcon,
  InfoIcon,
  MagnifyingGlassIcon,
  PaperPlaneTiltIcon,
  PlusIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  createMoonfinMessage,
  deleteMoonfinMessage,
  fetchMoonfinMessages,
  fetchUsers,
  moonfinMessageColors,
  orderMoonfinMessages,
  queryClient,
  queryKeys,
  updateMoonfinMessage,
  type MoonfinMessage,
  type MoonfinMessageAudience,
  type MoonfinMessageColor,
  type MoonfinMessageDelivery,
  type MoonfinMessageDraft,
  type User,
} from '@/api'
import { dateTime, errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Modal,
  MoveButtons,
  Notice,
  Row,
  RowList,
  Segmented,
  SkeletonRows,
  Textarea,
  TextInput,
  useToast,
} from '@/ui'
import MessageMarkdown, { webLink } from './MessageMarkdown'

/** The bounds of a message's texts, in characters, as the server checks them. */
const lengths = { title: 120, body: 2000, linkLabel: 60, linkUrl: 2000 }

/** The audiences, in the order the dialog offers them. */
const audiences: MoonfinMessageAudience[] = ['all', 'users', 'admins']

/**
 * The tokens each color of a message is drawn with: Moonfin's apps draw them in fixed blue,
 * green, yellow, red and white.
 */
const tones: Record<MoonfinMessageColor, { edge: string; ink: string; dot: string }> = {
  white: { edge: 'border-ink/70', ink: 'text-ink', dot: 'bg-ink' },
  blue: { edge: 'border-link', ink: 'text-link', dot: 'bg-link' },
  green: { edge: 'border-ok', ink: 'text-ok', dot: 'bg-ok' },
  yellow: { edge: 'border-warn', ink: 'text-warn', dot: 'bg-warn' },
  red: { edge: 'border-danger', ink: 'text-danger', dot: 'bg-danger' },
}

/** An RFC 3339 time as a datetime-local field holds it, in the browser's time zone. */
function localInput(iso: string): string {
  if (iso === '') return ''
  const at = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`
}

/** A datetime-local field's value, in the browser's time zone, as RFC 3339; empty stays empty. */
function isoTime(local: string): string {
  return local === '' ? '' : new Date(local).toISOString()
}

/**
 * Settings › General › Messages to Moonfin's apps, under Serve Moonfin's companion: the messages
 * the administrator writes to the users of Moonfin's apps, in the order the apps show them, each
 * with whom it is shown to, its dates and whether they show it now, Move, Edit and Delete, and
 * Write a message.
 */
export default function MoonfinMessages() {
  const { t } = useI18n()
  const text = t.settings.moonfinMessages
  const toast = useToast()
  const [editing, setEditing] = useState<MoonfinMessage | 'new' | null>(null)
  const messages = useQuery({
    queryKey: queryKeys.moonfinMessages,
    queryFn: ({ signal }) => fetchMoonfinMessages(signal),
  })
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
  })
  const order = useMutation({
    mutationFn: (ids: string[]) => orderMoonfinMessages(ids),
    onSuccess: (answer) => queryClient.setQueryData(queryKeys.moonfinMessages, answer),
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })

  const head = (
    <div>
      <h3 className="text-h3 text-ink max-md:text-[1.125rem]">{text.title}</h3>
      <p className="mt-1 max-w-[66ch] text-small text-ink-2">{text.help}</p>
    </div>
  )
  if (messages.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {head}
        <SkeletonRows rows={2} label={text.loading} />
      </div>
    )
  }
  if (messages.isError) {
    return (
      <div className="flex flex-col gap-4">
        {head}
        <InlineError onRetry={() => void messages.refetch()} retrying={messages.isFetching}>
          {errorMessage(t, messages.error)}
        </InlineError>
      </div>
    )
  }
  const list = messages.data.messages
  const move = (from: number, to: number) => {
    const ids = list.map((message) => message.id)
    const [moved] = ids.splice(from, 1)
    ids.splice(to, 0, moved)
    order.mutate(ids)
  }
  return (
    <div className="flex flex-col gap-4" data-testid="moonfin-messages">
      {head}
      {list.length === 0 ? (
        <EmptyState icon={ChatCenteredTextIcon} title={text.empty}>
          {text.emptyHint}
        </EmptyState>
      ) : (
        <RowList variant="plain" aria-label={text.title}>
          {list.map((message, index) => (
            <MessageEntry
              key={message.id}
              message={message}
              index={index}
              count={list.length}
              moving={order.isPending}
              onMove={(to) => move(index, to)}
              users={users.data ?? []}
              onEdit={() => setEditing(message)}
            />
          ))}
        </RowList>
      )}
      <div>
        <Button icon={PlusIcon} onClick={() => setEditing('new')}>
          {text.add}
        </Button>
      </div>
      {editing !== null && (
        <MessageModal
          message={editing === 'new' ? null : editing}
          users={users.data ?? []}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

/** A message's title, or the start of its body when it has none. */
function messageName(message: MoonfinMessageDraft, untitled: string): string {
  if (message.title !== '') return message.title
  const start = message.body.replace(/\s+/g, ' ').trim()
  if (start === '') return untitled
  return start.length > 60 ? `${start.slice(0, 60)}…` : start
}

/** A row of the list: its title, its color, how it arrives, whom it is shown to and when. */
function MessageEntry({
  message,
  index,
  count,
  moving,
  onMove,
  users,
  onEdit,
}: {
  message: MoonfinMessage
  index: number
  count: number
  moving: boolean
  onMove: (to: number) => void
  users: User[]
  onEdit: () => void
}) {
  const { t, language } = useI18n()
  const text = t.settings.moonfinMessages
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const name = messageName(message, text.untitled)
  const remove = useMutation({
    mutationFn: () => deleteMoonfinMessage(message.id),
    onSuccess: () => {
      setConfirming(false)
      toast(text.deleted(name))
      void queryClient.invalidateQueries({ queryKey: queryKeys.moonfinMessages })
    },
  })
  const audience =
    message.audience === 'all'
      ? text.shownToAll
      : message.audience === 'admins'
        ? text.shownToAdmins
        : text.shownToUsers(
            message.users.map((id) => users.find((user) => user.id === id)?.name ?? text.someone),
          )
  const dates = [
    ...(message.start !== '' ? [text.from(dateTime(message.start, language))] : []),
    ...(message.end !== '' ? [text.until(dateTime(message.end, language))] : []),
  ]
  return (
    <Row
      leading={
        <span
          aria-hidden="true"
          className={`grid size-10 shrink-0 place-items-center rounded-full bg-s2 ${tones[message.color].ink}`}
        >
          <InfoIcon size={22} />
        </span>
      }
      title={name}
      titleAside={
        <span className="flex flex-wrap gap-1.5">
          <Badge>{text.colors[message.color]}</Badge>
          {message.delivery === 'popup' && <Badge tone="accent">{text.popupBadge}</Badge>}
          {message.state !== 'shown' && (
            <Badge tone={message.state === 'scheduled' ? 'warn' : 'neutral'}>
              {text.states[message.state]}
            </Badge>
          )}
        </span>
      }
      meta={<span className="min-w-0 break-words">{[audience, ...dates].join(' · ')}</span>}
      trailing={
        <div className="flex flex-wrap items-center justify-end gap-2 max-md:gap-1">
          <Button variant="ghost" size="sm" onClick={onEdit}>
            {text.edit}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setConfirming(true)}>
            {text.delete}
          </Button>
          <MoveButtons name={name} index={index} count={count} disabled={moving} onMove={onMove} />
          <ConfirmDialog
            open={confirming}
            onClose={() => {
              setConfirming(false)
              remove.reset()
            }}
            onConfirm={() => remove.mutate()}
            title={text.deleteTitle(name)}
            confirmLabel={text.delete}
            tone="danger"
            busy={remove.isPending}
            error={remove.isError ? errorMessage(t, remove.error) : undefined}
          >
            {text.deleteBody}
          </ConfirmDialog>
        </div>
      }
    />
  )
}

/** The form to write a message, or to change one, with its preview, in a floating panel. */
function MessageModal({
  message,
  users,
  onClose,
}: {
  message: MoonfinMessage | null
  users: User[]
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.settings.moonfinMessages
  const toast = useToast()
  const formId = useId()
  const [title, setTitle] = useState(message?.title ?? '')
  const [body, setBody] = useState(message?.body ?? '')
  const [color, setColor] = useState<MoonfinMessageColor>(message?.color ?? 'white')
  const [delivery, setDelivery] = useState<MoonfinMessageDelivery>(message?.delivery ?? 'inbox')
  const [audience, setAudience] = useState<MoonfinMessageAudience>(message?.audience ?? 'all')
  const [chosen, setChosen] = useState<string[]>(message?.users ?? [])
  const [start, setStart] = useState(localInput(message?.start ?? ''))
  const [end, setEnd] = useState(localInput(message?.end ?? ''))
  const [linkLabel, setLinkLabel] = useState(message?.linkLabel ?? '')
  const [linkUrl, setLinkUrl] = useState(message?.linkUrl ?? '')
  const [tried, setTried] = useState(false)
  const mutation = useMutation({
    mutationFn: (draft: MoonfinMessageDraft) =>
      message === null ? createMoonfinMessage(draft) : updateMoonfinMessage(message.id, draft),
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.moonfinMessages })
      toast(text.saved(messageName(saved, text.untitled)))
      onClose()
    },
  })

  const noText = title.trim() === '' && body.trim() === ''
  const missingUser = audience === 'users' && chosen.length === 0
  // datetime-local values of one time zone compare as text.
  const datesOutOfOrder = start !== '' && end !== '' && end <= start
  const label = linkLabel.trim()
  const url = linkUrl.trim()
  const badLink = url !== '' && !webLink(url)
  const halfLink = !badLink && (label === '') !== (url === '')
  const invalid = noText || missingUser || datesOutOfOrder || badLink || halfLink

  function submit(event: FormEvent) {
    event.preventDefault()
    setTried(true)
    if (invalid) return
    mutation.mutate({
      title: title.trim(),
      body: body.trim(),
      color,
      delivery,
      audience,
      users: audience === 'users' ? chosen : [],
      start: isoTime(start),
      end: isoTime(end),
      linkLabel: label,
      linkUrl: url,
    })
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={message === null ? text.addTitle : text.editTitle(messageName(message, text.untitled))}
      width={640}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            icon={message === null ? PaperPlaneTiltIcon : undefined}
            loading={mutation.isPending}
          >
            {message === null
              ? mutation.isPending
                ? text.creating
                : text.create
              : mutation.isPending
                ? t.common.saving
                : t.common.save}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        onSubmit={submit}
        noValidate
        className="space-y-5 p-5"
        data-testid="moonfin-message-form"
      >
        <Field label={text.titleLabel}>
          <TextInput
            value={title}
            onValue={(value) => {
              mutation.reset()
              setTitle(value)
            }}
            maxLength={lengths.title}
            autoComplete="off"
          />
        </Field>
        <Field
          label={text.body}
          help={text.bodyHelp(body.trim().length, lengths.body)}
          error={tried && noText ? text.noText : undefined}
        >
          <Textarea
            value={body}
            rows={6}
            maxLength={lengths.body}
            onValue={(value) => {
              mutation.reset()
              setBody(value)
            }}
          />
        </Field>
        <MessagePreview
          title={title.trim()}
          body={body.trim()}
          color={color}
          linkLabel={label}
          linkUrl={url}
        />
        <div>
          <p id={`${formId}-color`} className="mb-2 text-control font-medium text-ink">
            {text.color}
          </p>
          <div
            role="group"
            aria-labelledby={`${formId}-color`}
            className="flex items-center gap-1.5"
          >
            {moonfinMessageColors.map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={color === value}
                aria-label={text.colors[value]}
                title={text.colors[value]}
                onClick={() => setColor(value)}
                className="grid size-9.5 cursor-pointer place-items-center rounded-full bg-s2 focus-lit aria-pressed:bg-accent max-sm:size-11"
              >
                <span className={`size-5 rounded-full ${tones[value].dot}`} />
              </button>
            ))}
            <span className="ml-1.5 text-small text-ink-2">{text.colors[color]}</span>
          </div>
        </div>
        <Field label={text.delivery} help={text.deliveryHelp[delivery]}>
          <Segmented
            label={text.delivery}
            value={delivery}
            options={(['inbox', 'popup'] as const).map((value) => ({
              value,
              label: text.deliveries[value],
            }))}
            onChange={setDelivery}
          />
        </Field>
        <Field label={text.audience} error={tried && missingUser ? text.noUser : undefined}>
          <Segmented
            label={text.audience}
            value={audience}
            options={audiences.map((value) => ({ value, label: text.audiences[value] }))}
            onChange={(value) => {
              mutation.reset()
              setAudience(value)
            }}
          />
        </Field>
        {audience === 'users' && <UserPicker users={users} chosen={chosen} onChosen={setChosen} />}
        <fieldset className="space-y-3">
          <legend className="text-control font-medium text-ink">{text.dates}</legend>
          <p className="max-w-[60ch] text-small text-ink-3">{text.datesHelp}</p>
          <div className="flex flex-wrap gap-4">
            <Field label={text.start} className="w-60">
              <TextInput
                type="datetime-local"
                value={start}
                onValue={(value) => {
                  mutation.reset()
                  setStart(value)
                }}
              />
            </Field>
            <Field
              label={text.end}
              error={datesOutOfOrder ? text.dateOrder : undefined}
              className="w-60"
            >
              <TextInput
                type="datetime-local"
                value={end}
                onValue={(value) => {
                  mutation.reset()
                  setEnd(value)
                }}
              />
            </Field>
          </div>
        </fieldset>
        <fieldset className="space-y-3">
          <legend className="text-control font-medium text-ink">{text.link}</legend>
          <p className="max-w-[60ch] text-small text-ink-3">{text.linkHelp}</p>
          <div className="flex flex-wrap gap-4">
            <Field
              label={text.linkLabel}
              error={tried && halfLink && label === '' ? text.linkHalf : undefined}
              className="w-48"
            >
              <TextInput
                value={linkLabel}
                onValue={(value) => {
                  mutation.reset()
                  setLinkLabel(value)
                }}
                maxLength={lengths.linkLabel}
                autoComplete="off"
              />
            </Field>
            <Field
              label={text.linkUrl}
              error={
                badLink
                  ? text.linkInvalid
                  : tried && halfLink && url === ''
                    ? text.linkHalf
                    : undefined
              }
              className="min-w-0 flex-1"
            >
              <TextInput
                type="url"
                value={linkUrl}
                onValue={(value) => {
                  mutation.reset()
                  setLinkUrl(value)
                }}
                maxLength={lengths.linkUrl}
                placeholder="https://"
                autoComplete="off"
              />
            </Field>
          </div>
        </fieldset>
        {mutation.isError && <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>}
      </form>
    </Modal>
  )
}

/** The message as Moonfin's apps show it in their messages window, drawn as it is written. */
function MessagePreview({
  title,
  body,
  color,
  linkLabel,
  linkUrl,
}: {
  title: string
  body: string
  color: MoonfinMessageColor
  linkLabel: string
  linkUrl: string
}) {
  const { t } = useI18n()
  const text = t.settings.moonfinMessages
  const id = useId()
  const empty = title === '' && body === ''
  return (
    <section aria-labelledby={id} className="space-y-2">
      <h4 id={id} className="text-control font-medium text-ink">
        {text.preview}
      </h4>
      <div
        className={`flex gap-3 rounded-row border-2 bg-s2 p-4 text-body text-ink ${tones[color].edge}`}
        data-testid="moonfin-message-preview"
      >
        <InfoIcon size={22} className={`mt-0.5 shrink-0 ${tones[color].ink}`} aria-hidden="true" />
        {empty ? (
          <p className="text-ink-3">{text.previewEmpty}</p>
        ) : (
          <div className="min-w-0 flex-1 space-y-2">
            {title !== '' && <p className="font-semibold text-ink">{title}</p>}
            {body !== '' && (
              <div className="text-ink-2">
                <MessageMarkdown source={body} />
              </div>
            )}
            {linkLabel !== '' && webLink(linkUrl) && (
              <span
                className={`inline-flex items-center rounded-control border px-3 py-1 text-control font-medium ${tones[color].edge} ${tones[color].ink}`}
              >
                {linkLabel}
              </span>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

/**
 * The users a message is shown to: a search over their names, then a checklist of those found,
 * the chosen ones first, with how many are chosen.
 */
function UserPicker({
  users,
  chosen,
  onChosen,
}: {
  users: User[]
  chosen: string[]
  onChosen: (chosen: string[]) => void
}) {
  const { t, language } = useI18n()
  const text = t.settings.moonfinMessages
  const [search, setSearch] = useState('')
  const needle = search.trim().toLocaleLowerCase(language)
  const found = users
    .filter((user) => needle === '' || user.name.toLocaleLowerCase(language).includes(needle))
    .sort(
      (a, b) =>
        Number(chosen.includes(b.id)) - Number(chosen.includes(a.id)) ||
        a.name.localeCompare(b.name, language),
    )
  return (
    <fieldset className="space-y-3" data-testid="moonfin-message-users">
      <legend className="sr-only">{text.users}</legend>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <TextInput
          icon={MagnifyingGlassIcon}
          value={search}
          onValue={setSearch}
          placeholder={text.findUser}
          aria-label={text.findUser}
          className="w-64 max-w-full"
          autoComplete="off"
        />
        <p className="text-small text-ink-2" aria-live="polite">
          {text.chosenCount(chosen.length)}
        </p>
      </div>
      {found.length === 0 ? (
        <p className="text-small text-ink-3">{text.noUserFound}</p>
      ) : (
        <div className="grid max-h-64 grid-cols-2 gap-x-4 overflow-y-auto max-sm:grid-cols-1">
          {found.map((user) => (
            <Checkbox
              key={user.id}
              label={user.name}
              help={user.isAdministrator ? text.administrator : undefined}
              checked={chosen.includes(user.id)}
              onChange={(checked) =>
                onChosen(checked ? [...chosen, user.id] : chosen.filter((id) => id !== user.id))
              }
            />
          ))}
        </div>
      )}
    </fieldset>
  )
}
