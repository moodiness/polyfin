import { EyeIcon, EyeSlashIcon, type Icon } from '@phosphor-icons/react'
import {
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type Ref,
  type TextareaHTMLAttributes,
} from 'react'
import { useI18n } from '@/i18n'
import { cx, rem } from './cx'
import { describedBy, useField } from './Field'
import { IconButton } from './Button'

/**
 * The well every text control sits in: a big soft fill, lit while you type in it, with a red edge
 * when invalid (the `well` utility).
 */
export const fieldBox = 'well'

/**
 * The sizes of a well. `md` is a 38 px well (32 in Compact, 44 on a phone), the default, for
 * forms and setting rows; `sm` 34 px for filter bars and toolbars; `compact` 32 px, for compact
 * rows, table cells and dense filter bars. All light up the same way, and keep 16 px words on a
 * phone.
 */
export type WellSize = 'md' | 'sm' | 'compact'

/**
 * The classes of each well size: the box, its text (and in mono), an icon at its start, a unit at
 * its end, and the IconButton size that fits inside it.
 */
export const wellSizes: Record<
  WellSize,
  {
    box: string
    /** The box's least height, for a well that grows (a Select that wraps). */
    min: string
    text: string
    mono: string
    lead: string
    icon: number
    suffix: string
    button: 'md' | 'sm'
  }
> = {
  md: {
    box: 'h-9.5 max-sm:h-11',
    min: 'min-h-9.5 max-sm:min-h-11',
    text: 'px-3 text-control font-medium',
    mono: 'px-3 font-mono text-small max-sm:text-base',
    lead: 'ml-3',
    icon: 18,
    suffix: 'pr-3 text-control',
    button: 'sm',
  },
  sm: {
    box: 'h-8.5 max-sm:h-10',
    min: 'min-h-8.5 max-sm:min-h-10',
    text: 'px-2.5 text-control',
    mono: 'px-2.5 font-mono text-small max-sm:text-base',
    lead: 'ml-2.5',
    icon: 17,
    suffix: 'pr-2.5 text-control',
    button: 'sm',
  },
  compact: {
    box: 'h-8 [--well-radius:0.5rem]',
    min: 'min-h-8 [--well-radius:0.5rem]',
    text: 'px-2.5 text-small max-sm:text-base',
    mono: 'px-2.5 font-mono text-micro max-sm:text-base',
    lead: 'ml-2.5',
    icon: 16,
    suffix: 'pr-2.5 text-small',
    button: 'sm',
  },
}

/** The text inside a well, with the `text` of its `wellSizes`. */
export const control =
  'h-full min-w-0 flex-1 bg-transparent text-ink outline-none placeholder:text-ink-3 focus-visible:outline-none disabled:cursor-not-allowed'

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
  /** A unit or short text at the end: "%", "min", "MB". */
  suffix?: ReactNode
  /** Controls at the end, inside the box: an IconButton or a Kbd. */
  end?: ReactNode
  /** Marks the field invalid without a Field (a Field with an error does it already). */
  invalid?: boolean
  /** For `type="password"`: adds an eye that shows what is typed (`aria-pressed`). */
  revealable?: boolean
  /** The well's size; see WellSize. Defaults to the 38 px `md`. */
  size?: WellSize
  /** Mono with tabular figures, for codes, paths and addresses. */
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
  const well = wellSizes[size]
  return (
    <div className={cx(fieldBox, well.box, className)}>
      {Lead && (
        <Lead
          size={rem(well.icon)}
          aria-hidden="true"
          className={cx('shrink-0 text-ink-2', well.lead)}
        />
      )}
      <input
        type={revealable && shown ? 'text' : type}
        className={cx(control, mono ? well.mono : well.text, Lead && 'pl-3')}
        onChange={(event) => {
          onChange?.(event)
          onValue?.(event.target.value)
        }}
        {...controlProps}
        {...rest}
      />
      {suffix !== undefined && (
        <span className={cx('shrink-0 text-ink-2', well.suffix)}>{suffix}</span>
      )}
      {revealable && (
        <IconButton
          label={shown ? t.ui.hide : t.ui.show}
          icon={shown ? EyeSlashIcon : EyeIcon}
          size={well.button}
          pressed={shown}
          aria-controls={controlProps.id}
          onClick={() => setShown(!shown)}
          className="mr-0.5"
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
  /** The well's size; see WellSize. Defaults to the 38 px `md`. */
  size?: WellSize
  /** Classes of the outer box; it is 128 px wide by default (96 for `compact`). */
  className?: string
}

/**
 * A number field, with tabular figures. `min`, `max` and `step` go to the browser; show
 * the range in the Field's help ("From 50 to 100. 90 by default.").
 */
export function NumberInput({
  value,
  onValue,
  suffix,
  invalid,
  size = 'md',
  className,
  id,
  'aria-describedby': ownDescribedBy,
  ...rest
}: NumberInputProps) {
  const controlProps = useControlProps(id, ownDescribedBy, invalid)
  const well = wellSizes[size]
  return (
    <div className={cx(fieldBox, well.box, size === 'compact' ? 'w-24' : 'w-32', className)}>
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
          well.text,
          'figures [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
        )}
        {...controlProps}
        {...rest}
      />
      {suffix !== undefined && (
        <span className={cx('shrink-0 text-ink-2', well.suffix)}>{suffix}</span>
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
        className={cx(
          control,
          'resize-y px-3 py-2',
          mono ? 'font-mono text-small max-sm:text-base' : 'text-body',
        )}
        {...controlProps}
        {...rest}
      />
    </div>
  )
}
