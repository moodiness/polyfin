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
   * The word of the state beside the switch: "On" / "Off" by default, your own pair such as
   * `['Active', 'Off']`, or `false` where the row already says it.
   */
  stateText?: boolean | readonly [on: string, off: string]
  disabled?: boolean
  id?: string
  className?: string
}

/**
 * The 60 × 36 toggle drawn alone, hidden from screen readers: for a row that is itself the
 * `role="switch"` button (a settings row). Periwinkle when on; inside a lit row its colors switch
 * by themselves.
 */
export function SwitchTrack({ checked, className }: { checked: boolean; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        'relative inline-block h-9 w-15 shrink-0 rounded-full transition-[background-color] duration-350 ease-nuit',
        'max-sm:h-8 max-sm:w-13',
        checked ? 'bg-accent' : 'bg-s4',
        className,
      )}
    >
      <span
        className={cx(
          'absolute top-1 left-1 size-7 rounded-full shadow-[0_4px_10px_-2px_rgb(0_0_0/0.4)] transition-[translate,background-color] duration-500 ease-nuit max-sm:size-6',
          checked ? 'translate-x-6 bg-white max-sm:translate-x-5' : 'bg-ink',
        )}
      />
    </span>
  )
}

/**
 * An on/off switch (`role="switch"`) that applies at once or with the page's save bar, with the
 * word of its state beside it. For a choice inside a list of options, use Checkbox. Under the
 * keyboard the toggle and its word light up together.
 */
export function Switch({
  checked,
  onChange,
  label,
  labelledBy,
  describedById,
  stateText = true,
  disabled = false,
  id,
  className,
}: SwitchProps) {
  const { t } = useI18n()
  const field = useField()
  const words = stateText === true ? [t.ui.on, t.ui.off] : stateText || null
  return (
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
        'inline-flex shrink-0 cursor-pointer items-center gap-3 rounded-full text-body text-ink-2 focus-lit [--lift:1.04]',
        words ? 'py-1 pr-1 pl-4' : 'p-1',
        'hover:text-ink disabled:cursor-not-allowed disabled:opacity-45',
        className,
      )}
    >
      {words && (
        <span aria-hidden="true" className="min-w-[2.2em] text-left">
          {checked ? words[0] : words[1]}
        </span>
      )}
      <SwitchTrack checked={checked} />
    </button>
  )
}
