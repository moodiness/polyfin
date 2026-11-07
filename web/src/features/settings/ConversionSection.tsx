import type { ReactNode } from 'react'
import type {
  AdaptiveQuality,
  DeinterlaceMethod,
  DownmixAlgorithm,
  DynamicRangeCompression,
  EncoderPreset,
  HardwareAcceleration,
  HardwareDecodingCodec,
  OlderDriverFFmpeg,
  Settings,
  SubtitleOcr,
  ToneMappingAlgorithm,
} from '@/api'
import { type Messages, useI18n } from '@/i18n'
import { Checkbox, FieldError, SettingRow, StatusPill } from '@/ui'
import { PlainRow, SettingsGroup } from './parts'
import type { SectionFormApi } from './SectionForm'
import WorkFolderRow from './WorkFolders'
import { hdrPathText } from './hdrPath'

/**
 * The Conversion section: whether and how much the server converts, picture subtitles read into
 * text, the graphics card and what it was found to do, then video, HDR, interlaced video, audio
 * and performance. Options the hardware cannot do are disabled; the tone mapping peak and
 * desaturation, which only the processor uses, show unless an NVIDIA card tone maps every HDR
 * video.
 */
export default function ConversionSection(api: SectionFormApi) {
  const { form, saved, update, error, number, range, minMax } = api
  const { language, t } = useI18n()
  const s = t.settings
  const c = s.conversion
  const hardware = form.conversionHardware
  const gpu = hardware.gpu
  const encodesHevc =
    hardware.encoders.includes('libx265') ||
    (gpu?.encoders.some((encoder) => encoder.startsWith('hevc_')) ?? false)
  const gpuCanToneMap = gpu?.toneMapping ?? false
  const gpuToneMaps = gpuCanToneMap && form.gpuToneMapping
  /**
   * NVIDIA cards tone map with libplacebo, which takes BT.2390 and leaves nothing to the
   * processor, unless the card is too slow for HDR and the processor takes it.
   */
  const placeboToneMaps = gpuToneMaps && gpu?.method === 'cuda' && gpu.hdr?.path !== 'processor'
  /** Where HDR titles go, as the timings at startup chose it and the form's setting says. */
  const hdrPath =
    gpu?.hdr && form.toneMapping
      ? hdrPathText(gpu.hdr, c.hdrPath, language, form.gpuToneMapping)
      : null
  const toneMapsAnywhere = hardware.toneMapping || gpuToneMaps
  /** The height the processor tone maps HDR up to: the setting, else what was measured. */
  const toneMappedHeight = Math.min(
    form.processorToneMappingHeight || hardware.toneMappingHeight || 720,
    gpu === null ? 1080 : 2160,
  )
  const decodedOnGpu = (codec: string) =>
    form.hardwareDecodingCodecs.some((decoded) => decoded === codec)
  /** The values the server accepts for a setting. */
  const choices = <T,>(name: keyof Settings) => (form.bounds[name]?.choices ?? []) as T[]
  /** QSV and AMF aim for the bitrate, as VAAPI without QVBR does. */
  const qualityIgnored =
    (gpu?.method === 'vaapi' && !gpu.qvbr) || gpu?.method === 'qsv' || gpu?.method === 'amf'
  /** The peak and desaturation only matter while the processor tone maps. */
  const processorToneMapOff = !form.toneMapping || !hardware.toneMapping
  const toneMappingPeakHelp = hardware.toneMapping
    ? c.toneMappingPeakHelp(range('toneMappingPeak'), range('toneMappingDesat'))
    : c.processorCannotToneMap

  return (
    <>
      <SettingsGroup title={c.groups.general}>
        <SettingRow
          kind="switch"
          anchor="transcoding"
          label={s.transcoding}
          help={s.transcodingHelp}
          checked={form.transcoding}
          saved={saved.transcoding}
          onChange={(transcoding) => update({ transcoding })}
        />
        <SettingRow
          kind="number"
          anchor="max-conversions"
          label={s.maxConversions}
          help={s.maxConversionsHelp(range('maxConversions'))}
          error={error('max-conversions')}
          {...minMax('maxConversions')}
          step={1}
          saved={saved.maxConversions}
          {...number('max-conversions', form.maxConversions, (value) =>
            update({ maxConversions: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="choice"
          anchor="max-conversion-height"
          label={s.maxConversionHeight}
          help={s.maxConversionHeightHelp}
          error={error('max-conversion-height')}
          value={form.maxConversionHeight}
          saved={saved.maxConversionHeight}
          options={choices<number>('maxConversionHeight').map((height) => ({
            value: height,
            label: height === 0 ? s.conversionHeightOriginal : s.conversionHeight(height),
          }))}
          onChange={(maxConversionHeight) => update({ maxConversionHeight })}
        />
        <SettingRow
          kind="switch"
          anchor="converted-downloads"
          label={s.convertedDownloads}
          help={s.convertedDownloadsHelp}
          checked={form.convertedDownloads}
          saved={saved.convertedDownloads}
          onChange={(convertedDownloads) => update({ convertedDownloads })}
        />
        <WorkFolderRow folder="conversionFolder" {...api} />
      </SettingsGroup>

      <SettingsGroup title={c.groups.subtitles}>
        <SettingRow
          kind="choice"
          anchor="subtitle-ocr"
          label={c.subtitleOcr}
          help={c.subtitleOcrHelp}
          error={error('subtitle-ocr')}
          value={form.subtitleOcr}
          saved={saved.subtitleOcr}
          options={choices<SubtitleOcr>('subtitleOcr').map((mode) => ({
            value: mode,
            label: c.subtitleOcrChoices[mode],
          }))}
          onChange={(subtitleOcr) => update({ subtitleOcr })}
        />
        <SettingRow
          kind="number"
          anchor="subtitle-ocr-tracks"
          label={c.subtitleOcrTracks}
          help={c.subtitleOcrTracksHelp(range('subtitleOcrTracks'))}
          error={error('subtitle-ocr-tracks')}
          disabled={form.subtitleOcr === 'off'}
          {...minMax('subtitleOcrTracks')}
          step={1}
          saved={saved.subtitleOcrTracks}
          {...number('subtitle-ocr-tracks', form.subtitleOcrTracks, (value) =>
            update({ subtitleOcrTracks: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="switch"
          anchor="subtitle-ocr-download"
          label={c.subtitleOcrDownload}
          help={c.subtitleOcrDownloadHelp}
          checked={form.subtitleOcrDownload}
          saved={saved.subtitleOcrDownload}
          disabled={form.subtitleOcr === 'off'}
          onChange={(subtitleOcrDownload) => update({ subtitleOcrDownload })}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.gpu}>
        <SettingRow
          kind="choice"
          anchor="hardware-acceleration"
          label={c.hardwareAcceleration}
          help={c.hardwareAccelerationHelp}
          error={error('hardware-acceleration')}
          value={form.hardwareAcceleration}
          saved={saved.hardwareAcceleration}
          options={choices<HardwareAcceleration>('hardwareAcceleration').map((choice) => ({
            value: choice,
            label: c.hardware[choice],
          }))}
          onChange={(hardwareAcceleration) => update({ hardwareAcceleration })}
        />
        {choices<HardwareAcceleration>('hardwareAcceleration').includes('vaapi') &&
          (form.hardwareAcceleration === 'auto' || form.hardwareAcceleration === 'vaapi') && (
            <SettingRow
              kind="choice"
              anchor="vaapi-device"
              label={c.vaapiDevice}
              help={c.vaapiDeviceHelp}
              error={error('vaapi-device')}
              value={form.vaapiDevice}
              saved={saved.vaapiDevice}
              options={[
                { value: '', label: c.vaapiDeviceEach },
                ...form.renderNodes.map((node) => ({ value: node, label: node })),
                ...(form.vaapiDevice !== '' && !form.renderNodes.includes(form.vaapiDevice)
                  ? [{ value: form.vaapiDevice, label: c.vaapiDeviceMissing(form.vaapiDevice) }]
                  : []),
              ]}
              onChange={(vaapiDevice) => update({ vaapiDevice })}
            />
          )}
        <PlainRow anchor="detected-hardware">
          <h4 className="text-h3 text-ink max-md:text-[1.125rem]">{c.detected}</h4>
          <dl className="mt-3.5 grid gap-1.5 text-small">
            {gpu === null ? (
              // A term and its value, as a list of details holds: the term left to screen
              // readers, the sentence says it.
              <div className="rounded-row bg-s2 px-4 py-3 text-ink-2">
                <dt className="sr-only">{c.gpu}</dt>
                <dd>{c.noGpu}</dd>
              </div>
            ) : (
              <>
                <Detail term={c.gpu}>
                  <span className="text-ink">{c.gpuNames[gpu.method] ?? gpu.method}</span>
                  {gpu.device !== '' && (
                    <code className="font-mono text-micro text-ink-3">{gpu.device}</code>
                  )}
                </Detail>
                <Detail term={c.encoders}>
                  <Codes names={gpu.encoders} />
                </Detail>
                <Detail
                  term={c.gpuToneMapping}
                  hint={
                    !gpu.toneMapping && hardware.toneMapping
                      ? c.processorToneMappingHint
                      : undefined
                  }
                >
                  <YesNo value={gpu.toneMapping} />
                </Detail>
                {hdrPath && (
                  <Detail term={c.hdrPath.term} hint={hdrPath.why}>
                    <span className="text-ink">{hdrPath.where}</span>
                  </Detail>
                )}
                {gpu.method === 'vaapi' && (
                  <Detail term={c.qualityFactor}>
                    <YesNo value={gpu.qvbr} />
                  </Detail>
                )}
              </>
            )}
            <Detail term={c.processor}>
              <Codes names={hardware.encoders} />
            </Detail>
            <Detail term={c.processorToneMapping}>
              {hardware.toneMapping && (
                <span className="text-ink-2">{c.upToHeight(toneMappedHeight)}</span>
              )}
              <YesNo value={hardware.toneMapping} />
            </Detail>
            <Detail term={c.ffmpeg} hint={olderDriverNote(c, hardware.olderDriverFFmpeg)}>
              <span className="whitespace-nowrap text-ink">
                {gpu?.olderDrivers ? c.ffmpegOlderDrivers : c.ffmpegShipped}
              </span>
            </Detail>
          </dl>
        </PlainRow>
        <PlainRow anchor="hardware-decoding">
          <fieldset aria-describedby="hardware-decoding-help">
            <legend className="text-h3 text-ink max-md:text-[1.125rem]">
              {c.hardwareDecoding}
            </legend>
            <p id="hardware-decoding-help" className="mt-1 max-w-[60ch] text-small text-ink-3">
              {gpu === null ? c.hardwareDecodingNoGpu : c.hardwareDecodingHelp}
            </p>
            <div className="mt-3.5 grid grid-cols-2 gap-3 sm:grid-cols-4">
              {choices<HardwareDecodingCodec>('hardwareDecodingCodecs').map((codec, _, all) => (
                <Checkbox
                  key={codec}
                  label={c.codecs[codec]}
                  checked={decodedOnGpu(codec)}
                  disabled={gpu === null || (codec === 'hevc_10bit' && !decodedOnGpu('hevc'))}
                  onChange={(checked) =>
                    update({
                      hardwareDecodingCodecs: all.filter((other) =>
                        other === codec ? checked : decodedOnGpu(other),
                      ),
                    })
                  }
                />
              ))}
            </div>
            {error('hardware-decoding') && (
              <div className="mt-2">
                <FieldError>{error('hardware-decoding')}</FieldError>
              </div>
            )}
          </fieldset>
        </PlainRow>
      </SettingsGroup>

      <SettingsGroup title={c.groups.video}>
        <SettingRow
          kind="choice"
          anchor="encoder-preset"
          label={c.encoderPreset}
          help={c.encoderPresetHelp}
          error={error('encoder-preset')}
          value={form.encoderPreset}
          saved={saved.encoderPreset}
          options={choices<EncoderPreset>('encoderPreset').map((preset) => ({
            value: preset,
            label: c.presets[preset],
          }))}
          onChange={(encoderPreset) => update({ encoderPreset })}
        />
        <SettingRow
          kind="number"
          anchor="video-quality"
          label={c.h264Quality}
          help={c.qualityHelp(range('h264Quality'))}
          error={error('h264-quality') ?? error('video-quality')}
          {...minMax('h264Quality')}
          step={1}
          saved={saved.h264Quality}
          {...number('h264-quality', form.h264Quality, (value) =>
            update({ h264Quality: Math.trunc(value) }),
          )}
        >
          {qualityIgnored && <p className="text-small text-warn">{c.qualityIgnored}</p>}
        </SettingRow>
        <SettingRow
          kind="number"
          anchor="hevc-quality"
          label={c.hevcQuality}
          help={c.qualityHelp(range('hevcQuality'))}
          error={error('hevc-quality')}
          {...minMax('hevcQuality')}
          step={1}
          saved={saved.hevcQuality}
          {...number('hevc-quality', form.hevcQuality, (value) =>
            update({ hevcQuality: Math.trunc(value) }),
          )}
        >
          {qualityIgnored && <p className="text-small text-warn">{c.qualityIgnored}</p>}
        </SettingRow>
        <SettingRow
          kind="switch"
          anchor="allow-hevc-encoding"
          label={c.allowHevcEncoding}
          help={encodesHevc ? c.allowHevcEncodingHelp : c.noHevcEncoder}
          checked={form.allowHevcEncoding}
          saved={saved.allowHevcEncoding}
          disabled={!encodesHevc && !form.allowHevcEncoding}
          onChange={(allowHevcEncoding) => update({ allowHevcEncoding })}
        />
        <SettingRow
          kind="choice"
          anchor="adaptive-quality"
          label={c.adaptiveQuality}
          help={c.adaptiveQualityHelp}
          error={error('adaptive-quality')}
          value={form.adaptiveQuality}
          saved={saved.adaptiveQuality}
          options={choices<AdaptiveQuality>('adaptiveQuality').map((choice) => ({
            value: choice,
            label: c.adaptiveQualities[choice],
          }))}
          onChange={(adaptiveQuality) => update({ adaptiveQuality })}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.hdr}>
        <SettingRow
          kind="switch"
          anchor="tone-mapping"
          label={c.toneMapping}
          help={toneMapsAnywhere ? c.toneMappingHelp : c.toneMappingUnavailable}
          checked={form.toneMapping}
          saved={saved.toneMapping}
          onChange={(toneMapping) => update({ toneMapping })}
        />
        <SettingRow
          kind="choice"
          anchor="tone-mapping-algorithm"
          label={c.toneMappingAlgorithm}
          help={c.toneMappingAlgorithmHelp}
          error={error('tone-mapping-algorithm')}
          disabled={!form.toneMapping || !toneMapsAnywhere}
          value={form.toneMappingAlgorithm}
          saved={saved.toneMappingAlgorithm}
          options={choices<ToneMappingAlgorithm>('toneMappingAlgorithm').map((algorithm) => ({
            value: algorithm,
            label: c.algorithms[algorithm],
            disabled:
              algorithm === 'bt2390' && !placeboToneMaps && form.toneMappingAlgorithm !== 'bt2390',
          }))}
          onChange={(toneMappingAlgorithm) => update({ toneMappingAlgorithm })}
        />
        <SettingRow
          kind="switch"
          anchor="gpu-tone-mapping"
          label={c.gpuToneMappingSetting}
          help={gpuCanToneMap ? c.gpuToneMappingSettingHelp : c.gpuToneMappingUnavailable}
          checked={form.gpuToneMapping}
          saved={saved.gpuToneMapping}
          disabled={!form.toneMapping || !gpuCanToneMap}
          onChange={(gpuToneMapping) => update({ gpuToneMapping })}
        />
        <SettingRow
          kind="choice"
          anchor="processor-tone-mapping-height"
          label={c.processorToneMappingHeight}
          help={hardware.toneMapping ? c.processorToneMappingHeightHelp : c.processorCannotToneMap}
          error={error('processor-tone-mapping-height')}
          disabled={processorToneMapOff}
          value={form.processorToneMappingHeight}
          saved={saved.processorToneMappingHeight}
          options={choices<number>('processorToneMappingHeight').map((height) => ({
            value: height,
            label:
              height === 0
                ? c.toneMappingHeightAuto
                : height === 2160
                  ? c.height4k
                  : s.conversionHeight(height),
          }))}
          onChange={(processorToneMappingHeight) => update({ processorToneMappingHeight })}
        >
          {form.processorToneMappingHeight === 0 && hardware.toneMappingHeight > 0 && (
            <p className="text-small text-ink-3">{c.measuredHeight(hardware.toneMappingHeight)}</p>
          )}
        </SettingRow>
        {!placeboToneMaps && (
          <>
            <SettingRow
              kind="number"
              anchor="tone-mapping-peak"
              label={c.toneMappingPeak}
              help={toneMappingPeakHelp}
              error={error('tone-mapping-peak-value') ?? error('tone-mapping-peak')}
              disabled={processorToneMapOff}
              {...minMax('toneMappingPeak')}
              step={1}
              saved={saved.toneMappingPeak}
              {...number('tone-mapping-peak-value', form.toneMappingPeak, (value) =>
                update({ toneMappingPeak: Math.trunc(value) }),
              )}
            />
            <SettingRow
              kind="number"
              anchor="tone-mapping-desat"
              label={c.toneMappingDesat}
              help={toneMappingPeakHelp}
              error={error('tone-mapping-desat')}
              disabled={processorToneMapOff}
              {...minMax('toneMappingDesat')}
              step={0.1}
              saved={saved.toneMappingDesat}
              {...number('tone-mapping-desat', form.toneMappingDesat, (value) =>
                update({ toneMappingDesat: value }),
              )}
            />
          </>
        )}
      </SettingsGroup>

      <SettingsGroup title={c.groups.interlaced}>
        <SettingRow
          kind="choice"
          anchor="deinterlace-method"
          label={c.deinterlaceMethod}
          help={hardware.bwdif ? c.deinterlaceMethodHelp : c.noBwdif}
          error={error('deinterlace-method')}
          value={form.deinterlaceMethod}
          saved={saved.deinterlaceMethod}
          options={choices<DeinterlaceMethod>('deinterlaceMethod').map((method) => ({
            value: method,
            label: c.deinterlacers[method],
            disabled: method === 'bwdif' && !hardware.bwdif && form.deinterlaceMethod !== 'bwdif',
          }))}
          onChange={(deinterlaceMethod) => update({ deinterlaceMethod })}
        />
        <SettingRow
          kind="switch"
          anchor="deinterlace-double-rate"
          label={c.deinterlaceDoubleRate}
          help={c.deinterlaceDoubleRateHelp}
          checked={form.deinterlaceDoubleRate}
          saved={saved.deinterlaceDoubleRate}
          onChange={(deinterlaceDoubleRate) => update({ deinterlaceDoubleRate })}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.audio}>
        <SettingRow
          kind="choice"
          anchor="downmix-algorithm"
          label={c.downmixAlgorithm}
          help={c.downmixAlgorithmHelp}
          error={error('downmix-algorithm')}
          value={form.downmixAlgorithm}
          saved={saved.downmixAlgorithm}
          options={choices<DownmixAlgorithm>('downmixAlgorithm').map((algorithm) => ({
            value: algorithm,
            label: c.downmixes[algorithm],
          }))}
          onChange={(downmixAlgorithm) => update({ downmixAlgorithm })}
        />
        <SettingRow
          kind="number"
          anchor="downmix-boost"
          label={c.downmixBoost}
          help={c.downmixBoostHelp(range('downmixBoost'))}
          error={error('downmix-boost')}
          {...minMax('downmixBoost')}
          step={0.1}
          saved={saved.downmixBoost}
          {...number('downmix-boost', form.downmixBoost, (value) =>
            update({ downmixBoost: value }),
          )}
        />
        <SettingRow
          kind="switch"
          anchor="normalize-converted-audio"
          label={c.normalizeConvertedAudio}
          help={c.normalizeConvertedAudioHelp}
          checked={form.normalizeConvertedAudio}
          saved={saved.normalizeConvertedAudio}
          onChange={(normalizeConvertedAudio) => update({ normalizeConvertedAudio })}
        />
        <SettingRow
          kind="choice"
          anchor="dynamic-range-compression"
          label={c.dynamicRangeCompression}
          help={c.dynamicRangeCompressionHelp}
          error={error('dynamic-range-compression')}
          value={form.dynamicRangeCompression}
          saved={saved.dynamicRangeCompression}
          options={choices<DynamicRangeCompression>('dynamicRangeCompression').map((choice) => ({
            value: choice,
            label: c.dynamicRangeCompressions[choice],
          }))}
          onChange={(dynamicRangeCompression) => update({ dynamicRangeCompression })}
        />
        <SettingRow
          kind="choice"
          anchor="max-audio-channels"
          label={c.maxAudioChannels}
          help={c.maxAudioChannelsHelp}
          error={error('max-audio-channels')}
          value={form.maxAudioChannels}
          saved={saved.maxAudioChannels}
          options={choices<number>('maxAudioChannels').map((channels) => ({
            value: channels,
            label: c.audioChannels(channels),
          }))}
          onChange={(maxAudioChannels) => update({ maxAudioChannels })}
        />
        <SettingRow
          kind="number"
          anchor="audio-bitrate-per-channel"
          label={c.audioBitratePerChannel}
          help={c.audioBitratePerChannelHelp(range('audioBitratePerChannel'))}
          error={error('audio-bitrate-per-channel')}
          {...minMax('audioBitratePerChannel')}
          step={1}
          saved={saved.audioBitratePerChannel}
          {...number('audio-bitrate-per-channel', form.audioBitratePerChannel, (value) =>
            update({ audioBitratePerChannel: Math.trunc(value) }),
          )}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.performance}>
        <SettingRow
          kind="number"
          anchor="encoding-threads"
          label={c.encodingThreads}
          help={c.encodingThreadsHelp(range('encodingThreads'))}
          error={error('encoding-threads')}
          {...minMax('encodingThreads')}
          step={1}
          saved={saved.encodingThreads}
          {...number('encoding-threads', form.encodingThreads, (value) =>
            update({ encodingThreads: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="number"
          anchor="ahead-seconds"
          label={c.aheadSeconds}
          help={c.aheadSecondsHelp(range('aheadSeconds'))}
          error={error('ahead-seconds')}
          {...minMax('aheadSeconds')}
          step={1}
          saved={saved.aheadSeconds}
          {...number('ahead-seconds', form.aheadSeconds, (value) =>
            update({ aheadSeconds: Math.trunc(value) }),
          )}
        />
        <SettingRow
          kind="switch"
          anchor="keep-converted-video"
          label={c.keepConvertedVideo}
          help={c.keepConvertedVideoHelp}
          checked={form.keepConvertedVideo}
          saved={saved.keepConvertedVideo}
          onChange={(keepConvertedVideo) => update({ keepConvertedVideo })}
        />
        <SettingRow
          kind="switch"
          anchor="fast-start"
          label={c.fastStart}
          help={c.fastStartHelp}
          checked={form.fastStart}
          saved={saved.fastStart}
          onChange={(fastStart) => update({ fastStart })}
        />
      </SettingsGroup>
    </>
  )
}

/** What the FFmpeg for older NVIDIA drivers is doing, when NVENC waits for it or failed with it. */
function olderDriverNote(
  c: Messages['settings']['conversion'],
  older: OlderDriverFFmpeg | null,
): string | undefined {
  switch (older?.state) {
    case 'downloading':
      return c.ffmpegOlderDownloading
    case 'failed':
      return c.ffmpegOlderFailed(older.error ?? '')
    case 'unusable':
      return c.ffmpegOlderUnusable(older.error ?? '')
  }
  return undefined
}

/** One line of what was detected, with a hint under its term when it has one. */
function Detail({ term, hint, children }: { term: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 rounded-row bg-s2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <dt className="text-ink-2">
        {term}
        {hint && <span className="mt-0.5 block text-micro text-ink-3">{hint}</span>}
      </dt>
      <dd className="flex flex-wrap items-center gap-1.5 sm:justify-end">{children}</dd>
    </div>
  )
}

function YesNo({ value }: { value: boolean }) {
  const { t } = useI18n()
  return value ? (
    <StatusPill tone="ok">{t.settings.conversion.yes}</StatusPill>
  ) : (
    <StatusPill tone="muted">{t.settings.conversion.no}</StatusPill>
  )
}

function Codes({ names }: { names: string[] }) {
  const { t } = useI18n()
  if (names.length === 0) {
    return <span className="text-ink-3">{t.settings.conversion.none}</span>
  }
  return names.map((name) => (
    <code key={name} className="rounded-check bg-s3 px-1.5 py-0.5 font-mono text-micro text-ink-2">
      {name}
    </code>
  ))
}
