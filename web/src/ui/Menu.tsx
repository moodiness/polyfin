import type { Icon } from '@phosphor-icons/react'
import {
  createContext,
  use,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { Link } from 'react-router'
import { cx } from './cx'

type MenuContextValue = { close: (returnFocus?: boolean) => void }

const MenuContext = createContext<MenuContextValue>({ close: () => {} })

/** The props a Menu gives its trigger button: spread them on a `<button>`. */
export type MenuTriggerProps = {
  ref: (element: HTMLButtonElement | null) => void
  id: string
  type: 'button'
  'aria-haspopup': 'menu'
  'aria-expanded': boolean
  'aria-controls': string
  onClick: () => void
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
}

const itemSelector = '[role^="menuitem"]:not([aria-disabled="true"])'

/**
 * A menu opened from a button: the account menu, a row's "more actions". The trigger is yours:
 *
 *   <Menu label={t.nav.accountMenu} align="end"
 *     trigger={(props) => <button {...props} className="…"><Avatar name={user.name} /></button>}>
 *     <MenuItem icon={UserCircleIcon} to="/me/account">{t.nav.myAccount}</MenuItem>
 *     <MenuSeparator />
 *     <MenuItem icon={SignOutIcon} onSelect={signOut}>{t.nav.signOut}</MenuItem>
 *   </Menu>
 *
 * Keyboard: Enter, Space or ↓ opens it on the first item; ↑ ↓ Home End move; Escape and Tab close
 * it, Escape returning to the button. A click outside closes it. It scrolls with its button,
 * over the page's save bar and beneath the app's top bar.
 */
export function Menu({
  label,
  trigger,
  align = 'start',
  width = 240,
  children,
  className,
}: {
  /** The accessible name of the menu. */
  label: string
  /** Draws the button that opens the menu, from the props to spread on it. */
  trigger: (props: MenuTriggerProps) => ReactNode
  /** Which edge of the button the menu lines up with. */
  align?: 'start' | 'end'
  /** Width in px. */
  width?: number
  /** MenuItem, MenuSeparator, MenuHeader and MenuGroup. */
  children: ReactNode
  className?: string
}) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const button = useRef<HTMLButtonElement | null>(null)
  const list = useRef<HTMLDivElement>(null)
  const root = useRef<HTMLDivElement>(null)

  function items(): HTMLElement[] {
    return Array.from(list.current?.querySelectorAll<HTMLElement>(itemSelector) ?? [])
  }

  function focusItem(which: 'first' | 'last') {
    requestAnimationFrame(() => {
      const all = items()
      all[which === 'first' ? 0 : all.length - 1]?.focus()
    })
  }

  function close(returnFocus = true) {
    setOpen(false)
    if (returnFocus) button.current?.focus()
  }

  // A press outside the menu closes it, without stealing the focus from where it lands.
  useEffect(() => {
    if (!open) return
    function onPointerDown(event: PointerEvent) {
      if (!root.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [open])

  function onListKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const all = items()
    const index = all.indexOf(document.activeElement as HTMLElement)
    let next = -1
    if (event.key === 'ArrowDown') next = (index + 1) % all.length
    else if (event.key === 'ArrowUp') next = (index - 1 + all.length) % all.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = all.length - 1
    else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
      return
    } else if (event.key === 'Tab') {
      setOpen(false)
      return
    }
    if (next >= 0) {
      event.preventDefault()
      all[next]?.focus()
    }
  }

  return (
    <MenuContext value={{ close }}>
      <div ref={root} className={cx('relative', className)}>
        {trigger({
          ref: (element) => {
            button.current = element
          },
          id: `${id}-button`,
          type: 'button',
          'aria-haspopup': 'menu',
          'aria-expanded': open,
          'aria-controls': `${id}-menu`,
          onClick: () => {
            setOpen(!open)
            if (!open) focusItem('first')
          },
          onKeyDown: (event) => {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              setOpen(true)
              focusItem(event.key === 'ArrowDown' ? 'first' : 'last')
            }
          },
        })}
        <div
          ref={list}
          id={`${id}-menu`}
          role="menu"
          aria-label={label}
          hidden={!open}
          onKeyDown={onListKeyDown}
          style={{ width }}
          className={cx(
            // Above the SaveBar (z-20), under the TopBar (z-30).
            'absolute top-[calc(100%+6px)] z-25 rounded-row bg-s2 p-1.5 shadow-pop animate-fade',
            align === 'end' ? 'right-0' : 'left-0',
          )}
        >
          {children}
        </div>
      </div>
    </MenuContext>
  )
}

