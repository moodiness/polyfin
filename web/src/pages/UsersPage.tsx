import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  createUser,
  deleteUser,
  fetchUserDevices,
  fetchUsers,
  queryClient,
  queryKeys,
  signOutUserDevice,
  updateUser,
  type User,
  type UserPatch,
} from '@/api'
import DeviceList from '@/components/DeviceList'
import { useSessionUser } from '@/components/session'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  Card,
  Checkbox,
  ConfirmButton,
  Loading,
  Notice,
  PageHeader,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export default function UsersPage() {
  const { t } = useI18n()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deletedName, setDeletedName] = useState<string | null>(null)
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })

  return (
    <>
      <PageHeader title={t.users.title} description={t.users.description} />
      <div className="space-y-6">
        <Card title={t.users.listTitle}>
          {deletedName !== null && (
            <div className="mb-4">
              <Notice kind="success">{t.users.deleted(deletedName)}</Notice>
            </div>
          )}
          {users.isPending ? (
            <Loading />
          ) : users.isError ? (
            <Notice kind="error">{errorMessage(t, users.error)}</Notice>
          ) : users.data.length === 0 ? (
            <p className="text-sm text-muted">{t.users.empty}</p>
          ) : (
            <ul className="divide-y divide-line rounded-xl border border-line">
              {users.data.map((user) => (
                <UserRow
                  key={user.id}
                  user={user}
                  expanded={editingId === user.id}
                  onToggle={() => {
                    setDeletedName(null)
                    setEditingId(editingId === user.id ? null : user.id)
                  }}
                  onDeleted={() => {
                    setEditingId(null)
                    setDeletedName(user.name)
                  }}
                />
              ))}
            </ul>
          )}
        </Card>
        <Card title={t.users.createTitle}>
          <CreateUserForm />
        </Card>
      </div>
    </>
  )
}

function UserRow({
  user,
  expanded,
  onToggle,
  onDeleted,
}: {
  user: User
  expanded: boolean
  onToggle: () => void
  onDeleted: () => void
}) {
  const { t } = useI18n()
  const self = useSessionUser()
  const editorId = `user-editor-${user.id}`

  return (
    <li className="p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="flex flex-wrap items-center gap-2 font-medium text-white">
            <span className="break-all">{user.name}</span>
            {user.id === self.id && <Badge tone="muted">{t.users.you}</Badge>}
            {user.isAdministrator && <Badge tone="fin">{t.users.administrator}</Badge>}
            {user.isHidden && <Badge tone="muted">{t.users.hidden}</Badge>}
            {user.isDisabled && <Badge tone="danger">{t.users.disabled}</Badge>}
          </p>
          <p className="mt-1 text-sm text-muted">
            {t.users.lastSignIn} —{' '}
            {user.lastLoginAt === null ? t.common.never : <RelativeTime iso={user.lastLoginAt} />}
          </p>
        </div>
        <button
          type="button"
          className={`${buttonSecondary} shrink-0`}
          aria-expanded={expanded}
          aria-controls={editorId}
          onClick={onToggle}
        >
          {expanded ? t.users.close : t.users.edit}
        </button>
      </div>
      {expanded && (
        <div id={editorId} className="mt-4">
          <UserEditor user={user} onDeleted={onDeleted} />
        </div>
      )}
    </li>
  )
}

/** A PATCH mutation for one user that keeps the list (and our own session) up to date. */
function useUserPatch(user: User) {
  const self = useSessionUser()
  return useMutation({
    mutationFn: (patch: UserPatch) => updateUser(user.id, patch),
    onSuccess: (updated) => {
      queryClient.setQueryData<User[]>(queryKeys.users, (old) =>
        old?.map((item) => (item.id === updated.id ? updated : item)),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      void queryClient.invalidateQueries({ queryKey: queryKeys.userDevices(user.id) })
      if (updated.id === self.id)
        void queryClient.invalidateQueries({ queryKey: queryKeys.session })
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 404) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      }
    },
  })
}

