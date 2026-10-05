import type { ReactNode } from 'react'
import type { SessionUser } from '@/api'
import { useI18n } from '@/i18n'
import { Brand, LanguageSwitch } from './Brand'
import { CommandPaletteProvider } from './palette/CommandPalette'
import { SessionContext } from './session'
import { TopBar } from './TopBar'

function SkipLink() {
  const { t } = useI18n()
  return (
    <a
      href="#main"
      className="fixed top-[-48px] left-4 z-[70] rounded-field bg-accent px-3 py-2 text-control font-medium text-white transition-[top] duration-160 focus:top-3"
    >
      {t.nav.skipToContent}
    </a>
  )
}

/**
 * The signed-in frame: the TopBar, the page in a 1232 px column, and the command palette. It
 * provides the session to `useSessionUser`.
 */
export function AppShell({ user, children }: { user: SessionUser; children: ReactNode }) {
  return (
    <SessionContext value={user}>
      <CommandPaletteProvider>
        <SkipLink />
        <TopBar user={user} />
        <main
          id="main"
          tabIndex={-1}
          className="mx-auto w-full max-w-page px-14 pt-11 pb-24 outline-none max-xl:px-8 max-md:px-5 max-md:pt-7 max-md:pb-16"
        >
          {children}
        </main>
      </CommandPaletteProvider>
    </SessionContext>
  )
}

/** The frame without a session (setup, sign-in, loading): the logo, the language, one column. */
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SkipLink />
      <header className="border-b border-line">
        <div className="mx-auto flex h-topbar w-full max-w-page items-center justify-between gap-4 px-14 max-xl:px-8 max-md:px-5">
          <Brand link={false} />
          <LanguageSwitch />
        </div>
      </header>
      <main
        id="main"
        tabIndex={-1}
        className="mx-auto w-full max-w-[880px] flex-1 animate-rise px-5 py-12 outline-none max-md:py-8"
      >
        {children}
      </main>
    </div>
  )
}
