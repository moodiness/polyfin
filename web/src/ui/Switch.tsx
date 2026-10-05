import { useI18n } from '@/i18n'
import { cx } from './cx'
import { describedBy, useField } from './Field'

export type SwitchProps = {
  /** Whether it is on. */
  checked: boolean
  /** Called with the new state. */
  onChange: (checked: boolean) => void
  /**
   * The accessible name when no visible label points to the switch (a Field, or `labelledBy`
   * naming the setting's title).
   */
  label?: string
  /** The id of the visible text naming the switch, such as a settings row title. */
  labelledBy?: string
  /** The id of the text explaining it. */
  describedById?: string
  /**
   * Writes the state beside the switch, for settings where on and off need a word: `true` says
   * "On" / "Off", or give your own pair, such as `['Active', 'Off']`.
   */
  stateText?: boolean | readonly [on: string, off: string]
  disabled?: boolean
  id?: string
  className?: string
}

/**
 * An on/off switch (`role="switch"`) that applies at once or with the page's save bar. For a
 * choice inside a list of options, use Checkbox. The thumb slides in 220 ms.
 */
export function Switch({
  checked,
  onChange,
  label,
  labelledBy,
  describedById,
  stateText = false,
  disabled = false,
  id,
  className,
}: SwitchProps) {
  const { t } = useI18n()
  const field = useField()
  const words = stateText === true ? [t.ui.on, t.ui.off] : stateText || null
  const button = (
    <button
      type="button"
      role="switch"
      id={id ?? field?.id}
      aria-checked={checked}
      aria-label={label}
      aria-labelledby={labelledBy}
      aria-describedby={describedBy(field?.describedBy, describedById)}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        'group relative inline-flex h-[22px] w-[38px] shrink-0 cursor-pointer rounded-full border border-line-3 bg-s4',
        'transition-[background-color,border-color] duration-220 ease-nuit',
        'aria-checked:border-accent aria-checked:bg-accent',
        'disabled:cursor-not-allowed disabled:opacity-45',
        !words && className,
      )}
    >
      <span
        aria-hidden="true"
        className="absolute top-[3px] left-[3px] size-3.5 rounded-full bg-ink-3 transition-[translate,background-color] duration-220 ease-nuit group-aria-checked:translate-x-4 group-aria-checked:bg-white"
      />
    </button>
  )
  if (!words) return button
  return (
    <span className={cx('inline-flex items-center gap-2.5 text-small text-ink-2', className)}>
      <span aria-hidden="true">{checked ? words[0] : words[1]}</span>
      {button}
    </span>
  )
}
