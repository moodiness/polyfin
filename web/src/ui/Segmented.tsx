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
   * `md` (38 px choices, 32 in Compact, 44 on a phone) in pages and setting rows, the default;
   * `sm` (34 px) for toolbars; `compact` (32 px) for compact rows, table cells and dense filter
   * bars.
   */
  size?: SegmentSize
  className?: string
}

export type SegmentSize = 'md' | 'sm' | 'compact'

const sizes: Record<SegmentSize, string> = {
  md: 'h-9.5 min-w-9.5 rounded-choice px-3 text-control max-sm:h-11 max-sm:min-w-11',
  sm: 'h-8.5 min-w-8.5 rounded-[0.5625rem] px-2.5 text-control',
  compact: 'h-8 min-w-8 rounded-[0.5rem] px-2.5 text-small',
}

const iconSizes = { md: 18, sm: 17, compact: 16 }

/**
 * The classes of one choice among two to four side by side, chosen when `aria-pressed` or
 * `aria-checked` (a SettingRow's radio group).
 */
export function segmentClass(size: SegmentSize): string {
  return cx(
    'inline-flex cursor-pointer items-center justify-center gap-2 bg-s1 font-[620] text-ink-2 figures focus-lit [--lift:1.06] hover:bg-s2 hover:text-ink',
    'aria-checked:bg-accent aria-checked:text-on-accent aria-pressed:bg-accent aria-pressed:text-on-accent',
    'disabled:cursor-not-allowed disabled:opacity-45',
    sizes[size],
  )
}

/**
 * Two to four exclusive choices shown side by side (a list's filter, a view mode), the current
 * one in periwinkle. Each is a button with `aria-pressed` that lights up under the keyboard. The
 * choices wrap rather than overflow a phone. For more choices, use Select;
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
        size === 'md' ? 'gap-1.5' : 'gap-1',
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
          className={segmentClass(size)}
        >
          {option.icon && <option.icon size={rem(iconSizes[size])} aria-hidden="true" />}
          {option.label}
        </button>
      ))}
    </span>
  )
}
