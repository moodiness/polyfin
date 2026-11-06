/** Joins class names, skipping the empty and false ones: `cx('a', on && 'b')`. */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(' ')
}

/**
 * A length in pixels as rem, for sizes set from code (icons, inline styles): they then follow the
 * root's density, as the classes do (16 px to the rem on a phone, 13.6 on a desktop).
 */
export function rem(px: number): string {
  return `${px / 16}rem`
}

/**
 * A length in pixels at the root's density, for code that places or measures in pixels what the
 * classes size in rem: `scaledPx(16)` is 16 on a phone and 13.6 on a desktop.
 */
export function scaledPx(px: number): number {
  return (px * parseFloat(getComputedStyle(document.documentElement).fontSize)) / 16
}

/** Lower case without accents, so that a search for "resume" finds "Résumé". */
export function searchable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}
