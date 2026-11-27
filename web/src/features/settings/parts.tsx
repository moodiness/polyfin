import { useId, type ReactNode } from 'react'
import { cx, Field, SettingHelp } from '@/ui'

/** The ring a row targeted by its `#anchor` wears for a moment (`data-flash`). */
const flash =
  'scroll-mt-32 rounded-row transition-[box-shadow] duration-350 ease-nuit data-flash:shadow-[0_0_0_4px] data-flash:shadow-link/45'

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
 * What is not one setting (a list, an editor, a group of checkboxes) as a tile of its own beside
 * the setting rows: the target of its `#anchor`, which scrolls to it and highlights it for a
 * moment (`data-flash`).
 */
export function PlainRow({ anchor, children }: { anchor: string; children: ReactNode }) {
  return (
    <div id={anchor} className={cx('min-w-0 bg-s1 px-4 py-3', flash)}>
      {children}
    </div>
  )
}

/**
 * A list that draws its own rows (Home rows, notification targets, backup destinations, the
 * orders of versions): its help, two lines with More, then the rows straight in the group, with
 * no tile around them. The target of its `#anchor`.
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
    <div id={anchor} className={cx('min-w-0', flash)}>
      <div className="mb-2.5">
        <SettingHelp id={`${anchor}-help`}>{help}</SettingHelp>
      </div>
      {children}
    </div>
  )
}

/**
 * A setting whose control has no `SettingRow` kind (several checkboxes, a list, an editor):
 * label, help (two lines with More), the control, and its error under it, in a plain tile.
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
  const helpId = `${anchor}-help`
  return (
    <PlainRow anchor={anchor}>
      <div role="group" aria-describedby={help === undefined ? undefined : helpId}>
        <Field
          label={label}
          error={error}
          aside={aside}
          className="[&_label]:text-h3 [&_label]:text-ink"
        >
          {help !== undefined && (
            <div className="-mt-1">
              <SettingHelp id={helpId}>{help}</SettingHelp>
            </div>
          )}
          {children}
        </Field>
      </div>
    </PlainRow>
  )
}

/**
 * Prose under a group's title (a setup step), outside any control: no tile, as text, two lines
 * with More; then what must stay in view under it, such as an address to copy. The target of its
 * `#anchor`.
 */
export function TextRow({
  anchor,
  text,
  children,
}: {
  anchor: string
  text: ReactNode
  children?: ReactNode
}) {
  return (
    <div id={anchor} className={cx('min-w-0 pb-1', flash)}>
      <SettingHelp id={`${anchor}-help`}>{text}</SettingHelp>
      {children}
    </div>
  )
}
