import { CaretDownIcon, CheckIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '@/i18n'
import { cx, searchable } from './cx'
import { useField } from './Field'
import { control, fieldBox, TextInput, useControlProps, type TextInputProps } from './Input'

/** The tallest a list gets before it scrolls, in px. */
const maxListHeight = 320
/** Between the field and its list, and between the list and the window's edge, in px. */
const gap = 6
const edge = 8
/** Past this many options, a Select's list starts with a search field. */
const searchAbove = 12

/**
 * The bottom of the app's sticky top bar (marked `data-top-bar`) in the window, in px, which a
 * list portaled to the body must stay under; 0 without one, and in a dialog, which covers it.
 */
function topBarBottom(host: HTMLElement) {
  if (host !== document.body) return 0
  return document.querySelector('[data-top-bar]')?.getBoundingClientRect().bottom ?? 0
}

/**
 * The floating surface under a field: portaled to the body (or to the open dialog holding the
 * field, which keeps it usable there) and shown in the top layer, so no table, scroll box or
 * dialog clips it. It sits under the field, above it when there is no room below, at least as
 * wide as the field, and follows it on scroll and resize, never over the app's top bar. A press
 * outside both calls `onDismiss`, as does the field scrolling out of view under the top bar.
 */
function ListPopover({
  anchor,
  onDismiss,
  ref,
  children,
}: {
  anchor: RefObject<HTMLElement | null>
  onDismiss: () => void
  ref: RefObject<HTMLDivElement | null>
  children: ReactNode
}) {
  const [host] = useState(() => anchor.current?.closest('dialog') ?? document.body)
  const dismiss = useRef(onDismiss)
  useLayoutEffect(() => {
    dismiss.current = onDismiss
  })

  useLayoutEffect(() => {
    const list = ref.current
    if (!list) return
    if (typeof list.showPopover === 'function' && !list.matches(':popover-open')) {
      list.showPopover()
    }
    function place() {
      const field = anchor.current
      if (!field || !list) return
      const box = field.getBoundingClientRect()
      const top = topBarBottom(host)
      if (box.bottom <= top) {
        dismiss.current()
        return
      }
      const viewport = document.documentElement
      const width = viewport.clientWidth
      const height = viewport.clientHeight
      list.style.minWidth = `${Math.min(box.width, width - 2 * edge)}px`
      list.style.maxWidth = `${Math.min(width - 2 * edge, Math.max(box.width, 420))}px`
      list.style.maxHeight = `${maxListHeight}px`
      const below = height - box.bottom - gap - edge
      const above = box.top - gap - top - edge
      const up = list.offsetHeight > below && above > below
      list.style.maxHeight = `${Math.max(0, Math.min(maxListHeight, up ? above : below))}px`
      list.style.top = up ? 'auto' : `${box.bottom + gap}px`
      list.style.bottom = up ? `${height - box.top + gap}px` : 'auto'
      list.style.left = `${Math.max(edge, Math.min(box.left, width - edge - list.offsetWidth))}px`
    }
    place()
    const resized = new ResizeObserver(place)
    resized.observe(list)
    if (anchor.current) resized.observe(anchor.current)
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
    function onPointerDown(event: PointerEvent) {
      const target = event.target as Node
      if (!list?.contains(target) && !anchor.current?.contains(target)) dismiss.current()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => {
      resized.disconnect()
      window.removeEventListener('scroll', place, true)
      window.removeEventListener('resize', place)
      document.removeEventListener('pointerdown', onPointerDown, true)
    }
  }, [anchor, ref, host])

  return createPortal(
    <div
      ref={ref}
      popover="manual"
      // A press in the list keeps the focus where it is, unless it lands in the search field.
      onMouseDown={(event) => {
        if (!(event.target instanceof HTMLInputElement)) event.preventDefault()
      }}
      className="fixed inset-auto z-50 m-0 flex animate-fade flex-col overflow-hidden rounded-row border-0 bg-s2 p-0 text-ink shadow-pop"
    >
      {children}
    </div>,
    host,
  )
}

/** Scrolls the list so the option with this id shows. */
function useActiveInView(list: RefObject<HTMLElement | null>, activeId: string | undefined) {
  useLayoutEffect(() => {
    const scroller = list.current
    const item = activeId === undefined ? null : document.getElementById(activeId)
    if (!scroller || !item) return
    if (item.offsetTop < scroller.scrollTop) scroller.scrollTop = item.offsetTop - gap
    else if (item.offsetTop + item.offsetHeight > scroller.scrollTop + scroller.clientHeight) {
      scroller.scrollTop = item.offsetTop + item.offsetHeight - scroller.clientHeight + gap
    }
  }, [list, activeId])
}

const listboxClass = 'relative min-h-0 flex-1 overflow-y-auto overscroll-contain p-1.5'

/** One choice of a list. Pointer moves make it the active one; a press does not move the focus. */
function ListOption({
  id,
  active,
  selected,
  checked = false,
  disabled = false,
  onActivate,
  onChoose,
  children,
}: {
  id: string
  active: boolean
  selected: boolean
  /** Draws the check mark of the chosen option. */
  checked?: boolean
  disabled?: boolean
  onActivate: () => void
  onChoose: () => void
  children: ReactNode
}) {
  return (
    <div
      id={id}
      role="option"
      aria-selected={selected}
      aria-disabled={disabled || undefined}
      data-active={active || undefined}
      onPointerMove={() => {
        if (!active && !disabled) onActivate()
      }}
      onClick={() => {
        if (!disabled) onChoose()
      }}
      className={cx(
        'flex min-h-9 cursor-pointer items-center gap-2.5 rounded-field px-2.5 py-1.5 text-control text-ink-2 transition-[background-color,color] duration-160 ease-nuit select-none',
        'data-active:bg-s3 data-active:text-ink active:bg-s4',
        checked && 'text-ink',
        disabled && 'cursor-not-allowed opacity-45 active:bg-transparent',
      )}
    >
      <span className="min-w-0 flex-1 break-words">{children}</span>
      {checked && <CheckIcon size={16} aria-hidden="true" className="shrink-0 text-link" />}
    </div>
  )
}

export type SelectOption<T extends string | number> = {
  value: T
  label: string
  disabled?: boolean
}

export type SelectProps<T extends string | number> = {
  /** The chosen value. */
  value: T
  /** The choices, in order. */
  options: readonly SelectOption<T>[]
  /** Called with the value chosen, typed like the options. */
  onValue: (value: T) => void
  disabled?: boolean
  invalid?: boolean
  id?: string
  'aria-label'?: string
  'aria-describedby'?: string
  /** Classes of the outer box (width, height). */
  className?: string
}

/**
 * A list to choose one value from: a button styled like the other fields, showing the choice,
 * that opens a listbox with a check on the chosen option. Past 12 options the list starts with a
 * search field, accents and case ignored.
 *
 * Keyboard: ↓ ↑ Enter or Space opens it on the chosen option; ↓ ↑ Home End Page Up Page Down
 * move; Enter (or Space outside the search) chooses; Escape closes it back on the button; Tab
 * closes it and moves on; typing a letter jumps to the next option starting with it. A press
 * outside closes it.
 */
export function Select<T extends string | number>({
  value,
  options,
  onValue,
  disabled,
  invalid,
  id,
  'aria-describedby': ownDescribedBy,
  'aria-label': ariaLabel,
  className,
}: SelectProps<T>) {
  const { t } = useI18n()
  const controlProps = useControlProps(id, ownDescribedBy, invalid)
  const ownId = useId()
  const triggerId = controlProps.id ?? `${ownId}-button`
  const listId = `${ownId}-list`
  const optionId = (index: number) => `${ownId}-option-${index}`
  const box = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const popover = useRef<HTMLDivElement>(null)
  const listbox = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const typed = useRef({ text: '', at: 0 })
  /** Opened by a finger: the search is not focused, which would bring up the keyboard. */
  const byTouch = useRef(false)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(-1)

  const withSearch = options.length > searchAbove
  const chosen = options.findIndex((option) => String(option.value) === String(value))
  const shown = matching(query)
  const usable = shown.filter((index) => !options[index].disabled)
  const activeId = open && shown.includes(active) ? optionId(active) : undefined

  useActiveInView(listbox, activeId)
  useLayoutEffect(() => {
    if (open && withSearch && !byTouch.current) search.current?.focus()
  }, [open, withSearch])

  function matching(text: string): number[] {
    const needle = searchable(text.trim())
    const all = options.map((_, index) => index)
    if (!withSearch || needle === '') return all
    return all.filter((index) => searchable(options[index].label).includes(needle))
  }

  function show(at: number, text = '') {
    setQuery(text)
    setActive(at)
    setOpen(true)
  }

  function close(focusButton: boolean) {
    setOpen(false)
    setQuery('')
    if (focusButton) trigger.current?.focus()
  }

  function choose(index: number) {
    const option = options[index]
    if (!option || option.disabled) return
    close(true)
    if (index !== chosen) onValue(option.value)
  }

  /** The usable option `delta` steps from the active one, stopping at the ends. */
  function step(delta: number): number {
    if (usable.length === 0) return -1
    const at = usable.indexOf(active)
    if (at < 0) return delta > 0 ? usable[0] : usable[usable.length - 1]
    return usable[Math.max(0, Math.min(usable.length - 1, at + delta))]
  }

  /**
   * The usable option starting with what was typed in the last half second, from `from` on.
   * Typing the same letter again moves on to the next option starting with it.
   */
  function typeahead(key: string, from: number): number {
    const now = Date.now()
    const text = now - typed.current.at > 500 ? key : typed.current.text + key
    typed.current = { text, at: now }
    const needle = searchable(text)
    const sameLetter = [...needle].every((letter) => letter === needle[0])
    const prefix = sameLetter ? needle[0] : needle
    const start = sameLetter ? from + 1 : Math.max(0, from)
    for (let offset = 0; offset < options.length; offset++) {
      const index = (start + offset) % options.length
      const option = options[index]
      if (!option.disabled && searchable(option.label).startsWith(prefix)) return index
    }
    return -1
  }

  function firstUsable(among: number[]): number {
    return among.find((index) => !options[index].disabled) ?? -1
  }

  /** The option to highlight for a search: the first starting with it, else the first found. */
  function bestMatch(text: string): number {
    const found = matching(text).filter((index) => !options[index].disabled)
    const needle = searchable(text.trim())
    return (
      found.find((index) => searchable(options[index].label).startsWith(needle)) ?? found[0] ?? -1
    )
  }

  function onKeyDown(event: KeyboardEvent<HTMLElement>, inSearch: boolean) {
    const { key } = event
    const letter = key.length === 1 && key !== ' ' && !event.ctrlKey && !event.metaKey
    if (!open) {
      const all = options.map((_, index) => index)
      if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(key)) {
        event.preventDefault()
        show(chosen >= 0 ? chosen : firstUsable(all))
      } else if (key === 'Home' || key === 'End') {
        event.preventDefault()
        show(firstUsable(key === 'Home' ? all : all.reverse()))
      } else if (letter && !event.altKey) {
        event.preventDefault()
        if (withSearch) show(bestMatch(key), key)
        else {
          const found = typeahead(key, chosen)
          show(found >= 0 ? found : chosen)
        }
      }
      return
    }
    let next: number | undefined
    if (key === 'ArrowDown') next = event.altKey ? active : step(1)
    else if (key === 'ArrowUp') next = event.altKey ? active : step(-1)
    else if (key === 'PageDown') next = step(10)
    else if (key === 'PageUp') next = step(-10)
    else if (key === 'Home') next = usable[0] ?? -1
    else if (key === 'End') next = usable[usable.length - 1] ?? -1
    else if (key === 'Enter' || (key === ' ' && !inSearch)) {
      event.preventDefault()
      if (usable.includes(active)) choose(active)
      else close(true)
      return
    } else if (key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close(true)
      return
    } else if (key === 'Tab') {
      // From the search, the focus goes back to the button first, so Tab moves on from there.
      close(inSearch)
      return
    } else if (letter && !inSearch && !event.altKey) {
      if (withSearch) {
        // Opened by touch, the focus stayed on the button: the letters go to the search.
        event.preventDefault()
        const text = query + key
        setQuery(text)
        setActive(bestMatch(text))
        search.current?.focus()
        return
      }
      const found = typeahead(key, active)
      if (found >= 0) next = found
    }
    if (next !== undefined) {
      event.preventDefault()
      setActive(next)
    }
  }

  function onBlur(event: FocusEvent<HTMLElement>) {
    const to = event.relatedTarget as Node | null
    if (open && !box.current?.contains(to) && !popover.current?.contains(to)) close(false)
  }

  return (
    <div
      ref={box}
      className={cx(fieldBox, 'h-10', open && 'border-accent ring-3 ring-accent/15', className)}
    >
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={activeId}
        aria-label={ariaLabel}
        disabled={disabled}
        {...controlProps}
        id={triggerId}
        onPointerDown={(event) => {
          byTouch.current = event.pointerType === 'touch'
        }}
        onClick={() => {
          if (open) close(true)
          else show(chosen >= 0 ? chosen : firstUsable(options.map((_, index) => index)))
        }}
        onKeyDown={(event) => {
          byTouch.current = false
          onKeyDown(event, false)
        }}
        onBlur={onBlur}
        className={cx(control, 'flex cursor-pointer items-center pr-9 text-left')}
      >
        <span className="min-w-0 truncate">{chosen >= 0 ? options[chosen].label : ''}</span>
      </button>
      <CaretDownIcon
        size={14}
        aria-hidden="true"
        className={cx(
          'pointer-events-none absolute right-3 text-ink-3 transition-transform duration-160 ease-nuit',
          open && 'rotate-180',
        )}
      />
      {open && (
        <ListPopover anchor={box} ref={popover} onDismiss={() => close(false)}>
          {withSearch && (
            <div className="border-b border-line p-1.5">
              <div className={cx(fieldBox, 'h-[34px]')}>
                <MagnifyingGlassIcon
                  size={16}
                  aria-hidden="true"
                  className="ml-3 shrink-0 text-ink-3"
                />
                <input
                  ref={search}
                  type="text"
                  role="combobox"
                  aria-autocomplete="list"
                  aria-expanded="true"
                  aria-controls={listId}
                  aria-activedescendant={activeId}
                  aria-label={t.ui.searchList}
                  placeholder={t.ui.searchList}
                  autoComplete="off"
                  spellCheck={false}
                  value={query}
                  onChange={(event) => {
                    const text = event.target.value
                    setQuery(text)
                    setActive(bestMatch(text))
                  }}
                  onKeyDown={(event) => onKeyDown(event, true)}
                  onBlur={onBlur}
                  className={cx(control, 'pl-2 text-control')}
                />
              </div>
            </div>
          )}
          <div
            ref={listbox}
            id={listId}
            role="listbox"
            aria-labelledby={ariaLabel === undefined ? triggerId : undefined}
            aria-label={ariaLabel}
            className={listboxClass}
          >
            {shown.map((index) => (
              <ListOption
                key={index}
                id={optionId(index)}
                active={index === active}
                selected={index === chosen}
                checked={index === chosen}
                disabled={options[index].disabled}
                onActivate={() => setActive(index)}
                onChoose={() => choose(index)}
              >
                {options[index].label}
              </ListOption>
            ))}
            {shown.length === 0 && (
              <p role="status" className="px-2.5 py-2 text-small text-ink-3">
                {t.ui.noMatch}
              </p>
            )}
          </div>
        </ListPopover>
      )}
    </div>
  )
}

export type SuggestInputProps = Omit<TextInputProps, 'role' | 'list'> & {
  /** What may be typed. Those containing the text so far show under the field. */
  suggestions: readonly string[]
  /** Called with the suggestion picked, by a click or Enter. */
  onPick: (value: string) => void
}

/**
 * A text field offering suggestions under it as one types (a combobox). ↓ ↑ move through them,
 * Enter or a click picks one, Escape closes them. Enter with none highlighted goes to the field's
 * own `onKeyDown`, so free text can still be added. Suggestions match anywhere, accents and case
 * ignored, those starting with the text first.
 */
export function SuggestInput({
  suggestions,
  onPick,
  onValue,
  onKeyDown,
  onBlur,
  value,
  ...rest
}: SuggestInputProps) {
  const field = useField()
  const ownId = useId()
  const inputId = rest.id ?? field?.id
  const listId = `${ownId}-list`
  const optionId = (index: number) => `${ownId}-option-${index}`
  const input = useRef<HTMLInputElement>(null)
  const anchor = useRef<HTMLElement | null>(null)
  const popover = useRef<HTMLDivElement>(null)
  const listbox = useRef<HTMLDivElement>(null)
  const [wanted, setWanted] = useState(false)
  const [active, setActive] = useState(-1)

  const text = typeof value === 'string' ? value : ''
  const needle = searchable(text.trim())
  const shown = suggestions
    .map((suggestion) => ({ suggestion, key: searchable(suggestion) }))
    .filter(({ key }) => key.includes(needle))
    .sort((a, b) => Number(b.key.startsWith(needle)) - Number(a.key.startsWith(needle)))
    .map(({ suggestion }) => suggestion)
  const open = wanted && shown.length > 0
  const activeId = open && active >= 0 && active < shown.length ? optionId(active) : undefined

  useActiveInView(listbox, activeId)
  useEffect(() => {
    anchor.current = input.current?.parentElement ?? null
  }, [])

  function show() {
    setWanted(true)
    setActive(-1)
  }

  function pick(suggestion: string) {
    setWanted(false)
    setActive(-1)
    onPick(suggestion)
  }

  return (
    <>
      <TextInput
        {...rest}
        ref={input}
        value={value}
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={activeId}
        autoComplete="off"
        onValue={(next) => {
          onValue?.(next)
          show()
        }}
        onClick={() => {
          if (!open) show()
        }}
        onKeyDown={(event) => {
          const { key } = event
          if (key === 'ArrowDown' || key === 'ArrowUp') {
            event.preventDefault()
            if (!open) {
              show()
              return
            }
            const last = shown.length - 1
            if (key === 'ArrowDown') setActive(active >= last ? 0 : active + 1)
            else setActive(active <= 0 ? last : active - 1)
            return
          }
          if (open && key === 'Enter' && activeId !== undefined) {
            event.preventDefault()
            pick(shown[active])
            return
          }
          if (open && key === 'Escape') {
            event.preventDefault()
            event.stopPropagation()
            setWanted(false)
            return
          }
          if (key === 'Tab' || key === 'Enter') setWanted(false)
          onKeyDown?.(event)
        }}
        onBlur={(event) => {
          if (!popover.current?.contains(event.relatedTarget as Node | null)) setWanted(false)
          onBlur?.(event)
        }}
      />
      {open && (
        <ListPopover anchor={anchor} ref={popover} onDismiss={() => setWanted(false)}>
          <div
            ref={listbox}
            id={listId}
            role="listbox"
            aria-labelledby={inputId}
            aria-label={inputId === undefined ? rest['aria-label'] : undefined}
            className={listboxClass}
          >
            {shown.map((suggestion, index) => (
              <ListOption
                key={suggestion}
                id={optionId(index)}
                active={index === active}
                selected={index === active}
                onActivate={() => setActive(index)}
                onChoose={() => pick(suggestion)}
              >
                {suggestion}
              </ListOption>
            ))}
          </div>
        </ListPopover>
      )}
    </>
  )
}
