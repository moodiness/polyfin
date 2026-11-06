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
 * libraries, permissions): a 26 px soft box, periwinkle when checked. Under the keyboard the whole
 * row lights up. For a setting that turns something on, prefer Switch.
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
        'group -mx-3 flex min-w-0 cursor-pointer items-start gap-3.5 rounded-choice px-3 py-2 text-body text-ink transition-[background-color,color,scale] duration-300 ease-nuit hover:bg-s1 has-[:focus-visible]:lit',
        disabled && 'cursor-not-allowed opacity-50',
        className,
      )}
    >
      <span className="relative inline-grid size-6.5 shrink-0 place-items-center">
        <input
          ref={input}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={help ? `${id}-help` : undefined}
          onChange={(event) => onChange(event.target.checked)}
          className="peer absolute inset-0 m-0 cursor-pointer appearance-none rounded-check bg-s4 transition-[background-color] duration-300 ease-nuit focus-visible:outline-none checked:bg-accent indeterminate:bg-accent disabled:cursor-not-allowed"
        />
        <Mark
          size={16}
          weight="bold"
          aria-hidden="true"
          className="pointer-events-none relative text-on-accent opacity-0 peer-checked:opacity-100 peer-indeterminate:opacity-100"
        />
      </span>
      <span className={cx('min-w-0 flex-1 pt-px', hideLabel && 'sr-only')}>
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
