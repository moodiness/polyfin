import { createContext, use, useId, type ReactNode } from 'react'

/** Lower case without accents, so that a search for "resume" finds "Résumé". */
export function searchable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}

/** The search typed, made searchable; empty shows every setting. */
export const SearchContext = createContext('')

/** One setting, hidden while the search matches none of its words. */
export function Setting({ text, children }: { text: readonly string[]; children: ReactNode }) {
  const query = use(SearchContext)
  const shown = query === '' || searchable(text.join(' ')).includes(query)
  return (
    <div data-setting="" hidden={!shown}>
      {children}
    </div>
  )
}

/** A titled group of settings within a section, hidden when the search leaves none of them. */
export function SettingsGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-6 pt-3 first:pt-0 [&:not(:has([data-setting]:not([hidden])))]:hidden">
      <h3 className="text-sm font-semibold tracking-wide text-fin-5 uppercase">{title}</h3>
      {children}
    </div>
  )
}

/** A labelled list to choose a value from; options may be disabled. */
export function SelectField<T extends string | number>({
  label,
  hint,
  value,
  options,
  onValue,
  disabled,
}: {
  label: string
  hint?: string
  value: T
  options: readonly { value: T; label: string; disabled?: boolean }[]
  onValue: (value: T) => void
  disabled?: boolean
}) {
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {label}
      </label>
      <select
        id={id}
        value={String(value)}
        disabled={disabled}
        onChange={(event) => {
          const chosen = options.find((option) => String(option.value) === event.target.value)
          if (chosen) onValue(chosen.value)
        }}
        aria-describedby={hint ? `${id}-hint` : undefined}
        className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white disabled:opacity-50"
      >
        {options.map((option) => (
          <option
            key={String(option.value)}
            value={String(option.value)}
            disabled={option.disabled}
          >
            {option.label}
          </option>
        ))}
      </select>
      {hint && (
        <p id={`${id}-hint`} className="mt-1 text-xs text-muted">
          {hint}
        </p>
      )}
    </div>
  )
}

/**
 * A whole number typed in a field where 0 is a valid value: an empty field is -1, which the server
 * refuses, rather than 0, which would turn the setting off without the user typing it.
 */
export function wholeNumber(value: string): number {
  return value.trim() === '' ? -1 : Math.trunc(Number(value))
}

/** The text of a field holding a whole number; -1 (see wholeNumber) shows as empty. */
export function wholeNumberField(value: number): number | '' {
  return value < 0 ? '' : value
}
