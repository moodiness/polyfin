import type { ReactNode } from 'react'
import { cx } from './cx'

/**
 * A data table in a panel that scrolls sideways on its own, so the page never does. Write plain
 * `<thead>`, `<tbody>`, `<tr>`, `<th>` and `<td>` inside: they are styled from here. Give
 * figures `className="figures text-right"`, and `scope="col"` to header cells.
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
  children,
  className,
}: {
  /** The accessible name, when no visible caption names the table. */
  label?: string
  /** A visible caption above the table. */
  caption?: ReactNode
  /** Below this width (px) the table scrolls inside its panel. */
  minWidth?: number
  children: ReactNode
  className?: string
}) {
  return (
    <div
      role="region"
      aria-label={label}
      tabIndex={0}
      className={cx(
        'overflow-x-auto rounded-panel border border-line-2 bg-s1 focus-visible:outline-offset-0',
        className,
      )}
    >
      <table
        style={{ minWidth }}
        className={cx(
          'w-full border-collapse text-left text-body',
          '[&_th]:h-10 [&_th]:px-4 [&_th]:text-[12.5px] [&_th]:font-medium [&_th]:whitespace-nowrap [&_th]:text-ink-3',
          '[&_thead_tr]:border-b [&_thead_tr]:border-line-2',
          '[&_td]:px-4 [&_td]:py-3 [&_td]:align-middle [&_td]:text-ink-2',
          '[&_tbody_tr:not(:first-child)]:border-t [&_tbody_tr:not(:first-child)]:border-line',
          '[&_tbody_tr]:transition-colors [&_tbody_tr]:duration-160 [&_tbody_tr:hover]:bg-s2/60',
        )}
      >
        {caption !== undefined && (
          <caption className="px-4 pt-4 pb-2 text-left text-small text-ink-3">{caption}</caption>
        )}
        {children}
      </table>
    </div>
  )
}
