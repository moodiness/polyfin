import { sourceCachePresets, type Settings, type SourceCachePreset } from '@/api'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import { SettingRow } from '@/ui'
import { SettingsGroup } from './parts'
import type { SectionFormApi } from './SectionForm'
import WorkFolderRow from './WorkFolders'

/**
 * The source cache, under Settings › Playback: its preset with the free space of the folder for
 * files being read, the size, days and credit the preset sets, the folder itself, and the
 * next-episode preloading. Choosing a preset sets its values; changing one of them, or whole-file
 * preloading, makes it custom, as the server does for a value saved alone.
 */
export default function SourceCache({
  form,
  saved,
  update,
  error,
  number,
  range,
  limits,
  minMax,
}: Pick<
  SectionFormApi,
  'form' | 'saved' | 'update' | 'error' | 'number' | 'range' | 'limits' | 'minMax'
>) {
  const { language, t } = useI18n()
  const s = t.settings
  const custom = (patch: Partial<Settings>) => update({ ...patch, sourceCachePreset: 'custom' })
  const choose = (preset: SourceCachePreset) => {
    if (preset === 'custom') {
      update({ sourceCachePreset: preset })
      return
    }
    update({ sourceCachePreset: preset, ...form.sourceCachePresetLimits[preset] })
  }
  const label = (preset: SourceCachePreset) => {
    const name = s.sourceCachePresetNames[preset]
    if (preset === 'custom') return name
    const limits = form.sourceCachePresetLimits[preset]
    return s.sourceCachePresetChoice(
      name,
      limits.cacheSizeGb.toLocaleString(language),
      limits.sourceCacheDays,
    )
  }
  const free =
    form.readingFree === null ? '' : s.sourceCacheFree(formatBytes(form.readingFree, language))
  const preloading = !form.preloadNextEpisodes
  return (
    <>
      <SettingsGroup title={s.sourceCacheGroup}>
        <SettingRow
          kind="choice"
          anchor="source-cache-preset"
          label={s.sourceCachePreset}
          help={s.sourceCachePresetHelp(free)}
          error={error('source-cache-preset')}
          value={form.sourceCachePreset}
          saved={saved.sourceCachePreset}
          options={sourceCachePresets.map((value) => ({ value, label: label(value) }))}
          onChange={choose}
        />
        <SettingRow
          kind="number"
          anchor="cache-size"
          label={s.cacheSize}
          help={s.cacheSizeHelp(range('cacheSizeGb'))}
          error={error('cache-size')}
          {...minMax('cacheSizeGb')}
          step={1}
          saved={saved.cacheSizeGb}
          {...number('cache-size', form.cacheSizeGb, (value) =>
            custom({ cacheSizeGb: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="number"
          anchor="source-cache-days"
          label={s.sourceCacheDays}
          help={s.sourceCacheDaysHelp(range('sourceCacheDays'))}
          error={error('source-cache-days')}
          {...minMax('sourceCacheDays')}
          step={1}
          saved={saved.sourceCacheDays}
          {...number('source-cache-days', form.sourceCacheDays, (value) =>
            custom({ sourceCacheDays: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="number"
          anchor="source-cache-credit"
          label={s.sourceCacheCreditDays}
          help={s.sourceCacheCreditDaysHelp(range('sourceCacheCreditDays'))}
          error={error('source-cache-credit')}
          {...minMax('sourceCacheCreditDays')}
          step={1}
          saved={saved.sourceCacheCreditDays}
          {...number('source-cache-credit', form.sourceCacheCreditDays, (value) =>
            custom({ sourceCacheCreditDays: Math.trunc(value) }),
          )}
        />
        <WorkFolderRow
          folder="readingFolder"
          form={form}
          saved={saved}
          update={update}
          error={error}
          limits={limits}
        />
      </SettingsGroup>
      <SettingsGroup title={s.preloadGroup}>
        <SettingRow
          kind="switch"
          anchor="preload-next-episodes"
          label={s.preloadNextEpisodes}
          help={s.preloadNextEpisodesHelp}
          checked={form.preloadNextEpisodes}
          saved={saved.preloadNextEpisodes}
          onChange={(preloadNextEpisodes) => update({ preloadNextEpisodes })}
        />
        <SettingRow
          kind="number"
          anchor="preload-episodes"
          label={s.preloadEpisodes}
          help={s.preloadEpisodesHelp(range('preloadEpisodes'))}
          error={error('preload-episodes')}
          {...minMax('preloadEpisodes')}
          step={1}
          saved={saved.preloadEpisodes}
          disabled={preloading}
          {...number('preload-episodes', form.preloadEpisodes, (value) =>
            update({ preloadEpisodes: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="number"
          anchor="preload-mbps"
          label={s.preloadMbps}
          help={s.preloadMbpsHelp(range('preloadMbps'))}
          error={error('preload-mbps')}
          {...minMax('preloadMbps')}
          step={1}
          saved={saved.preloadMbps}
          disabled={preloading}
          {...number('preload-mbps', form.preloadMbps, (value) =>
            update({ preloadMbps: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="number"
          anchor="preload-reserve"
          label={s.preloadReserveGb}
          help={s.preloadReserveGbHelp(range('preloadReserveGb'))}
          error={error('preload-reserve')}
          {...minMax('preloadReserveGb')}
          step={1}
          saved={saved.preloadReserveGb}
          disabled={preloading}
          {...number('preload-reserve', form.preloadReserveGb, (value) =>
            update({ preloadReserveGb: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="switch"
          anchor="preload-whole-files"
          label={s.preloadWholeFiles}
          help={s.preloadWholeFilesHelp}
          checked={form.preloadWholeFiles}
          saved={saved.preloadWholeFiles}
          disabled={preloading}
          onChange={(preloadWholeFiles) => custom({ preloadWholeFiles })}
        />
      </SettingsGroup>
    </>
  )
}
