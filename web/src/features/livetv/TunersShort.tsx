import { StatusPill, Tooltip } from '@/ui'

/**
 * The marker of a recording timer on a tuner's channel when the timers recording at the same time
 * need more tuners than it has, with its reason on hover or focus. Live TV and Schedule show it.
 */
export function TunersShort({ text }: { text: { tunersShort: string; tunersShortHelp: string } }) {
  return (
    <Tooltip content={text.tunersShortHelp}>
      <span tabIndex={0} className="rounded-sm">
        <StatusPill tone="warn">{text.tunersShort}</StatusPill>
      </span>
    </Tooltip>
  )
}
