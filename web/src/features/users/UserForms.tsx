import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { PlusIcon, TrashIcon, XIcon } from '@phosphor-icons/react'
import { Fragment, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  fetchGroup,
  fetchGroupDefaults,
  fetchLineupChannelChoices,
  fetchUserContentChoices,
  fetchUserDefaults,
  fetchUsers,
  groupSections,
  maxPlaybacksRange,
  qualityGroups,
  queryClient,
  queryKeys,
  resetGroup,
  resetUser,
  scheduleDays,
  type AccessSchedule,
  type ChannelRule,
  type Group,
  type GroupLimits,
  type GroupPatch,
  type GroupSection,
  type QualityGroup,
  type ScheduleDay,
  type SyncPlayAccess,
  type User,
} from '@/api'
import { useDebounced } from '@/features/iptv/shared'
import { useSessionUser } from '@/app/session'
import { useHealthProblems } from '@/features/system/problems'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  IconButton,
  InlineError,
  Notice,
  Select,
  SettingRow,
  Skeleton,
  SuggestInput,
  TextInput,
  TextLink,
  useToast,
} from '@/ui'
import { changedFields, countChanged, setDifference, useDraftPart } from './draft'
import { PageSection } from './rows'
import {
  groupRatings,
  groupSaved,
  qualityGroupName,
  refreshGroupIfGone,
  refreshIfGone,
  selectedGroup,
  useParentalRatings,
  userSaved,
  withLimit,
} from './shared'

/**
 * Whose limits a section edits: a user's, or a group's, which its following members take at once.
 */
export type LimitsTarget = { kind: 'user'; user: User } | { kind: 'group'; group: Group }

/** The group of a member, whose values they take in the sections they follow; null for none. */
export function memberGroup(user: User): { id: string; name: string } | null {
  return user.groupId === null ? null : { id: user.groupId, name: user.groupName ?? '' }
}

/** The group whose values a member follows in a section, shown read only there; null otherwise. */
export function followedGroup(
  user: User,
  section: GroupSection,
): { id: string; name: string } | null {
  return user.ownSections.includes(section) ? null : memberGroup(user)
}

/** The sections a member keeps as own, with one section kept or following the group again. */
export function withOwnSection(
  ownSections: readonly GroupSection[],
  section: GroupSection,
  own: boolean,
): GroupSection[] {
  return groupSections.filter((item) => (item === section ? own : ownSections.includes(item)))
}

