import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { PlusIcon, TrashIcon, XIcon } from '@phosphor-icons/react'
import { useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  fetchGroupDefaults,
  fetchLineupChannelChoices,
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
  type AccessSchedule,
  type ChannelRule,
  type Group,
  type GroupLimits,
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

/** Whether the section shows a group's values, read only, on the page of a member following it. */
function isReadOnly(target: LimitsTarget) {
  return target.kind === 'user' && followedGroup(target.user) !== null
}

/**
 * A section of limits on a user's or a group's page: its title, its rows, and on the page of a
 * member following their group, the notice saying the values are the group's. Its changes go to
 * the page's dock.
 */
function LimitsSection({
  id,
  title,
  description,
  target,
  children,
}: {
  id: string
  title: string
  description?: ReactNode
  target: LimitsTarget
  children: ReactNode
}) {
  const followed = target.kind === 'user' ? followedGroup(target.user) : null
  return (
    <PageSection id={id} title={title} description={description}>
      {followed !== null && <FollowedNotice group={followed} />}
      {children}
    </PageSection>
  )
}

/** A tile for what is more than a row (chips, lists to tick), as wide as the rows. */
const tile = 'min-w-0 rounded-row bg-s1 px-[30px] py-6 max-md:px-5 max-md:py-5'

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
      className="flex min-h-11 max-w-full items-center gap-1 rounded-full bg-s2 py-1 pr-1.5 pl-4 text-control text-ink"
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
  const { t, language } = useI18n()
  const readOnly = isReadOnly(target)
  const saved = playbackOf(limitsOf(target))
  const [form, setForm] = useState(saved)
  useDraftPart(id, readOnly ? 0 : countChanged(form, saved), () => changedFields(form, saved))
  // The saved values give each changed row its "was …"; a member following a group changes none.
  const was = readOnly ? undefined : saved

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
    <LimitsSection id={id} target={target} title={t.users.playbackAccessTitle}>
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
    </LimitsSection>
  )
}

const unratedMovie = 'Movie'
const unratedSeries = 'Series'

export function ParentalControlSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const ratings = useParentalRatings()
  const readOnly = isReadOnly(target)
  const initial = limitsOf(target).parentalControl
  const savedMovies = initial.blockUnrated.includes(unratedMovie)
  const savedShows = initial.blockUnrated.includes(unratedSeries)
  const [limit, setLimit] = useState<{ maxRating: number | null; maxSubRating: number | null }>({
    maxRating: initial.maxRating,
    maxSubRating: initial.maxSubRating,
  })
  const [blockMovies, setBlockMovies] = useState(savedMovies)
  const [blockShows, setBlockShows] = useState(savedShows)

  const groups = ratings.data === undefined ? [] : groupRatings(ratings.data)
  const selected = selectedGroup(groups, { ...limit, blockUnrated: [] })
  const savedSelected = selectedGroup(groups, initial)

  const changes =
    Number(limit.maxRating !== initial.maxRating || limit.maxSubRating !== initial.maxSubRating) +
    Number(blockMovies !== savedMovies) +
    Number(blockShows !== savedShows)
  useDraftPart(id, readOnly ? 0 : changes, () => {
    // Keep values set by Jellyfin apps (live TV, books...) that this section does not show.
    const blockUnrated = initial.blockUnrated.filter(
      (item) => item !== unratedMovie && item !== unratedSeries,
    )
    if (blockMovies) blockUnrated.push(unratedMovie)
    if (blockShows) blockUnrated.push(unratedSeries)
    return { parentalControl: { ...limit, blockUnrated } }
  })

  return (
    <LimitsSection id={id} target={target} title={t.users.parentalTitle}>
      {ratings.isError ? (
        <InlineError onRetry={() => void ratings.refetch()} retrying={ratings.isFetching}>
          {errorMessage(t, ratings.error)}
        </InlineError>
      ) : ratings.isPending ? (
        <Skeleton className="h-[82px] rounded-row max-md:h-[68px]" />
      ) : (
        <SettingRow<number>
          kind="choice"
          label={t.users.maxRating}
          help={t.users.maxRatingHelp}
          value={selected}
          saved={readOnly ? undefined : savedSelected}
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
        saved={readOnly ? undefined : savedMovies}
        disabled={readOnly}
        onChange={setBlockMovies}
      />
      <SettingRow
        kind="switch"
        label={t.users.blockUnratedShows}
        checked={blockShows}
        saved={readOnly ? undefined : savedShows}
        disabled={readOnly}
        onChange={setBlockShows}
      />
    </LimitsSection>
  )
}

export function VisibleLibrariesSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const readOnly = isReadOnly(target)
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const savedHidden = limitsOf(target).hiddenLibraries
  const [hidden, setHidden] = useState(savedHidden)
  useDraftPart(id, readOnly ? 0 : setDifference(hidden, savedHidden), () => {
    if (!choices.isSuccess) return null
    // Libraries removed from the server since are dropped.
    const libraries = choices.data.libraries
    return { hiddenLibraries: hidden.filter((id) => libraries.some((l) => l.id === id)) }
  })

  return (
    <LimitsSection
      id={id}
      target={target}
      title={t.users.visibleLibrariesTitle}
      description={t.users.visibleLibrariesHelp}
    >
      {choices.isPending ? (
        <Skeleton className="h-[82px] rounded-row max-md:h-[68px]" />
      ) : choices.isError ? (
        <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
          {errorMessage(t, choices.error)}
        </InlineError>
      ) : choices.data.libraries.length === 0 ? (
        <EmptyState title={t.users.noServerLibraries}>{t.users.noServerLibrariesHelp}</EmptyState>
      ) : (
        choices.data.libraries.map((library) => (
          <SettingRow
            key={library.id}
            kind="switch"
            label={library.name}
            checked={!hidden.includes(library.id)}
            saved={readOnly ? undefined : !savedHidden.includes(library.id)}
            stateText={[t.users.libraryShown, t.users.libraryHidden]}
            disabled={readOnly}
            onChange={(shown) =>
              setHidden(shown ? hidden.filter((id) => id !== library.id) : [...hidden, library.id])
            }
          />
        ))
      )}
    </LimitsSection>
  )
}

