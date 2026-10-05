import { useId, useState } from 'react'
import type { SegmentSource } from '@/api'
import { Badge, MoveButtons, buttonSecondary } from '@/components/ui'
import { useI18n } from '@/i18n'

/** The names the segment databases go by. */
const sourceNames: Record<SegmentSource, string> = {
  theintrodb: 'TheIntroDB',
  introdb: 'IntroDB',
  publicmetadb: 'PublicMetaDB',
}

/**
 * The order of preference of the skip marker sources, to move up and down, each with its state:
 * asked, waiting for its key, or turned off by POLYFIN_SEGMENTS. The order the variable gives can be
 * restored while another is chosen.
 */
export default function SegmentSources({
  order,
  defaultOrder,
  off,
  publicMetaDbKey,
  onOrder,
}: {
  order: SegmentSource[]
  defaultOrder: SegmentSource[]
  off: SegmentSource[]
  /** Whether a PublicMetaDB key is saved or about to be. */
  publicMetaDbKey: boolean
  onOrder: (order: SegmentSource[]) => void
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

  function state(source: SegmentSource) {
    if (off.includes(source)) {
      return <Badge tone="muted">{text.off}</Badge>
    }
    if (source === 'publicmetadb' && !publicMetaDbKey) {
      return <Badge tone="warning">{text.needsKey}</Badge>
    }
    return <Badge tone="ok">{text.on}</Badge>
  }

  return (
    <div role="group" aria-labelledby={`${id}-label`} aria-describedby={`${id}-hint`}>
      <p id={`${id}-label`} className="text-sm font-medium text-zinc-200">
        {text.label}
      </p>
      <p className="sr-only" role="status">
        {announcement}
      </p>
      <ol className="mt-1.5 divide-y divide-line rounded-lg border border-line bg-ink">
        {order.map((source, index) => (
          <li key={source} className="flex items-center gap-3 px-3 py-2">
            <span className="w-5 text-right text-sm text-muted tabular-nums" aria-hidden="true">
              {index + 1}
            </span>
            <span
              className={`min-w-0 flex-1 text-sm ${off.includes(source) ? 'text-muted' : 'text-white'}`}
            >
              {sourceNames[source]}
            </span>
            {state(source)}
            <MoveButtons
              name={sourceNames[source]}
              index={index}
              count={order.length}
              onMove={(to) => move(index, to)}
            />
          </li>
        ))}
      </ol>
      <p id={`${id}-hint`} className="mt-1 text-xs text-muted">
        {text.help}
        {off.length > 0 && ` ${text.offHelp}`}
      </p>
      {customized && (
        <button
          type="button"
          className={`${buttonSecondary} mt-2`}
          onClick={() => {
            onOrder(defaultOrder)
            setAnnouncement(text.resetDone)
          }}
        >
          {text.reset}
        </button>
      )}
    </div>
  )
}
