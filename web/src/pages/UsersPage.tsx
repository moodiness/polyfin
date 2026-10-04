import { useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  createUser,
  deleteUser,
  fetchParentalRatings,
  fetchUserDevices,
  fetchUsers,
  queryClient,
  queryKeys,
  signOutUserDevice,
  unblockUser,
  updateUser,
  maxPlaybacksRange,
  type ParentalControl,
  type ParentalRating,
  type User,
  type UserPatch,
  type SyncPlayAccess,
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
            <RatingLimitBadge parentalControl={user.parentalControl} />
            {!user.transcoding && <Badge tone="muted">{t.users.noTranscoding}</Badge>}
            {!user.downloads && <Badge tone="muted">{t.users.noDownloads}</Badge>}
            {!user.personalAddons && <Badge tone="muted">{t.users.noPersonalAddons}</Badge>}
            {user.blockedUntil !== null && <BlockedBadge until={user.blockedUntil} />}
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
  const unblock = useMutation({
    mutationFn: () => unblockUser(user.id),
    onSuccess: (updated) => {
      queryClient.setQueryData<User[]>(queryKeys.users, (old) =>
        old?.map((item) => (item.id === updated.id ? updated : item)),
      )
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 404) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      }
    },
  })
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
        <Checkbox
          label={t.users.canTranscode}
          help={t.users.canTranscodeHelp}
          checked={user.transcoding}
          onChange={(transcoding) => access.mutate({ transcoding })}
        />
        <Checkbox
          label={t.users.canDownload}
          help={t.users.canDownloadHelp}
          checked={user.downloads}
          onChange={(downloads) => access.mutate({ downloads })}
        />
        <Checkbox
          label={t.users.canAddAddons}
          help={t.users.canAddAddonsHelp}
          checked={user.personalAddons}
          onChange={(personalAddons) => access.mutate({ personalAddons })}
        />
        {access.isError && <Notice kind="error">{errorMessage(t, access.error)}</Notice>}
        {access.isSuccess && <Notice kind="success">{t.users.updated}</Notice>}
        {user.blockedUntil !== null && (
          <div className="space-y-2 rounded-lg border border-rose-400/40 bg-rose-500/5 p-3">
            <p className="text-sm text-rose-100">{t.users.blockedHelp}</p>
            <button
              type="button"
              className={buttonSecondary}
              disabled={unblock.isPending}
              onClick={() => unblock.mutate()}
            >
              {unblock.isPending ? t.users.unblocking : t.users.unblock}
            </button>
          </div>
        )}
        {unblock.isError && <Notice kind="error">{errorMessage(t, unblock.error)}</Notice>}
        {unblock.isSuccess && <Notice kind="success">{t.users.unblocked}</Notice>}
      </fieldset>

      <PlaybackAccessForm user={user} />

      <ParentalControlForm user={user} />

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

/** Ratings sharing a score and sub-score, offered as one choice (like jellyfin-web). */
type RatingGroup = { name: string; score: number; subScore: number | null }

function groupRatings(ratings: ParentalRating[]): RatingGroup[] {
  const groups: RatingGroup[] = []
  for (const rating of ratings) {
    const last = groups.at(-1)
    if (last !== undefined && last.score === rating.score && last.subScore === rating.subScore) {
      last.name = `${last.name} / ${rating.name}`
    } else {
      groups.push({ ...rating })
    }
  }
  return groups
}

/** The group matching the limit exactly, else the last one whose score fits under it. */
function selectedGroup(groups: RatingGroup[], control: ParentalControl): number {
  const { maxRating, maxSubRating } = control
  if (maxRating === null) return -1
  const exact = groups.findIndex((g) => g.score === maxRating && g.subScore === maxSubRating)
  if (exact !== -1) return exact
  let fallback = -1
  groups.forEach((group, index) => {
    if (group.score <= maxRating) fallback = index
  })
  return fallback
}

function useParentalRatings(enabled = true) {
  return useQuery({
    queryKey: queryKeys.parentalRatings,
    queryFn: ({ signal }) => fetchParentalRatings(signal),
    staleTime: Infinity,
    enabled,
  })
}

