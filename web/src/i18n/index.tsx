import { createContext, use, useEffect, useMemo, useState, type ReactNode } from 'react'
import enAccount from './en/account'
import enNotifications from './en/notifications'
import enAuth from './en/auth'
import enCommon from './en/common'
import enHome from './en/home'
import enIptv from './en/iptv'
import enLibraries from './en/libraries'
import enLivetv from './en/livetv'
import enNav from './en/nav'
import enSettings from './en/settings'
import enSources from './en/sources'
import enStatistics from './en/statistics'
import enSystem from './en/system'
import enUsers from './en/users'
import frAccount from './fr/account'
import frNotifications from './fr/notifications'
import frAuth from './fr/auth'
import frCommon from './fr/common'
import frHome from './fr/home'
import frIptv from './fr/iptv'
import frLibraries from './fr/libraries'
import frLivetv from './fr/livetv'
import frNav from './fr/nav'
import frSettings from './fr/settings'
import frSources from './fr/sources'
import frStatistics from './fr/statistics'
import frSystem from './fr/system'
import frUsers from './fr/users'

/*
 * The messages are split into one file per area, `en/<area>.ts` and `fr/<area>.ts`, each holding
 * whole top-level keys (`t.users`, `t.lineup`…). English is the reference: each French file is typed
 * with its English twin, so a missing or extra French key fails type checking.
 */

export type Language = 'en' | 'fr'

export const languages: readonly Language[] = ['en', 'fr']

const en = {
  ...enCommon,
  ...enNav,
  ...enHome,
  ...enAuth,
  ...enSources,
  ...enIptv,
  ...enLibraries,
  ...enLivetv,
  ...enUsers,
  ...enSystem,
  ...enSettings,
  ...enAccount,
  ...enNotifications,
  ...enStatistics,
}

/** Every message of the interface, in one language. */
export type Messages = typeof en

const fr: Messages = {
  ...frCommon,
  ...frNav,
  ...frHome,
  ...frAuth,
  ...frSources,
  ...frIptv,
  ...frLibraries,
  ...frLivetv,
  ...frUsers,
  ...frSystem,
  ...frSettings,
  ...frAccount,
  ...frNotifications,
  ...frStatistics,
}

// Two areas defining the same top-level key would silently hide one of them.
if (import.meta.env.DEV) {
  const seen = new Set<string>()
  for (const area of [
    enCommon,
    enNav,
    enHome,
    enAuth,
    enSources,
    enIptv,
    enLibraries,
    enLivetv,
    enUsers,
    enSystem,
    enSettings,
    enAccount,
    enNotifications,
    enStatistics,
  ]) {
    for (const key of Object.keys(area)) {
      if (seen.has(key)) throw new Error(`i18n: the key "${key}" is defined by two areas`)
      seen.add(key)
    }
  }
}

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
