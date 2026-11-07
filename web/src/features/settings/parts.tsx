import { useId, type ReactNode } from 'react'
import { cx, Field } from '@/ui'

/**
 * A titled group of settings in a section, like « Passer l'intro et le générique »: its rows,
 * `SettingRow`s of `@/ui` or `PlainRow`s, go 6 px apart.
 */
export function SettingsGroup({ title, children }: { title?: string; children: ReactNode }) {
  const id = useId()
  return (
    <section
      aria-labelledby={title === undefined ? undefined : id}
      className="mt-7 first:mt-5 max-md:mt-6 max-md:first:mt-4"
    >
      {title !== undefined && (
        <h3 id={id} className="mb-2 text-group text-ink">
          {title}
        </h3>
      )}
      <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">{children}</div>
    </section>
  )
}

/**
 * What is not one setting (a list, an editor, a group of checkboxes, a setup step) as a tile of
 * its own beside the setting rows: the target of its `#anchor`, which scrolls to it and
 * highlights it for a moment (`data-flash`).
 */
export function PlainRow({
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
        'min-w-0 scroll-mt-32 rounded-row bg-s1 px-4 py-3',
        'transition-[box-shadow] duration-350 ease-nuit data-flash:shadow-[0_0_0_4px] data-flash:shadow-link/45',
        className,
      )}
    >
      {children}
    </div>
  )
}

/**
 * A list that draws its own rows (Home rows, notification targets, backup destinations): its
 * help, then the rows straight in the group, with no tile around them. The target of its
 * `#anchor`.
 */
export function ListRows({
  anchor,
  help,
  children,
}: {
  anchor: string
  help: ReactNode
  children: ReactNode
}) {
  return (
    <div
      id={anchor}
      className="min-w-0 scroll-mt-32 rounded-row transition-[box-shadow] duration-350 ease-nuit data-flash:shadow-[0_0_0_4px] data-flash:shadow-link/45"
    >
      <p className="mb-2.5 max-w-[72ch] text-small text-ink-3">{help}</p>
      {children}
    </div>
  )
}

/**
 * A setting whose control has no `SettingRow` kind (several checkboxes, a list, an editor):
 * label, help, the control, and its error under it, in a plain tile.
 */
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
    <PlainRow anchor={anchor}>
      <Field
        label={label}
        help={help}
        error={error}
        aside={aside}
        helpFirst
        className="[&_label]:text-h3 [&_label]:text-ink"
      >
        {children}
      </Field>
    </PlainRow>
  )
}

/** Plain text in a tile (a setup step), outside any control. */
export function TextRow({ anchor, children }: { anchor: string; children: ReactNode }) {
  return (
    <PlainRow anchor={anchor} className="text-body text-ink-2">
      {children}
    </PlainRow>
  )
}
