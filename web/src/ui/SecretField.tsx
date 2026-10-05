import { CheckCircleIcon, EyeIcon, EyeSlashIcon, InfoIcon } from '@phosphor-icons/react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, IconButton } from './Button'
import { cx } from './cx'
import { describedBy, FieldError } from './Field'
import { fieldBox } from './Input'

/** How long a revealed secret stays shown before it hides again. */
const revealedFor = 60_000

export type SecretFieldProps = {
  /** The visible label: "PublicMetaDB key". */
  label: string
  /** What the key is for, under the label. */
  help?: ReactNode
  /** Whether the server holds a saved secret. The server never sends it with the settings. */
  saved: boolean
  /**
   * The pending change: `undefined` keeps the saved secret, a string replaces it, and `""`
   * removes it on save.
   */
  value: string | undefined
  /** Called with the pending change (see `value`). Omit it for a saved secret shown read only. */
  onValue?: (value: string | undefined) => void
  /**
   * Fetches the saved secret from the server (the `reveal…` functions of `api.ts`). With it, the
   * eye on the saved dots shows the secret, for a minute at most or until clicked again.
   */
  reveal?: () => Promise<string>
  /** Offers to type a new secret over a saved one. Defaults to true. */
  replaceable?: boolean
  /** Offers to remove a saved secret. Defaults to true. */
  removable?: boolean
  /**
   * The line under the field. By default it says whether a secret is saved; pass your own (such
   * as "Saved and checked on 2 October"), or `null` for none.
   */
  status?: ReactNode | null
  /** The error from the server, shown under the field. */
  error?: string
  /** Placeholder of the empty field. */
  placeholder?: string
  /** Buttons after the field, such as "Check and save". */
  actions?: ReactNode
}

/**
 * A write-only secret: an API key, a client secret, a password the server keeps.
 * - Typing is masked, with an eye to show what is typed (`aria-pressed`). Autocomplete is off.
 * - A saved secret shows as dots. With `reveal`, its eye fetches it and shows it until clicked
 *   again, for 60 seconds at most, or until the secret changes or the field leaves the page.
 * - "Replace" opens an empty field for the new secret; "Keep the saved one" closes it.
 * - "Remove" marks it for removal on save; "Keep it" undoes that.
 */
