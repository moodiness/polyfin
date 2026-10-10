import type { PlayMethod } from '@/api'
import { formatHour, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import { cx } from '@/ui'

/** The colors of the users in the charts, in order; past them, the others share the last one. */
export const userColors = ['bg-accent', 'bg-ok', 'bg-warn', 'bg-link', 'bg-danger', 'bg-ink-2']
const otherColor = 'bg-ink-3'

/** The color of the user at `index` among those ranked. */
export const userColor = (index: number) => userColors[index] ?? otherColor

const methodColors: Record<PlayMethod, string> = {
  direct_play: 'bg-ok',
  direct_stream: 'bg-link',
  conversion: 'bg-warn',
  '': 'bg-ink-3',
}

export type BarItem = { key: string; label: string; detail: string; value: number }

/** Things compared by a value: a label and a detail above a bar as long as its share of the largest. */
export function BarList({ items, label }: { items: readonly BarItem[]; label: string }) {
  const largest = Math.max(1, ...items.map((item) => item.value))
  return (
    <ol aria-label={label} className="space-y-3.5">
      {items.map((item) => (
        <li key={item.key}>
          <div className="flex items-baseline justify-between gap-4 text-control">
            <span className="min-w-0 truncate text-ink">{item.label}</span>
            <span className="figures shrink-0 text-small text-ink-3">{item.detail}</span>
          </div>
          <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-s3" aria-hidden="true">
            <div
              className="h-full rounded-full bg-accent"
              style={{ width: `${(item.value / largest) * 100}%` }}
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
    <figure aria-label={label} className="rounded-panel border border-line-2 bg-s1 px-4 pt-4 pb-3">
      <div className="flex h-40 items-end gap-px" role="list">
        {columns.map((column) => {
          const text = t.statistics.bar(column.label, formatSpan(column.played, language))
          return (
            <div
              key={column.key}
              role="listitem"
              title={text}
              aria-label={text}
              className="flex h-full min-w-0 flex-1 flex-col-reverse rounded-t-[2px] hover:bg-s2"
            >
              {column.parts.map((part) => (
                <div
                  key={part.index}
                  className={cx(
                    'w-full first:rounded-none last:rounded-t-[2px]',
                    userColor(part.index),
                  )}
                  style={{ height: `${(part.played / largest) * 100}%` }}
                />
              ))}
            </div>
          )
        })}
      </div>
      <div className="mt-2 flex justify-between text-[12px] text-ink-3" aria-hidden="true">
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
    <div className="rounded-panel border border-line-2 bg-s1 p-4">
      <div className="flex h-3 overflow-hidden rounded-full bg-s3" aria-hidden="true">
        {methods.map((m) => (
          <div
            key={m.method}
            className={methodColors[m.method]}
            style={{ width: `${(m.plays / total) * 100}%` }}
          />
        ))}
      </div>
      <ul className="mt-4 grid gap-3 sm:grid-cols-2">
        {methods.map((m) => (
          <li key={m.method} className="flex items-start gap-2.5 text-control">
            <span
              aria-hidden="true"
              className={cx('mt-1.5 size-2.5 shrink-0 rounded-full', methodColors[m.method])}
            />
            <span>
              <span className="text-ink">{text.methodNames[m.method]}</span>
              <span className="block text-small text-ink-3">
                {text.share(percent.format(m.plays / total))} · {formatSpan(m.played, language)}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** The hours of the week, Monday first, each as dark as how long was watched in it. */
export function HourGrid({ hours, label }: { hours: readonly number[]; label: string }) {
  const { language, t } = useI18n()
  const largest = Math.max(1, ...hours)
  const weekday = new Intl.DateTimeFormat(language, { weekday: 'short' })
  // 1 January 2024 was a Monday.
  const days = Array.from({ length: 7 }, (_, day) => weekday.format(new Date(2024, 0, 1 + day)))
  return (
    <figure
      aria-label={label}
      className="overflow-x-auto rounded-panel border border-line-2 bg-s1 p-4"
    >
      <div
        className="grid min-w-[560px] gap-[3px]"
        style={{ gridTemplateColumns: '3rem repeat(24, minmax(0, 1fr))' }}
      >
        {days.map((day, row) => (
          <div key={day} className="contents">
            <span className="pr-2 text-[12px] leading-4 text-ink-3">{day}</span>
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
                  className={cx('h-4 rounded-[3px]', played > 0 ? 'bg-accent' : 'bg-s3')}
                  style={played > 0 ? { opacity: 0.15 + 0.85 * (played / largest) } : undefined}
                />
              )
            })}
          </div>
        ))}
        <span />
        {Array.from({ length: 24 }, (_, hour) => (
          <span key={hour} className="text-[11px] text-ink-3" aria-hidden="true">
            {hour % 6 === 0 ? formatHour(hour, language) : ''}
          </span>
        ))}
      </div>
    </figure>
  )
}
