import { queryClient, queryKeys, type CatalogTarget, type Scope } from '@/api'

/** The scope a route names, or null for an unknown one. */
export function routeScope(value: string | undefined): Scope | null {
  return value === 'shared' || value === 'me' ? value : null
}

/** The Live TV catalog of an IPTV source. */
export function iptvCatalog(id: string): CatalogTarget {
  return { addonId: id, catalogType: 'tv', catalogId: 'channels' }
}

/** After any change to a source's line-up, everything that shows it is read again. */
export function invalidateLineup(scope: Scope, id: string) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.lineup(scope, id) })
  void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
  void queryClient.invalidateQueries({ queryKey: queryKeys.catalogGuides(scope, iptvCatalog(id)) })
  void queryClient.invalidateQueries({ queryKey: queryKeys.sources })
}

/**
 * The address of a source's line-up page, or of one of its sections: the server's sources are
 * under Content, a user's own under My sources.
 */
export function lineupPath(scope: Scope, id: string, section?: string): string {
  const base = scope === 'me' ? `/me/sources/${id}` : `/sources/shared/${id}`
  return `${base}${section ? `/${section}` : ''}`
}

/** The address of a Stremio Live TV catalog's guides page, or of its mapping. */
export function catalogGuidesPath(scope: Scope, target: CatalogTarget, mapping = false): string {
  return `/live-tv/guides/${scope}/${target.addonId}/${encodeURIComponent(target.catalogId)}${mapping ? '/mapping' : ''}`
}
