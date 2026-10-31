import { sourceCachePresets, type Settings, type SourceCachePreset } from '@/api'
import { formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import { NumberInput, Select } from '@/ui'
import { FieldRow, SettingsGroup, SwitchRow } from './parts'
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
  update,
  error,
  number,
  range,
  limits,
}: Pick<SectionFormApi, 'form' | 'update' | 'error' | 'number' | 'range' | 'limits'>) {
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
        <FieldRow
          anchor="source-cache-preset"
          label={s.sourceCachePreset}
          help={s.sourceCachePresetHelp(free)}
          error={error('source-cache-preset')}
        >
          <Select
            value={form.sourceCachePreset}
            options={sourceCachePresets.map((value) => ({ value, label: label(value) }))}
            onValue={choose}
            className="max-w-md"
          />
        </FieldRow>
        <FieldRow
          anchor="cache-size"
          label={s.cacheSize}
          help={s.cacheSizeHelp(range('cacheSizeGb'))}
          error={error('cache-size')}
        >
          <NumberInput
            {...limits('cacheSizeGb')}
            step={1}
            {...number('cache-size', form.cacheSizeGb, (value) =>
              custom({ cacheSizeGb: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <div className="grid gap-x-8 sm:grid-cols-2">
          <FieldRow
            anchor="source-cache-days"
            label={s.sourceCacheDays}
            help={s.sourceCacheDaysHelp(range('sourceCacheDays'))}
            error={error('source-cache-days')}
          >
            <NumberInput
              {...limits('sourceCacheDays')}
              step={1}
              {...number('source-cache-days', form.sourceCacheDays, (value) =>
                custom({ sourceCacheDays: Math.trunc(value) }),
              )}
            />
          </FieldRow>
          <FieldRow
            anchor="source-cache-credit"
            label={s.sourceCacheCreditDays}
            help={s.sourceCacheCreditDaysHelp(range('sourceCacheCreditDays'))}
            error={error('source-cache-credit')}
          >
            <NumberInput
              {...limits('sourceCacheCreditDays')}
              step={1}
              {...number('source-cache-credit', form.sourceCacheCreditDays, (value) =>
                custom({ sourceCacheCreditDays: Math.trunc(value) }),
              )}
            />
          </FieldRow>
        </div>
        <WorkFolderRow
          folder="readingFolder"
          form={form}
          update={update}
          error={error}
          limits={limits}
        />
      </SettingsGroup>
      <SettingsGroup title={s.preloadGroup}>
        <SwitchRow
          anchor="preload-next-episodes"
          label={s.preloadNextEpisodes}
          help={s.preloadNextEpisodesHelp}
          checked={form.preloadNextEpisodes}
          onChange={(preloadNextEpisodes) => update({ preloadNextEpisodes })}
        />
        <div className="grid gap-x-8 sm:grid-cols-3">
          <FieldRow
            anchor="preload-episodes"
            label={s.preloadEpisodes}
            help={s.preloadEpisodesHelp(range('preloadEpisodes'))}
            error={error('preload-episodes')}
          >
            <NumberInput
              {...limits('preloadEpisodes')}
              step={1}
              disabled={preloading}
              {...number('preload-episodes', form.preloadEpisodes, (value) =>
                update({ preloadEpisodes: Math.trunc(value) }),
              )}
            />
          </FieldRow>
          <FieldRow
            anchor="preload-mbps"
            label={s.preloadMbps}
            help={s.preloadMbpsHelp(range('preloadMbps'))}
            error={error('preload-mbps')}
          >
            <NumberInput
              {...limits('preloadMbps')}
              step={1}
              disabled={preloading}
              {...number('preload-mbps', form.preloadMbps, (value) =>
                update({ preloadMbps: Math.trunc(value) }),
              )}
            />
          </FieldRow>
          <FieldRow
            anchor="preload-reserve"
            label={s.preloadReserveGb}
            help={s.preloadReserveGbHelp(range('preloadReserveGb'))}
            error={error('preload-reserve')}
          >
            <NumberInput
              {...limits('preloadReserveGb')}
              step={1}
              disabled={preloading}
              {...number('preload-reserve', form.preloadReserveGb, (value) =>
                update({ preloadReserveGb: Math.trunc(value) }),
              )}
            />
          </FieldRow>
        </div>
        <SwitchRow
          anchor="preload-whole-files"
          label={s.preloadWholeFiles}
          help={s.preloadWholeFilesHelp}
          checked={form.preloadWholeFiles}
          disabled={preloading}
          onChange={(preloadWholeFiles) => custom({ preloadWholeFiles })}
        />
      </SettingsGroup>
    </>
  )
}
