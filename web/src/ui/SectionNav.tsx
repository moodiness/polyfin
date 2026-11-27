import { CaretRightIcon, MagnifyingGlassIcon, type Icon } from '@phosphor-icons/react'
import { useEffect, useId, useRef } from 'react'
import { Link, NavLink } from 'react-router'
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

/**
 * A heading word above a group of sections ("Account", then its sections). On a phone, where the
 * list is a strip, the heading is read to screen readers only and the groups follow one another.
 */
type SectionNavGroup = {
  id: string
  label: string
  items: readonly SectionNavItem[]
}

export type SectionNavProps = {
  /** The accessible name: "Settings sections". */
  label: string
  /** Sections, groups of sections under a heading, or both. */
  items: readonly (SectionNavItem | SectionNavGroup)[]
  /**
   * For anchors: the id of the section in view. Page links follow the address on their own.
   * `null`: none is current, page links included, while the page shows something else than a
   * section (Settings' search results).
   */
  current?: string | null
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
// phone, where the list is a strip, it stays on one line. The current section is filled and
// bold, with a bar on its leading edge (an underline on the strip) and a chevron; the one under
// the keyboard is lit as a row, its edge.
const linkClass = cx(
  'group flex min-h-9 shrink-0 items-center justify-between gap-2 rounded-[0.5rem] px-3 py-1 text-control leading-snug font-medium text-ink-2 focus-lit-row hover:bg-s1 hover:text-ink focus-visible:text-ink',
  'aria-[current]:font-semibold aria-[current]:text-ink aria-[current]:current-row',
  'max-md:min-h-10 max-md:bg-s1 max-md:px-3 max-md:py-0 max-md:whitespace-nowrap max-md:aria-[current]:current-under',
)

/**
 * The list of a split page's sections on the left (Settings, My account, a user), with an
 * optional search and optional groups under a heading. Place it in the first column of
 * `PageLayout`'s `nav`; it stays in view while the page scrolls. Below 768 px it becomes a strip
 * of words under the title that scrolls sideways, sticky at the top, with the current one
 * underlined and kept in view.
 */
export function SectionNav({ label, items, current, search, className }: SectionNavProps) {
  const row = useRef<HTMLDivElement>(null)
  const id = useId()
  const hintId = `${id}-hint`
  useEffect(() => keepCurrentInView(row.current))
  useScrollFade(row)

  const link = (item: SectionNavItem) => {
    const content = (
      <>
        <span className="flex min-w-0 items-center gap-2">
          {item.icon && (
            <item.icon
              size="1rem"
              aria-hidden="true"
              className="shrink-0 text-ink-3 max-md:hidden"
            />
          )}
          {item.label}
        </span>
        <CaretRightIcon
          size="1rem"
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
    ) : current === null ? (
      <Link key={item.id} to={item.to} className={linkClass}>
        {content}
      </Link>
    ) : (
      <NavLink key={item.id} to={item.to} className={linkClass}>
        {content}
      </NavLink>
    )
  }

  return (
    // On a phone the wrapper steps aside (`contents`), so that the strip sticks along the page.
    <div className={cx('max-md:contents md:sticky md:top-6', className)}>
      {search && (
        <div className="mb-3">
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
          className="flex flex-col gap-0.5 max-md:scroll-fade max-md:scrollbar-none max-md:flex-row max-md:gap-1 max-md:overflow-x-auto max-md:px-5 max-md:py-2"
        >
          {items.map((entry) => {
            if (!('items' in entry)) return link(entry)
            const headingId = `${id}-${entry.id}`
            return (
              // A group adds a heading, then its sections, in the same column (or strip).
              <div
                key={entry.id}
                role="group"
                aria-labelledby={headingId}
                className="flex flex-col gap-0.5 not-first:mt-4 max-md:contents"
              >
                <div
                  id={headingId}
                  className="px-3 pb-1 text-micro font-semibold text-ink-3 max-md:sr-only"
                >
                  {entry.label}
                </div>
                {entry.items.map(link)}
              </div>
            )
          })}
        </div>
      </nav>
    </div>
  )
}
