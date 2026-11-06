import { useEffect, useRef, useState, type RefObject } from 'react'
import {
  ArrowClockwiseIcon,
  CloudArrowDownIcon,
  GearIcon,
  InfoIcon,
  KeyIcon,
  LinkBreakIcon,
  LinkIcon,
  MagnifyingGlassIcon,
  PasswordIcon,
  PlayIcon,
  PuzzlePieceIcon,
  RecordIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
  ShieldSlashIcon,
  SignInIcon,
  StopIcon,
  UserIcon,
  UsersThreeIcon,
  WarningCircleIcon,
  WarningIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { fetchActivity, queryKeys, type ActivityEntry, type ActivityQuery } from '@/api'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  cx,
  EmptyState,
  IconTile,
  InlineError,
  Segmented,
  Skeleton,
  Spinner,
  StatusPill,
  TextInput,
  TextLink,
  useScrollFade,
} from '@/ui'

const pageSize = 25

/** The activity log's entry types each filter keeps; empty keeps all. */
const filters = {
  all: {},
  problems: { severities: ['Warning', 'Error'] },
  signIns: {
    types: ['AuthenticationSucceeded', 'AuthenticationFailed', 'UserRecoveryCodeUsed'],
  },
  playback: { types: ['VideoPlayback', 'VideoPlaybackStopped', 'SharedLinkPlayback'] },
  users: {
    types: [
      'UserCreated',
      'UserDeleted',
      'UserPolicyUpdated',
      'UserPasswordChanged',
      'UserTwoStepOn',
      'UserTwoStepOff',
      'UserEnded',
      'ShareLinkCreated',
      'ShareLinkRevoked',
      'ShareLinkExpired',
      'GroupCreated',
      'GroupChanged',
      'GroupDeleted',
      'GroupMemberAdded',
      'GroupMemberRemoved',
      'GroupMemberDetached',
    ],
  },
  server: {
    types: [
      'ServerConfigurationUpdated',
      'AddonInstalled',
      'AddonUninstalled',
      'AddonUsedByChanged',
      'SecretRevealed',
      'ServerRestarted',
      'ServerUpdateStarted',
    ],
  },
  recordings: { types: ['RecordingScheduled', 'RecordingDeleted'] },
} satisfies Record<string, Omit<ActivityQuery, 'limit'>>

type Filter = keyof typeof filters

const typeIcons: Record<string, Icon> = {
  AuthenticationSucceeded: SignInIcon,
  AuthenticationFailed: SignInIcon,
  VideoPlayback: PlayIcon,
  VideoPlaybackStopped: StopIcon,
  UserCreated: UserIcon,
  UserDeleted: UserIcon,
  UserPolicyUpdated: UserIcon,
  UserEnded: UserIcon,
  UserPasswordChanged: PasswordIcon,
  UserTwoStepOn: ShieldCheckIcon,
  UserTwoStepOff: ShieldSlashIcon,
  UserRecoveryCodeUsed: SignInIcon,
  GroupCreated: UsersThreeIcon,
  GroupChanged: UsersThreeIcon,
  GroupDeleted: UsersThreeIcon,
  GroupMemberAdded: UsersThreeIcon,
  GroupMemberRemoved: UsersThreeIcon,
  GroupMemberDetached: UsersThreeIcon,
  ShareLinkCreated: LinkIcon,
  ShareLinkRevoked: LinkBreakIcon,
  ShareLinkExpired: LinkBreakIcon,
  SharedLinkPlayback: ShareNetworkIcon,
  ServerConfigurationUpdated: GearIcon,
  AddonInstalled: PuzzlePieceIcon,
  AddonUninstalled: PuzzlePieceIcon,
  AddonUsedByChanged: PuzzlePieceIcon,
  SecretRevealed: KeyIcon,
  ServerRestarted: ArrowClockwiseIcon,
  ServerUpdateStarted: CloudArrowDownIcon,
  RecordingScheduled: RecordIcon,
  RecordingDeleted: RecordIcon,
}

