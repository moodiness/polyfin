import type { Addon, Library, Scope } from '@/api'
import { catalogGuidesPath, lineupPath } from '@/components/lineup/common'
import type { Messages } from '@/i18n'
import type { StatusTone } from '@/ui'

/** Whether a catalog lists live channels: its channels appear in Live TV in Jellyfin apps. */
export function isTvCatalog(library: Pick<Library, 'catalogType'>): boolean {
  return library.catalogType === 'tv'
}

/** Whether an addon is an IPTV source, whose TV catalog is its line-up. */
export function isIptv(addon: Pick<Addon, 'kind'>): boolean {
  return addon.kind === 'm3u' || addon.kind === 'xtream'
}

/**
 * Where a TV catalog's guides are managed: an IPTV source's on its page, with its line-up; a
 * Stremio catalog's on its own guides page.
 */
export function guidesLink(scope: Scope, library: Library, iptv: boolean): string {
  return iptv
    ? lineupPath(scope, library.addonId, 'guides')
    : catalogGuidesPath(scope, {
        addonId: library.addonId,
        catalogType: library.catalogType,
        catalogId: library.catalogId,
      })
}

/** The guides of a saved TV catalog whose last download failed. */
export function failingGuides(library: Library) {
  return (library.guides ?? []).filter((guide) => guide.error !== '')
}

/**
 * The state of a TV catalog as one word and a tone: whether it is in Live TV, and how far its
 * guides cover its channels.
 */
export function catalogState(
  t: Messages,
  library: Library,
  addonOff: boolean,
): { tone: StatusTone; label: string } {
  const states = t.livetv.states
  if (addonOff) return { tone: 'danger', label: states.addonOff }
  if (!library.browsable) return { tone: 'danger', label: states.missing }
  if (!library.enabled) return { tone: 'muted', label: states.notShown }
  const guides = library.guides ?? []
  if (failingGuides(library).length > 0) return { tone: 'danger', label: states.failed }
  if (guides.length === 0) return { tone: 'warn', label: states.noGuide }
  if (guides.every((guide) => guide.fetchedAt === null)) {
    return { tone: 'muted', label: states.notFetched }
  }
  const guide = library.guide
  if (guide !== null && guide.channels > 0 && guide.matched < guide.channels) {
    return { tone: 'warn', label: states.incomplete }
  }
  return { tone: 'ok', label: states.complete }
}

/** "1 guide · 13 of 18 channels with a guide", or "No programme guide". */
export function guideSummary(t: Messages, language: string, library: Library): string {
  const guides = library.guides ?? []
  if (guides.length === 0) return t.livetv.guideNone
  const parts = [t.livetv.guides(guides.length)]
  if (library.guide !== null && library.guide.channels > 0) {
    parts.push(
      t.livetv.mapped(
        library.guide.matched.toLocaleString(language),
        library.guide.channels.toLocaleString(language),
      ),
    )
  }
  return parts.join(' · ')
}
