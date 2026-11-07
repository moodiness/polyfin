import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from 'react'
import { useI18n, type Messages } from '@/i18n'
import { cx } from './cx'
import { describedBy, FieldError } from './Field'
import { NumberInput, TextInput } from './Input'
import { SecretField, type SecretFieldProps } from './SecretField'
import { segmentClass } from './Segmented'
import { Select, type SelectOption } from './Select'
import { Switch } from './Switch'

/** Up to this many choices sit side by side in the row; more take a list. */
const sideBySideMax = 4
/** Side-by-side choices whose labels pass this many characters together take a list instead. */
const sideBySideChars = 34

type Common = {
  /** The id of the row: the target of its `#anchor`, which a page may flash (`data-flash`). */
  anchor?: string
  /** The setting's name, on the left. */
  label: string
  /** A second line under the label. */
  detail?: ReactNode
  /**
   * What the setting does: always shown, as a muted caption under the name, and read by screen
   * readers with the control (`aria-describedby`). Range and default belong here ("From 0 to 32.
   * 0 by default.").
   */
  help?: ReactNode
  /** The error of this setting, from validation or the server; shown under the row. */
  error?: ReactNode
  /** More under the row, always shown: a warning, a note. */
  children?: ReactNode
  disabled?: boolean
  /** Inside a tile that has the s1 fill already: one fill step up until lit, so it stands out. */
  nested?: boolean
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
  /**
   * The choices in order: side by side in the row up to 4 short ones, else a list that opens in
   * one click, with a search past 12.
   */
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
  /** The unit in the well, after the number: "min", "GB". */
  unit?: string
  /** Writes a value in "was …": `(n) => n === 0 ? 'No limit' : String(n)`. */
  format?: (value: number) => string
  saved?: number | null
}

