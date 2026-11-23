import { PlusIcon, StackIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import {
  ApiError,
  createCollection,
  fetchCollections,
  queryClient,
  queryKeys,
  type CollectionSummary,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  Row,
  RowList,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'

/** The page of a collection of the server. */
export function collectionPath(id: string): string {
  return `/libraries/collections/${encodeURIComponent(id)}`
}

/** The message of a failed collection request: the collections' own codes, else the usual one. */
export function collectionError(t: Messages, error: unknown): string {
  const errors = t.collections.errors
  if (error instanceof ApiError && Object.hasOwn(errors, error.code))
    return errors[error.code as keyof typeof errors]
  return errorMessage(t, error)
}

/**
 * Under Content › Libraries: the server's collections (those users make in Jellyfin apps), each
 * opening its page, with the button that makes another.
 */
export default function Collections() {
  const { t } = useI18n()
  const c = t.collections
  const [creating, setCreating] = useState(false)
  const collections = useQuery({
    queryKey: queryKeys.collections,
    queryFn: ({ signal }) => fetchCollections(signal),
  })
  const add = (
    <Button icon={PlusIcon} size="sm" onClick={() => setCreating(true)}>
      {c.add}
    </Button>
  )

  let body
  if (collections.isPending) body = <SkeletonRows rows={2} label={c.loading} />
  else if (collections.isError)
    body = (
      <InlineError onRetry={() => void collections.refetch()} retrying={collections.isFetching}>
        {errorMessage(t, collections.error)}
      </InlineError>
    )
  else if (collections.data.length === 0)
    body = (
      <EmptyState icon={StackIcon} title={c.emptyTitle} action={add}>
        {c.emptyHow}
      </EmptyState>
    )
  else
    body = (
      <RowList variant="boxed" aria-label={c.title}>
        {collections.data.map((collection) => (
          <Row
            key={collection.id}
            leading={StackIcon}
            to={collectionPath(collection.id)}
            title={collection.name}
            titleAside={
              collection.displayOrder === 'chosen' && <Badge>{c.displayOrders.chosen}</Badge>
            }
            meta={summary(collection)}
          />
        ))}
      </RowList>
    )

  function summary(collection: CollectionSummary): string {
    const parts = [c.entryCount(collection.entries)]
    if (collection.collections > 0) parts.push(c.collectionCount(collection.collections))
    return parts.join(' · ')
  }

  return (
    <Block
      title={c.title}
      count={collections.data?.length}
      aside={collections.data && collections.data.length > 0 ? add : undefined}
    >
      <p className="mb-3 max-w-[66ch] text-small text-ink-2">{c.help}</p>
      {body}
      {creating && <CreateDialog onClose={() => setCreating(false)} />}
    </Block>
  )
}

/** Asks the name of a new collection, makes it, then opens its page. */
function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n()
  const c = t.collections
  const toast = useToast()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const create = useMutation({
    mutationFn: () => createCollection(name.trim()),
    onSuccess: (collection) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.collections })
      queryClient.setQueryData(queryKeys.collection(collection.id), collection)
      toast(c.created(collection.name))
      void navigate(collectionPath(collection.id))
    },
  })
  const formId = 'new-collection'
  return (
    <Modal
      open
      onClose={onClose}
      title={c.newTitle}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            loading={create.isPending}
            disabled={name.trim() === ''}
          >
            {c.create}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="flex flex-col gap-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          if (name.trim() !== '') create.mutate()
        }}
      >
        <Field label={c.name}>
          <TextInput value={name} maxLength={500} onChange={(e) => setName(e.target.value)} />
        </Field>
        {create.isError && <Notice tone="danger">{collectionError(t, create.error)}</Notice>}
      </form>
    </Modal>
  )
}
