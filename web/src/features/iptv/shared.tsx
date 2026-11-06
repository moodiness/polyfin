import { CaretLeftIcon, CaretRightIcon, DotsSixVerticalIcon } from '@phosphor-icons/react'
import { useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { useI18n } from '@/i18n'
import { Button, cx } from '@/ui'

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
    <nav aria-label={t.lineup.pager.label} className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <p aria-live="polite" className="figures text-small text-ink-2">
        {t.lineup.pager.range(
          number(offset + 1),
          number(Math.min(offset + limit, total)),
          number(total),
        )}
      </p>
      <div className="ml-auto flex gap-2">
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

/** The grip of a draggable row: a hint for the mouse; keyboards use the arrows. */
export function DragGrip({ name }: { name: string }) {
  const { t } = useI18n()
  return (
    <span
      aria-hidden="true"
      title={t.lineup.dragHandle(name)}
      className="hidden shrink-0 cursor-grab text-ink-3 active:cursor-grabbing sm:inline"
    >
      <DotsSixVerticalIcon size={20} />
    </span>
  )
}

/**
 * A channel's logo, through Polyfin as Jellyfin apps get it, on a soft round tile; its initial when
 * it has none or it fails to load. `logo` only tells whether there is one, and refreshes it when it
 * changes. 36 px in a compact row, 56 px (`large`) at the head of its editor.
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
    'shrink-0 overflow-hidden bg-s2',
    large ? 'size-14 rounded-[18px]' : 'size-9 rounded-[12px]',
  )
  if (!logo || failed === logo) {
    return (
      <span
        aria-hidden="true"
        className={cx(
          frame,
          'inline-grid place-items-center font-bold text-ink',
          large ? 'text-lead' : 'text-small',
        )}
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
      className={cx(frame, 'object-contain p-1')}
    />
  )
}

/** One figure of a row of figures: a label, a value, a detail; a link when `to` is given. */
export type Figure = {
  label: string
  value: string
  detail?: ReactNode
  tone?: 'warn' | 'ok'
  to?: string
}

/**
 * Figures side by side as Salon tiles: a big value, what it counts, and a detail under it. A tile
 * with `to` is a link as a whole, lit under the keyboard. Two columns on a phone.
 */
export function Figures({ items, label }: { items: readonly Figure[]; label?: string }) {
  return (
    <dl aria-label={label} className="grid grid-cols-4 gap-2.5 max-md:grid-cols-2">
      {items.map((item) => (
        <div
          key={item.label}
          className={cx(
            'relative flex min-w-0 flex-col rounded-row bg-s1 px-6 pt-[22px] pb-6 max-md:px-5 max-md:pt-5 max-md:pb-5',
            item.to &&
              'transition-[scale,background-color,color,box-shadow] duration-350 ease-nuit hover:bg-s2 has-[a:focus-visible]:lit has-[a:focus-visible]:lift [--lift:1.03]',
          )}
        >
          <dt className="order-2 mt-2.5 text-small text-ink-2">
            {item.to ? (
              // The link covers the whole tile.
              <Link
                to={item.to}
                className="after:absolute after:inset-0 after:rounded-row focus-visible:outline-none"
              >
                {item.label}
              </Link>
            ) : (
              item.label
            )}
          </dt>
          <dd
            className={cx(
              'figures order-1 text-[44px] leading-none font-[650] tracking-[-0.02em] max-md:text-[36px]',
              item.tone === 'warn' && 'text-warn',
              item.tone === 'ok' && 'text-ok',
            )}
          >
            {item.value}
          </dd>
          {item.detail !== undefined && (
            <dd className="order-3 mt-1 min-w-0 truncate text-micro text-ink-3">{item.detail}</dd>
          )}
        </div>
      ))}
    </dl>
  )
}
