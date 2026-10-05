import { ArrowLeftIcon, ArrowRightIcon } from '@phosphor-icons/react'
import { useId, type ReactNode } from 'react'
import { Link } from 'react-router'
import { Count } from './Badge'
import { cx } from './cx'

/**
 * The top of a page: an h1 (30 px, 26 on a phone), a lede of one or two sentences, and the
 * page's main actions on the right. `back` adds a link above the title for detail pages.
 */
export function PageHeader({
  title,
  lede,
  actions,
  back,
  className,
}: {
  /** The page title: the same words as its tab or menu entry. */
  title: ReactNode
  /** What the page is for, or its state in one sentence. */
  lede?: ReactNode
  /** The page's main actions (one primary at most). */
  actions?: ReactNode
  /** A link back to the list this page belongs to. */
  back?: { to: string; label: string }
  className?: string
}) {
  return (
    <header className={cx('mb-10 max-md:mb-6', className)}>
      {back && (
        <Link
          to={back.to}
          className="group mb-3 inline-flex items-center gap-1.5 rounded-md text-small font-medium text-ink-3 transition-colors duration-160 hover:text-ink"
        >
          <ArrowLeftIcon
            size={14}
            aria-hidden="true"
            className="transition-transform duration-160 ease-nuit group-hover:-translate-x-0.5"
          />
          {back.label}
        </Link>
      )}
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
        <div className="min-w-0">
          <h1 className="text-h1 text-ink max-md:text-[26px]">{title}</h1>
          {lede !== undefined && (
            <p className="mt-2 max-w-[62ch] text-lead text-ink-2 max-md:text-body">{lede}</p>
          )}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </header>
  )
}

/**
 * A block of a page: a 16 px heading with an optional Count, an aside on the right (a figure or
 * a TextLink), then the content. Blocks are 52 px apart inside PageLayout.
 */
export function Block({
  title,
  count,
  aside,
  children,
  className,
}: {
  title: ReactNode
  /** A number after the heading, in a Count chip. */
  count?: number
  /** On the right of the heading: a TextLink ("Open Health"), a small figure. */
  aside?: ReactNode
  children: ReactNode
  className?: string
}) {
  const id = useId()
  return (
    <section aria-labelledby={id} className={className}>
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
        <h2 id={id} className="flex items-center gap-2.5 text-h3 text-ink">
          {title}
          {count !== undefined && <Count>{count}</Count>}
        </h2>
        {aside !== undefined && (
          <div className="flex items-center gap-2 text-small text-ink-3">{aside}</div>
        )}
      </div>
      {children}
    </section>
  )
}

/**
 * A link in the link color, with an arrow that moves on hover: "Open Health →". For links inside
 * running text, use `className="text-link underline"` on a plain Link instead.
 */
export function TextLink({
  to,
  children,
  arrow = true,
  className,
}: {
  to: string
  children: ReactNode
  /** Shows the arrow after the words. */
  arrow?: boolean
  className?: string
}) {
  return (
    <Link
      to={to}
      className={cx(
        'group inline-flex items-center gap-1.5 rounded-md text-control font-medium text-link transition-colors duration-160 hover:text-link-hover',
        className,
      )}
    >
      {children}
      {arrow && (
        <ArrowRightIcon
          size={16}
          aria-hidden="true"
          className="transition-transform duration-160 ease-nuit group-hover:translate-x-0.5"
        />
      )}
    </Link>
  )
}