/** The title of a section a group holds, as the pages name it. */
export function sectionTitle(t: Messages, section: GroupSection): string {
  const titles: Record<GroupSection, string> = {
    access: t.users.accessTitle,
    playback: t.users.playbackAccessTitle,
    parental: t.users.parentalTitle,
    libraries: t.users.visibleLibrariesTitle,
    genres: t.users.blockedGenresTitle,
    hours: t.users.allowedHoursTitle,
    channels: t.users.liveTvChannelsTitle,
  }
  return titles[section]
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
 * Values, From the group Kids or Own for this user: whether a member follows their group in a
 * section or keeps their own values there.
 */
export function SectionValuesRow({
  group,
  own,
  saved,
  onChange,
}: {
  group: { name: string }
  own: boolean
  /** The saved choice, for "was …"; undefined where the choice saves at once. */
  saved?: boolean
  onChange: (own: boolean) => void
}) {
  const { t } = useI18n()
  const value = (own: boolean) => (own ? 'own' : 'group')
  return (
    <SettingRow<'group' | 'own'>
      kind="choice"
      label={t.groups.values}
      help={t.groups.valuesHelp}
      value={value(own)}
      saved={saved === undefined ? undefined : value(saved)}
      options={[
        { value: 'group', label: t.groups.valuesFromGroup(group.name) },
        { value: 'own', label: t.groups.valuesOwn },
      ]}
      onChange={(choice) => onChange(choice === 'own')}
    />
  )
}

/** The sections LimitsSection draws, each with the fields it holds. */
const sectionFields = {
  playback: [
    'maxPlaybacks',
    'maxBitrate',
    'qualityGroup',
    'syncPlay',
    'liveTv',
    'remoteControl',
    'liveTvManagement',
    'localFirst',
    'boostVolume',
    'convertSound',
  ],
  parental: ['parentalControl'],
  libraries: ['hiddenLibraries'],
  genres: ['blockedGenres'],
  hours: ['accessSchedules'],
  channels: ['liveTvChannels'],
} satisfies Record<Exclude<GroupSection, 'access'>, (keyof GroupLimits)[]>

/**
 * What the rows of a section show: the values they start from, the saved ones that mark a changed
 * row with "was …" (undefined to mark none, and count no change), and whether they are read only.
 */
type SectionView = { values: GroupLimits; saved: GroupLimits | undefined; readOnly: boolean }

/**
 * Tells the dock what a section's rows change: nothing while they show the group's values a
 * member follows; while a member goes back to the group, the rows differing from their own
 * values, without sending them, as the group's replace them.
 */
function useSectionDraft(
  id: string,
  view: SectionView,
  changes: number,
  patch: () => GroupPatch | null,
) {
  useDraftPart(id, view.saved === undefined ? 0 : changes, () => (view.readOnly ? {} : patch()))
}

/**
 * A section of limits on a user's or a group's page: its title and its rows, whose changes go to
 * the page's dock. On a member's page it starts with Values, From the group or Own for this user,
 * itself a change in the dock: following the group, the rows show its values read only, under a
 * notice saying so; going back to it shows the group's values against the member's own.
 */
function LimitsSection({
  id,
  section,
  title,
  description,
  target,
  children,
}: {
  id: string
  section: keyof typeof sectionFields
  title: string
  description?: ReactNode
  target: LimitsTarget
  children: (view: SectionView) => ReactNode
}) {
  const { t } = useI18n()
  const limits = limitsOf(target)
  const member = target.kind === 'user' ? target.user : null
  const group = member === null ? null : memberGroup(member)
  const savedOwn = member === null || group === null || member.ownSections.includes(section)
  const [own, setOwn] = useState(savedOwn)
  const back = savedOwn && !own
  const groupValues = useQuery({
    queryKey: queryKeys.group(group?.id ?? ''),
    queryFn: ({ signal }) => fetchGroup(group?.id ?? '', signal),
    enabled: group !== null && back,
  })
  useDraftPart(`${id}-values`, own === savedOwn ? 0 : 1, () =>
    member === null ? {} : { ownSections: withOwnSection(member.ownSections, section, own) },
  )

  const view: SectionView | null = own
    ? { values: limits, saved: limits, readOnly: false }
    : !back
      ? { values: limits, saved: undefined, readOnly: true }
      : groupValues.data === undefined
        ? null
        : { values: groupValues.data, saved: limits, readOnly: true }
  // The rows start again from their values when the choice changes, and, read only, when the
  // group's values reach the member.
  const key = own
    ? 'own'
    : back
      ? 'back'
      : JSON.stringify(sectionFields[section].map((field) => limits[field]))

  return (
    <PageSection id={id} title={title} description={description}>
      {group !== null && (
        <SectionValuesRow group={group} own={own} saved={savedOwn} onChange={setOwn} />
      )}
      {group !== null && !own && <FollowedNotice group={group} />}
      {view !== null ? (
        <Fragment key={key}>{children(view)}</Fragment>
      ) : groupValues.isError ? (
        <InlineError onRetry={() => void groupValues.refetch()} retrying={groupValues.isFetching}>
          {errorMessage(t, groupValues.error)}
        </InlineError>
      ) : (
        <Skeleton className="h-24 rounded-row" />
      )}
    </PageSection>
  )
}

/** A tile for what is more than a row (chips, lists to tick), as wide as the rows. */
const tile = 'min-w-0 rounded-row bg-s1 px-4 py-3'

/** A chip of a list (a genre, a channel) with its button taking it out. */
function Chip({
  label,
  title,
  removeLabel,
  onClick,
}: {
  label: string
  title?: string
  removeLabel: string
  onClick?: () => void
}) {
  return (
    <li
      title={title}
      className="flex min-h-9 max-w-full items-center gap-0.5 rounded-full bg-s2 py-0.5 pr-0.5 pl-3.5 text-control text-ink"
    >
      <span className="min-w-0 break-words">{label}</span>
      <IconButton
        size="sm"
        icon={XIcon}
        danger
        label={removeLabel}
        className="rounded-full"
        onClick={onClick}
      />
    </li>
  )
}

/** Keeps Enter in a filter or search field from saving the page. */
function holdEnter(event: KeyboardEvent<HTMLInputElement>) {
  if (event.key === 'Enter') event.preventDefault()
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

function playbackOf(limits: GroupLimits) {
  return {
    maxPlaybacks: limits.maxPlaybacks,
    maxBitrate: limits.maxBitrate,
    qualityGroup: limits.qualityGroup,
    syncPlay: limits.syncPlay,
    liveTv: limits.liveTv,
    remoteControl: limits.remoteControl,
    liveTvManagement: limits.liveTvManagement,
    localFirst: limits.localFirst,
    boostVolume: limits.boostVolume,
    convertSound: limits.convertSound,
  }
}

export function PlaybackAccessSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection id={id} section="playback" target={target} title={t.users.playbackAccessTitle}>
      {(view) => <PlaybackAccessRows id={id} view={view} />}
    </LimitsSection>
  )
}

