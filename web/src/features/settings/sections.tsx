import { ArrowSquareOutIcon, PlayIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  analysisTimeoutRange,
  backupsKeptRange,
  catalogLimitRange,
  catalogRefreshMinutesRange,
  channelLimitRange,
  customCodeMaxBytes,
  fetchStatus,
  inactiveDeviceDaysRange,
  liveTvRefreshHoursRange,
  loginAttemptsRange,
  loginDisclaimerMaxBytes,
  playedPercentRange,
  queryKeys,
  recordingPaddingMinutesRange,
  recordingRetentionDaysRange,
  resumePercentRange,
  revealServerSecret,
  thumbnailStorageRange,
  trickplayIntervalRange,
  trickplayWidths,
  versionAttemptsRange,
  versionListMinutesRange,
} from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { formatHour } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'
import { ExternalButtonLink, Notice, NumberInput, SecretField, Select, TextInput } from '@/ui'
import CodeEditor from './CodeEditor'
import ConversionSection from './ConversionSection'
import { LastBackup, Variables } from './Diagnostics'
import { shownNumber, wholeNumber } from './numbers'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow, TextRow } from './parts'
import type { SectionFormApi } from './SectionForm'
import SegmentSources from './SegmentSources'

/** The redirect URI of a Trakt or Simkl app whose users enter a code instead of being redirected. */
const oobRedirectUri = 'urn:ietf:wg:oauth:2.0:oob'

/** The fields of each section, drawn inside its SectionForm. */
export const sectionFields: Record<SettingsSectionId, (api: SectionFormApi) => ReactNode> = {
  general: (api) => <General {...api} />,
  playback: (api) => <Playback {...api} />,
  conversion: (api) => <ConversionSection {...api} />,
  content: (api) => <Content {...api} />,
  catalogs: (api) => <Catalogs {...api} />,
  thumbnails: (api) => <Thumbnails {...api} />,
  security: (api) => <Security {...api} />,
  tracking: (api) => <Tracking {...api} />,
  'live-tv': (api) => <LiveTv {...api} />,
  recordings: (api) => <Recordings {...api} />,
  backups: (api) => <Backups {...api} />,
  'web-player': (api) => <WebPlayer {...api} />,
  diagnostics: (api) => <DiagnosticsFields {...api} />,
}

function General({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="server-name"
        label={s.serverName}
        help={s.serverNameHelp}
        error={error('server-name')}
      >
        <TextInput
          value={form.serverName}
          onValue={(serverName) => update({ serverName })}
          maxLength={64}
          required
          className="max-w-sm"
        />
      </FieldRow>
      <FieldRow
        anchor="language"
        label={s.language}
        help={s.languageHelp}
        error={error('language')}
      >
        <Select
          value={form.language}
          options={languages.map((code) => ({ value: code, label: t.language[code].name }))}
          onValue={(language: Language) => update({ language })}
          className="max-w-xs"
        />
      </FieldRow>
      <SwitchRow
        anchor="quick-connect"
        label={s.quickConnect}
        help={s.quickConnectHelp}
        checked={form.quickConnectEnabled}
        onChange={(quickConnectEnabled) => update({ quickConnectEnabled })}
      />
      <SwitchRow
        anchor="legacy-authorization"
        label={s.legacyAuthorization}
        help={s.legacyAuthorizationHelp}
        checked={form.legacyAuthorization}
        onChange={(legacyAuthorization) => update({ legacyAuthorization })}
      >
        <Notice tone="warn">
          <strong className="font-medium">{s.legacyWarningTitle}.</strong> {s.legacyWarning}
        </Notice>
      </SwitchRow>
    </SettingsGroup>
  )
}

