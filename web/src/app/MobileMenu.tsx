import {
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  GithubLogoIcon,
  GlobeIcon,
  ListIcon,
  MagnifyingGlassIcon,
  MonitorPlayIcon,
  SignOutIcon,
  XIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { userImageUrl, type SessionUser } from '@/api'
import { useI18n, type Messages } from '@/i18n'
import { webPlayerLink } from '@/jellyfinApps'
import { Avatar, cx } from '@/ui'
import { LanguageSwitch, repositoryUrl } from './Brand'
import {
  accountPages,
  adminSections,
  isCurrent,
  memberSections,
  settingsPath,
  settingsSections,
  type NavPage,
  type NavSection,
} from './navigation'
import { useOpenPalette } from './palette/CommandPalette'
import { useSignOut } from './session'
import { ProblemCount, topControl } from './shellParts'

/** The rows of the menu's foot: the account's pages, the web player, the language, signing out. */
const rowClass =
  'focus-lit flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-choice px-3 text-left text-lead text-ink hover:bg-s1 [--lift:1.012] disabled:cursor-not-allowed disabled:opacity-45'

function RowIcon({ icon: Glyph }: { icon: Icon }) {
  return <Glyph size="1.25rem" aria-hidden="true" className="shrink-0 text-ink-2" />
}

/** The pages under an area, as the menu lists them: Content's and System's, Settings' sections. */
function areaPages(section: NavSection, t: Messages): readonly { to: string; label: string }[] {
  if (section.pages) return section.pages.map((page) => ({ to: page.to, label: page.label(t) }))
  if (section.id === 'settings') {
    return settingsSections.map((each) => ({
      to: settingsPath(each.id),
      label: t.nav.settingsSections[each.key],
    }))
  }
  return []
}

function isCurrentPage(page: { to: string }, pathname: string): boolean {
  return pathname === page.to || pathname.startsWith(`${page.to}/`)
}

/**
 * The phone menu: the menu button of the top row and the full-screen room it opens. The account
 * at the top, then search, then the five areas as section-title words, each with its pages as
 * smaller words, then My account, My sources, Quick Connect, the web player, the language and
 * signing out. A member's areas are their own pages, so their foot starts at the web player.
 * `inPlace`: the web player opens in place, as the way back to it.
 */
export function MobileMenu({
  user,
  webClient,
  inPlace,
}: {
  user: SessionUser
  webClient: boolean
  inPlace: boolean
}) {
  const { t } = useI18n()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)
  const dialog = useRef<HTMLDialogElement>(null)
  const openPalette = useOpenPalette()
  const signOut = useSignOut()
  const sections = user.isAdministrator ? adminSections : memberSections

  // Following a link closes the menu.
  useEffect(() => setOpen(false), [pathname])

  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) {
      element.showModal()
      // The light starts on the current area, as the menu opens.
      element.querySelector<HTMLElement>('[data-area][aria-current="page"]')?.focus()
    }
    if (!open && element.open) element.close()
  }, [open])

  return (
    <>
      <button
        type="button"
        aria-label={t.nav.menu}
        title={t.nav.menu}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        className={cx(topControl, 'size-11 justify-center')}
      >
        <ListIcon size="1.25rem" aria-hidden="true" />
      </button>
      <dialog
        ref={dialog}
        aria-label={t.nav.menu}
        onCancel={(event) => {
          event.preventDefault()
          setOpen(false)
        }}
        onClick={(event) => {
          // A link to the page already shown changes no address: it closes the menu too.
          if ((event.target as HTMLElement).closest('a')) setOpen(false)
        }}
        className="m-0 h-dvh max-h-none w-screen max-w-none overflow-y-auto overscroll-contain bg-bg bg-[radial-gradient(600px_400px_at_0%_0%,rgb(76_63_217/0.22),transparent_70%)] p-0 text-ink backdrop:bg-bg open:animate-fade"
      >
        {open && (
          <div className="px-4 pt-3 pb-8">
            <div className="mb-3 flex h-14 items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-3">
                <Avatar name={user.name} image={userImageUrl(user)} size="md" />
                <div className="min-w-0">
                  <p className="truncate text-lead font-semibold">{user.name}</p>
                  <p className="text-micro text-ink-3">
                    {user.isAdministrator ? t.nav.administrator : t.nav.member}
                  </p>
                </div>
              </div>
              <button
                type="button"
                aria-label={t.nav.closeMenu}
                title={t.nav.closeMenu}
                onClick={() => setOpen(false)}
                className={cx(topControl, 'size-11 justify-center')}
              >
                <XIcon size="1.25rem" aria-hidden="true" />
              </button>
            </div>
            <button
              type="button"
              onClick={() => {
                setOpen(false)
                openPalette()
              }}
              className={cx(
                topControl,
                'mb-2.5 h-11 w-full gap-2.5 px-4 text-control text-ink-3 [--lift:1.02]',
              )}
            >
              <MagnifyingGlassIcon
                size="1.25rem"
                aria-hidden="true"
                className="shrink-0 text-ink-2"
              />
              <span className="truncate">{t.palette.placeholder}</span>
              <span className="sr-only">{t.nav.search}</span>
            </button>
            <nav aria-label={t.nav.label}>
              {sections.map((section) => (
                <Area
                  key={section.id}
                  section={section}
                  current={isCurrent(section, pathname)}
                  pages={areaPages(section, t)}
                  pathname={pathname}
                />
              ))}
            </nav>
            <div className="mt-3 flex flex-col pt-3 shadow-[inset_0_1px_0] shadow-line-2">
              {user.isAdministrator &&
                accountPages.map((page: NavPage) => (
                  <Link
                    key={page.id}
                    to={page.to}
                    aria-current={isCurrentPage(page, pathname) ? 'page' : undefined}
                    className={cx(rowClass, 'aria-[current=page]:bg-s2')}
                  >
                    <RowIcon icon={page.icon} />
                    {page.label(t)}
                  </Link>
                ))}
              {webClient && (
                <a {...webPlayerLink(inPlace)} className={rowClass}>
                  {inPlace ? (
                    <>
                      <RowIcon icon={ArrowLeftIcon} />
                      {t.nav.backToWebPlayer}
                    </>
                  ) : (
                    <>
                      <RowIcon icon={MonitorPlayIcon} />
                      {t.nav.webPlayer}
                      <span className="sr-only">{t.nav.webPlayerHint}</span>
                      <ArrowSquareOutIcon
                        size="1.125rem"
                        aria-hidden="true"
                        className="ml-auto shrink-0 text-ink-3"
                      />
                    </>
                  )}
                </a>
              )}
              <div className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-2 px-3 py-1 text-lead">
                <RowIcon icon={GlobeIcon} />
                <span className="mr-auto">{t.nav.language}</span>
                <LanguageSwitch className="w-[11.5rem]" />
              </div>
              <a href={repositoryUrl} target="_blank" rel="noopener" className={rowClass}>
                <RowIcon icon={GithubLogoIcon} />
                GitHub
                <ArrowSquareOutIcon
                  size="1.125rem"
                  aria-hidden="true"
                  className="ml-auto shrink-0 text-ink-3"
                />
              </a>
              <button
                type="button"
                className={rowClass}
                disabled={signOut.isPending}
                onClick={() => signOut.mutate()}
              >
                <RowIcon icon={SignOutIcon} />
                {signOut.isPending ? t.nav.signingOut : t.nav.signOut}
              </button>
            </div>
          </div>
        )}
      </dialog>
    </>
  )
}

