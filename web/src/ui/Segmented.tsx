import type { Icon } from '@phosphor-icons/react'
import { cx } from './cx'

export type SegmentedOption<T extends string> = {
  value: T
  label: string
  /** Optional Phosphor icon before the label. */
  icon?: Icon
  /** A `lang` attribute, for options written in another language (FR / EN). */
  lang?: string
  /** A longer name, as a tooltip and for screen readers. */
  title?: string
}

export type SegmentedProps<T extends string> = {
  /** The accessible name of the group. */
  label: string
  options: readonly SegmentedOption<T>[]
  value: T
  onChange: (value: T) => void
  /** `sm` (26 px buttons) for menus and toolbars; `md` (32 px) in pages. */
  size?: 'sm' | 'md'
  className?: string
}

/**
 * Two to four exclusive choices shown side by side (language, a list's filter, a view mode). Each
 * is a button with `aria-pressed`. For more choices, use Select; for navigation, use Tabs.
 */
export function Segmented<T extends string>({
  label,
  options,
  value,
  onChange,
  size = 'md',
  className,
}: SegmentedProps<T>) {
  return (
    <span
      role="group"
      aria-label={label}
      className={cx(
        'inline-flex shrink-0 rounded-field border border-line-2 bg-bg p-0.5',
        className,
      )}
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          lang={option.lang}
          title={option.title}
          aria-pressed={option.value === value}
          onClick={() => onChange(option.value)}
          className={cx(
            'inline-flex cursor-pointer items-center gap-1.5 rounded-md font-medium text-ink-3 transition-[background-color,color] duration-160 ease-nuit hover:text-ink',
            'aria-pressed:bg-s4 aria-pressed:text-ink',
            size === 'sm' ? 'h-[26px] px-2.5 text-[12.5px]' : 'h-8 px-3 text-control',
          )}
        >
          {option.icon && <option.icon size={16} aria-hidden="true" />}
          {option.label}
        </button>
      ))}
    </span>
  )
}
