import { useId, useMemo, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useI18n } from '@/i18n'

const encoder = new TextEncoder()

/** Spaces a Tab inserts. */
const indent = '  '

/**
 * A monospace text area for code, with line numbers beside it. Tab inserts spaces; after Escape,
 * Tab leaves the field as usual, so that the keyboard never gets stuck in it. Shows the size the
 * server counts, in bytes, against its limit.
 */
export default function CodeEditor({
  label,
  hint,
  value,
  onValue,
  maxBytes,
  rows = 14,
  children,
}: {
  label: string
  hint: string
  value: string
  onValue: (value: string) => void
  maxBytes: number
  rows?: number
  /** Shown between the label and the field, such as a warning. */
  children?: ReactNode
}) {
  const { t } = useI18n()
  const id = useId()
  const gutter = useRef<HTMLPreElement>(null)
  const released = useRef(false)
  const bytes = useMemo(() => encoder.encode(value).length, [value])
  const lines = useMemo(() => {
    let count = 1
    for (const character of value) if (character === '\n') count++
    return Array.from({ length: count }, (_, index) => index + 1).join('\n')
  }, [value])
  const tooLarge = bytes > maxBytes

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Escape') {
      released.current = true
      return
    }
    if (event.key !== 'Tab' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) {
      released.current = false
      return
    }
    if (released.current) {
      released.current = false
      return
    }
    event.preventDefault()
    const field = event.currentTarget
    field.setRangeText(indent, field.selectionStart, field.selectionEnd, 'end')
    onValue(field.value)
  }

  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {label}
      </label>
      {children}
      <div className="mt-1.5 flex overflow-hidden rounded-lg border border-line bg-bg font-mono text-[13px] leading-5 has-[textarea:focus-visible]:border-fin-4">
        <pre
          ref={gutter}
          aria-hidden="true"
          className="m-0 shrink-0 overflow-hidden border-r border-line px-2 py-2 text-right text-zinc-500 select-none"
        >
          {lines}
        </pre>
        <textarea
          id={id}
          value={value}
          rows={rows}
          wrap="off"
          spellCheck={false}
          autoCapitalize="off"
          autoComplete="off"
          onChange={(event) => onValue(event.target.value)}
          onKeyDown={keyDown}
          onScroll={(event) => {
            if (gutter.current) gutter.current.scrollTop = event.currentTarget.scrollTop
          }}
          aria-describedby={`${id}-hint`}
          aria-invalid={tooLarge ? true : undefined}
          className="block min-w-0 flex-1 resize-y bg-transparent px-3 py-2 text-white outline-none"
        />
      </div>
      <p
        id={`${id}-hint`}
        className="mt-1 flex flex-wrap justify-between gap-x-4 text-xs text-muted"
      >
        <span>{hint}</span>
        <span className={tooLarge ? 'text-rose-300' : undefined}>
          {t.settings.codeSize(Math.ceil(bytes / 1024), maxBytes / 1024)}
        </span>
      </p>
    </div>
  )
}
