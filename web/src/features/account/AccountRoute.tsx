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
  userImageUrl,
  type OwnAppImport,
  type SessionUser,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import DeviceList from '@/features/users/DeviceList'
import { useWatching, WatchingLink } from '@/features/users/rows'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import StatisticsView from '@/features/statistics/StatisticsView'
import { useI18n } from '@/i18n'
import { Avatar, SectionNav, type SectionNavItem } from '@/ui'
import AppImport, { AppConnections } from './AppImport'
import PasswordForm from './PasswordForm'
import ServerImport from './ServerImport'
import SharedLinks from './SharedLinks'
import TrackingServices from './TrackingServices'
import { ReminderList, ReminderSettingsForm } from './Reminders'
import { OwnMetadataLanguage } from './MetadataLanguage'
import { OwnWebTheme } from './WebTheme'
import { OwnAudioRegion } from './AudioRegion'
import { OwnInterfaceSize } from './InterfaceSize'
import { OwnSoundOptions } from './Sound'
import ResetPreferences from './ResetPreferences'
import TasteProfile from './TasteProfile'
import { HiddenList, HomeRowList, NextUpList, ResumeList } from './WatchingLists'
import OwnTrackChoices from './TrackChoices'
import TwoStep from './TwoStep'

const sectionIds = [
  'statistics',
  'tracking',
  'resume',
  'next-up',
  'hidden',
  'home-rows',
  'taste',
  'track-choices',
  'metadata-language',
  'web-theme',
  'audio-region',
  'interface-size',
  'sound',
  'shared-links',
  'reminders',
  'import',
  'app-import',
  'notifications',
  'devices',
  'password',
  'two-step',
] as const
type SectionId = (typeof sectionIds)[number]

/** How often the user's own import is read while it runs. */
const ownImportPollMs = 2000

/** A section's anchor stays clear of the top row, and on a phone of the strip of sections too. */
const anchorMargin =
  'scroll-mt-[calc(var(--spacing-topbar)+2rem)] max-md:scroll-mt-[calc(var(--spacing-topbar)+4.5rem)]'

/**
 * `/me/account`, in the frame of a user's page: a band with the person (avatar, name, role) and
 * Reset my preferences, then the sections as words on the left and their content beside them:
 * statistics, tracking services, Continue Watching, hidden titles, home rows, the taste profile,
 * track choices, the metadata language, the web player theme, the audio region, the interface
 * size, sound, shared links (while the user may share or has links), programme reminders (with
 * the Live TV permission), importing from another server and from an app (unless the server
 * turns it off), notifications, devices, password and two-step sign-in. Every setting saves as it
 * changes, so the page has no unsaved dock.
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
  const appImportShown = importingFromApps || appConnections.length > 0
  const { hash } = useLocation()

  // Sections drawn once their answer comes: a #section address scrolls once it is there.
  useEffect(() => {
    if (hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [hash, sharing, reminding])

  const shown: Record<SectionId, boolean> = {
    statistics: true,
    tracking: true,
    resume: true,
    'next-up': true,
    hidden: true,
    'home-rows': true,
    taste: true,
    'track-choices': true,
    'metadata-language': true,
    'web-theme': true,
    'audio-region': true,
    'interface-size': true,
    sound: true,
    'shared-links': sharing,
    reminders: reminding,
    import: importing,
    'app-import': appImportShown,
    notifications: true,
    devices: true,
    password: true,
    'two-step': true,
  }
  const labels: Record<SectionId, string> = {
    statistics: text.sections.statistics,
    tracking: text.sections.tracking,
    resume: text.sections.resume,
    'next-up': text.sections.nextUp,
    hidden: text.sections.hidden,
    'home-rows': text.sections.homeRows,
    taste: text.sections.taste,
    'track-choices': text.sections.trackChoices,
    'metadata-language': text.sections.metadataLanguage,
    'web-theme': text.sections.webTheme,
    'audio-region': text.sections.audioRegion,
    'interface-size': text.sections.interfaceSize,
    sound: text.sections.sound,
    'shared-links': text.sections.sharedLinks,
    reminders: text.sections.reminders,
    import: text.sections.import,
    'app-import': text.sections.appImport,
    notifications: text.sections.notifications,
    devices: text.sections.devices,
    password: text.sections.password,
    'two-step': text.sections.twoStep,
  }
  // Words only, as on a user's page.
  const items: SectionNavItem[] = sectionIds
    .filter((id) => shown[id])
    .map((id) => ({ id, label: labels[id], to: `#${id}` }))

  return (
    <PageLayout
      head={<PersonBand user={user} />}
      nav={<SectionNav label={text.sectionsLabel} current={current} items={items} />}
    >
      <AccountSection id="statistics" title={labels.statistics} help={text.statisticsHelp}>
        <div className="space-y-10">
          <StatisticsView scope="own" />
        </div>
      </AccountSection>
      <AccountSection id="tracking" title={labels.tracking} help={text.tracking.description}>
        <TrackingServices />
      </AccountSection>
      <AccountSection id="resume" title={labels.resume} help={text.watching.resumeHelp}>
        <ResumeList />
      </AccountSection>
      <AccountSection id="next-up" title={labels['next-up']} help={text.watching.nextUpHelp}>
        <NextUpList />
      </AccountSection>
      <AccountSection id="hidden" title={labels.hidden} help={text.watching.hiddenHelp}>
        <HiddenList />
      </AccountSection>
      <AccountSection id="home-rows" title={labels['home-rows']} help={text.homeRows.help}>
        <HomeRowList />
      </AccountSection>
      <AccountSection id="taste" title={labels.taste} help={text.taste.help}>
        <TasteProfile />
      </AccountSection>
      <AccountSection
        id="track-choices"
        title={labels['track-choices']}
        help={text.trackChoices.help}
      >
        <OwnTrackChoices />
      </AccountSection>
      {/* Four settings of one row each: the row is the section, its label the heading's
              words, its help shown while it is lit. */}
      <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
        <div id="metadata-language" className={anchorMargin}>
          <OwnMetadataLanguage />
        </div>
        <div id="web-theme" className={anchorMargin}>
          <OwnWebTheme />
        </div>
        <div id="audio-region" className={anchorMargin}>
          <OwnAudioRegion />
        </div>
        <div id="interface-size" className={anchorMargin}>
          <OwnInterfaceSize />
        </div>
      </div>
      <AccountSection id="sound" title={labels.sound} help={text.sound.help}>
        <OwnSoundOptions />
      </AccountSection>
      {sharing && (
        <AccountSection
          id="shared-links"
          title={labels['shared-links']}
          help={text.sharedLinksHelp}
        >
          <SharedLinks allowed={shareLinks.data.allowed} links={shareLinks.data.links} />
        </AccountSection>
      )}
      {reminding && (
        <AccountSection id="reminders" title={labels.reminders} help={text.reminders.help}>
          <ReminderList />
        </AccountSection>
      )}
      {ownImport.data !== undefined && importing && (
        <AccountSection id="import" title={labels.import} help={text.serverImport.description}>
          <ServerImport own={ownImport.data} />
        </AccountSection>
      )}
      {ownAppImport.data !== undefined && appImportShown && (
        <AccountSection
          id="app-import"
          title={labels['app-import']}
          help={text.appImport.description}
        >
          <div className="flex flex-col gap-6">
            <AppConnections
              connections={appConnections}
              actions={ownAppConnections}
              onChanged={() =>
                void queryClient.invalidateQueries({ queryKey: queryKeys.ownAppImport })
              }
              scope="own"
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
      <AccountSection id="notifications" title={labels.notifications} help={text.notificationsHelp}>
        <div className="flex flex-col gap-10">
          <NotificationTargets scope="own" />
          <ReminderSettingsForm />
        </div>
      </AccountSection>
      <AccountSection id="devices" title={labels.devices} help={text.devicesHelp}>
        <DeviceList
          queryKey={queryKeys.myDevices}
          load={fetchMyDevices}
          signOut={signOutMyDevice}
        />
      </AccountSection>
      <AccountSection id="password" title={labels.password} help={text.passwordHelp}>
        <PasswordForm />
      </AccountSection>
      <AccountSection id="two-step" title={labels['two-step']} help={text.twoStep.help}>
        <TwoStep />
      </AccountSection>
    </PageLayout>
  )
}

