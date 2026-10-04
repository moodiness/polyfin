import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
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
import { icons } from '@/components/icons'
import OwnerChip from '@/components/OwnerChip'
import { Empty, Panel, Skeleton, StatusText } from '@/components/panels'
import { Badge, buttonSecondary, Notice, PageHeader, RelativeTime } from '@/components/ui'
import { dateTime, errorMessage, formatSpan } from '@/format'
import { useI18n } from '@/i18n'

export default function SchedulePage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader
        title={t.dashboard.schedule.title}
        description={t.dashboard.schedule.description}
      />
      <div className="space-y-6">
        <Tasks />
        <div className="grid gap-6 xl:grid-cols-2">
          <Timers />
          <Refreshes />
        </div>
      </div>
    </>
  )
}

function Tasks() {
  const { language, t } = useI18n()
  const text = t.dashboard.schedule
  const tasks = useQuery({
    queryKey: queryKeys.tasks(language),
    queryFn: ({ signal }) => fetchTasks(language, signal),
    // Faster while a task runs, to show it end.
    refetchInterval: (query) =>
      query.state.data?.some((task) => task.state !== 'Idle') ? 1_000 : 5_000,
  })
  const [notice, setNotice] = useState<string | null>(null)
  const act = useMutation({
    mutationFn: ({ task, action }: { task: Task; action: 'run' | 'stop' }) =>
      action === 'run' ? runTask(task.id) : stopTask(task.id),
    onSuccess: (_, { task, action }) => {
      setNotice(action === 'run' ? text.started(task.name) : null)
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
  const categories = new Map<string, Task[]>()
  for (const task of tasks.data ?? []) {
    categories.set(task.category, [...(categories.get(task.category) ?? []), task])
  }
  const columns = 'md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(8.5rem,auto)]'

  return (
    <Panel id="tasks" title={text.tasksTitle} description={text.autoRefresh}>
      <div aria-live="polite" className="empty:hidden mb-4 space-y-2">
        {notice !== null && <Notice kind="success">{notice}</Notice>}
        {act.isError && <Notice kind="error">{errorMessage(t, act.error)}</Notice>}
      </div>
      {tasks.isPending ? (
        <Skeleton rows={4} label={t.common.loading} />
      ) : tasks.isError ? (
        <Notice kind="error">{errorMessage(t, tasks.error)}</Notice>
      ) : (
        <div className="space-y-5">
          <div
            aria-hidden="true"
            className={`hidden gap-3 px-4 text-xs font-medium text-muted md:grid ${columns}`}
          >
            <span>{text.task}</span>
            <span>{text.lastRun}</span>
            <span>{text.nextRun}</span>
            <span />
          </div>
          {[...categories].map(([category, list]) => (
            <div key={category}>
              <h3 className="mb-2 text-xs font-semibold tracking-wide text-muted uppercase">
                {category}
              </h3>
              <ul className="divide-y divide-line rounded-xl border border-line">
                {list.map((task) => (
                  <li
                    key={task.id}
                    className={`grid gap-3 px-4 py-3 text-sm md:items-center ${columns}`}
                  >
                    <div className="min-w-0">
                      <p className="font-medium text-white">{task.name}</p>
                      <p className="mt-0.5 text-xs text-muted">{task.description}</p>
                    </div>
                    <div className="text-xs">
                      <p className="text-muted md:sr-only">{text.lastRun}</p>
                      <LastRun task={task} />
                    </div>
                    <div className="text-xs">
                      <p className="text-muted md:sr-only">{text.nextRun}</p>
                      {task.next !== null ? (
                        <p className="text-zinc-100">
                          <RelativeTime iso={task.next} />
                        </p>
                      ) : (
                        <p className="text-muted">{text.byHandOnly}</p>
                      )}
                      {task.interval > 0 && (
                        <p className="text-muted">
                          {text.every(formatSpan(task.interval, language))}
                        </p>
                      )}
                    </div>
                    <div className="flex gap-2 md:justify-end">
                      {task.state === 'Idle' ? (
                        <button
                          type="button"
                          className={buttonSecondary}
                          aria-label={text.runLabel(task.name)}
                          disabled={act.isPending}
                          onClick={() => act.mutate({ task, action: 'run' })}
                        >
                          <icons.play className="size-4" />
                          {text.runNow}
                        </button>
                      ) : (
                        <button
                          type="button"
                          className={buttonSecondary}
                          aria-label={text.stopLabel(task.name)}
                          disabled={act.isPending || task.state === 'Cancelling'}
                          onClick={() => act.mutate({ task, action: 'stop' })}
                        >
                          <icons.stop className="size-4" />
                          {task.state === 'Cancelling' ? text.cancelling : text.stop}
                        </button>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </Panel>
  )
}

function LastRun({ task }: { task: Task }) {
  const { language, t } = useI18n()
  const text = t.dashboard.schedule
  if (task.state !== 'Idle') {
    return (
      <StatusText tone="active">
        <span className="motion-safe:animate-pulse">
          {task.state === 'Running' ? text.running : text.cancelling}
        </span>
      </StatusText>
    )
  }
  if (task.last === null) return <p className="text-muted">{text.notRunYet}</p>
  const last = task.last
  const tone = last.status === 'Completed' ? 'ok' : last.status === 'Failed' ? 'error' : 'muted'
  return (
    <div>
      <StatusText tone={tone}>{text.results[last.status] ?? last.status}</StatusText>
      <p className="mt-0.5 text-muted">
        <RelativeTime iso={last.end} />
        {' · '}
        {text.took(formatSpan((Date.parse(last.end) - Date.parse(last.start)) / 1000, language))}
      </p>
      {last.error !== '' && <p className="mt-0.5 break-words text-rose-300">{last.error}</p>}
    </div>
  )
}

function Timers() {
  const { language, t } = useI18n()
  const text = t.dashboard.schedule
  const timers = useQuery({
    queryKey: queryKeys.timers,
    queryFn: ({ signal }) => fetchTimers(signal),
    refetchInterval: 30_000,
  })
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })

  return (
    <Panel id="recordings" title={text.recordingsTitle}>
      {timers.isPending ? (
        <Skeleton rows={2} label={t.common.loading} />
      ) : timers.isError ? (
        <Notice kind="error">{errorMessage(t, timers.error)}</Notice>
      ) : !timers.data.available ? (
        <Empty>{text.recordingsOff}</Empty>
      ) : timers.data.timers.length === 0 ? (
        <Empty>{text.noTimers}</Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {timers.data.timers.map((timer) => (
            <li
              key={timer.id}
              className="flex flex-wrap items-start justify-between gap-3 px-4 py-3 text-sm"
            >
              <div className="min-w-0">
                <p className="font-medium text-white">{timer.name}</p>
                <p className="text-xs text-muted">
                  {timer.channel || text.unknownChannel} · {text.scheduledBy(timer.userName)}
                </p>
              </div>
              <div className="flex flex-col items-end gap-1 text-xs tabular-nums">
                <time dateTime={timer.from} className="text-zinc-100">
                  {dateTime(timer.from, language)} – {time.format(new Date(timer.until))}
                </time>
                <span className="flex gap-1">
                  {timer.series && <Badge tone="muted">{text.series}</Badge>}
                  {timer.status === 'InProgress' && <Badge tone="fin">{text.recordingNow}</Badge>}
                  {timer.status === 'Error' && <Badge tone="danger">{text.failed}</Badge>}
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

/** When the server's IPTV lists and guides are downloaded again. */
function Refreshes() {
  const { t } = useI18n()
  const text = t.dashboard.schedule
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
        kind: text.channelList,
        last: addon.source?.fetchedAt ?? null,
        next: addon.source?.nextAt ?? null,
      })),
    ...(sources.data?.guides ?? [])
      .filter((library) => library.guide !== null && library.guide.url !== '')
      .map((library) => ({
        key: `guide-${library.addonId}-${library.catalogType}-${library.catalogId}`,
        name: library.name ?? library.catalogName,
        owner: library.owner,
        kind: text.guide,
        last: library.guide?.fetchedAt ?? null,
        next: library.guide?.nextAt ?? null,
      })),
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
  const pending = sources.isPending
  const error = sources.error

  return (
    <Panel
      id="refreshes"
      title={text.refreshesTitle}
      description={settings.data ? text.refreshesHelp(settings.data.liveTvRefreshHours) : undefined}
    >
      {pending ? (
        <Skeleton rows={2} label={t.common.loading} />
      ) : error !== null ? (
        <Notice kind="error">{errorMessage(t, error)}</Notice>
      ) : rows.length === 0 ? (
        <Empty>{text.noRefreshes}</Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {rows.map((row) => (
            <li
              key={row.key}
              className="flex flex-wrap items-start justify-between gap-3 px-4 py-3 text-sm"
            >
              <div className="min-w-0">
                <p className="flex flex-wrap items-center gap-2 font-medium text-white">
                  {row.name}
                  <OwnerChip owner={row.owner} />
                </p>
                <p className="text-xs text-muted">
                  {row.kind} · {text.lastDownload}:{' '}
                  {row.last ? <RelativeTime iso={row.last} /> : text.notYet}
                </p>
              </div>
              <div className="text-right text-xs">
                <p className="text-muted">{text.nextDownload}</p>
                <p className="text-zinc-100">
                  {row.next === null || Date.parse(row.next) <= Date.now() ? (
                    text.due
                  ) : (
                    <RelativeTime iso={row.next} />
                  )}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}
