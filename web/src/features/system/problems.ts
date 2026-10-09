import { useQuery } from '@tanstack/react-query'
import {
  fetchHealth,
  fetchSources,
  fetchTasks,
  queryKeys,
  type Health,
  type Owner,
  type Sources,
  type Task,
} from '@/api'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'

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
  // The server's addons, IPTV sources and guides, then each user's own.
  const sources = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
    refetchInterval: 30_000,
  })
  const tasks = useQuery({
    queryKey: queryKeys.tasks(language),
    queryFn: ({ signal }) => fetchTasks(language, signal),
    refetchInterval: 10_000,
  })
  return { health, sources, tasks }
}

/** A row's name, followed by whose it is when it is a user's own. */
export function ownedName(t: Messages, name: string, owner: Owner): string {
  return owner === null ? name : `${name} (${t.system.health.owner.user(owner.name)})`
}

/**
 * The name of a stored secret the key cannot decrypt: a server setting, a user's connection, or a
 * notification target of a user or of the server.
 */
export function unreadableName(
  t: Messages,
  secret: NonNullable<Health['secrets']>['unreadable'][number],
): string {
  const text = t.system.health
  if (secret.setting !== null) return text.secretNames[secret.setting]
  if (secret.target !== null) {
    return secret.user === null
      ? text.serverTargetOf(secret.target)
      : text.targetOf(secret.target, secret.user)
  }
  return text.connectionOf(serviceNames[secret.service ?? 'trakt'], secret.user ?? '')
}

/** The tracking services by their names. */
const serviceNames = {
  trakt: 'Trakt',
  simkl: 'Simkl',
  mdblist: 'MDBList',
  publicmetadb: 'PublicMetaDB',
  lastfm: 'Last.fm',
  listenbrainz: 'ListenBrainz',
}

/** The problems the health data shows, errors first; users' own sources count too. */
export function findProblems(
  t: Messages,
  language: string,
  health: Health | undefined,
  sources: Sources | undefined,
  tasks: Task[] | undefined,
): Problem[] {
  const text = t.system.health.problems
  const problems: Problem[] = []
  if (health !== undefined) {
    if (!health.database.reachable) {
      problems.push({ tone: 'error', text: text.database, to: '/system/health#database' })
    }
    for (const disk of health.disks) {
      if (lowOnSpace(disk)) {
        problems.push({
          tone: 'warning',
          text: text.disk(t.system.health.folders[disk.folder], formatBytes(disk.free, language)),
          to: '/system/health#disks',
        })
      }
    }
    for (const addon of health.addons) {
      if (addon.enabled && addon.failure !== '') {
        problems.push({
          tone: 'warning',
          text: text.addon(
            ownedName(t, addon.name, addon.owner),
            t.system.health.failures[addon.failure] ?? addon.failure,
          ),
          to: '/system/health#addons',
        })
      }
    }
    const transcoder = health.transcoder
    if (transcoder && transcoder.limit > 0 && transcoder.conversions >= transcoder.limit) {
      problems.push({
        tone: 'warning',
        text: text.conversionsFull,
        to: '/system/health#transcoder',
      })
    }
    for (const paused of health.thumbnails?.pausedHosts ?? []) {
      problems.push({
        tone: 'warning',
        text: text.paused(paused.host),
        to: '/system/health#thumbnails',
      })
    }
    const secrets = health.secrets
    if (secrets !== null && secrets.unreadable.length > 0) {
      problems.push({
        tone: 'error',
        text: text.unreadableSecrets(
          secrets.unreadable.map((secret) => unreadableName(t, secret)).join(', '),
        ),
        to: '/system/health#secrets',
      })
    }
    if (secrets !== null && !secrets.encrypted && secrets.plaintext > 0) {
      problems.push({ tone: 'warning', text: text.plaintextSecrets, to: '/system/health#secrets' })
    }
    const backup = health.backup
    if (backup?.problem) {
      problems.push(
        backup.error !== ''
          ? { tone: 'error', text: text.backupFailed, to: '/system/health#backups' }
          : { tone: 'warning', text: text.backupStale, to: '/system/health#backups' },
      )
    }
  }
  for (const addon of sources?.addons ?? []) {
    if (addon.enabled && addon.source !== null && addon.source.error !== '') {
      problems.push({
        tone: 'warning',
        text: text.iptv(ownedName(t, addon.name, addon.owner)),
        to: '/system/health#iptv',
      })
    }
  }
  for (const library of sources?.guides ?? []) {
    if ((library.guides ?? []).some((guide) => guide.error !== '')) {
      problems.push({
        tone: 'warning',
        text: text.guide(ownedName(t, library.name ?? library.catalogName, library.owner)),
        to: '/system/health#guides',
      })
    }
  }
  for (const task of tasks ?? []) {
    // A failed backup is told above, with what failed.
    if (task.key === 'BackUpDatabase' && health?.backup?.error) continue
    if (task.last?.status === 'Failed') {
      problems.push({ tone: 'warning', text: text.task(task.name), to: '/system/schedule' })
    }
  }
  return problems.sort((a, b) => (a.tone === b.tone ? 0 : a.tone === 'error' ? -1 : 1))
}
