import { createContext, use, useEffect, useId, useState, type ReactNode } from 'react'
import { icons } from '@/components/icons'
import { Badge, buttonDanger, buttonSecondary } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Lower case without accents, so that a search for "resume" finds "Résumé". */
export function searchable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}

/** The search typed, made searchable; empty shows every setting. */
export const SearchContext = createContext('')

/** One setting, hidden while the search matches none of its words. */
export function Setting({ text, children }: { text: readonly string[]; children: ReactNode }) {
  const query = use(SearchContext)
  const shown = query === '' || searchable(text.join(' ')).includes(query)
  return (
    <div data-setting="" hidden={!shown}>
      {children}
    </div>
  )
}

/** A titled group of settings within a section, hidden when the search leaves none of them. */
export function SettingsGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-6 pt-3 first:pt-0 [&:not(:has([data-setting]:not([hidden])))]:hidden">
      <h3 className="text-sm font-semibold tracking-wide text-fin-5 uppercase">{title}</h3>
      {children}
    </div>
  )
}

/** A labelled list to choose a value from; options may be disabled. */
export function SelectField<T extends string | number>({
  label,
  hint,
  value,
  options,
  onValue,
  disabled,
}: {
  label: string
  hint?: string
  value: T
  options: readonly { value: T; label: string; disabled?: boolean }[]
  onValue: (value: T) => void
  disabled?: boolean
}) {
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {label}
      </label>
      <select
        id={id}
        value={String(value)}
        disabled={disabled}
        onChange={(event) => {
          const chosen = options.find((option) => String(option.value) === event.target.value)
          if (chosen) onValue(chosen.value)
        }}
        aria-describedby={hint ? `${id}-hint` : undefined}
        className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white disabled:opacity-50"
      >
        {options.map((option) => (
          <option
            key={String(option.value)}
            value={String(option.value)}
            disabled={option.disabled}
          >
            {option.label}
          </option>
        ))}
      </select>
      {hint && (
        <p id={`${id}-hint`} className="mt-1 text-xs text-muted">
          {hint}
        </p>
      )}
    </div>
  )
}

/**
 * A whole number typed in a field where 0 is a valid value: an empty field is -1, which the server
 * refuses, rather than 0, which would turn the setting off without the user typing it.
 */
export function wholeNumber(value: string): number {
  return value.trim() === '' ? -1 : Math.trunc(Number(value))
}

/** The text of a field holding a whole number; -1 (see wholeNumber) shows as empty. */
export function wholeNumberField(value: number): number | '' {
  return value < 0 ? '' : value
}

/** How long a revealed secret stays shown before it hides again. */
const revealedFor = 60_000

/**
 * The eye that shows or hides a secret, a toggle button. While busy it stays enabled, so that it
 * keeps the focus, and ignores clicks.
 */
function EyeButton({
  shown,
  busy = false,
  controls,
  onClick,
}: {
  shown: boolean
  busy?: boolean
  controls: string
  onClick: () => void
}) {
  const { t } = useI18n()
  const label = shown ? t.settings.secret.hide : t.settings.secret.show
  const Icon = shown ? icons.eyeOff : icons.eye
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={shown}
      aria-controls={controls}
      aria-busy={busy || undefined}
      onClick={() => {
        if (!busy) onClick()
      }}
      className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-surface hover:text-white focus-visible:outline-2 focus-visible:outline-fin-4 aria-busy:opacity-50"
    >
      <Icon className="size-4" />
    </button>
  )
}

/**
 * A write-only secret: the server only says whether one is saved. `value` is undefined to keep the
 * saved one, a new secret to replace it, or "" to remove it on save. What is typed is masked, with an
 * eye to show it. A saved secret shows as dots; with `reveal`, its eye fetches it from the server and
 * shows it until clicked again, for a minute at most, or until the field leaves the page.
 * `replaceable` offers to type a new one, `removable` to remove it, and `showStatus` tells whether one
 * is saved.
 */
