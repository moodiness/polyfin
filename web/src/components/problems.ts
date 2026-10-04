import { useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  fetchHealth,
  fetchLibraries,
  fetchTasks,
  queryKeys,
  type Addon,
  type Health,
  type Library,
  type Task,
} from '@/api'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n/en'

/** Something an administrator should look at, linking to where it is described. */
export type Problem = { tone: 'error' | 'warning'; text: string; to: string }

/** Below this much free space, or this share of the disk, a folder's disk is short of room. */
const lowDiskBytes = 2_000_000_000
const lowDiskShare = 0.05

/** Whether a measured disk is short of room; `free` is -1 when it could not be measured. */
export function lowOnSpace(disk: Health['disks'][number]): boolean {
  return (
    disk.free >= 0 &&
    (disk.free < lowDiskBytes || disk.free < (disk.free + disk.used) * lowDiskShare)
  )
}

/** The server's health and what it is built from, refreshed every 10 seconds. */
export function useHealthData() {
  const { language } = useI18n()
  const health = useQuery({
    queryKey: queryKeys.health,
    queryFn: ({ signal }) => fetchHealth(signal),
    refetchInterval: 10_000,
  })
  const addons = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
    refetchInterval: 30_000,
  })
  const libraries = useQuery({
    queryKey: queryKeys.libraries('shared'),
    queryFn: ({ signal }) => fetchLibraries('shared', signal),
    refetchInterval: 30_000,
  })
  const tasks = useQuery({
    queryKey: queryKeys.tasks(language),
    queryFn: ({ signal }) => fetchTasks(language, signal),
    refetchInterval: 10_000,
  })
  return { health, addons, libraries, tasks }
}

/** The problems the health data shows, errors first. */
export function findProblems(
  t: Messages,
  language: string,
  health: Health | undefined,
  addons: Addon[] | undefined,
  libraries: Library[] | undefined,
  tasks: Task[] | undefined,
): Problem[] {
  const text = t.dashboard.health.problems
  const problems: Problem[] = []
  if (health !== undefined) {
    if (!health.database.reachable) {
      problems.push({ tone: 'error', text: text.database, to: '/health#database' })
    }
    for (const disk of health.disks) {
      if (lowOnSpace(disk)) {
        problems.push({
          tone: 'warning',
          text: text.disk(
            t.dashboard.health.folders[disk.folder],
            formatBytes(disk.free, language),
          ),
          to: '/health#disks',
        })
      }
    }
    for (const addon of health.addons) {
      if (addon.enabled && addon.failure !== '') {
        problems.push({
          tone: 'warning',
          text: text.addon(addon.name, t.dashboard.health.failures[addon.failure] ?? addon.failure),
          to: '/health#addons',
        })
      }
    }
    const transcoder = health.transcoder
    if (transcoder && transcoder.limit > 0 && transcoder.conversions >= transcoder.limit) {
      problems.push({ tone: 'warning', text: text.conversionsFull, to: '/health#transcoder' })
    }
    for (const paused of health.thumbnails?.pausedHosts ?? []) {
      problems.push({ tone: 'warning', text: text.paused(paused.host), to: '/health#thumbnails' })
    }
  }
  for (const addon of addons ?? []) {
    if (addon.enabled && addon.source !== null && addon.source.error !== '') {
      problems.push({ tone: 'warning', text: text.iptv(addon.name), to: '/health#iptv' })
    }
  }
  for (const library of libraries ?? []) {
    if (library.guide !== null && library.guide.url !== '' && library.guide.error !== '') {
      problems.push({
        tone: 'warning',
        text: text.guide(library.name ?? library.catalogName),
        to: '/health#guides',
      })
    }
  }
  for (const task of tasks ?? []) {
    if (task.last?.status === 'Failed') {
      problems.push({ tone: 'warning', text: text.task(task.name), to: '/schedule' })
    }
  }
  return problems.sort((a, b) => (a.tone === b.tone ? 0 : a.tone === 'error' ? -1 : 1))
}
