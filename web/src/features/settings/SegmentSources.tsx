import { ArrowCounterClockwiseIcon, ArrowDownIcon, ArrowUpIcon } from '@phosphor-icons/react'
import { useId, useState } from 'react'
import type { SegmentSource } from '@/api'
import { useI18n } from '@/i18n'
import { Button, FieldError, IconButton, StatusPill } from '@/ui'

/** The names the segment databases go by. */
const sourceNames: Record<SegmentSource, string> = {
  theintrodb: 'TheIntroDB',
  introdb: 'IntroDB',
  publicmetadb: 'PublicMetaDB',
}

/**
 * The order of preference of the skip marker sources, moved with the arrows, each with its
 * state: on, waiting for its key, or turned off by POLYFIN_SEGMENTS. The order the variable
 * gives can be restored while another is chosen.
 */
export default function SegmentSources({
  order,
  defaultOrder,
  off,
  publicMetaDbKey,
  onOrder,
  error,
}: {
  order: SegmentSource[]
  defaultOrder: SegmentSource[]
  off: SegmentSource[]
  /** Whether a PublicMetaDB key is saved or about to be. */
  publicMetaDbKey: boolean
  onOrder: (order: SegmentSource[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const text = t.settings.segmentSources
  const id = useId()
  const [announcement, setAnnouncement] = useState('')
  const customized = order.join() !== defaultOrder.join()

  function move(from: number, to: number) {
    const next = [...order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onOrder(next)
    setAnnouncement(t.common.moved(sourceNames[moved], to + 1, next.length))
  }

  return (
    <div role="group" aria-labelledby={`${id}-label`} aria-describedby={`${id}-help`}>
      <p id={`${id}-label`} className="text-[15px] font-medium tracking-[-0.01em] text-ink">
        {text.label}
      </p>
      <p id={`${id}-help`} className="mt-1 max-w-[60ch] text-small text-ink-3">
        {text.help}
        {off.length > 0 && ` ${text.offHelp}`}
      </p>
      <p className="sr-only" role="status">
        {announcement}
      </p>
      <ol className="mt-3.5 rounded-row border border-line-2 bg-s1">
        {order.map((source, index) => (
          <li
            key={source}
            className="flex min-h-[52px] items-center gap-3.5 py-1.5 pr-2.5 pl-3.5 not-first:border-t not-first:border-line"
          >
            <span aria-hidden="true" className="figures w-4 text-right text-micro text-ink-3">
              {index + 1}
            </span>
            <span
              className={`min-w-0 flex-1 truncate text-body font-medium ${off.includes(source) ? 'text-ink-3' : 'text-ink'}`}
            >
              {sourceNames[source]}
            </span>
            {off.includes(source) ? (
              <StatusPill tone="muted">{text.off}</StatusPill>
            ) : source === 'publicmetadb' && !publicMetaDbKey ? (
              <StatusPill tone="warn">{text.needsKey}</StatusPill>
            ) : (
              <StatusPill tone="ok">{text.on}</StatusPill>
            )}
            <span className="flex">
              <IconButton
                size="sm"
                icon={ArrowUpIcon}
                label={t.common.moveUp(sourceNames[source])}
                disabled={index === 0}
                onClick={() => move(index, index - 1)}
              />
              <IconButton
                size="sm"
                icon={ArrowDownIcon}
                label={t.common.moveDown(sourceNames[source])}
                disabled={index === order.length - 1}
                onClick={() => move(index, index + 1)}
              />
            </span>
          </li>
        ))}
      </ol>
      {customized && (
        <div className="mt-2.5 flex justify-end">
          <Button
            variant="ghost"
            icon={ArrowCounterClockwiseIcon}
            onClick={() => {
              onOrder(defaultOrder)
              setAnnouncement(text.resetDone)
            }}
          >
            {text.reset}
          </Button>
        </div>
      )}
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}