export function SecretField({
  label,
  help,
  saved,
  value,
  onValue,
  reveal,
  replaceable = true,
  removable = true,
  status,
  error,
  placeholder,
  actions,
}: SecretFieldProps) {
  const { t } = useI18n()
  const text = t.ui.secret
  const id = useId()
  const input = useRef<HTMLInputElement>(null)
  const removing = saved && value === ''
  const [replacing, setReplacing] = useState(false)
  const typing = !saved || replacing || (value !== undefined && value !== '')
  const [typedShown, setTypedShown] = useState(false)
  const [revealed, setRevealed] = useState<string | null>(null)
  const [revealing, setRevealing] = useState(false)
  const [revealError, setRevealError] = useState<unknown>(null)

  // What was revealed hides once the saved secret may have changed: saved, replaced or removed.
  const [revealedFrom, setRevealedFrom] = useState({ saved, value })
  if (revealedFrom.saved !== saved || revealedFrom.value !== value) {
    setRevealedFrom({ saved, value })
    setRevealed(null)
    // Once saved, the field goes back to the dots.
    if (revealedFrom.saved !== saved) setReplacing(false)
  }
  // And after a minute; leaving the page drops it with the field.
  useEffect(() => {
    if (revealed === null) return
    const timer = setTimeout(() => setRevealed(null), revealedFor)
    return () => clearTimeout(timer)
  }, [revealed])
  // A replacement the form saved or discarded (the value goes back to undefined while nobody is
  // typing in the field) brings back the saved dots.
  const pending = useRef(value)
  useEffect(() => {
    const dropped = pending.current !== undefined && pending.current !== '' && value === undefined
    pending.current = value
    if (dropped && document.activeElement !== input.current) setReplacing(false)
  }, [value])

  async function toggleRevealed() {
    if (revealing) return
    if (revealed !== null || reveal === undefined) {
      setRevealed(null)
      return
    }
    setRevealing(true)
    setRevealError(null)
    try {
      setRevealed(await reveal())
    } catch (failure) {
      setRevealError(failure)
    } finally {
      setRevealing(false)
    }
  }

  function startReplacing() {
    setReplacing(true)
    setRevealed(null)
    // The field appears with this render; focus it once drawn.
    requestAnimationFrame(() => input.current?.focus())
  }

  const helpId = help ? `${id}-help` : undefined
  const statusId = `${id}-status`
  const errors = [error, revealError === null ? undefined : errorMessage(t, revealError)].filter(
    (message): message is string => message !== undefined,
  )
  const errorIds = errors.map((_, index) => `${id}-error-${index}`)
  const described = describedBy(helpId, statusId, ...errorIds)

  const statusLine =
    status === null ? null : removing ? (
      <StatusLine id={statusId} tone="warn">
        {text.removing}
      </StatusLine>
    ) : status !== undefined ? (
      <StatusLine id={statusId}>{status}</StatusLine>
    ) : (
      <StatusLine id={statusId} tone={saved ? 'ok' : 'muted'}>
        {saved ? text.saved : text.notSet}
      </StatusLine>
    )

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <label
        htmlFor={typing && !removing ? id : `${id}-saved`}
        className="text-control font-medium text-ink"
      >
        {label}
      </label>
      {help && (
        <p id={helpId} className="-mt-1 max-w-[60ch] text-small text-ink-3">
          {help}
        </p>
      )}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        {removing || !typing ? (
          <div className={cx(fieldBox, 'h-10 flex-1', removing && 'opacity-50')}>
            <output
              id={`${id}-saved`}
              tabIndex={-1}
              aria-label={revealed === null ? text.savedHidden : undefined}
              aria-live="polite"
              aria-describedby={described}
              className={cx(
                'min-w-0 flex-1 truncate px-3 font-mono',
                revealed === null
                  ? 'text-small tracking-[0.3em] text-ink-2'
                  : 'text-small text-ink select-all',
              )}
            >
              {revealed ?? '••••••••••••••••••••'}
            </output>
            {reveal !== undefined && !removing && (
              <IconButton
                label={revealed === null ? text.show : text.hide}
                icon={revealed === null ? EyeIcon : EyeSlashIcon}
                pressed={revealed !== null}
                loading={revealing}
                aria-controls={`${id}-saved`}
                onClick={() => void toggleRevealed()}
                className="mr-[3px] size-8 rounded-md"
              />
            )}
          </div>
        ) : (
          <div className={cx(fieldBox, 'h-10 flex-1')}>
            <input
              ref={input}
              id={id}
              type={typedShown ? 'text' : 'password'}
              autoComplete="off"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              data-1p-ignore
              data-lpignore="true"
              value={value ?? ''}
              placeholder={placeholder ?? (saved ? text.replacePlaceholder : text.pastePlaceholder)}
              onChange={(event) =>
                onValue?.(event.target.value === '' ? undefined : event.target.value)
              }
              aria-invalid={error ? true : undefined}
              aria-describedby={described}
              className="h-full min-w-0 flex-1 bg-transparent px-3 font-mono text-small text-ink outline-none placeholder:font-sans placeholder:text-body placeholder:text-ink-3 focus-visible:outline-none"
            />
            <IconButton
              label={typedShown ? text.hide : text.show}
              icon={typedShown ? EyeSlashIcon : EyeIcon}
              pressed={typedShown}
              aria-controls={id}
              onClick={() => setTypedShown(!typedShown)}
              className="mr-[3px] size-8 rounded-md"
            />
          </div>
        )}
        <div className="flex shrink-0 items-center gap-2">
          {removing ? (
            <Button onClick={() => onValue?.(undefined)}>{text.keep}</Button>
          ) : saved && typing ? (
            <Button
              variant="ghost"
              onClick={() => {
                setReplacing(false)
                onValue?.(undefined)
              }}
            >
              {text.cancelReplace}
            </Button>
          ) : (
            saved && (
              <>
                {replaceable && <Button onClick={startReplacing}>{text.replace}</Button>}
                {removable && (
                  <Button variant="danger" onClick={() => onValue?.('')}>
                    {text.remove}
                  </Button>
                )}
              </>
            )
          )}
          {actions}
        </div>
      </div>
      {statusLine}
      {removing && <p className="text-small text-ink-3">{text.removeHint}</p>}
      {errors.map((message, index) => (
        <FieldError key={message} id={errorIds[index]}>
          {message}
        </FieldError>
      ))}
    </div>
  )
}

function StatusLine({
  id,
  tone,
  children,
}: {
  id: string
  tone?: 'ok' | 'warn' | 'muted'
  children: ReactNode
}) {
  const Glyph = tone === 'ok' ? CheckCircleIcon : InfoIcon
  return (
    <p id={id} className="flex items-center gap-2 text-small text-ink-3">
      {tone && (
        <Glyph
          size={16}
          aria-hidden="true"
          className={cx('shrink-0', tone === 'ok' ? 'text-ok' : tone === 'warn' ? 'text-warn' : '')}
        />
      )}
      <span>{children}</span>
    </p>
  )
}
