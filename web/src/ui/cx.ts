/** Joins class names, skipping the empty and false ones: `cx('a', on && 'b')`. */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(' ')
}

/** Lower case without accents, so that a search for "resume" finds "Résumé". */
export function searchable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}
