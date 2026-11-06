import type { ReactNode } from 'react'
import { cx } from './cx'
import { useInPanel } from './Panel'

/**
 * A table drawn as Salon rows: each body row a soft tile, 8 px apart (6 when compact), that
 * scrolls sideways on its own so the page never does. Write plain `<thead>`, `<tbody>`, `<tr>`,
 * `<th>` and `<td>` inside: they are styled from here. Values go right: give their cells
 * `className="figures text-right"`, and `scope="col"` to header cells. The first cell is the
 * label, in ink.
 *
 *   <Table label={t.apiKeys.title}>
 *     <thead><tr><th scope="col">App</th>…</tr></thead>
 *     <tbody>{keys.map((key) => <tr key={key.id}><td>{key.app}</td>…</tr>)}</tbody>
 *   </Table>
 */
export function Table({
  label,
  caption,
  minWidth = 560,
  compact = false,
  children,
  className,
}: {
  /** The accessible name, when no visible caption names the table. */
  label?: string
  /** A visible caption above the table. */
  caption?: ReactNode
  /** Below this width (px) the table scrolls inside itself. */
  minWidth?: number
  /** 56 px rows with smaller words, for long tables (logs, channels). Big rows are 80 px. */
  compact?: boolean
  children: ReactNode
  className?: string
}) {
  const inPanel = useInPanel()
  return (
    <div
      role="region"
      aria-label={label}
      tabIndex={0}
      // `relative`: what is positioned inside (a visually hidden header) scrolls with the table.
      className={cx(
        'relative overflow-x-auto rounded-row focus-visible:outline-offset-4',
        className,
      )}
    >
      <table
        style={{ minWidth }}
        className={cx(
          'w-full border-separate text-left',
          compact ? 'border-spacing-y-1.5 text-body' : 'border-spacing-y-2 text-lead',
          '[&_th]:px-5 [&_th]:pb-1 [&_th]:text-micro [&_th]:font-medium [&_th]:whitespace-nowrap [&_th]:text-ink-3 [&_th:first-child]:pl-7',
          compact
            ? '[&_td]:h-14 [&_td]:py-2 [&_td]:pr-4 [&_td]:pl-4'
            : '[&_td]:h-20 [&_td]:py-3.5 [&_td]:pr-5 [&_td]:pl-5',
          '[&_td]:align-middle [&_td]:text-ink-2 [&_td:first-child]:pl-7 [&_td:first-child]:font-semibold [&_td:first-child]:text-ink',
          compact
            ? '[&_td:first-child]:rounded-l-[18px] [&_td:last-child]:rounded-r-[18px]'
            : '[&_td:first-child]:rounded-l-row [&_td:last-child]:rounded-r-row',
          !inPanel && '[&_tbody_td]:bg-s1',
          '[&_tbody_tr:hover_td]:bg-s2 [&_tbody_td]:transition-colors [&_tbody_td]:duration-300 [&_tbody_td]:ease-nuit',
        )}
      >
        {caption !== undefined && (
          <caption className="pb-2 pl-7 text-left text-small text-ink-2">{caption}</caption>
        )}
        {children}
      </table>
    </div>
  )
}
