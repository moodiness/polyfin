import { useCallback, useEffect, useState, type RefCallback, type RefObject } from 'react'
import { scaledPx } from './cx'

/**
 * Scrolls a sideways row (tabs on a phone) so that its current item shows, without ever scrolling
 * the page vertically as `scrollIntoView` would.
 */
export function keepCurrentInView(row: HTMLElement | null) {
  const current = row?.querySelector<HTMLElement>('[aria-current], [aria-selected="true"]')
  if (!row || !current || row.scrollWidth <= row.clientWidth) return
  const bounds = row.getBoundingClientRect()
  const item = current.getBoundingClientRect()
  const margin = scaledPx(16)
  if (item.left < bounds.left) row.scrollLeft -= bounds.left - item.left + margin
  else if (item.right > bounds.right) row.scrollLeft += item.right - bounds.right + margin
}

/**
 * Marks a row that scrolls sideways with `data-more-start` and `data-more-end` while items lie
 * past that edge, so that the `scroll-fade` utility fades the edge: the cue that the row scrolls.
 * Follows scrolling, resizing and items coming and going.
 */
export function useScrollFade(ref: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const row = ref.current
    if (!row) return
    const update = () => {
      const hidden = row.scrollWidth - row.clientWidth
      row.toggleAttribute('data-more-start', hidden > 1 && row.scrollLeft > 1)
      row.toggleAttribute('data-more-end', hidden > 1 && row.scrollLeft < hidden - 1)
    }
    const resized = new ResizeObserver(update)
    const observeAll = () => {
      resized.disconnect()
      resized.observe(row)
      for (const item of row.children) resized.observe(item)
      update()
    }
    const changed = new MutationObserver(observeAll)
    observeAll()
    changed.observe(row, { childList: true })
    row.addEventListener('scroll', update, { passive: true })
    return () => {
      resized.disconnect()
      changed.disconnect()
      row.removeEventListener('scroll', update)
    }
  }, [ref])
}

/**
 * Whether an element shows on screen, for what is worth reading again only while it is seen: give
 * the callback as its ref.
 */
export function useOnScreen<T extends Element>(): [RefCallback<T>, boolean] {
  const [onScreen, setOnScreen] = useState(false)
  const ref = useCallback((element: T | null) => {
    if (!element) return
    const observer = new IntersectionObserver(([entry]) =>
      setOnScreen(entry?.isIntersecting ?? false),
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return [ref, onScreen]
}
