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
        'flex flex-col items-center rounded-panel bg-s1 px-6 py-9 text-center max-sm:px-5 max-sm:py-8',
        className,
      )}
    >
      {Glyph && (
        <span className="mb-3.5 inline-grid size-11 place-items-center rounded-full bg-s2 text-ink-2">
          <Glyph size="1.375rem" aria-hidden="true" />
        </span>
      )}
      <p className="text-group text-ink">{title}</p>
      {children !== undefined && (
        <p className="mt-2 max-w-[56ch] text-small text-ink-2">{children}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
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
      className={cx(
        'block animate-pulse rounded-[0.625rem] bg-s2 motion-reduce:animate-none',
        className,
      )}
    />
  )
}

/** Lines of text loading; the last is shorter. */
export function SkeletonText({ lines = 2, className }: { lines?: number; className?: string }) {
  return (
    <span aria-hidden="true" className={cx('flex flex-col gap-3', className)}>
      {Array.from({ length: lines }, (_, index) => (
        <Skeleton key={index} className={cx('h-4', index === lines - 1 ? 'w-2/5' : 'w-full')} />
      ))}
    </span>
  )
}

/**
 * Rows loading, as a RowList draws them: a bubble, a title and a meta line, each row its own fill
 * 6 px apart, 60 px high (51 in Compact), or 48 with `compact`. The list is announced as loading
 * with `label`.
 */
export function SkeletonRows({
  rows = 3,
  label,
  boxed = true,
  compact = false,
}: {
  rows?: number
  /** Read by screen readers: "Loading sources…". Defaults to the shared "Loading…". */
  label?: string
  /** Each row on its own fill, like RowList's `boxed`; without, the rows sit on the room. */
  boxed?: boolean
  /** The rows of a `compact` RowList. */
  compact?: boolean
}) {
  const { t } = useI18n()
  return (
    <div
      role="status"
      aria-label={label ?? t.common.loading}
      className={cx('flex flex-col', compact ? 'gap-1' : 'gap-1.5')}
    >
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className={cx(
            'flex items-center',
            compact
              ? 'min-h-12 gap-2.5 rounded-[0.625rem] py-1.5'
              : 'min-h-15 gap-3 rounded-row py-2',
            boxed ? (compact ? 'bg-s1 pr-3 pl-3.5' : 'bg-s1 pr-3 pl-4') : 'px-2',
          )}
        >
          <Skeleton className={cx('rounded-full', compact ? 'size-7.5' : 'size-9')} />
          <span className="flex flex-1 flex-col gap-1.5">
            <Skeleton className={cx('w-1/3', compact ? 'h-3.5' : 'h-4')} />
            <Skeleton className="h-3 w-1/2" />
          </span>
          <Skeleton className="h-3.5 w-16" />
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
        'flex flex-wrap items-center gap-3 rounded-row bg-s1 py-2.5 pr-3 pl-4 text-body text-ink',
        className,
      )}
    >
      <span
        aria-hidden="true"
        className="inline-grid size-9 shrink-0 place-items-center rounded-full bg-danger/14 text-danger"
      >
        <WarningCircleIcon size="1.125rem" />
      </span>
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

const noticeTones: Record<NoticeTone, { bubble: string; icon: Icon }> = {
  info: { bubble: 'bg-link/14 text-link', icon: InfoIcon },
  ok: { bubble: 'bg-ok/14 text-ok', icon: CheckCircleIcon },
  warn: { bubble: 'bg-warn/14 text-warn', icon: WarningIcon },
  danger: { bubble: 'bg-danger/14 text-danger', icon: WarningCircleIcon },
}

/**
 * A note inside a page or panel, as a fill with the tone's icon on a tinted bubble: a problem to look at ("8 channels out of 10 have no guide. Open
 * the mapping"), a confirmation, a tip. For a failed load use InlineError, for a passing
 * confirmation a Toast.
 */
export function Notice({
  tone = 'info',
  children,
  action,
  live = false,
  nested = false,
  className,
}: {
  tone?: NoticeTone
  children: ReactNode
  /** A link or button at the end of the text. */
  action?: ReactNode
  /** Announces it when it appears (`role="status"`, or `alert` for danger). */
  live?: boolean
  /** Inside a tile that has the s1 fill already (a row): one fill step up, so it stands out. */
  nested?: boolean
  className?: string
}) {
  const { bubble, icon: Glyph } = noticeTones[tone]
  return (
    <div
      role={live ? (tone === 'danger' ? 'alert' : 'status') : undefined}
      className={cx(
        'flex flex-wrap items-center gap-x-3 gap-y-2 rounded-row py-2.5 pr-3 pl-4 text-body text-ink',
        nested ? 'bg-s2' : 'bg-s1',
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cx(
          'inline-grid size-9 shrink-0 place-items-center self-start rounded-full',
          bubble,
        )}
      >
        <Glyph size="1.125rem" />
      </span>
      <div className="min-w-0 flex-1 basis-48 self-center">{children}</div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
