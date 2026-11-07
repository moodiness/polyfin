import { CaretRightIcon, type Icon } from '@phosphor-icons/react'
import { createContext, isValidElement, use, type ReactNode } from 'react'
import { Link } from 'react-router'
import { cx, rem } from './cx'
import { useInPanel } from './Panel'

/**
 * - `boxed`: each row a soft tile, 8 px apart (settings-like rows, devices, an ordered list).
 * - `separate`: the same tiles, 6 px apart, for a narrow list beside its detail (sources), where
 *   the open one is lit.
 * - `plain`: no tiles; a row fills only on hover or focus (recent activity, a log).
 *
 * Inside a Panel every list is drawn `plain`, so no card sits inside a card.
 */
export type RowListVariant = 'boxed' | 'plain' | 'separate'

type ListContextValue = { variant: RowListVariant; compact: boolean }

const ListContext = createContext<ListContextValue>({ variant: 'boxed', compact: false })

const listStyles: Record<RowListVariant, string> = {
  boxed: 'grid grid-cols-[minmax(0,1fr)] gap-1.5',
  separate: 'grid grid-cols-[minmax(0,1fr)] gap-1',
  plain: 'grid grid-cols-[minmax(0,1fr)] gap-0.5',
}

/** A list of Rows. It is a `<ul>`; give it an `aria-label` when no heading names it. */
export function RowList({
  variant = 'boxed',
  compact = false,
  children,
  className,
  'aria-label': ariaLabel,
}: {
  variant?: RowListVariant
  /** 48 px rows (41 in Compact) with smaller words, for long lists (logs, channels, devices). */
  compact?: boolean
  children: ReactNode
  className?: string
  'aria-label'?: string
}) {
  const inPanel = useInPanel()
  const drawn = inPanel ? 'plain' : variant
  return (
    <ListContext value={{ variant: drawn, compact }}>
      <ul
        aria-label={ariaLabel}
        // In a panel the rows' words line up with the panel's, their fill reaching its padding.
        className={cx(listStyles[drawn], inPanel && (compact ? '-mx-3.5' : '-mx-4'), className)}
      >
        {children}
      </ul>
    </ListContext>
  )
}

export type RowProps = {
  /** A Phosphor icon in a bubble, or any node (Avatar, poster, IconTile with letters, a time). */
  leading?: Icon | ReactNode
  /** The row's label: the name of its subject. */
  title: ReactNode
  /** Tags or a state right after the label (Badge). */
  titleAside?: ReactNode
  /** The second line under the label: kind, owner, counts. */
  meta?: ReactNode
  /**
   * The value on the right (a figure, a time, a StatusPill), or buttons. Buttons here must not
   * sit inside a `to` or `onClick` row: give them as `actions`.
   */
  trailing?: ReactNode
  /**
   * Controls at the end of a `to` or `onClick` row, outside its link or button (a switch, move
   * buttons): the row is still lit as one, while its link or one of them has the keyboard.
   */
  actions?: ReactNode
  /** Content under the row inside its tile, aligned with the label (an editor, a nested list). */
  children?: ReactNode
  /** Makes the whole row a link to this page of the app. */
  to?: string
  /** Makes the whole row a button. */
  onClick?: () => void
  /**
   * The chevron after the value. Shown by default on `to` and `onClick` rows; `false` hides it
   * (a row that opens nothing, such as a choice).
   */
  chevron?: boolean
  /** The row shown in the detail beside the list: lit, with `aria-current`. */
  selected?: boolean
  /** Greys the label, for a source turned off or a service not configured. */
  muted?: boolean
  className?: string
}

const tileStyles: Record<RowListVariant, string> = {
  boxed: 'bg-s1',
  separate: 'bg-s1',
  plain: '',
}

/**
 * One Salon row: an optional bubble, the label with its second line, then the value on the right
 * and a chevron. With `to` or `onClick` the whole row is a link or a button that lights up on
 * focus (`aria-current` when `selected`); its own controls go in `actions`, beside the link. A row
 * is 60 px (51 in Compact), 48 in a `compact` list.
 */
