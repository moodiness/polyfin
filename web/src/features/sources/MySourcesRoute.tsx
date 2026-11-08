import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, saveAddonPreferences } from '@/api'
import LibraryEditor from '@/features/libraries/LibraryEditor'
import SmartLists from '@/features/libraries/SmartLists'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { InlineError, Notice, SettingRow, SkeletonText, useToast } from '@/ui'
import { useOwnSources } from './ownSources'
import { SourcesPage } from './SourcesPage'

/**
 * `/me/sources`, for every signed-in user: whether they use the server's sources, their own
 * sources (the same list, page and add flows as the server's, in the `me` scope), which of their
 * catalogs become libraries, and their smart lists. While their own sources are not used, a notice
 * says why, nothing can be added, and their kept smart lists can only be deleted.
 */
export default function MySourcesRoute() {
  const { preferences, used } = useOwnSources()
  let after
  // Whether the libraries and lists can be changed is known with the preferences.
  if (preferences.isPending) after = null
  else if (used === false) after = <SmartLists scope="me" kept />
  else
    after = (
      <>
        <LibraryEditor scope="me" />
        <SmartLists scope="me" />
      </>
    )
  return <SourcesPage area="mine" before={<SharedSourcesSwitch />} after={after} />
}

/**
 * Whether the user's apps get the server's sources before their own, or only their own; in its
 * place, why their own sources and smart lists are not used.
 */
function SharedSourcesSwitch() {
  const { t } = useI18n()
  const toast = useToast()
  const { preferences } = useOwnSources()
  const mutation = useMutation({
    mutationFn: saveAddonPreferences,
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.addonPreferences, saved)
      toast(t.mySources.preferenceSaved)
    },
  })

  if (preferences.isPending) return <SkeletonText lines={2} />
  if (preferences.isError)
    return (
      <InlineError onRetry={() => void preferences.refetch()} retrying={preferences.isRefetching}>
        {errorMessage(t, preferences.error)}
      </InlineError>
    )
  // The server's sources give the ratings parental control hides titles by.
  if (preferences.data.parentalControl) return <Notice>{t.mySources.parentalControl}</Notice>
  // The server or the user's own permission turned their own sources off.
  if (!preferences.data.personalAddons)
    return <Notice tone="warn">{t.mySources.personalAddonsOff}</Notice>
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
      <SettingRow
        kind="switch"
        label={t.mySources.useShared}
        help={t.mySources.useSharedHelp}
        checked={
          mutation.isPending ? mutation.variables.useSharedAddons : preferences.data.useSharedAddons
        }
        disabled={mutation.isPending}
        error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
        onChange={(useSharedAddons) => mutation.mutate({ useSharedAddons })}
      />
    </div>
  )
}
