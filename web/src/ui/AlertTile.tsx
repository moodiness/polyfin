import { InfoIcon, WarningCircleIcon, WarningIcon, type Icon } from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { cx } from './cx'
import { useInPanel } from './Panel'
import { IconTile } from './RowList'

export type AlertTone = 'danger' | 'warn' | 'info'

const tones: Record<
  AlertTone,
  { icon: Icon; bubble: 'danger' | 'warn' | 'neutral'; kind: string }
> = {
  danger: { icon: WarningCircleIcon, bubble: 'danger', kind: 'text-danger' },
  warn: { icon: WarningIcon, bubble: 'warn', kind: 'text-warn' },
  info: { icon: InfoIcon, bubble: 'neutral', kind: 'text-ink-2' },
}

/**
 * A problem to look at, on a tile: a tinted icon bubble, the kind in its color ("Error",
 * "Warning"), the title, what happened, then Cause and Fix (or Meanwhile) and big buttons. The
 * bubble, the kind word and the title say it together, never the color alone.
 *
 *   <AlertTile tone="danger" title="Demo TV: the last programme guide fetch failed."
 *     cause={…} fix={…} actions={<Button icon={ArrowClockwiseIcon}>Try again</Button>} />
 */
export function AlertTile({
  tone,
  kind,
  icon,
  title,
  titleAs: Title = 'h3',
  children,
  cause,
  fix,
  meanwhile,
  actions,
  className,
}: {
  tone: AlertTone
  /** The kind word above the title; "Error", "Warning" or "Note" by default. */
  kind?: string
  /** Another Phosphor icon for the bubble. */
  icon?: Icon
  /** What is wrong, in one sentence. */
  title: ReactNode
  /** Heading level, so pages keep a correct outline. */
  titleAs?: 'h2' | 'h3'
  /** What happened, under the title. */
  children?: ReactNode
  /** Why it happens. */
  cause?: ReactNode
  /** What to do about it. */
  fix?: ReactNode
  /** What keeps working until then. */
  meanwhile?: ReactNode
  /** Buttons under it all: Try again, Open the source. */
  actions?: ReactNode
  className?: string
}) {
  const { t } = useI18n()
  const inPanel = useInPanel()
  const look = tones[tone]
  const facts = (
    [
      [t.ui.cause, cause],
      [t.ui.fix, fix],
      [t.ui.meanwhile, meanwhile],
    ] as const
  ).filter(([, value]) => value !== undefined)
  return (
    <div
      className={cx(
        'grid grid-cols-[3.5rem_minmax(0,1fr)] gap-[1.375rem] max-md:grid-cols-[minmax(0,1fr)] max-md:gap-3.5',
        !inPanel &&
          'rounded-panel bg-s1 py-7 pr-[1.875rem] pl-7 max-md:rounded-[1.5rem] max-md:px-5 max-md:py-[1.375rem]',
        className,
      )}
    >
      <span className="max-md:hidden">
        <IconTile icon={icon ?? look.icon} size="lg" tone={look.bubble} />
      </span>
      <span className="md:hidden">
        <IconTile icon={icon ?? look.icon} size="md" tone={look.bubble} />
      </span>
      <div className="min-w-0">
        <p className={cx('mb-1 text-[1rem] font-semibold', look.kind)}>
          {kind ?? t.ui.alertKind[tone]}
        </p>
        <Title className="text-[1.375rem] leading-[1.3] font-semibold tracking-[-0.005em] text-ink max-md:text-[1.1875rem]">
          {title}
        </Title>
        {children !== undefined && (
          <div className="mt-2 max-w-[60ch] text-[1.125rem] text-ink-2 max-md:text-[1.03125rem]">
            {children}
          </div>
        )}
        {facts.length > 0 && (
          <dl className="mt-3.5 grid grid-cols-[auto_minmax(0,1fr)] gap-x-[1.125rem] gap-y-1.5 text-[1.125rem] max-md:grid-cols-[minmax(0,1fr)] max-md:gap-y-0.5 max-md:text-[1.03125rem]">
            {facts.map(([term, value], index) => (
              <div key={term} className="contents">
                <dt className={cx('text-ink-3', index > 0 && 'max-md:mt-2')}>{term}</dt>
                <dd className="max-w-[58ch] text-ink-2">{value}</dd>
              </div>
            ))}
          </dl>
        )}
        {actions && <div className="mt-[1.375rem] flex flex-wrap gap-3">{actions}</div>}
      </div>
    </div>
  )
}
