import { MagnifyingGlassIcon, type Icon } from '@phosphor-icons/react'
import { useEffect, useId, useRef } from 'react'
import { NavLink } from 'react-router'
import { cx } from './cx'
import { TextInput } from './Input'
import { keepCurrentInView } from './scroll'

export type SectionNavItem = {
  id: string
  label: string
  /** A Phosphor icon; hidden on a phone, where the list becomes a row of tabs. */
  icon?: Icon
  /** The page of the section (`/settings/content`), or an anchor of this page (`#devices`). */
  to: string
}

export type SectionNavProps = {
  /** The accessible name: "Settings sections". */
  label: string
  items: readonly SectionNavItem[]
  /**
   * For anchors: the id of the section in view. Page links follow the address on their own.
   */
  current?: string
  /** A search field above the list (Settings). It filters what the page shows, not the list. */
  search?: {
    value: string
    onChange: (value: string) => void
    placeholder: string
    /** Explains what the search does, for screen readers. */
    hint?: string
  }
  className?: string
}

const linkClass = cx(
  'flex h-9 shrink-0 items-center gap-3 rounded-field px-2.5 text-body font-[450] whitespace-nowrap text-ink-2 transition-[background-color,color] duration-160 ease-nuit hover:bg-s2 hover:text-ink',
  'aria-[current]:bg-s3 aria-[current]:text-ink aria-[current]:shadow-[inset_0_0_0_1px] aria-[current]:shadow-line',
  'max-md:h-[34px] max-md:border max-md:border-line-2 max-md:px-3.5',
)

/**
 * The vertical list of a split page's sections (Settings, My account), with an optional search.
 * Place it in the first column of `PageLayout`'s `aside`; below 768 px it becomes a sticky row of
 * tabs under the top bar that scrolls sideways.
 */
export function SectionNav({ label, items, current, search, className }: SectionNavProps) {
  const row = useRef<HTMLDivElement>(null)
  const hintId = useId()
  useEffect(() => keepCurrentInView(row.current))

  return (
    // On a phone the wrapper steps aside (`contents`), so that the row sticks along the whole page.
    <div
      className={cx(
        'max-md:contents md:sticky md:top-[calc(var(--spacing-topbar)+32px)]',
        className,
      )}
    >
      {search && (
        <div className="mb-3.5">
          <TextInput
            type="search"
            size="sm"
            icon={MagnifyingGlassIcon}
            value={search.value}
            onValue={search.onChange}
            placeholder={search.placeholder}
            aria-label={search.placeholder}
            aria-describedby={search.hint ? hintId : undefined}
            className="h-9"
          />
          {search.hint && (
            <p id={hintId} className="sr-only">
              {search.hint}
            </p>
          )}
        </div>
      )}
      <nav
        aria-label={label}
        className="max-md:sticky max-md:top-topbar max-md:z-10 max-md:-mx-5 max-md:border-b max-md:border-line max-md:bg-bg/90 max-md:backdrop-blur-[14px]"
      >
        <div
          ref={row}
          className="flex flex-col gap-0.5 max-md:scrollbar-none max-md:flex-row max-md:gap-1.5 max-md:overflow-x-auto max-md:px-5 max-md:py-2.5"
        >
          {items.map((item) => {
            const content = (
              <>
                {item.icon && (
                  <item.icon
                    size={16}
                    aria-hidden="true"
                    className="shrink-0 text-ink-3 transition-colors duration-160 group-aria-[current]:text-link max-md:hidden"
                  />
                )}
                {item.label}
              </>
            )
            return item.to.startsWith('#') ? (
              <a
                key={item.id}
                href={item.to}
                aria-current={current === item.id ? 'true' : undefined}
                className={cx('group', linkClass)}
              >
                {content}
              </a>
            ) : (
              <NavLink key={item.id} to={item.to} className={cx('group', linkClass)}>
                {content}
              </NavLink>
            )
          })}
        </div>
      </nav>
    </div>
  )
}
