import { ArrowDownIcon, ArrowUpIcon } from '@phosphor-icons/react'
import { useEffect, useId, useRef } from 'react'
import { useI18n } from '@/i18n'
import { IconButton } from './Button'

/**
 * Up and down buttons for one item of an ordered list. After a move, focus follows the item (or
 * jumps to its other button once it reaches an end) so it can be moved again from the keyboard.
 */
export function MoveButtons({
  name,
  index,
  count,
  disabled = false,
  onMove,
}: {
  /** The item's name, for the buttons' labels: "Move Popular up". */
  name: string
  index: number
  count: number
  disabled?: boolean
  /** Called with the index the item moves to. */
  onMove: (to: number) => void
}) {
  const { t } = useI18n()
  const id = useId()
  const upId = `${id}-up`
  const downId = `${id}-down`
  const moved = useRef<'up' | 'down' | null>(null)
  const first = index === 0
  const last = index === count - 1

  useEffect(() => {
    const direction = moved.current
    if (direction === null) return
    moved.current = null
    const target = direction === 'up' ? (first ? downId : upId) : last ? upId : downId
    document.getElementById(target)?.focus()
  }, [index, first, last, upId, downId])

  return (
    <span className="flex shrink-0">
      <IconButton
        id={upId}
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
        id={downId}
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
