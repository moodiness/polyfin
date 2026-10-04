import type { ReactNode } from 'react'
import { Link } from 'react-router'

/** How something fares: the color of a dot or a meter, always paired with words. */
export type Tone = 'ok' | 'warning' | 'error' | 'muted' | 'active'

const dotColors: Record<Tone, string> = {
  ok: 'bg-emerald-400',
  warning: 'bg-amber-400',
  error: 'bg-rose-400',
  muted: 'bg-zinc-600',
  active: 'bg-fin-5',
}

const textColors: Record<Tone, string> = {
  ok: 'text-emerald-300',
  warning: 'text-amber-200',
  error: 'text-rose-300',
  muted: 'text-muted',
  active: 'text-fin-5',
}

/** A colored dot followed by its meaning. */
export function StatusText({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className={`inline-flex items-center gap-2 font-medium ${textColors[tone]}`}>
      <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${dotColors[tone]}`} />
      {children}
    </span>
  )
}

/** A titled block of a dashboard page, with optional controls on its right. */
export function Panel({
  id,
  title,
  description,
  actions,
  children,
  className = '',
}: {
  id?: string
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  const headingId = id ? `${id}-title` : undefined
  return (
    <section
      id={id}
      aria-labelledby={headingId}
      className={`scroll-mt-24 rounded-2xl border border-line bg-surface ${className}`}
    >
      <div className="flex flex-wrap items-start justify-between gap-3 px-5 pt-4 pb-3">
        <div className="min-w-0">
          <h2 id={headingId} className="text-base font-semibold text-white">
            {title}
          </h2>
          {description !== undefined && (
            <p className="mt-1 max-w-prose text-sm text-muted">{description}</p>
          )}
        </div>
        {actions !== undefined && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
      <div className="px-5 pb-5">{children}</div>
    </section>
  )
}

/** A bar showing how full something is; `label` names it for screen readers. */
export function Meter({
  value,
  max,
  label,
  tone = 'active',
}: {
  value: number
  max: number
  label: string
  tone?: Tone
}) {
  const share = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={Math.min(value, max)}
      className="h-1.5 w-full overflow-hidden rounded-full bg-line"
    >
      <div
        className={`h-full origin-left rounded-full transition-transform duration-500 ${dotColors[tone]}`}
        style={{ transform: `scaleX(${share})` }}
      />
    </div>
  )
}

/** A figure of the overview strip, linking to its page when `to` is set. */
export function Stat({
  label,
  value,
  detail,
  tone,
  to,
}: {
  label: string
  value: ReactNode
  detail?: ReactNode
  tone?: Tone
  to?: string
}) {
  const body = (
    <>
      <p className="text-xs font-medium text-muted">{label}</p>
      <p
        className={`mt-1 text-2xl font-semibold tracking-tight tabular-nums ${tone ? textColors[tone] : 'text-white'}`}
      >
        {value}
      </p>
      {detail !== undefined && <p className="mt-0.5 truncate text-xs text-muted">{detail}</p>}
    </>
  )
  const frame = 'block rounded-xl border border-line bg-surface px-4 py-3'
  const linked = `${frame} transition-colors hover:border-fin-4 active:translate-y-px`
  if (to === undefined) return <div className={frame}>{body}</div>
  // An anchor on the same page scrolls to it; another page is opened by the router.
  return to.startsWith('#') ? (
    <a href={to} className={linked}>
      {body}
    </a>
  ) : (
    <Link to={to} className={linked}>
      {body}
    </Link>
  )
}

/** Label and value pairs, two columns from small screens up. */
export function Facts({ items }: { items: { label: string; value: ReactNode }[] }) {
  return (
    <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <dt className="text-xs text-muted">{item.label}</dt>
          <dd className="mt-0.5 break-words text-zinc-100 tabular-nums">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

/** Grey bars in the shape of rows, while a list loads. */
export function Skeleton({ rows = 3, label }: { rows?: number; label: string }) {
  return (
    <div role="status" aria-label={label} className="space-y-3 motion-safe:animate-pulse">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="h-12 rounded-lg bg-surface-2" />
      ))}
    </div>
  )
}

/** What an empty list says, with an optional hint below. */
export function Empty({ children, hint }: { children: ReactNode; hint?: ReactNode }) {
  return (
    <div className="rounded-xl border border-dashed border-line px-4 py-8 text-center">
      <p className="text-sm text-zinc-200">{children}</p>
      {hint !== undefined && <p className="mt-1 text-xs text-muted">{hint}</p>}
    </div>
  )
}
