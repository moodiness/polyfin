import { useQuery } from '@tanstack/react-query'
import {
  fetchHealth,
  fetchHealthProblems,
  queryKeys,
  type HealthProblem,
  type Owner,
  type UnreadableSecret,
} from '@/api'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'

/** The server's health, refreshed every 10 seconds. */
export function useHealth() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: ({ signal }) => fetchHealth(signal),
    refetchInterval: 10_000,
  })
}

/** The problems the server finds, errors first, refreshed every 10 seconds. */
export function useHealthProblems() {
  const { language } = useI18n()
  return useQuery({
    queryKey: queryKeys.healthProblems(language),
    queryFn: async ({ signal }) => (await fetchHealthProblems(language, signal)).problems,
    refetchInterval: 10_000,
  })
}

/** A row's name, followed by whose it is when it is a user's own. */
export function ownedName(t: Messages, name: string, owner: Owner): string {
  return owner === null ? name : `${name} (${t.system.health.owner.user(owner.name)})`
}

/**
 * The name of a stored secret the key cannot decrypt: a server setting, a user's connection, or a
 * notification target of a user or of the server.
 */
export function unreadableName(t: Messages, secret: UnreadableSecret): string {
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

/** What a problem the server found says. */
export function problemText(t: Messages, language: string, problem: HealthProblem): string {
  const health = t.system.health
  const text = health.problems
  switch (problem.code) {
    case 'database':
      return text.database
    case 'disk':
      return text.disk(health.folders[problem.folder], formatBytes(problem.free, language))
    case 'addon':
      return text.addon(
        ownedName(t, problem.name, problem.owner ?? null),
        health.failures[problem.failure] ?? problem.failure,
      )
    case 'conversions_full':
      return text.conversionsFull
    case 'thumbnails_paused':
      return text.paused(problem.host)
    case 'secrets_unreadable':
      return text.unreadableSecrets(
        problem.secrets.map((secret) => unreadableName(t, secret)).join(', '),
      )
    case 'secrets_plaintext':
      return text.plaintextSecrets
    case 'backup_failed':
      return text.backupFailed
    case 'backup_stale':
      return text.backupStale
    case 'iptv':
      return text.iptv(ownedName(t, problem.name, problem.owner ?? null))
    case 'guide':
      return text.guide(ownedName(t, problem.name, problem.owner ?? null))
    case 'folder':
      return text.folder(problem.name)
    case 'task':
      return text.task(problem.task)
  }
}
