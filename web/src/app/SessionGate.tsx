import { HouseIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { Outlet, ScrollRestoration } from 'react-router'
import { fetchSession, fetchStatus, queryKeys } from '@/api'
import LoginRoute from '@/features/auth/LoginRoute'
import SetupRoute from '@/features/auth/SetupRoute'
import { ServerStatus } from '@/features/home/ServerStatus'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { ButtonLink, InlineError, Spinner } from '@/ui'
import { PageLayout } from './PageLayout'
import { useSessionUser } from './session'
import { AppShell, PublicShell } from './Shell'

function Loading() {
  const { t } = useI18n()
  return (
    <p role="status" className="flex items-center gap-2.5 text-control text-ink-3">
      <Spinner />
      {t.common.loading}
    </p>
  )
}

/**
 * Decides what the whole app shows: setup while no administrator exists, sign-in without a
 * session, and the requested page otherwise. The URL is left untouched so that signing in
 * lands on the page that was asked for.
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
      <PageLayout title={t.forbidden.title} lede={t.forbidden.description}>
        <div>
          <ButtonLink to="/" icon={HouseIcon}>
            {t.forbidden.home}
          </ButtonLink>
        </div>
      </PageLayout>
    )
  }
  return <Outlet />
}
