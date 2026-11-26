import { LockKeyIcon, LockKeyOpenIcon, PasswordIcon } from '@phosphor-icons/react'
import { useMutation, useQuery, type UseQueryResult } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  fetchOwnMoonfinPin,
  fetchUserMoonfinPin,
  queryClient,
  queryKeys,
  removeOwnMoonfinPin,
  removeUserMoonfinPin,
  saveOwnMoonfinPin,
  saveUserMoonfinPin,
  type MoonfinPin,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  Field,
  InlineError,
  Modal,
  Notice,
  Skeleton,
  TextInput,
  useToast,
} from '@/ui'

/** A PIN as Moonfin's apps take it: four digits. */
const pinShape = /^[0-9]{4}$/

/**
 * How the PIN of an account changes: `own` for the signed-in user, who gives the current one, or
 * an administrator's for the user `name`, who needs none.
 */
type PinChanges = {
  own: boolean
  name: string
  save: (pin: string, currentPin: string) => Promise<MoonfinPin>
  remove: (currentPin: string) => Promise<MoonfinPin>
}

/** The errors of a PIN typed in a dialog, shown under its field rather than below the form. */
const fieldErrors = ['invalid_moonfin_pin', 'wrong_moonfin_pin']

/** After a change or a refusal: every PIN shown again, a refused guess counting. */
function refreshPins() {
  void queryClient.invalidateQueries({ queryKey: queryKeys.moonfinPins })
}

/** A field for a PIN: four digits, masked, with the eye that shows them. */
function PinInput({
  value,
  onValue,
  autoFocus,
}: {
  value: string
  onValue: (value: string) => void
  autoFocus?: boolean
}) {
  return (
    <TextInput
      type="password"
      inputMode="numeric"
      autoComplete="off"
      maxLength={4}
      revealable
      mono
      autoFocus={autoFocus}
      value={value}
      onValue={(next) => onValue(next.replace(/[^0-9]/g, ''))}
      className="max-w-[12rem]"
    />
  )
}

/**
 * Sets or changes the PIN: the current one first for the user's own when they have one, then the
 * new one.
 */
function SetPinDialog({
  changes,
  hasPin,
  onClose,
}: {
  changes: PinChanges
  hasPin: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.account.moonfin.pin
  const toast = useToast()
  const asksCurrent = changes.own && hasPin
  const [current, setCurrent] = useState('')
  const [pin, setPin] = useState('')
  const mutation = useMutation({
    mutationFn: () => changes.save(pin, current),
    onSuccess: () => {
      refreshPins()
      toast(text.saved)
      onClose()
    },
    onError: refreshPins,
  })
  const failure = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const code = mutation.error instanceof ApiError ? mutation.error.code : ''
  const formId = 'moonfin-pin-set'
  return (
    <Modal
      open
      onClose={onClose}
      width={480}
      title={hasPin ? text.changeTitle : text.addTitle}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            type="submit"
            form={formId}
            loading={mutation.isPending}
            disabled={!pinShape.test(pin) || (asksCurrent && !pinShape.test(current))}
          >
            {text.save}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="space-y-4 p-5"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          mutation.mutate()
        }}
      >
        <p className="text-control text-ink-2">
          {changes.own ? text.ownText : text.userText(changes.name)}
        </p>
        {asksCurrent && (
          <Field
            label={text.current}
            help={text.forgot}
            error={code === 'wrong_moonfin_pin' ? failure : undefined}
          >
            <PinInput
              value={current}
              autoFocus
              onValue={(next) => {
                mutation.reset()
                setCurrent(next)
              }}
            />
          </Field>
        )}
        <Field
          label={text.new}
          help={text.digits}
          error={code === 'invalid_moonfin_pin' ? failure : undefined}
        >
          <PinInput
            value={pin}
            autoFocus={!asksCurrent}
            onValue={(next) => {
              mutation.reset()
              setPin(next)
            }}
          />
        </Field>
        {failure !== undefined && !fieldErrors.includes(code) && (
          <Notice tone="danger" live>
            {failure}
          </Notice>
        )}
      </form>
    </Modal>
  )
}

/** Removes the user's own PIN once they gave it. */
function RemoveOwnPinDialog({ changes, onClose }: { changes: PinChanges; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.account.moonfin.pin
  const toast = useToast()
  const [current, setCurrent] = useState('')
  const mutation = useMutation({
    mutationFn: () => changes.remove(current),
    onSuccess: () => {
      refreshPins()
      toast(text.removed)
      onClose()
    },
    onError: refreshPins,
  })
  const failure = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const code = mutation.error instanceof ApiError ? mutation.error.code : ''
  const formId = 'moonfin-pin-remove'
  return (
    <Modal
      open
      onClose={onClose}
      width={480}
      title={text.removeTitle}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="danger"
            className="bg-danger/14"
            loading={mutation.isPending}
            disabled={!pinShape.test(current)}
          >
            {text.remove}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="space-y-4 p-5"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          mutation.mutate()
        }}
      >
        <p className="text-control text-ink-2">{text.removeText}</p>
        <Field
          label={text.current}
          help={text.forgot}
          error={fieldErrors.includes(code) ? failure : undefined}
        >
          <PinInput
            value={current}
            autoFocus
            onValue={(next) => {
              mutation.reset()
              setCurrent(next)
            }}
          />
        </Field>
        {failure !== undefined && !fieldErrors.includes(code) && (
          <Notice tone="danger" live>
            {failure}
          </Notice>
        )}
      </form>
    </Modal>
  )
}

