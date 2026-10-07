import { MagnifyingGlassIcon, TerminalIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useLocation, useParams } from 'react-router'
import { fetchSettings, fetchVariables, queryKeys, type Settings } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import {
  isSettingsSection,
  settingsPath,
  settingsSections,
  type SettingsSectionId,
} from '@/app/navigation'
import NotFoundRoute from '@/features/home/NotFoundRoute'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  EmptyState,
  InlineError,
  Row,
  RowList,
  searchable,
  SectionNav,
  Skeleton,
  SkeletonText,
} from '@/ui'
import { rangeText } from './bounds'
import { settingEntries, variableAnchor, type RangeOf } from './catalog'
import SectionForm from './SectionForm'
import { sectionFields } from './sections'

/** `/settings/:section`: one section of the server settings. */
export default function SettingsRoute() {
  const { section } = useParams()
  if (!isSettingsSection(section)) return <NotFoundRoute />
  return <SettingsPage section={section} />
}

/** How long a setting opened by its anchor stays highlighted. */
const flashFor = 2_000

function SettingsPage({ section }: { section: SettingsSectionId }) {
  const { t } = useI18n()
  const text = t.settingsPage
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const [search, setSearch] = useState('')
  const location = useLocation()

  // Opening a setting (from the search, the palette or a link) ends the search.
  const [openedKey, setOpenedKey] = useState(location.key)
  if (openedKey !== location.key) {
    setOpenedKey(location.key)
    setSearch('')
  }

  // A setting's anchor scrolls to it once its section is drawn, and highlights it for a moment.
  const loaded = settings.data !== undefined
  useEffect(() => {
    if (!loaded || location.hash === '') return
    const target = document.getElementById(decodeURIComponent(location.hash.slice(1)))
    if (target === null) return
    const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    target.scrollIntoView({ block: 'center', behavior: still ? 'auto' : 'smooth' })
    target
      .querySelector<HTMLElement>('input, textarea, [role="combobox"], button[role="switch"]')
      ?.focus({ preventScroll: true })
    target.dataset.flash = ''
    const timer = setTimeout(() => delete target.dataset.flash, flashFor)
    return () => {
      clearTimeout(timer)
      delete target.dataset.flash
    }
  }, [loaded, location.hash, location.key])

  const current = settingsSections.find((item) => item.id === section)!
  const lede = sectionLede(t, section, settings.data)

  return (
    <PageLayout
      title={t.settings.title}
      lede={t.settings.description}
      nav={
        <SectionNav
          label={t.nav.settingsSectionsLabel}
          items={settingsSections.map((item) => ({
            id: item.id,
            label: t.nav.settingsSections[item.key],
            icon: item.icon,
            to: settingsPath(item.id),
          }))}
          search={{
            value: search,
            onChange: setSearch,
            placeholder: text.searchPlaceholder,
            hint: text.searchHint,
          }}
        />
      }
    >
      {search.trim() !== '' && (
        <SearchResults query={search.trim()} bounds={settings.data?.bounds} />
      )}
      <div hidden={search.trim() !== ''}>
        <header className="max-md:pt-6">
          <h2 className="text-h2 text-ink">{t.nav.settingsSections[current.key]}</h2>
          {lede !== undefined && (
            <p className="mt-1.5 max-w-[68ch] text-[14.5px] text-ink-2">{lede}</p>
          )}
        </header>
        {settings.isPending ? (
          <div aria-busy="true" aria-label={t.common.loading} className="mt-8 space-y-8">
            {[0, 1, 2].map((index) => (
              <div key={index} className="space-y-3">
                <Skeleton className="h-4 w-56" />
                <SkeletonText lines={2} />
                <Skeleton className="h-10 w-full max-w-sm" />
              </div>
            ))}
          </div>
        ) : settings.isError ? (
          <InlineError
            className="mt-8"
            onRetry={() => void settings.refetch()}
            retrying={settings.isFetching}
          >
            {errorMessage(t, settings.error)}
          </InlineError>
        ) : (
          <SectionForm key={section} section={section} initial={settings.data}>
            {sectionFields[section]}
          </SectionForm>
        )}
      </div>
    </PageLayout>
  )
}

