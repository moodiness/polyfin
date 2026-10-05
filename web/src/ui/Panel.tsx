import { useId, type ReactNode } from 'react'
import { cx } from './cx'

export type PanelProps = {
  /** The heading inside the panel (h2 by default). */
  title?: ReactNode
  /** Heading level, so pages keep a correct outline. */
  titleAs?: 'h2' | 'h3'
  /** A line under the title. */
  description?: ReactNode
  /** Buttons at the top right (refresh, more actions). */
  actions?: ReactNode
  /** A leading tile at the top left, such as a source's kind icon. */
  media?: ReactNode
  /** A bar under the content for the panel's own save or cancel (see PanelFooter). */
  footer?: ReactNode
  /** `section` with an accessible name from its title, or a plain `div`. */
  as?: 'section' | 'div'
  /** Removes the padding of the body, for RowList, Table or PanelSection children. */
  flush?: boolean
  children?: ReactNode
  className?: string
}

/**
 * A surface (s1, 16 px radius, hairline) grouping one subject: a source's details, a form, a list
 * with its title. Pages are mostly made of blocks without panels; use one when content needs an
 * edge (a detail beside a list, a group of rows).
 */
export function Panel({
  title,
  titleAs: Title = 'h2',
  description,
  actions,
  media,
  footer,
  as: Tag = 'section',
  flush = false,
  children,
  className,
}: PanelProps) {
  const hasHead = title !== undefined || actions !== undefined || media !== undefined
  const titleId = useId()
  return (
    <Tag
      aria-labelledby={Tag === 'section' && title !== undefined ? titleId : undefined}
      className={cx('min-w-0 overflow-hidden rounded-panel border border-line-2 bg-s1', className)}
    >
      {hasHead && (
        <div className="flex items-start gap-4 px-6 pt-[22px] max-sm:px-4">
          {media}
          <div className="min-w-0 flex-1">
            {title !== undefined && (
              <Title id={titleId} className="text-h3 text-ink">
                {title}
              </Title>
            )}
            {description !== undefined && (
              <p className="mt-1 text-small text-ink-2">{description}</p>
            )}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
        </div>
      )}
      {children !== undefined && (
        <div className={cx(!flush && 'p-6 max-sm:p-4', !flush && hasHead && 'pt-5')}>
          {children}
        </div>
      )}
      {footer}
    </Tag>
  )
}

/**
 * A part of a Panel with `flush`, separated from the previous one by a hairline. Use it for the
 * sections of a detail view ("Summary", "What to import").
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
    <section className={cx('p-6 not-first:border-t not-first:border-line max-sm:p-4', className)}>
      {(title !== undefined || actions !== undefined) && (
        <div className="mb-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            {title !== undefined && (
              <h3 className="text-[15px] font-semibold tracking-[-0.01em]">{title}</h3>
            )}
            {description !== undefined && (
              <p className="mt-1 text-small text-ink-3">{description}</p>
            )}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
        </div>
      )}
      {children}
    </section>
  )
}

/** The bar at the bottom of a Panel: a note on the left, buttons on the right. */
export function PanelFooter({ note, children }: { note?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-3 border-t border-line bg-bg/50 px-6 py-3.5 max-sm:px-4">
      {note !== undefined && <p className="mr-auto text-small text-ink-3">{note}</p>}
      <div className="ml-auto flex items-center gap-2">{children}</div>
    </div>
  )
}
