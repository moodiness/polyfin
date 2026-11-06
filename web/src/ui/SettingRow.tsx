import { CaretRightIcon } from '@phosphor-icons/react'
import {
  useEffect,
  useId,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { useI18n, type Messages } from '@/i18n'
import { cx } from './cx'
import { describedBy, FieldError } from './Field'
import { NumberInput, TextInput } from './Input'
import { Kbd } from './Kbd'
import { SecretField, type SecretFieldProps } from './SecretField'
import { Select, type SelectOption } from './Select'
import { SwitchTrack } from './Switch'

/** Past this many choices, a choice opens a searchable list instead of a strip. */
const stripMax = 12
/** Up to this many values, a number opens a strip of them instead of a well. */
const numberStripMax = 33

type Common = {
  /** The id of the row: the target of its `#anchor`, which a page may flash (`data-flash`). */
  anchor?: string
  /** The setting's name, on the left. */
  label: string
  /** A second line under the label. */
  detail?: ReactNode
  /**
   * What the setting does: shown only while the row is lit, always read by screen readers
   * (`aria-describedby`). Range and default belong here ("From 0 to 32. 0 by default.").
   */
  help?: ReactNode
  /** The error of this setting, from validation or the server; shown under the row. */
  error?: ReactNode
  /** More under the row, always shown: a warning, a note. */
  children?: ReactNode
  disabled?: boolean
  className?: string
}

export type SwitchSetting = {
  kind: 'switch'
  checked: boolean
  onChange: (checked: boolean) => void
  /** The saved state; with it, a changed row keeps a dot and "was Off". */
  saved?: boolean
  /** The words of the states; On / Off by default. */
  stateText?: readonly [on: string, off: string]
}

export type ChoiceSetting<T extends string | number> = {
  kind: 'choice'
  value: T
  /** The choices in order: a strip of big choices, or a searchable list past 12. */
  options: readonly SelectOption<T>[]
  onChange: (value: T) => void
  saved?: T
}

export type NumberSetting = {
  kind: 'number'
  /** The number, or null while the well is empty. */
  value: number | null
  onChange: (value: number | null) => void
  min: number
  max: number
  step?: number
  /** The unit after the value and in the well: "min", "GB". */
  unit?: string
  /** Writes a value (strip and display): `(n) => n === 0 ? 'No limit' : String(n)`. */
  format?: (value: number) => string
  /**
   * `strip`: a strip of every value, for small ranges (0 to 32); `well`: a number field. By
   * default a strip up to 33 values, a well past that.
   */
  picker?: 'strip' | 'well'
  saved?: number | null
}

export type TextSetting = {
  /** `path` writes the value in tabular figures and, on a phone, under the label. */
  kind: 'text' | 'path'
  value: string
  onChange: (value: string) => void
  saved?: string
  /**
   * The value used while the setting is empty (a default folder or address): shown in the empty
   * well, and dimmed in the closed row, read "Default: …" by screen readers.
   */
  placeholder?: string
  maxLength?: number
  inputMode?: 'text' | 'url' | 'email' | 'numeric'
  /** Mono in the well, for codes and addresses. */
  mono?: boolean
}

export type SecretSetting = {
  kind: 'secret'
} & Omit<SecretFieldProps, 'label' | 'help' | 'error'>

export type SettingRowProps<T extends string | number = string> = Common &
  (SwitchSetting | ChoiceSetting<T> | NumberSetting | TextSetting | SecretSetting)

/**
 * One setting as a Salon row: the label on the left, the value and a chevron on the right, the
 * help shown only while the row is lit. Enter, Space or a click opens its control inside the
 * row; Escape closes it and returns to the row; ↑ ↓ move to the next row without trapping Tab.
 * - `switch`: the whole row is the `role="switch"`, with the toggle and its word.
 * - `choice`: a strip of big choices (`role="radiogroup"`, ← → move and choose, Enter closes),
 *   or a searchable list past 12 choices.
 * - `number`: a strip of values, or a number well with its unit.
 * - `text` and `path`: a text well (Enter closes).
 * - `secret`: the SecretField (masked typing, reveal, replace, remove).
 *
 * With `saved`, a changed row keeps a dot and "was 0", and its value in the link color. Rows go
 * 8 px apart (`grid grid-cols-[minmax(0,1fr)] gap-2`).
 */
export function SettingRow<T extends string | number = string>(props: SettingRowProps<T>) {
  const { anchor, label, detail, help, error, children, disabled = false, className } = props
  const { t } = useI18n()
  const id = useId()
  const labelId = `${id}-label`
  const valueId = `${id}-value`
  const helpId = help !== undefined ? `${id}-help` : undefined
  const errorId = error ? `${id}-error` : undefined
  const pickerId = `${id}-picker`
  const head = useRef<HTMLButtonElement>(null)
  const picker = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(false)
  const [open, setOpen] = useState(false)
  const lit = focused || open

  const { display, was, changed } = describe(props, t)
  const strip = stripChoices(props)

  // Opening moves the keyboard into the control: the chosen choice, or the well.
  useEffect(() => {
    if (!open) return
    const box = picker.current
    const into =
      box?.querySelector<HTMLElement>('[aria-checked="true"]') ??
      box?.querySelector<HTMLElement>('input, button:not([disabled])')
    into?.focus()
  }, [open])

  // The chosen choice stays in view: from the start while it fits, centred once past the middle.
  const chosen = strip?.find((choice) => choice.checked)?.key
  useEffect(() => {
    const row = picker.current?.querySelector<HTMLElement>('[role="radiogroup"]')
    const on = row?.querySelector<HTMLElement>('[aria-checked="true"]')
    if (!row || !on) return
    const fits = on.offsetLeft + on.offsetWidth < row.clientWidth - 90
    const left = fits ? 0 : Math.max(0, on.offsetLeft - row.clientWidth / 2 + on.offsetWidth / 2)
    row.scrollTo({
      left,
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
    })
  }, [open, chosen])

  function close() {
    setOpen(false)
    head.current?.focus()
  }

  function onBlur(event: FocusEvent<HTMLDivElement>) {
    const to = event.relatedTarget
    if (to instanceof Node && event.currentTarget.contains(to)) return
    setFocused(false)
    // Another control took the focus: the picker closes. A list, popover or dialog it opened
    // keeps it.
    if (
      to instanceof Element &&
      !to.closest('[popover], [role="listbox"], [role="dialog"], [role="menu"]')
    ) {
      setOpen(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.defaultPrevented) return
    const target = event.target as HTMLElement
    if (event.key === 'Escape' && open) {
      event.preventDefault()
      event.stopPropagation()
      close()
      return
    }
    if (event.key === 'Enter' && open && target.matches('[role="radio"], [data-setting-well]')) {
      event.preventDefault()
      close()
      return
    }
    if (target !== head.current || (event.key !== 'ArrowDown' && event.key !== 'ArrowUp')) return
    const scope = head.current.closest('form') ?? document
    const heads = [...scope.querySelectorAll<HTMLElement>('[data-setting-head]')]
    const next = heads[heads.indexOf(head.current) + (event.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    event.preventDefault()
    next.focus()
  }

  function onStripKey(event: KeyboardEvent<HTMLDivElement>) {
    if (!strip) return
    const enabled = strip.filter((choice) => !choice.disabled)
    const index = enabled.findIndex((choice) => choice.checked)
    const target =
      event.key === 'ArrowRight' || event.key === 'ArrowDown'
        ? enabled[Math.min(index + 1, enabled.length - 1)]
        : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
          ? enabled[Math.max(index - 1, 0)]
          : event.key === 'Home'
            ? enabled[0]
            : event.key === 'End'
              ? enabled[enabled.length - 1]
              : undefined
    if (!target) return
    event.preventDefault()
    target.choose()
    const group = event.currentTarget
    requestAnimationFrame(() =>
      group.querySelector<HTMLElement>(`[data-choice="${CSS.escape(target.key)}"]`)?.focus(),
    )
  }

  const isSwitch = props.kind === 'switch'
  const words = props.kind === 'switch' ? (props.stateText ?? ([t.ui.on, t.ui.off] as const)) : null
  const wasId = `${id}-was`

  return (
    <div
      id={anchor}
      data-lit={lit || undefined}
      onFocus={() => setFocused(true)}
      onBlur={onBlur}
      onKeyDown={onKeyDown}
      className={cx(
        'min-w-0 scroll-mt-32 rounded-row transition-[scale,background-color,color,box-shadow] duration-350 ease-nuit',
        'data-flash:shadow-[0_0_0_4px] data-flash:shadow-link/45',
        lit ? 'lit lift [--lift:1.012]' : cx('bg-s1', !disabled && 'hover:bg-s2'),
        disabled && 'opacity-60',
        className,
      )}
    >
      <button
        ref={head}
        type="button"
        data-setting-head
        disabled={disabled}
        role={isSwitch ? 'switch' : undefined}
        aria-checked={props.kind === 'switch' ? props.checked : undefined}
        aria-expanded={isSwitch ? undefined : open}
        aria-controls={!isSwitch && open ? pickerId : undefined}
        aria-labelledby={isSwitch ? labelId : `${labelId} ${valueId}`}
        aria-describedby={describedBy(helpId, errorId, isSwitch && changed ? wasId : undefined)}
        onClick={() => {
          if (props.kind === 'switch') props.onChange(!props.checked)
          else setOpen(!open)
        }}
        className={cx(
          'flex w-full cursor-pointer items-center gap-4 rounded-row py-3.5 pr-[22px] pl-[30px] text-left focus-visible:outline-none disabled:cursor-not-allowed',
          'min-h-[82px] max-md:min-h-[68px] max-md:gap-2.5 max-md:py-3 max-md:pr-3.5 max-md:pl-5',
          props.kind === 'path' && 'max-md:flex-wrap max-md:gap-y-0.5',
        )}
      >
        {changed && (
          <span aria-hidden="true" className="-mr-0.5 size-2.5 shrink-0 rounded-full bg-link" />
        )}
        <span
          id={labelId}
          className={cx(
            'min-w-0 flex-1 text-h3 text-ink',
            props.kind === 'path' && 'max-md:basis-[calc(100%-40px)]',
          )}
        >
          {label}
          {detail !== undefined && (
            <span className="mt-0.5 block text-[16.5px] font-[450] tracking-normal text-ink-3 max-md:text-micro">
              {detail}
            </span>
          )}
        </span>
        <span
          id={valueId}
          className={cx(
            'flex min-w-0 items-baseline justify-end gap-3 text-right text-lead text-ink-2 max-md:gap-2 max-md:text-[17px]',
            props.kind === 'path' &&
              'figures max-md:order-3 max-md:basis-full max-md:justify-start max-md:text-left',
            isSwitch && 'sr-only',
          )}
        >
          {changed && (
            <span id={wasId} className="text-[16px] font-medium text-ink-3 max-md:text-[15px]">
              {t.ui.was(was)}
            </span>
          )}
          <span className={cx('min-w-0 break-words', changed && 'font-[650] text-link')}>
            {display}
          </span>
        </span>
        {words && props.kind === 'switch' ? (
          <>
            <span aria-hidden="true" className="min-w-[2.2em] text-right text-body text-ink-2">
              {props.checked ? words[0] : words[1]}
            </span>
            <SwitchTrack checked={props.checked} />
          </>
        ) : (
          <CaretRightIcon
            size={24}
            aria-hidden="true"
            className={cx(
              'shrink-0 text-ink-3 transition-transform duration-500 ease-nuit',
              open && 'rotate-90',
              props.kind === 'path' && 'max-md:order-2',
            )}
          />
        )}
      </button>
      {error && (
        <div className="px-[30px] pb-5 max-md:px-5 max-md:pb-4">
          <FieldError id={errorId}>{error}</FieldError>
        </div>
      )}
      {children !== undefined && (
        <div className="px-[30px] pb-5 text-small text-ink-2 max-md:px-5 max-md:pb-4">
          {children}
        </div>
      )}
      {(help !== undefined || open) && (
        <div className={lit ? 'px-[30px] pb-[30px] max-md:px-5 max-md:pb-[22px]' : 'sr-only'}>
          {help !== undefined && (
            <p
              id={helpId}
              className="max-w-[66ch] text-[17.5px] leading-[1.58] text-ink-2 max-md:text-[16px]"
            >
              {help}
            </p>
          )}
          {open && (
            <div
              ref={picker}
              id={pickerId}
              className={cx(help !== undefined && 'mt-6 max-md:mt-4')}
            >
              {strip ? (
                <div
                  role="radiogroup"
                  aria-labelledby={labelId}
                  onKeyDown={onStripKey}
                  className="relative -mx-[30px] flex scroll-px-[30px] gap-2 overflow-x-auto px-[30px] pt-2 pb-2.5 scrollbar-none [mask-image:linear-gradient(90deg,transparent_0,#000_30px,#000_calc(100%-90px),transparent)] max-md:-mx-5 max-md:px-5"
                >
                  {strip.map((choice) => (
                    <button
                      key={choice.key}
                      type="button"
                      role="radio"
                      data-choice={choice.key}
                      aria-checked={choice.checked}
                      tabIndex={choice.checked ? 0 : -1}
                      disabled={choice.disabled}
                      onClick={choice.choose}
                      className={cx(
                        'figures h-[60px] min-w-[60px] shrink-0 cursor-pointer rounded-choice bg-s1 px-[18px] text-[21px] font-[620] whitespace-nowrap text-ink',
                        'transition-[scale,background-color,color,box-shadow] duration-450 ease-nuit hover:bg-s2 disabled:cursor-not-allowed disabled:opacity-45',
                        'focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-accent focus-visible:outline-solid',
                        'aria-checked:scale-[1.08] aria-checked:bg-on-focus aria-checked:text-focus aria-checked:shadow-[0_14px_26px_-14px_rgb(28_26_33/0.9)] motion-reduce:aria-checked:scale-100',
                        'max-md:h-[52px] max-md:min-w-[52px] max-md:px-3.5 max-md:text-[19px]',
                      )}
                    >
                      {choice.label}
                    </button>
                  ))}
                </div>
              ) : (
                <Control
                  {...props}
                  labelId={labelId}
                  describedById={describedBy(helpId, errorId)}
                />
              )}
              <p className="mt-3.5 flex flex-wrap gap-x-5 gap-y-1.5 text-micro text-ink-2 max-md:hidden">
                {strip && (
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>←</Kbd>
                    <Kbd>→</Kbd>
                    {props.kind === 'number' ? t.ui.pickerKeys.change : t.ui.pickerKeys.choose}
                  </span>
                )}
                {props.kind !== 'secret' && (
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>Enter</Kbd>
                    {t.ui.pickerKeys.confirm}
                  </span>
                )}
                <span className="inline-flex items-center gap-1.5">
                  <Kbd>Esc</Kbd>
                  {t.ui.pickerKeys.close}
                </span>
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** The value written on the right, the saved one for "was …", and whether they differ. */
function describe<T extends string | number>(
  props: SettingRowProps<T>,
  t: Messages,
): { display: ReactNode; was: string; changed: boolean } {
  switch (props.kind) {
    case 'switch': {
      const [on, off] = props.stateText ?? [t.ui.on, t.ui.off]
      return {
        display: props.checked ? on : off,
        was: props.saved ? on : off,
        changed: props.saved !== undefined && props.saved !== props.checked,
      }
    }
    case 'choice': {
      const name = (value: T | undefined) =>
        props.options.find((option) => option.value === value)?.label ?? String(value ?? '')
      return {
        display: name(props.value),
        was: name(props.saved),
        changed: props.saved !== undefined && props.saved !== props.value,
      }
    }
    case 'number': {
      const write = (value: number | null | undefined) =>
        value === null || value === undefined
          ? ''
          : props.format
            ? props.format(value)
            : props.unit
              ? `${value} ${props.unit}`
              : String(value)
      return {
        display:
          props.value === null ? (
            <span className="text-ink-3">{t.ui.notSet}</span>
          ) : (
            write(props.value)
          ),
        was: write(props.saved),
        changed: props.saved !== undefined && props.saved !== props.value,
      }
    }
    case 'text':
    case 'path': {
      // Empty means the default when the setting has one: its placeholder, dimmed.
      const fallback = props.placeholder || undefined
      const empty = (
        <span className="text-ink-3">
          {fallback === undefined ? (
            t.ui.notSet
          ) : (
            <>
              <span className="sr-only">{t.ui.byDefault} </span>
              {fallback}
            </>
          )}
        </span>
      )
      const emptyWas = fallback === undefined ? t.ui.notSet : `${t.ui.byDefault} ${fallback}`
      return {
        display: props.value === '' ? empty : props.value,
        was: props.saved === '' ? emptyWas : (props.saved ?? ''),
        changed: props.saved !== undefined && props.saved !== props.value,
      }
    }
    case 'secret': {
      const was = props.saved ? t.ui.secret.saved : t.ui.notSet
      const display =
        props.value === undefined
          ? was
          : props.value === ''
            ? props.saved
              ? t.ui.secret.removing
              : t.ui.notSet
            : '••••••••'
      return { display, was, changed: props.value !== undefined }
    }
  }
}

type StripChoice = {
  key: string
  label: string
  checked: boolean
  disabled: boolean
  choose: () => void
}

/** The choices of a strip, or null when the setting opens another control. */
function stripChoices<T extends string | number>(props: SettingRowProps<T>): StripChoice[] | null {
  if (props.kind === 'choice' && props.options.length <= stripMax) {
    return props.options.map((option) => ({
      key: String(option.value),
      label: option.label,
      checked: option.value === props.value,
      disabled: option.disabled ?? false,
      choose: () => props.onChange(option.value),
    }))
  }
  if (props.kind !== 'number') return null
  const step = props.step ?? 1
  const count = Math.floor((props.max - props.min) / step) + 1
  const picker = props.picker ?? (count <= numberStripMax ? 'strip' : 'well')
  if (picker !== 'strip') return null
  return Array.from({ length: count }, (_, index) => {
    const value = props.min + index * step
    return {
      key: String(value),
      label: props.format ? props.format(value) : String(value),
      checked: value === props.value,
      disabled: false,
      choose: () => props.onChange(value),
    }
  })
}

/** The control a row opens when it is not a strip: a well, a list, or the secret field. */
function Control<T extends string | number>({
  labelId,
  describedById,
  ...props
}: SettingRowProps<T> & { labelId: string; describedById: string | undefined }) {
  const invalid = Boolean(props.error)
  switch (props.kind) {
    case 'choice':
      return (
        <Select
          value={props.value}
          options={props.options}
          onValue={props.onChange}
          aria-label={props.label}
          aria-describedby={describedById}
          invalid={invalid}
          className="max-w-[420px]"
        />
      )
    case 'number':
      return (
        <NumberInput
          data-setting-well
          value={props.value}
          onValue={props.onChange}
          min={props.min}
          max={props.max}
          step={props.step}
          suffix={props.unit}
          aria-labelledby={labelId}
          aria-describedby={describedById}
          invalid={invalid}
          className="w-[220px] max-w-full"
        />
      )
    case 'text':
    case 'path':
      return (
        <TextInput
          data-setting-well
          value={props.value}
          onValue={props.onChange}
          placeholder={props.placeholder}
          maxLength={props.maxLength}
          inputMode={props.inputMode}
          mono={props.mono}
          autoComplete="off"
          spellCheck={props.kind === 'path' ? false : undefined}
          aria-labelledby={labelId}
          aria-describedby={describedById}
          invalid={invalid}
          className="max-w-[560px]"
        />
      )
    case 'secret':
      return (
        <SecretField
          label={props.label}
          hideLabel
          saved={props.saved}
          value={props.value}
          onValue={props.onValue}
          reveal={props.reveal}
          replaceable={props.replaceable}
          removable={props.removable}
          status={props.status}
          placeholder={props.placeholder}
          actions={props.actions}
        />
      )
    case 'switch':
      return null
  }
}