function RatingLimitBadge({ parentalControl }: { parentalControl: ParentalControl }) {
  const { t } = useI18n()
  const limited = parentalControl.maxRating !== null
  const ratings = useParentalRatings(limited)
  if (!limited || ratings.data === undefined) return null
  const groups = groupRatings(ratings.data)
  const group = groups[selectedGroup(groups, parentalControl)]
  if (group === undefined) return null
  return <Badge tone="muted">{t.users.ratingLimit(group.name)}</Badge>
}

/** "Blocked until 14:05": the account refuses sign-ins until then, after too many wrong passwords. */
function BlockedBadge({ until }: { until: string }) {
  const { t, language } = useI18n()
  const time = new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit' }).format(
    new Date(until),
  )
  return <Badge tone="danger">{t.users.blockedUntil(time)}</Badge>
}

const unratedMovie = 'Movie'
const unratedSeries = 'Series'

function ParentalControlForm({ user }: { user: User }) {
  const { t } = useI18n()
  const selectId = useId()
  const ratings = useParentalRatings()
  const save = useUserPatch(user)
  const initial = user.parentalControl
  const [limit, setLimit] = useState<{ maxRating: number | null; maxSubRating: number | null }>({
    maxRating: initial.maxRating,
    maxSubRating: initial.maxSubRating,
  })
  const [blockMovies, setBlockMovies] = useState(initial.blockUnrated.includes(unratedMovie))
  const [blockShows, setBlockShows] = useState(initial.blockUnrated.includes(unratedSeries))

  const groups = ratings.data === undefined ? [] : groupRatings(ratings.data)
  const selected = selectedGroup(groups, { ...limit, blockUnrated: [] })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    // Keep values set by Jellyfin apps (live TV, books...) that this form does not show.
    const blockUnrated = user.parentalControl.blockUnrated.filter(
      (item) => item !== unratedMovie && item !== unratedSeries,
    )
    if (blockMovies) blockUnrated.push(unratedMovie)
    if (blockShows) blockUnrated.push(unratedSeries)
    save.mutate({ parentalControl: { ...limit, blockUnrated } })
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-3 lg:col-span-2">
      <h4 className="text-sm font-semibold text-white">{t.users.parentalTitle}</h4>
      {ratings.isError ? (
        <Notice kind="error">{errorMessage(t, ratings.error)}</Notice>
      ) : (
        <div>
          <label htmlFor={selectId} className="block text-sm font-medium text-zinc-200">
            {t.users.maxRating}
          </label>
          <select
            id={selectId}
            value={selected}
            disabled={ratings.isPending}
            onChange={(event) => {
              save.reset()
              const group = groups[Number(event.target.value)]
              setLimit(
                group === undefined
                  ? { maxRating: null, maxSubRating: null }
                  : { maxRating: group.score, maxSubRating: group.subScore },
              )
            }}
            aria-describedby={`${selectId}-hint`}
            className="mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white sm:max-w-sm"
          >
            <option value={-1}>{t.users.noLimit}</option>
            {groups.map((group, index) => (
              <option key={group.name} value={index}>
                {group.name}
              </option>
            ))}
          </select>
          <p id={`${selectId}-hint`} className="mt-1 text-xs text-muted">
            {t.users.maxRatingHelp}
          </p>
        </div>
      )}
      <Checkbox
        label={t.users.blockUnratedMovies}
        checked={blockMovies}
        onChange={(checked) => {
          save.reset()
          setBlockMovies(checked)
        }}
      />
      <Checkbox
        label={t.users.blockUnratedShows}
        checked={blockShows}
        onChange={(checked) => {
          save.reset()
          setBlockShows(checked)
        }}
      />
      {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
      {save.isSuccess && <Notice kind="success">{t.users.updated}</Notice>}
      <button
        type="submit"
        className={buttonSecondary}
        disabled={save.isPending || ratings.isPending}
      >
        {save.isPending ? t.common.saving : t.users.saveParental}
      </button>
    </form>
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

/** The maximum quality choices, in bits per second (0 for no limit), like jellyfin-web's. */
const bitrateChoices = [
  { value: 0, label: 'bitrateNoLimit' },
  { value: 40_000_000, label: 'bitrate4k' },
  { value: 20_000_000, label: 'bitrate1080High' },
  { value: 10_000_000, label: 'bitrate1080' },
  { value: 4_000_000, label: 'bitrate720' },
  { value: 2_000_000, label: 'bitrate480' },
] as const

function PlaybackAccessForm({ user }: { user: User }) {
  const { t, language } = useI18n()
  const bitrateId = useId()
  const syncPlayId = useId()
  const save = useUserPatch(user)
  const [form, setForm] = useState({
    maxPlaybacks: user.maxPlaybacks,
    maxBitrate: user.maxBitrate,
    liveTv: user.liveTv,
    syncPlay: user.syncPlay,
    remoteControl: user.remoteControl,
  })

  function update(change: Partial<typeof form>) {
    save.reset()
    setForm((current) => ({ ...current, ...change }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    save.mutate(form)
  }

  // A limit set from a Jellyfin app may match none of the choices: it is shown as it is.
  const custom = bitrateChoices.some((choice) => choice.value === form.maxBitrate)
    ? null
    : (form.maxBitrate / 1_000_000).toLocaleString(language, { maximumFractionDigits: 2 })
  const selectClass =
    'mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white sm:max-w-sm'

  return (
    <form onSubmit={submit} noValidate className="space-y-3 lg:col-span-2">
      <h4 className="text-sm font-semibold text-white">{t.users.playbackAccessTitle}</h4>
      <TextField
        label={t.users.maxPlaybacks}
        hint={t.users.maxPlaybacksHelp}
        type="number"
        inputMode="numeric"
        min={maxPlaybacksRange.min}
        max={maxPlaybacksRange.max}
        step={1}
        value={form.maxPlaybacks}
        onValue={(value) => update({ maxPlaybacks: Math.trunc(Number(value)) })}
        className="sm:max-w-sm"
      />
      <div>
        <label htmlFor={bitrateId} className="block text-sm font-medium text-zinc-200">
          {t.users.maxBitrate}
        </label>
        <select
          id={bitrateId}
          value={form.maxBitrate}
          onChange={(event) => update({ maxBitrate: Number(event.target.value) })}
          aria-describedby={`${bitrateId}-hint`}
          className={selectClass}
        >
          {bitrateChoices.map((choice) => (
            <option key={choice.value} value={choice.value}>
              {t.users[choice.label]}
            </option>
          ))}
          {custom !== null && (
            <option value={form.maxBitrate}>{t.users.bitrateOther(custom)}</option>
          )}
        </select>
        <p id={`${bitrateId}-hint`} className="mt-1 text-xs text-muted">
          {t.users.maxBitrateHelp}
        </p>
      </div>
      <Checkbox
        label={t.users.liveTv}
        help={t.users.liveTvHelp}
        checked={form.liveTv}
        onChange={(liveTv) => update({ liveTv })}
      />
      <div>
        <label htmlFor={syncPlayId} className="block text-sm font-medium text-zinc-200">
          {t.users.syncPlay}
        </label>
        <select
          id={syncPlayId}
          value={form.syncPlay}
          onChange={(event) => update({ syncPlay: event.target.value as SyncPlayAccess })}
          aria-describedby={`${syncPlayId}-hint`}
          className={selectClass}
        >
          <option value="CreateAndJoinGroups">{t.users.syncPlayCreateAndJoin}</option>
          <option value="JoinGroups">{t.users.syncPlayJoin}</option>
          <option value="None">{t.users.syncPlayNone}</option>
        </select>
        <p id={`${syncPlayId}-hint`} className="mt-1 text-xs text-muted">
          {t.users.syncPlayHelp}
        </p>
      </div>
      <Checkbox
        label={t.users.remoteControl}
        help={t.users.remoteControlHelp}
        checked={form.remoteControl}
        onChange={(remoteControl) => update({ remoteControl })}
      />
      {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
      {save.isSuccess && <Notice kind="success">{t.users.updated}</Notice>}
      <button type="submit" className={buttonSecondary} disabled={save.isPending}>
        {save.isPending ? t.common.saving : t.users.savePlaybackAccess}
      </button>
    </form>
  )
}
