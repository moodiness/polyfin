import type { Icon } from '@phosphor-icons/react'
import type { SessionUser } from '@/api'
import type { Messages } from '@/i18n'

/** The groups of results, in the order the palette shows them. */
export const paletteGroups = ['pages', 'settings', 'users', 'sources'] as const

export type PaletteGroup = (typeof paletteGroups)[number]

/** One result of the command palette. */
export type PaletteEntry = {
  /** Unique across the palette: prefix it with your area (`users:42`, `settings:content:segments`). */
  id: string
  group: PaletteGroup
  /** What it is called, as the page or setting shows it. */
  label: string
  /** Where it sits, on the right: "Settings › Content", "Member", "IPTV source". Searched too. */
  hint?: string
  /** More words that find it (synonyms, the English name of a French setting). Never shown. */
  keywords?: readonly string[]
  /** A Phosphor icon, 16 px. */
  icon: Icon
  /** The address Enter opens, with an `#anchor` for a setting inside its section. */
  to: string
}

/** What every entry source receives while the palette is open. */
export type PaletteContext = {
  t: Messages
  user: SessionUser
}

/**
 * Gives entries to the palette. It is called as a React hook each time the open palette draws,
 * so it may use React Query to list users or sources (`useQuery` with the area's existing query
 * key, so the cache is shared with its pages), and return `[]` for users it does not concern.
 */
export type PaletteSource = (context: PaletteContext) => readonly PaletteEntry[]

const sources: { id: string; use: PaletteSource }[] = []

/**
 * Adds a source of entries to the palette. Call it at the top level of
 * `src/features/<area>/palette.ts`: the palette loads every such file, so no other file needs
 * editing. Sources are called in a stable order, as hooks must be.
 *
 *   registerPaletteSource('users', ({ t, user }) => {
 *     const users = useQuery({ queryKey: queryKeys.users, queryFn: …, enabled: user.isAdministrator })
 *     return (users.data ?? []).map((u) => ({ id: `users:${u.id}`, group: 'users', … }))
 *   })
 */
export function registerPaletteSource(id: string, source: PaletteSource) {
  // A module reloaded in development registers again: it replaces its earlier source.
  const existing = sources.find((entry) => entry.id === id)
  if (existing) existing.use = source
  else sources.push({ id, use: source })
}

/** Every registered source, in registration order. */
export function paletteSources(): readonly { id: string; use: PaletteSource }[] {
  return sources
}
