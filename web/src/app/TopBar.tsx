import {
  ArrowSquareOutIcon,
  CaretDownIcon,
  GithubLogoIcon,
  MagnifyingGlassIcon,
  SignOutIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router'
import { fetchStatus, queryKeys, userImageUrl, type SessionUser } from '@/api'
import { languages, useI18n } from '@/i18n'
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
import { MobileMenu } from './MobileMenu'
import { accountPages, adminSections, isCurrent, memberSections } from './navigation'
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
 * The 60 px bar on top of every signed-in page: the logo, the sections (or a member's own
 * pages), search, the web player and the account menu. Below 768 px it keeps the logo, search
 * and a menu button that opens the same entries in a sheet.
 */
export function TopBar({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const { pathname } = useLocation()
  const openPalette = useOpenPalette()
  const webClient = useWebClient()
  const sections = user.isAdministrator ? adminSections : memberSections

  return (
    <header className="sticky top-0 z-30 h-topbar border-b border-line bg-bg/78 backdrop-blur-[18px] backdrop-saturate-130">
      <div className="mx-auto flex h-full max-w-page items-center gap-10 px-14 max-xl:gap-6 max-xl:px-8 max-md:gap-3 max-md:px-5">
        <Brand />
        <nav aria-label={t.nav.label} className="flex h-full items-stretch gap-1 max-md:hidden">
          {sections.map((section) => (
            <Link
              key={section.id}
              to={section.to}
              aria-current={isCurrent(section, pathname) ? 'page' : undefined}
              className={cx(
                'relative flex items-center px-3 text-body font-medium whitespace-nowrap text-ink-2 transition-colors duration-160 ease-nuit hover:text-ink max-lg:px-2',
                'after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:scale-x-0 after:rounded-full after:bg-accent after:transition-transform after:duration-220 after:ease-nuit max-lg:after:inset-x-2',
                'aria-[current=page]:text-ink aria-[current=page]:after:scale-x-100',
                'focus-visible:rounded-field focus-visible:outline-offset-[-6px]',
              )}
            >
              {section.label(t)}
            </Link>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            onClick={openPalette}
            aria-label={t.nav.searchShortcut(`${modKey} K`)}
            aria-keyshortcuts="Meta+K Control+K /"
            className={cx(
              'flex h-9 cursor-pointer items-center gap-2.5 rounded-field border border-line-2 bg-s2 pr-1.5 pl-3 text-control text-ink-3 transition-[background-color,border-color] duration-160 ease-nuit hover:border-line-3 hover:bg-s3',
              'w-[248px] max-xl:w-auto max-md:w-9 max-md:justify-center max-md:border-transparent max-md:bg-transparent max-md:p-0 max-md:text-ink-2',
            )}
          >
            <MagnifyingGlassIcon size={16} aria-hidden="true" className="shrink-0" />
            <span className="flex-1 text-left max-xl:hidden">{t.nav.search}</span>
            <Kbd className="max-md:hidden">{modKey === '⌘' ? '⌘K' : 'Ctrl K'}</Kbd>
          </button>
          {webClient && (
            <a
              href="/web/"
              target="_blank"
              rel="noopener"
              title={t.nav.webPlayerHint}
              className="inline-flex h-9 items-center gap-2 rounded-field px-3 text-control font-medium whitespace-nowrap text-ink-2 transition-[background-color,color] duration-160 ease-nuit hover:bg-s2 hover:text-ink max-md:hidden"
            >
              <span className="max-lg:sr-only">{t.nav.webPlayer}</span>
              <ArrowSquareOutIcon size={16} aria-hidden="true" />
              <span className="sr-only">{t.nav.webPlayerHint}</span>
            </a>
          )}
          <div className="max-md:hidden">
            <AccountMenu user={user} />
          </div>
          <div className="md:hidden">
            <MobileMenu user={user} webClient={webClient} />
          </div>
        </div>
      </div>
    </header>
  )
}

/**
 * The account button and its menu: who is signed in, an administrator's own pages (a member has
 * them in the bar), the language, the source code and signing out.
 */
function AccountMenu({ user }: { user: SessionUser }) {
  const { language, setLanguage, t } = useI18n()
  const signOut = useSignOut()
  return (
    <Menu
      label={t.nav.accountMenu}
      align="end"
      width={264}
      trigger={(props) => (
        <button
          {...props}
          aria-label={t.nav.accountOf(user.name)}
          className="flex h-9 cursor-pointer items-center gap-1.5 rounded-field pr-1.5 pl-1 text-ink-3 transition-colors duration-160 ease-nuit hover:bg-s2 aria-expanded:bg-s2"
        >
          <Avatar name={user.name} image={userImageUrl(user)} />
          <CaretDownIcon size={14} aria-hidden="true" />
        </button>
      )}
    >
      <MenuHeader>
        <Avatar name={user.name} image={userImageUrl(user)} />
        <div className="min-w-0">
          <p className="truncate text-body font-semibold text-ink">{user.name}</p>
          <p className="text-[12.5px] text-ink-3">
            {user.isAdministrator ? t.nav.administrator : t.nav.member}
          </p>
        </div>
      </MenuHeader>
      <MenuSeparator />
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
