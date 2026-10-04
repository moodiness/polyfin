import { useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  analysisTimeoutRange,
  catalogLimitRange,
  catalogRefreshMinutesRange,
  channelLimitRange,
  conversionHeights,
  fetchSettings,
  inactiveDeviceDaysRange,
  loginAttemptsRange,
  maxConversionsRange,
  playedPercentRange,
  queryClient,
  queryKeys,
  resumePercentRange,
  saveSettings,
  thumbnailStorageRange,
  trickplayIntervalRange,
  trickplayWidths,
  versionAttemptsRange,
  versionListMinutesRange,
  type Settings,
} from '@/api'
import {
  buttonPrimary,
  Card,
  Checkbox,
  Loading,
  Notice,
  PageHeader,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'

export default function SettingsPage() {
  const { t } = useI18n()
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })

  return (
    <>
      <PageHeader title={t.settings.title} description={t.settings.description} />
      {settings.isPending ? (
        <Loading />
      ) : settings.isError ? (
        <Notice kind="error">{errorMessage(t, settings.error)}</Notice>
      ) : (
        <SettingsForm initial={settings.data} />
      )}
    </>
  )
}

function SettingsForm({ initial }: { initial: Settings }) {
  const { t } = useI18n()
  const languageId = useId()
  const playbackId = useId()
  const catalogsId = useId()
  const contentId = useId()
  const securityId = useId()
  const heightId = useId()
  const thumbnailsId = useId()
  const trickplayWidthId = useId()
  const [form, setForm] = useState(initial)

  const mutation = useMutation({
    mutationFn: saveSettings,
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.settings, saved)
      setForm(saved)
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

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ ...form, serverName: form.serverName.trim() })
  }

  return (
    <form onSubmit={submit} noValidate className="max-w-2xl">
      <Card>
        <div className="space-y-6">
          <TextField
            label={t.settings.serverName}
            hint={t.settings.serverNameHelp}
            value={form.serverName}
            onValue={(serverName) => update({ serverName })}
            maxLength={64}
            required
          />
          <div>
            <label htmlFor={languageId} className="block text-sm font-medium text-zinc-200">
              {t.settings.language}
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
              {t.settings.languageHelp}
            </p>
          </div>
          <Checkbox
            label={t.settings.quickConnect}
            help={t.settings.quickConnectHelp}
            checked={form.quickConnectEnabled}
            onChange={(quickConnectEnabled) => update({ quickConnectEnabled })}
          />
          <div className="space-y-3">
            <Checkbox
              label={t.settings.legacyAuthorization}
              help={t.settings.legacyAuthorizationHelp}
              checked={form.legacyAuthorization}
              onChange={(legacyAuthorization) => update({ legacyAuthorization })}
            />
            <div className="rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-sm">
              <p className="flex items-center gap-2 font-semibold text-amber-200">
                <span aria-hidden="true">⚠</span>
                {t.settings.legacyWarningTitle}
              </p>
              <p className="mt-1 text-amber-100">{t.settings.legacyWarning}</p>
            </div>
          </div>
          <section className="space-y-3 border-t border-line pt-6" aria-labelledby={playbackId}>
            <h2 id={playbackId} className="text-sm font-semibold text-white">
              {t.settings.playbackTitle}
            </h2>
            <Checkbox
              label={t.settings.chapters}
              help={t.settings.chaptersHelp}
              checked={form.chapters}
              onChange={(chapters) => update({ chapters })}
            />
            <Checkbox
              label={t.settings.prepareAhead}
              help={t.settings.prepareAheadHelp}
              checked={form.prepareAhead}
              onChange={(prepareAhead) => update({ prepareAhead })}
            />
            <Checkbox
              label={t.settings.transcoding}
              help={t.settings.transcodingHelp}
              checked={form.transcoding}
              onChange={(transcoding) => update({ transcoding })}
            />
            <Checkbox
              label={t.settings.downloads}
              help={t.settings.downloadsHelp}
              checked={form.downloads}
              onChange={(downloads) => update({ downloads })}
            />
            <TextField
              label={t.settings.analysisTimeout}
              hint={t.settings.analysisTimeoutHelp}
              type="number"
              inputMode="numeric"
              min={analysisTimeoutRange.min}
              max={analysisTimeoutRange.max}
              step={1}
              value={form.analysisTimeout || ''}
              onValue={(value) => update({ analysisTimeout: Math.trunc(Number(value)) })}
              required
            />
            <TextField
              label={t.settings.versionAttempts}
              hint={t.settings.versionAttemptsHelp}
              type="number"
              inputMode="numeric"
              min={versionAttemptsRange.min}
              max={versionAttemptsRange.max}
              step={1}
              value={form.versionAttempts || ''}
              onValue={(value) => update({ versionAttempts: Math.trunc(Number(value)) })}
              required
            />
            <Checkbox
              label={t.settings.preferDirectPlay}
              help={t.settings.preferDirectPlayHelp}
              checked={form.preferDirectPlay}
              onChange={(preferDirectPlay) => update({ preferDirectPlay })}
            />
            <TextField
              label={t.settings.maxConversions}
              hint={t.settings.maxConversionsHelp}
              type="number"
              inputMode="numeric"
              min={maxConversionsRange.min}
              max={maxConversionsRange.max}
              step={1}
              value={form.maxConversions}
              onValue={(value) => update({ maxConversions: Math.trunc(Number(value)) })}
              required
            />
            <div>
              <label htmlFor={heightId} className="block text-sm font-medium text-zinc-200">
                {t.settings.maxConversionHeight}
              </label>
              <select
                id={heightId}
                value={form.maxConversionHeight}
                onChange={(event) => update({ maxConversionHeight: Number(event.target.value) })}
                aria-describedby={`${heightId}-hint`}
                className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white"
              >
                {conversionHeights.map((height) => (
                  <option key={height} value={height}>
                    {height === 0
                      ? t.settings.conversionHeightOriginal
                      : t.settings.conversionHeight(height)}
                  </option>
                ))}
              </select>
              <p id={`${heightId}-hint`} className="mt-1 text-xs text-muted">
                {t.settings.maxConversionHeightHelp}
              </p>
            </div>
          </section>
          <section className="space-y-4 border-t border-line pt-6" aria-labelledby={contentId}>
            <h2 id={contentId} className="text-sm font-semibold text-white">
              {t.settings.contentTitle}
            </h2>
            <Checkbox
              label={t.settings.skipButtons}
              help={t.settings.skipButtonsHelp}
              checked={form.skipButtons}
              onChange={(skipButtons) => update({ skipButtons })}
            />
            <Checkbox
              label={t.settings.similarTitles}
              help={t.settings.similarTitlesHelp}
              checked={form.similarTitles}
              onChange={(similarTitles) => update({ similarTitles })}
            />
            <TextField
              label={t.settings.playedPercent}
              hint={t.settings.playedPercentHelp}
              type="number"
              inputMode="numeric"
              min={playedPercentRange.min}
              max={playedPercentRange.max}
              step={1}
              value={form.playedPercent || ''}
              onValue={(value) => update({ playedPercent: Math.trunc(Number(value)) })}
              required
            />
            <TextField
              label={t.settings.resumePercent}
              hint={t.settings.resumePercentHelp}
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
          </section>
          <section className="space-y-4 border-t border-line pt-6" aria-labelledby={catalogsId}>
            <h2 id={catalogsId} className="text-sm font-semibold text-white">
              {t.settings.catalogsTitle}
            </h2>
            <TextField
              label={t.settings.catalogLimit}
              hint={t.settings.catalogLimitHelp}
              type="number"
              inputMode="numeric"
              min={catalogLimitRange.min}
              max={catalogLimitRange.max}
              step={1}
              value={form.catalogLimit || ''}
              onValue={(value) => update({ catalogLimit: Math.trunc(Number(value)) })}
              required
            />
            <TextField
              label={t.settings.channelLimit}
              hint={t.settings.channelLimitHelp}
              type="number"
              inputMode="numeric"
              min={channelLimitRange.min}
              max={channelLimitRange.max}
              step={1}
              value={form.channelLimit || ''}
              onValue={(value) => update({ channelLimit: Math.trunc(Number(value)) })}
              required
            />
            <TextField
              label={t.settings.versionListMinutes}
              hint={t.settings.versionListMinutesHelp}
              type="number"
              inputMode="numeric"
              min={versionListMinutesRange.min}
              max={versionListMinutesRange.max}
              step={1}
              value={form.versionListMinutes || ''}
              onValue={(value) => update({ versionListMinutes: Math.trunc(Number(value)) })}
              required
            />
            <TextField
              label={t.settings.catalogRefreshMinutes}
              hint={t.settings.catalogRefreshMinutesHelp}
              type="number"
              inputMode="numeric"
              min={catalogRefreshMinutesRange.min}
              max={catalogRefreshMinutesRange.max}
              step={1}
              value={form.catalogRefreshMinutes || ''}
              onValue={(value) => update({ catalogRefreshMinutes: Math.trunc(Number(value)) })}
              required
            />
          </section>
          <section className="space-y-4 border-t border-line pt-6" aria-labelledby={thumbnailsId}>
            <h2 id={thumbnailsId} className="text-sm font-semibold text-white">
              {t.settings.thumbnailsTitle}
            </h2>
            <p className="text-sm text-muted">{t.settings.thumbnailsHelp}</p>
            <Checkbox
              label={t.settings.trickplay}
              help={t.settings.trickplayHelp}
              checked={form.trickplay}
              onChange={(trickplay) => update({ trickplay })}
            />
            <TextField
              label={t.settings.trickplayInterval}
              hint={t.settings.trickplayIntervalHelp}
              type="number"
              inputMode="numeric"
              min={trickplayIntervalRange.min}
              max={trickplayIntervalRange.max}
              step={1}
              value={form.trickplayInterval || ''}
              onValue={(value) => update({ trickplayInterval: Math.trunc(Number(value)) })}
              required
            />
            <div>
              <label htmlFor={trickplayWidthId} className="block text-sm font-medium text-zinc-200">
                {t.settings.trickplayWidth}
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
                    {t.settings.pixels(width)}
                  </option>
                ))}
              </select>
              <p id={`${trickplayWidthId}-hint`} className="mt-1 text-xs text-muted">
                {t.settings.trickplayWidthHelp}
              </p>
            </div>
            <Checkbox
              label={t.settings.chapterImages}
              help={t.settings.chapterImagesHelp}
              checked={form.chapterImages}
              onChange={(chapterImages) => update({ chapterImages })}
            />
            <TextField
              label={t.settings.thumbnailStorage}
              hint={t.settings.thumbnailStorageHelp}
              type="number"
              inputMode="numeric"
              min={thumbnailStorageRange.min}
              max={thumbnailStorageRange.max}
              step={1}
              value={form.thumbnailStorageGB || ''}
              onValue={(value) => update({ thumbnailStorageGB: Math.trunc(Number(value)) })}
              required
            />
          </section>
          <section className="space-y-4 border-t border-line pt-6" aria-labelledby={securityId}>
            <h2 id={securityId} className="text-sm font-semibold text-white">
              {t.settings.securityTitle}
            </h2>
            <Checkbox
              label={t.settings.personalAddons}
              help={t.settings.personalAddonsHelp}
              checked={form.personalAddons}
              onChange={(personalAddons) => update({ personalAddons })}
            />
            <TextField
              label={t.settings.loginAttempts}
              hint={t.settings.loginAttemptsHelp}
              type="number"
              inputMode="numeric"
              min={0}
              max={loginAttemptsRange.max}
              step={1}
              value={wholeNumberField(form.loginAttempts)}
              onValue={(value) => update({ loginAttempts: wholeNumber(value) })}
              required
            />
            <TextField
              label={t.settings.inactiveDeviceDays}
              hint={t.settings.inactiveDeviceDaysHelp}
              type="number"
              inputMode="numeric"
              min={inactiveDeviceDaysRange.min}
              max={inactiveDeviceDaysRange.max}
              step={1}
              value={wholeNumberField(form.inactiveDeviceDays)}
              onValue={(value) => update({ inactiveDeviceDays: wholeNumber(value) })}
              required
            />
            <Checkbox
              label={t.settings.detailedLog}
              help={t.settings.detailedLogHelp}
              checked={form.detailedLog}
              onChange={(detailedLog) => update({ detailedLog })}
            />
          </section>
          {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
          {mutation.isSuccess && <Notice kind="success">{t.settings.saved}</Notice>}
          <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
            {mutation.isPending ? t.common.saving : t.common.save}
          </button>
        </div>
      </Card>
    </form>
  )
}

/**
 * A whole number typed in a field where 0 is a valid value: an empty field is -1, which the server
 * refuses, rather than 0, which would turn the setting off without the user typing it.
 */
function wholeNumber(value: string): number {
  return value.trim() === '' ? -1 : Math.trunc(Number(value))
}

/** The text of a field holding a whole number; -1 (see wholeNumber) shows as empty. */
function wholeNumberField(value: number): number | '' {
  return value < 0 ? '' : value
}
