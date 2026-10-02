import { createContext, use, useEffect, useMemo, useState, type ReactNode } from 'react'
import en, { type Messages } from './en'
import fr from './fr'

export type Language = 'en' | 'fr'

export const languages: readonly Language[] = ['en', 'fr']

const dictionaries: Record<Language, Messages> = { en, fr }

const storageKey = 'polyfin.language'

function initialLanguage(): Language {
  try {
    const stored = window.localStorage.getItem(storageKey)
    if (stored === 'en' || stored === 'fr') return stored
  } catch {
    // Storage can be disabled by the browser; fall back to the browser language.
  }
  return navigator.language.toLowerCase().startsWith('fr') ? 'fr' : 'en'
}

type I18nContextValue = {
  language: Language
  setLanguage: (language: Language) => void
  t: Messages
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(initialLanguage)

  useEffect(() => {
    document.documentElement.lang = language
    document.title = dictionaries[language].documentTitle
  }, [language])

  const value = useMemo<I18nContextValue>(
    () => ({
      language,
      t: dictionaries[language],
      setLanguage: (next) => {
        try {
          window.localStorage.setItem(storageKey, next)
        } catch {
          // The choice still applies for this session when storage is unavailable.
        }
        setLanguageState(next)
      },
    }),
    [language],
  )

  return <I18nContext value={value}>{children}</I18nContext>
}

export function useI18n(): I18nContextValue {
  const value = use(I18nContext)
  if (!value) throw new Error('useI18n must be used inside LanguageProvider')
  return value
}
