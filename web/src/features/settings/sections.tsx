import { ArrowSquareOutIcon, PlayIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useId, useState, type ReactNode } from 'react'
import {
  fetchStatus,
  queryKeys,
  revealServerSecret,
  type AcmeChallenge,
  type HttpsMode,
  type Settings,
  type VersionCodec,
  type VersionCriterion,
  type WebTheme,
  webThemes,
} from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { formatHour } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'
import {
  Button,
  Checkbox,
  ExternalButtonLink,
  FieldError,
  Notice,
  NumberInput,
  SecretField,
  Select,
  Textarea,
  TextInput,
} from '@/ui'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import { computer, useServerSystem } from '@/features/sources/LocalFolders'
import CodeEditor from './CodeEditor'
import CollectionLibraryApps from './CollectionLibraryApps'
import BackupDestinations from './BackupDestinations'
import HomeRows from './HomeRows'
import ConversionSection from './ConversionSection'
import { LastBackup, Variables } from './Diagnostics'
import DlnaSection from './DlnaSection'
import TorrentsSection from './TorrentsSection'
import MetricsToken from './MetricsToken'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow, TextRow } from './parts'
import type { SectionFormApi } from './SectionForm'
import SegmentSources from './SegmentSources'
import VersionOrder, { CodecOrder, SourceOrder } from './VersionOrder'
import { EditAsCustom, shownTexts, ThemeCopyPanel } from './WebTheme'

/** The redirect URI of a Trakt or Simkl app whose users enter a code instead of being redirected. */
const oobRedirectUri = 'urn:ietf:wg:oauth:2.0:oob'

/** The redirect URL of an AniList app whose users paste the code AniList shows them. */
const aniListPinUri = 'https://anilist.co/api/v2/oauth/pin'

/**
 * The page of the admin app MyAnimeList sends users' browsers back to, at the server's public
 * address, else at the address the app is used at.
 */
function myAnimeListRedirect(publicAddress: string): string {
  const base = publicAddress.trim().replace(/\/+$/, '') || window.location.origin
  return `${base}/admin/me/account/myanimelist`
}

/** An address to copy into a service's app. */
function Address({ label, value }: { label: string; value: string }) {
  const { t } = useI18n()
  return (
    <p className="mt-2 text-ink-3">
      {label}
      {t.common.colon}{' '}
      <code className="rounded-check bg-s3 px-1.5 py-0.5 font-mono text-micro break-all text-ink select-all">
        {value}
      </code>
    </p>
  )
}

