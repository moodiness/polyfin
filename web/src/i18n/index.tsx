import { createContext, use, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type englishMessages from './en/index'
import type englishSettings from './en/settings'

/*
 * The messages are split into one file per area and language, `en/<area>.ts`, `fr/<area>.ts` and so
 * on, each holding whole top-level keys (`t.users`, `t.lineup`…), and merged by each language's
 * `index.ts`. English is the reference: each other language's file is typed with its English twin,
 * so a missing or extra key fails type checking. Each language is a file of its own, downloaded
 * when the interface first shows it: before the first draw, or before a switch to it shows.
 *
 * Settings' messages (`t.settings`, `t.settingsPage`), over a quarter of a language with their
 * help, are a second file per language, which its `index.ts` leaves out. They download with the
 * pages that read them, Settings, My account, Health and a title's versions
 * (`withSettingsMessages` in `app/router.tsx`), and with the command palette, whose settings it
 * finds by name: see `loadSettingsMessages`. Another page reading them must load with them too, or
 * it fails to draw.
 */

export type Language = 'en' | 'fr' | 'de' | 'es' | 'it' | 'pt-BR'

export const languages: readonly Language[] = ['en', 'fr', 'de', 'es', 'it', 'pt-BR']

/** The messages a language's `index.ts` holds: every area's but Settings'. */
export type UpFrontMessages = typeof englishMessages

/** Every message of the interface, in one language; Settings' once `loadSettingsMessages` is done. */
export type Messages = UpFrontMessages & typeof englishSettings

const upFrontLoaders: Record<Language, () => Promise<{ default: UpFrontMessages }>> = {
  en: () => import('./en/index'),
  fr: () => import('./fr/index'),
  de: () => import('./de/index'),
  es: () => import('./es/index'),
  it: () => import('./it/index'),
  'pt-BR': () => import('./pt-BR/index'),
}

const settingsLoaders: Record<Language, () => Promise<{ default: typeof englishSettings }>> = {
  en: () => import('./en/settings'),
  fr: () => import('./fr/settings'),
  de: () => import('./de/settings'),
  es: () => import('./es/settings'),
  it: () => import('./it/settings'),
  'pt-BR': () => import('./pt-BR/settings'),
}

type Downloads<T> = Partial<Record<Language, Promise<T>>>

/** A download per language, made once; a failed one is made again when asked again. */
function once<T>(downloads: Downloads<T>, language: Language, load: () => Promise<T>): Promise<T> {
  return (downloads[language] ??= load().catch((error: unknown) => {
    delete downloads[language]
    throw error
  }))
}

/** Each language's messages: one object per language, which Settings' join in place. */
const loaded: Downloads<Messages> = {}
const settingsJoined: Downloads<void> = {}

/** Whether a page read Settings' messages: from then on, a switch to a language brings them too. */
let settingsWanted = false

/** A language's messages, downloaded once, with Settings' once a page wanted them. */
async function loadMessages(language: Language): Promise<Messages> {
  const messages = await once(
    loaded,
    language,
    // Settings' are missing until joinSettings adds them.
    async () => ({ ...(await upFrontLoaders[language]()).default }) as Messages,
  )
  if (settingsWanted) await joinSettings(language, messages)
  return messages
}

/**
 * Adds Settings' messages to a language's object, in place, so that the `t` already shown holds
 * them when a page reading them draws.
 */
function joinSettings(language: Language, messages: Messages): Promise<void> {
  return once(settingsJoined, language, async () => {
    const settings = (await settingsLoaders[language]()).default
    // Two areas defining the same top-level key would silently hide one of them.
    if (import.meta.env.DEV) {
      for (const key of Object.keys(settings)) {
        if (Object.hasOwn(messages, key)) throw new Error(`i18n: "${key}" is defined twice`)
      }
    }
    Object.assign(messages, settings)
  })
}

/**
 * Downloads Settings' messages in each language shown or on its way, and makes a later switch to
 * a language bring them as well: a page reading `t.settings` or `t.settingsPage` draws once it is
 * done.
 */
export async function loadSettingsMessages(): Promise<void> {
  settingsWanted = true
  await Promise.all(
    languages.map(async (language) => {
      // A language whose download fails is not shown; a switch to it again joins them.
      const messages = await loaded[language]?.catch(() => undefined)
      if (messages) await joinSettings(language, messages)
    }),
  )
}

/** The language the interface starts in, with its messages: the app draws once they are in. */
export async function loadInitialLanguage(): Promise<ShownLanguage> {
  const language = initialLanguage()
  return { language, t: await loadMessages(language) }
}

const storageKey = 'polyfin.language'

function initialLanguage(): Language {
  try {
    const stored = window.localStorage.getItem(storageKey)
    if (languages.includes(stored as Language)) return stored as Language
  } catch {
    // Storage can be disabled by the browser; fall back to the browser languages.
  }
  for (const tag of navigator.languages ?? [navigator.language]) {
    const language = browserLanguage(tag)
    if (language) return language
  }
  return 'en'
}

/**
 * The language of the interface for a browser language tag, such as `de-AT` or `pt-BR`: its
 * language without the region, except Portuguese, which only Brazilian Portuguese covers.
 */
function browserLanguage(tag: string): Language | undefined {
  const lower = tag.toLowerCase()
  if (lower.startsWith('pt')) return 'pt-BR'
  return languages.find((code) => code !== 'pt-BR' && lower.split('-')[0] === code)
}

/** The language the interface shows, with its messages. */
export type ShownLanguage = { language: Language; t: Messages }

type I18nContextValue = ShownLanguage & {
  /** Shows another language once its messages are in, so that no key ever shows raw. */
  setLanguage: (language: Language) => void
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function LanguageProvider({
  initial,
  children,
}: {
  initial: ShownLanguage
  children: ReactNode
}) {
  const [shown, setShown] = useState(initial)
  // The last language chosen: a slower download of an earlier choice must not replace it.
  const chosen = useRef(initial.language)

  useEffect(() => {
    document.documentElement.lang = shown.language
  }, [shown])

  const value = useMemo<I18nContextValue>(
    () => ({
      ...shown,
      setLanguage: (next) => {
        chosen.current = next
        try {
          window.localStorage.setItem(storageKey, next)
        } catch {
          // The choice still applies for this session when storage is unavailable.
        }
        void loadMessages(next).then((t) => {
          if (chosen.current === next) setShown({ language: next, t })
        })
      },
    }),
    [shown],
  )

  return <I18nContext value={value}>{children}</I18nContext>
}

export function useI18n(): I18nContextValue {
  const value = use(I18nContext)
  if (!value) throw new Error('useI18n must be used inside LanguageProvider')
  return value
}
