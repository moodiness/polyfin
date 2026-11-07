import { WarningCircleIcon } from '@phosphor-icons/react'
import { createContext, use, useId, type ReactNode } from 'react'
import { cx } from './cx'

type FieldContextValue = {
  /** The id of the control, which the label points to. */
  id: string
  /** The ids of the help and error texts, for `aria-describedby`. */
  describedBy: string | undefined
  invalid: boolean
}

const FieldContext = createContext<FieldContextValue | null>(null)

/**
 * The label, help and error of the Field around a control. TextInput, NumberInput, Select,
 * Textarea and Switch read it, so they need no `id` of their own inside a Field.
 */
export function useField(): FieldContextValue | null {
  return use(FieldContext)
}

/** Joins `aria-describedby` ids, dropping empty ones. */
export function describedBy(...ids: (string | undefined)[]): string | undefined {
  const joined = ids.filter(Boolean).join(' ')
  return joined === '' ? undefined : joined
}

export type FieldProps = {
  /** The visible label. Use sentence case, no colon. */
  label: ReactNode
  /** A short explanation under the control: range, default, consequence. */
  help?: ReactNode
  /** The error under the control, from validation or the server. Marks the control invalid. */
  error?: ReactNode
  /** Hides the label visually (it stays for screen readers), when the context already names it. */
  hideLabel?: boolean
  /** Something at the end of the label row, such as a StatusPill or an "optional" word. */
  aside?: ReactNode
  /** Puts the help between the label and the control, as settings rows do. */
  helpFirst?: boolean
  /** One control: TextInput, NumberInput, Select, Textarea, or a row holding one. */
  children: ReactNode
  className?: string
}

/**
 * A labelled control with its help and error. The control gets its id and `aria-describedby`
 * from the Field, so a click on the label focuses it and screen readers read the help:
 *
 *   <Field label={t.users.name} help={t.common.nameRule} error={nameError}>
 *     <TextInput value={name} onValue={setName} />
 *   </Field>
 */
export function Field({
  label,
  help,
  error,
  hideLabel = false,
  aside,
  helpFirst = false,
  children,
  className,
}: FieldProps) {
  const id = useId()
  const helpId = help ? `${id}-help` : undefined
  const errorId = error ? `${id}-error` : undefined
  const helpText = help && (
    <p id={helpId} className={cx('max-w-[66ch] text-small text-ink-2', helpFirst && '-mt-0.5')}>
      {help}
    </p>
  )
  return (
    <FieldContext
      value={{ id, describedBy: describedBy(helpId, errorId), invalid: Boolean(error) }}
    >
      <div className={cx('flex min-w-0 flex-col gap-1.5', className)}>
        <div className={cx('flex items-center justify-between gap-3', hideLabel && 'sr-only')}>
          <label htmlFor={id} className={fieldLabel}>
            {label}
          </label>
          {aside}
        </div>
        {helpFirst && helpText}
        {children}
        {!helpFirst && helpText}
        {error && <FieldError id={errorId}>{error}</FieldError>}
      </div>
    </FieldContext>
  )
}

/** The look of a control's label: 17 px, semibold, softer than the value typed under it. */
export const fieldLabel = 'text-small font-semibold text-ink-2'

/** An error under a control, with its icon. Field draws it; use it alone only outside a Field. */
export function FieldError({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <p id={id} role="alert" className="flex items-start gap-2 text-small font-medium text-danger">
      <WarningCircleIcon size="1.25rem" aria-hidden="true" className="mt-px shrink-0" />
      <span>{children}</span>
    </p>
  )
}