export function Row({
  leading,
  title,
  titleAside,
  meta,
  trailing,
  actions,
  children,
  to,
  onClick,
  chevron,
  selected = false,
  muted = false,
  className,
}: RowProps) {
  const { variant, compact } = use(ListContext)
  const interactive = to !== undefined || onClick !== undefined
  const showChevron = chevron ?? interactive
  // A Phosphor icon is a component (a forwardRef object); anything else is drawn as given.
  const lead =
    leading === undefined ||
    leading === null ||
    isValidElement(leading) ||
    (typeof leading !== 'object' && typeof leading !== 'function') ? (
      (leading as ReactNode)
    ) : (
      <IconTile icon={leading as Icon} size={compact ? 'sm' : 'md'} />
    )
  const body = (
    <>
      {lead}
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          <span
            className={cx(
              'min-w-0 hyphenate',
              compact ? 'text-control font-semibold' : 'text-h3',
              muted ? 'text-ink-2' : 'text-ink',
            )}
          >
            {title}
          </span>
          {titleAside}
        </div>
        {meta !== undefined && (
          <div
            className={cx(
              'mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 text-ink-3',
              compact ? 'text-micro' : 'text-small',
            )}
          >
            {meta}
          </div>
        )}
      </div>
      {trailing !== undefined && (
        <div
          className={cx(
            'flex shrink-0 flex-col items-end gap-1 text-right text-ink-2 max-md:max-w-[50%]',
            compact ? 'text-small' : 'text-control',
          )}
        >
          {trailing}
        </div>
      )}
      {showChevron && (
        <CaretRightIcon
          size={rem(compact ? 16 : 18)}
          aria-hidden="true"
          className="-mr-1 shrink-0 text-ink-3"
        />
      )}
    </>
  )
  const lit = cx(
    // Over the tile's s1, the lit fill adds one more s1: one step up, as a hover.
    (interactive || selected) && '[--lit-fill:var(--color-s1)]',
    selected && !interactive && 'lit-row',
  )
  const rowClass = cx(
    'flex w-full items-center text-left',
    compact
      ? 'min-h-12 gap-2.5 rounded-[0.625rem] py-1.5 pl-3.5'
      : 'min-h-15 gap-3 rounded-row py-2 pl-4',
    actions === undefined ? 'pr-3' : 'min-w-0 flex-1 pr-2',
    actions === undefined && lit,
    interactive &&
      (actions === undefined
        ? 'focus-lit-row cursor-pointer not-aria-[current]:hover:bg-s1 aria-[current]:lit-row'
        : 'cursor-pointer focus-visible:outline-none'),
    className,
  )
  const radius = compact ? 'rounded-[0.625rem]' : 'rounded-row'
  const head =
    to !== undefined ? (
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
    )
  return (
    <li className={cx('min-w-0', radius, tileStyles[variant])}>
      {actions === undefined ? (
        head
      ) : (
        <div
          className={cx(
            'flex min-w-0 items-center transition-[background-color,box-shadow] duration-350 ease-nuit',
            radius,
            lit,
            interactive &&
              'not-has-[[aria-current]]:hover:bg-s1 has-[[aria-current]]:lit-row has-[:focus-visible]:lit-row',
          )}
        >
          {head}
          <div className={cx('flex shrink-0 items-center gap-1', compact ? 'pr-2' : 'pr-2.5')}>
            {actions}
          </div>
        </div>
      )}

      {children !== undefined && (
        <div className={cx('pr-3 pb-3', compact ? 'pl-3.5' : 'pl-4')}>{children}</div>
      )}
    </li>
  )
}

/**
 * The round bubble holding a row's icon (kind of source, device, activity) or letters (a tracking
 * service's monogram): 36 px (`md`), 30 (`sm`), 44 (`lg`, an alert) or 64 (`xl`, 56 on a phone: the
 * band of a group's page). A tone tints it at 14 % with the state's color; the state still needs
 * its word beside it.
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
  size?: 'xl' | 'lg' | 'md' | 'sm'
  /** Colors the icon with the link color, for the selected subject. */
  selected?: boolean
  /** Tints the bubble with a state's color: `warn` for a problem, `danger` for an error. */
  tone?: 'neutral' | 'ok' | 'warn' | 'danger'
}) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        'inline-grid shrink-0 place-items-center rounded-full font-bold',
        size === 'xl' && 'size-16 text-[1.5rem] max-md:size-14 max-md:text-[1.25rem]',
        size === 'lg' && 'size-11 text-[1.0625rem]',
        size === 'md' && 'size-9 text-[0.9375rem]',
        size === 'sm' && 'size-7.5 text-[0.8125rem]',
        tone === 'neutral' &&
          cx('bg-s2', selected ? 'text-link' : letters ? 'text-ink' : 'text-ink-2'),
        tone === 'ok' && 'bg-ok/14 text-ok',
        tone === 'warn' && 'bg-warn/14 text-warn',
        tone === 'danger' && 'bg-danger/14 text-danger',
      )}
    >
      {Glyph ? (
        <Glyph
          size={rem(size === 'xl' ? 30 : size === 'lg' ? 22 : size === 'md' ? 18 : 16)}
          className={size === 'xl' ? 'max-md:size-6.5' : undefined}
        />
      ) : (
        letters
      )}
    </span>
  )
}