function PlaybackAccessRows({ id, view }: { id: string; view: SectionView }) {
  const { t, language } = useI18n()
  const { readOnly } = view
  const saved = playbackOf(view.saved ?? view.values)
  const [form, setForm] = useState(() => playbackOf(view.values))
  useSectionDraft(id, view, countChanged(form, saved), () => changedFields(form, saved))
  // The saved values give each changed row its "was …"; rows following the group mark none.
  const was = view.saved === undefined ? undefined : saved

  function update(change: Partial<typeof form>) {
    setForm((current) => ({ ...current, ...change }))
  }

  // A limit set from a Jellyfin app may match none of the choices: it is shown as it is.
  const bitrates: { value: number; label: string }[] = bitrateChoices.map((choice) => ({
    value: choice.value,
    label: t.users[choice.label],
  }))
  for (const value of new Set([saved.maxBitrate, form.maxBitrate])) {
    if (!bitrates.some((choice) => choice.value === value)) {
      const mbits = (value / 1_000_000).toLocaleString(language, { maximumFractionDigits: 2 })
      bitrates.push({ value, label: t.users.bitrateOther(mbits) })
    }
  }

  const switches = [
    ['liveTv', t.users.liveTv, t.users.liveTvHelp],
    ['remoteControl', t.users.remoteControl, t.users.remoteControlHelp],
    ['liveTvManagement', t.users.liveTvManagement, t.users.liveTvManagementHelp],
    ['localFirst', t.users.localFirst, t.users.localFirstHelp],
    ['boostVolume', t.users.boostVolume, t.users.boostVolumeHelp],
    ['convertSound', t.users.convertSound, t.users.convertSoundHelp],
  ] as const

  return (
    <>
      <SettingRow
        kind="number"
        label={t.users.maxPlaybacks}
        help={t.users.maxPlaybacksHelp}
        value={form.maxPlaybacks}
        saved={was?.maxPlaybacks}
        min={maxPlaybacksRange.min}
        max={maxPlaybacksRange.max}
        format={(value) => (value === 0 ? t.users.noLimit : String(value))}
        disabled={readOnly}
        onChange={(value) => update({ maxPlaybacks: Math.trunc(value ?? 0) })}
      />
      <SettingRow<number>
        kind="choice"
        label={t.users.maxBitrate}
        help={t.users.maxBitrateHelp}
        value={form.maxBitrate}
        saved={was?.maxBitrate}
        options={bitrates}
        disabled={readOnly}
        onChange={(maxBitrate) => update({ maxBitrate })}
      />
      <SettingRow<number>
        kind="choice"
        label={t.users.qualityGroup}
        help={t.users.qualityGroupHelp}
        value={form.qualityGroup}
        saved={was?.qualityGroup}
        options={qualityGroups.map((group) => ({
          value: group as number,
          label: group === 0 ? t.users.qualityGroupOriginal : qualityGroupName(group),
        }))}
        disabled={readOnly}
        onChange={(qualityGroup) => update({ qualityGroup: qualityGroup as QualityGroup })}
      />
      <SettingRow<SyncPlayAccess>
        kind="choice"
        label={t.users.syncPlay}
        help={t.users.syncPlayHelp}
        value={form.syncPlay}
        saved={was?.syncPlay}
        options={[
          { value: 'CreateAndJoinGroups', label: t.users.syncPlayCreateAndJoin },
          { value: 'JoinGroups', label: t.users.syncPlayJoin },
          { value: 'None', label: t.users.syncPlayNone },
        ]}
        disabled={readOnly}
        onChange={(syncPlay) => update({ syncPlay })}
      />
      {switches.map(([field, label, help]) => (
        <SettingRow
          key={field}
          kind="switch"
          label={label}
          help={help}
          checked={form[field]}
          saved={was?.[field]}
          disabled={readOnly}
          onChange={(checked) => update({ [field]: checked })}
        />
      ))}
    </>
  )
}

const unratedMovie = 'Movie'
const unratedSeries = 'Series'

export function ParentalControlSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection id={id} section="parental" target={target} title={t.users.parentalTitle}>
      {(view) => <ParentalControlRows id={id} view={view} />}
    </LimitsSection>
  )
}

