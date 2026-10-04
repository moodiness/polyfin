import { useEffect, useRef, type ComponentType, type ReactNode, type SVGProps } from 'react'
import { NavLink, useLocation } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { fetchStatus, queryClient, queryKeys, signOut, type SessionUser } from '@/api'
import { icons } from '@/components/icons'
import { languages, useI18n } from '@/i18n'

const repositoryUrl = 'https://github.com/moodiness/polyfin'

/**
 * Page frame. Signed in, a sidebar holds the navigation, which phones open from a menu button;
 * signed out (setup, sign-in), the page is a single centered column.
 */
export default function Shell({ user, children }: { user?: SessionUser; children: ReactNode }) {
  const { t } = useI18n()
  const skipLink = (
    <a
      href="#main"
      className="sr-only rounded-lg bg-fin-2 px-4 py-2 text-sm font-medium text-white focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50"
    >
      {t.nav.skipToContent}
    </a>
  )

  if (!user) {
    return (
      <div className="flex min-h-dvh flex-col">
        {skipLink}
        <div aria-hidden="true" className="bg-fin-gradient h-0.5" />
        <header className="border-b border-line">
          <div className="mx-auto flex w-full max-w-5xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
            <Brand />
            <LanguageSwitch />
          </div>
        </header>
        <main id="main" className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:px-6 sm:py-12">
          {children}
        </main>
        <footer className="border-t border-line">
          <div className="mx-auto w-full max-w-5xl px-4 py-6 text-sm sm:px-6">
            <SourceLink />
          </div>
        </footer>
      </div>
    )
  }

  return (
    <div className="min-h-dvh">
      {skipLink}
      <aside className="fixed inset-y-0 left-0 z-20 hidden w-64 flex-col border-r border-line bg-ink lg:flex">
        <div aria-hidden="true" className="bg-fin-gradient h-0.5 shrink-0" />
        <div className="px-5 pt-5 pb-4">
          <Brand />
        </div>
        <Sidebar user={user} />
      </aside>
      <MobileBar user={user} />
      <div className="lg:pl-64">
        <main
          id="main"
          className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 sm:py-8 lg:px-10 lg:py-10"
        >
          {children}
        </main>
      </div>
    </div>
  )
}

function Brand() {
  const { t } = useI18n()
  return (
    <div className="flex min-w-0 items-center gap-3">
      <img
        src={`${import.meta.env.BASE_URL}polyfin.svg`}
        alt=""
        width={36}
        height={36}
        className="size-9 shrink-0 drop-shadow-[0_0_14px_rgb(34_211_238/0.35)]"
      />
      <div className="min-w-0">
        <p className="truncate text-base leading-tight font-semibold tracking-tight text-white">
          {t.header.productName}
        </p>
        <p className="truncate text-xs text-muted">{t.header.subtitle}</p>
      </div>
    </div>
  )
}

function SourceLink() {
  const { t } = useI18n()
  return (
    <a
      href={repositoryUrl}
      rel="noreferrer"
      className="rounded-sm text-fin-5 underline decoration-fin-5/40 underline-offset-4 transition-colors hover:decoration-fin-5"
    >
      {t.footer.sourceCode}
    </a>
  )
}

