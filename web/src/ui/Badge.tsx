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
  neutral: 'bg-ink/10 text-ink-2',
  accent: 'bg-link/14 text-link',
  ok: 'bg-ok/14 text-ok',
  warn: 'bg-warn/14 text-warn',
  danger: 'bg-danger/14 text-danger',
}

/**
 * A tag beside a name: a kind ("Stremio", "Xtream"), a role ("Administrator"), a flag
 * ("Default"), as a word on a tinted bubble, with an icon if it helps. Not for states: those use
 * StatusPill.
 */
export function Badge({
  tone = 'neutral',
  icon: Glyph,
  children,
  className,
}: {
  /** Color; `neutral` by default. Colored tones still need words that say the meaning. */
  tone?: BadgeTone
  /** A Phosphor icon before the word. */
  icon?: Icon
  children: ReactNode
  className?: string
}) {
  return (
    <span
      className={cx(
        'inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2.5 text-micro leading-none font-semibold whitespace-nowrap',
        Glyph && 'pl-2',
        badgeTones[tone],
        className,
      )}
    >
      {Glyph && <Glyph size="0.875rem" aria-hidden="true" className="shrink-0" />}
      {children}
    </span>
  )
}

/** A count beside a heading ("Now playing 2"), in tabular figures. */
export function Count({ children, label }: { children: ReactNode; label?: string }) {
  return (
    <span
      aria-label={label}
      className="inline-flex h-6 min-w-6 items-center justify-center rounded-full bg-s2 px-2 text-micro font-semibold text-ink-2 figures"
    >
      {children}
    </span>
  )
}

export type StatusTone = 'ok' | 'warn' | 'danger' | 'muted' | 'live'

const statusTones: Record<
  Exclude<StatusTone, 'live'>,
  { color: string; bubble: string; icon: Icon }
> = {
  ok: { color: 'text-ok', bubble: 'bg-ok/14', icon: CheckCircleIcon },
  warn: { color: 'text-warn', bubble: 'bg-warn/14', icon: WarningIcon },
  danger: { color: 'text-danger', bubble: 'bg-danger/14', icon: WarningCircleIcon },
  muted: { color: 'text-ink-2', bubble: 'bg-ink/10', icon: InfoIcon },
}

/**
 * A state, always as an icon and a word: "Active", "Incomplete guide", "Not configured". The icon
 * sits on a 14 % tinted bubble; on a lit control (a menu's option) the colors darken by
 * themselves. `live` is a green dot for something happening now ("Playing"). It keeps to one
 * line, and wraps its word only where its row is narrower than it, as a long German state on a
 * phone.
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
          'inline-flex max-w-full shrink-0 items-center gap-2 text-small font-semibold text-ink',
          className,
        )}
      >
        <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-ok ring-3 ring-ok/20" />
        <span className="min-w-0 break-words">{children}</span>
      </span>
    )
  }
  const { color, bubble, icon: ToneIcon } = statusTones[tone]
  const Glyph = icon ?? ToneIcon
  return (
    <span
      className={cx(
        'inline-flex max-w-full shrink-0 items-center gap-1.5 text-small font-semibold',
        color,
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cx('inline-grid size-5.5 shrink-0 place-items-center rounded-full', bubble)}
      >
        <Glyph size="0.875rem" />
      </span>
      <span className="min-w-0 break-words">{children}</span>
    </span>
  )
}
