import {
  ArrowSquareInIcon,
  BellIcon,
  DevicesIcon,
  ListChecksIcon,
  PasswordIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type ReactNode } from 'react'
import { fetchMyDevices, fetchOwnImport, queryKeys, signOutMyDevice } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import DeviceList from '@/features/users/DeviceList'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import { useI18n } from '@/i18n'
import { SectionNav } from '@/ui'
import PasswordForm from './PasswordForm'
import ServerImport from './ServerImport'
import TrackingServices from './TrackingServices'

const sectionIds = ['tracking', 'import', 'notifications', 'devices', 'password'] as const
type SectionId = (typeof sectionIds)[number]

/** How often the user's own import is read while it runs. */
const ownImportPollMs = 2000

/**
 * `/me/account`: tracking services, importing from another server (unless the server turns it
 * off), notifications, devices and password, with a list of sections beside them.
 */
export default function AccountRoute() {
  const { t } = useI18n()
  const text = t.account
  const user = useSessionUser()
  const current = useCurrentSection()
  const ownImport = useQuery({
    queryKey: queryKeys.ownImport,
    queryFn: ({ signal }) => fetchOwnImport(signal),
    // React Query pauses this while the page is hidden, and stops it when the page is left.
    refetchInterval: (query) =>
      query.state.data?.import?.state === 'running' ? ownImportPollMs : false,
  })
  // Shown once the server says the setting allows it.
  const importing = ownImport.data?.enabled === true

  return (
    <PageLayout
      title={text.title}
      lede={
        <>
          {text.signedInAs}
          <span className="font-mono text-ink">{user.name}</span>.
        </>
      }
      nav={
        <SectionNav
          label={text.sectionsLabel}
          current={current}
          items={[
            {
              id: 'tracking',
              label: text.sections.tracking,
              icon: ListChecksIcon,
              to: '#tracking',
            },
            ...(importing
              ? [
                  {
                    id: 'import' as const,
                    label: text.sections.import,
                    icon: ArrowSquareInIcon,
                    to: '#import',
                  },
                ]
              : []),
            {
              id: 'notifications',
              label: text.sections.notifications,
              icon: BellIcon,
              to: '#notifications',
            },
            { id: 'devices', label: text.sections.devices, icon: DevicesIcon, to: '#devices' },
            { id: 'password', label: text.sections.password, icon: PasswordIcon, to: '#password' },
          ]}
        />
      }
    >
      <AccountSection id="tracking" title={text.sections.tracking} help={text.tracking.description}>
        <TrackingServices />
      </AccountSection>
      {importing && (
        <AccountSection
          id="import"
          title={text.sections.import}
          help={text.serverImport.description}
        >
          <ServerImport current={ownImport.data?.import ?? null} />
        </AccountSection>
      )}
      <AccountSection
        id="notifications"
        title={text.sections.notifications}
        help={text.notificationsHelp}
      >
        <NotificationTargets scope="own" />
      </AccountSection>
      <AccountSection id="devices" title={text.sections.devices} help={text.devicesHelp}>
        <DeviceList
          queryKey={queryKeys.myDevices}
          load={fetchMyDevices}
          signOut={signOutMyDevice}
        />
      </AccountSection>
      <AccountSection id="password" title={text.sections.password} help={text.passwordHelp}>
        <PasswordForm />
      </AccountSection>
    </PageLayout>
  )
}

/** A section of the page: an anchor for the list of sections, a heading and what it is for. */
function AccountSection({
  id,
  title,
  help,
  children,
}: {
  id: SectionId
  title: string
  help: string
  children: ReactNode
}) {
  const headingId = useId()
  return (
    <section
      id={id}
      aria-labelledby={headingId}
      className="scroll-mt-[calc(var(--spacing-topbar)+32px)] max-md:scroll-mt-[calc(var(--spacing-topbar)+72px)] max-md:first:pt-6"
    >
      <h2 id={headingId} className="text-h3 text-ink">
        {title}
      </h2>
      <p className="mt-1.5 max-w-[60ch] text-small text-ink-2">{help}</p>
      <div className="mt-5">{children}</div>
    </section>
  )
}

/**
 * The section in view, for the list of sections: the last one whose top has passed the upper
 * third of the window, or the last one once the page is scrolled to its end.
 */
function useCurrentSection(): SectionId {
  const [current, setCurrent] = useState<SectionId>('tracking')
  useEffect(() => {
    function update() {
      const atEnd = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4
      let found: SectionId = 'tracking'
      for (const id of sectionIds) {
        const element = document.getElementById(id)
        if (element && element.getBoundingClientRect().top <= window.innerHeight / 3) found = id
      }
      setCurrent(atEnd && window.scrollY > 0 ? sectionIds[sectionIds.length - 1] : found)
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    window.addEventListener('resize', update)
    return () => {
      window.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
    }
  }, [])
  return current
}