function ParentalControlRows({ id, view }: { id: string; view: SectionView }) {
  const { t } = useI18n()
  const ratings = useParentalRatings()
  const { readOnly } = view
  const marked = view.saved !== undefined
  const start = view.values.parentalControl
  const initial = (view.saved ?? view.values).parentalControl
  const savedMovies = initial.blockUnrated.includes(unratedMovie)
  const savedShows = initial.blockUnrated.includes(unratedSeries)
  const [limit, setLimit] = useState<{ maxRating: number | null; maxSubRating: number | null }>({
    maxRating: start.maxRating,
    maxSubRating: start.maxSubRating,
  })
  const [blockMovies, setBlockMovies] = useState(start.blockUnrated.includes(unratedMovie))
  const [blockShows, setBlockShows] = useState(start.blockUnrated.includes(unratedSeries))

  // A saved limit the rating country has no rating for stays a choice, named by its age.
  const groups =
    ratings.data === undefined
      ? []
      : withLimit(
          withLimit(groupRatings(ratings.data), initial, t.users.ratingAge),
          start,
          t.users.ratingAge,
        )
  const selected = selectedGroup(groups, { ...limit, blockUnrated: [] })
  const savedSelected = selectedGroup(groups, initial)

  const changes =
    Number(limit.maxRating !== initial.maxRating || limit.maxSubRating !== initial.maxSubRating) +
    Number(blockMovies !== savedMovies) +
    Number(blockShows !== savedShows)
  useSectionDraft(id, view, changes, () => {
    // Keep values set by Jellyfin apps (live TV, books...) that this section does not show.
    const blockUnrated = initial.blockUnrated.filter(
      (item) => item !== unratedMovie && item !== unratedSeries,
    )
    if (blockMovies) blockUnrated.push(unratedMovie)
    if (blockShows) blockUnrated.push(unratedSeries)
    return { parentalControl: { ...limit, blockUnrated } }
  })

  return (
    <>
      {ratings.isError ? (
        <InlineError onRetry={() => void ratings.refetch()} retrying={ratings.isFetching}>
          {errorMessage(t, ratings.error)}
        </InlineError>
      ) : ratings.isPending ? (
        <Skeleton className="h-15 rounded-row" />
      ) : (
        <SettingRow<number>
          kind="choice"
          label={t.users.maxRating}
          help={t.users.maxRatingHelp}
          value={selected}
          saved={marked ? savedSelected : undefined}
          options={[
            { value: -1, label: t.users.noLimit },
            ...groups.map((group, index) => ({ value: index, label: group.name })),
          ]}
          disabled={readOnly}
          onChange={(index) => {
            const group = groups[index]
            setLimit(
              group === undefined
                ? { maxRating: null, maxSubRating: null }
                : { maxRating: group.score, maxSubRating: group.subScore },
            )
          }}
        />
      )}
      <SettingRow
        kind="switch"
        label={t.users.blockUnratedMovies}
        checked={blockMovies}
        saved={marked ? savedMovies : undefined}
        disabled={readOnly}
        onChange={setBlockMovies}
      />
      <SettingRow
        kind="switch"
        label={t.users.blockUnratedShows}
        checked={blockShows}
        saved={marked ? savedShows : undefined}
        disabled={readOnly}
        onChange={setBlockShows}
      />
    </>
  )
}

export function VisibleLibrariesSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection
      id={id}
      section="libraries"
      target={target}
      title={t.users.visibleLibrariesTitle}
      description={t.users.visibleLibrariesHelp}
    >
      {(view) => <VisibleLibrariesRows id={id} view={view} />}
    </LimitsSection>
  )
}

function VisibleLibrariesRows({ id, view }: { id: string; view: SectionView }) {
  const { t } = useI18n()
  const { readOnly } = view
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const savedHidden = (view.saved ?? view.values).hiddenLibraries
  const [hidden, setHidden] = useState(view.values.hiddenLibraries)
  useSectionDraft(id, view, setDifference(hidden, savedHidden), () => {
    if (!choices.isSuccess) return null
    // Libraries removed from the server since are dropped.
    const libraries = choices.data.libraries
    return { hiddenLibraries: hidden.filter((id) => libraries.some((l) => l.id === id)) }
  })

  return (
    <>
      {choices.isPending ? (
        <Skeleton className="h-24 rounded-row" />
      ) : choices.isError ? (
        <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
          {errorMessage(t, choices.error)}
        </InlineError>
      ) : choices.data.libraries.length === 0 ? (
        <EmptyState title={t.users.noServerLibraries}>{t.users.noServerLibrariesHelp}</EmptyState>
      ) : (
        // A checklist in two columns: a library is a line to tick, shown or hidden in the apps.
        <div
          role="group"
          aria-labelledby={`${id}-title`}
          className="grid min-w-0 gap-x-6 rounded-row bg-s1 px-4 py-2 sm:grid-cols-2"
        >
          {choices.data.libraries.map((library) => {
            const shown = !hidden.includes(library.id)
            const savedShown = !savedHidden.includes(library.id)
            return (
              <Checkbox
                key={library.id}
                label={library.name}
                checked={shown}
                disabled={readOnly}
                onChange={(on) =>
                  setHidden(on ? hidden.filter((id) => id !== library.id) : [...hidden, library.id])
                }
                aside={
                  view.saved !== undefined && shown !== savedShown ? (
                    <span className="inline-flex items-center gap-1.5 text-micro">
                      <span aria-hidden="true" className="size-1.5 rounded-full bg-link" />
                      {t.ui.was(savedShown ? t.users.libraryShown : t.users.libraryHidden)}
                    </span>
                  ) : undefined
                }
              />
            )
          })}
        </div>
      )}
    </>
  )
}

