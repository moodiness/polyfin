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

/** An hour of the day, 0 to 23, as the language writes it, "4:00 AM" or "04:00", in no time zone. */
export function formatHour(hour: number, language: string): string {
  return new Intl.DateTimeFormat(language, {
    hour: 'numeric',
    minute: '2-digit',
    timeZone: 'UTC',
  }).format(Date.UTC(2000, 0, 1, hour))
}

const byteUnits = ['byte', 'kilobyte', 'megabyte', 'gigabyte', 'terabyte'] as const

/** A size in bytes as "4.2 GB", decimal units as POLYFIN_CACHE_SIZE reads them. */
export function formatBytes(bytes: number, language: string): string {
  let value = bytes
  let unit = 0
  while (value >= 1000 && unit < byteUnits.length - 1) {
    value /= 1000
    unit++
  }
  return new Intl.NumberFormat(language, {
    style: 'unit',
    unit: byteUnits[unit],
    unitDisplay: 'short',
    maximumFractionDigits: value < 10 && unit > 0 ? 1 : 0,
  }).format(value)
}

/** A bitrate in bits per second as "8.2 Mbps". */
export function formatBitrate(bitsPerSecond: number, language: string): string {
  const megabits = bitsPerSecond / 1_000_000
  if (megabits >= 1) {
    return `${new Intl.NumberFormat(language, { maximumFractionDigits: megabits < 10 ? 1 : 0 }).format(megabits)} Mbps`
  }
  return `${new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(bitsPerSecond / 1000)} kbps`
}

/** A position or length in seconds as a clock, "1:02:03" or "2:03". */
export function formatClock(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds))
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = String(seconds % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
}

/** A span of time in seconds as "2 h 5 min", "45 s", in its two largest units. */
export function formatSpan(totalSeconds: number, language: string): string {
  const seconds = Math.max(0, Math.round(totalSeconds))
  const parts: [number, Intl.NumberFormatOptions['unit']][] = [
    [Math.floor(seconds / 86400), 'day'],
    [Math.floor((seconds % 86400) / 3600), 'hour'],
    [Math.floor((seconds % 3600) / 60), 'minute'],
    [seconds % 60, 'second'],
  ]
  const first = parts.findIndex(([value]) => value > 0)
  if (first < 0) return new Intl.NumberFormat(language, { style: 'unit', unit: 'second' }).format(0)
  return parts
    .slice(first, first + 2)
    .filter(([value]) => value > 0)
    .map(([value, unit]) =>
      new Intl.NumberFormat(language, { style: 'unit', unit, unitDisplay: 'short' }).format(value),
    )
    .join(' ')
}
