import type { Icon } from '@phosphor-icons/react'
import { cx, rem } from './cx'

export type SegmentedOption<T extends string> = {
  value: T
  label: string
  /** Optional Phosphor icon before the label. */
  icon?: Icon
  /** A `lang` attribute, for options written in another language. */
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
  /**
   * `md` (60 px choices) in pages, the default; `sm` (52 px) for toolbars; `compact` (44 px, 17 px
   * text) for compact rows, table cells and dense filter bars.
   */
  size?: 'md' | 'sm' | 'compact'
  className?: string
}

const sizes = {
  md: 'h-15 min-w-15 rounded-choice px-4.5 text-[1.3125rem] max-sm:h-13 max-sm:min-w-13 max-sm:px-3.5 max-sm:text-[1.1875rem]',
  sm: 'h-13 min-w-13 rounded-[1rem] px-4 text-control',
  compact: 'h-11 min-w-11 rounded-[0.875rem] px-3.5 text-small',
}

const iconSizes = { md: 22, sm: 20, compact: 18 }

/**
 * Two to four exclusive choices shown side by side as a strip of big choices (a list's filter, a
 * view mode), the current one in periwinkle. Each is a button with `aria-pressed` that lights up
 * under the keyboard. The strip wraps rather than overflow a phone. For more choices, use Select;
 * for navigation, use Tabs.
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
        'inline-flex max-w-full flex-wrap',
        size === 'compact' ? 'gap-1.5' : 'gap-2',
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
            'inline-flex cursor-pointer items-center justify-center gap-2 bg-s1 font-[620] text-ink-2 figures focus-lit [--lift:1.06] hover:bg-s2 hover:text-ink',
            'aria-pressed:bg-accent aria-pressed:text-on-accent',
            sizes[size],
          )}
        >
          {option.icon && <option.icon size={rem(iconSizes[size])} aria-hidden="true" />}
          {option.label}
        </button>
      ))}
    </span>
  )
}
