import type { ReactNode } from 'react'
import { NavLink } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, signOut, type SessionUser } from '@/api'
import { languages, useI18n } from '@/i18n'

const repositoryUrl = 'https://github.com/moodiness/polyfin'

/** Page frame: header, optional signed-in navigation, content and footer. */
export default function Shell({ user, children }: { user?: SessionUser; children: ReactNode }) {
  const { t } = useI18n()

  return (
    <div className="flex min-h-dvh flex-col">
      <div aria-hidden="true" className="bg-fin-gradient h-0.5" />
      <header className="border-b border-line">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <img
              src={`${import.meta.env.BASE_URL}polyfin.svg`}
              alt=""
              width={40}
              height={40}
              className="size-10 shrink-0 drop-shadow-[0_0_14px_rgb(34_211_238/0.35)]"
            />
            <div className="min-w-0">
              <p className="truncate text-lg leading-tight font-semibold tracking-tight text-white">
                {t.header.productName}
              </p>
              <p className="truncate text-sm text-muted">{t.header.subtitle}</p>
            </div>
          </div>
          <LanguageSwitch />
        </div>
        {user && <SignedInBar user={user} />}
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:px-6 sm:py-12">{children}</main>

      <footer className="border-t border-line">
        <div className="mx-auto w-full max-w-5xl px-4 py-6 text-sm sm:px-6">
          <a
            href={repositoryUrl}
            rel="noreferrer"
            className="rounded-sm text-fin-5 underline decoration-fin-5/40 underline-offset-4 transition-colors hover:decoration-fin-5"
          >
            {t.footer.sourceCode}
          </a>
        </div>
      </footer>
    </div>
  )
}

function SignedInBar({ user }: { user: SessionUser }) {
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

  const links = [
    { to: '/', label: t.nav.status, end: true },
    { to: '/quick-connect', label: t.nav.quickConnect, end: false },
    { to: '/account', label: t.nav.account, end: false },
    { to: '/my-addons', label: t.nav.myAddons, end: false },
    ...(user.isAdministrator
      ? [
          { to: '/users', label: t.nav.users, end: false },
          { to: '/addons', label: t.nav.addons, end: false },
          { to: '/libraries', label: t.nav.libraries, end: false },
          { to: '/settings', label: t.nav.settings, end: false },
          { to: '/api-keys', label: t.nav.apiKeys, end: false },
        ]
      : []),
  ]

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-3 px-4 pb-3 sm:px-6 lg:flex-row lg:items-center lg:justify-between">
      <nav aria-label={t.nav.label} className="min-w-0">
        <ul className="flex flex-wrap gap-1">
          {links.map((link) => (
            <li key={link.to}>
              <NavLink
                to={link.to}
                end={link.end}
                className={({ isActive }) =>
                  `inline-flex min-h-10 items-center rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-fin-2 text-white underline underline-offset-4'
                      : 'text-muted hover:bg-surface hover:text-white'
                  }`
                }
              >
                {link.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      <div className="flex items-center justify-between gap-3 lg:shrink-0 lg:justify-end">
        <p className="min-w-0 truncate text-sm text-muted">{t.nav.signedInAs(user.name)}</p>
        <button
          type="button"
          onClick={() => signOutMutation.mutate()}
          disabled={signOutMutation.isPending}
          className="shrink-0 rounded-lg border border-line bg-ink px-3 py-2 text-sm font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-70"
        >
          {signOutMutation.isPending ? t.nav.signingOut : t.nav.signOut}
        </button>
      </div>
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
