import type { Settings } from '@/api'
import { computer, useServerSystem } from '@/features/sources/LocalFolders'
import { useI18n } from '@/i18n'
import { TextInput } from '@/ui'
import { FieldRow } from './parts'
import type { SectionFormApi } from './SectionForm'

/** The settings that choose a folder for the files Polyfin works on. */
export type WorkFolder = 'conversionFolder' | 'readingFolder' | 'cacheFolder'

/** The anchor of each, which its errors are shown at. */
export const workFolderAnchors: Record<WorkFolder, string> = {
  conversionFolder: 'conversion-folder',
  readingFolder: 'reading-folder',
  cacheFolder: 'cache-folder',
}

/**
 * The conversion folder, the folder for files being read or the cache folder: empty for the
 * cache in the data folder, shown as its placeholder. A new folder applies at the next start.
 */
export default function WorkFolderRow({
  folder,
  form,
  update,
  error,
  limits,
}: Pick<SectionFormApi, 'form' | 'update' | 'error' | 'limits'> & { folder: WorkFolder }) {
  const { t } = useI18n()
  const s = t.settings.workFolders
  const host = computer(useServerSystem())
  const anchor = workFolderAnchors[folder]
  return (
    <FieldRow
      anchor={anchor}
      label={s[folder]}
      help={`${s[`${folder}Help`]} ${s.where[host]}`}
      error={error(anchor)}
    >
      <TextInput
        value={form[folder]}
        onValue={(value) => update({ [folder]: value } as Pick<Settings, WorkFolder>)}
        placeholder={form.cacheFolderDefault}
        maxLength={limits(folder).max}
        autoComplete="off"
        spellCheck={false}
        mono
        className="max-w-md"
      />
    </FieldRow>
  )
}
