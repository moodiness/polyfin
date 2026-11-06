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
        'inline-flex h-6.5 min-w-6.5 items-center justify-center rounded-[8px] bg-ink/7 px-2 text-[14px] leading-none font-semibold text-ink-3',
        className,
      )}
    >
      {children}
    </kbd>
  )
}
