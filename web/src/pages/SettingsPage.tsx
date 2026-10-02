import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { fetchSettings, queryClient, queryKeys, saveSettings, type Settings } from '@/api'
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
import { useI18n } from '@/i18n'

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
  const [form, setForm] = useState(initial)

  const mutation = useMutation({
    mutationFn: saveSettings,
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.settings, saved)
      setForm(saved)
      // The server name is part of the public status.
      void queryClient.invalidateQueries({ queryKey: queryKeys.status })
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
