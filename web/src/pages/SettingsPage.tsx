import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react'
import { useLocation } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  analysisTimeoutRange,
  ApiError,
  catalogLimitRange,
  catalogRefreshMinutesRange,
  channelLimitRange,
  customCodeMaxBytes,
  fetchSettings,
  fetchStatus,
  fetchVariables,
  inactiveDeviceDaysRange,
  liveTvRefreshHoursRange,
  loginAttemptsRange,
  loginDisclaimerMaxBytes,
  recordingPaddingMinutesRange,
  recordingRetentionDaysRange,
  playedPercentRange,
  queryClient,
  queryKeys,
  resumePercentRange,
  revealServerSecret,
  saveSettings,
  thumbnailStorageRange,
  trickplayIntervalRange,
  trickplayWidths,
  versionAttemptsRange,
  versionListMinutesRange,
  type Settings,
} from '@/api'
import CodeEditor from '@/components/CodeEditor'
import ConversionSettings from '@/components/ConversionSettings'
import { icons } from '@/components/icons'
import SegmentSources from '@/components/SegmentSources'
import { Skeleton } from '@/components/panels'
import {
  Badge,
  buttonPrimary,
  Checkbox,
  Loading,
  Notice,
  PageHeader,
  TextField,
} from '@/components/ui'
import {
  SearchContext,
  searchable,
  SettingsGroup,
  SecretField,
  Setting,
  wholeNumber,
  wholeNumberField,
} from '@/components/settings'
import { errorMessage } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'

/** The redirect URI of a Trakt or Simkl app whose users enter a code instead of being redirected. */
const oobRedirectUri = 'urn:ietf:wg:oauth:2.0:oob'

/** The sections of the page, in order; `variables` is read only, outside the form. */
const sectionIds = [
  'general',
  'playback',
  'conversion',
  'content',
  'catalogs',
  'thumbnails',
  'security',
  'tracking',
  'liveTv',
  'recordings',
  'diagnostics',
  'webPlayer',
  'variables',
] as const
type SectionId = (typeof sectionIds)[number]

/** A section of settings, hidden when the search leaves none of them. */
function Section({
  id,
  title,
  description,
  children,
}: {
  id: SectionId
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section
      id={`settings-${id}`}
      aria-labelledby={`settings-${id}-title`}
      className="scroll-mt-24 rounded-2xl border border-line bg-surface p-5 [&:not(:has([data-setting]:not([hidden])))]:hidden"
    >
      <h2 id={`settings-${id}-title`} className="text-base font-semibold text-white">
        {title}
      </h2>
      {description !== undefined && (
        <p className="mt-1 max-w-prose text-sm text-muted">{description}</p>
      )}
      <div className="mt-5 space-y-6">{children}</div>
    </section>
  )
}

