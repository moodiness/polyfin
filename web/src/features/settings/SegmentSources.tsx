import { ArrowDownIcon, ArrowUpIcon } from '@phosphor-icons/react'
import { useId, useState } from 'react'
import type { SegmentSource } from '@/api'
import { useI18n } from '@/i18n'
import { FieldError, IconButton, StatusPill, Switch } from '@/ui'

/** The names the segment databases go by. */
const sourceNames: Record<SegmentSource, string> = {
  theintrodb: 'TheIntroDB',
  introdb: 'IntroDB',
  publicmetadb: 'PublicMetaDB',
}

/**
 * The skip marker sources in their order of preference, moved with the arrows, each with a switch
 * turning it on or off. PublicMetaDB, on without a key, says it needs one.
 */
export default function SegmentSources({
  order,
  off,
  publicMetaDbKey,
  onChange,
  error,
}: {
  order: SegmentSource[]
  off: SegmentSource[]
  /** Whether a PublicMetaDB key is saved or about to be. */
  publicMetaDbKey: boolean
  onChange: (order: SegmentSource[], off: SegmentSource[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const text = t.settings.segmentSources
  const id = useId()
  const [announcement, setAnnouncement] = useState('')

  function move(from: number, to: number) {
    const next = [...order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onChange(next, off)
    setAnnouncement(t.common.moved(sourceNames[moved], to + 1, next.length))
  }

  return (
    <div role="group" aria-labelledby={`${id}-label`} aria-describedby={`${id}-help`}>
      <p id={`${id}-label`} className="text-[15px] font-medium tracking-[-0.01em] text-ink">
        {text.label}
      </p>
      <p id={`${id}-help`} className="mt-1 max-w-[60ch] text-small text-ink-3">
        {text.help}
      </p>
      <p className="sr-only" role="status">
        {announcement}
      </p>
      <ol className="mt-3.5 rounded-row border border-line-2 bg-s1">
        {order.map((source, index) => {
          const on = !off.includes(source)
          return (
            <li
              key={source}
              className="flex min-h-[52px] items-center gap-3.5 py-1.5 pr-2.5 pl-3.5 not-first:border-t not-first:border-line"
            >
              <span aria-hidden="true" className="figures w-4 text-right text-micro text-ink-3">
                {index + 1}
              </span>
              <Switch
                checked={on}
                label={text.turnOn(sourceNames[source])}
                onChange={(checked) =>
                  onChange(
                    order,
                    checked ? off.filter((other) => other !== source) : [...off, source],
                  )
                }
              />
              <span
                className={`min-w-0 flex-1 truncate text-body font-medium ${on ? 'text-ink' : 'text-ink-3'}`}
              >
                {sourceNames[source]}
              </span>
              {on && source === 'publicmetadb' && !publicMetaDbKey && (
                <StatusPill tone="warn">{text.needsKey}</StatusPill>
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
          )
        })}
      </ol>
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}
