/**
 * A whole number typed in a settings field. An empty field is -1, which the server refuses with
 * the setting's own error, rather than 0, which could turn a setting off without the user typing it.
 */
export function wholeNumber(value: number | null): number {
  return value === null ? -1 : Math.trunc(value)
}

/** The value a number field shows: -1 (see wholeNumber) shows as empty. */
export function shownNumber(value: number): number | null {
  return value < 0 ? null : value
}
