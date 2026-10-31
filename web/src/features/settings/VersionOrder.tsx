import { DotsSixVerticalIcon, PlusIcon, XIcon } from '@phosphor-icons/react'
import { useId, useState } from 'react'
import type { VersionCodec, VersionCriterion, VersionEdition, VersionSource } from '@/api'
import { useI18n } from '@/i18n'
import { Button, cx, FieldError, IconButton, MoveButtons, Select } from '@/ui'

/** The most criteria the server takes. */
const maxCriteria = 6

/** What a criterion looks at, whichever way it sorts: resolution for both resolution criteria. */
function kind(criterion: VersionCriterion) {
  return criterion.split('-')[0]
}

/**
 * The criteria ordering a title's versions, in order: each row chooses one, moved with the arrows
 * or removed, and a button adds the next kind not chosen yet, up to six. A row offers the
 * criteria of its kind and of the kinds no other row has.
 */
export default function VersionOrder({
  order,
  choices,
  onChange,
  error,
}: {
  order: VersionCriterion[]
  /** Every criterion, in the server's order. */
  choices: VersionCriterion[]
  onChange: (order: VersionCriterion[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const text = t.settings.versionOrder
  const id = useId()
  const [announcement, setAnnouncement] = useState('')
  const unused = choices.filter((choice) => !order.some((chosen) => kind(chosen) === kind(choice)))

  function move(from: number, to: number) {
    const next = [...order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onChange(next)
    setAnnouncement(t.common.moved(text.criteria[moved], to + 1, next.length))
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
      {order.length === 0 ? (
        <p className="mt-3.5 text-small text-ink-2">{text.none}</p>
      ) : (
        <ol className="mt-3.5 rounded-row border border-line-2 bg-s1">
          {order.map((criterion, index) => (
            <li
              key={kind(criterion)}
              className="flex min-h-[52px] items-center gap-3.5 py-1.5 pr-2.5 pl-3.5 not-first:border-t not-first:border-line"
            >
              <span aria-hidden="true" className="figures w-4 text-right text-micro text-ink-3">
                {index + 1}
              </span>
              <Select
                aria-label={text.criterion(index + 1)}
                value={criterion}
                options={choices
                  .filter((choice) => kind(choice) === kind(criterion) || unused.includes(choice))
                  .map((choice) => ({ value: choice, label: text.criteria[choice] }))}
                onValue={(chosen: VersionCriterion) =>
                  onChange(order.map((other, at) => (at === index ? chosen : other)))
                }
                className="min-w-0 flex-1 sm:max-w-sm"
              />
              <MoveButtons
                name={text.criteria[criterion]}
                index={index}
                count={order.length}
                onMove={(to) => move(index, to)}
              />
              <IconButton
                size="sm"
                icon={XIcon}
                danger
                label={text.remove(text.criteria[criterion])}
                onClick={() => onChange(order.filter((_, at) => at !== index))}
              />
            </li>
          ))}
        </ol>
      )}
      <Button
        className="mt-3"
        size="sm"
        icon={PlusIcon}
        disabled={order.length >= maxCriteria || unused.length === 0}
        onClick={() => onChange([...order, unused[0]])}
      >
        {text.add}
      </Button>
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}

/** The order of the video codecs for the codec criterion, moved with the arrows. */
export function CodecOrder({
  order,
  onChange,
  error,
}: {
  order: VersionCodec[]
  onChange: (order: VersionCodec[]) => void
  error?: string
}) {
  const { t } = useI18n()
  return (
    <FixedOrder
      order={order}
      names={t.settings.versionCodecs}
      text={t.settings.versionCodecOrder}
      onChange={onChange}
      error={error}
    />
  )
}

/** The order of the editions for the edition criterion, moved with the arrows. */
export function EditionOrder({
  order,
  onChange,
  error,
}: {
  order: VersionEdition[]
  onChange: (order: VersionEdition[]) => void
  error?: string
}) {
  const { t } = useI18n()
  return (
    <FixedOrder
      order={order}
      names={t.settings.versionEditions}
      text={t.settings.versionEditionOrder}
      onChange={onChange}
      error={error}
    />
  )
}

/** An order of every one of a fixed set of values, moved with the arrows. */
function FixedOrder<T extends string>({
  order,
  names,
  text,
  onChange,
  error,
}: {
  order: T[]
  names: Record<T, string>
  text: { label: string; help: string }
  onChange: (order: T[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const id = useId()
  const [announcement, setAnnouncement] = useState('')

  function move(from: number, to: number) {
    const next = [...order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onChange(next)
    setAnnouncement(t.common.moved(names[moved], to + 1, next.length))
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
      <ol className="mt-3.5 max-w-md rounded-row border border-line-2 bg-s1">
        {order.map((codec, index) => (
          <li
            key={codec}
            className="flex min-h-[44px] items-center gap-3.5 py-1 pr-2.5 pl-3.5 not-first:border-t not-first:border-line"
          >
            <span aria-hidden="true" className="figures w-4 text-right text-micro text-ink-3">
              {index + 1}
            </span>
            <span className="min-w-0 flex-1 truncate text-body text-ink">{names[codec]}</span>
            <MoveButtons
              name={names[codec]}
              index={index}
              count={order.length}
              onMove={(to) => move(index, to)}
            />
          </li>
        ))}
      </ol>
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}

/**
 * The order of the server's sources for the source criterion, moved by drag or with the arrows:
 * those `order` places first, in its order, then the others, in the order of Content › Sources. A
 * move places them all as they are shown.
 */
export function SourceOrder({
  order,
  sources,
  onChange,
  error,
}: {
  /** The IDs of the sources placed. */
  order: string[]
  /** The server's sources listing versions, in the order of Content › Sources. */
  sources: VersionSource[]
  onChange: (order: string[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const text = t.settings.versionSourceOrder
  const id = useId()
  const [announcement, setAnnouncement] = useState('')
  const [dragged, setDragged] = useState<number | null>(null)
  const shown = [
    ...order.flatMap((placed) => sources.filter((source) => source.id === placed)),
    ...sources.filter((source) => !order.includes(source.id)),
  ]

  function move(from: number, to: number) {
    const next = [...shown]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onChange(next.map((source) => source.id))
    setAnnouncement(t.common.moved(moved.name, to + 1, next.length))
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
      {shown.length === 0 ? (
        <p className="mt-3.5 text-small text-ink-2">{text.none}</p>
      ) : (
        <ol className="mt-3.5 max-w-md rounded-row border border-line-2 bg-s1">
          {shown.map((source, index) => (
            <li
              key={source.id}
              draggable
              onDragStart={(event) => {
                event.dataTransfer.effectAllowed = 'move'
                event.dataTransfer.setData('text/plain', source.id)
                setDragged(index)
              }}
              onDragEnd={() => setDragged(null)}
              onDragOver={(event) => dragged !== null && event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault()
                if (dragged !== null && dragged !== index) move(dragged, index)
                setDragged(null)
              }}
              className={cx(
                'flex min-h-[44px] items-center gap-3.5 py-1 pr-2.5 pl-3.5 not-first:border-t not-first:border-line',
                dragged === index && 'opacity-40',
              )}
            >
              <span
                aria-hidden="true"
                title={text.dragHandle(source.name)}
                className="hidden shrink-0 cursor-grab text-ink-3 sm:inline"
              >
                <DotsSixVerticalIcon size={16} />
              </span>
              <span aria-hidden="true" className="figures w-4 text-right text-micro text-ink-3">
                {index + 1}
              </span>
              <span className="min-w-0 flex-1 truncate text-body text-ink">{source.name}</span>
              <MoveButtons
                name={source.name}
                index={index}
                count={shown.length}
                onMove={(to) => move(index, to)}
              />
            </li>
          ))}
        </ol>
      )}
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}
