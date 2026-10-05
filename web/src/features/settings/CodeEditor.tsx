import { useId, useMemo, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { cx, FieldError } from '@/ui'

const encoder = new TextEncoder()

/** Spaces a Tab inserts. */
const indent = '  '

/**
 * A monospace text area for code, with line numbers beside it. Tab inserts spaces; after Escape,
 * Tab leaves the field as usual, so that the keyboard never gets stuck in it. Shows the size the
 * server counts, in bytes, against its limit, and the server's error under it. The box keeps the
 * height of `rows` lines whatever the text's length (its handle resizes it) and scrolls inside,
 * the line numbers following the text.
 */
export default function CodeEditor({
  label,
  help,
  value,
  onValue,
  maxBytes,
  rows = 19,
  error,
  children,
}: {
  label: string
  help: string
  value: string
  onValue: (value: string) => void
  maxBytes: number
  rows?: number
  error?: string
  /** Shown between the help and the field, such as a warning. */
  children?: ReactNode
}) {
  const { t } = useI18n()
  const id = useId()
  const gutter = useRef<HTMLPreElement>(null)
  const released = useRef(false)
  const bytes = useMemo(() => encoder.encode(value).length, [value])
  const lines = useMemo(() => {
    const count = value.split('\n').length
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
    <div className="flex flex-col gap-2">
      <label htmlFor={id} className="text-[15px] font-medium tracking-[-0.01em] text-ink">
        {label}
      </label>
      <p id={`${id}-help`} className="-mt-1 max-w-[60ch] text-small text-ink-3">
        {help}
      </p>
      {children}
      <div
        // Line height 20px, 20px of padding, 12px for a horizontal scrollbar.
        style={{ height: rows * 20 + 32 }}
        className={cx(
          'flex min-h-24 resize-y overflow-hidden rounded-field border bg-s2 font-mono text-small leading-5 transition-colors duration-160 ease-nuit has-[textarea:focus-visible]:border-link',
          error || tooLarge ? 'border-danger' : 'border-line-2',
        )}
      >
        <pre
          ref={gutter}
          aria-hidden="true"
          // Its bottom padding outlasts the text area's horizontal scrollbar, so the last numbers
          // can scroll as far as the last lines.
          className="figures m-0 shrink-0 overflow-hidden border-r border-line px-2 pt-2.5 pb-8 text-right text-ink-3 select-none"
        >
          {lines}
        </pre>
        <textarea
          id={id}
          value={value}
          wrap="off"
          spellCheck={false}
          autoCapitalize="off"
          autoComplete="off"
          onChange={(event) => onValue(event.target.value)}
          onKeyDown={keyDown}
          onScroll={(event) => {
            if (gutter.current) gutter.current.scrollTop = event.currentTarget.scrollTop
          }}
          aria-describedby={`${id}-help ${id}-size${error ? ` ${id}-error` : ''}`}
          aria-invalid={tooLarge || error ? true : undefined}
          className="block h-full min-w-0 flex-1 resize-none overflow-auto bg-transparent px-3 py-2.5 text-ink outline-none scrollbar-thin"
        />
      </div>
      <p
        id={`${id}-size`}
        className="flex flex-wrap justify-between gap-x-4 gap-y-1 text-micro text-ink-3"
      >
        <span>{t.settings.codeKeys}</span>
        <span className={cx('figures', tooLarge && 'text-danger')}>
          {t.settings.codeSize(Math.ceil(bytes / 1024), maxBytes / 1024)}
        </span>
      </p>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
  )
}
