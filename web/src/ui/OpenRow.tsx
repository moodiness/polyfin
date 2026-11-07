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

/**
 * A row that opens something other than a SettingRow's picker: the
 * label on the left, the value and a chevron on the right, the help shown while the row is lit
 * (always in `aria-describedby`), as a SettingRow draws them.
 * - With `children`, Enter, Space or a click opens them inside the row (a date with its buttons, a
 *   list to choose from); Escape closes them and returns to the row.
 * - With `onClick`, the row is a button for an action shown elsewhere (a dialog).
 * ↑ ↓ on the row move to the next row, SettingRows included, without trapping Tab.
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
  /** What the row does: shown only while it is lit, always read by screen readers. */
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
      onFocus={() => setFocused(true)}
      onBlur={onBlur}
      onKeyDown={onKeyDown}
      className={cx(
        'min-w-0 scroll-mt-32 rounded-row transition-[background-color,box-shadow] duration-350 ease-nuit',
        lit ? 'lit-row' : cx('bg-s1', !disabled && 'hover:bg-s2'),
        disabled && 'opacity-60',
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
          'flex w-full cursor-pointer items-center gap-4 rounded-row py-3.5 pr-[1.375rem] pl-[1.875rem] text-left focus-visible:outline-none disabled:cursor-not-allowed',
          'min-h-[5.125rem] max-md:min-h-[4.25rem] max-md:gap-2.5 max-md:py-3 max-md:pr-3.5 max-md:pl-5',
        )}
      >
        <span id={labelId} className="min-w-0 flex-1 text-h3 text-ink">
          {label}
        </span>
        <span
          id={valueId}
          className="min-w-0 text-right text-lead break-words text-ink-2 max-md:text-[1.0625rem]"
        >
          {value}
        </span>
        <CaretRightIcon
          size="1.5rem"
          aria-hidden="true"
          className={cx(
            'shrink-0 text-ink-3 transition-transform duration-500 ease-nuit',
            open && 'rotate-90',
          )}
        />
      </button>
      {error && (
        <div className="px-[1.875rem] pb-5 max-md:px-5 max-md:pb-4">
          <FieldError id={errorId}>{error}</FieldError>
        </div>
      )}
      {note !== undefined && (
        <div className="px-[1.875rem] pb-5 text-small text-ink-2 max-md:px-5 max-md:pb-4">
          {note}
        </div>
      )}
      {(help !== undefined || open) && (
        <div
          className={
            lit ? 'px-[1.875rem] pb-[1.875rem] max-md:px-5 max-md:pb-[1.375rem]' : 'sr-only'
          }
        >
          {help !== undefined && (
            <p
              id={helpId}
              className="max-w-[66ch] text-[1.09375rem] leading-[1.58] text-ink-2 max-md:text-[1rem]"
            >
              {help}
            </p>
          )}
          {open && (
            <div ref={panel} id={panelId} className={cx(help !== undefined && 'mt-6 max-md:mt-4')}>
              {children}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