/** The fields of each section, drawn inside its SectionForm. */
export const sectionFields: Record<SettingsSectionId, (api: SectionFormApi) => ReactNode> = {
  general: (api) => <General {...api} />,
  playback: (api) => <Playback {...api} />,
  conversion: (api) => <ConversionSection {...api} />,
  content: (api) => <Content {...api} />,
  catalogs: (api) => <Catalogs {...api} />,
  thumbnails: (api) => <Thumbnails {...api} />,
  security: (api) => (
    <>
      <Security {...api} />
      <Https {...api} />
    </>
  ),
  tracking: (api) => <Tracking {...api} />,
  'live-tv': (api) => <LiveTv {...api} />,
  recordings: (api) => <Recordings {...api} />,
  dlna: (api) => <DlnaSection {...api} />,
  torrents: (api) => <TorrentsSection {...api} />,
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
      <SwitchRow
        anchor="local-discovery"
        label={s.localDiscovery}
        help={s.localDiscoveryHelp}
        checked={form.localDiscovery}
        onChange={(localDiscovery) => update({ localDiscovery })}
      />
      <FieldRow
        anchor="local-address"
        label={s.localAddress}
        help={s.localAddressHelp}
        error={error('local-address')}
      >
        <TextInput
          value={form.localAddress}
          onValue={(localAddress) => update({ localAddress })}
          placeholder="http://192.168.1.10:8096"
          maxLength={limits('localAddress').max}
          autoComplete="off"
          spellCheck={false}
          mono
          className="max-w-md"
        />
      </FieldRow>
      <SwitchRow
        anchor="metrics"
        label={s.metrics}
        help={s.metricsHelp}
        checked={form.metrics}
        onChange={(metrics) => update({ metrics })}
      />
      {form.metrics && (
        <SettingRow anchor="metrics-token">
          <MetricsToken saved={form.metricsTokenSet} />
        </SettingRow>
      )}
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
        <SwitchRow
          anchor="name-how-versions-play"
          label={s.nameHowVersionsPlay}
          help={s.nameHowVersionsPlayHelp}
          checked={form.nameHowVersionsPlay}
          onChange={(nameHowVersionsPlay) => update({ nameHowVersionsPlay })}
        />
        <SwitchRow
          anchor="sync-addon-subtitles"
          label={s.syncAddonSubtitles}
          help={s.syncAddonSubtitlesHelp}
          checked={form.syncAddonSubtitles}
          onChange={(syncAddonSubtitles) => update({ syncAddonSubtitles })}
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
      <SettingsGroup title={s.versionOrderGroup}>
        <SettingRow anchor="version-order">
          <VersionOrder
            order={form.versionOrder}
            choices={(form.bounds.versionOrder?.choices ?? []) as VersionCriterion[]}
            onChange={(versionOrder) => update({ versionOrder })}
            error={error('version-order')}
          />
        </SettingRow>
        <SettingRow anchor="version-codec-order">
          <CodecOrder
            order={form.versionCodecOrder}
            onChange={(versionCodecOrder) => update({ versionCodecOrder })}
            error={error('version-codec-order')}
          />
        </SettingRow>
        <SettingRow anchor="version-source-order">
          <SourceOrder
            order={form.versionSourceOrder}
            sources={form.versionSources}
            onChange={(versionSourceOrder) => update({ versionSourceOrder })}
            error={error('version-source-order')}
          />
        </SettingRow>
        <FieldRow
          anchor="hide-versions-larger"
          label={s.hideVersionsLarger}
          help={s.hideVersionsLargerHelp(range('hideVersionsLargerGb'))}
          error={error('hide-versions-larger')}
        >
          <NumberInput
            {...limits('hideVersionsLargerGb')}
            step={1}
            {...number('hide-versions-larger', form.hideVersionsLargerGb, (value) =>
              update({ hideVersionsLargerGb: Math.trunc(value) }),
            )}
          />
        </FieldRow>
        <SettingRow anchor="hidden-version-codecs">
          <fieldset aria-describedby="hidden-version-codecs-help">
            <legend className="text-[15px] font-medium tracking-[-0.01em] text-ink">
              {s.hiddenVersionCodecs}
            </legend>
            <p id="hidden-version-codecs-help" className="mt-1 max-w-[60ch] text-small text-ink-3">
              {s.hiddenVersionCodecsHelp}
            </p>
            <div className="mt-3.5 grid grid-cols-2 gap-3 sm:grid-cols-4">
              {((form.bounds.hiddenVersionCodecs?.choices ?? []) as VersionCodec[]).map(
                (codec, _, all) => (
                  <Checkbox
                    key={codec}
                    label={s.versionCodecs[codec]}
                    checked={form.hiddenVersionCodecs.includes(codec)}
                    onChange={(checked) =>
                      update({
                        hiddenVersionCodecs: all.filter((other) =>
                          other === codec ? checked : form.hiddenVersionCodecs.includes(other),
                        ),
                      })
                    }
                  />
                ),
              )}
            </div>
            {error('hidden-version-codecs') && (
              <div className="mt-2">
                <FieldError>{error('hidden-version-codecs')}</FieldError>
              </div>
            )}
          </fieldset>
        </SettingRow>
        <FieldRow
          anchor="hidden-version-words"
          label={s.hiddenVersionWords}
          help={s.hiddenVersionWordsHelp(range('hiddenVersionWords'))}
          error={error('hidden-version-words')}
        >
          <WordList
            value={form.hiddenVersionWords}
            onValue={(hiddenVersionWords) => update({ hiddenVersionWords })}
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

/**
 * The words hiding versions, one a line. The text stays as typed while it lists the same words,
 * so that a new line can be started; discarding or saving writes it anew.
 */
function WordList({ value, onValue }: { value: string[]; onValue: (value: string[]) => void }) {
  const [text, setText] = useState(() => value.join('\n'))
  const words = (typed: string) =>
    typed
      .split('\n')
      .map((word) => word.trim())
      .filter(Boolean)
  if (words(text).join('\n') !== value.join('\n')) {
    setText(value.join('\n'))
  }
  return (
    <Textarea
      rows={3}
      value={text}
      autoComplete="off"
      spellCheck={false}
      className="max-w-md"
      onValue={(next) => {
        setText(next)
        onValue(words(next))
      }}
    />
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
        <SwitchRow
          anchor="audio-intros"
          label={s.audioIntros}
          help={s.audioIntrosHelp}
          checked={form.audioIntros}
          onChange={(audioIntros) => update({ audioIntros })}
          disabled={!form.skipButtons}
        />
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
      <SettingsGroup title={groups.images}>
        <SwitchRow
          anchor="generate-images"
          label={s.generateImages}
          help={s.generateImagesHelp}
          checked={form.generateImages}
          onChange={(generateImages) => update({ generateImages })}
        />
        <SettingRow
          anchor="tmdb-key"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.tmdbKey}
            help={s.tmdbKeyHelp}
            saved={form.tmdbKeySet}
            value={form.tmdbKey}
            onValue={(tmdbKey) => update({ tmdbKey })}
            error={error('tmdb-key')}
            reveal={() => revealServerSecret('tmdbKey')}
          />
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title={groups.ratings}>
        <SettingRow
          anchor="mdblist-ratings-key"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.mdblistRatingsKey}
            help={s.mdblistRatingsKeyHelp}
            saved={form.mdblistRatingsKeySet}
            value={form.mdblistRatingsKey}
            onValue={(mdblistRatingsKey) => update({ mdblistRatingsKey })}
            error={error('mdblist-ratings-key')}
            reveal={() => revealServerSecret('mdblistRatingsKey')}
          />
        </SettingRow>
        <FieldRow
          anchor="critic-rating-shows"
          label={s.criticRatingShows}
          help={s.criticRatingShowsHelp}
          error={error('critic-rating-shows')}
        >
          <Select
            value={form.criticRatingShows}
            options={(['critics', 'audience'] as const).map((value) => ({
              value,
              label: s.criticRatingChoices[value],
            }))}
            onValue={(criticRatingShows) => update({ criticRatingShows })}
            className="max-w-xs"
          />
        </FieldRow>
      </SettingsGroup>
      <SettingsGroup title={groups.music}>
        <SwitchRow
          anchor="lyrics"
          label={s.lyrics}
          help={s.lyricsHelp}
          checked={form.lyrics}
          onChange={(lyrics) => update({ lyrics })}
        />
        <SwitchRow
          anchor="even-volume"
          label={s.evenVolume}
          help={s.evenVolumeHelp}
          checked={form.evenVolume}
          onChange={(evenVolume) => update({ evenVolume })}
        />
        <FieldRow
          anchor="measure-loudness"
          label={s.measureLoudness}
          help={s.measureLoudnessHelp}
          error={error('measure-loudness')}
        >
          <Select
            value={form.measureLoudness}
            options={(['off', 'local', 'all'] as const).map((value) => ({
              value,
              label: s.measureLoudnessChoices[value],
            }))}
            onValue={(measureLoudness) => update({ measureLoudness })}
            disabled={!form.evenVolume}
            className="max-w-xs"
          />
        </FieldRow>
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
  const host = computer(useServerSystem())
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
      <CollectionLibraryApps
        chosen={form.collectionLibraryApps}
        onChange={(collectionLibraryApps) => update({ collectionLibraryApps })}
        error={error('collection-library-apps')}
      />
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
        help={`${s.watchLocalFoldersHelp[host]} ${s.watchSharesHelp}`}
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
        anchor="import-every"
        label={s.importEveryMinutes}
        help={s.importEveryMinutesHelp}
        error={error('import-every')}
      >
        <Select
          value={form.importEveryMinutes}
          options={((form.bounds.importEveryMinutes?.choices ?? []) as number[]).map((minutes) => ({
            value: minutes,
            label: s.importEveryChoice(minutes),
          }))}
          onValue={(importEveryMinutes) => update({ importEveryMinutes })}
          className="max-w-xs"
        />
      </FieldRow>
      <SwitchRow
        anchor="share-links"
        label={s.shareLinks}
        help={s.shareLinksHelp}
        checked={form.shareLinks}
        onChange={(shareLinks) => update({ shareLinks })}
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
      <SwitchRow
        anchor="two-step-administrators"
        label={s.twoStepAdministrators}
        help={s.twoStepAdministratorsHelp}
        checked={form.twoStepAdministrators}
        onChange={(twoStepAdministrators) => update({ twoStepAdministrators })}
      />
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
      <FieldRow
        anchor="known-proxies"
        label={s.knownProxies}
        help={s.knownProxiesHelp(range('knownProxies'))}
        error={error('known-proxies')}
      >
        <ProxyList value={form.knownProxies} onValue={(knownProxies) => update({ knownProxies })} />
      </FieldRow>
    </SettingsGroup>
  )
}

