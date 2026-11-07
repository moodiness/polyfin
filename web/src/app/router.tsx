import type { ComponentType } from 'react'
import {
  createBrowserRouter,
  Navigate,
  useLocation,
  useParams,
  type RouteObject,
} from 'react-router'
import InviteRoute from '@/features/auth/InviteRoute'
import { isSettingsSection, settingsPath } from './navigation'
import { SectionTabs } from './SectionTabs'
import { AdminOnly, SessionGate } from './SessionGate'
import { PublicShell } from './Shell'

/**
 * A page in a file of its own, with the props its route gives it. The router starts the file as
 * the navigation starts, and shows the page once it is in: the page shown until then stays.
 */
function page<P extends object>(
  load: () => Promise<{ default: ComponentType<P> }>,
  ...[props]: object extends P ? [P?] : [P]
) {
  return async () => {
    const { default: Page } = await load()
    return { element: <Page {...(props as P)} /> }
  }
}

/** Sends an address of the app before « Nuit » to its new place, keeping its query and anchor. */
function Moved({ to }: { to: string }) {
  const { search, hash } = useLocation()
  const { '*': rest } = useParams()
  const pathname = rest ? `${to}/${rest}` : to
  return <Navigate to={{ pathname, search, hash }} replace />
}

/**
 * `/settings` opens General. A link from before « Nuit » to one of its anchors (`#settings-tracking`)
 * opens that section instead.
 */
function SettingsIndex() {
  const { hash } = useLocation()
  const old = hash.replace(/^#settings-/, '')
  const section =
    old === 'liveTv'
      ? 'live-tv'
      : old === 'webPlayer'
        ? 'web-player'
        : old === 'variables'
          ? 'diagnostics'
          : old
  return (
    <Navigate
      to={isSettingsSection(section) ? settingsPath(section) : settingsPath('general')}
      replace
    />
  )
}

/** The old addresses, still used by bookmarks and by links outside the app. */
const moved: RouteObject[] = [
  { path: 'addons', element: <Moved to="/sources" /> },
  { path: 'my-addons', element: <Moved to="/me/sources" /> },
  { path: 'account', element: <Moved to="/me/account" /> },
  { path: 'quick-connect', element: <Moved to="/me/quick-connect" /> },
  { path: 'health', element: <Moved to="/system/health" /> },
  { path: 'schedule', element: <Moved to="/system/schedule" /> },
  { path: 'logs', element: <Moved to="/system/logs" /> },
  { path: 'api-keys', element: <Moved to="/system/api-keys" /> },
  { path: 'guides/*', element: <Moved to="/live-tv/guides" /> },
]

/**
 * The development gallery of the design system. Imported lazily under `import.meta.env.DEV` so
 * that production builds drop it entirely: a static import would ship it.
 */
const development: RouteObject[] = import.meta.env.DEV
  ? [
      {
        path: 'dev/ui',
        lazy: async () => ({ Component: (await import('./dev/UiGallery')).default }),
      },
    ]
  : []

/**
 * Every page of the admin app, under `/admin`. Each route renders a component of
 * `src/features/<area>/`; the session gate wraps them all but the page an invite link opens, which
 * needs no session, and AdminOnly the administrators' ones.
 */
export const router = createBrowserRouter(
  [
    {
      path: 'invite/:token',
      element: (
        <PublicShell>
          <InviteRoute />
        </PublicShell>
      ),
    },
    {
      element: <SessionGate />,
      // On the first load, the session check and the shell go on while the page's file downloads.
      hydrateFallbackElement: <SessionGate />,
      children: [
        { index: true, lazy: page(() => import('@/features/home/HomeRoute')) },
        // Reached once setup or sign-in is done: continue home.
        { path: 'setup', element: <Navigate to="/" replace /> },
        { path: 'login', element: <Navigate to="/" replace /> },

        // Every user's own pages.
        { path: 'me/account', lazy: page(() => import('@/features/account/AccountRoute')) },
        // Where MyAnimeList sends the browser back once the user allowed Polyfin.
        {
          path: 'me/account/myanimelist',
          lazy: page(() => import('@/features/account/MyAnimeListReturn')),
        },
        { path: 'me/sources', lazy: page(() => import('@/features/sources/MySourcesRoute')) },
        {
          path: 'me/sources/smart/:id',
          lazy: page(() => import('@/features/libraries/SmartListRoute'), { scope: 'me' }),
        },
        {
          path: 'me/sources/:id/:section?',
          lazy: page(() => import('@/features/sources/SourceRoute'), { scope: 'me' }),
        },
        {
          path: 'me/quick-connect',
          lazy: page(() => import('@/features/home/QuickConnectRoute')),
        },

        // Content, with its tabs. A source or catalog may be a user's own: the API checks which.
        {
          element: <SectionTabs section="content" />,
          children: [
            {
              path: 'sources/:scope/:id/:section?',
              lazy: page(() => import('@/features/sources/SourceRoute')),
            },
            {
              path: 'live-tv/guides/:scope/:addonId/:catalogId/:section?',
              lazy: page(() => import('@/features/livetv/CatalogGuidesRoute')),
            },
            {
              element: <AdminOnly />,
              children: [
                { path: 'sources', lazy: page(() => import('@/features/sources/SourcesRoute')) },
                {
                  path: 'libraries',
                  lazy: page(() => import('@/features/libraries/LibrariesRoute')),
                },
                {
                  path: 'libraries/smart/:id',
                  lazy: page(() => import('@/features/libraries/SmartListRoute'), {
                    scope: 'shared',
                  }),
                },
                { path: 'live-tv', lazy: page(() => import('@/features/livetv/LiveTvRoute')) },
                {
                  path: 'live-tv/channels/:id',
                  lazy: page(() => import('@/features/livetv/OwnChannels')),
                },
                { path: 'versions', lazy: page(() => import('@/features/versions/VersionsRoute')) },
                {
                  path: 'versions/:id',
                  lazy: page(() => import('@/features/versions/TitleVersionsRoute')),
                },
              ],
            },
          ],
        },

        {
          element: <AdminOnly />,
          children: [
            {
              path: 'users/jellyfin-import',
              lazy: page(() => import('@/features/users/JellyfinImport')),
            },
            {
              path: 'users/app-import',
              lazy: page(() => import('@/features/users/AppImportRoute')),
            },
            {
              path: 'users/letterboxd-import',
              lazy: page(() => import('@/features/users/LetterboxdImportRoute')),
            },
            { path: 'users/groups/:id?', lazy: page(() => import('@/features/users/GroupsRoute')) },
            { path: 'users/:id?', lazy: page(() => import('@/features/users/UsersRoute')) },
            {
              path: 'system',
              element: <SectionTabs section="system" />,
              children: [
                { index: true, element: <Navigate to="/system/health" replace /> },
                { path: 'health', lazy: page(() => import('@/features/system/HealthRoute')) },
                {
                  path: 'statistics',
                  lazy: page(() => import('@/features/statistics/StatisticsRoute')),
                },
                { path: 'schedule', lazy: page(() => import('@/features/system/ScheduleRoute')) },
                { path: 'logs', lazy: page(() => import('@/features/system/LogsRoute')) },
                { path: 'api-keys', lazy: page(() => import('@/features/system/ApiKeysRoute')) },
              ],
            },
            { path: 'settings', element: <SettingsIndex /> },
            {
              path: 'settings/:section',
              lazy: page(() => import('@/features/settings/SettingsRoute')),
            },
          ],
        },

        ...moved,
        ...development,
        { path: '*', lazy: page(() => import('@/features/home/NotFoundRoute')) },
      ],
    },
  ],
  { basename: '/admin' },
)
