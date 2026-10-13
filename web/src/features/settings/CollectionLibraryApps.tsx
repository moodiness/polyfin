import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import { fetchSettings, fetchSettingsApps, queryKeys } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Checkbox, FieldError } from '@/ui'
import { SettingRow } from './parts'

/** Whether two app names are the same whatever their letter case, as the server compares them. */
const sameApp = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()

/**
 * The apps told an addon's collection library holds collections: a checkbox for each app users
 * are signed in with, and for each app saved, which stays listed when no device uses it now,
 * sorted by name whatever the case.
 */
export default function CollectionLibraryApps({
  chosen,
  onChange,
  error,
}: {
  chosen: readonly string[]
  onChange: (apps: string[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const s = t.settings
  const id = useId()
  const signedIn = useQuery({
    queryKey: queryKeys.settingsApps,
    queryFn: ({ signal }) => fetchSettingsApps(signal),
  })
  const saved = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const apps: string[] = []
  for (const app of [
    ...chosen,
    ...(saved.data?.collectionLibraryApps ?? []),
    ...(signedIn.data ?? []),
  ]) {
    if (!apps.some((listed) => sameApp(listed, app))) apps.push(app)
  }
  apps.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()))
  return (
    <SettingRow anchor="collection-library-apps">
      <fieldset aria-describedby={`${id}-help`}>
        <legend className="text-[15px] font-medium tracking-[-0.01em] text-ink">
          {s.collectionLibraryApps}
        </legend>
        <p id={`${id}-help`} className="mt-1 max-w-[60ch] text-small text-ink-3">
          {s.collectionLibraryAppsHelp}
        </p>
        {apps.length > 0 ? (
          <div className="mt-3.5 grid gap-3 sm:grid-cols-2">
            {apps.map((app) => (
              <Checkbox
                key={app}
                label={app}
                checked={chosen.some((other) => sameApp(other, app))}
                onChange={(checked) =>
                  onChange(
                    checked ? [...chosen, app] : chosen.filter((other) => !sameApp(other, app)),
                  )
                }
              />
            ))}
          </div>
        ) : (
          signedIn.isSuccess && (
            <p className="mt-3.5 text-small text-ink-3">{s.collectionLibraryAppsNone}</p>
          )
        )}
        {(error ?? signedIn.isError) && (
          <div className="mt-2">
            <FieldError>{error ?? errorMessage(t, signedIn.error)}</FieldError>
          </div>
        )}
      </fieldset>
    </SettingRow>
  )
}