/** An area as a section title's word (lit while current), and its pages as smaller words under it. */
function Area({
  section,
  current,
  pages,
  pathname,
}: {
  section: NavSection
  current: boolean
  pages: readonly { to: string; label: string }[]
  pathname: string
}): ReactNode {
  const { t } = useI18n()
  return (
    <div className="pt-1 pb-2">
      <Link
        to={section.to}
        data-area
        aria-current={current ? 'page' : undefined}
        className="focus-lit -mx-1 flex min-h-12 items-center justify-between gap-3 rounded-row px-3 text-h2 text-ink-2 hover:bg-s1 hover:text-ink [--lift:1.02] aria-[current=page]:lit"
      >
        <span className="min-w-0 break-words">{section.label(t)}</span>
        {section.id === 'system' && (
          <ProblemCount className="ml-0 h-auto rounded-full px-2 py-0.5 text-small" />
        )}
      </Link>
      {pages.length > 0 && (
        <div className="flex flex-wrap gap-x-1 gap-y-0.5 px-0.5 pt-1">
          {pages.map((page) => (
            <Link
              key={page.to}
              to={page.to}
              aria-current={isCurrentPage(page, pathname) ? 'page' : undefined}
              className="focus-lit rounded-control px-2.5 py-2 text-control text-ink-2 hover:bg-s1 hover:text-ink [--lift:1.04] aria-[current=page]:font-semibold aria-[current=page]:text-ink aria-[current=page]:underline aria-[current=page]:decoration-2 aria-[current=page]:underline-offset-[0.375rem]"
            >
              {page.label}
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
