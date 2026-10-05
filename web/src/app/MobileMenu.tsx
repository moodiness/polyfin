import {
  ArrowSquareOutIcon,
  GithubLogoIcon,
  ListIcon,
  SignOutIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useEffect, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { userImageUrl, type SessionUser } from '@/api'
import { useI18n } from '@/i18n'
import { Avatar, cx, IconButton, Modal } from '@/ui'
import { LanguageSwitch, repositoryUrl } from './Brand'
import { accountPages, adminSections, isCurrent, memberSections } from './navigation'
import { useSignOut } from './session'

const itemClass = cx(
  'flex h-11 w-full cursor-pointer items-center gap-3 rounded-field px-3 text-[15px] font-medium text-ink-2 transition-[background-color,color] duration-160 ease-nuit hover:bg-s2 hover:text-ink',
  'aria-[current=page]:bg-s3 aria-[current=page]:text-ink aria-[current=page]:shadow-[inset_0_0_0_1px] aria-[current=page]:shadow-line',
)

function Item({
  to,
  icon: Glyph,
  current,
  indent = false,
  children,
}: {
  to: string
  icon?: Icon
  current: boolean
  indent?: boolean
  children: ReactNode
}) {
  return (
    <Link
      to={to}
      aria-current={current ? 'page' : undefined}
      className={cx(itemClass, indent && 'h-10 pl-11 text-body')}
    >
      {Glyph && (
        <Glyph
          size={18}
          aria-hidden="true"
          className={cx('shrink-0', current ? 'text-link' : 'text-ink-3')}
        />
      )}
      {children}
    </Link>
  )
}

/**
 * The phone menu: the menu button of the TopBar and the sheet it opens, holding the sections with
 * their pages, the user's own pages, the web player, the language and signing out.
 */
export function MobileMenu({ user, webClient }: { user: SessionUser; webClient: boolean }) {
  const { t } = useI18n()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)
  const signOut = useSignOut()
  const sections = user.isAdministrator ? adminSections : memberSections

  // Following a link closes the sheet.
  useEffect(() => setOpen(false), [pathname])

  return (
    <>
      <IconButton
        label={t.nav.menu}
        icon={ListIcon}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      />
      <Modal open={open} onClose={() => setOpen(false)} title={t.nav.menu} width={400}>
        <div className="flex flex-col gap-1 p-3">
          <div className="mb-2 flex items-center gap-3 px-3 py-2">
            <Avatar name={user.name} image={userImageUrl(user)} size="lg" />
            <div className="min-w-0">
              <p className="truncate text-body font-semibold text-ink">{user.name}</p>
              <p className="text-[12.5px] text-ink-3">
                {user.isAdministrator ? t.nav.administrator : t.nav.member}
              </p>
            </div>
          </div>
          <nav aria-label={t.nav.label} className="flex flex-col gap-0.5">
            {sections.map((section) =>
              section.pages ? (
                <div key={section.id} className="flex flex-col gap-0.5">
                  <p className="flex h-10 items-center gap-3 px-3 text-[15px] font-medium text-ink">
                    <section.icon size={18} aria-hidden="true" className="text-ink-3" />
                    {section.label(t)}
                  </p>
                  {section.pages.map((page) => (
                    <Item
                      key={page.id}
                      to={page.to}
                      indent
                      current={pathname === page.to || pathname.startsWith(`${page.to}/`)}
                    >
                      {page.label(t)}
                    </Item>
                  ))}
                </div>
              ) : (
                <Item
                  key={section.id}
                  to={section.to}
                  icon={section.icon}
                  current={isCurrent(section, pathname)}
                >
                  {section.label(t)}
                </Item>
              ),
            )}
          </nav>
          {user.isAdministrator && (
            <>
              <hr className="mx-3 my-2 border-line" />
              {accountPages.map((page) => (
                <Item
                  key={page.id}
                  to={page.to}
                  icon={page.icon}
                  current={isCurrent(page, pathname)}
                >
                  {page.label(t)}
                </Item>
              ))}
            </>
          )}
          <hr className="mx-3 my-2 border-line" />
          {webClient && (
            <a href="/web/" target="_blank" rel="noopener" className={itemClass}>
              <ArrowSquareOutIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />
              {t.nav.webPlayer}
              <span className="sr-only">{t.nav.webPlayerHint}</span>
            </a>
          )}
          <div className="flex h-12 items-center justify-between px-3 text-[15px] font-medium text-ink-2">
            {t.nav.language}
            <LanguageSwitch size="md" />
          </div>
          <a href={repositoryUrl} target="_blank" rel="noopener" className={itemClass}>
            <GithubLogoIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />
            GitHub
          </a>
          <button
            type="button"
            className={itemClass}
            disabled={signOut.isPending}
            onClick={() => signOut.mutate()}
          >
            <SignOutIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />
            {signOut.isPending ? t.nav.signingOut : t.nav.signOut}
          </button>
        </div>
      </Modal>
    </>
  )
}
