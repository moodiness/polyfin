import { ArrowLeftIcon, CaretRightIcon } from '@phosphor-icons/react'
import { useId, type ReactNode, type Ref } from 'react'
import { Link } from 'react-router'
import { Count } from './Badge'
import { cx } from './cx'

/**
 * The top of a page: the title (34 px, 29 in Compact, 28 on a phone; 40 for a greeting), a lede
 * of one or two sentences, the page's main actions on the right, and the sub-area words under it
 * all (`nav`, a Tabs). `back` adds a link above the title for detail pages.
 */
export function PageHeader({
  title,
  titleAside,
  lede,
  actions,
  back,
  nav,
  greeting = false,
  className,
}: {
  /** The page title: the same words as its tab or menu entry. */
  title: ReactNode
  /** Badges beside the title, outside the h1 so they stay out of its name. */
  titleAside?: ReactNode
  /** What the page is for, or its state in one sentence. */
  lede?: ReactNode
  /** The page's main actions (one primary at most). */
  actions?: ReactNode
  /** A link back to the list this page belongs to. */
  back?: { to: string; label: string }
  /** The sub-area words under the title and lede: a Tabs of links. */
  nav?: ReactNode
  /** The display size of a greeting ("Good afternoon, Morgan"), for Home. */
  greeting?: boolean
  className?: string
}) {
  const titleClass = cx('min-w-0 hyphenate text-ink', greeting ? 'text-display' : 'text-h1')
  return (
    <header className={cx('mb-7 max-md:mb-6', className)}>
      {back && (
        <Link
          to={back.to}
          className="group focus-lit mb-2 -ml-2.5 inline-flex items-center gap-1.5 rounded-control px-2.5 py-1 text-small font-medium text-ink-2 hover:bg-s1 hover:text-ink"
        >
          <ArrowLeftIcon
            size="1rem"
            aria-hidden="true"
            className="transition-transform duration-300 ease-nuit group-hover:-translate-x-0.5"
          />
          {back.label}
        </Link>
      )}
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          {titleAside ? (
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              <h1 className={titleClass}>{title}</h1>
              {titleAside}
            </div>
          ) : (
            <h1 className={titleClass}>{title}</h1>
          )}
          {lede !== undefined && (
            <p
              className={cx(
                'max-w-[62ch] text-ink-2',
                greeting ? 'mt-1.5 text-[1.1875rem] max-md:text-[1.0625rem]' : 'mt-1.5 text-lead',
              )}
            >
              {lede}
            </p>
          )}
        </div>
        {/* No wider than the page, so that a long row of actions wraps on a phone. */}
        {actions && (
          <div className="flex max-w-full shrink-0 flex-wrap items-center gap-2">{actions}</div>
        )}
      </div>
      {nav !== undefined && <div className="mt-3">{nav}</div>}
    </header>
  )
}

/**
 * A shelf of a page: its title (24 px) with an optional Count, an aside on the right (a TextLink
 * such as "Open the log"), then the content. Shelves are 40 px apart inside PageLayout.
 */
export function Block({
  title,
  count,
  aside,
  children,
  className,
  ref,
}: {
  title: ReactNode
  /** A number after the title, in a Count. */
  count?: number
  /** On the right of the title: a TextLink ("Open Health"), a short figure. */
  aside?: ReactNode
  children: ReactNode
  className?: string
  /** The section, as for useOnScreen. */
  ref?: Ref<HTMLElement>
}) {
  const id = useId()
  return (
    <section ref={ref} aria-labelledby={id} className={className}>
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
        <h2 id={id} className="flex items-center gap-2.5 text-h2 text-ink">
          {title}
          {count !== undefined && <Count>{count}</Count>}
        </h2>
        {aside !== undefined && (
          <div className="-mr-3 flex items-center gap-2 text-small text-ink-2">{aside}</div>
        )}
      </div>
      {children}
    </section>
  )
}

/**
 * A quiet link with a chevron, lit on focus: "Open Health ›", beside a shelf title or under a
 * tile. For links inside running text, use `className="text-link underline"` on a plain Link.
 */
export function TextLink({
  to,
  children,
  arrow = true,
  className,
}: {
  to: string
  children: ReactNode
  /** Shows the chevron after the words. */
  arrow?: boolean
  className?: string
}) {
  return (
    <Link
      to={to}
      className={cx(
        'group focus-lit inline-flex items-center gap-1 rounded-[0.5rem] px-2.5 py-1 text-control font-medium text-ink-2 hover:bg-s1 hover:text-ink',
        className,
      )}
    >
      {children}
      {arrow && (
        <CaretRightIcon
          size="1rem"
          aria-hidden="true"
          className="transition-transform duration-300 ease-nuit group-hover:translate-x-0.5"
        />
      )}
    </Link>
  )
}
