import { useQuery } from '@tanstack/react-query'
import { fetchBackup, fetchVariables, queryKeys } from '@/api'
import BackupStatus from '@/features/system/BackupStatus'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Badge, EmptyState, InlineError, SkeletonRows, TextLink } from '@/ui'
import { variableAnchor } from './catalog'
import { PlainRow, SettingsGroup } from './parts'

/** How the last backup went, refreshed every 10 seconds. */
export function LastBackup() {
  const { t } = useI18n()
  const backup = useQuery({
    queryKey: queryKeys.backup,
    queryFn: ({ signal }) => fetchBackup(signal),
    refetchInterval: 10_000,
  })
  return (
    <PlainRow anchor="last-backup">
      <h4 className="mb-3 text-h3 text-ink">{t.settings.lastBackup}</h4>
      {backup.isPending ? (
        <SkeletonRows rows={1} label={t.common.loading} />
      ) : backup.isError ? (
        <InlineError onRetry={() => void backup.refetch()} retrying={backup.isFetching}>
          {errorMessage(t, backup.error)}
        </InlineError>
      ) : (
        <BackupStatus backup={backup.data} />
      )}
      <p className="mt-3 text-small">
        <TextLink to="/system/schedule" arrow>
          {t.system.health.backup.runHint}
        </TextLink>
      </p>
    </PlainRow>
  )
}

/** The POLYFIN_ environment variables in effect, read only. */
export function Variables() {
  const { t } = useI18n()
  const text = t.settingsPage
  const variables = useQuery({
    queryKey: queryKeys.variables,
    queryFn: ({ signal }) => fetchVariables(signal),
    staleTime: Infinity,
  })
  return (
    <SettingsGroup title={text.sections.variables}>
      <PlainRow anchor="environment-variables">
        <p className="max-w-[60ch] text-small text-ink-2">{text.variablesHelp}</p>
        <div className="mt-4">
          {variables.isPending ? (
            <SkeletonRows rows={5} label={t.common.loading} boxed />
          ) : variables.isError ? (
            <InlineError onRetry={() => void variables.refetch()} retrying={variables.isFetching}>
              {errorMessage(t, variables.error)}
            </InlineError>
          ) : variables.data.length === 0 ? (
            <EmptyState title={text.variablesEmpty}>{text.variablesEmptyHelp}</EmptyState>
          ) : (
            <ul aria-label={text.sections.variables} className="grid gap-2">
              {variables.data.map((variable) => (
                // A step over its row, s1 on s1: on s2, the accent badge falls under 4.5 to 1.
                <li
                  key={variable.name}
                  id={variableAnchor(variable.name)}
                  className="flex scroll-mt-32 flex-col gap-1.5 rounded-row bg-s1 px-4 py-3 transition-colors duration-700 data-[flash]:bg-accent/8 sm:flex-row sm:items-baseline sm:justify-between sm:gap-6"
                >
                  <code className="font-mono text-small break-all text-ink">{variable.name}</code>
                  <span className="flex flex-wrap items-center gap-2 sm:justify-end">
                    {variable.hidden ? (
                      <Badge>{text.hidden}</Badge>
                    ) : variable.value === '' ? (
                      <span className="text-small text-ink-3">{text.empty}</span>
                    ) : (
                      <code className="font-mono text-small break-all text-link">
                        {variable.value}
                      </code>
                    )}
                    <Badge tone={variable.set ? 'accent' : 'neutral'}>
                      {variable.set ? text.setValue : text.defaultValue}
                    </Badge>
                    {!variable.known && <Badge tone="warn">{text.notRead}</Badge>}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </PlainRow>
    </SettingsGroup>
  )
}
