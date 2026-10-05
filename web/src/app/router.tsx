import {
  createBrowserRouter,
  Navigate,
  useLocation,
  useParams,
  type RouteObject,
} from 'react-router'
import AccountRoute from '@/features/account/AccountRoute'
import NotFoundRoute from '@/features/home/NotFoundRoute'
import HomeRoute from '@/features/home/HomeRoute'
import QuickConnectRoute from '@/features/home/QuickConnectRoute'
import IptvSourceRoute from '@/features/iptv/IptvSourceRoute'
import LibrariesRoute from '@/features/libraries/LibrariesRoute'
import CatalogGuidesRoute from '@/features/livetv/CatalogGuidesRoute'
import LiveTvRoute from '@/features/livetv/LiveTvRoute'
import SettingsRoute from '@/features/settings/SettingsRoute'
import MySourcesRoute from '@/features/sources/MySourcesRoute'
import SourcesRoute from '@/features/sources/SourcesRoute'
import ApiKeysRoute from '@/features/system/ApiKeysRoute'
import HealthRoute from '@/features/system/HealthRoute'
import LogsRoute from '@/features/system/LogsRoute'
import ScheduleRoute from '@/features/system/ScheduleRoute'
import UsersRoute from '@/features/users/UsersRoute'
import { isSettingsSection, settingsPath } from './navigation'
import { SectionTabs } from './SectionTabs'
import { AdminOnly, SessionGate } from './SessionGate'

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
 * `src/features/<area>/`; the session gate wraps them all, AdminOnly the administrators' ones.
 */
export const router = createBrowserRouter(
  [
    {
      element: <SessionGate />,
      children: [
        { index: true, element: <HomeRoute /> },
        // Reached once setup or sign-in is done: continue home.
        { path: 'setup', element: <Navigate to="/" replace /> },
        { path: 'login', element: <Navigate to="/" replace /> },

        // Every user's own pages.
        { path: 'me/account', element: <AccountRoute /> },
        { path: 'me/sources', element: <MySourcesRoute /> },
        { path: 'me/sources/:id/:section?', element: <IptvSourceRoute scope="me" /> },
        { path: 'me/quick-connect', element: <QuickConnectRoute /> },

        // Content, with its tabs. A source or catalog may be a user's own: the API checks which.
        {
          element: <SectionTabs section="content" />,
          children: [
            { path: 'sources/:scope/:id/:section?', element: <IptvSourceRoute /> },
            {
              path: 'live-tv/guides/:scope/:addonId/:catalogId/:section?',
              element: <CatalogGuidesRoute />,
            },
            {
              element: <AdminOnly />,
              children: [
                { path: 'sources', element: <SourcesRoute /> },
                { path: 'libraries', element: <LibrariesRoute /> },
                { path: 'live-tv', element: <LiveTvRoute /> },
              ],
            },
          ],
        },

        {
          element: <AdminOnly />,
          children: [
            { path: 'users/:id?', element: <UsersRoute /> },
            {
              path: 'system',
              element: <SectionTabs section="system" />,
              children: [
                { index: true, element: <Navigate to="/system/health" replace /> },
                { path: 'health', element: <HealthRoute /> },
                { path: 'schedule', element: <ScheduleRoute /> },
                { path: 'logs', element: <LogsRoute /> },
                { path: 'api-keys', element: <ApiKeysRoute /> },
              ],
            },
            { path: 'settings', element: <SettingsIndex /> },
            { path: 'settings/:section', element: <SettingsRoute /> },
          ],
        },

        ...moved,
        ...development,
        { path: '*', element: <NotFoundRoute /> },
      ],
    },
  ],
  { basename: '/admin' },
)
