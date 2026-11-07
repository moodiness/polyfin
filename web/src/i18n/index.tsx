import { createContext, use, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type englishMessages from './en/index'

/*
 * The messages are split into one file per area and language, `en/<area>.ts`, `fr/<area>.ts` and so
 * on, each holding whole top-level keys (`t.users`, `t.lineup`…), and merged by each language's
 * `index.ts`. English is the reference: each other language's file is typed with its English twin,
 * so a missing or extra key fails type checking. Each language is a file of its own, downloaded
 * when the interface first shows it: before the first draw, or before a switch to it shows.
 */

export type Language = 'en' | 'fr' | 'de' | 'es' | 'it' | 'pt-BR'

export const languages: readonly Language[] = ['en', 'fr', 'de', 'es', 'it', 'pt-BR']

/** Every message of the interface, in one language. */
export type Messages = typeof englishMessages

const loaders: Record<Language, () => Promise<{ default: Messages }>> = {
  en: () => import('./en/index'),
  fr: () => import('./fr/index'),
  de: () => import('./de/index'),
  es: () => import('./es/index'),
  it: () => import('./it/index'),
  'pt-BR': () => import('./pt-BR/index'),
}

const loaded: Partial<Record<Language, Messages>> = {}

/** A language's messages, downloaded once. */
async function loadMessages(language: Language): Promise<Messages> {
  return (loaded[language] ??= (await loaders[language]()).default)
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
    document.title = shown.t.documentTitle
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
