import { ArrowLeftIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { Outlet, ScrollRestoration } from 'react-router'
import { fetchSession, fetchStatus, queryKeys } from '@/api'
import LoginRoute from '@/features/auth/LoginRoute'
import SetupRoute from '@/features/auth/SetupRoute'
import { ServerStatus } from '@/features/home/ServerStatus'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { webPlayerInPlace, webPlayerLink } from '@/jellyfinApps'
import { InlineError, Spinner } from '@/ui'
import { DeadEnd } from './DeadEnd'
import { useSessionUser } from './session'
import { AppShell, PublicShell } from './Shell'

function Loading() {
  const { t } = useI18n()
  return (
    <p
      role="status"
      className="flex items-center justify-center gap-3 py-16 text-control text-ink-2"
    >
      <Spinner />
      {t.common.loading}
    </p>
  )
}

/**
 * Decides what the whole app shows: setup while no administrator exists, sign-in without a
 * session, and the requested page otherwise. The URL is left untouched so that signing in
 * lands on the page that was asked for. Opened from the web player or inside a Jellyfin app,
 * sign-in also offers the way back to the web player, the only one after Sign out or when the
 * web player's sign-in is refused here, as for an account with two-step sign-in.
 */
export function SessionGate() {
  const { t } = useI18n()
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })
  const setupRequired = status.data?.setupRequired
  const session = useQuery({
    queryKey: queryKeys.session,
    queryFn: ({ signal }) => fetchSession(signal),
    enabled: setupRequired === false,
    // Read again when the tab comes back after 30 s: a profile picture or a name changed in an
    // app shows on the account button.
    staleTime: 30_000,
  })

  if (status.data === undefined) {
    return <PublicShell>{status.error === null ? <Loading /> : <ServerStatus />}</PublicShell>
  }
  if (setupRequired) {
    return (
      <PublicShell>
        <SetupRoute />
      </PublicShell>
    )
  }
  if (session.data === undefined) {
    return (
      <PublicShell>
        {session.error === null ? (
          <Loading />
        ) : (
          <InlineError onRetry={() => void session.refetch()} retrying={session.isFetching}>
            {errorMessage(t, session.error)}
          </InlineError>
        )}
      </PublicShell>
    )
  }
  if (session.data === null) {
    return (
      <PublicShell>
        <LoginRoute />
        {status.data.webClient === true && webPlayerInPlace() && (
          <p className="mt-8 text-center">
            <a
              {...webPlayerLink(true)}
              className="focus-lit inline-flex h-13 items-center gap-2.5 rounded-control px-5.5 text-control font-[620] text-ink-2 [--lift:1.05] hover:bg-s1 hover:text-ink"
            >
              <ArrowLeftIcon size={21} aria-hidden="true" />
              {t.nav.backToWebPlayer}
            </a>
          </p>
        )}
      </PublicShell>
    )
  }
  return (
    <AppShell user={session.data}>
      <ScrollRestoration />
      <Outlet />
    </AppShell>
  )
}

/** Lets administrators through to the nested routes; tells members they may not open the page. */
export function AdminOnly() {
  const { t } = useI18n()
  const user = useSessionUser()
  if (!user.isAdministrator) {
    return (
      <DeadEnd
        title={t.forbidden.title}
        description={t.forbidden.description}
        home={t.forbidden.home}
      />
    )
  }
  return <Outlet />
}
