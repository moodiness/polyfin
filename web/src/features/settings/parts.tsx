import { useId, type ReactNode } from 'react'
import { cx, Field, Switch } from '@/ui'

/** A titled group of settings in a section, like « Passer l'intro et le générique ». */
export function SettingsGroup({ title, children }: { title?: string; children: ReactNode }) {
  const id = useId()
  return (
    <section aria-labelledby={title === undefined ? undefined : id} className="mt-11 first:mt-8">
      {title !== undefined && (
        <h3 id={id} className="border-b border-line-2 pb-2.5 text-small font-medium text-ink-3">
          {title}
        </h3>
      )}
      <div className="[&>*+*]:border-t [&>*+*]:border-line">{children}</div>
    </section>
  )
}

/**
 * The row of one setting: the target of its `#anchor`, which scrolls to it and highlights it
 * for a moment (`data-flash`).
 */
export function SettingRow({
  anchor,
  className,
  children,
}: {
  anchor: string
  className?: string
  children: ReactNode
}) {
  return (
    <div
      id={anchor}
      className={cx(
        'scroll-mt-32 py-5 data-[flash]:rounded-row transition-[background-color,box-shadow] duration-700 ease-nuit data-[flash]:bg-accent/8 data-[flash]:shadow-[0_0_0_10px] data-[flash]:shadow-accent/8',
        className,
      )}
    >
      {children}
    </div>
  )
}

/** A setting turned on or off: its title and help on the left, the switch on the right. */
export function SwitchRow({
  anchor,
  label,
  help,
  checked,
  onChange,
  stateText = true,
  disabled,
  children,
}: {
  anchor: string
  label: string
  help: ReactNode
  checked: boolean
  onChange: (checked: boolean) => void
  /** The words beside the switch; On / Off by default. */
  stateText?: true | readonly [on: string, off: string]
  disabled?: boolean
  /** More under the help, such as a warning. */
  children?: ReactNode
}) {
  const id = useId()
  return (
    <SettingRow
      anchor={anchor}
      className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-8 gap-y-4"
    >
      <div className="min-w-0">
        <p id={`${id}-label`} className="text-[15px] font-medium tracking-[-0.01em] text-ink">
          {label}
        </p>
        <p id={`${id}-help`} className="mt-1 max-w-[60ch] text-small text-ink-3">
          {help}
        </p>
      </div>
      <Switch
        checked={checked}
        onChange={onChange}
        labelledBy={`${id}-label`}
        describedById={`${id}-help`}
        stateText={stateText}
        disabled={disabled}
        className="mt-0.5"
      />
      {children !== undefined && <div className="col-span-full">{children}</div>}
    </SettingRow>
  )
}

/** A setting typed or chosen in a field: label, help, the control, and its error under it. */
export function FieldRow({
  anchor,
  label,
  help,
  error,
  aside,
  children,
}: {
  anchor: string
  label: ReactNode
  help?: ReactNode
  error?: string
  aside?: ReactNode
  children: ReactNode
}) {
  return (
    <SettingRow anchor={anchor}>
      <Field
        label={label}
        help={help}
        error={error}
        aside={aside}
        helpFirst
        className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
      >
        {children}
      </Field>
    </SettingRow>
  )
}

/** Plain text in a row (a setup step), outside any control. */
export function TextRow({ anchor, children }: { anchor: string; children: ReactNode }) {
  return (
    <SettingRow anchor={anchor} className="text-small text-ink-2">
      {children}
    </SettingRow>
  )
}
