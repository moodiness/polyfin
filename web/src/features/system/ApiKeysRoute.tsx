import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { CheckIcon, CopyIcon, KeyIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react'
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
import { PageLayout } from '@/app/PageLayout'
import {
  Block,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Notice,
  Panel,
  PanelFooter,
  Row,
  RowList,
  SkeletonRows,
  TextInput,
  useToast,
  RelativeTime,
} from '@/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** `/system/api-keys`: keys for other tools, created once, shown once, revoked here. */
export default function ApiKeysRoute() {
  const { t } = useI18n()
  const text = t.apiKeys
  const toast = useToast()
  const keys = useQuery({
    queryKey: queryKeys.apiKeys,
    queryFn: ({ signal }) => fetchApiKeys(signal),
  })
  const [asked, setAsked] = useState<ApiKey | null>(null)
  const revoke = useMutation({
    mutationFn: (key: ApiKey) => deleteApiKey(key.id),
    onSuccess: (_data, key) => {
      setAsked(null)
      toast(text.revoked(key.app), { tone: 'ok' })
    },
    onSettled: (_data, error) => {
      // A 404 means the key is already gone: refresh either way.
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
      }
    },
  })

  return (
    <PageLayout title={text.title} lede={text.description}>
      <Block title={text.listTitle} count={keys.data?.length}>
        {keys.isPending ? (
          <SkeletonRows rows={2} />
        ) : keys.isError ? (
          <InlineError onRetry={() => void keys.refetch()} retrying={keys.isFetching}>
            {errorMessage(t, keys.error)}
          </InlineError>
        ) : keys.data.length === 0 ? (
          <EmptyState icon={KeyIcon} title={text.empty}>
            {text.emptyHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.listTitle}>
            {keys.data.map((key) => (
              <Row
                key={key.id}
                leading={KeyIcon}
                title={key.app}
                meta={
                  <span className="truncate tabular-nums">
                    {text.created}
                    {t.common.colon} <RelativeTime iso={key.createdAt} />
                    {' · '}
                    {text.lastUsed}
                    {t.common.colon}{' '}
                    {key.lastUsedAt === null ? (
                      t.common.never
                    ) : (
                      <RelativeTime iso={key.lastUsedAt} />
                    )}
                  </span>
                }
                trailing={
                  <Button
                    size="sm"
                    variant="danger"
                    icon={TrashIcon}
                    aria-label={text.revokeLabel(key.app)}
                    loading={revoke.isPending && revoke.variables.id === key.id}
                    onClick={() => {
                      revoke.reset()
                      setAsked(key)
                    }}
                  >
                    {text.revoke}
                  </Button>
                }
              />
            ))}
          </RowList>
        )}
      </Block>
      <CreateApiKey />
      <ConfirmDialog
        open={asked !== null}
        onClose={() => setAsked(null)}
        onConfirm={() => asked && revoke.mutate(asked)}
        title={text.revokeTitle}
        confirmLabel={revoke.isPending ? text.revoking : text.revoke}
        tone="danger"
        busy={revoke.isPending}
        error={revoke.isError ? errorMessage(t, revoke.error) : undefined}
      >
        {asked && text.revokeConfirm(asked.app)}
      </ConfirmDialog>
    </PageLayout>
  )
}

function CreateApiKey() {
  const { t } = useI18n()
  const text = t.apiKeys
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
    <Panel title={text.createTitle} as="section" flush>
      <form onSubmit={submit} noValidate>
        <div className="max-w-lg px-6 pb-6 max-sm:px-4">
          <Field
            label={text.app}
            help={text.appHint}
            error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
          >
            <TextInput
              value={app}
              onValue={(value) => {
                mutation.reset()
                setApp(value)
              }}
              autoComplete="off"
              maxLength={64}
              required
            />
          </Field>
        </div>
        <PanelFooter>
          <Button type="submit" variant="primary" icon={PlusIcon} loading={mutation.isPending}>
            {mutation.isPending ? text.creating : text.create}
          </Button>
        </PanelFooter>
      </form>
    </Panel>
  )
}

/** The secret of a key just created: the server returns it this one time only. */
function NewKey({ apiKey, onDone }: { apiKey: NewApiKey; onDone: () => void }) {
  const { t } = useI18n()
  const text = t.apiKeys
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
    <Panel
      title={text.newKeyTitle(apiKey.app)}
      footer={
        <PanelFooter>
          <Button onClick={onDone}>{text.done}</Button>
          <Button
            variant="primary"
            icon={copy === 'copied' ? CheckIcon : CopyIcon}
            onClick={() => void copyKey()}
          >
            {text.copy}
          </Button>
        </PanelFooter>
      }
    >
      <div className="space-y-4">
        <Notice tone="warn" live>
          {text.newKeyNotice}
        </Notice>
        <code className="block rounded-field border border-line-2 bg-bg px-3.5 py-2.5 font-mono text-control break-all text-ink select-all">
          {apiKey.key}
        </code>
        <div aria-live="polite">
          {copy === 'copied' && <Notice tone="ok">{text.copied}</Notice>}
          {copy === 'failed' && <Notice tone="danger">{text.copyFailed}</Notice>}
        </div>
      </div>
    </Panel>
  )
}
