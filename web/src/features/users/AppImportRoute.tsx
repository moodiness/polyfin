import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import {
  fetchUserAppImport,
  fetchUsers,
  queryClient,
  queryKeys,
  startUserAppImport,
  userAppConnections,
  userAppReaders,
  type UserAppImport,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import AppImport, { AppConnections } from '@/features/account/AppImport'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Field, InlineError, Panel, Select, SkeletonRows } from '@/ui'

/** How often the import is read while it runs. */
const importPollMs = 2000

/**
 * `/users/app-import`: imports a user's watch data from their account on an app built on addons,
 * into the Polyfin account chosen, as the user does under My account. The app's password or auth
 * key only lives in the form's state.
 */
export default function AppImportRoute() {
  const { t } = useI18n()
  const text = t.users.appImport
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const [chosen, setChosen] = useState('')
  const userId = chosen === '' ? (users.data?.[0]?.id ?? '') : chosen
  const status = useQuery({
    queryKey: queryKeys.userAppImport(userId),
    queryFn: ({ signal }) => fetchUserAppImport(userId, signal),
    enabled: userId !== '',
    refetchInterval: (query) =>
      query.state.data?.import?.state === 'running' ||
      query.state.data?.connections.some((c) => c.importing)
        ? importPollMs
        : false,
  })
  const readers = useMemo(() => userAppReaders(userId), [userId])
  const connectionActions = useMemo(() => userAppConnections(userId), [userId])

  return (
    <PageLayout
      title={text.title}
      lede={text.description}
      back={{ to: '/users', label: t.users.title }}
    >
      {users.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : users.isError ? (
        <InlineError onRetry={() => void users.refetch()} retrying={users.isFetching}>
          {errorMessage(t, users.error)}
        </InlineError>
      ) : (
        <div className="flex flex-col gap-6">
          <Panel as="div">
            <div className="max-w-lg">
              <Field label={text.user} help={text.userHelp}>
                <Select
                  value={userId}
                  options={users.data.map((user) => ({ value: user.id, label: user.name }))}
                  onValue={setChosen}
                />
              </Field>
            </div>
          </Panel>
          {status.isError ? (
            <InlineError onRetry={() => void status.refetch()} retrying={status.isFetching}>
              {errorMessage(t, status.error)}
            </InlineError>
          ) : status.data === undefined ? (
            <SkeletonRows rows={2} boxed label={t.common.loading} />
          ) : (
            <>
              <AppConnections
                connections={status.data.connections}
                actions={connectionActions}
                onChanged={() =>
                  void queryClient.invalidateQueries({ queryKey: queryKeys.userAppImport(userId) })
                }
                own={false}
              />
              {/* A new form for each user: what was read of one account is never imported into another. */}
              <AppImport
                key={userId}
                current={status.data.import}
                addonsAllowed={status.data.addonsAllowed}
                readers={readers}
                start={(body) => startUserAppImport(userId, body)}
                onStarted={(started) =>
                  queryClient.setQueryData<UserAppImport>(
                    queryKeys.userAppImport(userId),
                    (data) => data && { ...data, import: started },
                  )
                }
                own={false}
              />
            </>
          )}
        </div>
      )}
    </PageLayout>
  )
}