export function BlockedGenresSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection
      id={id}
      section="genres"
      target={target}
      title={t.users.blockedGenresTitle}
      description={t.users.blockedGenresHelp}
    >
      {(view) => <BlockedGenresRows id={id} view={view} />}
    </LimitsSection>
  )
}

function BlockedGenresRows({ id, view }: { id: string; view: SectionView }) {
  const { t } = useI18n()
  const { readOnly } = view
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const savedGenres = (view.saved ?? view.values).blockedGenres
  const [genres, setGenres] = useState(view.values.blockedGenres)
  const [typed, setTyped] = useState('')
  useSectionDraft(id, view, setDifference(genres, savedGenres), () => ({
    blockedGenres: genres,
  }))

  function add(value = typed) {
    const genre = value.trim()
    setTyped('')
    if (genre !== '' && !genres.some((g) => g.toLowerCase() === genre.toLowerCase())) {
      setGenres([...genres, genre])
    }
  }

  return (
    <fieldset disabled={readOnly} className={cx(tile, 'space-y-5')}>
      {genres.length === 0 ? (
        <p className="text-body text-ink-2">{t.users.noBlockedGenres}</p>
      ) : (
        <ul className="flex flex-wrap gap-2" aria-label={t.users.blockedGenresTitle}>
          {genres.map((genre) => (
            <Chip
              key={genre}
              label={genre}
              removeLabel={t.users.removeGenre(genre)}
              onClick={() => setGenres(genres.filter((g) => g !== genre))}
            />
          ))}
        </ul>
      )}
      {!readOnly && (
        <div className="flex flex-wrap items-start gap-3">
          <Field
            label={t.users.genre}
            help={t.users.genreHint}
            className="w-full sm:max-w-[28.5rem]"
          >
            <SuggestInput
              size="sm"
              value={typed}
              onValue={setTyped}
              suggestions={(choices.data?.genres ?? []).filter(
                (genre) => !genres.some((g) => g.toLowerCase() === genre.toLowerCase()),
              )}
              onPick={add}
              maxLength={100}
              onKeyDown={(event) => {
                // Enter adds the genre rather than saving the page.
                if (event.key === 'Enter') {
                  event.preventDefault()
                  add()
                }
              }}
            />
          </Field>
          <Button icon={PlusIcon} className="sm:mt-[1.875rem]" onClick={() => add()}>
            {t.users.addGenre}
          </Button>
        </div>
      )}
    </fieldset>
  )
}

/** How many groups and channels the Live TV channels section lists at once; filters reach the rest. */
const listedChannelGroups = 200
const listedChannels = 50

export function LiveTvChannelsSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection
      id={id}
      section="channels"
      target={target}
      title={t.users.liveTvChannelsTitle}
      description={t.users.liveTvChannelsHelp}
    >
      {(view) => <LiveTvChannelsRows id={id} view={view} />}
    </LimitsSection>
  )
}

