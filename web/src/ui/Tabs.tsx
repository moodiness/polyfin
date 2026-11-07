import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { NavLink } from 'react-router'
import { cx } from './cx'
import { keepCurrentInView, useScrollFade } from './scroll'

export type TabItem = {
  /** A stable key; for buttons, the value `onChange` receives. */
  id: string
  label: ReactNode
  /** For navigation tabs: the page this tab opens. */
  to?: string
  /** For navigation tabs: active only on this exact address, not its children. */
  end?: boolean
  /** A count after the label. */
  count?: number
  /** For button tabs: the id of the panel it shows. */
  panelId?: string
}

export type TabsProps = {
  /** The accessible name of the row of words. */
  label: string
  items: readonly TabItem[]
  /**
   * - `words`: the sub-area words under a page title (Sources · Libraries · Live TV), 17.5 px.
   * - `underline`: smaller words inside a page (a source's Summary, Categories…), 15.5 px.
   *
   * Both mark the current word with a bar under it, as well as `aria-current` or `aria-selected`.
   */
  variant?: 'words' | 'underline'
  /** For button tabs: the selected id. Navigation tabs follow the address instead. */
  value?: string
  /** For button tabs: called with the chosen id. */
  onChange?: (id: string) => void
  className?: string
}

const tabStyles = {
  words: 'px-2.5 pt-1 pb-1.5 text-h3 max-md:px-2',
  underline: 'px-2.5 pt-1 pb-1.5 text-small font-semibold',
}

// The current word is ink with a bar under it; the others ink-3 until hovered. Focus lights it.
const tabBase = cx(
  'relative flex shrink-0 items-center gap-1.5 rounded-[0.5rem] whitespace-nowrap text-ink-3 focus-lit [--lift:1.04] hover:bg-s1 hover:text-ink',
  'after:absolute after:inset-x-2.5 after:bottom-0.5 after:h-0.5 after:scale-x-0 after:rounded-full after:bg-current after:transition-transform after:duration-300 after:ease-nuit max-md:after:inset-x-2',
  'aria-[current=page]:text-ink aria-[current=page]:after:scale-x-100 aria-selected:text-ink aria-selected:after:scale-x-100',
)

// The words' text lines up with the title above; the padding keeps a lit word's lift in view.
const railStyles = {
  words: '-mx-2.5 px-0.5 max-md:-mx-5 max-md:px-[0.75rem]',
  underline: '-mx-2.5',
}

/**
 * A row of words. With `to` on items it is navigation (links with `aria-current="page"`);
 * without, it is a `tablist` of buttons driven by `value` and `onChange`, moved through with the
 * arrows. On a phone the row scrolls sideways, fades the edge it scrolls past, and keeps the
 * current word in view.
 */
export function Tabs({ label, items, variant = 'words', value, onChange, className }: TabsProps) {
  const rail = useRef<HTMLDivElement>(null)
  const navigation = items.some((item) => item.to !== undefined)
  // The current word stays in view when the row scrolls.
  useEffect(() => keepCurrentInView(rail.current))
  useScrollFade(rail)

  const railClass = cx(
    'flex gap-1 overflow-x-auto py-1 scroll-fade scrollbar-none',
    railStyles[variant],
  )
  const itemClass = cx(tabBase, tabStyles[variant])
  const count = (item: TabItem) =>
    item.count !== undefined && (
      <span className="figures text-micro font-medium text-ink-3">{item.count}</span>
    )

  if (navigation) {
    return (
      <nav aria-label={label} className={cx('max-w-full', className)}>
        <div ref={rail} className={railClass}>
          {items.map((item) => (
            <NavLink key={item.id} to={item.to ?? ''} end={item.end} className={itemClass}>
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
      className={cx(railClass, className)}
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
          className={cx(itemClass, 'cursor-pointer')}
        >
          {item.label}
          {count(item)}
        </button>
      ))}
    </div>
  )
}