/** The line under a section's title; some say where files go, from the settings. */
function sectionLede(
  t: Messages,
  section: SettingsSectionId,
  settings:
    | Pick<
        Settings,
        | 'recording'
        | 'recordingsFolder'
        | 'recordingsFolderDefault'
        | 'backups'
        | 'backupFolder'
        | 'backupFolderDefault'
      >
    | undefined,
): string | undefined {
  const s = t.settings
  const ledes = t.settingsPage.ledes
  switch (section) {
    case 'general':
      return ledes.general
    case 'playback':
      return ledes.playback
    case 'conversion':
      return s.conversion.description
    case 'content':
      return ledes.content
    case 'catalogs':
      return ledes.catalogs
    case 'thumbnails':
      return s.thumbnailsHelp
    case 'security':
      return ledes.security
    case 'tracking':
      return s.tracking.description
    case 'live-tv':
      return ledes.liveTv
    case 'recordings':
      return settings?.recording
        ? s.recordingsFolder(settings.recordingsFolder || settings.recordingsFolderDefault)
        : undefined
    case 'backups':
      return settings?.backups
        ? s.backupsFolder(settings.backupFolder || settings.backupFolderDefault)
        : undefined
    case 'web-player':
      return s.webPlayerHelp
    case 'diagnostics':
      return ledes.diagnostics
  }
}

/**
 * The settings of every section that match the search, each opening at its anchor; their help
 * texts give the bounds once the settings are loaded.
 */
function SearchResults({
  query,
  bounds,
}: {
  query: string
  bounds: Settings['bounds'] | undefined
}) {
  const { t, language } = useI18n()
  const range: RangeOf = (name, scale) => rangeText(bounds?.[name], language, scale)
  const text = t.settingsPage
  const variables = useQuery({
    queryKey: queryKeys.variables,
    queryFn: ({ signal }) => fetchVariables(signal),
    staleTime: Infinity,
  })
  const words = searchable(query)
  const sectionOf = (id: SettingsSectionId) => settingsSections.find((item) => item.id === id)!
  const matches = [
    ...settingEntries
      .filter((entry) =>
        searchable(
          [
            entry.label(t),
            entry.help?.(t, range) ?? '',
            ...(entry.keywords ?? []),
            t.nav.settingsSections[sectionOf(entry.section).key],
          ].join(' '),
        ).includes(words),
      )
      .map((entry) => ({
        key: `${entry.section}:${entry.anchor}`,
        label: entry.label(t),
        hint: t.nav.settingsSections[sectionOf(entry.section).key],
        icon: sectionOf(entry.section).icon,
        to: settingsPath(entry.section, entry.anchor),
      })),
    ...(variables.data ?? [])
      .filter((variable) => searchable(variable.name).includes(words))
      .map((variable) => ({
        key: `variable:${variable.name}`,
        label: variable.name,
        hint: text.variableHint,
        icon: TerminalIcon,
        to: settingsPath('diagnostics', variableAnchor(variable.name)),
      })),
  ]

  return (
    <section aria-labelledby="settings-search-title" className="max-md:pt-6">
      <h2 id="settings-search-title" role="status" className="mb-4 text-h3 text-ink">
        {text.results(matches.length)}
      </h2>
      {matches.length === 0 ? (
        <EmptyState icon={MagnifyingGlassIcon} title={text.noMatch}>
          {text.noMatchHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={text.results(matches.length)}>
          {matches.map((match) => (
            <Row
              key={match.key}
              leading={match.icon}
              title={match.label}
              meta={`${t.nav.settings} › ${match.hint}`}
              to={match.to}
            />
          ))}
        </RowList>
      )}
    </section>
  )
}
