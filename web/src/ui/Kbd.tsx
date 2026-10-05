import type { ReactNode } from 'react'
import { cx } from './cx'

/** Whether the device is an Apple one, whose shortcuts use ⌘ rather than Ctrl. */
export const isApple = /Mac|iPhone|iPad|iPod/.test(navigator.platform || navigator.userAgent)

/** The modifier of shortcuts as keys show it: "⌘" on Apple devices, "Ctrl" elsewhere. */
export const modKey = isApple ? '⌘' : 'Ctrl'

/**
 * A key or shortcut as printed on the keyboard: `<Kbd>⌘K</Kbd>`, `<Kbd>{modKey} K</Kbd>`. Use it
 * in the search button, the palette footer and help texts.
 */
export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cx(
        'inline-flex h-5 min-w-5 items-center justify-center rounded-check border border-line-2 bg-s4 px-1.5 font-mono text-[11.5px] leading-none text-ink-2',
        className,
      )}
    >
      {children}
    </kbd>
  )
}
