import { ApiError } from '@/api'
import type { Messages } from '@/i18n/en'

type ErrorCode = keyof Messages['errors']

/** Maps an API error code to localized text; unknown codes fall back to a generic message. */
export function errorMessage(t: Messages, error: unknown): string {
  if (error instanceof ApiError) {
    return Object.hasOwn(t.errors, error.code)
      ? t.errors[error.code as ErrorCode]
      : t.errors.generic
  }
  // fetch rejects with a TypeError when the server cannot be reached.
  if (error instanceof TypeError) return t.errors.network
  return t.errors.generic
}

/** Localized name of a Stremio type or resource; values without a translation are shown as sent. */
export function stremioLabel(labels: Readonly<Record<string, string>>, value: string): string {
  return Object.hasOwn(labels, value) ? labels[value] : value
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

/** "3 minutes ago" style text in the active language. */
export function relativeTime(iso: string, language: string, justNow: string): string {
  const seconds = (new Date(iso).getTime() - Date.now()) / 1000
  const format = new Intl.RelativeTimeFormat(language, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return format.format(Math.round(seconds / size), unit)
  }
  return justNow
}

export function dateTime(iso: string, language: string): string {
  return new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'short' }).format(
    new Date(iso),
  )
}