export function SecretField({
  label,
  hint,
  saved,
  value,
  onValue,
  error,
  reveal,
  replaceable = true,
  removable = true,
  showStatus = true,
}: {
  label: string
  hint: string
  saved: boolean
  value: string | undefined
  onValue?: (value: string | undefined) => void
  error?: string
  reveal?: () => Promise<string>
  replaceable?: boolean
  removable?: boolean
  showStatus?: boolean
}) {
  const { t } = useI18n()
  const text = t.settings.secret
  const id = useId()
  const removing = value === ''
  const [typedShown, setTypedShown] = useState(false)
  const [revealed, setRevealed] = useState<string | null>(null)
  const [revealing, setRevealing] = useState(false)
  const [revealError, setRevealError] = useState<unknown>(null)

  // What was revealed hides once the saved secret may have changed: saved, replaced or removed.
  const [revealedFrom, setRevealedFrom] = useState({ saved, value })
  if (revealedFrom.saved !== saved || revealedFrom.value !== value) {
    setRevealedFrom({ saved, value })
    setRevealed(null)
  }
  // And after a minute; leaving the page drops it with the field.
  useEffect(() => {
    if (revealed === null) return
    const timer = setTimeout(() => setRevealed(null), revealedFor)
    return () => clearTimeout(timer)
  }, [revealed])

  async function toggleRevealed() {
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

  const errors = [error, revealError === null ? undefined : errorMessage(t, revealError)].filter(
    (message) => message !== undefined,
  )
  const describedBy = [`${id}-hint`, ...errors.map((_, index) => `${id}-error-${index}`)].join(' ')
  return (
    <div>
      <div className="flex flex-wrap items-center gap-2">
        <label
          htmlFor={replaceable ? id : `${id}-saved`}
          className="text-sm font-medium text-zinc-200"
        >
          {label}
        </label>
        {showStatus && (
          <Badge tone={removing ? 'warning' : saved ? 'ok' : 'muted'}>
            {removing ? text.removing : saved ? text.saved : text.notSet}
          </Badge>
        )}
      </div>
      {saved && !removing && (
        <div className="mt-1.5 flex min-h-11 items-center gap-2 rounded-lg border border-line bg-ink/60 py-1.5 pr-1.5 pl-3">
          <output
            id={`${id}-saved`}
            aria-label={revealed === null ? text.savedHidden : undefined}
            aria-live="polite"
            aria-describedby={describedBy}
            className={`min-w-0 flex-1 text-sm break-all ${revealed === null ? 'tracking-widest text-muted' : 'font-mono text-zinc-100 select-all'}`}
          >
            {revealed ?? '••••••••••••••••'}
          </output>
          {reveal !== undefined && (
            <EyeButton
              shown={revealed !== null}
              busy={revealing}
              controls={`${id}-saved`}
              onClick={() => void toggleRevealed()}
            />
          )}
        </div>
      )}
      {(replaceable || removing || (saved && removable)) && (
        <div className="mt-1.5 flex flex-col gap-2 sm:flex-row">
          {replaceable && (
            <div className="relative min-w-0 flex-1">
              <input
                id={id}
                type={typedShown ? 'text' : 'password'}
                autoComplete="off"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                value={value ?? ''}
                disabled={removing}
                placeholder={saved ? text.replacePlaceholder : text.pastePlaceholder}
                onChange={(event) =>
                  onValue?.(event.target.value === '' ? undefined : event.target.value)
                }
                aria-invalid={error ? true : undefined}
                aria-describedby={describedBy}
                className="block w-full min-w-0 rounded-lg border border-line bg-ink py-2 pr-11 pl-3 text-white placeholder:text-zinc-500 disabled:opacity-50 aria-invalid:border-rose-400"
              />
              {!removing && (
                <div className="absolute inset-y-0 right-1.5 flex items-center">
                  <EyeButton
                    shown={typedShown}
                    controls={id}
                    onClick={() => setTypedShown(!typedShown)}
                  />
                </div>
              )}
            </div>
          )}
          {removing ? (
            <button
              type="button"
              className={`${buttonSecondary} shrink-0 whitespace-nowrap`}
              onClick={() => onValue?.(undefined)}
            >
              {text.keep}
            </button>
          ) : (
            saved &&
            removable && (
              <button
                type="button"
                className={`${buttonDanger} shrink-0 whitespace-nowrap`}
                onClick={() => onValue?.('')}
              >
                {text.remove}
              </button>
            )
          )}
        </div>
      )}
      <p id={`${id}-hint`} className="mt-1 text-xs text-muted">
        {removing ? text.removeHint : hint}
      </p>
      {errors.map((message, index) => (
        <p
          key={message}
          id={`${id}-error-${index}`}
          role="alert"
          className="mt-1 text-sm text-rose-300"
        >
          {message}
        </p>
      ))}
    </div>
  )
}