export type TextSetting = {
  /** `path` is a wider well, in tabular figures. */
  kind: 'text' | 'path'
  value: string
  onChange: (value: string) => void
  saved?: string
  /**
   * The value used while the setting is empty (a default folder or address): shown in the empty
   * well, and in "was …" as "Default: …".
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
 * One setting as a Salon row, edited in the row itself: the name on the left with its help as a
 * muted caption under it, the control on the right. Nothing opens below the row.
 * - `switch`: the switch alone toggles (`role="switch"`, with its word); a click elsewhere on
 *   the row moves the focus to it, and never toggles.
 * - `choice`: up to 4 short choices side by side (`role="radiogroup"`, ← → choose); more, or
 *   longer ones, a list that opens in one click, with a search past 12.
 * - `number`: a number well with its unit: typed in place, ↑ ↓ step.
 * - `text` and `path`: a text well; each keystroke is a change for the dock, no Enter needed.
 * - `secret`: the SecretField under the name (masked typing, reveal, replace, remove).
 *
 * ↑ ↓ move to the previous or next row's control (`data-setting-head`), except in a number well,
 * where they step, and in an open list. A click on the name or the caption focuses the control.
 * With `saved`, a changed row keeps a dot, "was 0" and its value in the link color. Rows go 6 px
 * apart (`grid grid-cols-[minmax(0,1fr)] gap-1.5`).
 *
 * A `disabled` row is an inactive control (`data-inactive`), dimmed as WCAG allows for one: its
 * fill, and the inks of its name, help and children (`inactive`), its control as a whole. Its
 * More, links and buttons, which still work, and its error keep their full contrast.
 */
export function SettingRow<T extends string | number = string>(props: SettingRowProps<T>) {
  const {
    anchor,
    label,
    detail,
    help,
    error,
    children,
    disabled = false,
    nested = false,
    className,
  } = props
  const { t } = useI18n()
  const id = useId()
  const labelId = `${id}-label`
  const helpId = help !== undefined ? `${id}-help` : undefined
  const errorId = error ? `${id}-error` : undefined
  const wasId = `${id}-was`
  const row = useRef<HTMLDivElement>(null)

  const { was, changed } = describe(props, t)
  const describedById = describedBy(helpId, errorId, changed ? wasId : undefined)
  const below = props.kind === 'secret'

  // ↑ ↓ from a closed list's button move between rows, before the list would open on them.
  function onKeyDownCapture(event: KeyboardEvent<HTMLDivElement>) {
    const target = event.target as HTMLElement
    if (target.matches('button[role="combobox"][aria-expanded="false"]')) moveToRow(event)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const target = event.target as HTMLElement
    // A number well steps with the arrows, and an open list moves through its options.
    if (target.matches('input[type="number"], [aria-expanded="true"]')) return
    moveToRow(event)
  }

  function moveToRow(event: KeyboardEvent<HTMLDivElement>) {
    if (event.defaultPrevented || event.altKey) return
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    // The row's own control: the switch, the chosen choice, the list's button or the well.
    const head = row.current?.querySelector<HTMLElement>('[data-setting-head]')
    if (!head) return
    const scope = head.closest('form') ?? document
    const heads = [...scope.querySelectorAll<HTMLElement>('[data-setting-head]:not(:disabled)')]
    const next = heads[heads.indexOf(head) + (event.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    event.preventDefault()
    event.stopPropagation()
    next.focus()
  }

  // A click on the name, the caption or the row's padding focuses its control.
  function onClick(event: MouseEvent<HTMLDivElement>) {
    if ((event.target as Element).closest('button, input, a, [role="listbox"], [popover]')) return
    if (window.getSelection()?.isCollapsed === false) return
    row.current?.querySelector<HTMLElement>('[data-setting-head]')?.focus()
  }

  const mark = changed && (
    <span
      aria-hidden="true"
      className={cx(
        'absolute top-1/2 left-1.5 size-1.5 -translate-y-1/2 rounded-full bg-link',
        disabled && 'opacity-60',
      )}
    />
  )

  return (
    <div
      ref={row}
      id={anchor}
      data-inactive={disabled || undefined}
      onKeyDownCapture={onKeyDownCapture}
      onKeyDown={onKeyDown}
      className={cx(
        'relative min-w-0 scroll-mt-32 rounded-row transition-[background-color,box-shadow] duration-350 ease-nuit',
        'data-flash:shadow-[0_0_0_4px] data-flash:shadow-link/45',
        // The fill an inactive row had as a whole at 60 %.
        nested
          ? cx('[--lit-fill:var(--color-s3)]', disabled ? 'bg-s2/60' : 'bg-s2 hover:bg-s4')
          : disabled
            ? 'bg-s1/60'
            : 'bg-s1 hover:bg-s2',
        // Lit while the keyboard or the mouse is in its control, or its list is open.
        'focus-within:lit-row has-[[aria-expanded=true]]:lit-row',
        className,
      )}
    >
      {mark}
      <div
        onClick={onClick}
        className={cx(
          'flex min-h-[3.375rem] flex-wrap items-center gap-x-5 gap-y-2 py-2 pr-3 pl-4',
          disabled ? 'cursor-not-allowed' : 'cursor-default',
        )}
      >
        <div className="min-w-0 flex-[1_1_14rem]">
          <span
            id={labelId}
            className={cx('block text-h3 hyphenate text-ink', disabled && 'inactive')}
          >
            {label}
          </span>
          {detail !== undefined && (
            <span className={cx('block text-small text-ink-2', disabled && 'inactive')}>
              {detail}
            </span>
          )}
          {help !== undefined && (
            <SettingHelp id={helpId} dimmed={disabled}>
              {help}
            </SettingHelp>
          )}
        </div>
        <div
          className={cx(
            'flex max-w-full min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-1',
            below && 'basis-full justify-start',
            // A changed value is written in the link color, as the dot.
            changed && '[&_[role=combobox]]:text-link [&_input]:text-link',
            disabled && 'opacity-60',
          )}
        >
          {changed && (
            <span id={wasId} className="min-w-0 truncate text-micro text-ink-3 sm:max-w-[16rem]">
              {t.ui.was(was)}
            </span>
          )}
          <Control
            {...props}
            labelId={labelId}
            describedById={describedById}
            invalid={Boolean(error)}
          />
        </div>
      </div>
      {error && (
        <div className="px-4 pb-3">
          <FieldError id={errorId}>{error}</FieldError>
        </div>
      )}
      {children !== undefined && (
        <div className={cx('px-4 pb-3 text-small text-ink-2', disabled && 'inactive')}>
          {children}
        </div>
      )}
    </div>
  )
}

/**
 * A setting's help under its name (a SettingRow's, an OpenRow's): a muted caption, two lines at
 * most, with More at the end of a longer one to read the rest. Screen readers read it whole. A
 * list row's caption (a scheduled task's description) takes `lines={1}`. `dimmed`, in an
 * inactive row, dims the caption but never More.
 */
export function SettingHelp({
  id,
  lines = 2,
  dimmed = false,
  children,
}: {
  id: string | undefined
  lines?: 1 | 2
  dimmed?: boolean
  children: ReactNode
}) {
  const { t } = useI18n()
  const text = useRef<HTMLParagraphElement>(null)
  const [open, setOpen] = useState(false)
  const [long, setLong] = useState(false)

  // More shows only while the clamp cuts the caption, which a narrower window may do.
  useLayoutEffect(() => {
    const node = text.current
    if (!node || open) return
    const measure = () => setLong(node.scrollHeight > node.clientHeight + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [open])

  return (
    <div
      className={cx(
        'mt-0.5 flex max-w-[72ch] items-end gap-2 text-small text-ink-3',
        dimmed && 'inactive',
      )}
    >
      <p
        id={id}
        ref={text}
        className={cx('min-w-0', !open && (lines === 1 ? 'line-clamp-1' : 'line-clamp-2'))}
      >
        {children}
      </p>
      {(long || open) && (
        <button
          type="button"
          aria-expanded={open}
          aria-controls={id}
          onClick={() => setOpen(!open)}
          className="-mx-1 shrink-0 cursor-pointer rounded-[0.375rem] px-1 font-semibold whitespace-nowrap text-ink-2 focus-lit hover:text-ink"
        >
          {open ? t.ui.lessHelp : t.ui.moreHelp}
        </button>
      )}
    </div>
  )
}

/** The saved value for "was …", and whether the value differs from it. */
function describe<T extends string | number>(
  props: SettingRowProps<T>,
  t: Messages,
): { was: string; changed: boolean } {
  switch (props.kind) {
    case 'switch': {
      const [on, off] = props.stateText ?? [t.ui.on, t.ui.off]
      return {
        was: props.saved ? on : off,
        changed: props.saved !== undefined && props.saved !== props.checked,
      }
    }
    case 'choice':
      return {
        was:
          props.options.find((option) => option.value === props.saved)?.label ??
          String(props.saved ?? ''),
        changed: props.saved !== undefined && props.saved !== props.value,
      }
    case 'number': {
      const saved = props.saved
      return {
        was:
          saved === null || saved === undefined
            ? t.ui.notSet
            : props.format
              ? props.format(saved)
              : props.unit
                ? `${saved} ${props.unit}`
                : String(saved),
        changed: saved !== undefined && saved !== props.value,
      }
    }
    case 'text':
    case 'path': {
      // Empty means the default when the setting has one.
      const fallback = props.placeholder || undefined
      return {
        was:
          props.saved !== ''
            ? (props.saved ?? '')
            : fallback === undefined
              ? t.ui.notSet
              : `${t.ui.byDefault} ${fallback}`,
        changed: props.saved !== undefined && props.saved !== props.value,
      }
    }
    case 'secret':
      return {
        was: props.saved ? t.ui.secret.saved : t.ui.notSet,
        changed: props.value !== undefined,
      }
  }
}

/** Whether a choice's options sit side by side in the row, rather than in a list. */
function sideBySide(options: readonly SelectOption<string | number>[]): boolean {
  if (options.length < 2 || options.length > sideBySideMax) return false
  return options.reduce((sum, option) => sum + option.label.length, 0) <= sideBySideChars
}

/** The control of a row, in the row: a switch, choices, a list, a well or the secret field. */
function Control<T extends string | number>({
  labelId,
  describedById,
  invalid,
  ...props
}: SettingRowProps<T> & {
  labelId: string
  describedById: string | undefined
  invalid: boolean
}) {
  const disabled = props.disabled ?? false
  switch (props.kind) {
    case 'switch':
      return (
        <Switch
          data-setting-head
          checked={props.checked}
          onChange={props.onChange}
          labelledBy={labelId}
          describedById={describedById}
          stateText={props.stateText ?? true}
          disabled={disabled}
          // A comfortable target: 12 px around the switch take the click too.
          className="relative after:absolute after:-inset-3"
        />
      )
    case 'choice':
      return sideBySide(props.options) ? (
        <Choices
          value={props.value}
          options={props.options}
          onChange={props.onChange}
          labelId={labelId}
          describedById={describedById}
          disabled={disabled}
        />
      ) : (
        <Select
          data-setting-head
          value={props.value}
          options={props.options}
          onValue={props.onChange}
          aria-label={props.label}
          aria-describedby={describedById}
          invalid={invalid}
          disabled={disabled}
          className="w-[16.5rem] max-w-full"
        />
      )
    case 'number':
      return (
        <NumberInput
          data-setting-head
          value={props.value}
          onValue={props.onChange}
          min={props.min}
          max={props.max}
          step={props.step}
          suffix={props.unit}
          disabled={disabled}
          aria-labelledby={labelId}
          aria-describedby={describedById}
          invalid={invalid}
          className="w-[9rem]"
        />
      )
    case 'text':
    case 'path':
      return (
        <TextInput
          data-setting-head
          value={props.value}
          onValue={props.onChange}
          placeholder={props.placeholder}
          maxLength={props.maxLength}
          inputMode={props.inputMode}
          mono={props.mono}
          autoComplete="off"
          spellCheck={props.kind === 'path' ? false : undefined}
          disabled={disabled}
          aria-labelledby={labelId}
          aria-describedby={describedById}
          invalid={invalid}
          className={cx(
            'max-w-full',
            props.kind === 'path' ? 'w-[30rem] [&_input]:figures' : 'w-[22rem]',
          )}
        />
      )
    case 'secret':
      return (
        <SecretField
          label={props.label}
          hideLabel
          size="md"
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
  }
}

/**
 * Two to four choices side by side, a radio group: one click chooses; ← → (and Home, End) move
 * and choose, ↑ ↓ are the rows'. The chosen one is the row's control (`data-setting-head`).
 */
function Choices<T extends string | number>({
  value,
  options,
  onChange,
  labelId,
  describedById,
  disabled,
}: {
  value: T
  options: readonly SelectOption<T>[]
  onChange: (value: T) => void
  labelId: string
  describedById: string | undefined
  disabled: boolean
}) {
  const enabled = options.filter((option) => !option.disabled)
  // The chosen one takes the Tab stop, else the first that may be chosen.
  const stop = enabled.find((option) => option.value === value) ?? enabled[0]

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = enabled.findIndex((option) => option.value === value)
    const target =
      event.key === 'ArrowRight'
        ? enabled[Math.min(index + 1, enabled.length - 1)]
        : event.key === 'ArrowLeft'
          ? enabled[Math.max(index - 1, 0)]
          : event.key === 'Home'
            ? enabled[0]
            : event.key === 'End'
              ? enabled[enabled.length - 1]
              : undefined
    if (!target) return
    event.preventDefault()
    if (target.value !== value) onChange(target.value)
    const group = event.currentTarget
    requestAnimationFrame(() =>
      group
        .querySelector<HTMLElement>(`[data-choice="${CSS.escape(String(target.value))}"]`)
        ?.focus(),
    )
  }

  return (
    <div
      role="radiogroup"
      aria-labelledby={labelId}
      aria-describedby={describedById}
      onKeyDown={onKeyDown}
      className="inline-flex max-w-full flex-wrap justify-end gap-1"
    >
      {options.map((option) => (
        <button
          key={String(option.value)}
          type="button"
          role="radio"
          lang={option.lang}
          data-choice={String(option.value)}
          data-setting-head={option === stop || undefined}
          aria-checked={option.value === value}
          tabIndex={option === stop ? 0 : -1}
          disabled={disabled || option.disabled}
          onClick={() => {
            if (option.value !== value) onChange(option.value)
          }}
          className={segmentClass('md')}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