export default function SettingsPage() {
  const { t } = useI18n()
  const text = t.dashboard.settings
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const [search, setSearch] = useState('')
  const query = searchable(search.trim())
  const searchId = useId()
  const content = useRef<HTMLDivElement>(null)
  const [noMatch, setNoMatch] = useState(false)

  // Whether the search left any setting is read from what was drawn.
  useLayoutEffect(() => {
    setNoMatch(
      query !== '' && content.current?.querySelector('[data-setting]:not([hidden])') === null,
    )
  }, [query, settings.data])

  // A link to a section of this page, such as the tracking apps, scrolls to it once it is drawn.
  const location = useLocation()
  const loaded = settings.data !== undefined
  useEffect(() => {
    if (loaded && location.hash) {
      document.getElementById(location.hash.slice(1))?.scrollIntoView()
    }
  }, [loaded, location.hash])

  return (
    <>
      <PageHeader title={t.settings.title} description={t.settings.description} />
      <div className="lg:grid lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-8 xl:grid-cols-[12rem_minmax(0,48rem)]">
        <nav aria-label={text.sectionsLabel} className="hidden lg:block">
          <ul className="sticky top-8 space-y-0.5 text-sm">
            {sectionIds.map((id) => (
              <li key={id}>
                <a
                  href={`#settings-${id}`}
                  className="block rounded-lg px-3 py-1.5 text-muted transition-colors hover:bg-surface hover:text-white"
                >
                  {text.sections[id]}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="min-w-0">
          <div className="relative mb-6">
            <label htmlFor={searchId} className="sr-only">
              {text.search}
            </label>
            <icons.search className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted" />
            <input
              id={searchId}
              type="search"
              value={search}
              placeholder={text.search}
              aria-describedby={`${searchId}-hint`}
              onChange={(event) => setSearch(event.target.value)}
              className="block min-h-11 w-full rounded-xl border border-line bg-surface py-2 pr-3 pl-10 text-white placeholder:text-zinc-500"
            />
            <p id={`${searchId}-hint`} className="sr-only">
              {text.searchHint}
            </p>
          </div>
          <nav
            aria-label={text.sectionsLabel}
            className="-mx-4 mb-6 overflow-x-auto px-4 lg:hidden"
          >
            <ul className="flex gap-1.5 text-sm whitespace-nowrap">
              {sectionIds.map((id) => (
                <li key={id}>
                  <a
                    href={`#settings-${id}`}
                    className="block rounded-lg border border-line px-3 py-1.5 text-muted transition-colors hover:border-fin-4 hover:text-white"
                  >
                    {text.sections[id]}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
          <SearchContext value={query}>
            <div ref={content} className="space-y-6">
              {noMatch && (
                <p
                  role="status"
                  className="rounded-xl border border-dashed border-line px-4 py-8 text-center text-sm text-muted"
                >
                  {text.noMatch}
                </p>
              )}
              {settings.isPending ? (
                <Loading />
              ) : settings.isError ? (
                <Notice kind="error">{errorMessage(t, settings.error)}</Notice>
              ) : (
                <SettingsForm initial={settings.data} />
              )}
              <Variables />
            </div>
          </SearchContext>
        </div>
      </div>
    </>
  )
}

function SettingsForm({ initial }: { initial: Settings }) {
  const { t } = useI18n()
  const s = t.settings
  const sections = t.dashboard.settings.sections
  const languageId = useId()
  const trickplayWidthId = useId()
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  const [saved, setSaved] = useState(initial)
  const [form, setForm] = useState(initial)
  const dirty = JSON.stringify(form) !== JSON.stringify(saved)

  const mutation = useMutation({
    mutationFn: saveSettings,
    onSuccess: (result) => {
      queryClient.setQueryData(queryKeys.settings, result)
      setSaved(result)
      setForm(result)
      // The server name is part of the public status.
      void queryClient.invalidateQueries({ queryKey: queryKeys.status })
      // Library names in apps follow the server language.
      void queryClient.invalidateQueries({ queryKey: queryKeys.scopes })
    },
  })

  function update(patch: Partial<Settings>) {
    mutation.reset()
    setForm((current) => ({ ...current, ...patch }))
  }

  /** The save error, also shown under the field it is about when its code is one of `codes`. */
  function fieldError(codes: readonly string[]): string | undefined {
    const error = mutation.error
    return error instanceof ApiError && codes.includes(error.code)
      ? errorMessage(t, error)
      : undefined
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({
      ...form,
      serverName: form.serverName.trim(),
      traktClientId: form.traktClientId.trim(),
      simklClientId: form.simklClientId.trim(),
      // The order POLYFIN_SEGMENTS gives is not saved, so that it keeps following the variable.
      segmentOrder:
        form.segmentOrder.join() === form.segmentOrderDefault.join() ? [] : form.segmentOrder,
    })
  }

  // Trakt and Simkl ask for a redirect URI when an app is created; a code connection uses none.
  const redirectUri = (
    <p className="mt-2 text-sm text-muted">
      {s.tracking.redirectUri}
      {t.common.colon}{' '}
      <code className="rounded bg-ink px-1.5 py-0.5 font-mono text-xs break-all text-zinc-100 select-all">
        {oobRedirectUri}
      </code>
    </p>
  )

  return (
    <form onSubmit={submit} noValidate className="space-y-6">
      <Section id="general" title={sections.general}>
        <Setting text={[s.serverName, s.serverNameHelp]}>
          <TextField
            label={s.serverName}
            hint={s.serverNameHelp}
            value={form.serverName}
            onValue={(serverName) => update({ serverName })}
            maxLength={64}
            required
          />
        </Setting>
        <Setting text={[s.language, s.languageHelp]}>
          <label htmlFor={languageId} className="block text-sm font-medium text-zinc-200">
            {s.language}
          </label>
          <select
            id={languageId}
            value={form.language}
            onChange={(event) => update({ language: event.target.value as Language })}
            aria-describedby={`${languageId}-hint`}
            className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white"
          >
            {languages.map((code) => (
              <option key={code} value={code} lang={code}>
                {t.language[code].name}
              </option>
            ))}
          </select>
          <p id={`${languageId}-hint`} className="mt-1 text-xs text-muted">
            {s.languageHelp}
          </p>
        </Setting>
        <Setting text={[s.quickConnect, s.quickConnectHelp]}>
          <Checkbox
            label={s.quickConnect}
            help={s.quickConnectHelp}
            checked={form.quickConnectEnabled}
            onChange={(quickConnectEnabled) => update({ quickConnectEnabled })}
          />
        </Setting>
        <Setting text={[s.legacyAuthorization, s.legacyAuthorizationHelp, s.legacyWarning]}>
          <div className="space-y-3">
            <Checkbox
              label={s.legacyAuthorization}
              help={s.legacyAuthorizationHelp}
              checked={form.legacyAuthorization}
              onChange={(legacyAuthorization) => update({ legacyAuthorization })}
            />
            <div className="rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-sm">
              <p className="flex items-center gap-2 font-semibold text-amber-200">
                <span aria-hidden="true">⚠</span>
                {s.legacyWarningTitle}
              </p>
              <p className="mt-1 text-amber-100">{s.legacyWarning}</p>
            </div>
          </div>
        </Setting>
      </Section>

      <Section id="playback" title={sections.playback}>
        <Setting text={[s.chapters, s.chaptersHelp]}>
          <Checkbox
            label={s.chapters}
            help={s.chaptersHelp}
            checked={form.chapters}
            onChange={(chapters) => update({ chapters })}
          />
        </Setting>
        <Setting text={[s.prepareAhead, s.prepareAheadHelp]}>
          <Checkbox
            label={s.prepareAhead}
            help={s.prepareAheadHelp}
            checked={form.prepareAhead}
            onChange={(prepareAhead) => update({ prepareAhead })}
          />
        </Setting>
        <Setting text={[s.downloads, s.downloadsHelp]}>
          <Checkbox
            label={s.downloads}
            help={s.downloadsHelp}
            checked={form.downloads}
            onChange={(downloads) => update({ downloads })}
          />
        </Setting>
        <Setting text={[s.preferDirectPlay, s.preferDirectPlayHelp]}>
          <Checkbox
            label={s.preferDirectPlay}
            help={s.preferDirectPlayHelp}
            checked={form.preferDirectPlay}
            onChange={(preferDirectPlay) => update({ preferDirectPlay })}
          />
        </Setting>
        <Setting text={[s.analysisTimeout, s.analysisTimeoutHelp]}>
          <TextField
            label={s.analysisTimeout}
            hint={s.analysisTimeoutHelp}
            type="number"
            inputMode="numeric"
            min={analysisTimeoutRange.min}
            max={analysisTimeoutRange.max}
            step={1}
            value={form.analysisTimeout || ''}
            onValue={(value) => update({ analysisTimeout: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.versionAttempts, s.versionAttemptsHelp]}>
          <TextField
            label={s.versionAttempts}
            hint={s.versionAttemptsHelp}
            type="number"
            inputMode="numeric"
            min={versionAttemptsRange.min}
            max={versionAttemptsRange.max}
            step={1}
            value={form.versionAttempts || ''}
            onValue={(value) => update({ versionAttempts: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
      </Section>

      <Section id="conversion" title={sections.conversion} description={s.conversion.description}>
        <ConversionSettings form={form} update={update} />
      </Section>

      <Section id="content" title={sections.content}>
        <Setting text={[s.skipButtons, s.skipButtonsHelp]}>
          <Checkbox
            label={s.skipButtons}
            help={s.skipButtonsHelp}
            checked={form.skipButtons}
            onChange={(skipButtons) => update({ skipButtons })}
          />
        </Setting>
        <Setting
          text={[
            s.segmentSources.label,
            s.segmentSources.help,
            'TheIntroDB',
            'IntroDB',
            'PublicMetaDB',
          ]}
        >
          <SegmentSources
            order={form.segmentOrder}
            defaultOrder={form.segmentOrderDefault}
            off={form.segmentSourcesOff}
            publicMetaDbKey={
              form.publicMetaDbKey === undefined
                ? form.publicMetaDbKeySet
                : form.publicMetaDbKey.trim() !== ''
            }
            onOrder={(segmentOrder) => update({ segmentOrder })}
          />
          {fieldError(['invalid_segment_order']) && (
            <p role="alert" className="mt-1 text-sm text-rose-300">
              {fieldError(['invalid_segment_order'])}
            </p>
          )}
        </Setting>
        <Setting text={[s.publicMetaDbKey, s.publicMetaDbKeyHelp, 'PublicMetaDB']}>
          <SecretField
            label={s.publicMetaDbKey}
            hint={s.publicMetaDbKeyHelp}
            saved={form.publicMetaDbKeySet}
            value={form.publicMetaDbKey}
            onValue={(publicMetaDbKey) => update({ publicMetaDbKey })}
            error={fieldError(['invalid_publicmetadb_key', 'publicmetadb_unreachable'])}
            reveal={() => revealServerSecret('publicMetaDbKey')}
          />
        </Setting>
        <Setting text={[s.theIntroDbKey, s.theIntroDbKeyHelp, 'TheIntroDB']}>
          <SecretField
            label={s.theIntroDbKey}
            hint={s.theIntroDbKeyHelp}
            saved={form.theIntroDbKeySet}
            value={form.theIntroDbKey}
            onValue={(theIntroDbKey) => update({ theIntroDbKey })}
            error={fieldError(['invalid_theintrodb_key', 'theintrodb_unreachable'])}
            reveal={() => revealServerSecret('theIntroDbKey')}
          />
        </Setting>
        <Setting text={[s.similarTitles, s.similarTitlesHelp]}>
          <Checkbox
            label={s.similarTitles}
            help={s.similarTitlesHelp}
            checked={form.similarTitles}
            onChange={(similarTitles) => update({ similarTitles })}
          />
        </Setting>
        <Setting text={[s.playedPercent, s.playedPercentHelp]}>
          <TextField
            label={s.playedPercent}
            hint={s.playedPercentHelp}
            type="number"
            inputMode="numeric"
            min={playedPercentRange.min}
            max={playedPercentRange.max}
            step={1}
            value={form.playedPercent || ''}
            onValue={(value) => update({ playedPercent: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.resumePercent, s.resumePercentHelp]}>
          <TextField
            label={s.resumePercent}
            hint={s.resumePercentHelp}
            type="number"
            inputMode="numeric"
            min={resumePercentRange.min}
            max={resumePercentRange.max}
            step={1}
            // 0 is a valid threshold: it is shown, not left blank.
            value={form.resumePercent}
            onValue={(value) => update({ resumePercent: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
      </Section>

      <Section id="catalogs" title={sections.catalogs}>
        <Setting text={[s.catalogLimit, s.catalogLimitHelp]}>
          <TextField
            label={s.catalogLimit}
            hint={s.catalogLimitHelp}
            type="number"
            inputMode="numeric"
            min={catalogLimitRange.min}
            max={catalogLimitRange.max}
            step={1}
            value={form.catalogLimit || ''}
            onValue={(value) => update({ catalogLimit: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.channelLimit, s.channelLimitHelp]}>
          <TextField
            label={s.channelLimit}
            hint={s.channelLimitHelp}
            type="number"
            inputMode="numeric"
            min={channelLimitRange.min}
            max={channelLimitRange.max}
            step={1}
            value={form.channelLimit || ''}
            onValue={(value) => update({ channelLimit: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.versionListMinutes, s.versionListMinutesHelp]}>
          <TextField
            label={s.versionListMinutes}
            hint={s.versionListMinutesHelp}
            type="number"
            inputMode="numeric"
            min={versionListMinutesRange.min}
            max={versionListMinutesRange.max}
            step={1}
            value={form.versionListMinutes || ''}
            onValue={(value) => update({ versionListMinutes: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.catalogRefreshMinutes, s.catalogRefreshMinutesHelp]}>
          <TextField
            label={s.catalogRefreshMinutes}
            hint={s.catalogRefreshMinutesHelp}
            type="number"
            inputMode="numeric"
            min={catalogRefreshMinutesRange.min}
            max={catalogRefreshMinutesRange.max}
            step={1}
            value={form.catalogRefreshMinutes || ''}
            onValue={(value) => update({ catalogRefreshMinutes: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
      </Section>

      <Section id="thumbnails" title={sections.thumbnails} description={s.thumbnailsHelp}>
        <Setting text={[s.trickplay, s.trickplayHelp, s.thumbnailsHelp]}>
          <Checkbox
            label={s.trickplay}
            help={s.trickplayHelp}
            checked={form.trickplay}
            onChange={(trickplay) => update({ trickplay })}
          />
        </Setting>
        <Setting text={[s.trickplayInterval, s.trickplayIntervalHelp]}>
          <TextField
            label={s.trickplayInterval}
            hint={s.trickplayIntervalHelp}
            type="number"
            inputMode="numeric"
            min={trickplayIntervalRange.min}
            max={trickplayIntervalRange.max}
            step={1}
            value={form.trickplayInterval || ''}
            onValue={(value) => update({ trickplayInterval: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.trickplayWidth, s.trickplayWidthHelp]}>
          <label htmlFor={trickplayWidthId} className="block text-sm font-medium text-zinc-200">
            {s.trickplayWidth}
          </label>
          <select
            id={trickplayWidthId}
            value={form.trickplayWidth}
            onChange={(event) => update({ trickplayWidth: Number(event.target.value) })}
            aria-describedby={`${trickplayWidthId}-hint`}
            className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white"
          >
            {trickplayWidths.map((width) => (
              <option key={width} value={width}>
                {s.pixels(width)}
              </option>
            ))}
          </select>
          <p id={`${trickplayWidthId}-hint`} className="mt-1 text-xs text-muted">
            {s.trickplayWidthHelp}
          </p>
        </Setting>
        <Setting text={[s.chapterImages, s.chapterImagesHelp]}>
          <Checkbox
            label={s.chapterImages}
            help={s.chapterImagesHelp}
            checked={form.chapterImages}
            onChange={(chapterImages) => update({ chapterImages })}
          />
        </Setting>
        <Setting text={[s.thumbnailStorage, s.thumbnailStorageHelp]}>
          <TextField
            label={s.thumbnailStorage}
            hint={s.thumbnailStorageHelp}
            type="number"
            inputMode="numeric"
            min={thumbnailStorageRange.min}
            max={thumbnailStorageRange.max}
            step={1}
            value={form.thumbnailStorageGB || ''}
            onValue={(value) => update({ thumbnailStorageGB: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
      </Section>

      <Section id="security" title={sections.security}>
        <Setting text={[s.personalAddons, s.personalAddonsHelp]}>
          <Checkbox
            label={s.personalAddons}
            help={s.personalAddonsHelp}
            checked={form.personalAddons}
            onChange={(personalAddons) => update({ personalAddons })}
          />
        </Setting>
        <Setting text={[s.loginAttempts, s.loginAttemptsHelp]}>
          <TextField
            label={s.loginAttempts}
            hint={s.loginAttemptsHelp}
            type="number"
            inputMode="numeric"
            min={0}
            max={loginAttemptsRange.max}
            step={1}
            value={wholeNumberField(form.loginAttempts)}
            onValue={(value) => update({ loginAttempts: wholeNumber(value) })}
            required
          />
        </Setting>
        <Setting text={[s.inactiveDeviceDays, s.inactiveDeviceDaysHelp]}>
          <TextField
            label={s.inactiveDeviceDays}
            hint={s.inactiveDeviceDaysHelp}
            type="number"
            inputMode="numeric"
            min={inactiveDeviceDaysRange.min}
            max={inactiveDeviceDaysRange.max}
            step={1}
            value={wholeNumberField(form.inactiveDeviceDays)}
            onValue={(value) => update({ inactiveDeviceDays: wholeNumber(value) })}
            required
          />
        </Setting>
      </Section>

      <Section id="tracking" title={sections.tracking} description={s.tracking.description}>
        <SettingsGroup title="Trakt">
          <Setting text={['Trakt', s.tracking.traktSetup, s.tracking.redirectUri]}>
            <p className="text-sm text-muted">{s.tracking.traktSetup}</p>
            {redirectUri}
          </Setting>
          <Setting text={['Trakt', s.tracking.traktClientId, s.tracking.traktClientIdHelp]}>
            <TextField
              label={s.tracking.traktClientId}
              hint={s.tracking.traktClientIdHelp}
              value={form.traktClientId}
              onValue={(traktClientId) => update({ traktClientId })}
              autoComplete="off"
              spellCheck={false}
              error={fieldError(['invalid_trakt_app'])}
            />
          </Setting>
          <Setting text={['Trakt', s.tracking.traktClientSecret, s.tracking.traktClientSecretHelp]}>
            <SecretField
              label={s.tracking.traktClientSecret}
              hint={s.tracking.traktClientSecretHelp}
              saved={form.traktClientSecretSet}
              value={form.traktClientSecret}
              onValue={(traktClientSecret) => update({ traktClientSecret })}
              reveal={() => revealServerSecret('traktClientSecret')}
            />
          </Setting>
        </SettingsGroup>
        <SettingsGroup title="Simkl">
          <Setting text={['Simkl', s.tracking.simklSetup]}>
            <p className="text-sm text-muted">{s.tracking.simklSetup}</p>
          </Setting>
          <Setting text={['Simkl', s.tracking.simklClientId, s.tracking.simklClientIdHelp]}>
            <TextField
              label={s.tracking.simklClientId}
              hint={s.tracking.simklClientIdHelp}
              value={form.simklClientId}
              onValue={(simklClientId) => update({ simklClientId })}
              autoComplete="off"
              spellCheck={false}
              error={fieldError(['invalid_simkl_app'])}
            />
          </Setting>
        </SettingsGroup>
      </Section>

      <Section id="liveTv" title={sections.liveTv}>
        <Setting text={[s.liveTvRefreshHours, s.liveTvRefreshHoursHelp]}>
          <TextField
            label={s.liveTvRefreshHours}
            hint={s.liveTvRefreshHoursHelp}
            type="number"
            inputMode="numeric"
            min={liveTvRefreshHoursRange.min}
            max={liveTvRefreshHoursRange.max}
            step={1}
            value={wholeNumberField(form.liveTvRefreshHours)}
            onValue={(value) => update({ liveTvRefreshHours: wholeNumber(value) })}
            required
          />
        </Setting>
      </Section>

      <Section
        id="recordings"
        title={sections.recordings}
        description={
          form.recordingsFolder ? s.recordingsFolder(form.recordingsFolder) : s.recordingsOff
        }
      >
        <Setting text={[s.recordingPrePadding, s.recordingPrePaddingHelp]}>
          <TextField
            label={s.recordingPrePadding}
            hint={s.recordingPrePaddingHelp}
            type="number"
            inputMode="numeric"
            min={recordingPaddingMinutesRange.min}
            max={recordingPaddingMinutesRange.max}
            step={1}
            value={wholeNumberField(Math.round(form.recordingPrePadding / 60))}
            onValue={(value) => update({ recordingPrePadding: wholeNumber(value) * 60 })}
            required
          />
        </Setting>
        <Setting text={[s.recordingPostPadding, s.recordingPostPaddingHelp]}>
          <TextField
            label={s.recordingPostPadding}
            hint={s.recordingPostPaddingHelp}
            type="number"
            inputMode="numeric"
            min={recordingPaddingMinutesRange.min}
            max={recordingPaddingMinutesRange.max}
            step={1}
            value={wholeNumberField(Math.round(form.recordingPostPadding / 60))}
            onValue={(value) => update({ recordingPostPadding: wholeNumber(value) * 60 })}
            required
          />
        </Setting>
        <Setting text={[s.recordingRetentionDays, s.recordingRetentionDaysHelp]}>
          <TextField
            label={s.recordingRetentionDays}
            hint={s.recordingRetentionDaysHelp}
            type="number"
            inputMode="numeric"
            min={recordingRetentionDaysRange.min}
            max={recordingRetentionDaysRange.max}
            step={1}
            value={wholeNumberField(form.recordingRetentionDays)}
            onValue={(value) => update({ recordingRetentionDays: wholeNumber(value) })}
            required
          />
        </Setting>
      </Section>

      <Section id="diagnostics" title={sections.diagnostics}>
        <Setting text={[s.detailedLog, s.detailedLogHelp]}>
          <Checkbox
            label={s.detailedLog}
            help={s.detailedLogHelp}
            checked={form.detailedLog}
            onChange={(detailedLog) => update({ detailedLog })}
          />
        </Setting>
      </Section>

      <Section id="webPlayer" title={sections.webPlayer} description={s.webPlayerHelp}>
        {status.data?.webClient === true && (
          <Setting text={[s.openWebPlayer, sections.webPlayer]}>
            <a
              href="/web/"
              target="_blank"
              rel="noopener"
              className="inline-flex items-center gap-2 text-sm font-medium text-fin-5 underline-offset-4 hover:underline"
            >
              <icons.play className="size-4" />
              {s.openWebPlayer}
            </a>
          </Setting>
        )}
        <Setting text={[s.customCss, s.customCssHelp, 'css']}>
          <CodeEditor
            label={s.customCss}
            hint={`${s.customCssHelp} ${s.codeKeys}`}
            value={form.customCss}
            onValue={(customCss) => update({ customCss })}
            maxBytes={customCodeMaxBytes}
          />
        </Setting>
        <Setting text={[s.customJs, s.customJsHelp, s.customJsWarning, 'javascript js']}>
          <CodeEditor
            label={s.customJs}
            hint={`${s.customJsHelp} ${s.codeKeys}`}
            value={form.customJs}
            onValue={(customJs) => update({ customJs })}
            maxBytes={customCodeMaxBytes}
          >
            <div
              role="note"
              className="mt-1.5 rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-sm"
            >
              <p className="flex items-center gap-2 font-semibold text-amber-200">
                <span aria-hidden="true">⚠</span>
                {s.customJsWarningTitle}
              </p>
              <p className="mt-1 text-amber-100">{s.customJsWarning}</p>
            </div>
          </CodeEditor>
        </Setting>
        <Setting text={[s.loginDisclaimer, s.loginDisclaimerHelp]}>
          <CodeEditor
            label={s.loginDisclaimer}
            hint={s.loginDisclaimerHelp}
            value={form.loginDisclaimer}
            onValue={(loginDisclaimer) => update({ loginDisclaimer })}
            maxBytes={loginDisclaimerMaxBytes}
            rows={4}
          />
        </Setting>
      </Section>

      <div className="sticky bottom-0 z-10 -mx-1 rounded-t-2xl border border-b-0 border-line bg-ink/95 px-4 py-3 backdrop-blur">
        <div className="space-y-3">
          {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
          {mutation.isSuccess && !dirty && <Notice kind="success">{s.saved}</Notice>}
          <div className="flex items-center justify-between gap-3">
            <p aria-live="polite" className={`text-sm ${dirty ? 'text-amber-200' : 'text-muted'}`}>
              {dirty ? t.dashboard.settings.unsaved : t.dashboard.settings.upToDate}
            </p>
            <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
              {mutation.isPending ? t.common.saving : t.common.save}
            </button>
          </div>
        </div>
      </div>
    </form>
  )
}

/** The POLYFIN_ environment variables in effect, read only. */
function Variables() {
  const { t } = useI18n()
  const text = t.dashboard.settings
  const variables = useQuery({
    queryKey: queryKeys.variables,
    queryFn: ({ signal }) => fetchVariables(signal),
    staleTime: Infinity,
  })
  return (
    <Section id="variables" title={text.sections.variables} description={text.variablesHelp}>
      {variables.isPending ? (
        <Setting text={[text.sections.variables]}>
          <Skeleton rows={3} label={t.common.loading} />
        </Setting>
      ) : variables.isError ? (
        <Setting text={[text.sections.variables]}>
          <Notice kind="error">{errorMessage(t, variables.error)}</Notice>
        </Setting>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line [&>li:has(>[hidden])]:hidden">
          {variables.data.map((variable) => (
            <li key={variable.name}>
              <Setting text={[variable.name, variable.value, text.sections.variables]}>
                <div className="flex flex-col gap-1 px-4 py-2.5 sm:flex-row sm:items-baseline sm:justify-between sm:gap-6">
                  <code className="font-mono text-sm break-all text-zinc-100">{variable.name}</code>
                  <span className="flex flex-wrap items-center gap-2 sm:justify-end">
                    {variable.hidden ? (
                      <Badge tone="muted">{text.hidden}</Badge>
                    ) : (
                      <code className="font-mono text-sm break-all text-fin-5">
                        {variable.value === '' ? (
                          <span className="font-sans text-muted">{text.empty}</span>
                        ) : (
                          variable.value
                        )}
                      </code>
                    )}
                    <Badge tone={variable.set ? 'fin' : 'muted'}>
                      {variable.set ? text.setValue : text.defaultValue}
                    </Badge>
                    {!variable.known && <Badge tone="warning">{text.notRead}</Badge>}
                  </span>
                </div>
              </Setting>
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}