function LiveTvChannelsRows({ id, view }: { id: string; view: SectionView }) {
  const { t } = useI18n()
  const { readOnly } = view
  const initial = view.values.liveTvChannels
  const saved = (view.saved ?? view.values).liveTvChannels
  const [mode, setMode] = useState<ChannelRule['mode']>(initial.mode)
  const [groups, setGroups] = useState(initial.groups)
  const [channels, setChannels] = useState(initial.channels)
  const [hideNew, setHideNew] = useState(initial.hideNewGroups)
  // The groups added since the rule was saved, which it hides while it hides new groups: shown
  // ticked, unless unticked here.
  const [released, setReleased] = useState<string[]>([])
  const [groupFilter, setGroupFilter] = useState('')
  const [search, setSearch] = useState('')
  const query = useDebounced(search.trim())
  const choices = useQuery({
    queryKey: queryKeys.lineupChannelChoices(query, []),
    queryFn: ({ signal }) => fetchLineupChannelChoices({ q: query, limit: listedChannels }, signal),
    placeholderData: keepPreviousData,
  })
  const tickedKeys = channels.slice(0, listedChannels)
  const ticked = useQuery({
    queryKey: queryKeys.lineupChannelChoices('', tickedKeys),
    queryFn: ({ signal }) =>
      fetchLineupChannelChoices({ keys: tickedKeys, limit: listedChannels }, signal),
    enabled: tickedKeys.length > 0,
    placeholderData: keepPreviousData,
  })
  const added = (key: string) =>
    mode === 'except' &&
    initial.mode === 'except' &&
    initial.hideNewGroups &&
    !initial.knownGroups.includes(key)
  const allGroups = choices.data?.groups ?? []
  const groupNames = new Map(allGroups.map((group) => [group.key, group.name]))
  const filter = groupFilter.trim().toLowerCase()
  const filtered = allGroups.filter(
    (group) =>
      filter === '' ||
      group.name.toLowerCase().includes(filter) ||
      group.source.toLowerCase().includes(filter),
  )
  const tickedNames = new Map((ticked.data?.channels ?? []).map((c) => [c.key, c]))

  const changes =
    Number(mode !== saved.mode) +
    (mode === 'all'
      ? 0
      : setDifference(groups, saved.groups) +
        setDifference(channels, saved.channels) +
        released.length +
        Number(mode === 'except' && hideNew !== saved.hideNewGroups))
  useSectionDraft(id, view, changes, () => {
    if (mode !== 'all' && !choices.isSuccess) return null
    const kept =
      mode === 'except'
        ? [
            ...groups,
            ...allGroups
              .map((group) => group.key)
              .filter((key) => added(key) && !released.includes(key) && !groups.includes(key)),
          ]
        : groups
    return {
      liveTvChannels: {
        mode,
        groups: mode === 'all' ? [] : kept,
        channels: mode === 'all' ? [] : channels,
        hideNewGroups: mode === 'except' && hideNew,
        knownGroups: initial.knownGroups,
      },
    }
  })

  function tickGroup(key: string, on: boolean) {
    if (added(key) && !groups.includes(key)) {
      setReleased(on ? released.filter((k) => k !== key) : [...released, key])
      return
    }
    setGroups(on ? [...groups, key] : groups.filter((k) => k !== key))
  }

  function tickChannel(key: string, on: boolean) {
    setChannels(on ? [...channels, key] : channels.filter((k) => k !== key))
  }

  return (
    <>
      <SettingRow<ChannelRule['mode']>
        kind="choice"
        label={t.users.channelsMode}
        help={
          mode === 'only'
            ? t.users.channelsOnlyHelp
            : mode === 'except'
              ? t.users.channelsExceptHelp
              : undefined
        }
        value={mode}
        saved={view.saved === undefined ? undefined : saved.mode}
        options={[
          { value: 'all', label: t.users.channelsAll },
          { value: 'only', label: t.users.channelsOnly },
          { value: 'except', label: t.users.channelsExcept },
        ]}
        disabled={readOnly}
        onChange={setMode}
      />
      {mode !== 'all' &&
        (choices.isError ? (
          <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
            {errorMessage(t, choices.error)}
          </InlineError>
        ) : choices.isPending ? (
          <Skeleton className="h-40 rounded-row" />
        ) : allGroups.length === 0 ? (
          <EmptyState title={t.users.noChannelGroups} />
        ) : (
          <fieldset disabled={readOnly} className={cx(tile, 'space-y-5')}>
            {mode === 'except' && (
              <Checkbox
                label={t.users.hideNewChannelGroups}
                help={t.users.hideNewChannelGroupsHelp}
                checked={hideNew}
                onChange={setHideNew}
              />
            )}
            <div className="space-y-3">
              <h3 className="text-h3 text-ink">{t.users.channelGroups}</h3>
              <TextInput
                size="sm"
                aria-label={t.users.filterChannelGroups}
                placeholder={t.users.filterChannelGroups}
                value={groupFilter}
                onValue={setGroupFilter}
                onKeyDown={holdEnter}
                className="sm:max-w-[28.5rem]"
              />
              <div className="grid max-h-96 gap-x-4 gap-y-1 overflow-y-auto sm:grid-cols-2">
                {filtered.slice(0, listedChannelGroups).map((group) => (
                  <Checkbox
                    key={group.key}
                    label={group.name}
                    help={group.source}
                    aside={
                      <span className="flex items-center gap-2">
                        {added(group.key) && <Badge tone="accent">{t.users.newChannelGroup}</Badge>}
                        <span className="figures text-ink-3">{group.channels}</span>
                      </span>
                    }
                    checked={
                      groups.includes(group.key) ||
                      (added(group.key) && !released.includes(group.key))
                    }
                    onChange={(on) => tickGroup(group.key, on)}
                  />
                ))}
              </div>
              {filtered.length > listedChannelGroups && (
                <p className="text-small text-ink-2">
                  {t.users.moreChannelGroups(filtered.length - listedChannelGroups)}
                </p>
              )}
            </div>
            <div className="space-y-3">
              <h3 className="text-h3 text-ink">{t.users.tickedChannels}</h3>
              {channels.length === 0 ? (
                <p className="text-body text-ink-2">{t.users.noTickedChannels}</p>
              ) : (
                <ul className="flex flex-wrap gap-2" aria-label={t.users.tickedChannels}>
                  {tickedKeys.map((key) => {
                    const channel = tickedNames.get(key)
                    const name =
                      channel === undefined
                        ? ticked.isSuccess
                          ? t.users.goneChannel
                          : '…'
                        : `${channel.number} ${channel.name}`
                    return (
                      <Chip
                        key={key}
                        title={key}
                        label={name}
                        removeLabel={t.users.removeChannel(name)}
                        onClick={() => tickChannel(key, false)}
                      />
                    )
                  })}
                </ul>
              )}
              {channels.length > listedChannels && (
                <p className="text-small text-ink-2">
                  {t.users.moreChannels(channels.length - listedChannels)}
                </p>
              )}
            </div>
            <div className="space-y-3">
              <h3 className="text-h3 text-ink">{t.users.singleChannels}</h3>
              <TextInput
                type="search"
                size="sm"
                aria-label={t.users.searchChannels}
                placeholder={t.users.searchChannels}
                value={search}
                onValue={setSearch}
                onKeyDown={holdEnter}
                className="sm:max-w-[28.5rem]"
              />
              <div className="grid gap-x-4 gap-y-1 sm:grid-cols-2">
                {choices.data.channels.map((channel) => (
                  <Checkbox
                    key={channel.key}
                    label={`${channel.number} ${channel.name}`}
                    help={groupNames.get(channel.group)}
                    checked={channels.includes(channel.key)}
                    onChange={(on) => tickChannel(channel.key, on)}
                  />
                ))}
              </div>
              {choices.data.total > choices.data.channels.length && (
                <p className="text-small text-ink-2">
                  {t.users.moreChannels(choices.data.total - choices.data.channels.length)}
                </p>
              )}
            </div>
          </fieldset>
        ))}
    </>
  )
}