const itemClass =
  'flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-field px-2.5 text-left text-control text-ink-2 outline-none transition-[background-color,color] duration-160 ease-nuit hover:bg-s3 hover:text-ink focus-visible:bg-s3 focus-visible:text-ink focus-visible:outline-none aria-disabled:cursor-not-allowed aria-disabled:opacity-45'

/**
 * One entry of a Menu: a link with `to` (or `href` for outside the app), or an action with
 * `onSelect`. The menu closes once it is chosen.
 */
export function MenuItem({
  icon: Glyph,
  to,
  href,
  external = false,
  onSelect,
  danger = false,
  disabled = false,
  children,
}: {
  icon?: Icon
  /** A page of the app. */
  to?: string
  /** An address outside the app. */
  href?: string
  /** Opens `href` in a new tab. */
  external?: boolean
  onSelect?: () => void
  /** Red, for removing. */
  danger?: boolean
  disabled?: boolean
  children: ReactNode
}) {
  const { close } = use(MenuContext)
  const content = (
    <>
      {Glyph && <Glyph size={16} aria-hidden="true" className="shrink-0" />}
      <span className="min-w-0 flex-1 truncate">{children}</span>
    </>
  )
  const className = cx(
    itemClass,
    danger && 'text-danger hover:text-danger focus-visible:text-danger',
  )
  if (to !== undefined) {
    return (
      <Link
        to={to}
        role="menuitem"
        tabIndex={-1}
        className={className}
        onClick={() => close(false)}
      >
        {content}
      </Link>
    )
  }
  if (href !== undefined) {
    return (
      <a
        href={href}
        role="menuitem"
        tabIndex={-1}
        className={className}
        target={external ? '_blank' : undefined}
        rel={external ? 'noopener' : undefined}
        onClick={() => close(false)}
      >
        {content}
      </a>
    )
  }
  return (
    <button
      type="button"
      role="menuitem"
      tabIndex={-1}
      aria-disabled={disabled || undefined}
      className={className}
      onClick={() => {
        if (disabled) return
        close()
        onSelect?.()
      }}
    >
      {content}
    </button>
  )
}

/**
 * Exclusive choices inside a Menu, drawn as a segmented control (the language): each option is a
 * `menuitemradio` reached with the arrows like the other items.
 */
export function MenuChoice<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  /** Shown on the left, and the group's accessible name. */
  label: string
  options: readonly { value: T; label: string; lang?: string; title?: string }[]
  value: T
  onChange: (value: T) => void
}) {
  const id = useId()
  return (
    <div
      role="group"
      aria-labelledby={id}
      className="flex h-10 items-center justify-between gap-3 px-2.5"
    >
      <span id={id} className="text-control text-ink-2">
        {label}
      </span>
      <span className="inline-flex rounded-field border border-line-2 bg-bg p-0.5">
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            role="menuitemradio"
            tabIndex={-1}
            lang={option.lang}
            title={option.title}
            aria-checked={option.value === value}
            onClick={() => onChange(option.value)}
            className="h-[26px] cursor-pointer rounded-md px-2.5 text-[12.5px] font-medium text-ink-3 transition-[background-color,color] duration-160 hover:text-ink focus-visible:outline-offset-0 aria-checked:bg-s4 aria-checked:text-ink"
          >
            {option.label}
          </button>
        ))}
      </span>
    </div>
  )
}

/** A hairline between groups of a Menu. */
export function MenuSeparator() {
  return <hr role="separator" className="mx-1 my-1.5 border-0 border-t border-line" />
}

/** The top of a Menu: who is signed in. Not focusable. */
export function MenuHeader({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2.5 px-2.5 pt-2.5 pb-3">{children}</div>
}
