import { useState } from 'react'
import { cx } from './cx'

export type ProgressVariant = 'accent' | 'playback' | 'neutral' | 'warn' | 'danger'

const fills: Record<ProgressVariant, string> = {
  accent: 'bg-accent',
  playback: 'bg-focus',
  neutral: 'bg-ink-2',
  warn: 'bg-warn',
  danger: 'bg-danger',
}

/**
 * A thick, flat bar showing how far something is: playback (`playback`, pale as the lit state), a
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
  /** `sm` 6 px (inline figures), `md` 8 px (playback), `lg` 10 px (meters). */
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
        'relative overflow-hidden rounded-full bg-ink/20',
        size === 'sm' ? 'h-1.5' : size === 'md' ? 'h-2' : 'h-2.5',
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
 * A user's profile picture, or their initial on a periwinkle disc when they have none or it fails
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
  /**
   * `sm` 26 px (inline, who is playing), `md` 36 px (the account button, a menu), `lg` 44 px (at
   * the start of a row), `xl` 64 px (a person's page).
   */
  size?: 'sm' | 'md' | 'lg' | 'xl'
  label?: string
  className?: string
}) {
  // A picture that failed is not tried again until its address changes.
  const [failed, setFailed] = useState<string | undefined>(undefined)
  const initial = Array.from(name.trim())[0]?.toUpperCase() ?? '?'
  const box = cx(
    'shrink-0 rounded-full',
    size === 'sm' && 'size-6.5 text-[0.8125rem]',
    size === 'md' && 'size-9 text-[1rem]',
    size === 'lg' && 'size-11 text-[1.1875rem]',
    size === 'xl' && 'size-16 text-[1.75rem]',
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
      className={cx(box, 'inline-grid place-items-center bg-accent font-bold text-on-accent')}
    >
      {initial}
    </span>
  )
}
