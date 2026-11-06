import {
  ArrowLeftIcon,
  GithubLogoIcon,
  MagnifyingGlassIcon,
  MonitorPlayIcon,
  SignOutIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router'
import { fetchStatus, queryKeys, userImageUrl, type SessionUser } from '@/api'
import { languages, useI18n } from '@/i18n'
import { webPlayerInPlace, webPlayerLink } from '@/jellyfinApps'
import {
  Avatar,
  cx,
  Kbd,
  Menu,
  MenuChoice,
  MenuHeader,
  MenuItem,
  MenuSeparator,
  modKey,
} from '@/ui'
import { Brand, repositoryUrl } from './Brand'
import { pageFrame, ProblemCount, topControl } from './shellParts'
import { MobileMenu } from './MobileMenu'
import {
  accountPages,
  adminSections,
  isCurrent,
  memberSections,
  type NavSection,
} from './navigation'
import { useOpenPalette } from './palette/CommandPalette'
import { useSignOut } from './session'

/** Whether the server serves the web player at /web/, from the public status. */
export function useWebClient(): boolean {
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })
  return status.data?.webClient === true
}

/**
 * The top row of every signed-in page, which scrolls away with it: the logo, the five area words
 * (a member's own pages instead), then Search with ⌘K, the web player and the account. The area
 * words are 24 px, 22 under 1400 px and 20 under 1100 px; below 1180 px, where German ones
 * no longer fit, they move into the full-screen menu, behind the menu button. The account's name
 * gives way first when the row is short of room. `data-top-bar` marks it for the lists of
 * `Select` and `SuggestInput`, which stay under it.
 *
 * Opened from the web player or inside a Jellyfin app, the web player opens in place, and a
 * phone shows a back arrow to it before the logo, where jellyfin-web's dashboard has its own.
 */
