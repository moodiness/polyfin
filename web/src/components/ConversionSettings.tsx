import { type ReactNode } from 'react'
import {
  aheadSegmentsRange,
  audioBitratePerChannelRange,
  audioChannelLimits,
  conversionHeights,
  deinterlaceMethods,
  downmixAlgorithms,
  downmixBoostRange,
  encoderPresets,
  encodingThreadsRange,
  hardwareAccelerations,
  hardwareDecodingCodecs,
  maxConversionsRange,
  toneMappingAlgorithms,
  toneMappingDesatRange,
  toneMappingPeakRange,
  videoQualityRange,
  type Settings,
} from '@/api'
import {
  SelectField,
  Setting,
  SettingsGroup,
  wholeNumber,
  wholeNumberField,
} from '@/components/settings'
import { Badge, Checkbox, TextField } from '@/components/ui'
import { useI18n } from '@/i18n'

/**
 * The Conversion section of the settings: whether and how much the server converts, the graphics
 * card and what it was found to do, then video, HDR, interlaced video, audio and performance.
 * Options the hardware cannot do are disabled.
 */
export default function ConversionSettings({
  form,
  update,
}: {
  form: Settings
  update: (patch: Partial<Settings>) => void
}) {
  const { t } = useI18n()
  const s = t.settings
  const c = s.conversion
  const hardware = form.conversionHardware
  const gpu = hardware.gpu
  const encodesHevc =
    hardware.encoders.includes('libx265') ||
    (gpu?.encoders.some((encoder) => encoder.startsWith('hevc_')) ?? false)
  const toneMapsAnywhere = hardware.toneMapping || (gpu?.toneMapping ?? false)
  const decodedOnGpu = (codec: string) =>
    form.hardwareDecodingCodecs.some((decoded) => decoded === codec)

  return (
    <>
      <SettingsGroup title={c.groups.general}>
        <Setting text={[s.transcoding, s.transcodingHelp]}>
          <Checkbox
            label={s.transcoding}
            help={s.transcodingHelp}
            checked={form.transcoding}
            onChange={(transcoding) => update({ transcoding })}
          />
        </Setting>
        <Setting text={[s.maxConversions, s.maxConversionsHelp]}>
          <TextField
            label={s.maxConversions}
            hint={s.maxConversionsHelp}
            type="number"
            inputMode="numeric"
            min={maxConversionsRange.min}
            max={maxConversionsRange.max}
            step={1}
            value={form.maxConversions}
            onValue={(value) => update({ maxConversions: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
        <Setting text={[s.maxConversionHeight, s.maxConversionHeightHelp]}>
          <SelectField
            label={s.maxConversionHeight}
            hint={s.maxConversionHeightHelp}
            value={form.maxConversionHeight}
            options={conversionHeights.map((height) => ({
              value: height,
              label: height === 0 ? s.conversionHeightOriginal : s.conversionHeight(height),
            }))}
            onValue={(maxConversionHeight) => update({ maxConversionHeight })}
          />
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.gpu}>
        <Setting text={[c.hardwareAcceleration, c.hardwareAccelerationHelp, 'gpu nvidia vaapi']}>
          <SelectField
            label={c.hardwareAcceleration}
            hint={c.hardwareAccelerationHelp}
            value={form.hardwareAcceleration}
            options={hardwareAccelerations.map((choice) => ({
              value: choice,
              label:
                choice === ''
                  ? c.hardwareDefault(c.hardware[hardware.default as 'auto'] ?? hardware.default)
                  : c.hardware[choice],
            }))}
            onValue={(hardwareAcceleration) => update({ hardwareAcceleration })}
          />
        </Setting>
        <Setting text={[c.detected, c.noGpu, c.gpuToneMapping, c.processor]}>
          <div role="group" aria-label={c.detected} className="rounded-xl border border-line">
            <p className="border-b border-line px-4 py-2.5 text-sm font-medium text-zinc-200">
              {c.detected}
            </p>
            <dl className="divide-y divide-line text-sm">
              {gpu === null ? (
                <div className="px-4 py-2.5 text-muted">{c.noGpu}</div>
              ) : (
                <>
                  <Detail term={c.gpu}>
                    {c.gpuNames[gpu.method as 'cuda'] ?? gpu.method}
                    {gpu.device !== '' && (
                      <code className="ml-2 font-mono text-xs text-muted">{gpu.device}</code>
                    )}
                  </Detail>
                  <Detail term={c.encoders}>
                    <Codes names={gpu.encoders} />
                  </Detail>
                  <Detail term={c.gpuToneMapping}>
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
          </div>
        </Setting>
        <Setting text={[c.hardwareDecoding, c.hardwareDecodingHelp, ...Object.values(c.codecs)]}>
          <fieldset>
            <legend className="text-sm font-medium text-zinc-200">{c.hardwareDecoding}</legend>
            <p className="mt-1 text-xs text-muted">
              {gpu === null ? c.hardwareDecodingNoGpu : c.hardwareDecodingHelp}
            </p>
            <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
              {hardwareDecodingCodecs.map((codec) => (
                <Checkbox
                  key={codec}
                  label={c.codecs[codec]}
                  checked={decodedOnGpu(codec)}
                  disabled={gpu === null || (codec === 'hevc_10bit' && !decodedOnGpu('hevc'))}
                  onChange={(checked) =>
                    update({
                      hardwareDecodingCodecs: hardwareDecodingCodecs.filter((other) =>
                        other === codec ? checked : decodedOnGpu(other),
                      ),
                    })
                  }
                />
              ))}
            </div>
          </fieldset>
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.video}>
        <Setting text={[c.encoderPreset, c.encoderPresetHelp]}>
          <SelectField
            label={c.encoderPreset}
            hint={c.encoderPresetHelp}
            value={form.encoderPreset}
            options={encoderPresets.map((preset) => ({ value: preset, label: c.presets[preset] }))}
            onValue={(encoderPreset) => update({ encoderPreset })}
          />
        </Setting>
        <Setting text={[c.h264Quality, c.hevcQuality, c.qualityHelp, 'crf']}>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField
              label={c.h264Quality}
              type="number"
              inputMode="numeric"
              min={0}
              max={videoQualityRange.max}
              step={1}
              value={wholeNumberField(form.h264Quality)}
              onValue={(value) => update({ h264Quality: wholeNumber(value) })}
              required
            />
            <TextField
              label={c.hevcQuality}
              type="number"
              inputMode="numeric"
              min={0}
              max={videoQualityRange.max}
              step={1}
              value={wholeNumberField(form.hevcQuality)}
              onValue={(value) => update({ hevcQuality: wholeNumber(value) })}
              required
            />
          </div>
          <p className="mt-1 text-xs text-muted">{c.qualityHelp}</p>
          {gpu?.method === 'vaapi' && !gpu.qvbr && (
            <p className="mt-1 text-xs text-amber-200">{c.qualityIgnored}</p>
          )}
        </Setting>
        <Setting text={[c.allowHevcEncoding, c.allowHevcEncodingHelp, 'h265']}>
          <Checkbox
            label={c.allowHevcEncoding}
            help={encodesHevc ? c.allowHevcEncodingHelp : c.noHevcEncoder}
            checked={form.allowHevcEncoding}
            disabled={!encodesHevc && !form.allowHevcEncoding}
            onChange={(allowHevcEncoding) => update({ allowHevcEncoding })}
          />
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.hdr}>
        <Setting text={[c.toneMapping, c.toneMappingHelp, 'hdr sdr']}>
          <Checkbox
            label={c.toneMapping}
            help={toneMapsAnywhere ? c.toneMappingHelp : c.toneMappingUnavailable}
            checked={form.toneMapping}
            onChange={(toneMapping) => update({ toneMapping })}
          />
        </Setting>
        <Setting text={[c.toneMappingAlgorithm, c.toneMappingAlgorithmHelp]}>
          <SelectField
            label={c.toneMappingAlgorithm}
            hint={c.toneMappingAlgorithmHelp}
            value={form.toneMappingAlgorithm}
            disabled={!form.toneMapping || !toneMapsAnywhere}
            options={toneMappingAlgorithms.map((algorithm) => ({
              value: algorithm,
              label: c.algorithms[algorithm],
              disabled:
                algorithm === 'bt2390' &&
                !(gpu?.toneMapping ?? false) &&
                form.toneMappingAlgorithm !== 'bt2390',
            }))}
            onValue={(toneMappingAlgorithm) => update({ toneMappingAlgorithm })}
          />
        </Setting>
        <Setting text={[c.toneMappingPeak, c.toneMappingPeakHelp, c.toneMappingDesat]}>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField
              label={c.toneMappingPeak}
              type="number"
              inputMode="numeric"
              min={0}
              max={toneMappingPeakRange.max}
              step={1}
              disabled={!form.toneMapping || !hardware.toneMapping}
              value={wholeNumberField(form.toneMappingPeak)}
              onValue={(value) => update({ toneMappingPeak: wholeNumber(value) })}
              required
            />
            <TextField
              label={c.toneMappingDesat}
              type="number"
              inputMode="decimal"
              min={toneMappingDesatRange.min}
              max={toneMappingDesatRange.max}
              step={0.1}
              disabled={!form.toneMapping || !hardware.toneMapping}
              value={form.toneMappingDesat < 0 ? '' : form.toneMappingDesat}
              onValue={(value) =>
                update({ toneMappingDesat: value.trim() === '' ? -1 : Number(value) })
              }
              required
            />
          </div>
          <p className="mt-1 text-xs text-muted">
            {hardware.toneMapping ? c.toneMappingPeakHelp : c.processorCannotToneMap}
          </p>
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.interlaced}>
        <Setting text={[c.deinterlaceMethod, c.deinterlaceMethodHelp]}>
          <SelectField
            label={c.deinterlaceMethod}
            hint={hardware.bwdif ? c.deinterlaceMethodHelp : c.noBwdif}
            value={form.deinterlaceMethod}
            options={deinterlaceMethods.map((method) => ({
              value: method,
              label: c.deinterlacers[method],
              disabled: method === 'bwdif' && !hardware.bwdif && form.deinterlaceMethod !== 'bwdif',
            }))}
            onValue={(deinterlaceMethod) => update({ deinterlaceMethod })}
          />
        </Setting>
        <Setting text={[c.deinterlaceDoubleRate, c.deinterlaceDoubleRateHelp]}>
          <Checkbox
            label={c.deinterlaceDoubleRate}
            help={c.deinterlaceDoubleRateHelp}
            checked={form.deinterlaceDoubleRate}
            onChange={(deinterlaceDoubleRate) => update({ deinterlaceDoubleRate })}
          />
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.audio}>
        <Setting text={[c.downmixAlgorithm, c.downmixAlgorithmHelp]}>
          <SelectField
            label={c.downmixAlgorithm}
            hint={c.downmixAlgorithmHelp}
            value={form.downmixAlgorithm}
            options={downmixAlgorithms.map((algorithm) => ({
              value: algorithm,
              label: c.downmixes[algorithm],
            }))}
            onValue={(downmixAlgorithm) => update({ downmixAlgorithm })}
          />
        </Setting>
        <Setting text={[c.downmixBoost, c.downmixBoostHelp]}>
          <TextField
            label={c.downmixBoost}
            hint={c.downmixBoostHelp}
            type="number"
            inputMode="decimal"
            min={downmixBoostRange.min}
            max={downmixBoostRange.max}
            step={0.1}
            value={form.downmixBoost || ''}
            onValue={(value) => update({ downmixBoost: Number(value) })}
            required
          />
        </Setting>
        <Setting text={[c.maxAudioChannels, c.maxAudioChannelsHelp]}>
          <SelectField
            label={c.maxAudioChannels}
            hint={c.maxAudioChannelsHelp}
            value={form.maxAudioChannels}
            options={audioChannelLimits.map((channels) => ({
              value: channels,
              label: c.audioChannels(channels),
            }))}
            onValue={(maxAudioChannels) => update({ maxAudioChannels })}
          />
        </Setting>
        <Setting text={[c.audioBitratePerChannel, c.audioBitratePerChannelHelp]}>
          <TextField
            label={c.audioBitratePerChannel}
            hint={c.audioBitratePerChannelHelp}
            type="number"
            inputMode="numeric"
            min={0}
            max={audioBitratePerChannelRange.max}
            step={1}
            value={wholeNumberField(form.audioBitratePerChannel)}
            onValue={(value) => update({ audioBitratePerChannel: wholeNumber(value) })}
            required
          />
        </Setting>
      </SettingsGroup>

      <SettingsGroup title={c.groups.performance}>
        <Setting text={[c.encodingThreads, c.encodingThreadsHelp]}>
          <TextField
            label={c.encodingThreads}
            hint={c.encodingThreadsHelp}
            type="number"
            inputMode="numeric"
            min={encodingThreadsRange.min}
            max={encodingThreadsRange.max}
            step={1}
            value={wholeNumberField(form.encodingThreads)}
            onValue={(value) => update({ encodingThreads: wholeNumber(value) })}
            required
          />
        </Setting>
        <Setting text={[c.aheadSegments, c.aheadSegmentsHelp, 'throttle']}>
          <TextField
            label={c.aheadSegments}
            hint={c.aheadSegmentsHelp}
            type="number"
            inputMode="numeric"
            min={aheadSegmentsRange.min}
            max={aheadSegmentsRange.max}
            step={1}
            value={form.aheadSegments || ''}
            onValue={(value) => update({ aheadSegments: Math.trunc(Number(value)) })}
            required
          />
        </Setting>
      </SettingsGroup>
    </>
  )
}

/** One line of what was detected. */
function Detail({ term, children }: { term: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 px-4 py-2.5 sm:flex-row sm:items-baseline sm:justify-between sm:gap-6">
      <dt className="text-muted">{term}</dt>
      <dd className="flex flex-wrap items-center gap-1.5 text-zinc-100 sm:justify-end">
        {children}
      </dd>
    </div>
  )
}

function YesNo({ value }: { value: boolean }) {
  const { t } = useI18n()
  return (
    <Badge tone={value ? 'fin' : 'muted'}>
      {value ? t.settings.conversion.yes : t.settings.conversion.no}
    </Badge>
  )
}

function Codes({ names }: { names: string[] }) {
  const { t } = useI18n()
  if (names.length === 0) {
    return <span className="text-muted">{t.settings.conversion.none}</span>
  }
  return names.map((name) => (
    <code key={name} className="rounded bg-bg px-1.5 py-0.5 font-mono text-xs text-fin-5">
      {name}
    </code>
  ))
}
