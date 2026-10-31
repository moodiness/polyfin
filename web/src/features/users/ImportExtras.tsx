import { useId } from 'react'
import type { ImportExtras } from '@/api'
import { useI18n } from '@/i18n'
import { Checkbox } from '@/ui'

/** Every extra an import brings, as each is by default. */
export const allExtras: ImportExtras = { ratings: true, collections: true, playlists: true }

/**
 * What an import from another server brings besides watch data: ratings, collections and
 * playlists. Without `collections`, the user may not manage collections, and the choice is not
 * offered.
 */
export function ExtrasChoice({
  value,
  onChange,
  collections = true,
  plex = false,
}: {
  value: ImportExtras
  onChange: (value: ImportExtras) => void
  collections?: boolean
  /** Plex gives ratings and playlists only for the accounts read with their own token. */
  plex?: boolean
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport.extras
  const id = useId()
  return (
    <fieldset className="space-y-3" aria-describedby={plex ? `${id}-help` : undefined}>
      <legend className="mb-1 text-[15px] font-medium text-ink">{text.title}</legend>
      {plex && (
        <p id={`${id}-help`} className="text-small text-ink-3">
          {text.plexHelp}
        </p>
      )}
      <Checkbox
        label={text.ratings}
        help={text.ratingsHelp}
        checked={value.ratings}
        onChange={(ratings) => onChange({ ...value, ratings })}
      />
      {collections && (
        <Checkbox
          label={text.collections}
          help={text.collectionsHelp}
          checked={value.collections}
          onChange={(chosen) => onChange({ ...value, collections: chosen })}
        />
      )}
      <Checkbox
        label={text.playlists}
        help={text.playlistsHelp}
        checked={value.playlists}
        onChange={(playlists) => onChange({ ...value, playlists })}
      />
    </fieldset>
  )
}
