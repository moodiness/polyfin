import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  createApiKey,
  deleteApiKey,
  fetchApiKeys,
  queryClient,
  queryKeys,
  type ApiKey,
  type NewApiKey,
} from '@/api'
import {
  buttonPrimary,
  buttonSecondary,
  Card,
  ConfirmButton,
  Loading,
  Notice,
  PageHeader,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export default function ApiKeysPage() {
  const { t } = useI18n()
  const keys = useQuery({
    queryKey: queryKeys.apiKeys,
    queryFn: ({ signal }) => fetchApiKeys(signal),
  })
  const revoke = useMutation({
    mutationFn: (key: ApiKey) => deleteApiKey(key.id),
    onSettled: (_data, error) => {
      // A 404 means the key is already gone: refresh either way.
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
      }
    },
  })

  return (
    <>
      <PageHeader title={t.apiKeys.title} description={t.apiKeys.description} />
      <div className="space-y-6">
        <Card title={t.apiKeys.listTitle}>
          {revoke.isError && (
            <div className="mb-4">
              <Notice kind="error">{errorMessage(t, revoke.error)}</Notice>
            </div>
          )}
          {revoke.isSuccess && (
            <div className="mb-4">
              <Notice kind="success">{t.apiKeys.revoked(revoke.variables.app)}</Notice>
            </div>
          )}
          {keys.isPending ? (
            <Loading />
          ) : keys.isError ? (
            <Notice kind="error">{errorMessage(t, keys.error)}</Notice>
          ) : keys.data.length === 0 ? (
            <p className="text-sm text-muted">{t.apiKeys.empty}</p>
          ) : (
            <ul className="divide-y divide-line rounded-xl border border-line">
              {keys.data.map((key) => (
                <li
                  key={key.id}
                  className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0 text-sm">
                    <p className="font-medium break-words text-white">{key.app}</p>
                    <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 text-muted">
                      <dt>{t.apiKeys.created}</dt>
                      <dd className="text-zinc-200">
                        <RelativeTime iso={key.createdAt} />
                      </dd>
                      <dt>{t.apiKeys.lastUsed}</dt>
                      <dd className="text-zinc-200">
                        {key.lastUsedAt === null ? (
                          t.common.never
                        ) : (
                          <RelativeTime iso={key.lastUsedAt} />
                        )}
                      </dd>
                    </dl>
                  </div>
                  <div className="shrink-0">
                    <ConfirmButton
                      label={t.apiKeys.revoke}
                      busyLabel={t.apiKeys.revoking}
                      message={t.apiKeys.revokeConfirm(key.app)}
                      busy={revoke.isPending && revoke.variables.id === key.id}
                      onConfirm={() => revoke.mutate(key)}
                    />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card title={t.apiKeys.createTitle}>
          <CreateApiKeyForm />
        </Card>
      </div>
    </>
  )
}

function CreateApiKeyForm() {
  const { t } = useI18n()
  const [app, setApp] = useState('')

  const mutation = useMutation({
    mutationFn: createApiKey,
    onSuccess: () => {
      setApp('')
      void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate(app.trim())
  }

  if (mutation.isSuccess) {
    return <NewKey apiKey={mutation.data} onDone={() => mutation.reset()} />
  }

  return (
    <form onSubmit={submit} noValidate className="max-w-lg space-y-4">
      <TextField
        label={t.apiKeys.app}
        hint={t.apiKeys.appHint}
        value={app}
        onValue={(value) => {
          mutation.reset()
          setApp(value)
        }}
        autoComplete="off"
        maxLength={64}
        required
      />
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
        {mutation.isPending ? t.apiKeys.creating : t.apiKeys.create}
      </button>
    </form>
  )
}

/** The secret of a key just created: the server returns it this one time only. */
function NewKey({ apiKey, onDone }: { apiKey: NewApiKey; onDone: () => void }) {
  const { t } = useI18n()
  const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle')

  async function copyKey() {
    try {
      // The clipboard is missing outside secure contexts, such as plain http on a local network.
      await navigator.clipboard.writeText(apiKey.key)
      setCopy('copied')
    } catch {
      setCopy('failed')
    }
  }

  return (
    <div className="space-y-4">
      <p className="font-medium break-words text-white">{t.apiKeys.newKeyTitle(apiKey.app)}</p>
      <p
        role="status"
        className="rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-sm text-amber-200"
      >
        {t.apiKeys.newKeyNotice}
      </p>
      <code className="block rounded-lg border border-line bg-bg px-3 py-2 font-mono text-sm break-all text-white select-all">
        {apiKey.key}
      </code>
      {copy === 'copied' && <Notice kind="success">{t.apiKeys.copied}</Notice>}
      {copy === 'failed' && <Notice kind="error">{t.apiKeys.copyFailed}</Notice>}
      <div className="flex flex-wrap gap-2">
        <button type="button" className={buttonPrimary} onClick={() => void copyKey()}>
          {t.apiKeys.copy}
        </button>
        <button type="button" className={buttonSecondary} onClick={onDone}>
          {t.apiKeys.done}
        </button>
      </div>
    </div>
  )
}