/** The server's latest activity (sign-ins, playback, changes), filterable, for administrators. */
export function RecentActivity() {
  const { language, t } = useI18n()
  const text = t.dashboard.activity
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const query: ActivityQuery = { limit: pageSize, ...filters[filter] }
  const activity = useInfiniteQuery({
    queryKey: queryKeys.activity(query),
    queryFn: ({ pageParam, signal }) => fetchActivity({ ...query, start: pageParam }, signal),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((count, page) => count + page.items.length, 0)
      return loaded < last.total ? loaded : undefined
    },
    refetchInterval: 15_000,
  })
  const entries = activity.data?.pages.flatMap((page) => page.items) ?? []
  const total = activity.data?.pages.at(-1)?.total ?? 0
  const needle = search.trim().toLocaleLowerCase(language)
  const shown = needle
    ? entries.filter((entry) =>
        `${entry.name} ${entry.shortOverview ?? ''} ${entry.overview ?? ''}`
          .toLocaleLowerCase(language)
          .includes(needle),
      )
    : entries
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = activity
  const boxRef = useRef<HTMLDivElement>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const filterRef = useRef<HTMLDivElement>(null)
  useScrollFade(filterRef)
  const listed = !activity.isPending && !activity.isError && shown.length > 0
  const height = useListHeight(boxRef, listed)
  // The next page loads by itself once the end of the box comes near. Observing again after
  // each page makes a sentinel that is still in view (a short or filtered list) load the next.
  useEffect(() => {
    const root = boxRef.current
    const sentinel = sentinelRef.current
    if (!root || !sentinel || !hasNextPage || isFetchingNextPage) return
    const observer = new IntersectionObserver(
      ([seen]) => {
        if (seen?.isIntersecting) void fetchNextPage()
      },
      { root, rootMargin: '0px 0px 160px 0px' },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, shown.length])

  return (
    <Block
      title={text.title}
      aside={<TextLink to="/system/logs">{text.openLog}</TextLink>}
      className="scroll-mt-24"
    >
      <div className="mb-4 flex flex-col gap-3">
        <div
          ref={filterRef}
          className="-mx-5 scroll-fade overflow-x-auto px-5 scrollbar-none md:mx-0 md:px-0"
        >
          <Segmented
            size="sm"
            className="[&>button]:shrink-0 [&>button]:whitespace-nowrap"
            label={text.filter}
            value={filter}
            onChange={setFilter}
            options={(Object.keys(filters) as Filter[]).map((key) => ({
              value: key,
              label: text.filters[key],
            }))}
          />
        </div>
        <TextInput
          type="search"
          size="sm"
          icon={MagnifyingGlassIcon}
          value={search}
          onValue={setSearch}
          aria-label={text.search}
          placeholder={text.search}
        />
      </div>
      {activity.isPending ? (
        <div role="status" aria-label={t.common.loading} className="flex flex-col gap-1">
          {[0, 1, 2, 3].map((row) => (
            <div
              key={row}
              className="grid min-h-16 grid-cols-[5.25rem_2.25rem_minmax(0,1fr)] items-center gap-4 px-4 max-sm:grid-cols-[2.25rem_minmax(0,1fr)] max-sm:px-0"
            >
              <Skeleton className="h-4 w-14 max-sm:hidden" />
              <Skeleton className="size-9 rounded-full" />
              <Skeleton className="h-4 w-2/3" />
            </div>
          ))}
        </div>
      ) : activity.isError ? (
        <InlineError onRetry={() => void activity.refetch()} retrying={activity.isFetching}>
          {errorMessage(t, activity.error)}
        </InlineError>
      ) : shown.length === 0 ? (
        entries.length === 0 && filter === 'all' ? (
          <EmptyState icon={InfoIcon} title={text.empty}>
            {text.emptyHelp}
          </EmptyState>
        ) : (
          <EmptyState icon={MagnifyingGlassIcon} title={text.noMatch}>
            {text.noMatchHelp}
          </EmptyState>
        )
      ) : (
        <div
          ref={boxRef}
          tabIndex={0}
          role="region"
          aria-label={text.listLabel}
          aria-busy={isFetchingNextPage}
          style={height === undefined ? undefined : { height }}
          className={cx(
            'overflow-y-auto overscroll-contain rounded-row scrollbar-thin focus-lit [--lift:1]',
            height === undefined && 'max-h-[28.75rem] sm:max-h-[37.5rem]',
          )}
        >
          <ol className="flex flex-col gap-1">
            {shown.map((entry) => (
              <ActivityRow key={entry.id} entry={entry} />
            ))}
          </ol>
          {hasNextPage && (
            <div ref={sentinelRef} className="flex h-12 items-center justify-center text-ink-3">
              {isFetchingNextPage && (
                <span role="status" className="flex items-center gap-2 text-small">
                  <Spinner />
                  {t.common.loading}
                </span>
              )}
            </div>
          )}
        </div>
      )}
      {activity.data !== undefined && (
        <p className="mt-4 text-small text-ink-3">
          {text.count(entries.length, total)} {text.autoRefresh}
        </p>
      )}
    </Block>
  )
}