/** Every half hour from 00:00 to 24:00, as jellyfin-web offers them. */
const halfHours = Array.from({ length: 49 }, (_, i) => i / 2)

/** "09:30" for 9.5: hours count from midnight and may have fractions. */
function hourLabel(hour: number) {
  const minutes = Math.round(hour * 60)
  return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`
}

/** How many hours rows differ from the saved ones, counting those added or removed. */
function changedSchedules(a: readonly AccessSchedule[], b: readonly AccessSchedule[]) {
  let count = Math.abs(a.length - b.length)
  for (let i = 0; i < Math.min(a.length, b.length); i++) {
    const x = a[i]
    const y = b[i]
    if (x.day !== y.day || x.startHour !== y.startHour || x.endHour !== y.endHour) count++
  }
  return count
}

export function AllowedHoursSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  return (
    <LimitsSection
      id={id}
      section="hours"
      target={target}
      title={t.users.allowedHoursTitle}
      description={t.users.allowedHoursHelp}
    >
      {(view) => <AllowedHoursRows id={id} view={view} />}
    </LimitsSection>
  )
}

function AllowedHoursRows({ id, view }: { id: string; view: SectionView }) {
  const { t } = useI18n()
  const { readOnly } = view
  const saved = (view.saved ?? view.values).accessSchedules
  const [schedules, setSchedules] = useState<AccessSchedule[]>(view.values.accessSchedules)
  const misordered = schedules.some((s) => s.startHour >= s.endHour)
  useSectionDraft(id, view, changedSchedules(schedules, saved), () =>
    misordered ? null : { accessSchedules: schedules },
  )

  function change(index: number, changed: Partial<AccessSchedule>) {
    setSchedules(schedules.map((s, i) => (i === index ? { ...s, ...changed } : s)))
  }

  return (
    <fieldset disabled={readOnly} className="contents">
      {schedules.length === 0 ? (
        <p className={cx(tile, 'text-body text-ink-2')}>{t.users.noAllowedHours}</p>
      ) : (
        <ul
          className="grid grid-cols-[minmax(0,1fr)] gap-1.5"
          aria-label={t.users.allowedHoursTitle}
        >
          {schedules.map((schedule, index) => (
            // Rows have no identity of their own; they are edited in place.
            <li key={index} className={cx(tile, 'flex flex-wrap items-end gap-3')}>
              <Field label={t.users.day} className="min-w-[12.5rem] flex-1">
                <Select<ScheduleDay>
                  size="sm"
                  value={schedule.day}
                  onValue={(day) => change(index, { day })}
                  options={scheduleDays.map((day) => ({ value: day, label: t.users.days[day] }))}
                />
              </Field>
              {(['startHour', 'endHour'] as const).map((field) => (
                <Field
                  key={field}
                  label={field === 'startHour' ? t.users.from : t.users.to}
                  className="w-[8rem]"
                >
                  <Select
                    size="sm"
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
              {!readOnly && (
                <Button
                  variant="ghost"
                  icon={TrashIcon}
                  onClick={() => setSchedules(schedules.filter((_, i) => i !== index))}
                >
                  {t.users.removeHours}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {misordered && <Notice tone="danger">{t.users.hoursOrder}</Notice>}
      {!readOnly && (
        <div>
          <Button
            icon={PlusIcon}
            onClick={() =>
              setSchedules([...schedules, { day: 'Everyday', startHour: 8, endHour: 20 }])
            }
          >
            {t.users.addHours}
          </Button>
        </div>
      )}
    </fieldset>
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
 * The switches of the permissions in Access, on a user's page and on a group's, each saved as it
 * changes. Remote access off warns when Health sees a reverse proxy missing from Known proxies:
 * everyone coming through it looks local, so the limit does not hold for them.
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
  return rows.map(([field, title, help]) =>
    field === 'remoteAccess' && !values.remoteAccess && unknownProxy ? (
      <SettingRow
        key={field}
        kind="switch"
        label={title}
        help={help}
        checked={values[field]}
        disabled={disabled}
        onChange={(checked) => onChange({ [field]: checked })}
      >
        <p className="text-warn">{t.users.remoteAccessUnknownProxy}</p>
        <TextLink to="/settings/security#known-proxies" className="mt-2 -ml-3">
          {t.users.openKnownProxies}
        </TextLink>
      </SettingRow>
    ) : (
      <SettingRow
        key={field}
        kind="switch"
        label={title}
        help={help}
        checked={values[field]}
        disabled={disabled}
        onChange={(checked) => onChange({ [field]: checked })}
      />
    ),
  )
}

/** Whether a and b hold the same values, in any order. */
function sameSet(a: readonly string[], b: readonly string[]) {
  return a.length === b.length && a.every((value) => b.includes(value))
}

/**
 * What Reset to defaults changes, in the order of the page: each of the sections it resets whose
 * values change, with the settings that change in it for Access and Playback and access, whose
 * settings are many.
 */
function resetChanges(
  t: Messages,
  current: GroupLimits,
  defaults: GroupLimits,
  reset: readonly GroupSection[],
) {
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
      ['localFirst', t.users.localFirst],
      ['boostVolume', t.users.boostVolume],
      ['convertSound', t.users.convertSound],
      ['syncPlay', t.users.syncPlay],
      ['liveTv', t.users.liveTv],
      ['remoteControl', t.users.remoteControl],
      ['liveTvManagement', t.users.liveTvManagement],
    ] as const
  ).filter(([field]) => current[field] !== defaults[field])
  const parental = current.parentalControl
  const sections: { section: GroupSection; settings: string[]; changed: boolean }[] = [
    {
      section: 'access',
      settings: access.map(([, label]) => label),
      changed: access.length > 0,
    },
    {
      section: 'playback',
      settings: playback.map(([, label]) => label),
      changed: playback.length > 0,
    },
    {
      section: 'parental',
      settings: [],
      changed:
        parental.maxRating !== defaults.parentalControl.maxRating ||
        parental.maxSubRating !== defaults.parentalControl.maxSubRating ||
        !sameSet(parental.blockUnrated, defaults.parentalControl.blockUnrated),
    },
    {
      section: 'libraries',
      settings: [],
      changed: !sameSet(current.hiddenLibraries, defaults.hiddenLibraries),
    },
    {
      section: 'genres',
      settings: [],
      changed: !sameSet(current.blockedGenres, defaults.blockedGenres),
    },
    {
      section: 'hours',
      settings: [],
      changed: JSON.stringify(current.accessSchedules) !== JSON.stringify(defaults.accessSchedules),
    },
    {
      section: 'channels',
      settings: [],
      changed:
        current.liveTvChannels.mode !== defaults.liveTvChannels.mode ||
        !sameSet(current.liveTvChannels.groups, defaults.liveTvChannels.groups) ||
        !sameSet(current.liveTvChannels.channels, defaults.liveTvChannels.channels),
    },
  ]
  return sections.filter((section) => section.changed && reset.includes(section.section))
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
  // The members who follow the group in a section take its values there at once.
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
    enabled: open && target.kind === 'group',
  })
  const followers = (users.data ?? []).filter(
    (user) => user.groupId === id && user.ownSections.length < groupSections.length,
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
  // A member's reset reaches only the sections they keep as their own.
  const resets =
    target.kind === 'user' && target.user.groupId !== null ? target.user.ownSections : groupSections
  const changes =
    defaults.data === undefined ? null : resetChanges(t, limitsOf(target), defaults.data, resets)

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
          <p>
            {text.ownSettingsHelp(
              target.user.groupName,
              target.user.ownSections.map((section) => sectionTitle(t, section)).join(', '),
            )}
          </p>
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
                <li key={change.section}>
                  <span className="text-ink">{sectionTitle(t, change.section)}</span>
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
