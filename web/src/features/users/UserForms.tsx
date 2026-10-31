import { useMutation, useQuery } from '@tanstack/react-query'
import { PlusIcon, TrashIcon, XIcon } from '@phosphor-icons/react'
import { Fragment, useState, type FormEvent, type ReactNode } from 'react'
import {
  fetchGroupDefaults,
  fetchUserContentChoices,
  fetchUserDefaults,
  fetchUsers,
  maxPlaybacksRange,
  qualityGroups,
  queryClient,
  queryKeys,
  resetGroup,
  resetUser,
  scheduleDays,
  updateGroup,
  updateUser,
  type AccessSchedule,
  type Group,
  type GroupLimits,
  type GroupPatch,
  type QualityGroup,
  type ScheduleDay,
  type SyncPlayAccess,
  type User,
} from '@/api'
import { useSessionUser } from '@/app/session'
import { useHealthProblems } from '@/features/system/problems'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  IconButton,
  InlineError,
  Notice,
  NumberInput,
  Panel,
  PanelFooter,
  Select,
  Skeleton,
  SuggestInput,
  TextLink,
  useToast,
} from '@/ui'
import {
  groupRatings,
  groupSaved,
  qualityGroupName,
  refreshGroupIfGone,
  refreshIfGone,
  selectedGroup,
  SwitchRow,
  useParentalRatings,
  userSaved,
} from './shared'

/**
 * Whose limits a section edits: a user's, or a group's, which its following members take at once.
 */
export type LimitsTarget = { kind: 'user'; user: User } | { kind: 'group'; group: Group }

/** The group whose values a user follows, shown read only on their page; null otherwise. */
export function followedGroup(user: User): { id: string; name: string } | null {
  return user.groupId !== null && user.followGroup
    ? { id: user.groupId, name: user.groupName ?? '' }
    : null
}

/** "Set by the group Kids.", with a link to the group, above the values a member follows. */
export function FollowedNotice({ group }: { group: { id: string; name: string } }) {
  const { t } = useI18n()
  return (
    <Notice
      action={
        <TextLink to={`/users/groups/${group.id}`} arrow>
          {t.groups.openGroup}
        </TextLink>
      }
    >
      {t.groups.setByGroup(group.name)}
    </Notice>
  )
}

/**
 * A section of a user's or a group's page: a panel holding one form, its error and its save
 * button at the foot. Saving shows a toast. On the page of a member following their group, the
 * form shows the group's values, read only, without its button.
 */
function SectionForm({
  id,
  title,
  description,
  onSubmit,
  error,
  saving,
  saveLabel,
  disabled = false,
  target,
  children,
}: {
  id: string
  title: string
  description?: ReactNode
  onSubmit: () => void
  error: string | null
  saving: boolean
  saveLabel: string
  disabled?: boolean
  target: LimitsTarget
  children: ReactNode
}) {
  const { t } = useI18n()
  const followed = target.kind === 'user' ? followedGroup(target.user) : null
  return (
    <form
      id={id}
      noValidate
      className="scroll-mt-[calc(var(--spacing-topbar)+32px)] max-md:scroll-mt-[calc(var(--spacing-topbar)+64px)]"
      onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault()
        if (followed === null) onSubmit()
      }}
    >
      <Panel
        title={title}
        description={description}
        footer={
          followed === null && (
            <PanelFooter>
              <Button variant="secondary" type="submit" loading={saving} disabled={disabled}>
                {saving ? t.common.saving : saveLabel}
              </Button>
            </PanelFooter>
          )
        }
      >
        <div className="space-y-5">
          {followed !== null && <FollowedNotice group={followed} />}
          {/* A disabled fieldset turns off every control in it, for the values a member follows. */}
          <fieldset disabled={followed !== null} className="min-w-0 space-y-5">
            {children}
          </fieldset>
          {error !== null && <Notice tone="danger">{error}</Notice>}
        </div>
      </Panel>
    </form>
  )
}

/** A patch for one section of a user or a group, with its toast and inline error. */
function useSectionSave(target: LimitsTarget) {
  const { t } = useI18n()
  const toast = useToast()
  const self = useSessionUser()
  const save = useMutation({
    mutationFn: async (patch: GroupPatch) => {
      if (target.kind === 'group') groupSaved(await updateGroup(target.group.id, patch))
      else userSaved(await updateUser(target.user.id, patch), self.id)
    },
    onError: target.kind === 'group' ? refreshGroupIfGone : refreshIfGone,
  })
  return {
    save,
    error: save.isError ? errorMessage(t, save.error) : null,
    submit: (patch: GroupPatch) =>
      save.mutate(patch, {
        onSuccess: () => toast(target.kind === 'group' ? t.groups.updated : t.users.updated),
      }),
  }
}

