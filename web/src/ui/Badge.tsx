import {
  CheckCircleIcon,
  InfoIcon,
  WarningCircleIcon,
  WarningIcon,
  type Icon,
} from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { cx } from './cx'

export type BadgeTone = 'neutral' | 'accent' | 'ok' | 'warn' | 'danger'

const badgeTones: Record<BadgeTone, string> = {
  neutral: 'bg-ink/6 text-ink-2',
  accent: 'bg-accent/15 text-link',
  ok: 'bg-ok/12 text-ok',
  warn: 'bg-warn/12 text-warn',
  danger: 'bg-danger/12 text-danger',
}

/**
 * A small tag beside a name: a kind ("Stremio", "Xtream"), a role ("Administrator"), a flag
 * ("Default"). Not for states: those use StatusPill, which carries an icon.
 */
export function Badge({
  tone = 'neutral',
  children,
  className,
}: {
  /** Color; `neutral` by default. Colored tones still need words that say the meaning. */
  tone?: BadgeTone
  children: ReactNode
  className?: string
}) {
  return (
    <span
      className={cx(
        'inline-flex h-5 shrink-0 items-center rounded-check px-1.5 text-[11.5px] leading-none font-medium tracking-[0.01em] whitespace-nowrap',
        badgeTones[tone],
        className,
      )}
    >
      {children}
    </span>
  )
}

/** A count beside a heading ("Now playing 2"), in mono. */
export function Count({ children, label }: { children: ReactNode; label?: string }) {
  return (
    <span
      aria-label={label}
      className="inline-flex h-5 items-center rounded-md border border-line-2 bg-s3 px-[7px] font-mono text-micro font-medium text-ink-2 tabular-nums"
    >
      {children}
    </span>
  )
}

export type StatusTone = 'ok' | 'warn' | 'danger' | 'muted' | 'live'

const statusTones: Record<Exclude<StatusTone, 'live'>, { color: string; icon: Icon }> = {
  ok: { color: 'text-ok', icon: CheckCircleIcon },
  warn: { color: 'text-warn', icon: WarningIcon },
  danger: { color: 'text-danger', icon: WarningCircleIcon },
  muted: { color: 'text-ink-3', icon: InfoIcon },
}

/**
 * A state, always as an icon and a word: "Active", "Incomplete guide", "Not configured". `live`
 * is a green dot for something happening now ("Playing").
 */
export function StatusPill({
  tone,
  icon,
  children,
  className,
}: {
  /** `ok` green, `warn` amber, `danger` red, `muted` grey, `live` a dot. */
  tone: StatusTone
  /** Replaces the tone's icon. */
  icon?: Icon
  /** The word: never leave it out, color alone carries no meaning. */
  children: ReactNode
  className?: string
}) {
  if (tone === 'live') {
    return (
      <span
        className={cx(
          'inline-flex items-center gap-2 text-small font-medium whitespace-nowrap text-ink',
          className,
        )}
      >
        <span aria-hidden="true" className="size-[7px] rounded-full bg-ok ring-3 ring-ok/18" />
        {children}
      </span>
    )
  }
  const { color, icon: ToneIcon } = statusTones[tone]
  const Glyph = icon ?? ToneIcon
  return (
    <span
      className={cx(
        'inline-flex items-center gap-1.5 text-small font-medium whitespace-nowrap',
        color,
        className,
      )}
    >
      <Glyph size={15} aria-hidden="true" className="shrink-0" />
      {children}
    </span>
  )
}