/**
 * The addresses Let's Encrypt serves HTTPS at, with the first host name: without a port, as the
 * internet reaches it once the router forwards 443 to the HTTPS port, then with the HTTPS port.
 */
function letsEncryptAddresses(form: Settings): string[] {
  const host = form.acmeHosts[0]
  if (form.httpsMode !== 'letsencrypt' || host === undefined) return []
  return form.httpsPort === 443
    ? [`https://${host}`]
    : [`https://${host}`, `https://${host}:${form.httpsPort}`]
}

/** Settings › Security › HTTPS: how Polyfin serves HTTPS itself, next to HTTP. */
function Https({ form, update, error, number, range, limits }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings.https
  const modes: HttpsMode[] = ['off', 'files', 'letsencrypt']
  const challenges: AcmeChallenge[] = ['http-01', 'tls-alpn-01']
  return (
    <SettingsGroup title={s.title}>
      <FieldRow
        anchor="https"
        label={s.mode}
        help={`${s.intro} ${s.modeHelp}`}
        error={error('https')}
      >
        <Select
          value={form.httpsMode}
          options={modes.map((mode) => ({ value: mode, label: s.modes[mode] }))}
          onValue={(httpsMode: HttpsMode) => update({ httpsMode })}
          className="max-w-xs"
        />
      </FieldRow>
      {form.httpsMode !== 'off' && (
        <>
          <FieldRow
            anchor="https-port"
            label={s.port}
            help={s.portHelp(range('httpsPort'))}
            error={error('https-port')}
          >
            <NumberInput
              {...limits('httpsPort')}
              step={1}
              {...number('https-port', form.httpsPort, (value) =>
                update({ httpsPort: Math.trunc(value) }),
              )}
            />
          </FieldRow>
          <SwitchRow
            anchor="https-redirect"
            label={s.redirect}
            help={s.redirectHelp}
            checked={form.httpsRedirect}
            onChange={(httpsRedirect) => update({ httpsRedirect })}
          />
        </>
      )}
      {form.httpsMode === 'files' && (
        <>
          <FieldRow
            anchor="https-certificate"
            label={s.certificate}
            help={s.certificateHelp}
            error={error('https-certificate')}
          >
            <TextInput
              value={form.httpsCertificate}
              onValue={(httpsCertificate) => update({ httpsCertificate })}
              placeholder="/certs/fullchain.pem"
              maxLength={limits('httpsCertificate').max}
              autoComplete="off"
              spellCheck={false}
              mono
              className="max-w-md"
            />
          </FieldRow>
          <FieldRow
            anchor="https-private-key"
            label={s.privateKey}
            help={s.privateKeyHelp}
            error={error('https-private-key')}
          >
            <TextInput
              value={form.httpsPrivateKey}
              onValue={(httpsPrivateKey) => update({ httpsPrivateKey })}
              placeholder="/certs/privkey.pem"
              maxLength={limits('httpsPrivateKey').max}
              autoComplete="off"
              spellCheck={false}
              mono
              className="max-w-md"
            />
          </FieldRow>
        </>
      )}
      {form.httpsMode === 'letsencrypt' && (
        <>
          <FieldRow
            anchor="acme-hosts"
            label={s.hosts}
            help={s.hostsHelp(range('acmeHosts'))}
            error={error('acme-hosts')}
          >
            <ProxyList
              value={form.acmeHosts}
              onValue={(acmeHosts) => update({ acmeHosts })}
              placeholder="media.example.org"
            />
          </FieldRow>
          <FieldRow
            anchor="acme-email"
            label={s.email}
            help={s.emailHelp}
            error={error('acme-email')}
          >
            <TextInput
              type="email"
              value={form.acmeEmail}
              onValue={(acmeEmail) => update({ acmeEmail })}
              placeholder="admin@example.org"
              maxLength={limits('acmeEmail').max}
              autoComplete="off"
              spellCheck={false}
              className="max-w-md"
            />
          </FieldRow>
          <SwitchRow
            anchor="acme-terms"
            label={s.terms}
            help={s.termsHelp}
            checked={form.acmeTerms}
            onChange={(acmeTerms) => update({ acmeTerms })}
          >
            {error('acme-terms') && <FieldError>{error('acme-terms')}</FieldError>}
          </SwitchRow>
          <FieldRow
            anchor="acme-challenge"
            label={s.challenge}
            help={s.challengeHelp}
            error={error('acme-challenge')}
          >
            <Select
              value={form.acmeChallenge}
              options={challenges.map((challenge) => ({
                value: challenge,
                label: s.challenges[challenge],
              }))}
              onValue={(acmeChallenge: AcmeChallenge) => update({ acmeChallenge })}
              className="max-w-sm"
            />
          </FieldRow>
        </>
      )}
    </SettingsGroup>
  )
}

