import type { ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { cx } from './cx'
import { useInPanel } from './Panel'

/**
 * A big figure on a soft tile, with what it counts under it: "4 of 4 / Categories on". Lay
 * several side by side in a grid (`grid grid-cols-4 gap-2.5 max-md:grid-cols-2`). Inside a Panel
 * it draws no tile of its own (the Bandwidth panel's two figures).
 */
export function FigureTile({
  value,
  total,
  unit,
  label,
  tone,
  className,
}: {
  /** The figure: 4, "12". */
  value: ReactNode
  /** What it is out of, written "of 4" after it. */
  total?: number | string
  /** A unit after the figure instead: "Mbps", "GB". */
  unit?: ReactNode
  /** What the figure counts: "Categories on". */
  label: ReactNode
  /** Colors the figure for a state; the label must still say it. */
  tone?: 'ok' | 'warn' | 'danger'
  className?: string
}) {
  const { t } = useI18n()
  const inPanel = useInPanel()
  const after = total !== undefined ? t.ui.ofTotal(total) : unit
  return (
    <div
      className={cx(
        'min-w-0',
        !inPanel && 'rounded-row bg-s1 px-6 pt-[1.375rem] pb-6 max-md:px-5 max-md:pt-5 max-md:pb-5',
        className,
      )}
    >
      <p
        className={cx(
          'figures text-[2.75rem] leading-none font-[650] tracking-[-0.02em] max-md:text-[2.25rem]',
          tone === 'ok' && 'text-ok',
          tone === 'warn' && 'text-warn',
          tone === 'danger' && 'text-danger',
        )}
      >
        {value}
        {after !== undefined && (
          <span className="ml-1.5 text-[1.1875rem] font-medium tracking-normal text-ink-3 max-md:text-[1.0625rem]">
            {after}
          </span>
        )}
      </p>
      <p className="mt-2.5 text-[1.0625rem] text-ink-2 max-md:text-[1rem]">{label}</p>
    </div>
  )
}
