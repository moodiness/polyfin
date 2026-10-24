import { useEffect, useRef, useState, type PointerEvent, type RefObject } from 'react'

/** How close to an edge, in px, the pointer scrolls the list or the page while dragging. */
const scrollZone = 48
/** The most a frame scrolls, in px, with the pointer at the edge itself. */
const scrollStep = 14

/**
 * The distance to scroll for a pointer at `y` between `top` and `bottom`: negative near the top,
 * positive near the bottom, faster closer to the edge; 0 elsewhere.
 */
function edgeScroll(y: number, top: number, bottom: number): number {
  if (y < top + scrollZone)
    return -Math.ceil((scrollStep * (top + scrollZone - Math.max(y, top))) / scrollZone)
  if (y > bottom - scrollZone) {
    return Math.ceil((scrollStep * (Math.min(y, bottom) - bottom + scrollZone)) / scrollZone)
  }
  return 0
}

/** Whether `element` can still scroll by `delta` (its top or bottom not reached). */
function canScroll(element: Element, delta: number): boolean {
  return delta < 0
    ? element.scrollTop > 0
    : element.scrollTop + element.clientHeight < element.scrollHeight - 1
}

type Drag = {
  /** The index of the dragged row. */
  from: number
  /** The index of the row it would land before; the row count for the end. */
  before: number
}

/**
 * Drags a row of an ordered list to its place by its grip, with a mouse, a pen or a finger, on
 * pointer events: the grip captures the pointer, the row is dropped before the first row whose
 * middle is below the pointer, and the list (when it scrolls in its own box) or the page scrolls
 * while the pointer nears its edge. Escape, or a pointer the browser takes back, drops nothing.
 *
 * `list` is the `<ol>` or `<ul>` whose children are the rows; `onDrop(from, to)` gets the index the
 * row moves to, only when it moves. Keyboards and screen readers move rows with `MoveButtons`.
 */
export function useRowDrag({
  list,
  onDrop,
}: {
  list: RefObject<HTMLElement | null>
  onDrop: (from: number, to: number) => void
}) {
  const [drag, setDrag] = useState<Drag | null>(null)
  // The drag in progress, for the pointer handlers and the scrolling frames.
  const current = useRef<{ pointerId: number; grip: Element; from: number; y: number } | null>(null)
  const frame = useRef(0)

  function before(y: number): number {
    const rows = list.current === null ? [] : [...list.current.children]
    const index = rows.findIndex((row) => {
      const box = row.getBoundingClientRect()
      return y < box.top + box.height / 2
    })
    return index < 0 ? rows.length : index
  }

  /** Shows the place of the row at `from` before the row at `index`, rendering only on a change. */
  function aim(from: number, index: number) {
    setDrag((shown) =>
      shown?.from === from && shown.before === index ? shown : { from, before: index },
    )
  }

  function stop() {
    cancelAnimationFrame(frame.current)
    const drag = current.current
    current.current = null
    setDrag(null)
    if (drag !== null && drag.grip.hasPointerCapture(drag.pointerId)) {
      drag.grip.releasePointerCapture(drag.pointerId)
    }
  }

  // Scrolls while the pointer stays near an edge, even still, and follows the rows under it.
  function scroll() {
    const drag = current.current
    if (drag === null) return
    const box = list.current
    let scrolled = false
    if (box !== null && box.scrollHeight > box.clientHeight) {
      const { top, bottom } = box.getBoundingClientRect()
      const delta = edgeScroll(drag.y, top, bottom)
      if (delta !== 0 && canScroll(box, delta)) {
        box.scrollTop += delta
        scrolled = true
      }
    }
    const page = document.scrollingElement
    if (!scrolled && page !== null) {
      // The top bar stays over the page: its bottom is the top of what can be seen.
      const top = document.querySelector('[data-top-bar]')?.getBoundingClientRect().bottom ?? 0
      const delta = edgeScroll(drag.y, top, window.innerHeight)
      if (delta !== 0 && canScroll(page, delta)) {
        window.scrollBy(0, delta)
        scrolled = true
      }
    }
    if (scrolled) aim(drag.from, before(drag.y))
    frame.current = requestAnimationFrame(scroll)
  }

  // Escape drops nothing; `stop` only reads refs, so the listener of the drag's start serves it all.
  const dragging = drag !== null
  useEffect(() => {
    if (!dragging) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        stop()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [dragging])

  useEffect(() => () => cancelAnimationFrame(frame.current), [])

  /** The pointer handlers of the grip of the row at `index`. */
  function grip(index: number) {
    return {
      onPointerDown: (event: PointerEvent<HTMLElement>) => {
        if (!event.isPrimary || event.button !== 0 || current.current !== null) return
        // No text selection, no focus change: the grip only drags.
        event.preventDefault()
        event.currentTarget.setPointerCapture(event.pointerId)
        current.current = {
          pointerId: event.pointerId,
          grip: event.currentTarget,
          from: index,
          y: event.clientY,
        }
        setDrag({ from: index, before: index })
        frame.current = requestAnimationFrame(scroll)
      },
      onPointerMove: (event: PointerEvent<HTMLElement>) => {
        const drag = current.current
        if (drag === null || event.pointerId !== drag.pointerId) return
        drag.y = event.clientY
        aim(drag.from, before(event.clientY))
      },
      onPointerUp: (event: PointerEvent<HTMLElement>) => {
        const drag = current.current
        if (drag === null || event.pointerId !== drag.pointerId) return
        const at = before(event.clientY)
        stop()
        if (at !== drag.from && at !== drag.from + 1)
          onDrop(drag.from, at > drag.from ? at - 1 : at)
      },
      onPointerCancel: stop,
      onLostPointerCapture: (event: PointerEvent<HTMLElement>) => {
        if (current.current?.pointerId === event.pointerId) stop()
      },
    }
  }

  // A drop where the row already is shows no place.
  const moves = drag !== null && drag.before !== drag.from && drag.before !== drag.from + 1
  return {
    /** The index of the row being dragged, or null. */
    dragged: drag?.from ?? null,
    /** The index of the row the dragged one would land before (the row count for the end), or null. */
    dropBefore: moves ? drag.before : null,
    grip,
  }
}
