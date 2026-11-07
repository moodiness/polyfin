import { CaretRightIcon, MagnifyingGlassIcon, type Icon } from '@phosphor-icons/react'
import { useEffect, useId, useRef } from 'react'
import { NavLink } from 'react-router'
import { cx } from './cx'
import { TextInput } from './Input'
import { keepCurrentInView, useScrollFade } from './scroll'

export type SectionNavItem = {
  id: string
  label: string
  /** A Phosphor icon before the words, in ink-3; hidden on a phone, where the list is a strip. */
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

// A long label ("Konvertierung (Transkodierung)") wraps inside its fill in the column; on a
// phone, where the list is a strip, it stays on one line. The current section is filled, bold
// and lit as a row, with a chevron; the one under the keyboard is lit as a row too.
const linkClass = cx(
  'group flex min-h-[2.875rem] shrink-0 items-center justify-between gap-2.5 rounded-[0.9375rem] px-4 py-2 text-[1.15625rem] leading-snug font-medium text-ink-2 focus-lit-row hover:bg-s1 hover:text-ink focus-visible:text-ink',
  'aria-[current]:font-semibold aria-[current]:text-ink aria-[current]:lit-row',
  'max-md:min-h-11 max-md:bg-s1 max-md:px-3.5 max-md:py-0 max-md:text-[1.03125rem] max-md:whitespace-nowrap',
)

/**
 * The list of a split page's sections on the left (Settings, My account, a user), with an
 * optional search. Place it in the first column of `PageLayout`'s `nav`; it stays in view while
 * the page scrolls. Below 768 px it becomes a strip of words under the title that scrolls
 * sideways, sticky at the top, with the current one lit and kept in view.
 */
export function SectionNav({ label, items, current, search, className }: SectionNavProps) {
  const row = useRef<HTMLDivElement>(null)
  const hintId = useId()
  useEffect(() => keepCurrentInView(row.current))
  useScrollFade(row)

  return (
    // On a phone the wrapper steps aside (`contents`), so that the strip sticks along the page.
    <div className={cx('max-md:contents md:sticky md:top-8', className)}>
      {search && (
        <div className="mb-[1.125rem] max-md:mb-3.5">
          <TextInput
            type="search"
            size="sm"
            icon={MagnifyingGlassIcon}
            value={search.value}
            onValue={search.onChange}
            placeholder={search.placeholder}
            aria-label={search.placeholder}
            aria-describedby={search.hint ? hintId : undefined}
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
        className="max-md:sticky max-md:top-0 max-md:z-10 max-md:-mx-5 max-md:bg-bg/85 max-md:backdrop-blur-[14px]"
      >
        <div
          ref={row}
          className="flex flex-col gap-0.5 max-md:scroll-fade max-md:scrollbar-none max-md:flex-row max-md:gap-1.5 max-md:overflow-x-auto max-md:px-5 max-md:py-2"
        >
          {items.map((item) => {
            const content = (
              <>
                <span className="flex min-w-0 items-center gap-3">
                  {item.icon && (
                    <item.icon
                      size="1.25rem"
                      aria-hidden="true"
                      className="shrink-0 text-ink-3 max-md:hidden"
                    />
                  )}
                  {item.label}
                </span>
                <CaretRightIcon
                  size="1.25rem"
                  aria-hidden="true"
                  className="shrink-0 opacity-0 group-aria-[current]:opacity-80 max-md:hidden"
                />
              </>
            )
            return item.to.startsWith('#') ? (
              <a
                key={item.id}
                href={item.to}
                aria-current={current === item.id ? 'true' : undefined}
                className={linkClass}
              >
                {content}
              </a>
            ) : (
              <NavLink key={item.id} to={item.to} className={linkClass}>
                {content}
              </NavLink>
            )
          })}
        </div>
      </nav>
    </div>
  )
}
