import type { ReactNode } from 'react'
import { Link, type To } from 'react-router'
import { CaretRightIcon, type Icon } from '@phosphor-icons/react'
import { cx, IconTile } from '@/ui'
import { detailOpened } from './healthDetails'

/**
 * A check of Health on a tile, as a link to its details: an icon, its state as an icon and a word
 * (StatusPill), the kind, the name or figure, a line on it, and a foot with a chevron. The tile is
 * lit as a row on focus. In a `CheckGrid`, `wide` tiles take two columns.
 */
export function CheckTile({
  to,
  state,
  icon,
  kind,
  title,
  children,
  foot,
  wide = false,
}: {
  to: To
  /** The state: a StatusPill, always an icon and a word. */
  state: ReactNode
  icon: Icon
  /** What is checked: "Addons", "Database". */
  kind: string
  /** Its name or its figure: "Demo Catalog", "563 MB of 10 GB". */
  title: ReactNode
  /** A line or two under it, and a meter. */
  children?: ReactNode
  /** What the link leads to, or one more fact. */
  foot: ReactNode
  wide?: boolean
}) {
  return (
    <Link
      to={to}
      preventScrollReset
      // A part of Health opens over the page, and closing it goes back.
      state={typeof to === 'object' && to.pathname === undefined ? detailOpened : undefined}
      className={cx(
        'focus-lit-row grid min-h-[9.25rem] min-w-0 grid-rows-[auto_1fr_auto] gap-2.5 rounded-panel bg-s1 px-4 py-3.5 hover:bg-s2 max-md:min-h-0 max-md:rounded-[1.125rem] max-md:p-3.5',
        wide && 'xl:col-span-2',
        // Two columns: an odd last tile takes the row, so that none ends with a hole.
        'md:max-xl:[&:last-child:nth-child(odd)]:col-span-2',
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <IconTile icon={icon} size="sm" />
        {state}
      </div>
      <div className="min-w-0">
        <p className="text-small text-ink-3 max-md:text-micro">{kind}</p>
        <h3 className="text-[1.25rem] leading-[1.2] font-[650] tracking-[-0.01em] break-words text-ink max-md:text-[1.125rem]">
          {title}
        </h3>
        {children !== undefined && <div className="mt-1 text-small text-ink-2">{children}</div>}
      </div>
      <div className="flex items-center justify-between gap-2 text-small text-ink-3 max-md:text-micro">
        <span className="min-w-0 break-words">{foot}</span>
        <CaretRightIcon size="1rem" aria-hidden="true" className="shrink-0" />
      </div>
    </Link>
  )
}
