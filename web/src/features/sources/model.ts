import {
  BroadcastIcon,
  CloudIcon,
  FolderIcon,
  GlobeIcon,
  HardDrivesIcon,
  MusicNotesIcon,
  PlugsConnectedIcon,
  PuzzlePieceIcon,
  TerminalWindowIcon,
  type Icon,
} from '@phosphor-icons/react'
import { queryClient, queryKeys, type Addon, type LocalFolder, type Owner, type Scope } from '@/api'

/** The kinds the list filters by: IPTV gathers M3U playlists and Xtream accounts. */
export type KindFilter = 'stremio' | 'eclipse' | 'iptv' | 'local'

export const isIptv = (addon: Addon) => addon.kind === 'm3u' || addon.kind === 'xtream'

export const kindOf = (addon: Addon): KindFilter =>
  (isIptv(addon) ? 'iptv' : addon.kind) as KindFilter

export function kindIcon(addon: Addon): Icon {
  if (isIptv(addon)) return BroadcastIcon
  if (addon.folder?.share === 'smb') return HardDrivesIcon
  if (addon.folder?.share === 'webdav') return GlobeIcon
  if (addon.folder?.share === 's3') return CloudIcon
  if (addon.folder?.share === 'sftp') return TerminalWindowIcon
  if (addon.folder?.share === 'rclone') return PlugsConnectedIcon
  if (addon.kind === 'local') return FolderIcon
  return addon.kind === 'eclipse' ? MusicNotesIcon : PuzzlePieceIcon
}

/** The page of a source the signed-in user can manage. */
export const sourcePath = (scope: Scope, id: string, section?: string) =>
  `${scope === 'me' ? '/me/sources' : '/sources/shared'}/${encodeURIComponent(id)}${section ? `/${section}` : ''}`

/**
 * One row of the list: a source and who owns it. `scope` is null for another user's source, which
 * the signed-in administrator sees but cannot change.
 */
export type Entry = {
  key: string
  addon: Addon
  owner: Owner
  scope: Scope | null
}

/**
 * Adding, changing, reordering or removing a source can change the scope's libraries too, and a
 * failure (404, invalid_order) means the list is stale: refetch the scope (and the list of every
 * user's sources) after every change.
 */
export function invalidateScope(scope: Scope) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
  void queryClient.invalidateQueries({ queryKey: queryKeys.sources })
}

export function replaceCachedAddon(scope: Scope, updated: Addon) {
  queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
    old?.map((addon) => (addon.id === updated.id ? updated : addon)),
  )
}

/**
 * When a source was last read: its manifest, its IPTV list, or its local folder's last scan (null
 * before the first download or scan).
 */
export function lastTime(addon: Addon) {
  if (addon.folder !== null) return addon.folder.scannedAt
  return addon.source === null ? addon.refreshedAt : addon.source.fetchedAt
}

/**
 * Why a local folder cannot be read, '' when it can: its last scan's failure, or the host key its
 * SFTP server offers in place of the one saved, which Polyfin refuses before a scan meets it.
 */
export function folderFailure(folder: LocalFolder): string {
  if (folder.error !== '') return folder.error
  return folder.offeredHostKey !== '' ? 'host_key_changed' : ''
}
