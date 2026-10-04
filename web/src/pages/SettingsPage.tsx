import { useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  catalogLimitRange,
  channelLimitRange,
  fetchSettings,
  queryClient,
  queryKeys,
  saveSettings,
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
