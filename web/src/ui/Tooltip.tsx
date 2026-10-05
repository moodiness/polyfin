import {
  cloneElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'
import { cx } from './cx'

/**
 * A short hint shown when the pointer rests on, or the keyboard reaches, one control: what an
 * icon means, why a button is disabled. It describes the control (`aria-describedby`); it never
 * holds the only copy of something needed. Escape hides it.
 *
 *   <Tooltip content={t.nav.webPlayerHint}><a href="/web/">…</a></Tooltip>
 */
export function Tooltip({
  content,
  side = 'top',
  children,
}: {
  content: ReactNode
  /** Where it opens; `bottom` for controls at the top of the screen. */
  side?: 'top' | 'bottom'
  /** One focusable element. */
  children: ReactElement<{ 'aria-describedby'?: string }>
}) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const timer = useRef<number>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])

  function show(delay: number) {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setOpen(true), delay)
  }
  function hide() {
    window.clearTimeout(timer.current)
    setOpen(false)
  }

  return (
    <span
      className="relative inline-flex"
      onPointerEnter={() => show(400)}
      onPointerLeave={hide}
      onFocus={(event) => {
        if (event.target.matches(':focus-visible')) show(0)
      }}
      onBlur={hide}
      onKeyDown={(event) => {
        if (event.key === 'Escape' && open) {
          event.stopPropagation()
          hide()
        }
      }}
    >
      {cloneElement(children, {
        'aria-describedby': [children.props['aria-describedby'], id].filter(Boolean).join(' '),
      })}
      <span
        role="tooltip"
        id={id}
        className={cx(
          'pointer-events-none absolute left-1/2 z-50 w-max max-w-64 -translate-x-1/2 rounded-field bg-s4 px-2.5 py-1.5 text-micro text-ink shadow-pop transition-opacity duration-160 ease-nuit',
          side === 'top' ? 'bottom-full mb-2' : 'top-full mt-2',
          open ? 'opacity-100' : 'invisible opacity-0',
        )}
      >
        {content}
      </span>
    </span>
  )
}
