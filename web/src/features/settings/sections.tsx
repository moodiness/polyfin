import { ArrowSquareOutIcon, PlayIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { fetchStatus, queryKeys, revealServerSecret } from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { formatHour } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'
import { ExternalButtonLink, Notice, NumberInput, SecretField, Select, TextInput } from '@/ui'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import CodeEditor from './CodeEditor'
import ConversionSection from './ConversionSection'
import { LastBackup, Variables } from './Diagnostics'
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
  notifications: (api) => <Notifications {...api} />,
  'web-player': (api) => <WebPlayer {...api} />,
  diagnostics: (api) => <DiagnosticsFields {...api} />,
}

function General({ form, update, error, range, limits }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="server-name"
        label={s.serverName}
        help={s.serverNameHelp(range('serverName'))}
        error={error('server-name')}
      >
        <TextInput
          value={form.serverName}
          onValue={(serverName) => update({ serverName })}
          maxLength={limits('serverName').max}
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
      <SwitchRow
        anchor="update-check"
        label={s.updateCheck}
        help={s.updateCheckHelp}
        checked={form.updateCheck}
        onChange={(updateCheck) => update({ updateCheck })}
      />
    </SettingsGroup>
  )
}

function Playback({ form, update, error, number, range, limits }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SwitchRow
          anchor="prepare-ahead"
          label={s.prepareAhead}
          help={s.prepareAheadHelp}
          checked={form.prepareAhead}
          onChange={(prepareAhead) => update({ prepareAhead })}
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
          help={s.analysisTimeoutHelp(range('analysisTimeout'))}
          error={error('analysis-timeout')}
        >
          <NumberInput
            {...limits('analysisTimeout')}
            step={1}
            {...number('analysis-timeout', form.analysisTimeout, (value) =>
              update({ analysisTimeout: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="version-attempts"
          label={s.versionAttempts}
          help={s.versionAttemptsHelp(range('versionAttempts'))}
          error={error('version-attempts')}
        >
          <NumberInput
            {...limits('versionAttempts')}
            step={1}
            {...number('version-attempts', form.versionAttempts, (value) =>
              update({ versionAttempts: Math.trunc(value) }),
            )}
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
              update({ cacheSizeGb: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <SwitchRow
          anchor="remuxdb"
          label={s.remuxDb}
          help={s.remuxDbHelp}
          checked={form.remuxDb}
          onChange={(remuxDb) => update({ remuxDb })}
        />
        <FieldRow
          anchor="remuxdb-url"
          label={s.remuxDbUrl}
          help={s.remuxDbUrlHelp}
          error={error('remuxdb-url')}
        >
          <TextInput
            inputMode="url"
            value={form.remuxDbUrl}
            onValue={(remuxDbUrl) => update({ remuxDbUrl })}
            disabled={!form.remuxDb}
            maxLength={limits('remuxDbUrl').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
      </SettingsGroup>
      <SettingsGroup title={s.playbackHistoryGroup}>
        <SwitchRow
          anchor="playback-history"
          label={s.playbackHistory}
          help={s.playbackHistoryHelp}
          checked={form.playbackHistory}
          onChange={(playbackHistory) => update({ playbackHistory })}
        />
        <FieldRow
          anchor="playback-history-days"
          label={s.playbackHistoryDays}
          help={s.playbackHistoryDaysHelp(range('playbackHistoryDays'))}
          error={error('playback-history-days')}
        >
          <NumberInput
            {...limits('playbackHistoryDays')}
            step={1}
            {...number('playback-history-days', form.playbackHistoryDays, (value) =>
              update({ playbackHistoryDays: Math.trunc(value) }),
            )}
          />
        </FieldRow>
      </SettingsGroup>
    </>
  )
}

function Content({ form, update, error, number, range, limits }: SectionFormApi) {
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
            off={form.segmentSourcesOff}
            publicMetaDbKey={
              form.publicMetaDbKey === undefined
                ? form.publicMetaDbKeySet
                : form.publicMetaDbKey.trim() !== ''
            }
            onChange={(segmentOrder, segmentSourcesOff) =>
              update({ segmentOrder, segmentSourcesOff })
            }
            error={error('segment-order')}
          />
        </SettingRow>
        <SettingRow
          anchor="publicmetadb-key"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
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
      <SettingsGroup title={groups.music}>
        <SwitchRow
          anchor="lyrics"
          label={s.lyrics}
          help={s.lyricsHelp}
          checked={form.lyrics}
          onChange={(lyrics) => update({ lyrics })}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.thresholds}>
        <div className="grid gap-x-8 sm:grid-cols-2">
          <FieldRow
            anchor="played-percent"
            label={s.playedPercent}
            help={s.playedPercentHelp(range('playedPercent'))}
            error={error('played-percent')}
          >
            <NumberInput
              {...limits('playedPercent')}
              step={1}
              suffix="%"
              {...number('played-percent', form.playedPercent, (value) =>
                update({ playedPercent: Math.trunc(value) }),
              )}
            />
          </FieldRow>
          <FieldRow
            anchor="resume-percent"
            label={s.resumePercent}
            help={s.resumePercentHelp(range('resumePercent'))}
            error={error('resume-percent')}
          >
            <NumberInput
              {...limits('resumePercent')}
              step={1}
              suffix="%"
              // 0 is a valid threshold: it is shown, not left blank.
              {...number('resume-percent', form.resumePercent, (value) =>
                update({ resumePercent: Math.trunc(value) }),
              )}
            />
          </FieldRow>
        </div>
      </SettingsGroup>
    </>
  )
}

function Catalogs({ form, update, error, number, range, limits }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="catalog-limit"
        label={s.catalogLimit}
        help={s.catalogLimitHelp(range('catalogLimit'))}
        error={error('catalog-limit')}
      >
        <NumberInput
          {...limits('catalogLimit')}
          step={1}
          {...number('catalog-limit', form.catalogLimit, (value) =>
            update({ catalogLimit: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <FieldRow
        anchor="channel-limit"
        label={s.channelLimit}
        help={s.channelLimitHelp(range('channelLimit'))}
        error={error('channel-limit')}
      >
        <NumberInput
          {...limits('channelLimit')}
          step={1}
          {...number('channel-limit', form.channelLimit, (value) =>
            update({ channelLimit: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <FieldRow
        anchor="version-list-minutes"
        label={s.versionListMinutes}
        help={s.versionListMinutesHelp(range('versionListMinutes'))}
        error={error('version-list-minutes')}
      >
        <NumberInput
          {...limits('versionListMinutes')}
          step={1}
          {...number('version-list-minutes', form.versionListMinutes, (value) =>
            update({ versionListMinutes: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <FieldRow
        anchor="catalog-refresh-minutes"
        label={s.catalogRefreshMinutes}
        help={s.catalogRefreshMinutesHelp(range('catalogRefreshMinutes'))}
        error={error('catalog-refresh-minutes')}
      >
        <NumberInput
          {...limits('catalogRefreshMinutes')}
          step={1}
          {...number('catalog-refresh-minutes', form.catalogRefreshMinutes, (value) =>
            update({ catalogRefreshMinutes: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <FieldRow
        anchor="collection-read-hour"
        label={s.collectionReadHour}
        help={s.collectionReadHourHelp}
        error={error('collection-read-hour')}
      >
        <Select
          value={form.collectionReadHour}
          options={[
            { value: -1, label: s.collectionReadHourNever },
            ...Array.from({ length: 24 }, (_, hour) => ({
              value: hour,
              label: formatHour(hour, language),
            })),
          ]}
          onValue={(collectionReadHour) => update({ collectionReadHour })}
          className="max-w-[10rem]"
        />
      </FieldRow>
      <FieldRow
        anchor="local-scan-hours"
        label={s.localScanHours}
        help={s.localScanHoursHelp(range('localScanHours'))}
        error={error('local-scan-hours')}
      >
        <NumberInput
          {...limits('localScanHours')}
          step={1}
          {...number('local-scan-hours', form.localScanHours, (value) =>
            update({ localScanHours: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <SwitchRow
        anchor="watch-local-folders"
        label={s.watchLocalFolders}
        help={s.watchLocalFoldersHelp}
        checked={form.watchLocalFolders}
        onChange={(watchLocalFolders) => update({ watchLocalFolders })}
      />
    </SettingsGroup>
  )
}

function Thumbnails({ form, update, error, number, range, limits }: SectionFormApi) {
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
        help={s.trickplayIntervalHelp(range('trickplayInterval'))}
        error={error('trickplay-interval')}
      >
        <NumberInput
          {...limits('trickplayInterval')}
          step={1}
          {...number('trickplay-interval', form.trickplayInterval, (value) =>
            update({ trickplayInterval: Math.trunc(value) }),
          )}
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
          options={((form.bounds.trickplayWidth?.choices ?? []) as number[]).map((width) => ({
            value: width,
            label: s.pixels(width),
          }))}
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
        help={s.thumbnailStorageHelp(range('thumbnailStorageGB'))}
        error={error('thumbnail-storage')}
      >
        <NumberInput
          {...limits('thumbnailStorageGB')}
          step={1}
          {...number('thumbnail-storage', form.thumbnailStorageGB, (value) =>
            update({ thumbnailStorageGB: Math.trunc(value) }),
          )}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Security({ form, update, error, number, range, limits }: SectionFormApi) {
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
      <SwitchRow
        anchor="server-imports"
        label={s.serverImports}
        help={s.serverImportsHelp}
        checked={form.serverImports}
        onChange={(serverImports) => update({ serverImports })}
      />
      <FieldRow
        anchor="login-attempts"
        label={s.loginAttempts}
        help={s.loginAttemptsHelp(range('loginAttempts'))}
        error={error('login-attempts')}
      >
        <NumberInput
          {...limits('loginAttempts')}
          step={1}
          {...number('login-attempts', form.loginAttempts, (value) =>
            update({ loginAttempts: Math.trunc(value) }),
          )}
        />
      </FieldRow>
      <FieldRow
        anchor="inactive-device-days"
        label={s.inactiveDeviceDays}
        help={s.inactiveDeviceDaysHelp(range('inactiveDeviceDays'))}
        error={error('inactive-device-days')}
      >
        <NumberInput
          {...limits('inactiveDeviceDays')}
          step={1}
          {...number('inactive-device-days', form.inactiveDeviceDays, (value) =>
            update({ inactiveDeviceDays: Math.trunc(value) }),
          )}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Tracking({ form, update, error }: SectionFormApi) {
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
      <SettingsGroup title="Last.fm">
        <TextRow anchor="lastfm-setup">
          <p className="max-w-[60ch]">{s.lastFmSetup}</p>
        </TextRow>
        <FieldRow
          anchor="lastfm-api-key"
          label={s.lastFmApiKey}
          help={s.lastFmApiKeyHelp}
          error={error('lastfm-api-key')}
        >
          <TextInput
            value={form.lastFmApiKey}
            onValue={(lastFmApiKey) => update({ lastFmApiKey })}
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </FieldRow>
        <SettingRow
          anchor="lastfm-secret"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.lastFmSecret}
            help={s.lastFmSecretHelp}
            saved={form.lastFmSecretSet}
            value={form.lastFmSecret}
            onValue={(lastFmSecret) => update({ lastFmSecret })}
            reveal={() => revealServerSecret('lastFmSecret')}
          />
        </SettingRow>
      </SettingsGroup>
    </>
  )
}

function LiveTv({ form, update, error, number, range, limits }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="live-tv-refresh-hours"
        label={s.liveTvRefreshHours}
        help={s.liveTvRefreshHoursHelp(range('liveTvRefreshHours'))}
        error={error('live-tv-refresh-hours')}
      >
        <NumberInput
          {...limits('liveTvRefreshHours')}
          step={1}
          {...number('live-tv-refresh-hours', form.liveTvRefreshHours, (value) =>
            update({ liveTvRefreshHours: Math.trunc(value) }),
          )}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

function Recordings({ form, update, error, number, range, limits }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SwitchRow
          anchor="recording"
          label={s.recording}
          help={s.recordingHelp}
          checked={form.recording}
          onChange={(recording) => update({ recording })}
        />
        <FieldRow
          anchor="recordings-folder"
          label={s.recordingsFolderLabel}
          help={s.recordingsFolderHelp}
          error={error('recordings-folder')}
        >
          <TextInput
            value={form.recordingsFolder}
            onValue={(recordingsFolder) => update({ recordingsFolder })}
            placeholder={form.recordingsFolderDefault}
            disabled={!form.recording}
            maxLength={limits('recordingsFolder').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
        <FieldRow
          anchor="recording-pre-padding"
          label={s.recordingPrePadding}
          help={s.recordingPrePaddingHelp(range('recordingPrePadding', 1 / 60))}
          error={error('recording-pre-padding')}
        >
          <NumberInput
            {...limits('recordingPrePadding', 1 / 60)}
            step={1}
            {...number(
              'recording-pre-padding',
              Math.round(form.recordingPrePadding / 60),
              (value) => update({ recordingPrePadding: Math.trunc(value) * 60 }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="recording-post-padding"
          label={s.recordingPostPadding}
          help={s.recordingPostPaddingHelp(range('recordingPostPadding', 1 / 60))}
          error={error('recording-post-padding')}
        >
          <NumberInput
            {...limits('recordingPostPadding', 1 / 60)}
            step={1}
            {...number(
              'recording-post-padding',
              Math.round(form.recordingPostPadding / 60),
              (value) => update({ recordingPostPadding: Math.trunc(value) * 60 }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="recording-retention-days"
          label={s.recordingRetentionDays}
          help={s.recordingRetentionDaysHelp(range('recordingRetentionDays'))}
          error={error('recording-retention-days')}
        >
          <NumberInput
            {...limits('recordingRetentionDays')}
            step={1}
            {...number('recording-retention-days', form.recordingRetentionDays, (value) =>
              update({ recordingRetentionDays: Math.trunc(value) }),
            )}
          />
        </FieldRow>
      </SettingsGroup>
    </>
  )
}

function Backups({ form, update, error, number, range, limits }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SwitchRow
          anchor="backups"
          label={s.backups}
          help={s.backupsHelp}
          checked={form.backups}
          onChange={(backups) => update({ backups })}
        />
        <FieldRow
          anchor="backup-folder"
          label={s.backupFolderLabel}
          help={s.backupFolderHelp}
          error={error('backup-folder')}
        >
          <TextInput
            value={form.backupFolder}
            onValue={(backupFolder) => update({ backupFolder })}
            placeholder={form.backupFolderDefault}
            disabled={!form.backups}
            maxLength={limits('backupFolder').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
        <FieldRow
          anchor="backup-hour"
          label={s.backupHour}
          help={s.backupHourHelp(formatHour(Number(form.bounds.backupHour?.default), language))}
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
          help={s.backupsKeptHelp(range('backupsKept'))}
          error={error('backups-kept')}
        >
          <NumberInput
            {...limits('backupsKept')}
            step={1}
            {...number('backups-kept', form.backupsKept, (value) =>
              update({ backupsKept: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        {form.backups && <LastBackup />}
      </SettingsGroup>
    </>
  )
}

function WebPlayer({ form, update, error, limits }: SectionFormApi) {
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
          maxBytes={limits('customCss').max ?? 0}
          error={error('custom-css')}
        />
      </SettingRow>
      <SettingRow anchor="custom-js">
        <CodeEditor
          label={s.customJs}
          help={s.customJsHelp}
          value={form.customJs}
          onValue={(customJs) => update({ customJs })}
          maxBytes={limits('customJs').max ?? 0}
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
          maxBytes={limits('loginDisclaimer').max ?? 0}
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

function Notifications({ form, update, error, number, limits }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  const text = t.notifications
  // Sunday 7 January 2024 is day 0, as the server counts days; the week starts on Monday.
  const weekday = (day: number) =>
    new Intl.DateTimeFormat(language, { weekday: 'long', timeZone: 'UTC' }).format(
      Date.UTC(2024, 0, 7 + day),
    )
  return (
    <>
      <SettingsGroup>
        <FieldRow
          anchor="public-address"
          label={s.publicAddress}
          help={s.publicAddressHelp}
          error={error('public-address')}
        >
          <TextInput
            value={form.publicAddress}
            onValue={(publicAddress) => update({ publicAddress })}
            placeholder="https://media.example.org"
            maxLength={limits('publicAddress').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
      </SettingsGroup>
      <SettingsGroup title={s.weeklySummary}>
        <TextRow anchor="weekly-summary">
          <p className="max-w-[60ch]">{s.weeklySummaryHelp}</p>
        </TextRow>
        <FieldRow
          anchor="weekly-summary-day"
          label={s.weeklySummaryDay}
          help={s.weeklySummaryDayHelp(weekday(Number(form.bounds.weeklySummaryDay?.default)))}
          error={error('weekly-summary-day')}
        >
          <Select
            value={form.weeklySummaryDay}
            options={[1, 2, 3, 4, 5, 6, 0].map((day) => ({ value: day, label: weekday(day) }))}
            onValue={(weeklySummaryDay) => update({ weeklySummaryDay })}
            className="max-w-[12rem]"
          />
        </FieldRow>
        <FieldRow
          anchor="weekly-summary-hour"
          label={s.weeklySummaryHour}
          help={s.weeklySummaryHourHelp(
            formatHour(Number(form.bounds.weeklySummaryHour?.default), language),
          )}
          error={error('weekly-summary-hour')}
        >
          <Select
            value={form.weeklySummaryHour}
            options={Array.from({ length: 24 }, (_, hour) => ({
              value: hour,
              label: formatHour(hour, language),
            }))}
            onValue={(weeklySummaryHour) => update({ weeklySummaryHour })}
            className="max-w-[10rem]"
          />
        </FieldRow>
      </SettingsGroup>
      <SettingsGroup title={s.email}>
        <TextRow anchor="email-setup">
          <p className="max-w-[60ch]">{s.emailHelp}</p>
        </TextRow>
        <FieldRow
          anchor="smtp-host"
          label={s.smtpHost}
          help={s.smtpHostHelp}
          error={error('smtp-host')}
        >
          <TextInput
            value={form.smtpHost}
            onValue={(smtpHost) => update({ smtpHost })}
            placeholder="smtp.example.org"
            maxLength={limits('smtpHost').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
        <FieldRow
          anchor="smtp-port"
          label={s.smtpPort}
          help={s.smtpPortHelp}
          error={error('smtp-port')}
        >
          <NumberInput
            {...limits('smtpPort')}
            step={1}
            {...number('smtp-port', form.smtpPort, (value) =>
              update({ smtpPort: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <FieldRow
          anchor="smtp-security"
          label={s.smtpSecurity}
          help={s.smtpSecurityHelp}
          error={error('smtp-security')}
        >
          <Select
            value={form.smtpSecurity}
            options={(['starttls', 'tls', 'none'] as const).map((value) => ({
              value,
              label: s.smtpSecurities[value],
            }))}
            onValue={(smtpSecurity) => update({ smtpSecurity })}
            className="max-w-xs"
          />
        </FieldRow>
        <FieldRow
          anchor="smtp-user"
          label={s.smtpUser}
          help={s.smtpUserHelp}
          error={error('smtp-user')}
        >
          <TextInput
            value={form.smtpUser}
            onValue={(smtpUser) => update({ smtpUser })}
            maxLength={limits('smtpUser').max}
            autoComplete="off"
            spellCheck={false}
            className="max-w-md"
          />
        </FieldRow>
        <SettingRow
          anchor="smtp-password"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.smtpPassword}
            help={s.smtpPasswordHelp}
            saved={form.smtpPasswordSet}
            value={form.smtpPassword}
            onValue={(smtpPassword) => update({ smtpPassword })}
          />
        </SettingRow>
        <FieldRow
          anchor="smtp-from"
          label={s.smtpFrom}
          help={s.smtpFromHelp}
          error={error('smtp-from')}
        >
          <TextInput
            type="email"
            value={form.smtpFrom}
            onValue={(smtpFrom) => update({ smtpFrom })}
            placeholder="polyfin@example.org"
            maxLength={limits('smtpFrom').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
        </FieldRow>
        <FieldRow
          anchor="smtp-from-name"
          label={s.smtpFromName}
          help={s.smtpFromNameHelp}
          error={error('smtp-from-name')}
        >
          <TextInput
            value={form.smtpFromName}
            onValue={(smtpFromName) => update({ smtpFromName })}
            placeholder={form.serverName}
            maxLength={limits('smtpFromName').max}
            autoComplete="off"
            className="max-w-md"
          />
        </FieldRow>
      </SettingsGroup>
      <SettingsGroup title={text.serverTargets}>
        <SettingRow anchor="notification-targets">
          <p className="mb-4 max-w-[60ch] text-small text-ink-3">{text.serverTargetsHelp}</p>
          <NotificationTargets scope="server" />
        </SettingRow>
      </SettingsGroup>
    </>
  )
}
