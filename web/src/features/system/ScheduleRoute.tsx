import { useEffect, useId, useRef } from 'react'
import {
  useMutation,
  useQuery,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query'
import {
  CalendarDotsIcon,
  ListChecksIcon,
  PlayIcon,
  RecordIcon,
  StopIcon,
} from '@phosphor-icons/react'
import {
  fetchHealth,
  fetchSettings,
  fetchSources,
  fetchTasks,
  fetchTimers,
  queryClient,
  queryKeys,
  runTask,
  stopTask,
  type Task,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { settingsPath } from '@/app/navigation'
import {
  Badge,
  Block,
  Button,
  EmptyState,
  InlineError,
  Panel,
  Row,
  RowList,
  Skeleton,
  SettingHelp,
  SkeletonRows,
  SkeletonText,
  StatusPill,
  TextLink,
  useToast,
  RelativeTime,
} from '@/ui'
import { dateTime, errorMessage, formatHour, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import { TunersShort } from '@/features/livetv/TunersShort'
import BackupStatus from './BackupStatus'
import OwnerChip from './OwnerChip'
import RestoreBackup from './RestoreBackup'
import { Anchor } from './parts'

/** The task that backs the database up every day while backups are on in the settings. */
const backupTaskKey = 'BackUpDatabase'

/** `/system/schedule`: scheduled tasks, backups, recordings ahead and Live TV refreshes. */
export default function ScheduleRoute() {
  const { language, t } = useI18n()
  const tasks = useQuery({
    queryKey: queryKeys.tasks(language),
    queryFn: ({ signal }) => fetchTasks(language, signal),
    // Faster while a task runs, to show it end.
    refetchInterval: (query) =>
      query.state.data?.some((task) => task.state !== 'Idle') ? 1_000 : 5_000,
  })
  const toast = useToast()
  const act = useMutation({
    mutationFn: ({ task, action }: { task: Task; action: 'run' | 'stop' }) =>
      action === 'run' ? runTask(task.id) : stopTask(task.id),
    onSuccess: (_, { task, action }) => {
      if (action === 'run') toast(t.system.schedule.started(task.name), { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })

  return (
    <PageLayout title={t.system.schedule.title} lede={t.system.schedule.description}>
      <Tasks tasks={tasks} act={act} />
      <Backups tasks={tasks.data} act={act} />
      <div className="grid gap-x-8 gap-y-block xl:grid-cols-2">
        <Timers />
        <Refreshes />
      </div>
    </PageLayout>
  )
}

type TasksQuery = UseQueryResult<Task[]>
type TaskAction = { task: Task; action: 'run' | 'stop' }
type Act = UseMutationResult<void, Error, TaskAction>

function Tasks({ tasks, act }: { tasks: TasksQuery; act: Act }) {
  const { t } = useI18n()
  const text = t.system.schedule
  const categories = new Map<string, Task[]>()
  for (const task of tasks.data ?? []) {
    categories.set(task.category, [...(categories.get(task.category) ?? []), task])
  }

  return (
    <Anchor id="tasks">
      <Block title={text.tasksTitle} count={tasks.data?.length} aside={text.autoRefresh}>
        {tasks.isPending ? (
          <SkeletonRows rows={6} />
        ) : tasks.isError ? (
          <InlineError onRetry={() => void tasks.refetch()} retrying={tasks.isFetching}>
            {errorMessage(t, tasks.error)}
          </InlineError>
        ) : tasks.data.length === 0 ? (
          <EmptyState icon={ListChecksIcon} title={text.noTasks}>
            {text.noTasksHint}
          </EmptyState>
        ) : (
          <div className="space-y-6 max-md:space-y-5">
            {[...categories].map(([category, list]) => (
              <section key={category} aria-label={category}>
                <h3 className="mb-2 text-group text-ink">{category}</h3>
                <ul className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
                  {list.map((task) => (
                    <TaskRow key={task.id} task={task} act={act} />
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </Block>
    </Anchor>
  )
}

/**
 * One task on one row: its name with its description as a one-line caption (More reads the
 * rest), how its last run went, when it runs next, and Run now. Below 1024 px the name and caption
 * take the row's width, and the last and next runs go under them, beside Run now.
 */
function TaskRow({ task, act }: { task: Task; act: Act }) {
  const id = useId()
  const error = task.state === 'Idle' && task.last !== null ? task.last.error : ''
  return (
    <li className="grid min-h-15 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5 rounded-row bg-s1 py-2 pr-2.5 pl-4 lg:grid-cols-[minmax(0,1fr)_16.5rem_10rem_auto] lg:gap-y-1">
      <div className="col-span-full min-w-0 lg:col-span-1">
        <p className="text-h3 break-words hyphens-auto text-ink">{task.name}</p>
        {task.description !== '' && (
          <SettingHelp id={`${id}-about`} lines={1}>
            {task.description}
          </SettingHelp>
        )}
      </div>
      <div className="col-start-1 row-start-2 flex min-w-0 flex-col gap-y-0.5 lg:contents">
        <LastRun task={task} />
        <NextRun task={task} />
      </div>
      <div className="col-start-2 row-start-2 flex justify-end self-end lg:col-start-4 lg:row-start-1 lg:self-center">
        <TaskButton task={task} act={act} />
      </div>
      {error !== '' && <p className="col-span-full text-small break-words text-danger">{error}</p>}
    </li>
  )
}

function TaskButton({ task, act }: { task: Task; act: Act }) {
  const { t } = useI18n()
  const text = t.system.schedule
  const busy = act.isPending && act.variables.task.id === task.id
  return task.state === 'Idle' ? (
    <Button
      size="sm"
      icon={PlayIcon}
      aria-label={text.runLabel(task.name)}
      disabled={act.isPending && !busy}
      loading={busy}
      onClick={() => act.mutate({ task, action: 'run' })}
    >
      {text.runNow}
    </Button>
  ) : (
    <Button
      size="sm"
      icon={StopIcon}
      aria-label={text.stopLabel(task.name)}
      disabled={(act.isPending && !busy) || task.state === 'Cancelling'}
      loading={busy}
      onClick={() => act.mutate({ task, action: 'stop' })}
    >
      {task.state === 'Cancelling' ? text.cancelling : text.stop}
    </Button>
  )
}

/** When the task runs next, then how often; "Next run" is for screen readers. */
function NextRun({ task }: { task: Task }) {
  const { language, t } = useI18n()
  const text = t.system.schedule
  const cadence =
    task.interval > 0
      ? text.every(formatSpan(task.interval, language))
      : task.daily !== null
        ? text.daily(formatHour(task.daily, language))
        : null
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 text-small figures lg:block">
      <span className="sr-only">{text.nextRun}</span>
      <span className="text-ink lg:block">
        {task.next !== null ? <RelativeTime iso={task.next} /> : text.byHandOnly}
      </span>
      {cadence !== null && <span className="text-ink-3 lg:block lg:text-micro">{cadence}</span>}
    </div>
  )
}

/**
 * How the task's last run went, and how long it took, which a phone leaves out to keep it on one
 * line; "Last run" is for screen readers.
 */
function LastRun({ task }: { task: Task }) {
  const { language, t } = useI18n()
  const text = t.system.schedule
  const label = <span className="sr-only">{text.lastRun}</span>
  const box = 'flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-small figures lg:gap-y-0'
  if (task.state !== 'Idle') {
    return (
      <div className={box}>
        {label}
        <StatusPill tone="live">
          <span className="motion-safe:animate-pulse">
            {task.state === 'Running' ? text.running : text.cancelling}
          </span>
        </StatusPill>
      </div>
    )
  }
  if (task.last === null) {
    return (
      <div className={box}>
        {label}
        <span className="text-ink-2">{text.notRunYet}</span>
      </div>
    )
  }
  const last = task.last
  const tone = last.status === 'Completed' ? 'ok' : last.status === 'Failed' ? 'danger' : 'muted'
  return (
    <div className={box}>
      {label}
      <StatusPill tone={tone}>{text.results[last.status] ?? last.status}</StatusPill>
      <span className="text-ink-2">
        <RelativeTime iso={last.end} />
      </span>
      <span className="text-ink-3 max-sm:hidden lg:basis-full lg:text-micro">
        {text.took(formatSpan((Date.parse(last.end) - Date.parse(last.start)) / 1000, language))}
      </span>
    </div>
  )
}

/** The backups' state, from the health, with the backup task's own button. */
function Backups({ tasks, act }: { tasks: Task[] | undefined; act: Act }) {
  const { t } = useI18n()
  const text = t.system.schedule
  // The Health page polls this every 30 seconds; here it follows the backup task's runs.
  const health = useQuery({
    queryKey: queryKeys.health,
    queryFn: ({ signal }) => fetchHealth(signal),
  })
  const task = tasks?.find((each) => each.key === backupTaskKey)
  const running = task !== undefined && task.state !== 'Idle'
  const ran = task?.last?.end
  // Reads the health again once a backup run ends, so the new file shows.
  useRefetchOn(ran, () => void health.refetch())

  return (
    <Anchor id="backups">
      <Panel
        title={text.backupsTitle}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {health.data?.backup && <RestoreBackup unavailable={health.data.backup.restore} />}
            {task && (
              <TaskButtonLabelled task={task} act={act} running={running} label={text.backUpNow} />
            )}
          </div>
        }
      >
        {health.isPending ? (
          <SkeletonText lines={3} />
        ) : health.isError ? (
          <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
            {errorMessage(t, health.error)}
          </InlineError>
        ) : health.data.backup === null ? (
          <div className="space-y-3">
            <p className="max-w-[66ch] text-small text-ink-2">{t.system.health.backupOff}</p>
            <TextLink to={settingsPath('backups')} className="-ml-3">
              {t.system.health.checks.openSettings}
            </TextLink>
          </div>
        ) : (
          <BackupStatus backup={health.data.backup} />
        )}
      </Panel>
    </Anchor>
  )
}

function TaskButtonLabelled({
  task,
  act,
  running,
  label,
}: {
  task: Task
  act: Act
  running: boolean
  label: string
}) {
  const { t } = useI18n()
  if (running) return <TaskButton task={task} act={act} />
  const busy = act.isPending && act.variables.task.id === task.id
  return (
    <Button
      size="sm"
      icon={PlayIcon}
      aria-label={t.system.schedule.runLabel(task.name)}
      disabled={act.isPending && !busy}
      loading={busy}
      onClick={() => act.mutate({ task, action: 'run' })}
    >
      {label}
    </Button>
  )
}

/** Calls `refetch` each time `value` changes after the first render. */
function useRefetchOn(value: string | undefined, refetch: () => void) {
  const seen = useRef(value)
  useEffect(() => {
    if (seen.current === value) return
    seen.current = value
    refetch()
  }, [value, refetch])
}

function Timers() {
  const { language, t } = useI18n()
  const text = t.system.schedule
  const timers = useQuery({
    queryKey: queryKeys.timers,
    queryFn: ({ signal }) => fetchTimers(signal),
    refetchInterval: 30_000,
  })
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })

  return (
    <Anchor id="recordings">
      <Block title={text.recordingsTitle} count={timers.data?.timers.length || undefined}>
        {timers.isPending ? (
          <SkeletonRows rows={2} />
        ) : timers.isError ? (
          <InlineError onRetry={() => void timers.refetch()} retrying={timers.isFetching}>
            {errorMessage(t, timers.error)}
          </InlineError>
        ) : !timers.data.available ? (
          <EmptyState icon={RecordIcon} title={text.recordingsOff} />
        ) : timers.data.timers.length === 0 ? (
          <EmptyState icon={RecordIcon} title={text.noTimers}>
            {text.recordingsHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.recordingsTitle}>
            {timers.data.timers.map((timer) => (
              <Row
                key={timer.id}
                leading={RecordIcon}
                title={timer.name}
                titleAside={
                  <span className="flex shrink-0 gap-1">
                    {timer.series && <Badge>{text.series}</Badge>}
                    {timer.status === 'InProgress' && (
                      <StatusPill tone="live">{text.recordingNow}</StatusPill>
                    )}
                    {timer.status === 'Error' && (
                      <StatusPill tone="danger">{text.failed}</StatusPill>
                    )}
                    {timer.tunersShort && <TunersShort text={t.livetv} />}
                  </span>
                }
                meta={
                  <span className="truncate">
                    {timer.channel || text.unknownChannel} · {text.scheduledBy(timer.userName)}
                  </span>
                }
                trailing={
                  <time dateTime={timer.from} className="text-small text-ink tabular-nums">
                    {dateTime(timer.from, language)} – {time.format(new Date(timer.until))}
                  </time>
                }
              />
            ))}
          </RowList>
        )}
      </Block>
    </Anchor>
  )
}

/** When the server's IPTV lists and guides are downloaded again. */
function Refreshes() {
  const { t } = useI18n()
  const text = t.system.schedule
  // The server's IPTV sources and guides, then each user's own.
  const sources = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
    refetchInterval: 30_000,
  })
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const rows = [
    ...(sources.data?.addons ?? [])
      .filter((addon) => addon.enabled && addon.source !== null)
      .map((addon) => ({
        key: `iptv-${addon.id}`,
        name: addon.name,
        owner: addon.owner,
        // A source importing movies or series downloads their lists on the same schedule.
        kind:
          addon.source?.options.movies || addon.source?.options.series
            ? text.iptvLists
            : text.channelList,
        last: addon.source?.fetchedAt ?? null,
        next: addon.source?.nextAt ?? null,
      })),
    ...(sources.data?.guides ?? []).flatMap((library) =>
      (library.guides ?? []).map((guide) => ({
        key: `guide-${library.addonId}-${library.catalogType}-${library.catalogId}-${guide.position}`,
        name: library.name ?? library.catalogName,
        owner: library.owner,
        kind: `${text.guide} ${guide.position}`,
        last: guide.fetchedAt,
        next: guide.nextAt,
      })),
    ),
  ]
    // The server's first, then each user's, in the order the server lists them; by next fetch within.
    .map((row, index) => ({ row, index }))
    .sort(
      (a, b) =>
        Number(a.row.owner !== null) - Number(b.row.owner !== null) ||
        (a.row.owner?.name ?? '').localeCompare(b.row.owner?.name ?? '') ||
        (a.row.next ?? '').localeCompare(b.row.next ?? '') ||
        a.index - b.index,
    )
    .map(({ row }) => row)

  return (
    <Anchor id="refreshes">
      <Block title={text.refreshesTitle} count={sources.data ? rows.length : undefined}>
        {settings.data ? (
          <p className="-mt-2 mb-4 text-small text-ink-3">
            {text.refreshesHelp(settings.data.liveTvRefreshHours)}
          </p>
        ) : settings.isPending ? (
          <Skeleton className="-mt-2 mb-4 h-3 w-3/4" />
        ) : null}
        {sources.isPending ? (
          <SkeletonRows rows={2} />
        ) : sources.isError ? (
          <InlineError onRetry={() => void sources.refetch()} retrying={sources.isFetching}>
            {errorMessage(t, sources.error)}
          </InlineError>
        ) : rows.length === 0 ? (
          <EmptyState icon={CalendarDotsIcon} title={text.noRefreshes}>
            {text.refreshesHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.refreshesTitle}>
            {rows.map((row) => (
              <Row
                key={row.key}
                leading={CalendarDotsIcon}
                title={row.name}
                titleAside={<OwnerChip owner={row.owner} />}
                meta={
                  <span className="truncate">
                    {row.kind} · {text.lastDownload}
                    {t.common.colon} {row.last ? <RelativeTime iso={row.last} /> : text.notYet}
                  </span>
                }
                trailing={
                  <span className="text-small tabular-nums">
                    <span className="block text-ink-3">{text.nextDownload}</span>
                    <span className="block text-ink">
                      {row.next === null || Date.parse(row.next) <= Date.now() ? (
                        text.due
                      ) : (
                        <RelativeTime iso={row.next} />
                      )}
                    </span>
                  </span>
                }
              />
            ))}
          </RowList>
        )}
      </Block>
    </Anchor>
  )
}
