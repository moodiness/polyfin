import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { NavLink } from 'react-router'
import { cx } from './cx'
import { keepCurrentInView } from './scroll'

export type TabItem = {
  /** A stable key; for buttons, the value `onChange` receives. */
  id: string
  label: ReactNode
  /** For navigation tabs: the page this tab opens. */
  to?: string
  /** For navigation tabs: active only on this exact address, not its children. */
  end?: boolean
  /** A count after the label, in mono. */
  count?: number
  /** For button tabs: the id of the panel it shows. */
  panelId?: string
}

export type TabsProps = {
  /** The accessible name of the row of tabs. */
  label: string
  items: readonly TabItem[]
  /**
   * - `pill`: the section tabs above a page (Content, System), on a dark rail.
   * - `underline`: the sub-tabs inside a panel (a source's Summary, Categories…).
   */
  variant?: 'pill' | 'underline'
  /** For button tabs: the selected id. Navigation tabs follow the address instead. */
  value?: string
  /** For button tabs: called with the chosen id. */
  onChange?: (id: string) => void
  className?: string
}

const tabStyles = {
  pill: cx(
    'flex h-[30px] shrink-0 items-center gap-1.5 rounded-[9px] px-3.5 text-control font-medium whitespace-nowrap text-ink-2 transition-[background-color,color] duration-160 ease-nuit hover:text-ink',
    'aria-[current=page]:bg-s3 aria-[current=page]:text-ink aria-[current=page]:shadow-[inset_0_0_0_1px] aria-[current=page]:shadow-line-2',
    'aria-selected:bg-s3 aria-selected:text-ink aria-selected:shadow-[inset_0_0_0_1px] aria-selected:shadow-line-2',
  ),
  underline: cx(
    'relative flex shrink-0 items-center gap-1.5 px-2 pt-2.5 pb-3 text-control font-medium whitespace-nowrap text-ink-3 transition-colors duration-160 ease-nuit hover:text-ink',
    'after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:scale-x-0 after:rounded-full after:bg-accent after:transition-transform after:duration-220 after:ease-nuit',
    'aria-[current=page]:text-ink aria-[current=page]:after:scale-x-100 aria-selected:text-ink aria-selected:after:scale-x-100',
  ),
}

const railStyles = {
  pill: 'inline-flex max-w-full gap-0.5 rounded-row border border-line bg-s1 p-[3px]',
  underline: 'flex gap-1 border-b border-line',
}

/**
 * A row of tabs. With `to` on items it is navigation (links with `aria-current="page"`); without,
 * it is a `tablist` of buttons driven by `value` and `onChange`, moved through with the arrows.
 * On a phone the row scrolls sideways and keeps the current tab in view.
 */
export function Tabs({ label, items, variant = 'pill', value, onChange, className }: TabsProps) {
  const rail = useRef<HTMLDivElement>(null)
  const navigation = items.some((item) => item.to !== undefined)
  // The current tab stays in view when the row scrolls.
  useEffect(() => keepCurrentInView(rail.current))

  const count = (item: TabItem) =>
    item.count !== undefined && (
      <span className="font-mono text-micro text-ink-3">{item.count}</span>
    )

  if (navigation) {
    return (
      <nav aria-label={label} className={cx('max-w-full', className)}>
        <div ref={rail} className={cx(railStyles[variant], 'scrollbar-none overflow-x-auto')}>
          {items.map((item) => (
            <NavLink key={item.id} to={item.to ?? ''} end={item.end} className={tabStyles[variant]}>
              {item.label}
              {count(item)}
            </NavLink>
          ))}
        </div>
      </nav>
    )
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = items.findIndex((item) => item.id === value)
    const next =
      event.key === 'ArrowRight'
        ? (index + 1) % items.length
        : event.key === 'ArrowLeft'
          ? (index - 1 + items.length) % items.length
          : event.key === 'Home'
            ? 0
            : event.key === 'End'
              ? items.length - 1
              : -1
    if (next < 0) return
    event.preventDefault()
    onChange?.(items[next].id)
    requestAnimationFrame(() =>
      rail.current?.querySelector<HTMLButtonElement>(`[data-tab="${items[next].id}"]`)?.focus(),
    )
  }

  return (
    <div
      ref={rail}
      role="tablist"
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cx(railStyles[variant], 'scrollbar-none overflow-x-auto', className)}
    >
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="tab"
          data-tab={item.id}
          aria-selected={item.id === value}
          aria-controls={item.panelId}
          tabIndex={item.id === value ? 0 : -1}
          onClick={() => onChange?.(item.id)}
          className={cx(tabStyles[variant], 'cursor-pointer')}
        >
          {item.label}
          {count(item)}
        </button>
      ))}
    </div>
  )
}