function Playback({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SwitchRow
        anchor="chapters"
        label={s.chapters}
        help={s.chaptersHelp}
        checked={form.chapters}
        onChange={(chapters) => update({ chapters })}
      />
      <SwitchRow
        anchor="prepare-ahead"
        label={s.prepareAhead}
        help={s.prepareAheadHelp}
        checked={form.prepareAhead}
        onChange={(prepareAhead) => update({ prepareAhead })}
      />
      <SwitchRow
        anchor="downloads"
        label={s.downloads}
        help={s.downloadsHelp}
        checked={form.downloads}
        onChange={(downloads) => update({ downloads })}
      />
      <SwitchRow
        anchor="prefer-direct-play"
        label={s.preferDirectPlay}
        help={s.preferDirectPlayHelp}
        checked={form.preferDirectPlay}
        onChange={(preferDirectPlay) => update({ preferDirectPlay })}
      />
      <FieldRow
        anchor="analysis-timeout"
        label={s.analysisTimeout}
        help={s.analysisTimeoutHelp}
        error={error('analysis-timeout')}
      >
        <NumberInput
          min={analysisTimeoutRange.min}
          max={analysisTimeoutRange.max}
          step={1}
          value={shownNumber(form.analysisTimeout)}
          onValue={(value) => update({ analysisTimeout: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="version-attempts"
        label={s.versionAttempts}
        help={s.versionAttemptsHelp}
        error={error('version-attempts')}
      >
        <NumberInput
          min={versionAttemptsRange.min}
          max={versionAttemptsRange.max}
          step={1}
          value={shownNumber(form.versionAttempts)}
          onValue={(value) => update({ versionAttempts: wholeNumber(value) })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Content({ form, update, error, saves }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const groups = t.settingsPage.groups
  return (
    <>
      <SettingsGroup title={groups.skip}>
        <SwitchRow
          anchor="skip-buttons"
          label={s.skipButtons}
          help={s.skipButtonsHelp}
          checked={form.skipButtons}
          onChange={(skipButtons) => update({ skipButtons })}
        />
        <SettingRow anchor="segment-order">
          <SegmentSources
            order={form.segmentOrder}
            defaultOrder={form.segmentOrderDefault}
            off={form.segmentSourcesOff}
            publicMetaDbKey={
              form.publicMetaDbKey === undefined
                ? form.publicMetaDbKeySet
                : form.publicMetaDbKey.trim() !== ''
            }
            onOrder={(segmentOrder) => update({ segmentOrder })}
            error={error('segment-order')}
          />
        </SettingRow>
        <SettingRow
          anchor="publicmetadb-key"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            key={saves}
            label={s.publicMetaDbKey}
            help={s.publicMetaDbKeyHelp}
            saved={form.publicMetaDbKeySet}
            value={form.publicMetaDbKey}
            onValue={(publicMetaDbKey) => update({ publicMetaDbKey })}
            error={error('publicmetadb-key')}
            reveal={() => revealServerSecret('publicMetaDbKey')}
          />
        </SettingRow>
        <SettingRow
          anchor="theintrodb-key"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            key={saves}
            label={s.theIntroDbKey}
            help={s.theIntroDbKeyHelp}
            saved={form.theIntroDbKeySet}
            value={form.theIntroDbKey}
            onValue={(theIntroDbKey) => update({ theIntroDbKey })}
            error={error('theintrodb-key')}
            reveal={() => revealServerSecret('theIntroDbKey')}
          />
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title={groups.titlePages}>
        <SwitchRow
          anchor="similar-titles"
          label={s.similarTitles}
          help={s.similarTitlesHelp}
          checked={form.similarTitles}
          onChange={(similarTitles) => update({ similarTitles })}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.thresholds}>
        <div className="grid gap-x-8 sm:grid-cols-2">
          <FieldRow
            anchor="played-percent"
            label={s.playedPercent}
            help={s.playedPercentHelp}
            error={error('played-percent')}
          >
            <NumberInput
              min={playedPercentRange.min}
              max={playedPercentRange.max}
              step={1}
              suffix="%"
              value={shownNumber(form.playedPercent)}
              onValue={(value) => update({ playedPercent: wholeNumber(value) })}
            />
          </FieldRow>
          <FieldRow
            anchor="resume-percent"
            label={s.resumePercent}
            help={s.resumePercentHelp}
            error={error('resume-percent')}
          >
            <NumberInput
              min={resumePercentRange.min}
              max={resumePercentRange.max}
              step={1}
              suffix="%"
              // 0 is a valid threshold: it is shown, not left blank.
              value={shownNumber(form.resumePercent)}
              onValue={(value) => update({ resumePercent: wholeNumber(value) })}
            />
          </FieldRow>
        </div>
      </SettingsGroup>
    </>
  )
}

function Catalogs({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="catalog-limit"
        label={s.catalogLimit}
        help={s.catalogLimitHelp}
        error={error('catalog-limit')}
      >
        <NumberInput
          min={catalogLimitRange.min}
          max={catalogLimitRange.max}
          step={1}
          value={shownNumber(form.catalogLimit)}
          onValue={(value) => update({ catalogLimit: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="channel-limit"
        label={s.channelLimit}
        help={s.channelLimitHelp}
        error={error('channel-limit')}
      >
        <NumberInput
          min={channelLimitRange.min}
          max={channelLimitRange.max}
          step={1}
          value={shownNumber(form.channelLimit)}
          onValue={(value) => update({ channelLimit: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="version-list-minutes"
        label={s.versionListMinutes}
        help={s.versionListMinutesHelp}
        error={error('version-list-minutes')}
      >
        <NumberInput
          min={versionListMinutesRange.min}
          max={versionListMinutesRange.max}
          step={1}
          value={shownNumber(form.versionListMinutes)}
          onValue={(value) => update({ versionListMinutes: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="catalog-refresh-minutes"
        label={s.catalogRefreshMinutes}
        help={s.catalogRefreshMinutesHelp}
        error={error('catalog-refresh-minutes')}
      >
        <NumberInput
          min={catalogRefreshMinutesRange.min}
          max={catalogRefreshMinutesRange.max}
          step={1}
          value={shownNumber(form.catalogRefreshMinutes)}
          onValue={(value) => update({ catalogRefreshMinutes: wholeNumber(value) })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Thumbnails({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SwitchRow
        anchor="trickplay"
        label={s.trickplay}
        help={s.trickplayHelp}
        checked={form.trickplay}
        onChange={(trickplay) => update({ trickplay })}
      />
      <FieldRow
        anchor="trickplay-interval"
        label={s.trickplayInterval}
        help={s.trickplayIntervalHelp}
        error={error('trickplay-interval')}
      >
        <NumberInput
          min={trickplayIntervalRange.min}
          max={trickplayIntervalRange.max}
          step={1}
          value={shownNumber(form.trickplayInterval)}
          onValue={(value) => update({ trickplayInterval: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="trickplay-width"
        label={s.trickplayWidth}
        help={s.trickplayWidthHelp}
        error={error('trickplay-width')}
      >
        <Select
          value={form.trickplayWidth}
          options={trickplayWidths.map((width) => ({ value: width, label: s.pixels(width) }))}
          onValue={(trickplayWidth) => update({ trickplayWidth })}
          className="max-w-xs"
        />
      </FieldRow>
      <SwitchRow
        anchor="chapter-images"
        label={s.chapterImages}
        help={s.chapterImagesHelp}
        checked={form.chapterImages}
        onChange={(chapterImages) => update({ chapterImages })}
      />
      <FieldRow
        anchor="thumbnail-storage"
        label={s.thumbnailStorage}
        help={s.thumbnailStorageHelp}
        error={error('thumbnail-storage')}
      >
        <NumberInput
          min={thumbnailStorageRange.min}
          max={thumbnailStorageRange.max}
          step={1}
          value={shownNumber(form.thumbnailStorageGB)}
          onValue={(value) => update({ thumbnailStorageGB: wholeNumber(value) })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Security({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SwitchRow
        anchor="personal-addons"
        label={s.personalAddons}
        help={s.personalAddonsHelp}
        checked={form.personalAddons}
        onChange={(personalAddons) => update({ personalAddons })}
      />
      <FieldRow
        anchor="login-attempts"
        label={s.loginAttempts}
        help={s.loginAttemptsHelp}
        error={error('login-attempts')}
      >
        <NumberInput
          min={0}
          max={loginAttemptsRange.max}
          step={1}
          value={shownNumber(form.loginAttempts)}
          onValue={(value) => update({ loginAttempts: wholeNumber(value) })}
        />
      </FieldRow>
      <FieldRow
        anchor="inactive-device-days"
        label={s.inactiveDeviceDays}
        help={s.inactiveDeviceDaysHelp}
        error={error('inactive-device-days')}
      >
        <NumberInput
          min={inactiveDeviceDaysRange.min}
          max={inactiveDeviceDaysRange.max}
          step={1}
          value={shownNumber(form.inactiveDeviceDays)}
          onValue={(value) => update({ inactiveDeviceDays: wholeNumber(value) })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Tracking({ form, update, error, saves }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings.tracking
  return (
    <>
      <SettingsGroup title="Trakt">
        <TextRow anchor="trakt-setup">
          <p className="max-w-[60ch]">{s.traktSetup}</p>
          <p className="mt-2 text-ink-3">
            {s.redirectUri}
            {t.common.colon}{' '}
            <code className="rounded-check bg-s3 px-1.5 py-0.5 font-mono text-micro break-all text-ink select-all">
              {oobRedirectUri}
            </code>
          </p>
        </TextRow>
        <FieldRow
          anchor="trakt-client-id"
          label={s.traktClientId}
          help={s.traktClientIdHelp}
          error={error('trakt-client-id')}
        >
          <TextInput
            value={form.traktClientId}
            onValue={(traktClientId) => update({ traktClientId })}
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </FieldRow>
        <SettingRow
          anchor="trakt-client-secret"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            key={saves}
            label={s.traktClientSecret}
            help={s.traktClientSecretHelp}
            saved={form.traktClientSecretSet}
            value={form.traktClientSecret}
            onValue={(traktClientSecret) => update({ traktClientSecret })}
            reveal={() => revealServerSecret('traktClientSecret')}
          />
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title="Simkl">
        <TextRow anchor="simkl-setup">
          <p className="max-w-[60ch]">{s.simklSetup}</p>
        </TextRow>
        <FieldRow
          anchor="simkl-client-id"
          label={s.simklClientId}
          help={s.simklClientIdHelp}
          error={error('simkl-client-id')}
        >
          <TextInput
            value={form.simklClientId}
            onValue={(simklClientId) => update({ simklClientId })}
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </FieldRow>
      </SettingsGroup>
    </>
  )
}

function LiveTv({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="live-tv-refresh-hours"
        label={s.liveTvRefreshHours}
        help={s.liveTvRefreshHoursHelp}
        error={error('live-tv-refresh-hours')}
      >
        <NumberInput
          min={liveTvRefreshHoursRange.min}
          max={liveTvRefreshHoursRange.max}
          step={1}
          value={shownNumber(form.liveTvRefreshHours)}
          onValue={(value) => update({ liveTvRefreshHours: wholeNumber(value) })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Recordings({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      {form.recordingsFolder === '' && (
        <Notice tone="info" className="mt-8">
          {s.recordingsOff}
        </Notice>
      )}
      <SettingsGroup>
        <FieldRow
          anchor="recording-pre-padding"
          label={s.recordingPrePadding}
          help={s.recordingPrePaddingHelp}
          error={error('recording-pre-padding')}
        >
          <NumberInput
            min={recordingPaddingMinutesRange.min}
            max={recordingPaddingMinutesRange.max}
            step={1}
            value={shownNumber(Math.round(form.recordingPrePadding / 60))}
            onValue={(value) => update({ recordingPrePadding: wholeNumber(value) * 60 })}
          />
        </FieldRow>
        <FieldRow
          anchor="recording-post-padding"
          label={s.recordingPostPadding}
          help={s.recordingPostPaddingHelp}
          error={error('recording-post-padding')}
        >
          <NumberInput
            min={recordingPaddingMinutesRange.min}
            max={recordingPaddingMinutesRange.max}
            step={1}
            value={shownNumber(Math.round(form.recordingPostPadding / 60))}
            onValue={(value) => update({ recordingPostPadding: wholeNumber(value) * 60 })}
          />
        </FieldRow>
        <FieldRow
          anchor="recording-retention-days"
          label={s.recordingRetentionDays}
          help={s.recordingRetentionDaysHelp}
          error={error('recording-retention-days')}
        >
          <NumberInput
            min={recordingRetentionDaysRange.min}
            max={recordingRetentionDaysRange.max}
            step={1}
            value={shownNumber(form.recordingRetentionDays)}
            onValue={(value) => update({ recordingRetentionDays: wholeNumber(value) })}
          />
        </FieldRow>
      </SettingsGroup>
    </>
  )
}

function Backups({ form, update, error }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  return (
    <>
      {form.backupFolder === '' && (
        <Notice tone="info" className="mt-8">
          {s.backupsOff}
        </Notice>
      )}
      <SettingsGroup>
        <FieldRow
          anchor="backup-hour"
          label={s.backupHour}
          help={s.backupHourHelp}
          error={error('backup-hour')}
        >
          <Select
            value={form.backupHour}
            options={Array.from({ length: 24 }, (_, hour) => ({
              value: hour,
              label: formatHour(hour, language),
            }))}
            onValue={(backupHour) => update({ backupHour })}
            className="max-w-[10rem]"
          />
        </FieldRow>
        <FieldRow
          anchor="backups-kept"
          label={s.backupsKept}
          help={s.backupsKeptHelp}
          error={error('backups-kept')}
        >
          <NumberInput
            min={backupsKeptRange.min}
            max={backupsKeptRange.max}
            step={1}
            value={shownNumber(form.backupsKept)}
            onValue={(value) => update({ backupsKept: wholeNumber(value) })}
          />
        </FieldRow>
        {form.backupFolder !== '' && <LastBackup />}
      </SettingsGroup>
    </>
  )
}

function WebPlayer({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  return (
    <SettingsGroup>
      {status.data?.webClient === true && (
        <SettingRow anchor="open-web-player">
          <ExternalButtonLink
            href="/web/"
            target="_blank"
            rel="noopener"
            icon={PlayIcon}
            iconEnd={ArrowSquareOutIcon}
          >
            {s.openWebPlayer}
          </ExternalButtonLink>
        </SettingRow>
      )}
      <SettingRow anchor="custom-css">
        <CodeEditor
          label={s.customCss}
          help={s.customCssHelp}
          value={form.customCss}
          onValue={(customCss) => update({ customCss })}
          maxBytes={customCodeMaxBytes}
          error={error('custom-css')}
        />
      </SettingRow>
      <SettingRow anchor="custom-js">
        <CodeEditor
          label={s.customJs}
          help={s.customJsHelp}
          value={form.customJs}
          onValue={(customJs) => update({ customJs })}
          maxBytes={customCodeMaxBytes}
          error={error('custom-js')}
        >
          <Notice tone="warn">
            <strong className="font-medium">{s.customJsWarningTitle}.</strong> {s.customJsWarning}
          </Notice>
        </CodeEditor>
      </SettingRow>
      <SettingRow anchor="login-disclaimer">
        <CodeEditor
          label={s.loginDisclaimer}
          help={s.loginDisclaimerHelp}
          value={form.loginDisclaimer}
          onValue={(loginDisclaimer) => update({ loginDisclaimer })}
          maxBytes={loginDisclaimerMaxBytes}
          rows={4}
          error={error('login-disclaimer')}
        />
      </SettingRow>
    </SettingsGroup>
  )
}

function DiagnosticsFields({ form, update }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SwitchRow
          anchor="detailed-log"
          label={s.detailedLog}
          help={s.detailedLogHelp}
          checked={form.detailedLog}
          onChange={(detailedLog) => update({ detailedLog })}
        />
      </SettingsGroup>
      <Variables />
    </>
  )
}