/** On phones and tablets: a bar with the menu button, which opens the sidebar in a drawer. */
function MobileBar({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const drawer = useRef<HTMLDialogElement>(null)
  const location = useLocation()

  // Following a link closes the drawer.
  useEffect(() => {
    drawer.current?.close()
  }, [location.pathname])

  return (
    <div className="sticky top-0 z-20 border-b border-line bg-ink/95 backdrop-blur lg:hidden">
      <div aria-hidden="true" className="bg-fin-gradient h-0.5" />
      <div className="flex items-center justify-between gap-3 px-4 py-2.5">
        <Brand />
        <button
          type="button"
          aria-haspopup="dialog"
          onClick={() => drawer.current?.showModal()}
          className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-white transition-colors hover:border-fin-4"
        >
          <icons.menu className="size-5" />
          {t.nav.menu}
        </button>
      </div>
      <dialog
        ref={drawer}
        aria-label={t.nav.label}
        // A click on the backdrop lands on the dialog itself: it closes the drawer.
        onClick={(event) => event.target === event.currentTarget && drawer.current?.close()}
        className="m-0 h-dvh max-h-dvh w-[min(20rem,calc(100vw-3rem))] max-w-none border-r border-line bg-ink p-0 text-zinc-100 backdrop:bg-black/60 open:flex open:flex-col"
      >
        <div className="flex items-center justify-between gap-3 px-5 pt-4 pb-3">
          <Brand />
          <button
            type="button"
            onClick={() => drawer.current?.close()}
            aria-label={t.nav.closeMenu}
            className="inline-flex size-10 items-center justify-center rounded-lg text-muted transition-colors hover:bg-surface hover:text-white"
          >
            <icons.close className="size-5" />
          </button>
        </div>
        <Sidebar user={user} />
      </dialog>
    </div>
  )
}

type NavItem = {
  to: string
  label: string
  icon: ComponentType<SVGProps<SVGSVGElement>>
  end?: boolean
}

function Sidebar({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })

  const groups: { label: string; items: NavItem[] }[] = user.isAdministrator
    ? [
        {
          label: t.nav.groupMonitor,
          items: [
            { to: '/', label: t.nav.overview, icon: icons.overview, end: true },
            { to: '/schedule', label: t.nav.schedule, icon: icons.schedule },
            { to: '/health', label: t.nav.health, icon: icons.health },
            { to: '/logs', label: t.nav.logs, icon: icons.logs },
          ],
        },
        {
          label: t.nav.groupServer,
          items: [
            { to: '/users', label: t.nav.users, icon: icons.users },
            { to: '/libraries', label: t.nav.libraries, icon: icons.libraries },
            { to: '/addons', label: t.nav.addons, icon: icons.addons },
            { to: '/settings', label: t.nav.settings, icon: icons.settings },
            { to: '/api-keys', label: t.nav.apiKeys, icon: icons.key },
          ],
        },
        {
          label: t.nav.groupYou,
          items: [
            { to: '/my-addons', label: t.nav.myAddons, icon: icons.myAddons },
            { to: '/account', label: t.nav.account, icon: icons.account },
            { to: '/quick-connect', label: t.nav.quickConnect, icon: icons.quickConnect },
          ],
        },
      ]
    : [
        {
          label: t.nav.groupYou,
          items: [
            { to: '/', label: t.nav.status, icon: icons.overview, end: true },
            { to: '/my-addons', label: t.nav.myAddons, icon: icons.myAddons },
            { to: '/account', label: t.nav.account, icon: icons.account },
            { to: '/quick-connect', label: t.nav.quickConnect, icon: icons.quickConnect },
          ],
        },
      ]

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <nav aria-label={t.nav.label} className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
        {groups.map((group) => (
          <div key={group.label} className="mt-3 first:mt-0">
            <p className="px-3 pt-2 pb-1.5 text-[0.6875rem] font-semibold tracking-wider text-zinc-500 uppercase">
              {group.label}
            </p>
            <ul className="space-y-0.5">
              {group.items.map((item) => (
                <li key={item.to}>
                  <NavLink
                    to={item.to}
                    end={item.end}
                    className={({ isActive }) =>
                      `group relative flex min-h-10 items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                        isActive
                          ? 'bg-surface-2 text-white'
                          : 'text-muted hover:bg-surface hover:text-white'
                      }`
                    }
                  >
                    {({ isActive }) => (
                      <>
                        {isActive && (
                          <span
                            aria-hidden="true"
                            className="bg-fin-gradient absolute inset-y-2 left-0 w-0.5 rounded-full"
                          />
                        )}
                        <item.icon
                          className={`size-[1.125rem] shrink-0 ${isActive ? 'text-fin-5' : ''}`}
                        />
                        {item.label}
                      </>
                    )}
                  </NavLink>
                </li>
              ))}
            </ul>
          </div>
        ))}
        {status.data?.webClient === true && (
          <a
            href="/web/"
            target="_blank"
            rel="noopener"
            title={t.nav.webPlayerHint}
            className="mt-4 flex min-h-10 items-center gap-3 rounded-lg border border-line bg-surface px-3 py-2 text-sm font-medium text-white transition-colors hover:border-fin-4"
          >
            <icons.play className="size-[1.125rem] shrink-0 text-fin-5" />
            <span className="flex-1">{t.nav.webPlayer}</span>
            <icons.external className="size-4 shrink-0 text-muted" />
            <span className="sr-only">{t.nav.webPlayerHint}</span>
          </a>
        )}
      </nav>
      <SignedInFooter user={user} />
    </div>
  )
}

function SignedInFooter({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const signOutMutation = useMutation({
    mutationFn: signOut,
    onSettled: () => {
      // Update the mounted session query in place (removing it would detach its observer and
      // keep the signed-in screen), then forget the rest; the status query stays public.
      queryClient.setQueryData(queryKeys.session, null)
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== 'status' && query.queryKey[0] !== 'session',
      })
    },
  })

  return (
    <div className="space-y-3 border-t border-line px-4 py-4">
      <div className="flex items-center gap-3">
        <span
          aria-hidden="true"
          className="flex size-9 shrink-0 items-center justify-center rounded-[0.65rem] bg-surface-2 text-sm font-semibold text-fin-5"
        >
          {user.name.slice(0, 1).toUpperCase()}
        </span>
        <p className="min-w-0 flex-1 truncate text-sm text-zinc-200">
          {t.nav.signedInAs(user.name)}
        </p>
      </div>
      <div className="flex items-center justify-between gap-2">
        <LanguageSwitch />
        <button
          type="button"
          onClick={() => signOutMutation.mutate()}
          disabled={signOutMutation.isPending}
          className="min-h-9 rounded-lg border border-line bg-ink px-3 py-1.5 text-sm font-medium whitespace-nowrap text-white transition-colors hover:border-fin-4 active:translate-y-px disabled:cursor-progress disabled:opacity-70"
        >
          {signOutMutation.isPending ? t.nav.signingOut : t.nav.signOut}
        </button>
      </div>
      <p className="text-xs">
        <SourceLink />
      </p>
    </div>
  )
}

function LanguageSwitch() {
  const { language, setLanguage, t } = useI18n()

  return (
    <div
      role="group"
      aria-label={t.language.label}
      className="inline-flex shrink-0 rounded-lg border border-line bg-surface p-0.5"
    >
      {languages.map((code) => (
        <button
          key={code}
          type="button"
          lang={code}
          title={t.language[code].name}
          aria-pressed={language === code}
          onClick={() => setLanguage(code)}
          className={`min-w-11 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            language === code ? 'bg-fin-2 text-white' : 'text-muted hover:text-white'
          }`}
        >
          {t.language[code].short}
        </button>
      ))}
    </div>
  )
}
