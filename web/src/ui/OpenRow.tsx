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
import { cx } from './cx'
import { describedBy, FieldError } from './Field'
import { SettingHelp } from './SettingRow'

/**
 * A row for what no SettingRow kind edits in place: the label on the left with its help as a
 * muted caption under it, the value and a chevron on the right, as a SettingRow draws them.
 * - With `children`, Enter, Space or a click opens them inside the row (a date with its buttons, a
 *   list to choose from); Escape closes them and returns to the row.
 * - With `onClick`, the row is a button for an action shown elsewhere (a dialog).
 * ↑ ↓ on the row move to the next row, SettingRows included, without trapping Tab.
 * A `disabled` row is dimmed as a SettingRow's (`data-inactive`), its More and error excepted.
 */
export function OpenRow({
  anchor,
  label,
  value,
  help,
  error,
  note,
  disabled = false,
  onClick,
  children,
}: {
  anchor?: string
  label: string
  /** What is set now, on the right. */
  value?: ReactNode
  /** What the row does: always shown, as a caption under the label. */
  help?: ReactNode
  /** An error under the row. */
  error?: ReactNode
  /** More under the row, always shown. */
  note?: ReactNode
  disabled?: boolean
  onClick?: () => void
  /** What opens inside the row. */
  children?: ReactNode
}) {
  const id = useId()
  const labelId = `${id}-label`
  const valueId = `${id}-value`
  const helpId = help !== undefined ? `${id}-help` : undefined
  const errorId = error ? `${id}-error` : undefined
  const panelId = `${id}-panel`
  const head = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(false)
  const [open, setOpen] = useState(false)
  const opens = children !== undefined && onClick === undefined
  const lit = focused || open

  // Opening moves the keyboard into what opened: its first field or button.
  useEffect(() => {
    if (open) {
      panel.current
        ?.querySelector<HTMLElement>('input, button:not([disabled]), [tabindex="0"]')
        ?.focus()
    }
  }, [open])

  function onBlur(event: FocusEvent<HTMLDivElement>) {
    const to = event.relatedTarget
    if (to instanceof Node && event.currentTarget.contains(to)) return
    setFocused(false)
    // A list, popover or dialog opened from the row keeps it open.
    if (
      to instanceof Element &&
      !to.closest('[popover], [role="listbox"], [role="dialog"], [role="menu"]')
    ) {
      setOpen(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.defaultPrevented) return
    if (event.key === 'Escape' && open) {
      event.preventDefault()
      event.stopPropagation()
      setOpen(false)
      head.current?.focus()
      return
    }
    if (event.target !== head.current || (event.key !== 'ArrowDown' && event.key !== 'ArrowUp')) {
      return
    }
    const scope = head.current.closest('form') ?? document
    const heads = [...scope.querySelectorAll<HTMLElement>('[data-setting-head]')]
    const next = heads[heads.indexOf(head.current) + (event.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    event.preventDefault()
    next.focus()
  }

  return (
    <div
      id={anchor}
      data-inactive={disabled || undefined}
      onFocus={() => setFocused(true)}
      onBlur={onBlur}
      onKeyDown={onKeyDown}
      className={cx(
        'min-w-0 scroll-mt-32 rounded-row transition-[background-color,box-shadow] duration-350 ease-nuit',
        // An inactive row's fill as it was at 60 % as a whole.
        lit ? 'lit-row' : disabled ? 'bg-s1/60' : 'bg-s1 hover:bg-s2',
      )}
    >
      <button
        ref={head}
        type="button"
        data-setting-head
        disabled={disabled}
        aria-expanded={opens ? open : undefined}
        aria-controls={opens && open ? panelId : undefined}
        aria-labelledby={`${labelId} ${valueId}`}
        aria-describedby={describedBy(helpId, errorId)}
        onClick={() => (onClick ? onClick() : setOpen(!open))}
        className={cx(
          'flex w-full cursor-pointer items-center gap-4 rounded-row pr-3 pl-4 text-left focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60',
          help === undefined ? 'min-h-[3.375rem] py-2' : 'min-h-11 pt-2 pb-0.5',
        )}
      >
        <span id={labelId} className="min-w-0 flex-1 text-h3 hyphenate text-ink">
          {label}
        </span>
        <span id={valueId} className="min-w-0 text-right text-control break-words text-ink-2">
          {value}
        </span>
        <CaretRightIcon
          size="1.125rem"
          aria-hidden="true"
          className={cx(
            'shrink-0 text-ink-3 transition-transform duration-500 ease-nuit',
            open && 'rotate-90',
          )}
        />
      </button>
      {help !== undefined && (
        <div className="pr-12 pb-2.5 pl-4">
          <SettingHelp id={helpId} dimmed={disabled}>
            {help}
          </SettingHelp>
        </div>
      )}
      {error && (
        <div className="px-4 pb-3">
          <FieldError id={errorId}>{error}</FieldError>
        </div>
      )}
      {note !== undefined && (
        <div className={cx('px-4 pb-3 text-small text-ink-2', disabled && 'inactive')}>{note}</div>
      )}
      {open && (
        <div ref={panel} id={panelId} className="px-4 pb-4">
          {children}
        </div>
      )}
    </div>
  )
}