/** The values a section shows: the user's own, or the group's. */
function limitsOf(target: LimitsTarget): GroupLimits {
  return target.kind === 'group' ? target.group : target.user
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

export function PlaybackAccessForm({ target, id }: { target: LimitsTarget; id: string }) {
  const { t, language } = useI18n()
  const { save, error, submit } = useSectionSave(target)
  const limits = limitsOf(target)
  const [form, setForm] = useState({
    maxPlaybacks: limits.maxPlaybacks,
    maxBitrate: limits.maxBitrate,
    liveTv: limits.liveTv,
    syncPlay: limits.syncPlay,
    remoteControl: limits.remoteControl,
    liveTvManagement: limits.liveTvManagement,
    qualityGroup: limits.qualityGroup,
  })

  function update(change: Partial<typeof form>) {
    save.reset()
    setForm((current) => ({ ...current, ...change }))
  }

  // A limit set from a Jellyfin app may match none of the choices: it is shown as it is.
  const custom = bitrateChoices.some((choice) => choice.value === form.maxBitrate)
    ? null
    : (form.maxBitrate / 1_000_000).toLocaleString(language, { maximumFractionDigits: 2 })

  return (
    <SectionForm
      id={id}
      target={target}
      title={t.users.playbackAccessTitle}
      onSubmit={() => submit(form)}
      error={error}
      saving={save.isPending}
      saveLabel={t.users.savePlaybackAccess}
    >
      <Field label={t.users.maxPlaybacks} help={t.users.maxPlaybacksHelp}>
        <NumberInput
          inputMode="numeric"
          min={maxPlaybacksRange.min}
          max={maxPlaybacksRange.max}
          step={1}
          value={form.maxPlaybacks}
          onValue={(value) => update({ maxPlaybacks: Math.trunc(value ?? 0) })}
        />
      </Field>
      <div className="grid gap-5 sm:grid-cols-2">
        <Field label={t.users.maxBitrate} help={t.users.maxBitrateHelp}>
          <Select
            value={form.maxBitrate}
            onValue={(maxBitrate) => update({ maxBitrate })}
            options={[
              ...bitrateChoices.map((choice) => ({
                value: choice.value as number,
                label: t.users[choice.label],
              })),
              ...(custom === null
                ? []
                : [{ value: form.maxBitrate, label: t.users.bitrateOther(custom) }]),
            ]}
          />
        </Field>
        <Field label={t.users.qualityGroup} help={t.users.qualityGroupHelp}>
          <Select
            value={form.qualityGroup}
            onValue={(qualityGroup) => update({ qualityGroup: qualityGroup as QualityGroup })}
            options={qualityGroups.map((group) => ({
              value: group as number,
              label: group === 0 ? t.users.qualityGroupOriginal : qualityGroupName(group),
            }))}
          />
        </Field>
      </div>
      <Field label={t.users.syncPlay} help={t.users.syncPlayHelp}>
        <Select<SyncPlayAccess>
          value={form.syncPlay}
          onValue={(syncPlay) => update({ syncPlay })}
          className="sm:max-w-sm"
          options={[
            { value: 'CreateAndJoinGroups', label: t.users.syncPlayCreateAndJoin },
            { value: 'JoinGroups', label: t.users.syncPlayJoin },
            { value: 'None', label: t.users.syncPlayNone },
          ]}
        />
      </Field>
      <div>
        <SwitchRow
          title={t.users.liveTv}
          help={t.users.liveTvHelp}
          checked={form.liveTv}
          onChange={(liveTv) => update({ liveTv })}
        />
        <SwitchRow
          title={t.users.remoteControl}
          help={t.users.remoteControlHelp}
          checked={form.remoteControl}
          onChange={(remoteControl) => update({ remoteControl })}
        />
        <SwitchRow
          title={t.users.liveTvManagement}
          help={t.users.liveTvManagementHelp}
          checked={form.liveTvManagement}
          onChange={(liveTvManagement) => update({ liveTvManagement })}
        />
      </div>
    </SectionForm>
  )
}

const unratedMovie = 'Movie'
const unratedSeries = 'Series'

export function ParentalControlForm({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const ratings = useParentalRatings()
  const { save, error, submit } = useSectionSave(target)
  const initial = limitsOf(target).parentalControl
  const [limit, setLimit] = useState<{ maxRating: number | null; maxSubRating: number | null }>({
    maxRating: initial.maxRating,
    maxSubRating: initial.maxSubRating,
  })
  const [blockMovies, setBlockMovies] = useState(initial.blockUnrated.includes(unratedMovie))
  const [blockShows, setBlockShows] = useState(initial.blockUnrated.includes(unratedSeries))

  const groups = ratings.data === undefined ? [] : groupRatings(ratings.data)
  const selected = selectedGroup(groups, { ...limit, blockUnrated: [] })

  function onSubmit() {
    // Keep values set by Jellyfin apps (live TV, books...) that this form does not show.
    const blockUnrated = initial.blockUnrated.filter(
      (item) => item !== unratedMovie && item !== unratedSeries,
    )
    if (blockMovies) blockUnrated.push(unratedMovie)
    if (blockShows) blockUnrated.push(unratedSeries)
    submit({ parentalControl: { ...limit, blockUnrated } })
  }

  return (
    <SectionForm
      id={id}
      target={target}
      title={t.users.parentalTitle}
      onSubmit={onSubmit}
      error={error}
      saving={save.isPending}
      saveLabel={t.users.saveParental}
      disabled={ratings.isPending}
    >
      {ratings.isError ? (
        <InlineError onRetry={() => void ratings.refetch()} retrying={ratings.isFetching}>
          {errorMessage(t, ratings.error)}
        </InlineError>
      ) : (
        <Field label={t.users.maxRating} help={t.users.maxRatingHelp}>
          {ratings.isPending ? (
            <Skeleton className="h-10 sm:max-w-sm" />
          ) : (
            <Select
              value={selected}
              className="sm:max-w-sm"
              onValue={(index) => {
                save.reset()
                const group = groups[index]
                setLimit(
                  group === undefined
                    ? { maxRating: null, maxSubRating: null }
                    : { maxRating: group.score, maxSubRating: group.subScore },
                )
              }}
              options={[
                { value: -1, label: t.users.noLimit },
                ...groups.map((group, index) => ({ value: index, label: group.name })),
              ]}
            />
          )}
        </Field>
      )}
      <div className="space-y-3">
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
      </div>
    </SectionForm>
  )
}

export function VisibleLibrariesForm({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const { save, error, submit } = useSectionSave(target)
  const [hidden, setHidden] = useState(limitsOf(target).hiddenLibraries)

  function onSubmit() {
    const libraries = choices.data?.libraries ?? []
    // Libraries removed from the server since are dropped.
    submit({ hiddenLibraries: hidden.filter((id) => libraries.some((l) => l.id === id)) })
  }

  return (
    <SectionForm
      id={id}
      target={target}
      title={t.users.visibleLibrariesTitle}
      description={t.users.visibleLibrariesHelp}
      onSubmit={onSubmit}
      error={error}
      saving={save.isPending}
      saveLabel={t.users.saveVisibleLibraries}
      disabled={!choices.isSuccess}
    >
      {choices.isPending ? (
        <div className="grid gap-3 sm:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-5 w-40" />
          ))}
        </div>
      ) : choices.isError ? (
        <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
          {errorMessage(t, choices.error)}
        </InlineError>
      ) : choices.data.libraries.length === 0 ? (
        <EmptyState title={t.users.noServerLibraries}>{t.users.noServerLibrariesHelp}</EmptyState>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {choices.data.libraries.map((library) => (
            <Checkbox
              key={library.id}
              label={library.name}
              checked={!hidden.includes(library.id)}
              onChange={(shown) => {
                save.reset()
                setHidden(
                  shown ? hidden.filter((id) => id !== library.id) : [...hidden, library.id],
                )
              }}
            />
          ))}
        </div>
      )}
    </SectionForm>
  )
}

