import type { ReactNode } from 'react'
import type { SessionUser } from '@/api'
import { useI18n } from '@/i18n'
import { cx } from '@/ui'
import { Brand, LanguageSwitch } from './Brand'
import { CommandPaletteProvider } from './palette/CommandPalette'
import { SessionContext } from './session'
import { pageFrame } from './shellParts'
import { TopBar } from './TopBar'

function SkipLink() {
  const { t } = useI18n()
  return (
    <a
      href="#main"
      className="lit fixed top-[-80px] left-4 z-[70] rounded-[14px] px-[18px] py-3 font-[650] transition-[top] duration-160 focus:top-4 focus-visible:outline-none"
    >
      {t.nav.skipToContent}
    </a>
  )
}

/**
 * The signed-in frame: the top row, the page in the 1280 px column, and the command palette. It
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
          className={cx(pageFrame, 'pt-[30px] pb-[120px] outline-none max-md:pt-4 max-md:pb-20')}
        >
          {children}
        </main>
      </CommandPaletteProvider>
    </SessionContext>
  )
}

/** The frame without a session (setup, sign-in, loading): the logo and name, the language. */
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SkipLink />
      <header
        className={cx(
          pageFrame,
          'flex h-[100px] items-center justify-between gap-4 pt-[22px] max-md:h-[76px] max-md:pt-3',
        )}
      >
        <Brand link={false} />
        <LanguageSwitch />
      </header>
      <main
        id="main"
        tabIndex={-1}
        className="mx-auto w-full max-w-[880px] flex-1 animate-rise px-5 pt-5 pb-20 outline-none max-md:pt-7"
      >
        {children}
      </main>
    </div>
  )
}
