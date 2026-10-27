import { useState } from 'react'
import { Textarea } from '@/ui'

/**
 * A list of entries, one a line, such as the words hiding versions or the trackers added to every
 * torrent. The text stays as typed while it lists the same entries, so that a new line can be
 * started; discarding or saving writes it anew.
 */
export default function WordList({
  value,
  onValue,
}: {
  value: string[]
  onValue: (value: string[]) => void
}) {
  const [text, setText] = useState(() => value.join('\n'))
  const words = (typed: string) =>
    typed
      .split('\n')
      .map((word) => word.trim())
      .filter(Boolean)
  if (words(text).join('\n') !== value.join('\n')) {
    setText(value.join('\n'))
  }
  return (
    <Textarea
      rows={3}
      value={text}
      autoComplete="off"
      spellCheck={false}
      className="max-w-md"
      onValue={(next) => {
        setText(next)
        onValue(words(next))
      }}
    />
  )
}