export function BlockedGenresSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const readOnly = isReadOnly(target)
  const choices = useQuery({
    queryKey: queryKeys.userContentChoices,
    queryFn: ({ signal }) => fetchUserContentChoices(signal),
  })
  const savedGenres = limitsOf(target).blockedGenres
  const [genres, setGenres] = useState(savedGenres)
  const [typed, setTyped] = useState('')
  useDraftPart(id, readOnly ? 0 : setDifference(genres, savedGenres), () => ({
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
    <LimitsSection
      id={id}
      target={target}
      title={t.users.blockedGenresTitle}
      description={t.users.blockedGenresHelp}
    >
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
            <Field label={t.users.genre} help={t.users.genreHint} className="w-full sm:max-w-sm">
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
            <Button icon={PlusIcon} className="sm:mt-[30px]" onClick={() => add()}>
              {t.users.addGenre}
            </Button>
          </div>
        )}
      </fieldset>
    </LimitsSection>
  )
}

/** How many groups and channels the Live TV channels section lists at once; filters reach the rest. */
const listedChannelGroups = 200
const listedChannels = 50

export function LiveTvChannelsSection({ target, id }: { target: LimitsTarget; id: string }) {
  const { t } = useI18n()
  const readOnly = isReadOnly(target)
  const initial = limitsOf(target).liveTvChannels
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
    Number(mode !== initial.mode) +
    (mode === 'all'
      ? 0
      : setDifference(groups, initial.groups) +
        setDifference(channels, initial.channels) +
        released.length +
        Number(mode === 'except' && hideNew !== initial.hideNewGroups))
  useDraftPart(id, readOnly ? 0 : changes, () => {
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
    <LimitsSection
      id={id}
      target={target}
      title={t.users.liveTvChannelsTitle}
      description={t.users.liveTvChannelsHelp}
    >
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
        saved={readOnly ? undefined : initial.mode}
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
          <fieldset disabled={readOnly} className={cx(tile, 'space-y-8')}>
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
                className="sm:max-w-sm"
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
                className="sm:max-w-sm"
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
    </LimitsSection>
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
  const readOnly = isReadOnly(target)
  const saved = limitsOf(target).accessSchedules
  const [schedules, setSchedules] = useState<AccessSchedule[]>(saved)
  const misordered = schedules.some((s) => s.startHour >= s.endHour)
  useDraftPart(id, readOnly ? 0 : changedSchedules(schedules, saved), () =>
    misordered ? null : { accessSchedules: schedules },
  )

  function change(index: number, changed: Partial<AccessSchedule>) {
    setSchedules(schedules.map((s, i) => (i === index ? { ...s, ...changed } : s)))
  }

  return (
    <LimitsSection
      id={id}
      target={target}
      title={t.users.allowedHoursTitle}
      description={t.users.allowedHoursHelp}
    >
      <fieldset disabled={readOnly} className="contents">
        {schedules.length === 0 ? (
          <p className={cx(tile, 'text-body text-ink-2')}>{t.users.noAllowedHours}</p>
        ) : (
          <ul
            className="grid grid-cols-[minmax(0,1fr)] gap-2"
            aria-label={t.users.allowedHoursTitle}
          >
            {schedules.map((schedule, index) => (
              // Rows have no identity of their own; they are edited in place.
              <li key={index} className={cx(tile, 'flex flex-wrap items-end gap-3 py-5')}>
                <Field label={t.users.day} className="min-w-[200px] flex-1">
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
                    className="w-[128px]"
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
    </LimitsSection>
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
    {
      title: t.users.liveTvChannelsTitle,
      settings: [],
      changed:
        current.liveTvChannels.mode !== defaults.liveTvChannels.mode ||
        !sameSet(current.liveTvChannels.groups, defaults.liveTvChannels.groups) ||
        !sameSet(current.liveTvChannels.channels, defaults.liveTvChannels.channels),
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
