import type { ReactNode } from 'react'
import { cx, ProgressBar, type ProgressVariant } from '@/ui'

export type Fact = { label: string; value: ReactNode }

/** Labelled values in two columns: the facts of a panel (size, version, encoders). */
export function Facts({ items, className }: { items: Fact[]; className?: string }) {
  return (
    <dl className={cx('grid gap-x-6 gap-y-4 sm:grid-cols-2', className)}>
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <dt className="text-small text-ink-3">{item.label}</dt>
          <dd className="mt-1 text-body break-words text-ink tabular-nums">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

/** How full something is: a label, its figure on the right, and a bar under them. */
export function Meter({
  label,
  figure,
  value,
  max,
  variant = 'accent',
  hideBar = false,
}: {
  label: ReactNode
  /** The figure on the right, in mono. */
  figure: ReactNode
  value: number
  max: number
  variant?: ProgressVariant
  hideBar?: boolean
}) {
  return (
    <div>
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <span className="text-small text-ink-2">{label}</span>
        <span className="figures text-small text-ink">{figure}</span>
      </div>
      {!hideBar && (
        <ProgressBar
          value={max > 0 ? value / max : 0}
          label={typeof label === 'string' ? label : ''}
          variant={variant}
          size="lg"
        />
      )}
    </div>
  )
}

/**
 * A part of a page that a link reaches with `#id`: its id, and room above it for the sticky
 * TopBar once the page scrolls to it.
 */
export function Anchor({
  id,
  children,
  className,
}: {
  id: string
  children: ReactNode
  className?: string
}) {
  return (
    <div id={id} className={cx('scroll-mt-24 min-w-0', className)}>
      {children}
    </div>
  )
}