export function TopBar({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const { pathname } = useLocation()
  const openPalette = useOpenPalette()
  const webClient = useWebClient()
  const inPlace = webPlayerInPlace()
  const sections = user.isAdministrator ? adminSections : memberSections

  return (
    <header data-top-bar className="relative z-30">
      <div
        className={cx(
          pageFrame,
          'flex h-[100px] items-center gap-7 pt-[26px] max-[1400px]:gap-5 max-[1180px]:h-[72px] max-[1180px]:gap-2.5 max-[1180px]:pt-3.5',
        )}
      >
        {webClient && inPlace && (
          <a
            {...webPlayerLink(true)}
            aria-label={t.nav.backToWebPlayer}
            title={t.nav.backToWebPlayer}
            className={cx(topControl, 'size-12 justify-center bg-transparent min-[1180px]:hidden')}
          >
            <ArrowLeftIcon size={22} aria-hidden="true" />
          </a>
        )}
        <Brand />
        <nav aria-label={t.nav.label} className="shrink-0 max-[1180px]:hidden">
          <ul className="flex gap-1.5 max-[1400px]:gap-0.5">
            {sections.map((section) => (
              <li key={section.id}>
                <AreaWord section={section} current={isCurrent(section, pathname)} />
              </li>
            ))}
          </ul>
        </nav>
        <div className="ml-auto flex min-w-0 items-center gap-3.5 max-[1180px]:gap-2">
          <button
            type="button"
            onClick={openPalette}
            aria-label={t.nav.searchShortcut(`${modKey} K`)}
            aria-keyshortcuts="Meta+K Control+K /"
            className={cx(
              topControl,
              'h-[52px] w-[260px] gap-3 px-5 text-control text-ink-3 focus-visible:[--lift:1.03] max-[1400px]:w-[210px]',
              'max-xl:w-[52px] max-xl:justify-center max-xl:px-0 max-[1180px]:size-12',
            )}
          >
            <MagnifyingGlassIcon size={22} aria-hidden="true" className="shrink-0 text-ink-2" />
            <span className="max-xl:hidden">{t.nav.search}</span>
            <Kbd className="ml-auto max-xl:hidden">{modKey === '⌘' ? '⌘K' : 'Ctrl K'}</Kbd>
          </button>
          {webClient && (
            <a
              {...webPlayerLink(inPlace)}
              aria-label={
                inPlace ? t.nav.backToWebPlayer : `${t.nav.webPlayer}: ${t.nav.webPlayerHint}`
              }
              title={inPlace ? t.nav.backToWebPlayer : t.nav.webPlayer}
              className={cx(topControl, 'size-[52px] justify-center max-[1180px]:hidden')}
            >
              {inPlace ? (
                <ArrowLeftIcon size={24} aria-hidden="true" />
              ) : (
                <MonitorPlayIcon size={24} aria-hidden="true" />
              )}
            </a>
          )}
          <AccountMenu user={user} />
          <div className="min-[1180px]:hidden">
            <MobileMenu user={user} webClient={webClient} inPlace={inPlace} />
          </div>
        </div>
      </div>
    </header>
  )
}

/** One of the top row's area words, with the red count of Health's problems beside System. */
function AreaWord({ section, current }: { section: NavSection; current: boolean }) {
  const { t } = useI18n()
  return (
    <Link
      to={section.to}
      aria-current={current ? 'page' : undefined}
      className={cx(
        'focus-lit group relative flex items-center rounded-choice px-5 pt-2.5 pb-3 text-area whitespace-nowrap text-ink-3 hover:bg-s1 hover:text-ink [--lift:1.05]',
        'max-[1400px]:px-4 max-[1100px]:px-3 max-[1100px]:pt-2 max-[1100px]:pb-2.5',
        'aria-[current=page]:text-ink aria-[current=page]:after:absolute aria-[current=page]:after:bottom-0.5 aria-[current=page]:after:left-1/2 aria-[current=page]:after:h-1 aria-[current=page]:after:w-[22px] aria-[current=page]:after:-translate-x-1/2 aria-[current=page]:after:rounded-full aria-[current=page]:after:bg-current',
      )}
    >
      {section.label(t)}
      {section.id === 'system' && <ProblemCount />}
    </Link>
  )
}

/**
 * The account button (the avatar, and the name from 1280 px) and its raised panel of big rows:
 * who is signed in, an administrator's own pages (a member has them in the top row), the
 * language, the source code and signing out.
 */
function AccountMenu({ user }: { user: SessionUser }) {
  const { language, setLanguage, t } = useI18n()
  const signOut = useSignOut()
  return (
    <Menu
      label={t.nav.accountMenu}
      align="end"
      width={320}
      className="min-w-0"
      trigger={(props) => (
        <button
          {...props}
          aria-label={t.nav.accountOf(user.name)}
          className="focus-lit flex max-w-[240px] min-w-0 cursor-pointer items-center gap-3 rounded-full py-1 pr-[18px] pl-1 text-[19px] font-semibold text-ink hover:bg-s1 aria-expanded:bg-s1 [--lift:1.04] max-xl:p-1 max-[1180px]:p-0.5"
        >
          <Avatar name={user.name} image={userImageUrl(user)} size="md" />
          <span className="min-w-0 truncate max-xl:hidden">{user.name}</span>
        </button>
      )}
    >
      <MenuHeader>
        <Avatar name={user.name} image={userImageUrl(user)} size="md" />
        <div className="min-w-0">
          <p className="truncate text-[20px] font-semibold text-ink">{user.name}</p>
          <p className="text-small text-ink-3">
            {user.isAdministrator ? t.nav.administrator : t.nav.member}
          </p>
        </div>
      </MenuHeader>
      {user.isAdministrator && (
        <>
          {accountPages.map((page) => (
            <MenuItem key={page.id} icon={page.icon} to={page.to}>
              {page.label(t)}
            </MenuItem>
          ))}
          <MenuSeparator />
        </>
      )}
      <MenuChoice
        label={t.nav.language}
        value={language}
        onChange={setLanguage}
        options={languages.map((code) => ({
          value: code,
          label: t.language[code].short,
          title: t.language[code].name,
          lang: code,
        }))}
      />
      <MenuSeparator />
      <MenuItem icon={GithubLogoIcon} href={repositoryUrl} external>
        GitHub
      </MenuItem>
      <MenuItem icon={SignOutIcon} disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
        {signOut.isPending ? t.nav.signingOut : t.nav.signOut}
      </MenuItem>
    </Menu>
  )
}
