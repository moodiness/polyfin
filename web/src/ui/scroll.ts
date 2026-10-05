/**
 * Scrolls a sideways row (tabs on a phone) so that its current item shows, without ever scrolling
 * the page vertically as `scrollIntoView` would.
 */
export function keepCurrentInView(row: HTMLElement | null) {
  const current = row?.querySelector<HTMLElement>('[aria-current], [aria-selected="true"]')
  if (!row || !current || row.scrollWidth <= row.clientWidth) return
  const bounds = row.getBoundingClientRect()
  const item = current.getBoundingClientRect()
  const margin = 16
  if (item.left < bounds.left) row.scrollLeft -= bounds.left - item.left + margin
  else if (item.right > bounds.right) row.scrollLeft += item.right - bounds.right + margin
}
