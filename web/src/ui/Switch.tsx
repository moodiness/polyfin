import type { Ref } from 'react'
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
  ref?: Ref<HTMLButtonElement>
  /** Marks the switch as a setting row's head, which ↑ ↓ move between (`SettingRow`). */
  'data-setting-head'?: boolean
}

/** The 42 × 24 toggle (36 × 20 in Compact), hidden from screen readers. Periwinkle when on. */
function SwitchTrack({ checked }: { checked: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        'relative inline-block h-6 w-10.5 shrink-0 rounded-full transition-[background-color] duration-350 ease-nuit',
        'max-sm:h-7 max-sm:w-12',
        checked ? 'bg-accent' : 'bg-s4',
      )}
    >
      <span
        className={cx(
          'absolute top-0.75 left-0.75 size-4.5 rounded-full shadow-[0_3px_8px_-2px_rgb(0_0_0/0.4)] transition-[translate,background-color] duration-500 ease-nuit max-sm:size-5.5',
          checked ? 'translate-x-4.5 bg-white max-sm:translate-x-5' : 'bg-ink',
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
  ref,
  'data-setting-head': settingHead,
}: SwitchProps) {
  const { t } = useI18n()
  const field = useField()
  const words = stateText === true ? [t.ui.on, t.ui.off] : stateText || null
  return (
    <button
      ref={ref}
      type="button"
      role="switch"
      data-setting-head={settingHead}
      id={id ?? field?.id}
      aria-checked={checked}
      aria-label={label}
      aria-labelledby={labelledBy}
      aria-describedby={describedBy(field?.describedBy, describedById)}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        'inline-flex shrink-0 cursor-pointer items-center gap-2 rounded-full text-control text-ink-2 focus-lit [--lift:1.04]',
        words ? 'py-0.5 pr-0.5 pl-3' : 'p-0.5',
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
