import { useState } from 'react'
import { cx } from './cx'

export type ProgressVariant = 'accent' | 'brand' | 'neutral' | 'warn' | 'danger'

const fills: Record<ProgressVariant, string> = {
  accent: 'bg-accent',
  // The brand gradient, with its glow: playback progress only.
  brand: 'bg-brand shadow-[0_0_12px] shadow-brand-4/45',
  neutral: 'bg-ink-2',
  warn: 'bg-warn',
  danger: 'bg-danger',
}

/**
 * A bar showing how far something is: playback (`brand`, the only place for the gradient), a
 * disk or cache filling (`neutral`, `warn` when nearly full), a task running (`accent`).
 */
export function ProgressBar({
  value,
  label,
  valueText,
  variant = 'accent',
  size = 'md',
  live = false,
  className,
}: {
  /** From 0 to 1; clamped. */
  value: number
  /** What progresses, for screen readers: "Dune, progress". */
  label: string
  /** The value in words: "57 minutes of 2 hours 35". Defaults to a percentage. */
  valueText?: string
  variant?: ProgressVariant
  /** `sm` 3 px (inline figures), `md` 4 px (playback), `lg` 6 px (meters). */
  size?: 'sm' | 'md' | 'lg'
  /** Moves linearly over a second, for a value updated every second (playback). */
  live?: boolean
  className?: string
}) {
  const clamped = Math.min(1, Math.max(0, Number.isFinite(value) ? value : 0))
  const percent = Math.round(clamped * 100)
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent}
      aria-valuetext={valueText}
      className={cx(
        'relative overflow-hidden rounded-full bg-ink/12',
        size === 'sm' ? 'h-[3px]' : size === 'md' ? 'h-1' : 'h-1.5',
        className,
      )}
    >
      <span
        className={cx(
          'absolute inset-y-0 left-0 rounded-[inherit]',
          live
            ? 'transition-[width] duration-1000 ease-linear'
            : 'transition-[width] duration-220 ease-nuit',
          fills[variant],
        )}
        style={{ width: `${clamped * 100}%` }}
      />
    </div>
  )
}

/**
 * A user's profile picture, or their initial in a rounded square when they have none or it fails
 * to load: the account button, a member in a row, who is playing. Decorative by default, as the
 * name is written beside it; give `label` when it stands alone.
 */
export function Avatar({
  name,
  image,
  size = 'md',
  label,
  className,
}: {
  name: string
  /** The address of the profile picture (`userImageUrl`); the initial stands in without one. */
  image?: string
  /** `sm` 22 px (inline, in rows), `md` 28 px (top bar), `lg` 40 px (a user's page). */
  size?: 'sm' | 'md' | 'lg'
  label?: string
  className?: string
}) {
  // A picture that failed is not tried again until its address changes.
  const [failed, setFailed] = useState<string | undefined>(undefined)
  const initial = Array.from(name.trim())[0]?.toUpperCase() ?? '?'
  const box = cx(
    'shrink-0 border border-line-2',
    size === 'sm' && 'size-[22px] rounded-[7px] text-[11px]',
    size === 'md' && 'size-7 rounded-[9px] text-[12.5px]',
    size === 'lg' && 'size-10 rounded-row text-[15px]',
    className,
  )
  if (image !== undefined && failed !== image) {
    return (
      <img
        src={image}
        alt={label ?? ''}
        onError={() => setFailed(image)}
        className={cx(box, 'bg-s4 object-cover')}
      />
    )
  }
  return (
    <span
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cx(box, 'inline-grid place-items-center bg-s4 font-semibold text-ink')}
    >
      {initial}
    </span>
  )
}
