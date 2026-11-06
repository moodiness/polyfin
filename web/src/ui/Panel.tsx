import { createContext, use, useId, type ReactNode } from 'react'
import { cx } from './cx'

/**
 * Whether a component sits inside a Panel. Rows, tables and panels inside one drop their own
 * fill, so that no card sits inside a card.
 */
export const PanelContext = createContext(false)

/** True inside a Panel: draw no fill of your own. */
export function useInPanel(): boolean {
  return use(PanelContext)
}

export type PanelProps = {
  /** The heading inside the panel (h2 by default). */
  title?: ReactNode
  /** Heading level, so pages keep a correct outline. */
  titleAs?: 'h2' | 'h3'
  /** Badges beside the title, outside the heading so they stay out of its name. */
  titleAside?: ReactNode
  /** A line under the title. */
  description?: ReactNode
  /** Buttons at the top right (refresh, more actions). */
  actions?: ReactNode
  /** A leading bubble at the top left, such as a source's kind icon (IconTile). */
  media?: ReactNode
  /** The panel's foot, for its own save or cancel (see PanelFooter). */
  footer?: ReactNode
  /** `section` with an accessible name from its title, or a plain `div`. */
  as?: 'section' | 'div'
  /** Removes the padding of the body, for RowList, Table or PanelSection children. */
  flush?: boolean
  children?: ReactNode
  className?: string
}

/**
 * A soft tile (s1 fill, 28 px radius, no border) grouping one subject: a source's details, a
 * figure with its rows, a form. Salon pages are mostly rows and tiles on the room; use a panel
 * when content needs a ground of its own. A Panel inside a Panel, and the rows or table inside
 * one, draw no fill of their own.
 */
export function Panel({
  title,
  titleAs: Title = 'h2',
  titleAside,
  description,
  actions,
  media,
  footer,
  as: Tag = 'section',
  flush = false,
  children,
  className,
}: PanelProps) {
  const nested = useInPanel()
  const hasHead = title !== undefined || actions !== undefined || media !== undefined
  const titleId = useId()
  // Inside another panel it keeps its parts and spacing, without a second ground.
  const pad = nested ? '' : 'px-[1.875rem] max-md:px-5'
  return (
    <PanelContext value>
      <Tag
        aria-labelledby={Tag === 'section' && title !== undefined ? titleId : undefined}
        className={cx(
          'min-w-0',
          !nested && 'rounded-panel bg-s1 py-7 max-md:rounded-[1.5rem] max-md:py-[1.375rem]',
          className,
        )}
      >
        {hasHead && (
          <div className={cx('flex items-start gap-5 max-md:gap-4', pad)}>
            {media}
            <div className="min-w-0 flex-1 self-center">
              {title !== undefined &&
                (titleAside ? (
                  <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                    <Title id={titleId} className="min-w-0 text-group break-words text-ink">
                      {title}
                    </Title>
                    {titleAside}
                  </div>
                ) : (
                  <Title id={titleId} className="text-group break-words text-ink">
                    {title}
                  </Title>
                ))}
              {description !== undefined && (
                <p className="mt-1 text-small text-ink-2">{description}</p>
              )}
            </div>
            {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
          </div>
        )}
        {children !== undefined && (
          <div className={cx(!flush && pad, hasHead && 'mt-6 max-md:mt-5')}>{children}</div>
        )}
        {footer}
      </Tag>
    </PanelContext>
  )
}

/**
 * A part of a Panel with `flush`, apart from the previous one by space alone: the sections of a
 * detail view ("Summary", "What to import").
 */
export function PanelSection({
  title,
  description,
  actions,
  children,
  className,
}: {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <section
      className={cx(
        'px-[1.875rem] py-7 first:pt-0 last:pb-0 max-md:px-5 max-md:py-[1.375rem] max-md:first:pt-0 max-md:last:pb-0',
        className,
      )}
    >
      {(title !== undefined || actions !== undefined) && (
        <div className="mb-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            {title !== undefined && <h3 className="text-h3 text-ink">{title}</h3>}
            {description !== undefined && (
              <p className="mt-1 text-small text-ink-2">{description}</p>
            )}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </div>
      )}
      {children}
    </section>
  )
}

/**
 * The foot of a Panel: a note on the left, buttons on the right, under the one inset line Salon
 * draws.
 */
export function PanelFooter({ note, children }: { note?: ReactNode; children: ReactNode }) {
  return (
    <div className="mt-7 flex flex-wrap items-center gap-3 px-[1.875rem] pt-5 shadow-[inset_0_1px_0] shadow-line-2 max-md:mt-5 max-md:px-5">
      {note !== undefined && <p className="mr-auto text-small text-ink-2">{note}</p>}
      <div className="ml-auto flex flex-wrap items-center gap-3">{children}</div>
    </div>
  )
}
