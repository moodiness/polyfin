import {
  AlarmIcon,
  ArrowSquareInIcon,
  BellIcon,
  ChartBarIcon,
  ClockCounterClockwiseIcon,
  DeviceMobileIcon,
  DevicesIcon,
  EyeSlashIcon,
  ListChecksIcon,
  PasswordIcon,
  RowsIcon,
  ShareNetworkIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type ReactNode } from 'react'
import { useLocation, useSearchParams } from 'react-router'
import {
  fetchMyDevices,
  fetchOwnAppImport,
  fetchOwnReminders,
  fetchOwnImport,
  fetchOwnShareLinks,
  ownAppConnections,
  ownAppReaders,
  queryClient,
  queryKeys,
  signOutMyDevice,
  startOwnAppImport,
  type OwnAppImport,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import DeviceList from '@/features/users/DeviceList'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import StatisticsView from '@/features/statistics/StatisticsView'
import { useI18n } from '@/i18n'
import { SectionNav } from '@/ui'
import AppImport, { AppConnections } from './AppImport'
import PasswordForm from './PasswordForm'
import ServerImport from './ServerImport'
import SharedLinks from './SharedLinks'
import TrackingServices from './TrackingServices'
import { ReminderList, ReminderSettingsForm } from './Reminders'
import { HiddenList, HomeRowList, ResumeList } from './WatchingLists'

const sectionIds = [
  'statistics',
  'tracking',
  'resume',
  'hidden',
  'home-rows',
  'shared-links',
  'reminders',
  'import',
  'app-import',
  'notifications',
  'devices',
  'password',
] as const
type SectionId = (typeof sectionIds)[number]

/** How often the user's own import is read while it runs. */
const ownImportPollMs = 2000

/**
 * `/me/account`: statistics, tracking services, Continue Watching, hidden titles, home rows, shared
 * links (while the user may share or has links), programme reminders (with the Live TV permission),
 * importing from another server and from an app (unless the server turns it off), notifications,
 * devices and password, with a list of sections beside them.
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
  const ownAppImport = useQuery({
    queryKey: queryKeys.ownAppImport,
    queryFn: ({ signal }) => fetchOwnAppImport(signal),
    refetchInterval: (query) =>
      query.state.data?.import?.state === 'running' ||
      query.state.data?.connections.some((c) => c.importing)
        ? ownImportPollMs
        : false,
  })
  // Shown once the server says the setting allows it; the apps connected, while any is, to
  // disconnect them.
  const importing = ownImport.data?.enabled === true
  const importingFromApps = ownAppImport.data?.enabled === true
  const appConnections = ownAppImport.data?.connections ?? []
  const shareLinks = useQuery({
    queryKey: queryKeys.ownShareLinks,
    queryFn: ({ signal }) => fetchOwnShareLinks(signal),
  })
  // Shown while the user may share or has links, and when the web player's Share opened the page,
  // which then says why no link can be made.
  const [params] = useSearchParams()
  const sharing =
    shareLinks.data !== undefined &&
    (shareLinks.data.allowed || shareLinks.data.links.length > 0 || params.has('share'))
  // Reminders are listed for a user with the Live TV permission.
  const reminders = useQuery({
    queryKey: queryKeys.ownReminders,
    queryFn: ({ signal }) => fetchOwnReminders(signal),
  })
  const reminding = reminders.data?.available === true
  const { hash } = useLocation()

  // Sections drawn once their answer comes: a #section address scrolls once it is there.
  useEffect(() => {
    if (hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [hash, sharing, reminding])

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
              id: 'statistics',
              label: text.sections.statistics,
              icon: ChartBarIcon,
              to: '#statistics',
            },
            {
              id: 'tracking',
              label: text.sections.tracking,
              icon: ListChecksIcon,
              to: '#tracking',
            },
            {
              id: 'resume',
              label: text.sections.resume,
              icon: ClockCounterClockwiseIcon,
              to: '#resume',
            },
            { id: 'hidden', label: text.sections.hidden, icon: EyeSlashIcon, to: '#hidden' },
            {
              id: 'home-rows',
              label: text.sections.homeRows,
              icon: RowsIcon,
              to: '#home-rows',
            },
            ...(sharing
              ? [
                  {
                    id: 'shared-links' as const,
                    label: text.sections.sharedLinks,
                    icon: ShareNetworkIcon,
                    to: '#shared-links',
                  },
                ]
              : []),
            ...(reminding
              ? [
                  {
                    id: 'reminders' as const,
                    label: text.sections.reminders,
                    icon: AlarmIcon,
                    to: '#reminders',
                  },
                ]
              : []),
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
            ...(importingFromApps || appConnections.length > 0
              ? [
                  {
                    id: 'app-import' as const,
                    label: text.sections.appImport,
                    icon: DeviceMobileIcon,
                    to: '#app-import',
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
      <AccountSection id="statistics" title={text.sections.statistics} help={text.statisticsHelp}>
        <div className="space-y-10">
          <StatisticsView scope="own" />
        </div>
      </AccountSection>
      <AccountSection id="tracking" title={text.sections.tracking} help={text.tracking.description}>
        <TrackingServices />
      </AccountSection>
      <AccountSection id="resume" title={text.sections.resume} help={text.watching.resumeHelp}>
        <ResumeList />
      </AccountSection>
      <AccountSection id="hidden" title={text.sections.hidden} help={text.watching.hiddenHelp}>
        <HiddenList />
      </AccountSection>
      <AccountSection id="home-rows" title={text.sections.homeRows} help={text.homeRows.help}>
        <HomeRowList />
      </AccountSection>
      {sharing && (
        <AccountSection
          id="shared-links"
          title={text.sections.sharedLinks}
          help={text.sharedLinksHelp}
        >
          <SharedLinks allowed={shareLinks.data.allowed} links={shareLinks.data.links} />
        </AccountSection>
      )}
      {reminding && (
        <AccountSection id="reminders" title={text.sections.reminders} help={text.reminders.help}>
          <ReminderList />
        </AccountSection>
      )}
      {ownImport.data !== undefined && importing && (
        <AccountSection
          id="import"
          title={text.sections.import}
          help={text.serverImport.description}
        >
          <ServerImport own={ownImport.data} />
        </AccountSection>
      )}
      {ownAppImport.data !== undefined && (importingFromApps || appConnections.length > 0) && (
        <AccountSection
          id="app-import"
          title={text.sections.appImport}
          help={text.appImport.description}
        >
          <div className="flex flex-col gap-6">
            <AppConnections
              connections={appConnections}
              actions={ownAppConnections}
              onChanged={() =>
                void queryClient.invalidateQueries({ queryKey: queryKeys.ownAppImport })
              }
              own
              importEveryMinutes={ownAppImport.data.importEveryMinutes}
            />
            {importingFromApps && (
              <AppImport
                current={ownAppImport.data.import}
                addonsAllowed={ownAppImport.data.addonsAllowed}
                readers={ownAppReaders}
                start={startOwnAppImport}
                onStarted={(started) =>
                  queryClient.setQueryData<OwnAppImport>(
                    queryKeys.ownAppImport,
                    (data) => data && { ...data, import: started },
                  )
                }
                own
                importEveryMinutes={ownAppImport.data.importEveryMinutes}
              />
            )}
          </div>
        </AccountSection>
      )}
      <AccountSection
        id="notifications"
        title={text.sections.notifications}
        help={text.notificationsHelp}
      >
        <div className="flex flex-col gap-6">
          <NotificationTargets scope="own" />
          <ReminderSettingsForm />
        </div>
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
  const [current, setCurrent] = useState<SectionId>(sectionIds[0])
  useEffect(() => {
    function update() {
      const atEnd = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4
      let found: SectionId = sectionIds[0]
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
