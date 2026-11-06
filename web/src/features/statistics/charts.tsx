import { useRef } from 'react'
import type { PlayMethod } from '@/api'
import { formatHour, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import { useScrollFade } from '@/ui'

/**
 * The colors of a chart's series (users, play methods), in order. They are hues apart from the
 * state colors (green, amber, red), so that a user never reads as a state, and each has 3:1 or more
 * against a tile; a legend always names them. Past them, the others share the quiet ink.
 */
const seriesColors = ['#9ea4ff', '#5cc8ee', '#e68fd8', '#4fcdb6', '#c3a3ff']
const otherColor = 'var(--color-ink-3)'

/** The color of the series at `index`, as a CSS color. */
export const seriesColor = (index: number) => seriesColors[index] ?? otherColor

const methodIndex: Record<PlayMethod, number> = {
  direct_play: 0,
  direct_stream: 1,
  conversion: 2,
  '': -1,
}

/** A chart's ground: a tile with no border. */
const tile = 'rounded-panel bg-s1 px-[30px] py-7 max-md:rounded-[24px] max-md:px-5 max-md:py-[22px]'

export type BarItem = { key: string; label: string; detail: string; value: number }

/** Things compared by a value: a label and a detail above a bar as long as its share of the largest. */
export function BarList({ items, label }: { items: readonly BarItem[]; label: string }) {
  const largest = Math.max(1, ...items.map((item) => item.value))
  return (
    <ol aria-label={label} className="space-y-4">
      {items.map((item) => (
        <li key={item.key}>
          <div className="flex flex-wrap items-baseline justify-between gap-x-4">
            <span className="min-w-0 truncate text-control font-medium text-ink">{item.label}</span>
            <span className="figures shrink-0 text-small text-ink-2">{item.detail}</span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded-full bg-s3" aria-hidden="true">
            <div
              className="h-full rounded-full"
              style={{ width: `${(item.value / largest) * 100}%`, backgroundColor: seriesColor(0) }}
            />
          </div>
        </li>
      ))}
    </ol>
  )
}

export type TimeColumn = {
  key: string
  /** The day or month, as the language writes it. */
  label: string
  /** Seconds played by each user, in the order of the users' colors. */
  parts: { index: number; played: number }[]
  played: number
}

/** Hours watched per day or month: one column each, stacked by user. */
export function TimeChart({ columns, label }: { columns: readonly TimeColumn[]; label: string }) {
  const { language, t } = useI18n()
  const largest = Math.max(1, ...columns.map((column) => column.played))
  const ticks =
    columns.length > 1 ? [0, Math.floor((columns.length - 1) / 2), columns.length - 1] : [0]
  return (
    <figure aria-label={label} className={tile}>
      <div className="flex h-48 items-end gap-[3px] max-md:h-40 max-md:gap-px" role="list">
        {columns.map((column) => {
          const text = t.statistics.bar(column.label, formatSpan(column.played, language))
          return (
            <div
              key={column.key}
              role="listitem"
              title={text}
              aria-label={text}
              className="flex h-full min-w-0 flex-1 flex-col-reverse overflow-hidden rounded-t-[4px] hover:bg-s2"
            >
              {column.parts.map((part) => (
                <div
                  key={part.index}
                  className="w-full"
                  style={{
                    height: `${(part.played / largest) * 100}%`,
                    backgroundColor: seriesColor(part.index),
                  }}
                />
              ))}
            </div>
          )
        })}
      </div>
      <div className="mt-3 flex justify-between text-micro text-ink-2 figures" aria-hidden="true">
        {ticks.map((index) => (
          <span key={index}>{columns[index]?.label}</span>
        ))}
      </div>
    </figure>
  )
}

/** The share of each play method in the playbacks, as one bar and a legend. */
export function MethodShare({
  methods,
}: {
  methods: readonly { method: PlayMethod; plays: number; played: number }[]
}) {
  const { language, t } = useI18n()
  const text = t.statistics
  const total = Math.max(
    1,
    methods.reduce((sum, m) => sum + m.plays, 0),
  )
  const percent = new Intl.NumberFormat(language, { style: 'percent', maximumFractionDigits: 0 })
  return (
    <div className={tile}>
      <div className="flex h-4 gap-[3px] overflow-hidden rounded-full" aria-hidden="true">
        {methods.map((m) => (
          <div
            key={m.method}
            style={{
              width: `${(m.plays / total) * 100}%`,
              backgroundColor: seriesColor(methodIndex[m.method]),
            }}
          />
        ))}
      </div>
      <ul className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {methods.map((m) => (
          <li key={m.method} className="flex items-start gap-3">
            <span
              aria-hidden="true"
              className="mt-1.5 size-3 shrink-0 rounded-full"
              style={{ backgroundColor: seriesColor(methodIndex[m.method]) }}
            />
            <span>
              <span className="text-control font-medium text-ink">
                {text.methodNames[m.method]}
              </span>
              <span className="block text-small text-ink-2 figures">
                {text.share(percent.format(m.plays / total))} · {formatSpan(m.played, language)}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** The hours of the week, Monday first, each as strong as how long was watched in it. */
export function HourGrid({ hours, label }: { hours: readonly number[]; label: string }) {
  const { language, t } = useI18n()
  const largest = Math.max(1, ...hours)
  const weekday = new Intl.DateTimeFormat(language, { weekday: 'short' })
  // 1 January 2024 was a Monday.
  const days = Array.from({ length: 7 }, (_, day) => weekday.format(new Date(2024, 0, 1 + day)))
  // On a phone the week scrolls sideways inside its tile, its edges fading.
  const scroller = useRef<HTMLDivElement>(null)
  useScrollFade(scroller)
  return (
    <figure aria-label={label} className={tile}>
      <div ref={scroller} className="scroll-fade overflow-x-auto">
        <div
          className="grid min-w-[620px] gap-1"
          style={{ gridTemplateColumns: '3.5rem repeat(24, minmax(0, 1fr))' }}
        >
          {days.map((day, row) => (
            <div key={day} className="contents">
              <span className="pr-2 text-micro leading-5 text-ink-2">{day}</span>
              {Array.from({ length: 24 }, (_, hour) => {
                const played = hours[row * 24 + hour] ?? 0
                const text = t.statistics.hoursCell(
                  day,
                  formatHour(hour, language),
                  formatSpan(played, language),
                )
                return (
                  <span
                    key={hour}
                    title={text}
                    aria-label={text}
                    role="img"
                    className="h-5 rounded-[5px] bg-s3"
                    style={
                      played > 0
                        ? {
                            backgroundColor: seriesColor(0),
                            opacity: 0.2 + 0.8 * (played / largest),
                          }
                        : undefined
                    }
                  />
                )
              })}
            </div>
          ))}
          <span />
          {Array.from({ length: 24 }, (_, hour) => (
            <span
              key={hour}
              className="pt-1 text-micro whitespace-nowrap text-ink-2 figures"
              aria-hidden="true"
            >
              {hour % 6 === 0 ? formatHour(hour, language) : ''}
            </span>
          ))}
        </div>
      </div>
    </figure>
  )
}
