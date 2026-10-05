import { useQuery } from '@tanstack/react-query'
import { PlusIcon, TrashIcon, XIcon } from '@phosphor-icons/react'
import { useId, useState, type FormEvent, type ReactNode } from 'react'
import {
  fetchUserContentChoices,
  maxPlaybacksRange,
  qualityGroups,
  queryKeys,
  scheduleDays,
  type AccessSchedule,
  type QualityGroup,
  type ScheduleDay,
  type SyncPlayAccess,
  type User,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
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
  TextInput,
  useToast,
} from '@/ui'
import {
  groupRatings,
  qualityGroupName,
  selectedGroup,
  SwitchRow,
  useParentalRatings,
  useUserPatch,
} from './shared'

/**
 * A section of a user's page: a panel holding one form, its error and its save button at the
 * foot. Saving shows a toast.
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
  children: ReactNode
}) {
  const { t } = useI18n()
  return (
    <form
      id={id}
      noValidate
      className="scroll-mt-[calc(var(--spacing-topbar)+32px)] max-md:scroll-mt-[calc(var(--spacing-topbar)+64px)]"
      onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault()
        onSubmit()
      }}
    >
      <Panel
        title={title}
        description={description}
        footer={
          <PanelFooter>
            <Button variant="secondary" type="submit" loading={saving} disabled={disabled}>
              {saving ? t.common.saving : saveLabel}
            </Button>
          </PanelFooter>
        }
      >
        <div className="space-y-5">
          {children}
          {error !== null && <Notice tone="danger">{error}</Notice>}
        </div>
      </Panel>
    </form>
  )
}

/** A patch for one section, with its toast and inline error. */
function useSectionSave(user: User) {
  const { t } = useI18n()
  const toast = useToast()
  const save = useUserPatch(user)
  return {
    save,
    error: save.isError ? errorMessage(t, save.error) : null,
    submit: (patch: Parameters<typeof save.mutate>[0]) =>
      save.mutate(patch, { onSuccess: () => toast(t.users.updated) }),
  }
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

export function PlaybackAccessForm({ user, id }: { user: User; id: string }) {
  const { t, language } = useI18n()
  const { save, error, submit } = useSectionSave(user)
  const [form, setForm] = useState({
    maxPlaybacks: user.maxPlaybacks,
    maxBitrate: user.maxBitrate,
    liveTv: user.liveTv,
    syncPlay: user.syncPlay,
    remoteControl: user.remoteControl,
    liveTvManagement: user.liveTvManagement,
    qualityGroup: user.qualityGroup,
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

export function ParentalControlForm({ user, id }: { user: User; id: string }) {
  const { t } = useI18n()
  const ratings = useParentalRatings()
  const { save, error, submit } = useSectionSave(user)
  const initial = user.parentalControl
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
    const blockUnrated = user.parentalControl.blockUnrated.filter(
      (item) => item !== unratedMovie && item !== unratedSeries,
    )
    if (blockMovies) blockUnrated.push(unratedMovie)
    if (blockShows) blockUnrated.push(unratedSeries)
    submit({ parentalControl: { ...limit, blockUnrated } })
  }

  return (
    <SectionForm
      id={id}
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

export function VisibleLibrariesForm({ user, id }: { user: User; id: string }) {
  const { t } = useI18n()
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const { save, error, submit } = useSectionSave(user)
  const [hidden, setHidden] = useState(user.hiddenLibraries)

  function onSubmit() {
    const libraries = choices.data?.libraries ?? []
    // Libraries removed from the server since are dropped.
    submit({ hiddenLibraries: hidden.filter((id) => libraries.some((l) => l.id === id)) })
  }

  return (
    <SectionForm
      id={id}
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

export function BlockedGenresForm({ user, id }: { user: User; id: string }) {
  const { t } = useI18n()
  const listId = useId()
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const { save, error, submit } = useSectionSave(user)
  const [genres, setGenres] = useState(user.blockedGenres)
  const [typed, setTyped] = useState('')

  function add() {
    const genre = typed.trim()
    save.reset()
    setTyped('')
    if (genre !== '' && !genres.some((g) => g.toLowerCase() === genre.toLowerCase())) {
      setGenres([...genres, genre])
    }
  }

  return (
    <SectionForm
      id={id}
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
          <TextInput
            value={typed}
            onValue={setTyped}
            list={listId}
            maxLength={100}
            autoComplete="off"
            onKeyDown={(event) => {
              // Enter adds the genre rather than saving the list.
              if (event.key === 'Enter') {
                event.preventDefault()
                add()
              }
            }}
          />
        </Field>
        <Button icon={PlusIcon} className="sm:mt-[26px]" onClick={add}>
          {t.users.addGenre}
        </Button>
        <datalist id={listId}>
          {(choices.data?.genres ?? []).map((genre) => (
            <option key={genre} value={genre} />
          ))}
        </datalist>
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

export function AllowedHoursForm({ user, id }: { user: User; id: string }) {
  const { t } = useI18n()
  const { save, error, submit } = useSectionSave(user)
  const [schedules, setSchedules] = useState<AccessSchedule[]>(user.accessSchedules)
  const misordered = schedules.some((s) => s.startHour >= s.endHour)

  function change(index: number, changed: Partial<AccessSchedule>) {
    save.reset()
    setSchedules(schedules.map((s, i) => (i === index ? { ...s, ...changed } : s)))
  }

  return (
    <SectionForm
      id={id}
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