function UserEditor({ user, onDeleted }: { user: User; onDeleted: () => void }) {
  const { t } = useI18n()
  const [name, setName] = useState(user.name)
  const [password, setPassword] = useState('')
  const rename = useUserPatch(user)
  const resetPassword = useUserPatch(user)
  const access = useUserPatch(user)
  const remove = useMutation({
    mutationFn: () => deleteUser(user.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.userDevices(user.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      onDeleted()
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 404) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      }
    },
  })

  function submitName(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    rename.mutate({ name: name.trim() })
  }

  function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    resetPassword.mutate({ password }, { onSuccess: () => setPassword('') })
  }

  return (
    <div className="grid gap-4 rounded-xl border border-line bg-ink/40 p-4 lg:grid-cols-2">
      <h3 className="text-base font-semibold text-white lg:col-span-2">
        {t.users.editTitle(user.name)}
      </h3>

      <form onSubmit={submitName} noValidate className="space-y-3">
        <TextField
          label={t.users.name}
          hint={t.common.nameRule}
          value={name}
          onValue={(value) => {
            rename.reset()
            setName(value)
          }}
          autoComplete="off"
          maxLength={64}
          required
        />
        {rename.isError && <Notice kind="error">{errorMessage(t, rename.error)}</Notice>}
        {rename.isSuccess && <Notice kind="success">{t.users.renamed}</Notice>}
        <button type="submit" className={buttonSecondary} disabled={rename.isPending}>
          {rename.isPending ? t.common.saving : t.users.rename}
        </button>
      </form>

      <form onSubmit={submitPassword} noValidate className="space-y-3">
        <TextField
          label={t.users.newPassword}
          hint={`${t.common.passwordRule} ${t.users.resetPasswordHelp}`}
          type="password"
          value={password}
          onValue={(value) => {
            resetPassword.reset()
            setPassword(value)
          }}
          autoComplete="new-password"
          required
        />
        {resetPassword.isError && (
          <Notice kind="error">{errorMessage(t, resetPassword.error)}</Notice>
        )}
        {resetPassword.isSuccess && <Notice kind="success">{t.users.passwordReset}</Notice>}
        <button type="submit" className={buttonSecondary} disabled={resetPassword.isPending}>
          {resetPassword.isPending ? t.common.saving : t.users.resetPassword}
        </button>
      </form>

      <fieldset className="space-y-3 lg:col-span-2" disabled={access.isPending}>
        <legend className="mb-2 text-sm font-semibold text-white">{t.users.accessTitle}</legend>
        <p className="text-xs text-muted">{t.users.lastAdminHelp}</p>
        <Checkbox
          label={t.users.isAdministrator}
          help={t.users.isAdministratorHelp}
          checked={user.isAdministrator}
          onChange={(isAdministrator) => access.mutate({ isAdministrator })}
        />
        <Checkbox
          label={t.users.showOnSignIn}
          help={t.users.showOnSignInHelp}
          checked={!user.isHidden}
          onChange={(shown) => access.mutate({ isHidden: !shown })}
        />
        <Checkbox
          label={t.users.isDisabled}
          help={t.users.isDisabledHelp}
          checked={user.isDisabled}
          onChange={(isDisabled) => access.mutate({ isDisabled })}
        />
        {access.isError && <Notice kind="error">{errorMessage(t, access.error)}</Notice>}
        {access.isSuccess && <Notice kind="success">{t.users.updated}</Notice>}
      </fieldset>

      <section className="lg:col-span-2">
        <h4 className="mb-2 text-sm font-semibold text-white">{t.users.devicesTitle}</h4>
        <DeviceList
          queryKey={queryKeys.userDevices(user.id)}
          load={(signal) => fetchUserDevices(user.id, signal)}
          signOut={(deviceId) => signOutUserDevice(user.id, deviceId)}
        />
      </section>

      <div className="space-y-3 border-t border-line pt-4 lg:col-span-2">
        {remove.isError && <Notice kind="error">{errorMessage(t, remove.error)}</Notice>}
        <ConfirmButton
          label={t.users.delete}
          busyLabel={t.users.deleting}
          message={t.users.deleteConfirm(user.name)}
          busy={remove.isPending}
          onConfirm={() => remove.mutate()}
        />
      </div>
    </div>
  )
}

function CreateUserForm() {
  const { t } = useI18n()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [isAdministrator, setIsAdministrator] = useState(false)
  // Hidden by default, like Jellyfin.
  const [showOnSignIn, setShowOnSignIn] = useState(false)

  const mutation = useMutation({
    mutationFn: createUser,
    onSuccess: () => {
      setName('')
      setPassword('')
      setIsAdministrator(false)
      setShowOnSignIn(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ name: name.trim(), password, isAdministrator, isHidden: !showOnSignIn })
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label={t.users.name}
          hint={t.common.nameRule}
          value={name}
          onValue={setName}
          autoComplete="off"
          maxLength={64}
          required
        />
        <TextField
          label={t.users.password}
          hint={t.common.passwordRule}
          type="password"
          value={password}
          onValue={setPassword}
          autoComplete="new-password"
          required
        />
      </div>
      <Checkbox
        label={t.users.isAdministrator}
        help={t.users.isAdministratorHelp}
        checked={isAdministrator}
        onChange={setIsAdministrator}
      />
      <Checkbox
        label={t.users.showOnSignIn}
        help={t.users.showOnSignInHelp}
        checked={showOnSignIn}
        onChange={setShowOnSignIn}
      />
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && <Notice kind="success">{t.users.created(mutation.data.name)}</Notice>}
      <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
        {mutation.isPending ? t.users.creating : t.users.create}
      </button>
    </form>
  )
}
