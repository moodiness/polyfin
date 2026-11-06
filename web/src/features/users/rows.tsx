import { ArrowLeftIcon, CaretRightIcon } from '@phosphor-icons/react'
import {
  useEffect,
  useId,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { Link } from 'react-router'
import { cx, describedBy, DockSlotContext, FieldError } from '@/ui'

/**
 * A row of a user's or a group's page that opens something other than a SettingRow's picker: the
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
      data-lit={lit || undefined}
      onFocus={() => setFocused(true)}
      onBlur={onBlur}
      onKeyDown={onKeyDown}
      className={cx(
        'min-w-0 scroll-mt-32 rounded-row transition-[scale,background-color,color,box-shadow] duration-350 ease-nuit',
        lit ? 'lit lift [--lift:1.012]' : cx('bg-s1', !disabled && 'hover:bg-s2'),
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
          'flex w-full cursor-pointer items-center gap-4 rounded-row py-3.5 pr-[22px] pl-[30px] text-left focus-visible:outline-none disabled:cursor-not-allowed',
          'min-h-[82px] max-md:min-h-[68px] max-md:gap-2.5 max-md:py-3 max-md:pr-3.5 max-md:pl-5',
        )}
      >
        <span id={labelId} className="min-w-0 flex-1 text-h3 text-ink">
          {label}
        </span>
        <span
          id={valueId}
          className="min-w-0 text-right text-lead break-words text-ink-2 max-md:text-[17px]"
        >
          {value}
        </span>
        <CaretRightIcon
          size={24}
          aria-hidden="true"
          className={cx(
            'shrink-0 text-ink-3 transition-transform duration-500 ease-nuit',
            open && 'rotate-90',
          )}
        />
      </button>
      {error && (
        <div className="px-[30px] pb-5 max-md:px-5 max-md:pb-4">
          <FieldError id={errorId}>{error}</FieldError>
        </div>
      )}
      {note !== undefined && (
        <div className="px-[30px] pb-5 text-small text-ink-2 max-md:px-5 max-md:pb-4">{note}</div>
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
            <div ref={panel} id={panelId} className={cx(help !== undefined && 'mt-6 max-md:mt-4')}>
              {children}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * One section of a user's or a group's page: its title (25 px), an optional line under it, then
 * its rows 8 px apart. `id` is the target of the section list's anchor.
 */
export function PageSection({
  id,
  title,
  description,
  aside,
  children,
}: {
  id: string
  title: string
  /** What the section is for, or how it saves, under the title. */
  description?: ReactNode
  /** On the right of the title: a button. */
  aside?: ReactNode
  children: ReactNode
}) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="scroll-mt-8 max-md:scroll-mt-20">
      <div className="mb-[18px] flex flex-wrap items-end justify-between gap-x-4 gap-y-3 max-md:mb-3.5">
        <div className="min-w-0">
          <h2 id={`${id}-title`} className="text-group break-words text-ink">
            {title}
          </h2>
          {description !== undefined && (
            <p className="mt-1.5 max-w-[66ch] text-small text-ink-2">{description}</p>
          )}
        </div>
        {aside}
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-2">{children}</div>
    </section>
  )
}

/**
 * The frame of a user's or a group's page: its head (the band), then the section list on the left
 * with the unsaved dock under it, and the sections on the right; on a phone the list is a strip
 * and the dock a bar along the bottom. It is PageLayout's split, with the band for a head.
 */
export function SplitFrame({
  head,
  nav,
  children,
}: {
  head: ReactNode
  nav: ReactNode
  children: ReactNode
}) {
  const [dock, setDock] = useState<HTMLElement | null>(null)
  return (
    <DockSlotContext value={dock}>
      <div className="stagger">
        {head}
        <div className="grid items-start gap-[72px] md:grid-cols-[300px_minmax(0,1fr)] max-xl:gap-12 max-lg:gap-8 max-lg:md:grid-cols-[232px_minmax(0,1fr)] max-md:block">
          {/* As tall as the sections: the list stays in view, the dock at the foot of the column. */}
          <div className="flex flex-col self-stretch max-md:contents">
            {nav}
            <div ref={setDock} className="sticky bottom-7 mt-auto max-md:hidden" />
          </div>
          <div className="min-w-0 max-md:mt-6">{children}</div>
        </div>
      </div>
    </DockSlotContext>
  )
}

/**
 * The head of a person's or a group's page: the link back to the list, then a tile with the
 * avatar or icon, the name (the h1), its facts, what they watch now, and the page's actions. With
 * `art`, the tile takes the colour of that poster, blurred under a scrim.
 */
export function Band({
  back,
  leading,
  title,
  facts,
  extra,
  actions,
  art,
}: {
  back: { to: string; label: string }
  leading: ReactNode
  title: string
  facts: ReactNode
  extra?: ReactNode
  actions?: ReactNode
  art?: string | null
}) {
  const titleId = useId()
  return (
    <header className="mb-12 max-md:mb-8">
      <Link
        to={back.to}
        className="group focus-lit mb-3 -ml-3 inline-flex items-center gap-2 rounded-control px-3 py-1.5 text-small font-medium text-ink-2 hover:bg-s1 hover:text-ink"
      >
        <ArrowLeftIcon
          size={18}
          aria-hidden="true"
          className="transition-transform duration-300 ease-nuit group-hover:-translate-x-0.5"
        />
        {back.label}
      </Link>
      <section
        aria-labelledby={titleId}
        className="relative isolate grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-7 overflow-hidden rounded-tile bg-s1 px-8 py-[30px] max-md:grid-cols-[auto_minmax(0,1fr)] max-md:gap-x-[18px] max-md:gap-y-4 max-md:rounded-panel max-md:px-5 max-md:py-[22px]"
      >
        {art && (
          <>
            <img
              src={art}
              alt=""
              loading="lazy"
              className="absolute -inset-[60px] -z-20 size-[calc(100%+120px)] max-w-none object-cover opacity-55 blur-[60px] brightness-[0.55] saturate-[1.2]"
            />
            <div
              aria-hidden="true"
              className="absolute inset-0 -z-10 bg-linear-to-r from-room/90 from-30% to-room/55"
            />
          </>
        )}
        {leading}
        <div className="min-w-0">
          <h1 id={titleId} className="text-h1 break-words text-ink max-md:text-[34px]">
            {title}
          </h1>
          <div className="mt-2 flex flex-wrap items-center gap-x-[18px] gap-y-1.5 text-[18px] text-ink-2 max-md:text-[16px]">
            {facts}
          </div>
          {extra}
        </div>
        {actions && (
          <div className="flex max-w-full flex-wrap gap-3 self-start max-md:col-span-full">
            {actions}
          </div>
        )}
      </section>
    </header>
  )
}
