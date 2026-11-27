import { CheckIcon, MinusIcon } from '@phosphor-icons/react'
import { useEffect, useId, useRef, type ReactNode } from 'react'
import { cx } from './cx'

export type CheckboxProps = {
  /** Whether it is checked. */
  checked: boolean
  /** Called with the new state. */
  onChange: (checked: boolean) => void
  /** The visible label; the whole row is clickable. */
  label: ReactNode
  /** A line under the label. */
  help?: ReactNode
  /** Something at the end of the row, such as a count in mono (category lists). */
  aside?: ReactNode
  /** Neither checked nor unchecked: some of a group's items are. */
  indeterminate?: boolean
  disabled?: boolean
  /** Keeps the label for screen readers only, in tables where the column says it. */
  hideLabel?: boolean
  className?: string
}

/**
 * A native checkbox drawn in the « Salon » style, for choosing items of a list (categories,
 * libraries, permissions): a 20 px soft box, periwinkle when checked, on a line of 36 px (31 in
 * Compact). The box takes a press over 24 × 24 px at least, and so does the whole row.
 * Under the keyboard the whole row lights up. A disabled one takes the inactive inks. For a
 * setting that turns something on, prefer Switch.
 */
export function Checkbox({
  checked,
  onChange,
  label,
  help,
  aside,
  indeterminate = false,
  disabled = false,
  hideLabel = false,
  className,
}: CheckboxProps) {
  const id = useId()
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (input.current) input.current.indeterminate = indeterminate
  }, [indeterminate])
  const Mark = indeterminate ? MinusIcon : CheckIcon
  return (
    <label
      className={cx(
        'group -mx-2 flex min-w-0 cursor-pointer items-start gap-2.5 rounded-choice px-2 py-1.5 text-body text-ink transition-[background-color,color,scale] duration-300 ease-nuit hover:bg-s1 has-[:focus-visible]:lit',
        disabled && 'cursor-not-allowed inactive',
        className,
      )}
    >
      <span className="relative mt-0.5 inline-grid size-5 shrink-0 place-items-center">
        {/* The native box, transparent and 24 px at least around the drawn one: the target. */}
        <input
          ref={input}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={help ? `${id}-help` : undefined}
          onChange={(event) => onChange(event.target.checked)}
          className="peer absolute top-1/2 left-1/2 m-0 size-[max(100%,24px)] -translate-1/2 cursor-pointer appearance-none bg-transparent focus-visible:outline-none disabled:cursor-not-allowed"
        />
        <span
          aria-hidden="true"
          className={cx(
            'pointer-events-none absolute inset-0 rounded-check transition-[background-color] duration-300 ease-nuit',
            disabled
              ? 'bg-s4/60 peer-checked:bg-accent/45 peer-indeterminate:bg-accent/45'
              : 'bg-s4 peer-checked:bg-accent peer-indeterminate:bg-accent',
          )}
        />
        <Mark
          size="0.8125rem"
          weight="bold"
          aria-hidden="true"
          className={cx(
            'pointer-events-none relative opacity-0 peer-checked:opacity-100 peer-indeterminate:opacity-100',
            disabled ? 'text-on-accent/60' : 'text-on-accent',
          )}
        />
      </span>
      <span className={cx('min-w-0 flex-1', hideLabel && 'sr-only')}>
        <span className="block">{label}</span>
        {help && (
          <span id={`${id}-help`} className="mt-0.5 block text-small text-ink-3">
            {help}
          </span>
        )}
      </span>
      {aside && <span className="ml-auto shrink-0 text-small text-ink-3">{aside}</span>}
    </label>
  )
}
