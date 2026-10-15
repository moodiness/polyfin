import { CaretRightIcon, PlusIcon, UsersThreeIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import {
  ApiError,
  createGroup,
  fetchGroups,
  fetchUsers,
  queryClient,
  queryKeys,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  Row,
  RowList,
  Select,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'
import { UsersTabs } from './shared'

/**
 * `/users/groups`: the groups, each opening its page, and the button creating one. A group holds
 * the limits of a user's page, which its members follow.
 */
export default function GroupsList() {
  const { t } = useI18n()
  const [creating, setCreating] = useState(false)
  const groups = useQuery({
    queryKey: queryKeys.groups,
    queryFn: ({ signal }) => fetchGroups(signal),
  })
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })

  const create = (
    <Button variant="primary" icon={PlusIcon} onClick={() => setCreating(true)}>
      {t.groups.create}
    </Button>
  )

  return (
    <>
      <UsersTabs />
      <PageLayout title={t.groups.title} lede={t.groups.description} actions={create}>
        <Block title={t.groups.listTitle} count={groups.data?.length}>
          {groups.isPending ? (
            <SkeletonRows rows={2} boxed label={t.common.loading} />
          ) : groups.isError ? (
            <InlineError onRetry={() => void groups.refetch()} retrying={groups.isFetching}>
              {errorMessage(t, groups.error)}
            </InlineError>
          ) : groups.data.length === 0 ? (
            <EmptyState icon={UsersThreeIcon} title={t.groups.empty} action={create}>
              {t.groups.emptyHelp}
            </EmptyState>
          ) : (
            <RowList aria-label={t.groups.listTitle}>
              {groups.data.map((group) => (
                <Row
                  key={group.id}
                  to={`/users/groups/${group.id}`}
                  leading={UsersThreeIcon}
                  title={group.name}
                  meta={t.groups.memberCount(group.memberCount)}
                  trailing={<CaretRightIcon size={16} aria-hidden="true" className="text-ink-3" />}
                />
              ))}
            </RowList>
          )}
        </Block>
      </PageLayout>
      {/* Outside the layout, whose block spacing would push the panel down. */}
      <CreateGroupModal
        open={creating}
        onClose={() => setCreating(false)}
        users={users.data ?? []}
      />
    </>
  )
}

/**
 * The form creating a group: its name, and whose settings it starts with, a new user's (as made
 * with Create a user) or a user's.
 */
function CreateGroupModal({
  open,
  onClose,
  users,
}: {
  open: boolean
  onClose: () => void
  users: readonly User[]
}) {
  const { t } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const formId = useId()
  const [name, setName] = useState('')
  const [copyFrom, setCopyFrom] = useState('')
  const mutation = useMutation({
    mutationFn: createGroup,
    onSuccess: (created) => {
      queryClient.setQueryData(queryKeys.group(created.id), created)
      void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
      toast(t.groups.created(created.name))
      close()
      void navigate(`/users/groups/${created.id}`)
    },
  })

  function close() {
    mutation.reset()
    setName('')
    setCopyFrom('')
    onClose()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ name: name.trim(), ...(copyFrom === '' ? {} : { copyFrom }) })
  }

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  // The shared name codes speak of a user and of the user name rule: a group's name has its own.
  const nameError =
    code === 'name_taken'
      ? t.groups.nameTaken
      : code === 'invalid_name'
        ? t.groups.nameRule
        : undefined
  const otherError =
    mutation.isError && nameError === undefined ? errorMessage(t, mutation.error) : undefined

  return (
    <Modal
      open={open}
      onClose={close}
      title={t.groups.create}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={mutation.isPending}>
            {mutation.isPending ? t.groups.creating : t.groups.submit}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <Field label={t.groups.name} help={t.groups.nameRule} error={nameError}>
          <TextInput
            value={name}
            onValue={(value) => {
              mutation.reset()
              setName(value)
            }}
            autoComplete="off"
            maxLength={64}
            required
            invalid={nameError !== undefined}
          />
        </Field>
        <Field label={t.groups.copyFrom} help={t.groups.copyFromHelp}>
          <Select
            value={copyFrom}
            onValue={(value) => {
              mutation.reset()
              setCopyFrom(value)
            }}
            options={[
              { value: '', label: t.invites.newUser },
              ...users.map((user) => ({ value: user.id, label: user.name })),
            ]}
          />
        </Field>
        {otherError && <Notice tone="danger">{otherError}</Notice>}
      </form>
    </Modal>
  )
}