/** What separates the entries of a list of addresses: line breaks, spaces or commas. */
const proxySeparators = /[\s,]+/

/**
 * Known proxies, one address, network or host name a line. The text stays as typed while it lists
 * the same entries, so that a new line can be started; discarding or saving writes it anew.
 */
function ProxyList({
  value,
  onValue,
  placeholder = '172.18.0.0/16',
}: {
  value: string[]
  onValue: (value: string[]) => void
  placeholder?: string
}) {
  const [text, setText] = useState(() => value.join('\n'))
  if (text.split(proxySeparators).filter(Boolean).join('\n') !== value.join('\n')) {
    setText(value.join('\n'))
  }
  return (
    <Textarea
      mono
      rows={3}
      value={text}
      placeholder={placeholder}
      autoComplete="off"
      spellCheck={false}
      onValue={(next) => {
        setText(next)
        onValue(next.split(proxySeparators).filter(Boolean))
      }}
    />
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
      <SettingsGroup title="AniList">
        <TextRow anchor="anilist-setup">
          <p className="max-w-[60ch]">{s.aniListSetup}</p>
          <Address label={s.redirectUri} value={aniListPinUri} />
        </TextRow>
        <FieldRow
          anchor="anilist-client-id"
          label={s.aniListClientId}
          help={s.aniListClientIdHelp}
          error={error('anilist-client-id')}
        >
          <TextInput
            value={form.aniListClientId}
            onValue={(aniListClientId) => update({ aniListClientId })}
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </FieldRow>
        <SettingRow
          anchor="anilist-client-secret"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.aniListClientSecret}
            help={s.aniListClientSecretHelp}
            saved={form.aniListClientSecretSet}
            value={form.aniListClientSecret}
            onValue={(aniListClientSecret) => update({ aniListClientSecret })}
            reveal={() => revealServerSecret('aniListClientSecret')}
          />
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title="MyAnimeList">
        <TextRow anchor="myanimelist-setup">
          <p className="max-w-[60ch]">{s.myAnimeListSetup}</p>
          <Address label={s.redirectUri} value={myAnimeListRedirect(form.publicAddress)} />
          <p className="mt-1 text-small text-ink-3">{s.myAnimeListRedirectHelp}</p>
        </TextRow>
        <FieldRow
          anchor="myanimelist-client-id"
          label={s.myAnimeListClientId}
          help={s.myAnimeListClientIdHelp}
          error={error('myanimelist-client-id')}
        >
          <TextInput
            value={form.myAnimeListClientId}
            onValue={(myAnimeListClientId) => update({ myAnimeListClientId })}
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </FieldRow>
        <SettingRow
          anchor="myanimelist-client-secret"
          className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
        >
          <SecretField
            label={s.myAnimeListClientSecret}
            help={s.myAnimeListClientSecretHelp}
            saved={form.myAnimeListClientSecretSet}
            value={form.myAnimeListClientSecret}
            onValue={(myAnimeListClientSecret) => update({ myAnimeListClientSecret })}
            reveal={() => revealServerSecret('myAnimeListClientSecret')}
          />
        </SettingRow>
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
  const { language, t } = useI18n()
  const s = t.settings
  const host = computer(useServerSystem())
  const untilId = useId()
  const hours = Array.from({ length: 24 }, (_, hour) => ({
    value: hour,
    label: formatHour(hour, language),
  }))
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
          help={s.recordingsFolderHelp[host]}
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
        <SwitchRow
          anchor="recording-commercials"
          label={s.recordingCommercials}
          help={s.recordingCommercialsHelp}
          checked={form.recordingCommercials}
          onChange={(recordingCommercials) => update({ recordingCommercials })}
        />
      </SettingsGroup>
      <SettingsGroup>
        <SwitchRow
          anchor="convert-recordings"
          label={s.convertRecordings}
          help={s.convertRecordingsHelp}
          checked={form.convertRecordings}
          onChange={(convertRecordings) => update({ convertRecordings })}
        />
        <FieldRow
          anchor="convert-recordings-between"
          label={s.convertRecordingsBetween}
          help={s.convertRecordingsBetweenHelp(
            formatHour(Number(form.bounds.convertRecordingsFrom?.default), language),
            formatHour(Number(form.bounds.convertRecordingsUntil?.default), language),
          )}
          error={error('convert-recordings-between')}
        >
          <div className="flex flex-wrap items-center gap-3">
            <Select
              value={form.convertRecordingsFrom}
              options={hours}
              onValue={(convertRecordingsFrom) => update({ convertRecordingsFrom })}
              disabled={!form.convertRecordings}
              aria-label={s.convertRecordingsFrom}
              className="max-w-[10rem]"
            />
            <span className="text-small text-ink-2">{s.convertRecordingsAnd}</span>
            <Select
              id={untilId}
              value={form.convertRecordingsUntil}
              options={hours}
              onValue={(convertRecordingsUntil) => update({ convertRecordingsUntil })}
              disabled={!form.convertRecordings}
              aria-label={s.convertRecordingsUntil}
              className="max-w-[10rem]"
            />
          </div>
        </FieldRow>
        <SwitchRow
          anchor="convert-recordings-on-processor"
          label={s.convertRecordingsOnProcessor}
          help={s.convertRecordingsOnProcessorHelp}
          checked={form.convertRecordingsOnProcessor}
          onChange={(convertRecordingsOnProcessor) => update({ convertRecordingsOnProcessor })}
          disabled={!form.convertRecordings}
        />
      </SettingsGroup>
    </>
  )
}