export function BlockedGenresForm({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const { save, error, submit } = useSectionSave(target)
  const [genres, setGenres] = useState(limitsOf(target).blockedGenres)
  const [typed, setTyped] = useState('')

  function add(value = typed) {
    const genre = value.trim()
    save.reset()
    setTyped('')
    if (genre !== '' && !genres.some((g) => g.toLowerCase() === genre.toLowerCase())) {
      setGenres([...genres, genre])
    }
  }

  return (
    <SectionForm
      id={id}
      target={target}
      title={t.users.blockedGenresTitle}
      description={t.users.blockedGenresHelp}
      onSubmit={() => submit({ blockedGenres: genres })}
      error={error}
      saving={save.isPending}
      saveLabel={t.users.saveBlockedGenres}
    >
      {genres.length === 0 ? (
        <p className="text-small text-ink-3">{t.users.noBlockedGenres}</p>
      ) : (
        <ul className="flex flex-wrap gap-2" aria-label={t.users.blockedGenresTitle}>
          {genres.map((genre) => (
            <li
              key={genre}
              className="flex h-8 items-center gap-1 rounded-full border border-line-2 bg-s2 pr-1 pl-3 text-control text-ink"
            >
              {genre}
              <IconButton
                size="sm"
                icon={XIcon}
                danger
                label={t.users.removeGenre(genre)}
                className="size-6 rounded-full"
                onClick={() => {
                  save.reset()
                  setGenres(genres.filter((g) => g !== genre))
                }}
              />
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-start gap-2">
        <Field label={t.users.genre} help={t.users.genreHint} className="w-full sm:max-w-xs">
          <SuggestInput
            value={typed}
            onValue={setTyped}
            suggestions={(choices.data?.genres ?? []).filter(
              (genre) => !genres.some((g) => g.toLowerCase() === genre.toLowerCase()),
            )}
            onPick={add}
            maxLength={100}
            onKeyDown={(event) => {
              // Enter adds the genre rather than saving the list.
              if (event.key === 'Enter') {
                event.preventDefault()
                add()
              }
            }}
          />
        </Field>
        <Button icon={PlusIcon} className="sm:mt-[26px]" onClick={() => add()}>
          {t.users.addGenre}
        </Button>
      </div>
    </SectionForm>
  )
}

/** Every half hour from 00:00 to 24:00, as jellyfin-web offers them. */
const halfHours = Array.from({ length: 49 }, (_, i) => i / 2)

/** "09:30" for 9.5: hours count from midnight and may have fractions. */
function hourLabel(hour: number) {
  const minutes = Math.round(hour * 60)
  return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`
}

export function AllowedHoursForm({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const { save, error, submit } = useSectionSave(target)
  const [schedules, setSchedules] = useState<AccessSchedule[]>(limitsOf(target).accessSchedules)
  const misordered = schedules.some((s) => s.startHour >= s.endHour)

  function change(index: number, changed: Partial<AccessSchedule>) {
    save.reset()
    setSchedules(schedules.map((s, i) => (i === index ? { ...s, ...changed } : s)))
  }

  return (
    <SectionForm
      id={id}
      target={target}
      title={t.users.allowedHoursTitle}
      description={t.users.allowedHoursHelp}
      onSubmit={() => {
        if (!misordered) submit({ accessSchedules: schedules })
      }}
      error={error}
      saving={save.isPending}
      saveLabel={t.users.saveAllowedHours}
      disabled={misordered}
    >
      {schedules.length === 0 ? (
        <p className="text-small text-ink-3">{t.users.noAllowedHours}</p>
      ) : (
        <ul className="space-y-3">
          {schedules.map((schedule, index) => (
            // Rows have no identity of their own; they are edited in place.
            <li
              key={index}
              className="flex flex-wrap items-end gap-3 rounded-row border border-line bg-bg/40 p-3"
            >
              <Field label={t.users.day} className="min-w-[200px] flex-1">
                <Select<ScheduleDay>
                  value={schedule.day}
                  onValue={(day) => change(index, { day })}
                  options={scheduleDays.map((day) => ({ value: day, label: t.users.days[day] }))}
                />
              </Field>
              {(['startHour', 'endHour'] as const).map((field) => (
                <Field
                  key={field}
                  label={field === 'startHour' ? t.users.from : t.users.to}
                  className="w-[112px]"
                >
                  <Select
                    value={schedule[field]}
                    invalid={misordered && schedule.startHour >= schedule.endHour}
                    onValue={(hour) => change(index, { [field]: hour })}
                    options={(halfHours.includes(schedule[field])
                      ? halfHours
                      : [...halfHours, schedule[field]].sort((a, b) => a - b)
                    ).map((hour) => ({ value: hour, label: hourLabel(hour) }))}
                  />
                </Field>
              ))}
              <Button
                variant="ghost"
                icon={TrashIcon}
                onClick={() => {
                  save.reset()
                  setSchedules(schedules.filter((_, i) => i !== index))
                }}
              >
                {t.users.removeHours}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <Button
        icon={PlusIcon}
        onClick={() => {
          save.reset()
          setSchedules([...schedules, { day: 'Everyday', startHour: 8, endHour: 20 }])
        }}
      >
        {t.users.addHours}
      </Button>
      {misordered && <Notice tone="danger">{t.users.hoursOrder}</Notice>}
    </SectionForm>
  )
}

/** The permissions of Access a group holds, each a switch saved as it changes. */
export type Permissions = Pick<
  GroupLimits,
  | 'remoteAccess'
  | 'transcoding'
  | 'downloads'
  | 'personalAddons'
  | 'collectionManagement'
  | 'subtitleManagement'
  | 'canShare'
  | 'torrentStreams'
>

/**
 * The switches of the permissions in Access, on a user's page and on a group's. Remote access off
 * warns when Health sees a reverse proxy missing from Known proxies: everyone coming through it
 * looks local, so the limit does not hold for them.
 */
export function PermissionRows({
  values,
  disabled,
  onChange,
}: {
  values: Permissions
  disabled: boolean
  onChange: (patch: Partial<Permissions>) => void
}) {
  const { t } = useI18n()
  const unknownProxy =
    useHealthProblems().data?.some((problem) => problem.code === 'unknown_proxy') ?? false
  const rows = [
    ['remoteAccess', t.users.remoteAccess, t.users.remoteAccessHelp],
    ['transcoding', t.users.canTranscode, t.users.canTranscodeHelp],
    ['downloads', t.users.canDownload, t.users.canDownloadHelp],
    ['torrentStreams', t.users.canPlayTorrents, t.users.canPlayTorrentsHelp],
    ['personalAddons', t.users.canAddAddons, t.users.canAddAddonsHelp],
    ['collectionManagement', t.users.canManageCollections, t.users.canManageCollectionsHelp],
    ['subtitleManagement', t.users.canManageSubtitles, t.users.canManageSubtitlesHelp],
    ['canShare', t.users.canShare, t.users.canShareHelp],
  ] as const
  return rows.map(([field, title, help]) => (
    <Fragment key={field}>
      <SwitchRow
        title={title}
        help={help}
        checked={values[field]}
        disabled={disabled}
        onChange={(checked) => onChange({ [field]: checked })}
      />
      {field === 'remoteAccess' && !values.remoteAccess && unknownProxy && (
        <div className="pb-4">
          <Notice
            tone="warn"
            action={
              <TextLink to="/settings/security#known-proxies">{t.users.openKnownProxies}</TextLink>
            }
          >
            {t.users.remoteAccessUnknownProxy}
          </Notice>
        </div>
      )}
    </Fragment>
  ))
}

/** Whether a and b hold the same values, in any order. */
function sameSet(a: readonly string[], b: readonly string[]) {
  return a.length === b.length && a.every((value) => b.includes(value))
}

/**
 * What Reset to defaults changes, in the order of the page: each section whose values change, with
 * the settings that change in it for Access and Playback and access, whose settings are many.
 */
function resetChanges(t: Messages, current: GroupLimits, defaults: GroupLimits) {
  const access = (
    [
      ['remoteAccess', t.users.remoteAccess],
      ['transcoding', t.users.canTranscode],
      ['downloads', t.users.canDownload],
      ['torrentStreams', t.users.canPlayTorrents],
      ['personalAddons', t.users.canAddAddons],
      ['collectionManagement', t.users.canManageCollections],
      ['subtitleManagement', t.users.canManageSubtitles],
      ['canShare', t.users.canShare],
    ] as const
  ).filter(([field]) => current[field] !== defaults[field])
  const playback = (
    [
      ['maxPlaybacks', t.users.maxPlaybacks],
      ['maxBitrate', t.users.maxBitrate],
      ['qualityGroup', t.users.qualityGroup],
      ['syncPlay', t.users.syncPlay],
      ['liveTv', t.users.liveTv],
      ['remoteControl', t.users.remoteControl],
      ['liveTvManagement', t.users.liveTvManagement],
    ] as const
  ).filter(([field]) => current[field] !== defaults[field])
  const parental = current.parentalControl
  const sections: { title: string; settings: string[]; changed: boolean }[] = [
    {
      title: t.users.accessTitle,
      settings: access.map(([, label]) => label),
      changed: access.length > 0,
    },
    {
      title: t.users.playbackAccessTitle,
      settings: playback.map(([, label]) => label),
      changed: playback.length > 0,
    },
    {
      title: t.users.parentalTitle,
      settings: [],
      changed:
        parental.maxRating !== defaults.parentalControl.maxRating ||
        parental.maxSubRating !== defaults.parentalControl.maxSubRating ||
        !sameSet(parental.blockUnrated, defaults.parentalControl.blockUnrated),
    },
    {
      title: t.users.visibleLibrariesTitle,
      settings: [],
      changed: !sameSet(current.hiddenLibraries, defaults.hiddenLibraries),
    },
    {
      title: t.users.blockedGenresTitle,
      settings: [],
      changed: !sameSet(current.blockedGenres, defaults.blockedGenres),
    },
    {
      title: t.users.allowedHoursTitle,
      settings: [],
      changed: JSON.stringify(current.accessSchedules) !== JSON.stringify(defaults.accessSchedules),
    },
  ]
  return sections.filter((section) => section.changed)
}

/**
 * Reset to defaults, on a user's page or a group's: asks first, saying what stays and listing
 * what changes, then puts the sections that set limits back to a new user's settings. A group's
 * says how many members follow it, as the change reaches them. `onReset` is told once it is done,
 * for the page to draw its forms again from the new values.
 */
export function ResetDefaultsDialog({
  target,
  open,
  onClose,
  onReset,
}: {
  target: LimitsTarget
  open: boolean
  onClose: () => void
  onReset: () => void
}) {
  const { t } = useI18n()
  const text = t.resetDefaults
  const toast = useToast()
  const self = useSessionUser()
  const id = target.kind === 'group' ? target.group.id : target.user.id
  const name = target.kind === 'group' ? target.group.name : target.user.name
  const defaults = useQuery({
    queryKey: target.kind === 'group' ? queryKeys.groupDefaults(id) : queryKeys.userDefaults(id),
    queryFn: ({ signal }) =>
      target.kind === 'group' ? fetchGroupDefaults(id, signal) : fetchUserDefaults(id, signal),
    enabled: open,
  })
  // The members who follow the group take its values at once.
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
    enabled: open && target.kind === 'group',
  })
  const followers = (users.data ?? []).filter(
    (user) => user.groupId === id && user.followGroup,
  ).length
  const reset = useMutation({
    mutationFn: async () => {
      if (target.kind === 'group') groupSaved(await resetGroup(id))
      else userSaved(await resetUser(id), self.id)
    },
    onSuccess: () => {
      if (target.kind === 'user') {
        void queryClient.invalidateQueries({ queryKey: queryKeys.userShareLinks(id) })
      }
      onReset()
      onClose()
      toast(target.kind === 'group' ? text.groupDone(name) : text.done(name))
    },
    onError: target.kind === 'group' ? refreshGroupIfGone : refreshIfGone,
  })
  const changes =
    defaults.data === undefined ? null : resetChanges(t, limitsOf(target), defaults.data)

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        reset.reset()
        onClose()
      }}
      onConfirm={() => reset.mutate()}
      title={target.kind === 'group' ? text.groupTitle(name) : text.title(name)}
      confirmLabel={reset.isPending ? text.resetting : text.confirm}
      busy={reset.isPending}
      error={reset.isError ? errorMessage(t, reset.error) : undefined}
    >
      <div className="space-y-3">
        <p>{target.kind === 'group' ? text.groupHelp : text.userHelp(name)}</p>
        {target.kind === 'user' && target.user.groupName !== null && (
          <p>{text.ownSettingsHelp(target.user.groupName)}</p>
        )}
        {target.kind === 'group' && users.isSuccess && <p>{text.followers(followers)}</p>}
        {defaults.isError ? (
          <p className="text-danger">{errorMessage(t, defaults.error)}</p>
        ) : changes === null ? (
          <p className="text-ink-3">{text.loading}</p>
        ) : changes.length === 0 ? (
          <p>{text.noChange}</p>
        ) : (
          <div>
            <p className="font-medium text-ink">{text.changesTitle}</p>
            <ul className="mt-1 list-disc space-y-0.5 ps-5">
              {changes.map((change) => (
                <li key={change.title}>
                  <span className="text-ink">{change.title}</span>
                  {change.settings.length > 0 && `: ${change.settings.join(', ')}`}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </ConfirmDialog>
  )
}
