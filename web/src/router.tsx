import { useQuery } from '@tanstack/react-query'
import { createBrowserRouter, Link, Navigate, Outlet } from 'react-router'
import { fetchSession, fetchStatus, queryKeys } from '@/api'
import { useSessionUser } from '@/components/session'
import Shell from '@/components/Shell'
import StatusPanel from '@/components/StatusPanel'
import { buttonSecondary, Loading, Notice, PageHeader } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import AccountPage from '@/pages/AccountPage'
import LoginPage from '@/pages/LoginPage'
import QuickConnectPage from '@/pages/QuickConnectPage'
import SettingsPage from '@/pages/SettingsPage'
import SetupPage from '@/pages/SetupPage'
import StatusPage from '@/pages/StatusPage'
import UsersPage from '@/pages/UsersPage'

/**
 * Decides what the whole app shows: setup while no administrator exists, sign-in without a
 * session, and the requested page otherwise. The URL is left untouched so that signing in
 * lands on the page that was asked for.
 */
function SessionGate() {
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
    staleTime: Infinity,
  })

  if (status.data === undefined) {
    return <Shell>{status.error === null ? <Loading /> : <StatusPanel />}</Shell>
  }
  if (setupRequired) {
    return (
      <Shell>
        <SetupPage />
      </Shell>
    )
  }
  if (session.data === undefined) {
    return (
      <Shell>
        {session.error === null ? (
          <Loading />
        ) : (
          <div className="space-y-4">
            <Notice kind="error">{errorMessage(t, session.error)}</Notice>
            <button
              type="button"
              className={buttonSecondary}
              onClick={() => void session.refetch()}
            >
              {t.common.retry}
            </button>
          </div>
        )}
      </Shell>
    )
  }
  if (session.data === null) {
    return (
      <Shell>
        <LoginPage />
      </Shell>
    )
  }
  return (
    <Shell user={session.data}>
      <Outlet context={session.data} />
    </Shell>
  )
}

function AdminOnly() {
  const { t } = useI18n()
  const user = useSessionUser()
  if (!user.isAdministrator) {
    return (
      <>
        <PageHeader title={t.forbidden.title} description={t.forbidden.description} />
        <HomeLink label={t.forbidden.home} />
      </>
    )
  }
  return <Outlet context={user} />
}

function NotFoundPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.notFound.title} description={t.notFound.description} />
      <HomeLink label={t.notFound.home} />
    </>
  )
}

function HomeLink({ label }: { label: string }) {
  return (
    <Link to="/" className={buttonSecondary}>
      {label}
    </Link>
  )
}

export const router = createBrowserRouter(
  [
    {
      element: <SessionGate />,
      children: [
        { index: true, element: <StatusPage /> },
        // Reached once setup or sign-in is done: continue to the status page.
        { path: 'setup', element: <Navigate to="/" replace /> },
        { path: 'login', element: <Navigate to="/" replace /> },
        { path: 'quick-connect', element: <QuickConnectPage /> },
        { path: 'account', element: <AccountPage /> },
        {
          element: <AdminOnly />,
          children: [
            { path: 'users', element: <UsersPage /> },
            { path: 'settings', element: <SettingsPage /> },
          ],
        },
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ],
  { basename: '/admin' },
)
