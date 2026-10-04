import { useEffect, useId, useRef, type InputHTMLAttributes, type ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { dateTime, relativeTime } from '@/format'

export const buttonPrimary =
  'inline-flex min-h-10 items-center justify-center gap-2 rounded-lg bg-fin-2 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-fin-1 active:translate-y-px disabled:cursor-progress disabled:opacity-70'
export const buttonSecondary =
  'inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border border-line bg-ink px-4 py-2 text-sm font-medium text-white transition-colors hover:border-fin-4 active:translate-y-px disabled:cursor-progress disabled:opacity-70'
export const buttonDanger =
  'inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border border-rose-400/50 bg-rose-500/10 px-4 py-2 text-sm font-medium text-rose-200 transition-colors hover:bg-rose-500/20 active:translate-y-px disabled:cursor-progress disabled:opacity-70'

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description: string
  /** Controls shown beside the title on wide screens, below it on phones. */
  actions?: ReactNode
}) {
  return (
    <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight text-white sm:text-3xl">{title}</h1>
        <p className="mt-2 max-w-prose text-muted">{description}</p>
      </div>
      {actions !== undefined && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
  )
}

export function Card({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-line bg-surface p-5">
      {title !== undefined && <h2 className="mb-4 text-lg font-semibold text-white">{title}</h2>}
      {children}
    </section>
  )
}

type TextFieldProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'id' | 'onChange'> & {
  label: string
  hint?: string
  error?: string
  onValue: (value: string) => void
}

export function TextField({ label, hint, error, onValue, className, ...input }: TextFieldProps) {
  const id = useId()
  const hintId = `${id}-hint`
  const errorId = `${id}-error`
  const describedBy = [hint && hintId, error && errorId].filter(Boolean).join(' ') || undefined
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {label}
      </label>
      <input
        id={id}
        {...input}
        onChange={(event) => onValue(event.target.value)}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={`mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white placeholder:text-zinc-500 aria-invalid:border-rose-400 ${className ?? ''}`}
      />
      {hint && (
        <p id={hintId} className="mt-1 text-xs text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="mt-1 text-sm text-rose-300">
          {error}
        </p>
      )}
    </div>
  )
}

export function Checkbox({
  label,
  help,
  checked,
  onChange,
}: {
  label: string
  help?: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  const id = useId()
  return (
    <div className="flex items-start gap-3">
      <input
        id={id}
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        aria-describedby={help ? `${id}-help` : undefined}
        className="mt-0.5 size-5 shrink-0 accent-fin-3"
      />
      <div>
        <label htmlFor={id} className="text-sm font-medium text-zinc-100">
          {label}
        </label>
        {help && (
          <p id={`${id}-help`} className="text-xs text-muted">
            {help}
          </p>
        )}
      </div>
    </div>
  )
}

export function Notice({ kind, children }: { kind: 'error' | 'success'; children: ReactNode }) {
  return kind === 'error' ? (
    <div
      role="alert"
      className="flex gap-2 rounded-lg border border-rose-400/40 bg-rose-400/5 p-3 text-sm text-rose-200"
    >
      <span aria-hidden="true">⚠</span>
      <p>{children}</p>
    </div>
  ) : (
    <div
      role="status"
      className="flex gap-2 rounded-lg border border-emerald-400/40 bg-emerald-400/5 p-3 text-sm text-emerald-200"
    >
      <span aria-hidden="true">✓</span>
      <p>{children}</p>
    </div>
  )
}

export function Badge({
  tone,
  children,
}: {
  tone: 'fin' | 'muted' | 'danger' | 'ok' | 'warning'
  children: ReactNode
}) {
  const tones = {
    fin: 'border-fin-4/50 text-fin-5',
    muted: 'border-line text-muted',
    danger: 'border-rose-400/50 text-rose-300',
    ok: 'border-emerald-400/40 text-emerald-300',
    warning: 'border-amber-400/50 text-amber-200',
  }
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs font-medium whitespace-nowrap ${tones[tone]}`}
    >
      {children}
    </span>
  )
}

export function RelativeTime({ iso }: { iso: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}

export function Loading() {
  const { t } = useI18n()
  return (
    <p role="status" className="text-muted">
      {t.common.loading}
    </p>
  )
}

const moveButton =
  'inline-flex size-10 items-center justify-center rounded-lg border border-line bg-ink text-base text-white transition-colors hover:border-fin-4 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:border-line'

/**
 * Up and down buttons for one item of an ordered list. After a move, focus follows the item (or
 * jumps to its other button once it reaches an end) so it can be moved again from the keyboard.
 */
export function MoveButtons({
  name,
  index,
  count,
  onMove,
}: {
  name: string
  index: number
  count: number
  onMove: (to: number) => void
}) {
  const { t } = useI18n()
  const up = useRef<HTMLButtonElement>(null)
  const down = useRef<HTMLButtonElement>(null)
  const moved = useRef<'up' | 'down' | null>(null)
  const first = index === 0
  const last = index === count - 1

  useEffect(() => {
    const direction = moved.current
    if (direction === null) return
    moved.current = null
    const target = direction === 'up' ? (first ? down : up) : last ? up : down
    target.current?.focus()
  }, [index, first, last])

  return (
    <div className="flex gap-1">
      <button
        ref={up}
        type="button"
        className={moveButton}
        disabled={first}
        aria-label={t.common.moveUp(name)}
        title={t.common.moveUp(name)}
        onClick={() => {
          moved.current = 'up'
          onMove(index - 1)
        }}
      >
        <span aria-hidden="true">↑</span>
      </button>
      <button
        ref={down}
        type="button"
        className={moveButton}
        disabled={last}
        aria-label={t.common.moveDown(name)}
        title={t.common.moveDown(name)}
        onClick={() => {
          moved.current = 'down'
          onMove(index + 1)
        }}
      >
        <span aria-hidden="true">↓</span>
      </button>
    </div>
  )
}

/** A button that asks for confirmation in a modal native dialog before acting. */
export function ConfirmButton({
  label,
  busyLabel,
  message,
  busy,
  onConfirm,
}: {
  label: string
  busyLabel: string
  message: string
  busy: boolean
  onConfirm: () => void
}) {
  const { t } = useI18n()
  const dialog = useRef<HTMLDialogElement>(null)
  const messageId = useId()

  return (
    <>
      <button
        type="button"
        className={buttonDanger}
        disabled={busy}
        onClick={() => dialog.current?.showModal()}
      >
        {busy ? busyLabel : label}
      </button>
      <dialog
        ref={dialog}
        aria-describedby={messageId}
        aria-label={label}
        className="m-auto w-[min(28rem,calc(100vw-2rem))] rounded-2xl border border-line bg-surface p-5 text-zinc-100 backdrop:bg-black/70"
      >
        <p id={messageId}>{message}</p>
        <div className="mt-5 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            autoFocus
            className={buttonSecondary}
            onClick={() => dialog.current?.close()}
          >
            {t.common.cancel}
          </button>
          <button
            type="button"
            className={buttonDanger}
            onClick={() => {
              dialog.current?.close()
              onConfirm()
            }}
          >
            {t.common.confirm}
          </button>
        </div>
      </dialog>
    </>
  )
}
