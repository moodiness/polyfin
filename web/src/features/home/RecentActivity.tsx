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
  InlineError,
  Segmented,
  Skeleton,
  Spinner,
  StatusPill,
  TextInput,
  TextLink,
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
  const footRef = useRef<HTMLParagraphElement>(null)
  const listed = !activity.isPending && !activity.isError && shown.length > 0
  const height = useWindowFill(boxRef, footRef, listed)
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
      {/* From xl, in Home's right column, the filters and the search stack. */}
      <div className="mb-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between xl:flex-col xl:items-stretch">
        <div className="-mx-5 overflow-x-auto px-5 scrollbar-none md:mx-0 md:px-0">
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
          className="lg:w-72 xl:w-full"
        />
      </div>
      {activity.isPending ? (
        <div role="status" aria-label={t.common.loading}>
          {[0, 1, 2, 3].map((row) => (
            <div
              key={row}
              className="grid grid-cols-[76px_32px_minmax(0,1fr)] items-center gap-3 border-line py-3 not-first:border-t max-sm:grid-cols-[32px_minmax(0,1fr)]"
            >
              <Skeleton className="h-3 w-11 max-sm:hidden" />
              <Skeleton className="size-[30px] rounded-[9px]" />
              <Skeleton className="h-3.5 w-2/3" />
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
            '-mx-2 overflow-y-auto overscroll-contain rounded-[10px] px-2 scrollbar-thin',
            height === undefined && 'max-h-[420px] sm:max-h-[520px]',
          )}
        >
          <ol>
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
        <p ref={footRef} className="mt-3 border-t border-line pt-3 text-small text-ink-3">
          {text.count(entries.length, total)} {text.autoRefresh}
        </p>
      )}
    </Block>
  )
}

/** Tailwind's xl, from which Home shows Recent activity in a column of its own. */
const columnQuery = '(min-width: 80rem)'
/** The space left under the block when its list fills the window. */
const bottomGap = 24
/** The list is never shorter than on narrower screens. */
const minHeight = 420

/**
 * From xl, the list's height, so that the block ends at the bottom of the window as the page
 * opens: Recent activity shows whole beside the rest of Home, scrolling in its own box. Measured
 * again when the window or the page changes size; undefined below xl, where the list keeps its
 * maximum height.
 */
function useWindowFill(
  box: RefObject<HTMLElement | null>,
  foot: RefObject<HTMLElement | null>,
  listed: boolean,
): number | undefined {
  const [height, setHeight] = useState<number>()
  useEffect(() => {
    const element = box.current
    if (!listed || !element) return
    const media = window.matchMedia(columnQuery)
    const measure = () => {
      if (!media.matches) {
        setHeight(undefined)
        return
      }
      const place = element.getBoundingClientRect()
      // What follows the list in the block (the count), whatever the list's height.
      const below = foot.current ? foot.current.getBoundingClientRect().bottom - place.bottom : 0
      const top = place.top + window.scrollY
      setHeight(Math.max(minHeight, Math.floor(window.innerHeight - top - below - bottomGap)))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(document.body)
    window.addEventListener('resize', measure)
    media.addEventListener('change', measure)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
      media.removeEventListener('change', measure)
    }
  }, [box, foot, listed])
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
    <li className="grid grid-cols-[76px_32px_minmax(0,1fr)_auto] items-center gap-3 border-line py-3 text-[14px] not-first:border-t max-sm:grid-cols-[32px_minmax(0,1fr)] max-sm:items-start">
      <time
        dateTime={entry.date}
        title={dateTime(entry.date, language)}
        className="figures text-[12.5px] text-ink-3 max-sm:hidden"
      >
        {today
          ? new Intl.DateTimeFormat(language, { timeStyle: 'short' }).format(date)
          : relativeTime(entry.date, language, t.time.justNow)}
      </time>
      <span
        aria-hidden="true"
        className={cx(
          'grid size-[30px] place-items-center rounded-[9px] border',
          entry.severity === 'Error'
            ? 'border-danger/25 bg-danger/8 text-danger'
            : entry.severity === 'Warning'
              ? 'border-warn/20 bg-warn/10 text-warn'
              : 'border-line bg-s2 text-ink-2',
        )}
      >
        <Glyph size={16} />
      </span>
      <div className="min-w-0">
        <p className="break-words text-ink">{entry.name}</p>
        {entry.shortOverview !== null && (
          <p className="mt-0.5 break-words text-[13px] text-ink-3">{entry.shortOverview}</p>
        )}
        <p className="mt-0.5 flex flex-wrap items-center gap-x-3 text-[12.5px] text-ink-3 sm:hidden">
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