/** Tailwind's sm, from which the list may be taller. */
const wideQuery = '(min-width: 40rem)'
/** The most the list shows, on a phone and from sm (its classes before measuring). */
const phoneMax = 460
const wideMax = 600

/**
 * The list's height: a list taller than its maximum ends after its last whole row, never in the
 * middle of one. Measured again when the window or the list changes size; undefined while the
 * list fits.
 */
function useListHeight(box: RefObject<HTMLElement | null>, listed: boolean): number | undefined {
  const [height, setHeight] = useState<number>()
  useEffect(() => {
    const element = box.current
    if (!listed || !element) return
    const wide = window.matchMedia(wideQuery)
    const measure = () => {
      const limit = wide.matches ? wideMax : phoneMax
      if (element.scrollHeight <= limit) {
        setHeight(undefined)
        return
      }
      // The bottom of the last row that shows whole within the limit.
      const contentTop = element.getBoundingClientRect().top - element.scrollTop
      let whole = 0
      for (const row of element.querySelectorAll(':scope > ol > li')) {
        const bottom = Math.ceil(row.getBoundingClientRect().bottom - contentTop)
        if (bottom > limit) break
        whole = bottom
      }
      setHeight(whole > 0 ? whole : limit)
    }
    measure()
    const observer = new ResizeObserver(measure)
    const list = element.firstElementChild
    if (list) observer.observe(list)
    window.addEventListener('resize', measure)
    wide.addEventListener('change', measure)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
      wide.removeEventListener('change', measure)
    }
  }, [box, listed])
  return height
}

function ActivityRow({ entry }: { entry: ActivityEntry }) {
  const { language, t } = useI18n()
  const text = t.dashboard.activity
  const date = new Date(entry.date)
  const today = date.toDateString() === new Date().toDateString()
  const problem = entry.severity !== 'Information'
  const Glyph =
    entry.severity === 'Error'
      ? WarningCircleIcon
      : entry.severity === 'Warning'
        ? WarningIcon
        : (typeIcons[entry.type] ?? InfoIcon)
  return (
    <li className="grid min-h-16 grid-cols-[5.25rem_2.25rem_minmax(0,1fr)_auto] items-center gap-4 rounded-[1.375rem] px-4 py-2.5 text-[1.125rem] max-sm:grid-cols-[2.25rem_minmax(0,1fr)] max-sm:items-start max-sm:gap-3 max-sm:px-0 max-sm:text-[1.03125rem]">
      <time
        dateTime={entry.date}
        title={dateTime(entry.date, language)}
        className="figures text-right text-micro text-ink-3 max-sm:hidden"
      >
        {today
          ? new Intl.DateTimeFormat(language, { timeStyle: 'short' }).format(date)
          : relativeTime(entry.date, language, t.time.justNow)}
      </time>
      <IconTile
        icon={Glyph}
        size="sm"
        tone={
          entry.severity === 'Error' ? 'danger' : entry.severity === 'Warning' ? 'warn' : 'neutral'
        }
      />
      <div className="min-w-0">
        <p className="break-words text-ink">{entry.name}</p>
        {entry.shortOverview !== null && (
          <p className="mt-0.5 break-words text-[1rem] text-ink-3 max-sm:text-micro">
            {entry.shortOverview}
          </p>
        )}
        <p className="mt-0.5 flex flex-wrap items-center gap-x-3 text-micro text-ink-3 sm:hidden">
          <time dateTime={entry.date} title={dateTime(entry.date, language)} className="figures">
            {relativeTime(entry.date, language, t.time.justNow)}
          </time>
          {problem && (
            <StatusPill tone={entry.severity === 'Error' ? 'danger' : 'warn'}>
              {entry.severity === 'Error' ? text.error : text.warning}
            </StatusPill>
          )}
        </p>
      </div>
      <span className="max-sm:hidden">
        {problem && (
          <StatusPill tone={entry.severity === 'Error' ? 'danger' : 'warn'}>
            {entry.severity === 'Error' ? text.error : text.warning}
          </StatusPill>
        )}
      </span>
    </li>
  )
}
