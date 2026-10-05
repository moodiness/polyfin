import { PlusIcon } from '@phosphor-icons/react'
import { useId, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchAddonPreferences,
  fetchAddons,
  queryClient,
  queryKeys,
  saveAddonPreferences,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import LibraryEditor from '@/features/libraries/LibraryEditor'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  FieldError,
  InlineError,
  Notice,
  Panel,
  SkeletonText,
  Switch,
  useToast,
} from '@/ui'
import { AddSourceModal } from './AddSource'
import type { Entry } from './model'
import { SourceBrowser } from './SourceBrowser'

/**
 * `/me/sources`, for every signed-in user: whether they use the server's sources, their own
 * sources (the same list, detail and add flows as the server's, in the `me` scope), and which of
 * their catalogs become libraries.
 */
export default function MySourcesRoute() {
  const { t } = useI18n()
  const user = useSessionUser()
  const [adding, setAdding] = useState(false)
  const [, setParams] = useSearchParams()
  const mine = useQuery({
    queryKey: queryKeys.addons('me'),
    queryFn: ({ signal }) => fetchAddons('me', signal),
  })
  const entries: Entry[] = (mine.data ?? []).map((addon) => ({
    key: `me:${addon.id}`,
    addon,
    owner: null,
    scope: 'me',
  }))

  return (
    <PageLayout
      title={t.nav.mySources}
      lede={t.mySources.lede}
      actions={
        <Button variant="primary" icon={PlusIcon} onClick={() => setAdding(true)}>
          {t.sources.add}
        </Button>
      }
    >
      <SharedSourcesSwitch />
      <Block title={t.mySources.listTitle}>
        <SourceBrowser
          entries={entries}
          queries={[mine]}
          selfId={user.id}
          emptyText={t.sources.emptyMine}
          ownerFilter={false}
          onAdd={() => setAdding(true)}
        />
      </Block>
      <LibraryEditor scope="me" />
      <AddSourceModal
        scope="me"
        open={adding}
        onClose={() => setAdding(false)}
        onAdded={(added) => setParams({ source: `me:${added.id}` }, { replace: true })}
      />
    </PageLayout>
  )
}

/** Whether the user's apps get the server's sources before their own, or only their own. */
function SharedSourcesSwitch() {
  const { t } = useI18n()
  const toast = useToast()
  const id = useId()
  const preferences = useQuery({
    queryKey: queryKeys.addonPreferences,
    queryFn: ({ signal }) => fetchAddonPreferences(signal),
  })
  const mutation = useMutation({
    mutationFn: saveAddonPreferences,
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.addonPreferences, saved)
      toast(t.mySources.preferenceSaved)
    },
  })

  let body
  if (preferences.isPending) body = <SkeletonText lines={2} />
  else if (preferences.isError)
    body = (
      <InlineError onRetry={() => void preferences.refetch()} retrying={preferences.isRefetching}>
        {errorMessage(t, preferences.error)}
      </InlineError>
    )
  // The server's sources give the ratings parental control hides titles by.
  else if (preferences.data.parentalControl) body = <Notice>{t.mySources.parentalControl}</Notice>
  // The server or the user's own permission turned their own sources off.
  else if (!preferences.data.personalAddons)
    body = <Notice tone="warn">{t.mySources.personalAddonsOff}</Notice>
  else
    body = (
      <div className="flex flex-col gap-3">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p id={`${id}-label`} className="text-control font-medium text-ink">
              {t.mySources.useShared}
            </p>
            <p id={`${id}-help`} className="mt-1 max-w-[70ch] text-small text-ink-3">
              {t.mySources.useSharedHelp}
            </p>
          </div>
          <Switch
            checked={preferences.data.useSharedAddons}
            labelledBy={`${id}-label`}
            describedById={`${id}-help`}
            disabled={mutation.isPending}
            stateText
            onChange={(useSharedAddons) => mutation.mutate({ useSharedAddons })}
          />
        </div>
        {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
      </div>
    )

  return <Panel title={t.mySources.preferenceTitle}>{body}</Panel>
}
