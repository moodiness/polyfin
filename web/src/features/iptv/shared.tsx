import {
  ArrowDownIcon,
  ArrowUpIcon,
  CaretLeftIcon,
  CaretRightIcon,
  DotsSixVerticalIcon,
} from '@phosphor-icons/react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { dateTime, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import { Button, cx, IconButton } from '@/ui'

/** A value that follows `value` once it stopped changing for `delay` milliseconds. */
export function useDebounced<T>(value: T, delay = 300): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return settled
}

/** Formats a count for the current language. */
export function useNumber(): (n: number) => string {
  const { language } = useI18n()
  return (n: number) => n.toLocaleString(language)
}

/** A time as "3 hours ago", with the full date on hover. */
export function RelativeTime({ iso }: { iso: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}

/** Previous and next buttons over a paged list, with where the page stands. */
export function Pager({
  offset,
  limit,
  total,
  busy,
  onOffset,
}: {
  offset: number
  limit: number
  total: number
  busy: boolean
  onOffset: (offset: number) => void
}) {
  const { t } = useI18n()
  const number = useNumber()
  if (total === 0) return null
  return (
    <nav aria-label={t.lineup.pager.label} className="flex flex-wrap items-center gap-3">
      <p aria-live="polite" className="text-small text-ink-3 tabular-nums">
        {t.lineup.pager.range(
          number(offset + 1),
          number(Math.min(offset + limit, total)),
          number(total),
        )}
      </p>
      <div className="ml-auto flex gap-1.5">
        <Button
          size="sm"
          icon={CaretLeftIcon}
          disabled={busy || offset === 0}
          onClick={() => onOffset(Math.max(0, offset - limit))}
        >
          {t.lineup.pager.previous}
        </Button>
        <Button
          size="sm"
          iconEnd={CaretRightIcon}
          disabled={busy || offset + limit >= total}
          onClick={() => onOffset(offset + limit)}
        >
          {t.lineup.pager.next}
        </Button>
      </div>
    </nav>
  )
}

/**
 * Up and down buttons for one item of an ordered list. After a move, focus follows the item (or
 * the other button at an end of the list).
 */
export function MoveButtons({
  name,
  index,
  count,
  disabled = false,
  onMove,
}: {
  name: string
  index: number
  count: number
  disabled?: boolean
  onMove: (to: number) => void
}) {
  const { t } = useI18n()
  const box = useRef<HTMLSpanElement>(null)
  const moved = useRef<'up' | 'down' | null>(null)
  const first = index === 0
  const last = index === count - 1
  useEffect(() => {
    const direction = moved.current
    if (direction === null) return
    moved.current = null
    const goUp = direction === 'up' ? !first : last
    box.current?.querySelectorAll('button')[goUp ? 0 : 1]?.focus()
  }, [index, first, last])
  return (
    <span ref={box} className="flex shrink-0">
      <IconButton
        size="sm"
        icon={ArrowUpIcon}
        label={t.common.moveUp(name)}
        disabled={disabled || first}
        onClick={() => {
          moved.current = 'up'
          onMove(index - 1)
        }}
      />
      <IconButton
        size="sm"
        icon={ArrowDownIcon}
        label={t.common.moveDown(name)}
        disabled={disabled || last}
        onClick={() => {
          moved.current = 'down'
          onMove(index + 1)
        }}
      />
    </span>
  )
}

/** The grip of a draggable row: a hint for the mouse; keyboards use the arrows. */
export function DragGrip({ name }: { name: string }) {
  const { t } = useI18n()
  return (
    <span
      aria-hidden="true"
      title={t.lineup.dragHandle(name)}
      className="hidden shrink-0 cursor-grab text-ink-3 sm:inline"
    >
      <DotsSixVerticalIcon size={16} />
    </span>
  )
}

/**
 * A channel's logo, through Polyfin as Jellyfin apps get it, in a fixed box; its initial when it has
 * none or it fails to load. `logo` only tells whether there is one, and refreshes it when it changes.
 */
export function ChannelLogo({
  id,
  logo,
  name,
  large = false,
}: {
  id: string
  logo: string | null
  name: string
  large?: boolean
}) {
  const [failed, setFailed] = useState<string | null>(null)
  const frame = cx(
    'shrink-0 overflow-hidden rounded-field border border-line bg-s3',
    large ? 'size-14' : 'size-9',
  )
  if (!logo || failed === logo) {
    return (
      <span
        aria-hidden="true"
        className={cx(frame, 'inline-grid place-items-center text-small font-semibold text-ink-3')}
      >
        {name.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return (
    <img
      src={`/Items/${encodeURIComponent(id)}/Images/Primary?maxHeight=80&tag=${encodeURIComponent(logo)}`}
      alt=""
      loading="lazy"
      onError={() => setFailed(logo)}
      className={cx(frame, 'object-contain p-0.5')}
    />
  )
}

/** One figure of a row of figures: a label, a mono value, a detail; a link when `to` is given. */
export type Figure = {
  label: string
  value: string
  detail?: ReactNode
  tone?: 'warn' | 'ok'
  to?: string
}

/**
 * A row of figures split by hairlines, without cards (the mockup's IPTV summary). Two columns on a
 * phone.
 */
export function Figures({ items, label }: { items: readonly Figure[]; label?: string }) {
  return (
    <dl
      aria-label={label}
      className="grid grid-cols-2 gap-y-6 border-y border-line py-5 md:grid-cols-4"
    >
      {items.map((item, index) => {
        const value = (
          <dd
            className={cx(
              'figures mt-2 text-[24px] leading-tight font-medium tracking-[-0.02em]',
              item.tone === 'warn' ? 'text-warn' : 'text-ink',
            )}
          >
            {item.value}
          </dd>
        )
        return (
          <div
            key={item.label}
            className={cx(
              'min-w-0 px-5 max-md:px-4',
              'border-line',
              index % 2 === 0 ? 'max-md:pl-0' : 'border-l',
              'md:not-first:border-l md:first:pl-0',
            )}
          >
            <dt className="text-[12.5px] text-ink-3">
              {item.to ? (
                <Link
                  to={item.to}
                  className="rounded-sm underline decoration-line-3 underline-offset-4 transition-colors duration-160 hover:text-ink hover:decoration-link"
                >
                  {item.label}
                </Link>
              ) : (
                item.label
              )}
            </dt>
            {value}
            {item.detail !== undefined && (
              <dd className="mt-1.5 truncate text-small text-ink-3">{item.detail}</dd>
            )}
          </div>
        )
      })}
    </dl>
  )
}
