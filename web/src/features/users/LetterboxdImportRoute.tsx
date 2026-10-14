import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchUserLetterboxdImport,
  fetchUsers,
  queryClient,
  queryKeys,
  startUserLetterboxdImport,
  type UserLetterboxdImport,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { LetterboxdForm, LetterboxdProgress } from '@/features/account/LetterboxdImport'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Field, InlineError, Panel, Select, SkeletonRows } from '@/ui'

/** How often the import is read while it runs. */
const importPollMs = 2000

/**
 * `/users/letterboxd-import`: imports a user's Letterboxd export file into the Polyfin account
 * chosen, as the user does under My account.
 */
export default function LetterboxdImportRoute() {
  const { t } = useI18n()
  const text = t.users.letterboxdImport
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const [chosen, setChosen] = useState('')
  const userId = chosen === '' ? (users.data?.[0]?.id ?? '') : chosen
  const status = useQuery({
    queryKey: queryKeys.userLetterboxdImport(userId),
    queryFn: ({ signal }) => fetchUserLetterboxdImport(userId, signal),
    enabled: userId !== '',
    refetchInterval: (query) =>
      query.state.data?.import?.state === 'running' ? importPollMs : false,
  })

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
              {status.data.import !== null && (
                <LetterboxdProgress current={status.data.import} own={false} />
              )}
              {status.data.import?.state !== 'running' && (
                <Panel as="div" flush>
                  {/* A new form for each user: a file chosen for one is never sent for another. */}
                  <LetterboxdForm
                    key={userId}
                    searchable={status.data.searchable}
                    own={false}
                    start={(upload) => startUserLetterboxdImport(userId, upload)}
                    onStarted={(started) =>
                      queryClient.setQueryData<UserLetterboxdImport>(
                        queryKeys.userLetterboxdImport(userId),
                        (data) => data && { ...data, import: started },
                      )
                    }
                  />
                </Panel>
              )}
            </>
          )}
        </div>
      )}
    </PageLayout>
  )
}