/** Removes a user's PIN, for an administrator, once confirmed. */
function RemoveUserPinDialog({ changes, onClose }: { changes: PinChanges; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.account.moonfin.pin
  const toast = useToast()
  const mutation = useMutation({
    mutationFn: () => changes.remove(''),
    onSuccess: () => {
      refreshPins()
      toast(text.removed)
      onClose()
    },
  })
  return (
    <ConfirmDialog
      open
      onClose={onClose}
      onConfirm={() => mutation.mutate()}
      title={text.userRemoveTitle(changes.name)}
      confirmLabel={text.remove}
      busy={mutation.isPending}
      error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
    >
      {text.userRemoveText}
    </ConfirmDialog>
  )
}

/**
 * The sign-in PIN of an account, as a row: whether it has one, the wrong guesses since the last
 * right one and the wait they set; with Set a PIN, or Change PIN and Remove PIN, each in a dialog.
 */
function MoonfinPinRow({ pin, changes }: { pin: MoonfinPin; changes: PinChanges }) {
  const { t, language } = useI18n()
  const text = t.account.moonfin.pin
  const [dialog, setDialog] = useState<'set' | 'remove' | null>(null)
  const close = () => setDialog(null)
  return (
    <>
      <div
        className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2 rounded-row bg-s1 px-4 py-3"
        data-testid="moonfin-pin"
      >
        <div className="min-w-0 flex-1 basis-60">
          <p className="text-h3 text-ink">{text.title}</p>
          <p className="mt-0.5 text-small text-ink-2">
            {pin.set ? (changes.own ? text.ownSet : text.userSet) : text.none}
          </p>
          {pin.set && pin.waitUntil !== null ? (
            <p className="mt-0.5 text-small text-warn">
              {text.waiting(
                new Intl.DateTimeFormat(language, { timeStyle: 'short' }).format(
                  new Date(pin.waitUntil),
                ),
              )}
            </p>
          ) : (
            pin.set &&
            pin.wrongGuesses > 0 && (
              <p className="mt-0.5 text-small text-ink-2">{text.wrong(pin.wrongGuesses)}</p>
            )
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          {pin.set ? (
            <>
              <Button variant="secondary" icon={PasswordIcon} onClick={() => setDialog('set')}>
                {text.change}
              </Button>
              <Button
                variant="secondary"
                icon={LockKeyOpenIcon}
                onClick={() => setDialog('remove')}
              >
                {text.remove}
              </Button>
            </>
          ) : (
            <Button variant="secondary" icon={LockKeyIcon} onClick={() => setDialog('set')}>
              {text.add}
            </Button>
          )}
        </div>
      </div>
      {dialog === 'set' && <SetPinDialog changes={changes} hasPin={pin.set} onClose={close} />}
      {dialog === 'remove' &&
        (changes.own ? (
          <RemoveOwnPinDialog changes={changes} onClose={close} />
        ) : (
          <RemoveUserPinDialog changes={changes} onClose={close} />
        ))}
    </>
  )
}

/** Loads a PIN for its row: a skeleton meanwhile, an error with Retry. */
function PinState({ query, changes }: { query: UseQueryResult<MoonfinPin>; changes: PinChanges }) {
  const { t } = useI18n()
  if (query.isPending) return <Skeleton className="h-15 rounded-row" />
  if (query.isError)
    return (
      <InlineError onRetry={() => void query.refetch()} retrying={query.isFetching}>
        {errorMessage(t, query.error)}
      </InlineError>
    )
  return <MoonfinPinRow pin={query.data} changes={changes} />
}

/** My account › Moonfin: the signed-in user's PIN, changed with the current one. */
export function OwnMoonfinPin() {
  const query = useQuery({
    queryKey: queryKeys.ownMoonfinPin,
    queryFn: ({ signal }) => fetchOwnMoonfinPin(signal),
  })
  return (
    <PinState
      query={query}
      changes={{ own: true, name: '', save: saveOwnMoonfinPin, remove: removeOwnMoonfinPin }}
    />
  )
}

/** A user's page › Moonfin: their PIN, set or removed by an administrator without the current one. */
export function UserMoonfinPin({ id, name }: { id: string; name: string }) {
  const query = useQuery({
    queryKey: queryKeys.userMoonfinPin(id),
    queryFn: ({ signal }) => fetchUserMoonfinPin(id, signal),
  })
  return (
    <PinState
      query={query}
      changes={{
        own: false,
        name,
        save: (pin) => saveUserMoonfinPin(id, pin),
        remove: () => removeUserMoonfinPin(id),
      }}
    />
  )
}
