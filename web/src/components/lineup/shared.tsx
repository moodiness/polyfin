import { useEffect, useId, useState, type ReactNode } from 'react'
import { useI18n } from '@/i18n'

/** A value that follows `value` once it stopped changing for `delay` milliseconds. */
export function useDebounced<T>(value: T, delay = 300): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return settled
}

export const fieldClass =
  'mt-1.5 block w-full rounded-lg border border-line bg-bg px-3 py-2 text-white placeholder:text-zinc-500'

export const smallField =
  'block min-h-9 w-full rounded-lg border border-line bg-bg px-3 py-1.5 text-sm text-white placeholder:text-zinc-500'

/** A set of exclusive choices, shown as a row of pressed buttons, labelled as one group. */
export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
  hideLabel = false,
}: {
  label: string
  value: T
  options: { value: T; label: string }[]
  onChange: (value: T) => void
  hideLabel?: boolean
}) {
  const labelId = useId()
  return (
    <div>
      <p
        id={labelId}
        className={hideLabel ? 'sr-only' : 'mb-1.5 block text-xs font-medium text-muted'}
      >
        {label}
      </p>
      <div
        role="radiogroup"
        aria-labelledby={labelId}
        className="inline-flex flex-wrap gap-1 rounded-lg border border-line bg-bg p-0.5"
      >
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={value === option.value}
            onClick={() => onChange(option.value)}
            className={`min-h-8 rounded-md px-3 py-1 text-sm font-medium transition-colors ${
              value === option.value ? 'bg-fin-2 text-white' : 'text-muted hover:text-white'
            }`}
          >
            {option.label}
          </button>
        ))}
      </div>
    </div>
  )
}

/** A labelled choice between options, as a native select. */
export function SelectField<T extends string>({
  label,
  hint,
  value,
  options,
  onChange,
}: {
  label: string
  hint?: string
  value: T
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}) {
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {label}
      </label>
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value as T)}
        aria-describedby={hint ? `${id}-hint` : undefined}
        className={fieldClass}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
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

/** Previous and next buttons over a paged list, with where the page stands. */
export function Pager({
  offset,
  limit,
  total,
  busy,
  onOffset,
}: {
  offset: number
  limit: number
  total: number
  busy: boolean
  onOffset: (offset: number) => void
}) {
  const { language, t } = useI18n()
  const number = (n: number) => n.toLocaleString(language)
  if (total === 0) return null
  return (
    <nav
      aria-label={t.lineup.pager.label}
      className="flex flex-wrap items-center justify-between gap-3 text-sm"
    >
      <p aria-live="polite" className="text-muted tabular-nums">
        {t.lineup.pager.range(
          number(offset + 1),
          number(Math.min(offset + limit, total)),
          number(total),
        )}
      </p>
      <div className="flex gap-2">
        <button
          type="button"
          className={pagerButton}
          disabled={busy || offset === 0}
          onClick={() => onOffset(Math.max(0, offset - limit))}
        >
          {t.lineup.pager.previous}
        </button>
        <button
          type="button"
          className={pagerButton}
          disabled={busy || offset + limit >= total}
          onClick={() => onOffset(offset + limit)}
        >
          {t.lineup.pager.next}
        </button>
      </div>
    </nav>
  )
}

const pagerButton =
  'inline-flex min-h-9 items-center rounded-lg border border-line bg-bg px-3 text-sm font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:border-line'

/** A switch: a checkbox drawn as a toggle, labelled for screen readers when its label is hidden. */
export function Switch({
  label,
  checked,
  disabled,
  onChange,
  showLabel = false,
}: {
  label: string
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
  showLabel?: boolean
}) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-zinc-200">
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        className="peer sr-only"
      />
      <span
        aria-hidden="true"
        className="relative h-5 w-9 shrink-0 rounded-full bg-line transition-colors peer-checked:bg-fin-3 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-fin-5 peer-disabled:opacity-50 after:absolute after:top-0.5 after:left-0.5 after:size-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4"
      />
      <span className={showLabel ? '' : 'sr-only'}>{label}</span>
    </label>
  )
}

/** A dialog sliding in from the right, full screen on phones, for editing one thing. */
export function SidePanel({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean
  title: string
  onClose: () => void
  children: ReactNode
}) {
  const { t } = useI18n()
  const titleId = useId()
  // The dialog is opened modally once mounted, and closed by the parent unmounting it.
  return open ? (
    <dialog
      ref={(element) => {
        if (element && !element.open) element.showModal()
      }}
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault()
        onClose()
      }}
      onClick={(event) => event.target === event.currentTarget && onClose()}
      className="m-0 ml-auto h-dvh max-h-dvh w-full max-w-xl border-l border-line bg-bg p-0 text-zinc-100 backdrop:bg-black/60 sm:w-[min(36rem,100vw)]"
    >
      <div className="flex h-full flex-col">
        <div className="flex items-center justify-between gap-3 border-b border-line px-5 py-3">
          <h2 id={titleId} className="min-w-0 truncate text-base font-semibold text-white">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex min-h-9 shrink-0 items-center rounded-lg border border-line px-3 text-sm font-medium text-white transition-colors hover:border-fin-4"
          >
            {t.lineup.close}
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">{children}</div>
      </div>
    </dialog>
  ) : null
}

/**
 * A channel's logo, through Polyfin as Jellyfin apps get it, in a fixed box; its initial when it has
 * none or it fails to load. `logo` only tells whether there is one, and refreshes it when it changes.
 */
export function ChannelLogo({
  id,
  logo,
  name,
  size = 'size-9',
}: {
  id: string
  logo: string | null
  name: string
  size?: string
}) {
  const [failed, setFailed] = useState<string | null>(null)
  const frame = `${size} shrink-0 overflow-hidden rounded-md bg-surface-2`
  if (!logo || failed === logo) {
    return (
      <span
        aria-hidden="true"
        className={`${frame} flex items-center justify-center text-xs font-semibold text-muted`}
      >
        {name.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return (
    <img
      src={`/Items/${encodeURIComponent(id)}/Images/Primary?maxHeight=80&tag=${encodeURIComponent(logo)}`}
      alt=""
      loading="lazy"
      onError={() => setFailed(logo)}
      className={`${frame} object-contain p-0.5`}
    />
  )
}
