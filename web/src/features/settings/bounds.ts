import type { SettingBounds } from '@/api'
import type { RangeText } from '@/i18n/en/settings'

/**
 * A setting's bounds and default as the help text shows them, written as numbers are in the
 * language (2,000 in English, 2 000 in French). `scale` converts the server's unit to the one the
 * field shows, such as 1/60 for seconds shown in minutes.
 */
export function rangeText(
  bounds: SettingBounds | undefined,
  language: string,
  scale = 1,
): RangeText {
  const write = (value: unknown) =>
    typeof value === 'number' ? (value * scale).toLocaleString(language) : ''
  return { min: write(bounds?.min), max: write(bounds?.max), default: write(bounds?.default) }
}

/** The smallest and largest values a number field takes: 0 when the setting accepts it too. */
export function numberLimits(
  bounds: SettingBounds | undefined,
  scale = 1,
): { min?: number; max?: number } {
  const min = bounds?.zero ? 0 : bounds?.min
  return {
    min: min === undefined ? undefined : min * scale,
    max: bounds?.max === undefined ? undefined : bounds.max * scale,
  }
}