function Backups({ form, update, error, number, range, limits }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  const host = computer(useServerSystem())
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
          help={s.backupFolderHelp[host]}
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
      <SettingsGroup title={s.backupDestinations}>
        <SettingRow anchor="backup-destinations">
          <p className="mb-4 max-w-[60ch] text-small text-ink-3">{s.backupDestinationsHelp}</p>
          <BackupDestinations />
        </SettingRow>
      </SettingsGroup>
    </>
  )
}

function WebPlayer({ form, update, error, limits, dirty, adopt }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  const custom = form.webTheme === 'custom'
  const shown = shownTexts(form)
  return (
    <>
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
        <FieldRow
          anchor="web-theme"
          label={s.webTheme}
          help={s.webThemeHelp}
          error={error('web-theme')}
          aside={form.webTheme === 'lumaaglaass' && <EditAsCustom form={form} update={update} />}
        >
          <Select
            value={form.webTheme}
            options={webThemes.map((value) => ({ value, label: s.webThemeChoices[value] }))}
            onValue={(webTheme: WebTheme) => update({ webTheme })}
            className="max-w-xs"
          />
          <p className="mt-2 max-w-[60ch] text-small text-ink-3">{s.webThemeReload}</p>
        </FieldRow>
        <SettingRow anchor="custom-css">
          <CodeEditor
            label={s.customCss}
            help={form.webTheme === 'custom' ? s.customCssHelp : s.webThemeTexts[form.webTheme]}
            value={shown.css}
            onValue={(customCss) => update({ customCss })}
            maxBytes={limits('customCss').max ?? 0}
            readOnly={!custom}
            error={error('custom-css')}
          />
        </SettingRow>
        <SettingRow anchor="custom-js">
          <CodeEditor
            label={s.customJs}
            help={form.webTheme === 'custom' ? s.customJsHelp : s.webThemeTexts[form.webTheme]}
            value={shown.js}
            onValue={(customJs) => update({ customJs })}
            maxBytes={limits('customJs').max ?? 0}
            readOnly={!custom}
            error={error('custom-js')}
          >
            {custom && (
              <Notice tone="warn">
                <strong className="font-medium">{s.customJsWarningTitle}.</strong>{' '}
                {s.customJsWarning}
              </Notice>
            )}
          </CodeEditor>
        </SettingRow>
        {custom && (
          <SettingRow anchor="theme-copy">
            <ThemeCopyPanel dirty={dirty} adopt={adopt} />
          </SettingRow>
        )}
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
        <SwitchRow
          anchor="add-versions-to-open-page"
          label={s.addVersionsToOpenPage}
          help={s.addVersionsToOpenPageHelp}
          checked={form.addVersionsToOpenPage}
          onChange={(addVersionsToOpenPage) => update({ addVersionsToOpenPage })}
        />
      </SettingsGroup>
      <SettingsGroup title={s.homeRows}>
        <SettingRow anchor="home-rows">
          <p className="mb-4 max-w-[60ch] text-small text-ink-3">{s.homeRowsHelp}</p>
          <HomeRows />
        </SettingRow>
      </SettingsGroup>
    </>
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
  // With Let's Encrypt, the addresses it serves HTTPS at are offered while none is set.
  const offered = letsEncryptAddresses(form)
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
            placeholder={offered[0] ?? 'https://media.example.org'}
            maxLength={limits('publicAddress').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-md"
          />
          {offered.length > 0 && form.publicAddress === '' && (
            <div className="mt-3 flex flex-wrap items-center gap-3">
              {offered.map((address) => (
                <Button key={address} size="sm" onClick={() => update({ publicAddress: address })}>
                  {s.https.useAddress(address)}
                </Button>
              ))}
              <span className="text-small text-ink-3">{s.https.offeredAddress}</span>
            </div>
          )}
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
