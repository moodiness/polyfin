import { useEffect, useRef } from 'react'
import {
  useMutation,
  useQuery,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query'
import {
  ArchiveIcon,
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
  SkeletonRows,
  SkeletonText,
  StatusPill,
  useToast,
  RelativeTime,
} from '@/ui'
import { dateTime, errorMessage, formatHour, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import BackupStatus from './BackupStatus'
import OwnerChip from './OwnerChip'
import { Anchor } from './parts'

/** The task that backs the database up, registered when POLYFIN_BACKUP_DIR is set. */
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

const columns = 'md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(8.5rem,auto)]'

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
          <SkeletonRows rows={4} />
        ) : tasks.isError ? (
          <InlineError onRetry={() => void tasks.refetch()} retrying={tasks.isFetching}>
            {errorMessage(t, tasks.error)}
          </InlineError>
        ) : tasks.data.length === 0 ? (
          <EmptyState icon={ListChecksIcon} title={text.noTasks}>
            {text.noTasksHint}
          </EmptyState>
        ) : (
          <div className="space-y-6">
            <div
              aria-hidden="true"
              className={`hidden gap-4 px-[18px] text-small font-medium text-ink-3 md:grid ${columns}`}
            >
              <span>{text.task}</span>
              <span>{text.lastRun}</span>
              <span>{text.nextRun}</span>
              <span />
            </div>
            {[...categories].map(([category, list]) => (
              <section key={category} aria-label={category}>
                <h3 className="mb-2.5 px-1 text-micro font-semibold tracking-[0.06em] text-ink-3 uppercase">
                  {category}
                </h3>
                <ul className="overflow-hidden rounded-panel border border-line-2 bg-s1">
                  {list.map((task) => (
                    <li
                      key={task.id}
                      className={`grid gap-3 px-[18px] py-4 not-first:border-t not-first:border-line md:items-center md:gap-4 max-sm:px-4 ${columns}`}
                    >
                      <div className="min-w-0">
                        <p className="text-[14.5px] font-medium text-ink">{task.name}</p>
                        <p className="mt-0.5 text-small text-ink-3">{task.description}</p>
                      </div>
                      <div className="text-small">
                        <p className="text-ink-3 md:sr-only">{text.lastRun}</p>
                        <LastRun task={task} />
                      </div>
                      <NextRun task={task} />
                      <div className="flex gap-2 md:justify-end">
                        <TaskButton task={task} act={act} />
                      </div>
                    </li>
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

function NextRun({ task }: { task: Task }) {
  const { language, t } = useI18n()
  const text = t.system.schedule
  return (
    <div className="text-small tabular-nums">
      <p className="text-ink-3 md:sr-only">{text.nextRun}</p>
      {task.next !== null ? (
        <p className="text-ink">
          <RelativeTime iso={task.next} />
        </p>
      ) : (
        <p className="text-ink-3">{text.byHandOnly}</p>
      )}
      {task.interval > 0 && (
        <p className="text-ink-3">{text.every(formatSpan(task.interval, language))}</p>
      )}
      {task.daily !== null && (
        <p className="text-ink-3">{text.daily(formatHour(task.daily, language))}</p>
      )}
    </div>
  )
}

function LastRun({ task }: { task: Task }) {
  const { language, t } = useI18n()
  const text = t.system.schedule
  if (task.state !== 'Idle') {
    return (
      <StatusPill tone="live">
        <span className="motion-safe:animate-pulse">
          {task.state === 'Running' ? text.running : text.cancelling}
        </span>
      </StatusPill>
    )
  }
  if (task.last === null) return <p className="text-ink-3">{text.notRunYet}</p>
  const last = task.last
  const tone = last.status === 'Completed' ? 'ok' : last.status === 'Failed' ? 'danger' : 'muted'
  return (
    <div>
      <StatusPill tone={tone}>{text.results[last.status] ?? last.status}</StatusPill>
      <p className="mt-0.5 text-ink-3 tabular-nums">
        <RelativeTime iso={last.end} />
        {' · '}
        {text.took(formatSpan((Date.parse(last.end) - Date.parse(last.start)) / 1000, language))}
      </p>
      {last.error !== '' && <p className="mt-0.5 break-words text-danger">{last.error}</p>}
    </div>
  )
}

/** The backups' state, from the health, with the backup task's own button. */
function Backups({ tasks, act }: { tasks: Task[] | undefined; act: Act }) {
  const { t } = useI18n()
  const text = t.system.schedule
  // The Health page polls this every 10 seconds; here it follows the backup task's runs.
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
          task && (
            <TaskButtonLabelled task={task} act={act} running={running} label={text.backUpNow} />
          )
        }
      >
        {health.isPending ? (
          <SkeletonText lines={3} />
        ) : health.isError ? (
          <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
            {errorMessage(t, health.error)}
          </InlineError>
        ) : health.data.backup === null ? (
          <EmptyState icon={ArchiveIcon} title={t.system.health.backupTitle}>
            {t.system.health.backupOff}
          </EmptyState>
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
