import type { ReactNode } from 'react'
import type {
  DeinterlaceMethod,
  DownmixAlgorithm,
  EncoderPreset,
  HardwareAcceleration,
  HardwareDecodingCodec,
  Settings,
  ToneMappingAlgorithm,
} from '@/api'
import { useI18n } from '@/i18n'
import { Checkbox, Field, FieldError, NumberInput, Select, StatusPill } from '@/ui'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow } from './parts'
import type { SectionFormApi } from './SectionForm'

/**
 * The Conversion section: whether and how much the server converts, the graphics card and what
 * it was found to do, then video, HDR, interlaced video, audio and performance. Options the
 * hardware cannot do are disabled; the tone mapping peak and desaturation, which only the
 * processor uses, show only when no graphics card tone maps.
 */
export default function ConversionSection({
  form,
  update,
  error,
  number,
  range,
  limits,
}: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const c = s.conversion
  const hardware = form.conversionHardware
  const gpu = hardware.gpu
  const encodesHevc =
    hardware.encoders.includes('libx265') ||
    (gpu?.encoders.some((encoder) => encoder.startsWith('hevc_')) ?? false)
  const gpuToneMaps = gpu?.toneMapping ?? false
  const toneMapsAnywhere = hardware.toneMapping || gpuToneMaps
  const decodedOnGpu = (codec: string) =>
    form.hardwareDecodingCodecs.some((decoded) => decoded === codec)
  /** The values the server accepts for a setting. */
  const choices = <T,>(name: keyof Settings) => (form.bounds[name]?.choices ?? []) as T[]

  return (
    <>
      <SettingsGroup title={c.groups.general}>
        <SwitchRow
          anchor="transcoding"
          label={s.transcoding}
          help={s.transcodingHelp}
          checked={form.transcoding}
          onChange={(transcoding) => update({ transcoding })}
        />
        <FieldRow
          anchor="max-conversions"
          label={s.maxConversions}
          help={s.maxConversionsHelp(range('maxConversions'))}
          error={error('max-conversions')}
        >
          <NumberInput
            {...limits('maxConversions')}
            step={1}
            {...number('max-conversions', form.maxConversions, (value) =>
              update({ maxConversions: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="max-conversion-height"
          label={s.maxConversionHeight}
          help={s.maxConversionHeightHelp}
          error={error('max-conversion-height')}
        >
          <Select
            value={form.maxConversionHeight}
            options={choices<number>('maxConversionHeight').map((height) => ({
              value: height,
              label: height === 0 ? s.conversionHeightOriginal : s.conversionHeight(height),
            }))}
            onValue={(maxConversionHeight) => update({ maxConversionHeight })}
            className="max-w-xs"
          />
        </FieldRow>
      </SettingsGroup>

      <SettingsGroup title={c.groups.gpu}>
        <FieldRow
          anchor="hardware-acceleration"
          label={c.hardwareAcceleration}
          help={c.hardwareAccelerationHelp}
          error={error('hardware-acceleration')}
        >
          <Select
            value={form.hardwareAcceleration}
            options={choices<HardwareAcceleration>('hardwareAcceleration').map((choice) => ({
              value: choice,
              label: c.hardware[choice],
            }))}
            onValue={(hardwareAcceleration) => update({ hardwareAcceleration })}
          />
        </FieldRow>
        {(form.hardwareAcceleration === 'auto' || form.hardwareAcceleration === 'vaapi') && (
          <FieldRow
            anchor="vaapi-device"
            label={c.vaapiDevice}
            help={c.vaapiDeviceHelp}
            error={error('vaapi-device')}
          >
            <Select
              value={form.vaapiDevice}
              options={[
                { value: '', label: c.vaapiDeviceEach },
                ...form.renderNodes.map((node) => ({ value: node, label: node })),
                ...(form.vaapiDevice !== '' && !form.renderNodes.includes(form.vaapiDevice)
                  ? [{ value: form.vaapiDevice, label: c.vaapiDeviceMissing(form.vaapiDevice) }]
                  : []),
              ]}
              onValue={(vaapiDevice) => update({ vaapiDevice })}
              className="max-w-xs"
            />
          </FieldRow>
        )}
        <SettingRow anchor="detected-hardware">
          <h4 className="text-[15px] font-medium tracking-[-0.01em] text-ink">{c.detected}</h4>
          <dl className="mt-3 rounded-row border border-line-2 bg-s1 text-small">
            {gpu === null ? (
              <div className="px-4 py-3 text-ink-2">{c.noGpu}</div>
            ) : (
              <>
                <Detail term={c.gpu}>
                  <span className="text-ink">{c.gpuNames[gpu.method as 'cuda'] ?? gpu.method}</span>
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
              <YesNo value={hardware.toneMapping} />
            </Detail>
          </dl>
        </SettingRow>
        <SettingRow anchor="hardware-decoding">
          <fieldset aria-describedby="hardware-decoding-help">
            <legend className="text-[15px] font-medium tracking-[-0.01em] text-ink">
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
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup title={c.groups.video}>
        <FieldRow
          anchor="encoder-preset"
          label={c.encoderPreset}
          help={c.encoderPresetHelp}
          error={error('encoder-preset')}
        >
          <Select
            value={form.encoderPreset}
            options={choices<EncoderPreset>('encoderPreset').map((preset) => ({
              value: preset,
              label: c.presets[preset],
            }))}
            onValue={(encoderPreset) => update({ encoderPreset })}
            className="max-w-xs"
          />
        </FieldRow>
        <SettingRow anchor="video-quality">
          <div className="grid gap-5 sm:grid-cols-2">
            <Field
              label={c.h264Quality}
              error={error('h264-quality')}
              className="[&_label]:text-[15px]"
            >
              <NumberInput
                {...limits('h264Quality')}
                step={1}
                {...number('h264-quality', form.h264Quality, (value) =>
                  update({ h264Quality: Math.trunc(value) }),
                )}
              />
            </Field>
            <Field
              label={c.hevcQuality}
              error={error('hevc-quality')}
              className="[&_label]:text-[15px]"
            >
              <NumberInput
                {...limits('hevcQuality')}
                step={1}
                {...number('hevc-quality', form.hevcQuality, (value) =>
                  update({ hevcQuality: Math.trunc(value) }),
                )}
              />
            </Field>
          </div>
          <p className="mt-2 max-w-[60ch] text-small text-ink-3">
            {c.qualityHelp(range('h264Quality'))}
          </p>
          {gpu?.method === 'vaapi' && !gpu.qvbr && (
            <p className="mt-1 text-small text-warn">{c.qualityIgnored}</p>
          )}
          {error('video-quality') && (
            <div className="mt-2">
              <FieldError>{error('video-quality')}</FieldError>
            </div>
          )}
        </SettingRow>
        <SwitchRow
          anchor="allow-hevc-encoding"
          label={c.allowHevcEncoding}
          help={encodesHevc ? c.allowHevcEncodingHelp : c.noHevcEncoder}
          checked={form.allowHevcEncoding}
          disabled={!encodesHevc && !form.allowHevcEncoding}
          onChange={(allowHevcEncoding) => update({ allowHevcEncoding })}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.hdr}>
        <SwitchRow
          anchor="tone-mapping"
          label={c.toneMapping}
          help={toneMapsAnywhere ? c.toneMappingHelp : c.toneMappingUnavailable}
          checked={form.toneMapping}
          onChange={(toneMapping) => update({ toneMapping })}
        />
        <FieldRow
          anchor="tone-mapping-algorithm"
          label={c.toneMappingAlgorithm}
          help={c.toneMappingAlgorithmHelp}
          error={error('tone-mapping-algorithm')}
        >
          <Select
            value={form.toneMappingAlgorithm}
            disabled={!form.toneMapping || !toneMapsAnywhere}
            options={choices<ToneMappingAlgorithm>('toneMappingAlgorithm').map((algorithm) => ({
              value: algorithm,
              label: c.algorithms[algorithm],
              disabled:
                algorithm === 'bt2390' && !gpuToneMaps && form.toneMappingAlgorithm !== 'bt2390',
            }))}
            onValue={(toneMappingAlgorithm) => update({ toneMappingAlgorithm })}
            className="max-w-xs"
          />
        </FieldRow>
        {!gpuToneMaps && (
          <SettingRow anchor="tone-mapping-peak">
            <div className="grid gap-5 sm:grid-cols-2">
              <Field
                label={c.toneMappingPeak}
                error={error('tone-mapping-peak-value')}
                className="[&_label]:text-[15px]"
              >
                <NumberInput
                  {...limits('toneMappingPeak')}
                  step={1}
                  disabled={!form.toneMapping || !hardware.toneMapping}
                  {...number('tone-mapping-peak-value', form.toneMappingPeak, (value) =>
                    update({ toneMappingPeak: Math.trunc(value) }),
                  )}
                />
              </Field>
              <Field
                label={c.toneMappingDesat}
                error={error('tone-mapping-desat')}
                className="[&_label]:text-[15px]"
              >
                <NumberInput
                  {...limits('toneMappingDesat')}
                  step={0.1}
                  disabled={!form.toneMapping || !hardware.toneMapping}
                  {...number('tone-mapping-desat', form.toneMappingDesat, (value) =>
                    update({ toneMappingDesat: value }),
                  )}
                />
              </Field>
            </div>
            <p className="mt-2 max-w-[60ch] text-small text-ink-3">
              {hardware.toneMapping
                ? c.toneMappingPeakHelp(range('toneMappingPeak'), range('toneMappingDesat'))
                : c.processorCannotToneMap}
            </p>
            {error('tone-mapping-peak') && (
              <div className="mt-2">
                <FieldError>{error('tone-mapping-peak')}</FieldError>
              </div>
            )}
          </SettingRow>
        )}
      </SettingsGroup>

      <SettingsGroup title={c.groups.interlaced}>
        <FieldRow
          anchor="deinterlace-method"
          label={c.deinterlaceMethod}
          help={hardware.bwdif ? c.deinterlaceMethodHelp : c.noBwdif}
          error={error('deinterlace-method')}
        >
          <Select
            value={form.deinterlaceMethod}
            options={choices<DeinterlaceMethod>('deinterlaceMethod').map((method) => ({
              value: method,
              label: c.deinterlacers[method],
              disabled: method === 'bwdif' && !hardware.bwdif && form.deinterlaceMethod !== 'bwdif',
            }))}
            onValue={(deinterlaceMethod) => update({ deinterlaceMethod })}
            className="max-w-xs"
          />
        </FieldRow>
        <SwitchRow
          anchor="deinterlace-double-rate"
          label={c.deinterlaceDoubleRate}
          help={c.deinterlaceDoubleRateHelp}
          checked={form.deinterlaceDoubleRate}
          onChange={(deinterlaceDoubleRate) => update({ deinterlaceDoubleRate })}
        />
      </SettingsGroup>

      <SettingsGroup title={c.groups.audio}>
        <FieldRow
          anchor="downmix-algorithm"
          label={c.downmixAlgorithm}
          help={c.downmixAlgorithmHelp}
          error={error('downmix-algorithm')}
        >
          <Select
            value={form.downmixAlgorithm}
            options={choices<DownmixAlgorithm>('downmixAlgorithm').map((algorithm) => ({
              value: algorithm,
              label: c.downmixes[algorithm],
            }))}
            onValue={(downmixAlgorithm) => update({ downmixAlgorithm })}
            className="max-w-xs"
          />
        </FieldRow>
        <FieldRow
          anchor="downmix-boost"
          label={c.downmixBoost}
          help={c.downmixBoostHelp(range('downmixBoost'))}
          error={error('downmix-boost')}
        >
          <NumberInput
            {...limits('downmixBoost')}
            step={0.1}
            {...number('downmix-boost', form.downmixBoost, (value) =>
              update({ downmixBoost: value }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="max-audio-channels"
          label={c.maxAudioChannels}
          help={c.maxAudioChannelsHelp}
          error={error('max-audio-channels')}
        >
          <Select
            value={form.maxAudioChannels}
            options={choices<number>('maxAudioChannels').map((channels) => ({
              value: channels,
              label: c.audioChannels(channels),
            }))}
            onValue={(maxAudioChannels) => update({ maxAudioChannels })}
            className="max-w-xs"
          />
        </FieldRow>
        <FieldRow
          anchor="audio-bitrate-per-channel"
          label={c.audioBitratePerChannel}
          help={c.audioBitratePerChannelHelp(range('audioBitratePerChannel'))}
          error={error('audio-bitrate-per-channel')}
        >
          <NumberInput
            {...limits('audioBitratePerChannel')}
            step={1}
            {...number('audio-bitrate-per-channel', form.audioBitratePerChannel, (value) =>
              update({ audioBitratePerChannel: Math.trunc(value) }),
            )}
          />
        </FieldRow>
      </SettingsGroup>

      <SettingsGroup title={c.groups.performance}>
        <FieldRow
          anchor="encoding-threads"
          label={c.encodingThreads}
          help={c.encodingThreadsHelp(range('encodingThreads'))}
          error={error('encoding-threads')}
        >
          <NumberInput
            {...limits('encodingThreads')}
            step={1}
            {...number('encoding-threads', form.encodingThreads, (value) =>
              update({ encodingThreads: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="ahead-seconds"
          label={c.aheadSeconds}
          help={c.aheadSecondsHelp(range('aheadSeconds'))}
          error={error('ahead-seconds')}
        >
          <NumberInput
            {...limits('aheadSeconds')}
            step={1}
            {...number('ahead-seconds', form.aheadSeconds, (value) =>
              update({ aheadSeconds: Math.trunc(value) }),
            )}
          />
        </FieldRow>
      </SettingsGroup>
    </>
  )
}

/** One line of what was detected, with a hint under its term when it has one. */
function Detail({ term, hint, children }: { term: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 px-4 py-3 not-first:border-t not-first:border-line sm:flex-row sm:items-center sm:justify-between sm:gap-6">
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
