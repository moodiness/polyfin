import { MagnifyingGlassIcon } from '@phosphor-icons/react'
import {
  createContext,
  use,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { useNavigate } from 'react-router'
import { useSessionUser } from '@/app/session'
import { useI18n } from '@/i18n'
import { cx, Kbd, searchable } from '@/ui'
import './navigationEntries'
import { paletteGroups, paletteSources, type PaletteEntry } from './registry'

// Each area registers its entries in `src/features/<area>/palette.ts`; loading them here is all
// it takes for the palette to offer them.
import.meta.glob('/src/features/*/palette.ts', { eager: true })

const PaletteContext = createContext<() => void>(() => {})

/** Opens the command palette, for a search button. */
export function useOpenPalette(): () => void {
  return use(PaletteContext)
}

/** Whether a key press happens while typing, where "/" must stay a character. */
function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target.matches('input:not([type=checkbox],[type=radio],[type=button]), textarea, select')
  )
}

/**
 * Holds the command palette of the signed-in shell, and its shortcuts: ⌘K or Ctrl K anywhere,
 * and "/" outside a text field.
 */
export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)

  useEffect(() => {
    function onKeyDown(event: globalThis.KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setOpen((current) => !current)
        return
      }
      if (
        event.key === '/' &&
        !event.metaKey &&
        !event.ctrlKey &&
        !event.altKey &&
        !isTyping(event.target) &&
        // Not over another modal (a dialog or the phone menu).
        document.querySelector('dialog[open]') === null
      ) {
        event.preventDefault()
        setOpen(true)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])

  return (
    <PaletteContext value={() => setOpen(true)}>
      {children}
      <CommandPalette open={open} onClose={() => setOpen(false)} />
    </PaletteContext>
  )
}

function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n()
  const dialog = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) element.showModal()
    if (!open && element.open) element.close()
  }, [open])

  return (
    <dialog
      ref={dialog}
      aria-label={t.palette.label}
      onCancel={(event) => {
        event.preventDefault()
        onClose()
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
      className="mx-auto mt-[14vh] w-[600px] max-w-[calc(100vw-32px)] overflow-hidden rounded-panel bg-s2 p-0 text-ink shadow-pop backdrop:bg-scrim backdrop:backdrop-blur-[3px] open:animate-pop max-md:mt-4"
    >
      {open && <PaletteBody onClose={onClose} />}
    </dialog>
  )
}

/** How well an entry matches: -1 not at all, 0 by its hint or keywords, 1 in its label, 2 at its start. */
function score(entry: PaletteEntry, words: readonly string[]): number {
  if (words.length === 0) return 1
  const label = searchable(entry.label)
  const haystack = [
    label,
    searchable(entry.hint ?? ''),
    ...(entry.keywords ?? []).map(searchable),
  ].join(' ')
  if (!words.every((word) => haystack.includes(word))) return -1
  if (label.startsWith(words[0])) return 2
  return words.every((word) => label.includes(word)) ? 1 : 0
}

/** Without a search, each group shows its first entries only. */
const browseLimit = 6
const searchLimit = 8

function PaletteBody({ onClose }: { onClose: () => void }) {
  const { t } = useI18n()
  const user = useSessionUser()
  const navigate = useNavigate()
  const id = useId()
  const list = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(0)

  // Each source is a hook: they run in registration order on every draw of the open palette.
  const entries = paletteSources().flatMap((source) => source.use({ t, user }))

  // Every word typed must be found, without regard to accents or case.
  const words = searchable(query.trim()).split(/\s+/).filter(Boolean)
  const groups = paletteGroups
    .map((group) => {
      const matches = entries
        .filter((entry) => entry.group === group)
        .map((entry) => ({ entry, score: score(entry, words) }))
        .filter((match) => match.score >= 0)
        .sort((a, b) => b.score - a.score)
        .map((match) => match.entry)
      const limit = words.length === 0 ? (group === 'pages' ? Infinity : browseLimit) : searchLimit
      return { group, entries: matches.slice(0, limit) }
    })
    .filter((group) => group.entries.length > 0)

  const flat = groups.flatMap((group) => group.entries)
  const current = Math.min(selected, Math.max(0, flat.length - 1))
  const optionId = (index: number) => `${id}-option-${index}`

  // The selected entry stays in view as the arrows move through the list.
  useEffect(() => {
    list.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [current])

  function go(entry: PaletteEntry | undefined) {
    if (!entry) return
    onClose()
    void navigate(entry.to)
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (flat.length === 0) return
      const step = event.key === 'ArrowDown' ? 1 : -1
      setSelected((current + step + flat.length) % flat.length)
    } else if (event.key === 'Enter') {
      event.preventDefault()
      go(flat[current])
    }
  }

  let index = -1
  return (
    <>
      <div className="flex h-14 items-center gap-3 border-b border-line px-4">
        <MagnifyingGlassIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />
        <input
          autoFocus
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls={`${id}-list`}
          aria-activedescendant={flat.length > 0 ? optionId(current) : undefined}
          aria-autocomplete="list"
          aria-label={t.palette.input}
          placeholder={t.palette.placeholder}
          autoComplete="off"
          spellCheck={false}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            setSelected(0)
          }}
          onKeyDown={onKeyDown}
          className="h-full min-w-0 flex-1 bg-transparent text-[15.5px] text-ink outline-none placeholder:text-ink-3 focus-visible:outline-none"
        />
        <Kbd>{t.palette.escape}</Kbd>
      </div>
      <div
        ref={list}
        id={`${id}-list`}
        role="listbox"
        aria-label={t.palette.label}
        className="max-h-[min(380px,60dvh)] overflow-y-auto p-2"
      >
        {flat.length === 0 ? (
          <p className="px-2.5 py-8 text-center text-control text-ink-3">
            {t.palette.empty(query.trim())}
          </p>
        ) : (
          groups.map((group) => (
            <div key={group.group} role="group" aria-labelledby={`${id}-${group.group}`}>
              <p
                id={`${id}-${group.group}`}
                className="px-2.5 pt-2.5 pb-1.5 text-micro font-medium text-ink-3"
              >
                {t.palette.groups[group.group]}
              </p>
              {group.entries.map((entry) => {
                index += 1
                const position = index
                const isSelected = position === current
                return (
                  <div
                    key={entry.id}
                    id={optionId(position)}
                    role="option"
                    aria-selected={isSelected}
                    onMouseMove={() => {
                      if (!isSelected) setSelected(position)
                    }}
                    onClick={() => go(entry)}
                    className={cx(
                      'flex h-10 cursor-pointer items-center gap-3 rounded-field px-2.5 text-body',
                      isSelected ? 'bg-s3 text-ink' : 'text-ink-2',
                    )}
                  >
                    <entry.icon
                      size={16}
                      aria-hidden="true"
                      className={cx('shrink-0', isSelected ? 'text-link' : 'text-ink-3')}
                    />
                    <span className="min-w-0 flex-1 truncate">{entry.label}</span>
                    {entry.hint && (
                      <span className="max-w-[45%] shrink-0 truncate text-[12.5px] text-ink-3">
                        {entry.hint}
                      </span>
                    )}
                  </div>
                )
              })}
            </div>
          ))
        )}
      </div>
      <div className="flex gap-4 border-t border-line px-4 py-2.5 text-micro text-ink-3 max-md:hidden">
        <span className="inline-flex items-center gap-1.5">
          <Kbd>↑</Kbd>
          <Kbd>↓</Kbd>
          {t.palette.choose}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <Kbd>↵</Kbd>
          {t.palette.open}
        </span>
      </div>
    </>
  )
}
