import {
  ArchiveIcon,
  BellIcon,
  BooksIcon,
  BrowserIcon,
  CalendarDotsIcon,
  ChartBarIcon,
  CpuIcon,
  FilmSlateIcon,
  FilmStripIcon,
  GearIcon,
  HeartbeatIcon,
  HouseIcon,
  ImagesSquareIcon,
  KeyIcon,
  ListChecksIcon,
  PlayCircleIcon,
  PuzzlePieceIcon,
  QrCodeIcon,
  RecordIcon,
  ScrollIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon,
  StethoscopeIcon,
  TelevisionSimpleIcon,
  UserCircleIcon,
  UsersIcon,
  type Icon,
} from '@phosphor-icons/react'
import type { Messages } from '@/i18n'

/** A page the navigation links to. */
export type NavPage = {
  id: string
  /** Its address. */
  to: string
  label: (t: Messages) => string
  icon: Icon
}

/** A TopBar entry: one page, or a section whose pages share tabs. */
export type NavSection = NavPage & {
  /** Address prefixes that make this entry current. */
  match: readonly string[]
  /** The tabs under the section, for Content and System. */
  pages?: readonly NavPage[]
}

export const contentPages: readonly NavPage[] = [
  { id: 'sources', to: '/sources', label: (t) => t.nav.sources, icon: PuzzlePieceIcon },
  { id: 'libraries', to: '/libraries', label: (t) => t.nav.libraries, icon: FilmStripIcon },
  { id: 'live-tv', to: '/live-tv', label: (t) => t.nav.liveTv, icon: TelevisionSimpleIcon },
]

export const systemPages: readonly NavPage[] = [
  { id: 'health', to: '/system/health', label: (t) => t.nav.health, icon: HeartbeatIcon },
  {
    id: 'statistics',
    to: '/system/statistics',
    label: (t) => t.nav.statistics,
    icon: ChartBarIcon,
  },
  { id: 'schedule', to: '/system/schedule', label: (t) => t.nav.schedule, icon: CalendarDotsIcon },
  { id: 'logs', to: '/system/logs', label: (t) => t.nav.logs, icon: ScrollIcon },
  { id: 'api-keys', to: '/system/api-keys', label: (t) => t.nav.apiKeys, icon: KeyIcon },
]

/** The sections of Settings, in order, as `/settings/:section`. */
export const settingsSections = [
  { id: 'general', key: 'general', icon: SlidersHorizontalIcon },
  { id: 'playback', key: 'playback', icon: PlayCircleIcon },
  { id: 'conversion', key: 'conversion', icon: CpuIcon },
  { id: 'content', key: 'content', icon: FilmSlateIcon },
  { id: 'catalogs', key: 'catalogs', icon: BooksIcon },
  { id: 'thumbnails', key: 'thumbnails', icon: ImagesSquareIcon },
  { id: 'security', key: 'security', icon: ShieldCheckIcon },
  { id: 'tracking', key: 'tracking', icon: ListChecksIcon },
  { id: 'live-tv', key: 'liveTv', icon: TelevisionSimpleIcon },
  { id: 'recordings', key: 'recordings', icon: RecordIcon },
  { id: 'backups', key: 'backups', icon: ArchiveIcon },
  { id: 'notifications', key: 'notifications', icon: BellIcon },
  { id: 'web-player', key: 'webPlayer', icon: BrowserIcon },
  { id: 'diagnostics', key: 'diagnostics', icon: StethoscopeIcon },
] as const satisfies readonly {
  id: string
  key: keyof Messages['nav']['settingsSections']
  icon: Icon
}[]

export type SettingsSectionId = (typeof settingsSections)[number]['id']

export function isSettingsSection(value: string | undefined): value is SettingsSectionId {
  return settingsSections.some((section) => section.id === value)
}

/** The address of a settings section, or of one setting in it (`#anchor`). */
export function settingsPath(section: SettingsSectionId, anchor?: string): string {
  return `/settings/${section}${anchor ? `#${anchor}` : ''}`
}

/** What an administrator's TopBar shows, in order. */
export const adminSections: readonly NavSection[] = [
  { id: 'home', to: '/', match: [], label: (t) => t.nav.home, icon: HouseIcon },
  {
    id: 'content',
    to: '/sources',
    match: ['/sources', '/libraries', '/live-tv'],
    label: (t) => t.nav.content,
    icon: PuzzlePieceIcon,
    pages: contentPages,
  },
  { id: 'users', to: '/users', match: ['/users'], label: (t) => t.nav.users, icon: UsersIcon },
  {
    id: 'system',
    to: '/system/health',
    match: ['/system'],
    label: (t) => t.nav.system,
    icon: HeartbeatIcon,
    pages: systemPages,
  },
  {
    id: 'settings',
    to: '/settings/general',
    match: ['/settings'],
    label: (t) => t.nav.settings,
    icon: GearIcon,
  },
]

const mySources: NavSection = {
  id: 'my-sources',
  to: '/me/sources',
  match: ['/me/sources'],
  label: (t) => t.nav.mySources,
  icon: PuzzlePieceIcon,
}
const myAccount: NavSection = {
  id: 'my-account',
  to: '/me/account',
  match: ['/me/account'],
  label: (t) => t.nav.myAccount,
  icon: UserCircleIcon,
}
const quickConnect: NavSection = {
  id: 'quick-connect',
  to: '/me/quick-connect',
  match: ['/me/quick-connect'],
  label: (t) => t.nav.quickConnect,
  icon: QrCodeIcon,
}

/** Every user's own pages, as an administrator's account menu lists them. */
export const accountPages: readonly NavSection[] = [myAccount, mySources, quickConnect]

/** What a member's TopBar shows, in order. */
export const memberSections: readonly NavSection[] = [
  { id: 'home', to: '/', match: [], label: (t) => t.nav.home, icon: HouseIcon },
  mySources,
  myAccount,
  quickConnect,
]

/** Whether a TopBar entry is the current one for this address. Home matches `/` only. */
export function isCurrent(section: NavSection, pathname: string): boolean {
  if (section.match.length === 0) return pathname === section.to
  return section.match.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`))
}
