import { useEffect, useRef, type ReactNode } from 'react'
import { cx } from '@/ui'

/** The form of a signed-out page: its wells 16 px apart. */
export const authForm = 'flex flex-col gap-4'

/** The main button of a signed-out page: as wide as the wells, 44 px tall (37 in Compact). */
export const wideButton = 'min-h-11 w-full text-lead'

/**
 * The first field of a signed-out page, or of Quick Connect, for its `ref`: typing works at once,
 * yet the page opens with nothing lit. Rather than taking the focus on arrival, which lights its
 * well before anyone acts, the field takes it at the first character typed while nothing else
 * has it, and that character goes into it. A click or Tab lights it as any field.
 */
export function useFirstField() {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key.length !== 1 || event.ctrlKey || event.metaKey || event.altKey) return
      const active = document.activeElement
      if (active !== null && active !== document.body && active.id !== 'main') return
      ref.current?.focus()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])
  return ref
}

/**
 * The frame of the pages shown before anyone is signed in: the title (the greeting's size) and a
 * lede, then the form, if any, in wells straight on the room, with no panel around it. The public
 * shell around it carries the one logo, with the product's name, and the language at the top.
 */
export function AuthFrame({
  title,
  lede,
  wide = false,
  children,
}: {
  title: string
  lede: string
  /** 600 px instead of 520, for the setup key and the recovery codes. */
  wide?: boolean
  /** The form; without one, the page is its heading and lede. */
  children?: ReactNode
}) {
  return (
    <div
      className={cx(
        'mx-auto w-full pt-[min(12vh,8rem)] max-md:pt-4',
        wide ? 'max-w-[37.5rem]' : 'max-w-[32.5rem]',
      )}
    >
      <h1 className="text-display break-words text-ink">{title}</h1>
      <p className="mt-2 max-w-[46ch] text-lead text-ink-2">{lede}</p>
      {children !== undefined && <div className="mt-8 max-md:mt-6">{children}</div>}
    </div>
  )
}
