import { EyeIcon, EyeSlashIcon, type Icon } from '@phosphor-icons/react'
import {
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type Ref,
  type TextareaHTMLAttributes,
} from 'react'
import { useI18n } from '@/i18n'
import { cx } from './cx'
import { describedBy, useField } from './Field'
import { IconButton } from './Button'

/** The box every text control sits in: background, hairline, hover, focus and invalid states. */
export const fieldBox = cx(
  'relative flex min-w-0 items-center rounded-field border border-line-2 bg-s2 transition-[border-color,box-shadow] duration-160 ease-nuit',
  'hover:border-line-3 focus-within:border-accent focus-within:ring-3 focus-within:ring-accent/15',
  'has-[[aria-invalid]]:border-danger/70 has-[[aria-invalid]]:focus-within:ring-danger/15',
  'has-disabled:cursor-not-allowed has-disabled:opacity-50 has-disabled:hover:border-line-2',
)

/** The text inside a field box. */
export const control =
  'h-full min-w-0 flex-1 bg-transparent px-3 text-body text-ink outline-none placeholder:text-ink-3 focus-visible:outline-none disabled:cursor-not-allowed'

/** The id and `aria-*` a control takes from its Field, unless given its own. */
export function useControlProps(
  id: string | undefined,
  describedById: string | undefined,
  invalid?: boolean,
) {
  const field = useField()
  const isInvalid = invalid ?? field?.invalid ?? false
  return {
    id: id ?? field?.id,
    'aria-describedby': describedBy(field?.describedBy, describedById),
    'aria-invalid': isInvalid || undefined,
  }
}

export type TextInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'prefix'> & {
  /** Called with the new text; simpler than `onChange` for controlled fields. */
  onValue?: (value: string) => void
  /** A Phosphor icon at the start, such as a magnifying glass for a filter. */
  icon?: Icon
  /** A unit or short text at the end, in mono: "%", "min", "MB". */
  suffix?: ReactNode
  /** Controls at the end, inside the box: an IconButton or a Kbd. */
  end?: ReactNode
  /** Marks the field invalid without a Field (a Field with an error does it already). */
  invalid?: boolean
  /** For `type="password"`: adds an eye that shows what is typed (`aria-pressed`). */
  revealable?: boolean
  /** `md` is 40 px tall; `sm` 34 px, for filters and toolbars. */
  size?: 'md' | 'sm'
  /** Mono with tabular figures, for codes, addresses and numbers. */
  mono?: boolean
  /** Classes of the outer box (width, margins). */
  className?: string
  ref?: Ref<HTMLInputElement>
}

/**
 * A one-line text field. Inside a Field it is labelled by it; alone, give it an `aria-label`.
 * Passwords: `type="password" revealable autoComplete="new-password"`.
 */
export function TextInput({
  onValue,
  onChange,
  icon: Lead,
  suffix,
  end,
  invalid,
  revealable = false,
  size = 'md',
  mono = false,
  className,
  type = 'text',
  id,
  'aria-describedby': ownDescribedBy,
  ...rest
}: TextInputProps) {
  const { t } = useI18n()
  const [shown, setShown] = useState(false)
  const controlProps = useControlProps(id, ownDescribedBy, invalid)
  return (
    <div className={cx(fieldBox, size === 'md' ? 'h-10' : 'h-[34px]', className)}>
      {Lead && <Lead size={16} aria-hidden="true" className="ml-3 shrink-0 text-ink-3" />}
      <input
        type={revealable && shown ? 'text' : type}
        className={cx(control, Lead && 'pl-2', mono && 'figures', size === 'sm' && 'text-control')}
        onChange={(event) => {
          onChange?.(event)
          onValue?.(event.target.value)
        }}
        {...controlProps}
        {...rest}
      />
      {suffix !== undefined && (
        <span className="shrink-0 pr-3 font-mono text-small text-ink-3">{suffix}</span>
      )}
      {revealable && (
        <IconButton
          label={shown ? t.ui.hide : t.ui.show}
          icon={shown ? EyeSlashIcon : EyeIcon}
          pressed={shown}
          aria-controls={controlProps.id}
          onClick={() => setShown(!shown)}
          className="mr-[3px] size-8 rounded-md"
        />
      )}
      {end}
    </div>
  )
}

export type NumberInputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'size' | 'value' | 'onChange' | 'type' | 'prefix'
> & {
  /** The number, or null while the field is empty. */
  value: number | null
  /** Called with the number typed, or null when the field is emptied or holds no number. */
  onValue: (value: number | null) => void
  /** A unit at the end: "%", "min", "h". */
  suffix?: ReactNode
  invalid?: boolean
  /** Classes of the outer box; it is 120 px wide by default. */
  className?: string
}

/**
 * A number field, in mono with tabular figures. `min`, `max` and `step` go to the browser; show
 * the range in the Field's help ("From 50 to 100. 90 by default.").
 */
export function NumberInput({
  value,
  onValue,
  suffix,
  invalid,
  className,
  id,
  'aria-describedby': ownDescribedBy,
  ...rest
}: NumberInputProps) {
  const controlProps = useControlProps(id, ownDescribedBy, invalid)
  return (
    <div className={cx(fieldBox, 'h-10 w-[120px]', className)}>
      <input
        type="number"
        inputMode="decimal"
        value={value ?? ''}
        onChange={(event) => {
          const text = event.target.value.trim()
          const number = Number(text)
          onValue(text === '' || !Number.isFinite(number) ? null : number)
        }}
        className={cx(
          control,
          'figures text-lead [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
        )}
        {...controlProps}
        {...rest}
      />
      {suffix !== undefined && (
        <span className="shrink-0 pr-3 font-mono text-small text-ink-3">{suffix}</span>
      )}
    </div>
  )
}

export type TextareaProps = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
  /** Called with the new text. */
  onValue?: (value: string) => void
  onChange?: TextareaHTMLAttributes<HTMLTextAreaElement>['onChange']
  invalid?: boolean
  /** Mono, for code and lists of addresses. */
  mono?: boolean
}

/** A multi-line text field. Rows default to 4; it can be resized vertically. */
export function Textarea({
  onValue,
  onChange,
  invalid,
  mono = false,
  rows = 4,
  className,
  id,
  'aria-describedby': ownDescribedBy,
  ...rest
}: TextareaProps) {
  const controlProps = useControlProps(id, ownDescribedBy, invalid)
  return (
    <div className={cx(fieldBox, 'items-stretch', className)}>
      <textarea
        rows={rows}
        onChange={(event) => {
          onChange?.(event)
          onValue?.(event.target.value)
        }}
        className={cx(control, 'resize-y py-2.5', mono && 'font-mono text-small')}
        {...controlProps}
        {...rest}
      />
    </div>
  )
}