/**
 * The band at the top, as on a user's page: the avatar, the page's title, who is signed in and
 * their role, what they watch now, and Reset my preferences.
 */
function PersonBand({ user }: { user: SessionUser }) {
  const { t } = useI18n()
  const text = t.account
  const watching = useWatching(user.id)
  const [artFailed, setArtFailed] = useState<string | null>(null)
  const posterId = watching?.item?.posterId
  const art = posterId && artFailed !== posterId ? posterId : null
  return (
    <header className="mb-14 flex flex-wrap items-center gap-x-8 gap-y-5 rounded-panel bg-s1 px-8 py-7 max-md:mb-6 max-md:gap-x-5 max-md:rounded-[1.5rem] max-md:px-5 max-md:py-5">
      <Avatar
        name={user.name}
        image={userImageUrl(user)}
        size="xl"
        className="max-md:size-16 max-md:text-[1.75rem]"
      />
      <div className="min-w-0 flex-1 basis-60 max-md:basis-40">
        <h1 className="text-h1 break-words text-ink max-md:text-[2.25rem]">{text.title}</h1>
        <p className="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-lead text-ink-2 max-md:text-[1.09375rem]">
          <span className="min-w-0 break-words">
            {text.signedInAs}
            <span className="font-semibold text-ink">{user.name}</span>
          </span>
          <span>{user.isAdministrator ? text.roles.administrator : text.roles.member}</span>
        </p>
        {watching !== null && (
          <WatchingLink session={watching} art={art} onArtError={() => setArtFailed(art)} />
        )}
      </div>
      <div className="flex max-w-full flex-wrap gap-3">
        <ResetPreferences />
      </div>
    </header>
  )
}

/** A section of the page: an anchor for the list of sections, its group title and what it is for. */
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
    <section id={id} aria-labelledby={headingId} className={anchorMargin}>
      <h2 id={headingId} className="text-group text-ink">
        {title}
      </h2>
      <p className="mt-2 max-w-[66ch] text-small text-ink-2">{help}</p>
      <div className="mt-6 max-md:mt-5">{children}</div>
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
