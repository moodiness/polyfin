import { CircleNotchIcon } from '@phosphor-icons/react'
import { cx } from './cx'

/**
 * A small turning circle for an action under way, inside a button or beside a word. Decorative:
 * pair it with `aria-busy` on the control or a visible "Saving…" word.
 */
export function Spinner({ size = 16, className }: { size?: number; className?: string }) {
  return (
    <CircleNotchIcon
      size={size}
      aria-hidden="true"
      className={cx('shrink-0 animate-spin motion-reduce:animate-none', className)}
    />
  )
}
