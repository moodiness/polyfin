import type { Icon } from '@phosphor-icons/react'
import { createContext, isValidElement, use, type ReactNode } from 'react'
import { Link } from 'react-router'
import { cx } from './cx'

/**
 * - `boxed`: rows in one panel, split by hairlines (devices, what to import, an ordered list).
 * - `plain`: rows split by hairlines with no box (recent activity, a log).
 * - `separate`: rows apart, each with its own rounded hover, for a list beside its detail
 *   (sources).
 */
export type RowListVariant = 'boxed' | 'plain' | 'separate'

const VariantContext = createContext<RowListVariant>('boxed')

const listStyles: Record<RowListVariant, string> = {
  boxed: 'overflow-hidden rounded-panel border border-line-2 bg-s1',
  plain: '',
  separate: 'flex flex-col gap-1',
}

/** A list of Rows. It is a `<ul>`; give it an `aria-label` when no heading names it. */
export function RowList({
  variant = 'boxed',
  children,
  className,
  'aria-label': ariaLabel,
}: {
  variant?: RowListVariant
  children: ReactNode
  className?: string
  'aria-label'?: string
}) {
  return (
    <VariantContext value={variant}>
      <ul aria-label={ariaLabel} className={cx(listStyles[variant], className)}>
        {children}
      </ul>
    </VariantContext>
  )
}

const rowStyles: Record<RowListVariant, string> = {
  boxed:
    'px-[18px] py-4 max-sm:px-4 [li:not(:first-child)>&]:border-t [li:not(:first-child)>&]:border-line',
  plain: 'py-3 [li:not(:first-child)>&]:border-t [li:not(:first-child)>&]:border-line',
  separate: 'rounded-row border border-transparent px-3.5 py-3 pl-3',
}

const interactiveStyles: Record<RowListVariant, string> = {
  boxed: 'hover:bg-s2',
  plain: 'hover:bg-s2/60',
  separate: 'hover:border-line hover:bg-s2',
}

export type RowProps = {
  /** A Phosphor icon in a tile, or any node (Avatar, poster, IconTile with letters). */
  leading?: Icon | ReactNode
  /** The name of the row's subject. */
  title: ReactNode
  /** Tags or a state right after the title (Badge). */
  titleAside?: ReactNode
  /** One line under the title: kind, owner, counts. */
  meta?: ReactNode
  /** Content at the end: StatusPill, time, buttons. Buttons here must not sit inside a `to` row. */
  trailing?: ReactNode
  /** Content under the row, aligned with the title (an expanded editor, a nested setting). */
  children?: ReactNode
  /** Makes the whole row a link to this page of the app. */
  to?: string
  /** Makes the whole row a button. */
  onClick?: () => void
  /** The row shown in the detail beside the list (`aria-current`). */
  selected?: boolean
  /** Greys the title, for a source turned off or a service not configured. */
  muted?: boolean
  className?: string
}

/**
 * One row: a leading tile, a title with its meta line, and trailing content. With `to` or
 * `onClick` the whole row is interactive and shows its hover; keep buttons out of such rows.
 */
export function Row({
  leading,
  title,
  titleAside,
  meta,
  trailing,
  children,
  to,
  onClick,
  selected = false,
  muted = false,
  className,
}: RowProps) {
  const variant = use(VariantContext)
  const interactive = to !== undefined || onClick !== undefined
  // A Phosphor icon is a component (a forwardRef object); anything else is drawn as given.
  const lead =
    leading === undefined ||
    leading === null ||
    isValidElement(leading) ||
    (typeof leading !== 'object' && typeof leading !== 'function') ? (
      (leading as ReactNode)
    ) : (
      <IconTile icon={leading as Icon} selected={selected} />
    )
  const body = (
    <>
      {lead}
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className={cx(
              'truncate text-[14.5px] font-medium tracking-[-0.005em]',
              muted ? 'text-ink-2' : 'text-ink',
            )}
          >
            {title}
          </span>
          {titleAside}
        </div>
        {meta !== undefined && (
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 truncate text-[12.5px] text-ink-3">
            {meta}
          </div>
        )}
      </div>
      {trailing !== undefined && (
        <div className="flex shrink-0 flex-col items-end gap-1 text-right max-sm:max-w-[45%]">
          {trailing}
        </div>
      )}
    </>
  )
  const rowClass = cx(
    'flex w-full items-center gap-3.5 text-left transition-[background-color,border-color] duration-160 ease-nuit',
    rowStyles[variant],
    interactive && cx('cursor-pointer', interactiveStyles[variant]),
    selected && 'border-accent/55! bg-s3! shadow-[0_0_0_3px] shadow-accent/8',
    className,
  )
  return (
    <li>
      {to !== undefined ? (
        <Link to={to} aria-current={selected || undefined} className={rowClass}>
          {body}
        </Link>
      ) : onClick !== undefined ? (
        <button
          type="button"
          aria-current={selected || undefined}
          onClick={onClick}
          className={rowClass}
        >
          {body}
        </button>
      ) : (
        <div className={rowClass}>{body}</div>
      )}
      {children !== undefined && <div className="pb-4 pl-[54px] max-sm:pl-0">{children}</div>}
    </li>
  )
}

/**
 * The square tile holding a row's icon (kind of source, device, activity) or letters (a tracking
 * service's monogram). 40 px with a 12 px radius; `sm` is 30 px.
 */
export function IconTile({
  icon: Glyph,
  letters,
  size = 'md',
  selected = false,
  tone = 'neutral',
}: {
  icon?: Icon
  /** One or two letters instead of an icon. */
  letters?: string
  size?: 'md' | 'sm'
  /** Colors the icon with the link color, for the selected row. */
  selected?: boolean
  /** `warn` tints it amber, for a problem. */
  tone?: 'neutral' | 'warn'
}) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        'inline-grid shrink-0 place-items-center border font-semibold tracking-[-0.02em]',
        size === 'md' ? 'size-10 rounded-row text-[15px]' : 'size-[30px] rounded-[9px] text-small',
        tone === 'warn'
          ? 'border-transparent bg-warn/10 text-warn'
          : cx('border-line-2 bg-s3', selected ? 'text-link' : letters ? 'text-ink' : 'text-ink-2'),
      )}
    >
      {Glyph ? <Glyph size={size === 'md' ? 18 : 16} /> : letters}
    </span>
  )
}
