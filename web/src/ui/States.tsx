import {
  CheckCircleIcon,
  InfoIcon,
  WarningCircleIcon,
  WarningIcon,
  type Icon,
} from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { Button } from './Button'
import { cx } from './cx'

/**
 * What a list shows when it has nothing yet. Say how to fill it, and offer the action that does:
 * "No sources yet. Add a Stremio addon, a music addon or an IPTV account." + "Add a source".
 */
export function EmptyState({
  icon: Glyph,
  title,
  children,
  action,
  className,
}: {
  /** A Phosphor icon for the kind of thing missing. */
  icon?: Icon
  title: ReactNode
  /** How to fill the list. */
  children?: ReactNode
  /** A Button or ButtonLink that fills it. */
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cx(
        'flex flex-col items-center rounded-panel border border-dashed border-line-2 px-6 py-10 text-center',
        className,
      )}
    >
      {Glyph && (
        <span className="mb-4 inline-grid size-10 place-items-center rounded-row border border-line-2 bg-s3 text-ink-2">
          <Glyph size={18} aria-hidden="true" />
        </span>
      )}
      <p className="text-[15px] font-semibold tracking-[-0.01em] text-ink">{title}</p>
      {children !== undefined && (
        <p className="mt-1.5 max-w-[52ch] text-small text-ink-2">{children}</p>
      )}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}

/**
 * A grey placeholder pulsing while data loads. Shape it like the content: `Skeleton` is a block of
 * the given classes (`h-4 w-40`), `SkeletonText` some lines, `SkeletonRows` a list of rows.
 */
export function Skeleton({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cx('block animate-pulse rounded-md bg-s3 motion-reduce:animate-none', className)}
    />
  )
}

/** Lines of text loading; the last is shorter. */
export function SkeletonText({ lines = 2, className }: { lines?: number; className?: string }) {
  return (
    <span aria-hidden="true" className={cx('flex flex-col gap-2.5', className)}>
      {Array.from({ length: lines }, (_, index) => (
        <Skeleton key={index} className={cx('h-3', index === lines - 1 ? 'w-2/5' : 'w-full')} />
      ))}
    </span>
  )
}

/**
 * Rows loading, as a RowList draws them: a tile, a title and a meta line. The list is announced
 * as loading with `label`.
 */
export function SkeletonRows({
  rows = 3,
  label,
  boxed = true,
}: {
  rows?: number
  /** Read by screen readers: "Loading sources…". Defaults to the shared "Loading…". */
  label?: string
  /** Inside a panel, like RowList's `boxed`. */
  boxed?: boolean
}) {
  const { t } = useI18n()
  return (
    <div
      role="status"
      aria-label={label ?? t.common.loading}
      className={cx(boxed && 'rounded-panel border border-line-2 bg-s1')}
    >
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className={cx(
            'flex items-center gap-3.5 py-4',
            boxed && 'px-[18px]',
            index > 0 && 'border-t border-line',
          )}
        >
          <Skeleton className="size-10 rounded-row" />
          <span className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-3.5 w-1/3" />
            <Skeleton className="h-3 w-1/2" />
          </span>
          <Skeleton className="h-3 w-16" />
        </div>
      ))}
    </div>
  )
}

/**
 * A failed load, in place of the content, with a way to try again. Pass the message from
 * `errorMessage(t, query.error)` and `onRetry={() => void query.refetch()}`.
 */
export function InlineError({
  children,
  onRetry,
  retrying = false,
  className,
}: {
  children: ReactNode
  onRetry?: () => void
  /** While the retry runs. */
  retrying?: boolean
  className?: string
}) {
  const { t } = useI18n()
  return (
    <div
      role="alert"
      className={cx(
        'flex flex-wrap items-center gap-3 rounded-row border border-danger/25 bg-danger/8 px-4 py-3 text-control text-ink',
        className,
      )}
    >
      <WarningCircleIcon size={18} aria-hidden="true" className="shrink-0 text-danger" />
      <p className="min-w-0 flex-1">{children}</p>
      {onRetry && (
        <Button size="sm" onClick={onRetry} loading={retrying}>
          {t.common.retry}
        </Button>
      )}
    </div>
  )
}

export type NoticeTone = 'info' | 'ok' | 'warn' | 'danger'

const noticeTones: Record<NoticeTone, { box: string; icon: Icon; color: string }> = {
  info: { box: 'border-line-2 bg-s2', icon: InfoIcon, color: 'text-link' },
  ok: { box: 'border-ok/20 bg-ok/8', icon: CheckCircleIcon, color: 'text-ok' },
  warn: { box: 'border-warn/18 bg-warn/10', icon: WarningIcon, color: 'text-warn' },
  danger: { box: 'border-danger/25 bg-danger/8', icon: WarningCircleIcon, color: 'text-danger' },
}

/**
 * A note inside a page or panel: a problem to look at ("8 channels out of 10 have no guide. Open
 * the mapping"), a confirmation, a tip. For a failed load use InlineError, for a passing
 * confirmation a Toast.
 */
export function Notice({
  tone = 'info',
  children,
  action,
  live = false,
  className,
}: {
  tone?: NoticeTone
  children: ReactNode
  /** A link or button at the end of the text. */
  action?: ReactNode
  /** Announces it when it appears (`role="status"`, or `alert` for danger). */
  live?: boolean
  className?: string
}) {
  const { box, icon: Glyph, color } = noticeTones[tone]
  return (
    <div
      role={live ? (tone === 'danger' ? 'alert' : 'status') : undefined}
      className={cx(
        'flex items-start gap-3 rounded-row border px-3.5 py-3 text-control text-ink',
        box,
        className,
      )}
    >
      <Glyph size={18} aria-hidden="true" className={cx('mt-px shrink-0', color)} />
      <div className="min-w-0 flex-1">{children}</div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
