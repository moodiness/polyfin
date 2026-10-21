import { createContext, use, useEffect, useMemo, useState, type ReactNode } from 'react'
import enAccount from './en/account'
import enNotifications from './en/notifications'
import enBackups from './en/backups'
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
import frBackups from './fr/backups'
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
import deAccount from './de/account'
import deNotifications from './de/notifications'
import deBackups from './de/backups'
import deAuth from './de/auth'
import deCommon from './de/common'
import deHome from './de/home'
import deIptv from './de/iptv'
import deLibraries from './de/libraries'
import deLivetv from './de/livetv'
import deNav from './de/nav'
import deSettings from './de/settings'
import deSources from './de/sources'
import deStatistics from './de/statistics'
import deSystem from './de/system'
import deUsers from './de/users'
import esAccount from './es/account'
import esNotifications from './es/notifications'
import esBackups from './es/backups'
import esAuth from './es/auth'
import esCommon from './es/common'
import esHome from './es/home'
import esIptv from './es/iptv'
import esLibraries from './es/libraries'
import esLivetv from './es/livetv'
import esNav from './es/nav'
import esSettings from './es/settings'
import esSources from './es/sources'
import esStatistics from './es/statistics'
import esSystem from './es/system'
import esUsers from './es/users'
import itAccount from './it/account'
import itNotifications from './it/notifications'
import itBackups from './it/backups'
import itAuth from './it/auth'
import itCommon from './it/common'
import itHome from './it/home'
import itIptv from './it/iptv'
import itLibraries from './it/libraries'
import itLivetv from './it/livetv'
import itNav from './it/nav'
import itSettings from './it/settings'
import itSources from './it/sources'
import itStatistics from './it/statistics'
import itSystem from './it/system'
import itUsers from './it/users'
import ptBRAccount from './pt-BR/account'
import ptBRNotifications from './pt-BR/notifications'
import ptBRBackups from './pt-BR/backups'
import ptBRAuth from './pt-BR/auth'
import ptBRCommon from './pt-BR/common'
import ptBRHome from './pt-BR/home'
import ptBRIptv from './pt-BR/iptv'
import ptBRLibraries from './pt-BR/libraries'
import ptBRLivetv from './pt-BR/livetv'
import ptBRNav from './pt-BR/nav'
import ptBRSettings from './pt-BR/settings'
import ptBRSources from './pt-BR/sources'
import ptBRStatistics from './pt-BR/statistics'
import ptBRSystem from './pt-BR/system'
import ptBRUsers from './pt-BR/users'

/*
 * The messages are split into one file per area and language, `en/<area>.ts`, `fr/<area>.ts` and so
 * on, each holding whole top-level keys (`t.users`, `t.lineup`…). English is the reference: each
 * other language's file is typed with its English twin, so a missing or extra key fails type
 * checking.
 */

export type Language = 'en' | 'fr' | 'de' | 'es' | 'it' | 'pt-BR'

export const languages: readonly Language[] = ['en', 'fr', 'de', 'es', 'it', 'pt-BR']

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
  ...enBackups,
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
  ...frBackups,
}

const de: Messages = {
  ...deCommon,
  ...deNav,
  ...deHome,
  ...deAuth,
  ...deSources,
  ...deIptv,
  ...deLibraries,
  ...deLivetv,
  ...deUsers,
  ...deSystem,
  ...deSettings,
  ...deAccount,
  ...deNotifications,
  ...deStatistics,
  ...deBackups,
}

const es: Messages = {
  ...esCommon,
  ...esNav,
  ...esHome,
  ...esAuth,
  ...esSources,
  ...esIptv,
  ...esLibraries,
  ...esLivetv,
  ...esUsers,
  ...esSystem,
  ...esSettings,
  ...esAccount,
  ...esNotifications,
  ...esStatistics,
  ...esBackups,
}

const it: Messages = {
  ...itCommon,
  ...itNav,
  ...itHome,
  ...itAuth,
  ...itSources,
  ...itIptv,
  ...itLibraries,
  ...itLivetv,
  ...itUsers,
  ...itSystem,
  ...itSettings,
  ...itAccount,
  ...itNotifications,
  ...itStatistics,
  ...itBackups,
}

const ptBR: Messages = {
  ...ptBRCommon,
  ...ptBRNav,
  ...ptBRHome,
  ...ptBRAuth,
  ...ptBRSources,
  ...ptBRIptv,
  ...ptBRLibraries,
  ...ptBRLivetv,
  ...ptBRUsers,
  ...ptBRSystem,
  ...ptBRSettings,
  ...ptBRAccount,
  ...ptBRNotifications,
  ...ptBRStatistics,
  ...ptBRBackups,
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
    enBackups,
  ]) {
    for (const key of Object.keys(area)) {
      if (seen.has(key)) throw new Error(`i18n: the key "${key}" is defined by two areas`)
      seen.add(key)
    }
  }
}

const dictionaries: Record<Language, Messages> = { en, fr, de, es, it, 'pt-BR': ptBR }

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
