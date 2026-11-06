import { ArrowSquareOutIcon, PlayIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import {
  fetchServerActions,
  fetchStatus,
  queryKeys,
  revealServerSecret,
  type AcmeChallenge,
  type HttpsMode,
  type ReleaseKind,
  type Settings,
  type TrackLanguage,
  type VersionCodec,
  type VersionCriterion,
  type WebTheme,
  webThemes,
  posterBadgeSizes,
} from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { formatHour } from '@/format'
import { languages, useI18n, type Language } from '@/i18n'
import { jellyfinApp, webPlayerLink } from '@/jellyfinApps'
import {
  Button,
  Checkbox,
  ExternalButtonLink,
  FieldError,
  Notice,
  SettingRow,
  Textarea,
} from '@/ui'
import NotificationTargets from '@/features/notifications/NotificationTargets'
import SummaryPreview from '@/features/notifications/SummaryPreview'
import { computer, useServerSystem } from '@/features/sources/LocalFolders'
import CodeEditor from './CodeEditor'
import CollectionLibraryApps from './CollectionLibraryApps'
import BackupDestinations from './BackupDestinations'
import HomeRows from './HomeRows'
import PosterPreviewPanel from './PosterPreview'
import ConversionSection from './ConversionSection'
import { LastBackup, Variables } from './Diagnostics'
import DlnaSection from './DlnaSection'
import TorrentsSection from './TorrentsSection'
import MetricsToken from './MetricsToken'
import { FieldRow, ListRows, PlainRow, SettingsGroup, TextRow } from './parts'
import type { SectionFormApi } from './SectionForm'
import SegmentSources from './SegmentSources'
import LabelRules from './LabelRules'
import SourceCache from './SourceCache'
import VersionOrder, { CodecOrder, EditionOrder, SourceOrder } from './VersionOrder'
import { EditAsCustom, shownTexts, ThemeCopyPanel } from './WebTheme'
import WorkFolderRow from './WorkFolders'
import WordList from './WordList'

/**
 * The redirect URI of a new Trakt app: Development apps take only localhost addresses, and the
 * server names this one when it refreshes the tokens of an app without a client secret.
 */
const traktRedirectUri = 'http://localhost'

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
      <IdentityProvider {...api} />
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

function General({ form, update, error, range, limits, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  // Polyfin Desktop starts Polyfin again itself: nothing to confirm.
  const desktop =
    useQuery({
      queryKey: queryKeys.serverActions,
      queryFn: ({ signal }) => fetchServerActions(signal),
    }).data?.restart === 'desktop'
  return (
    <SettingsGroup>
      <SettingRow
        kind="text"
        anchor="server-name"
        label={s.serverName}
        help={s.serverNameHelp(range('serverName'))}
        error={error('server-name')}
        value={form.serverName}
        onChange={(serverName) => update({ serverName })}
        maxLength={limits('serverName').max}
        saved={saved.serverName}
      />
      <SettingRow
        kind="choice"
        anchor="language"
        label={s.language}
        help={s.languageHelp}
        error={error('language')}
        value={form.language}
        options={languages.map((code) => ({ value: code, label: t.language[code].name }))}
        onChange={(language: Language) => update({ language })}
        saved={saved.language}
      />
      <SettingRow
        kind="switch"
        anchor="quick-connect"
        label={s.quickConnect}
        help={s.quickConnectHelp}
        checked={form.quickConnectEnabled}
        onChange={(quickConnectEnabled) => update({ quickConnectEnabled })}
        saved={saved.quickConnectEnabled}
      />
      <SettingRow
        kind="switch"
        anchor="legacy-authorization"
        label={s.legacyAuthorization}
        help={s.legacyAuthorizationHelp}
        checked={form.legacyAuthorization}
        onChange={(legacyAuthorization) => update({ legacyAuthorization })}
        saved={saved.legacyAuthorization}
      >
        <Notice tone="warn">
          <strong className="font-medium">{s.legacyWarningTitle}.</strong> {s.legacyWarning}
        </Notice>
      </SettingRow>
      <SettingRow
        kind="switch"
        anchor="silo-apps"
        label={s.siloApps}
        help={s.siloAppsHelp}
        checked={form.siloApps}
        onChange={(siloApps) => update({ siloApps })}
        saved={saved.siloApps}
      />
      <SettingRow
        kind="switch"
        anchor="update-check"
        label={s.updateCheck}
        help={s.updateCheckHelp}
        checked={form.updateCheck}
        onChange={(updateCheck) => update({ updateCheck })}
        saved={saved.updateCheck}
      />
      <SettingRow
        kind="switch"
        anchor="include-prereleases"
        label={s.includePrereleases}
        help={s.includePrereleasesHelp}
        checked={form.includePrereleases}
        disabled={!form.updateCheck}
        onChange={(includePrereleases) => update({ includePrereleases })}
        saved={saved.includePrereleases}
      />
      <SettingRow
        kind="switch"
        anchor="local-discovery"
        label={s.localDiscovery}
        help={s.localDiscoveryHelp}
        checked={form.localDiscovery}
        onChange={(localDiscovery) => update({ localDiscovery })}
        saved={saved.localDiscovery}
      />
      <SettingRow
        kind="text"
        anchor="local-address"
        label={s.localAddress}
        help={s.localAddressHelp}
        error={error('local-address')}
        value={form.localAddress}
        onChange={(localAddress) => update({ localAddress })}
        placeholder="http://192.168.1.10:8096"
        maxLength={limits('localAddress').max}
        mono
        saved={saved.localAddress}
      />
      <SettingRow
        kind="switch"
        anchor="metrics"
        label={s.metrics}
        help={s.metricsHelp}
        checked={form.metrics}
        onChange={(metrics) => update({ metrics })}
        saved={saved.metrics}
      />
      {form.metrics && (
        <PlainRow anchor="metrics-token">
          <MetricsToken saved={form.metricsTokenSet} />
        </PlainRow>
      )}
      {!desktop && (
        <SettingRow
          kind="switch"
          anchor="restart-confirmed"
          label={s.restartConfirmed}
          help={s.restartConfirmedHelp}
          checked={form.restartConfirmed}
          onChange={(restartConfirmed) => update({ restartConfirmed })}
          saved={saved.restartConfirmed}
        />
      )}
      <SettingRow
        kind="text"
        anchor="update-tool-url"
        label={s.updateToolUrl}
        help={s.updateToolUrlHelp}
        error={error('update-tool-url')}
        value={form.updateToolUrl}
        onChange={(updateToolUrl) => update({ updateToolUrl })}
        placeholder="http://updater:8080/v1/update"
        maxLength={limits('updateToolUrl').max}
        mono
        saved={saved.updateToolUrl}
      />
      <SettingRow
        kind="secret"
        anchor="update-tool-token"
        label={s.updateToolToken}
        help={s.updateToolTokenHelp}
        saved={form.updateToolTokenSet}
        value={form.updateToolToken}
        onValue={(updateToolToken) => update({ updateToolToken })}
        reveal={() => revealServerSecret('updateToolToken')}
        error={error('update-tool-token')}
      />
      <WorkFolderRow
        folder="cacheFolder"
        form={form}
        saved={saved}
        update={update}
        error={error}
        limits={limits}
      />
    </SettingsGroup>
  )
}

function Playback({ form, update, error, number, range, limits, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SettingRow
          kind="switch"
          anchor="prepare-ahead"
          label={s.prepareAhead}
          help={s.prepareAheadHelp}
          checked={form.prepareAhead}
          onChange={(prepareAhead) => update({ prepareAhead })}
          saved={saved.prepareAhead}
        />
        <SettingRow
          kind="switch"
          anchor="prefer-direct-play"
          label={s.preferDirectPlay}
          help={s.preferDirectPlayHelp}
          checked={form.preferDirectPlay}
          onChange={(preferDirectPlay) => update({ preferDirectPlay })}
          saved={saved.preferDirectPlay}
        />
        <SettingRow
          kind="switch"
          anchor="name-how-versions-play"
          label={s.nameHowVersionsPlay}
          help={s.nameHowVersionsPlayHelp}
          checked={form.nameHowVersionsPlay}
          onChange={(nameHowVersionsPlay) => update({ nameHowVersionsPlay })}
          saved={saved.nameHowVersionsPlay}
        />
        <SettingRow
          kind="switch"
          anchor="sync-addon-subtitles"
          label={s.syncAddonSubtitles}
          help={s.syncAddonSubtitlesHelp}
          checked={form.syncAddonSubtitles}
          onChange={(syncAddonSubtitles) => update({ syncAddonSubtitles })}
          saved={saved.syncAddonSubtitles}
        />
        <SettingRow
          kind="number"
          anchor="analysis-timeout"
          label={s.analysisTimeout}
          help={s.analysisTimeoutHelp(range('analysisTimeout'))}
          error={error('analysis-timeout')}
          {...minMax('analysisTimeout')}
          step={1}
          {...number('analysis-timeout', form.analysisTimeout, (value) =>
            update({ analysisTimeout: Math.trunc(value) }),
          )}
          saved={saved.analysisTimeout}
        />
        <SettingRow
          kind="number"
          anchor="version-attempts"
          label={s.versionAttempts}
          help={s.versionAttemptsHelp(range('versionAttempts'))}
          error={error('version-attempts')}
          {...minMax('versionAttempts')}
          step={1}
          {...number('version-attempts', form.versionAttempts, (value) =>
            update({ versionAttempts: Math.trunc(value) }),
          )}
          saved={saved.versionAttempts}
        />
        <SettingRow
          kind="switch"
          anchor="remuxdb"
          label={s.remuxDb}
          help={s.remuxDbHelp}
          checked={form.remuxDb}
          onChange={(remuxDb) => update({ remuxDb })}
          saved={saved.remuxDb}
        />
        <SettingRow
          kind="text"
          anchor="remuxdb-url"
          label={s.remuxDbUrl}
          help={s.remuxDbUrlHelp}
          error={error('remuxdb-url')}
          inputMode="url"
          value={form.remuxDbUrl}
          onChange={(remuxDbUrl) => update({ remuxDbUrl })}
          disabled={!form.remuxDb}
          maxLength={limits('remuxDbUrl').max}
          mono
          saved={saved.remuxDbUrl}
        />
      </SettingsGroup>
      <SourceCache
        form={form}
        saved={saved}
        minMax={minMax}
        update={update}
        error={error}
        number={number}
        range={range}
        limits={limits}
      />
      <SettingsGroup title={s.versionOrderGroup}>
        <PlainRow anchor="version-order">
          <VersionOrder
            order={form.versionOrder}
            choices={(form.bounds.versionOrder?.choices ?? []) as VersionCriterion[]}
            onChange={(versionOrder) => update({ versionOrder })}
            error={error('version-order')}
          />
        </PlainRow>
        <PlainRow anchor="version-codec-order">
          <CodecOrder
            order={form.versionCodecOrder}
            onChange={(versionCodecOrder) => update({ versionCodecOrder })}
            error={error('version-codec-order')}
          />
        </PlainRow>
        <PlainRow anchor="version-source-order">
          <SourceOrder
            order={form.versionSourceOrder}
            sources={form.versionSources}
            onChange={(versionSourceOrder) => update({ versionSourceOrder })}
            error={error('version-source-order')}
          />
        </PlainRow>
        <PlainRow anchor="version-edition-order">
          <EditionOrder
            order={form.versionEditionOrder}
            onChange={(versionEditionOrder) => update({ versionEditionOrder })}
            error={error('version-edition-order')}
          />
        </PlainRow>
        <SettingRow
          kind="number"
          anchor="version-mb-per-minute"
          label={s.versionMbPerMinute}
          help={s.versionMbPerMinuteHelp(range('versionMbPerMinute'))}
          error={error('version-mb-per-minute')}
          {...minMax('versionMbPerMinute')}
          step={1}
          {...number('version-mb-per-minute', form.versionMbPerMinute, (value) =>
            update({ versionMbPerMinute: Math.trunc(value) }),
          )}
          saved={saved.versionMbPerMinute}
        />
        <SettingRow
          kind="number"
          anchor="hide-versions-larger"
          label={s.hideVersionsLarger}
          help={s.hideVersionsLargerHelp(range('hideVersionsLargerGb'))}
          error={error('hide-versions-larger')}
          {...minMax('hideVersionsLargerGb')}
          step={1}
          {...number('hide-versions-larger', form.hideVersionsLargerGb, (value) =>
            update({ hideVersionsLargerGb: Math.trunc(value) }),
          )}
          saved={saved.hideVersionsLargerGb}
        />
        <PlainRow anchor="hidden-version-codecs">
          <fieldset aria-describedby="hidden-version-codecs-help">
            <legend className="text-h3 text-ink max-md:text-[18px]">{s.hiddenVersionCodecs}</legend>
            <p id="hidden-version-codecs-help" className="mt-2 max-w-[66ch] text-small text-ink-2">
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
        </PlainRow>
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
        <PlainRow anchor="hidden-releases">
          <fieldset aria-describedby="hidden-releases-help">
            <legend className="text-h3 text-ink max-md:text-[18px]">{s.hiddenReleases}</legend>
            <p id="hidden-releases-help" className="mt-2 max-w-[66ch] text-small text-ink-2">
              {s.hiddenReleasesHelp}
            </p>
            <div className="mt-3.5 grid gap-3 sm:grid-cols-2">
              {((form.bounds.hiddenReleases?.choices ?? []) as ReleaseKind[]).map(
                (kind, _, all) => (
                  <Checkbox
                    key={kind}
                    label={s.releaseKinds[kind]}
                    checked={form.hiddenReleases.includes(kind)}
                    onChange={(checked) =>
                      update({
                        hiddenReleases: all.filter((other) =>
                          other === kind ? checked : form.hiddenReleases.includes(other),
                        ),
                      })
                    }
                  />
                ),
              )}
            </div>
            {error('hidden-releases') && (
              <div className="mt-2">
                <FieldError>{error('hidden-releases')}</FieldError>
              </div>
            )}
          </fieldset>
        </PlainRow>
        <PlainRow anchor="version-label-rules">
          <LabelRules
            rules={form.versionLabelRules}
            max={limits('versionLabelRules').max ?? 0}
            onChange={(versionLabelRules) => update({ versionLabelRules })}
            error={error('version-label-rules')}
          />
        </PlainRow>
      </SettingsGroup>
      <SettingsGroup title={s.playbackHistoryGroup}>
        <SettingRow
          kind="switch"
          anchor="playback-history"
          label={s.playbackHistory}
          help={s.playbackHistoryHelp}
          checked={form.playbackHistory}
          onChange={(playbackHistory) => update({ playbackHistory })}
          saved={saved.playbackHistory}
        />
        <SettingRow
          kind="number"
          anchor="playback-history-days"
          label={s.playbackHistoryDays}
          help={s.playbackHistoryDaysHelp(range('playbackHistoryDays'))}
          error={error('playback-history-days')}
          {...minMax('playbackHistoryDays')}
          step={1}
          {...number('playback-history-days', form.playbackHistoryDays, (value) =>
            update({ playbackHistoryDays: Math.trunc(value) }),
          )}
          saved={saved.playbackHistoryDays}
        />
      </SettingsGroup>
      <SettingsGroup title={s.defaultLanguagesGroup}>
        <SettingRow
          kind="choice"
          anchor="default-audio-language"
          label={s.defaultAudioLanguage}
          help={s.defaultAudioLanguageHelp}
          error={error('default-audio-language')}
          value={form.defaultAudioLanguage}
          options={trackLanguageOptions(form.trackLanguages, s.noDefaultLanguage)}
          onChange={(defaultAudioLanguage) => update({ defaultAudioLanguage })}
          saved={saved.defaultAudioLanguage}
        />
        <SettingRow
          kind="choice"
          anchor="default-subtitle-language"
          label={s.defaultSubtitleLanguage}
          help={s.defaultSubtitleLanguageHelp}
          error={error('default-subtitle-language')}
          value={form.defaultSubtitleLanguage}
          options={trackLanguageOptions(form.trackLanguages, s.noDefaultLanguage)}
          onChange={(defaultSubtitleLanguage) => update({ defaultSubtitleLanguage })}
          saved={saved.defaultSubtitleLanguage}
        />
      </SettingsGroup>
    </>
  )
}

/** The choices of a default track language: none, then the languages by English name. */
function trackLanguageOptions(languages: TrackLanguage[], none: string) {
  return [
    { value: '', label: none },
    ...languages.map((language) => ({ value: language.code, label: language.name })),
  ]
}

function Content({ form, update, error, number, range, limits, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const groups = t.settingsPage.groups
  return (
    <>
      <SettingsGroup title={groups.skip}>
        <SettingRow
          kind="switch"
          anchor="skip-buttons"
          label={s.skipButtons}
          help={s.skipButtonsHelp}
          checked={form.skipButtons}
          onChange={(skipButtons) => update({ skipButtons })}
          saved={saved.skipButtons}
        />
        <PlainRow anchor="segment-order">
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
        </PlainRow>
        <SettingRow
          kind="secret"
          anchor="publicmetadb-key"
          label={s.publicMetaDbKey}
          help={s.publicMetaDbKeyHelp}
          saved={form.publicMetaDbKeySet}
          value={form.publicMetaDbKey}
          onValue={(publicMetaDbKey) => update({ publicMetaDbKey })}
          error={error('publicmetadb-key')}
          reveal={() => revealServerSecret('publicMetaDbKey')}
        />
        <SettingRow
          kind="secret"
          anchor="theintrodb-key"
          label={s.theIntroDbKey}
          help={s.theIntroDbKeyHelp}
          saved={form.theIntroDbKeySet}
          value={form.theIntroDbKey}
          onValue={(theIntroDbKey) => update({ theIntroDbKey })}
          error={error('theintrodb-key')}
          reveal={() => revealServerSecret('theIntroDbKey')}
        />
        <SettingRow
          kind="switch"
          anchor="audio-intros"
          label={s.audioIntros}
          help={s.audioIntrosHelp}
          checked={form.audioIntros}
          onChange={(audioIntros) => update({ audioIntros })}
          disabled={!form.skipButtons}
          saved={saved.audioIntros}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.titlePages}>
        <SettingRow
          kind="switch"
          anchor="similar-titles"
          label={s.similarTitles}
          help={s.similarTitlesHelp}
          checked={form.similarTitles}
          onChange={(similarTitles) => update({ similarTitles })}
          saved={saved.similarTitles}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.images}>
        <SettingRow
          kind="switch"
          anchor="generate-images"
          label={s.generateImages}
          help={s.generateImagesHelp}
          checked={form.generateImages}
          onChange={(generateImages) => update({ generateImages })}
          saved={saved.generateImages}
        />
        <SettingRow
          kind="secret"
          anchor="tmdb-key"
          label={s.tmdbKey}
          help={s.tmdbKeyHelp}
          saved={form.tmdbKeySet}
          value={form.tmdbKey}
          onValue={(tmdbKey) => update({ tmdbKey })}
          error={error('tmdb-key')}
          reveal={() => revealServerSecret('tmdbKey')}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.releases}>
        <SettingRow
          kind="switch"
          anchor="wait-digital-release"
          label={s.waitDigitalRelease}
          help={s.waitDigitalReleaseHelp}
          checked={form.waitDigitalRelease}
          onChange={(waitDigitalRelease) => update({ waitDigitalRelease })}
          disabled={!form.tmdbKeySet && !form.tmdbKey}
          saved={saved.waitDigitalRelease}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.ratings}>
        <SettingRow
          kind="secret"
          anchor="mdblist-ratings-key"
          label={s.mdblistRatingsKey}
          help={s.mdblistRatingsKeyHelp}
          saved={form.mdblistRatingsKeySet}
          value={form.mdblistRatingsKey}
          onValue={(mdblistRatingsKey) => update({ mdblistRatingsKey })}
          error={error('mdblist-ratings-key')}
          reveal={() => revealServerSecret('mdblistRatingsKey')}
        />
        <SettingRow
          kind="choice"
          anchor="critic-rating-shows"
          label={s.criticRatingShows}
          help={s.criticRatingShowsHelp}
          error={error('critic-rating-shows')}
          value={form.criticRatingShows}
          options={(['critics', 'audience'] as const).map((value) => ({
            value,
            label: s.criticRatingChoices[value],
          }))}
          onChange={(criticRatingShows) => update({ criticRatingShows })}
          saved={saved.criticRatingShows}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.posters}>
        <SettingRow
          kind="switch"
          anchor="poster-badges"
          label={s.posterBadges}
          help={s.posterBadgesHelp}
          checked={form.posterBadges}
          onChange={(posterBadges) => update({ posterBadges })}
          saved={saved.posterBadges}
        />
        <SettingRow
          kind="choice"
          anchor="poster-badge-size"
          label={s.posterBadgeSize}
          help={s.posterBadgeSizeHelp}
          error={error('poster-badge-size')}
          value={form.posterBadgeSize}
          options={posterBadgeSizes.map((value) => ({
            value,
            label: s.posterBadgeSizeChoices[value],
          }))}
          onChange={(posterBadgeSize) => update({ posterBadgeSize })}
          saved={saved.posterBadgeSize}
        />
        <PlainRow anchor="poster-preview">
          <p className="text-h3 text-ink max-md:text-[18px]">{s.posterPreview.title}</p>
          <p className="mt-2 mb-5 max-w-[66ch] text-small text-ink-2">{s.posterPreview.help}</p>
          <PosterPreviewPanel size={form.posterBadgeSize} />
        </PlainRow>
        <SettingRow
          kind="text"
          anchor="poster-template"
          label={s.posterTemplate}
          help={s.posterTemplateHelp}
          error={error('poster-template')}
          value={form.posterTemplate}
          onChange={(posterTemplate) => update({ posterTemplate })}
          placeholder="https://posters.example/{type}/{imdb_id}.jpg"
          maxLength={limits('posterTemplate').max}
          mono
          saved={saved.posterTemplate}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.music}>
        <SettingRow
          kind="switch"
          anchor="lyrics"
          label={s.lyrics}
          help={s.lyricsHelp}
          checked={form.lyrics}
          onChange={(lyrics) => update({ lyrics })}
          saved={saved.lyrics}
        />
        <SettingRow
          kind="switch"
          anchor="even-volume"
          label={s.evenVolume}
          help={s.evenVolumeHelp}
          checked={form.evenVolume}
          onChange={(evenVolume) => update({ evenVolume })}
          saved={saved.evenVolume}
        />
        <SettingRow
          kind="choice"
          anchor="measure-loudness"
          label={s.measureLoudness}
          help={s.measureLoudnessHelp}
          error={error('measure-loudness')}
          value={form.measureLoudness}
          options={(['off', 'local', 'all'] as const).map((value) => ({
            value,
            label: s.measureLoudnessChoices[value],
          }))}
          onChange={(measureLoudness) => update({ measureLoudness })}
          disabled={!form.evenVolume}
          saved={saved.measureLoudness}
        />
      </SettingsGroup>
      <SettingsGroup title={groups.thresholds}>
        <div className="grid gap-x-8 sm:grid-cols-2">
          <SettingRow
            kind="number"
            anchor="played-percent"
            label={s.playedPercent}
            help={s.playedPercentHelp(range('playedPercent'))}
            error={error('played-percent')}
            {...minMax('playedPercent')}
            step={1}
            unit="%"
            {...number('played-percent', form.playedPercent, (value) =>
              update({ playedPercent: Math.trunc(value) }),
            )}
            saved={saved.playedPercent}
          />
          <SettingRow
            kind="number"
            anchor="resume-percent"
            label={s.resumePercent}
            help={s.resumePercentHelp(range('resumePercent'))}
            error={error('resume-percent')}
            {...minMax('resumePercent')}
            step={1}
            unit="%"
            // 0 is a valid threshold: it is shown, not left blank.
            {...number('resume-percent', form.resumePercent, (value) =>
              update({ resumePercent: Math.trunc(value) }),
            )}
            saved={saved.resumePercent}
          />
        </div>
      </SettingsGroup>
    </>
  )
}

function Catalogs({ form, update, error, number, range, saved, minMax }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  const host = computer(useServerSystem())
  return (
    <SettingsGroup>
      <SettingRow
        kind="number"
        anchor="catalog-limit"
        label={s.catalogLimit}
        help={s.catalogLimitHelp(range('catalogLimit'))}
        error={error('catalog-limit')}
        {...minMax('catalogLimit')}
        step={1}
        {...number('catalog-limit', form.catalogLimit, (value) =>
          update({ catalogLimit: Math.trunc(value) }),
        )}
        saved={saved.catalogLimit}
      />
      <SettingRow
        kind="number"
        anchor="channel-limit"
        label={s.channelLimit}
        help={s.channelLimitHelp(range('channelLimit'))}
        error={error('channel-limit')}
        {...minMax('channelLimit')}
        step={1}
        {...number('channel-limit', form.channelLimit, (value) =>
          update({ channelLimit: Math.trunc(value) }),
        )}
        saved={saved.channelLimit}
      />
      <SettingRow
        kind="number"
        anchor="version-list-minutes"
        label={s.versionListMinutes}
        help={s.versionListMinutesHelp(range('versionListMinutes'))}
        error={error('version-list-minutes')}
        {...minMax('versionListMinutes')}
        step={1}
        {...number('version-list-minutes', form.versionListMinutes, (value) =>
          update({ versionListMinutes: Math.trunc(value) }),
        )}
        saved={saved.versionListMinutes}
      />
      <SettingRow
        kind="number"
        anchor="catalog-refresh-minutes"
        label={s.catalogRefreshMinutes}
        help={s.catalogRefreshMinutesHelp(range('catalogRefreshMinutes'))}
        error={error('catalog-refresh-minutes')}
        {...minMax('catalogRefreshMinutes')}
        step={1}
        {...number('catalog-refresh-minutes', form.catalogRefreshMinutes, (value) =>
          update({ catalogRefreshMinutes: Math.trunc(value) }),
        )}
        saved={saved.catalogRefreshMinutes}
      />
      <SettingRow
        kind="choice"
        anchor="collection-read-hour"
        label={s.collectionReadHour}
        help={s.collectionReadHourHelp}
        error={error('collection-read-hour')}
        value={form.collectionReadHour}
        options={[
          { value: -1, label: s.collectionReadHourNever },
          ...Array.from({ length: 24 }, (_, hour) => ({
            value: hour,
            label: formatHour(hour, language),
          })),
        ]}
        onChange={(collectionReadHour) => update({ collectionReadHour })}
        saved={saved.collectionReadHour}
      />
      <CollectionLibraryApps
        chosen={form.collectionLibraryApps}
        onChange={(collectionLibraryApps) => update({ collectionLibraryApps })}
        error={error('collection-library-apps')}
      />
      <SettingRow
        kind="number"
        anchor="watchlist-check-hours"
        label={s.watchlistCheckHours}
        help={s.watchlistCheckHoursHelp(range('watchlistCheckHours'))}
        error={error('watchlist-check-hours')}
        {...minMax('watchlistCheckHours')}
        step={1}
        {...number('watchlist-check-hours', form.watchlistCheckHours, (value) =>
          update({ watchlistCheckHours: Math.trunc(value) }),
        )}
        saved={saved.watchlistCheckHours}
      />
      <SettingRow
        kind="number"
        anchor="local-scan-hours"
        label={s.localScanHours}
        help={s.localScanHoursHelp(range('localScanHours'))}
        error={error('local-scan-hours')}
        {...minMax('localScanHours')}
        step={1}
        {...number('local-scan-hours', form.localScanHours, (value) =>
          update({ localScanHours: Math.trunc(value) }),
        )}
        saved={saved.localScanHours}
      />
      <SettingRow
        kind="switch"
        anchor="watch-local-folders"
        label={s.watchLocalFolders}
        help={`${s.watchLocalFoldersHelp[host]} ${s.watchSharesHelp}`}
        checked={form.watchLocalFolders}
        onChange={(watchLocalFolders) => update({ watchLocalFolders })}
        saved={saved.watchLocalFolders}
      />
    </SettingsGroup>
  )
}

function Thumbnails({ form, update, error, number, range, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="trickplay"
        label={s.trickplay}
        help={s.trickplayHelp}
        checked={form.trickplay}
        onChange={(trickplay) => update({ trickplay })}
        saved={saved.trickplay}
      />
      <SettingRow
        kind="number"
        anchor="trickplay-interval"
        label={s.trickplayInterval}
        help={s.trickplayIntervalHelp(range('trickplayInterval'))}
        error={error('trickplay-interval')}
        {...minMax('trickplayInterval')}
        step={1}
        {...number('trickplay-interval', form.trickplayInterval, (value) =>
          update({ trickplayInterval: Math.trunc(value) }),
        )}
        saved={saved.trickplayInterval}
      />
      <SettingRow
        kind="choice"
        anchor="trickplay-width"
        label={s.trickplayWidth}
        help={s.trickplayWidthHelp}
        error={error('trickplay-width')}
        value={form.trickplayWidth}
        options={((form.bounds.trickplayWidth?.choices ?? []) as number[]).map((width) => ({
          value: width,
          label: s.pixels(width),
        }))}
        onChange={(trickplayWidth) => update({ trickplayWidth })}
        saved={saved.trickplayWidth}
      />
      <SettingRow
        kind="switch"
        anchor="chapter-images"
        label={s.chapterImages}
        help={s.chapterImagesHelp}
        checked={form.chapterImages}
        onChange={(chapterImages) => update({ chapterImages })}
        saved={saved.chapterImages}
      />
      <SettingRow
        kind="number"
        anchor="thumbnail-storage"
        label={s.thumbnailStorage}
        help={s.thumbnailStorageHelp(range('thumbnailStorageGB'))}
        error={error('thumbnail-storage')}
        {...minMax('thumbnailStorageGB')}
        step={1}
        {...number('thumbnail-storage', form.thumbnailStorageGB, (value) =>
          update({ thumbnailStorageGB: Math.trunc(value) }),
        )}
        saved={saved.thumbnailStorageGB}
      />
    </SettingsGroup>
  )
}

function Security({ form, update, error, number, range, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="personal-addons"
        label={s.personalAddons}
        help={s.personalAddonsHelp}
        checked={form.personalAddons}
        onChange={(personalAddons) => update({ personalAddons })}
        saved={saved.personalAddons}
      />
      <SettingRow
        kind="switch"
        anchor="server-imports"
        label={s.serverImports}
        help={s.serverImportsHelp}
        checked={form.serverImports}
        onChange={(serverImports) => update({ serverImports })}
        saved={saved.serverImports}
      />
      <SettingRow
        kind="choice"
        anchor="import-every"
        label={s.importEveryMinutes}
        help={s.importEveryMinutesHelp}
        error={error('import-every')}
        value={form.importEveryMinutes}
        options={((form.bounds.importEveryMinutes?.choices ?? []) as number[]).map((minutes) => ({
          value: minutes,
          label: s.importEveryChoice(minutes),
        }))}
        onChange={(importEveryMinutes) => update({ importEveryMinutes })}
        saved={saved.importEveryMinutes}
      />
      <SettingRow
        kind="switch"
        anchor="share-links"
        label={s.shareLinks}
        help={s.shareLinksHelp}
        checked={form.shareLinks}
        onChange={(shareLinks) => update({ shareLinks })}
        saved={saved.shareLinks}
      />
      <SettingRow
        kind="number"
        anchor="login-attempts"
        label={s.loginAttempts}
        help={s.loginAttemptsHelp(range('loginAttempts'))}
        error={error('login-attempts')}
        {...minMax('loginAttempts')}
        step={1}
        {...number('login-attempts', form.loginAttempts, (value) =>
          update({ loginAttempts: Math.trunc(value) }),
        )}
        saved={saved.loginAttempts}
      />
      <SettingRow
        kind="switch"
        anchor="two-step-administrators"
        label={s.twoStepAdministrators}
        help={s.twoStepAdministratorsHelp}
        checked={form.twoStepAdministrators}
        onChange={(twoStepAdministrators) => update({ twoStepAdministrators })}
        saved={saved.twoStepAdministrators}
      />
      <SettingRow
        kind="number"
        anchor="inactive-device-days"
        label={s.inactiveDeviceDays}
        help={s.inactiveDeviceDaysHelp(range('inactiveDeviceDays'))}
        error={error('inactive-device-days')}
        {...minMax('inactiveDeviceDays')}
        step={1}
        {...number('inactive-device-days', form.inactiveDeviceDays, (value) =>
          update({ inactiveDeviceDays: Math.trunc(value) }),
        )}
        saved={saved.inactiveDeviceDays}
      />
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
 * Settings › Security › Identity provider: an OpenID Connect provider people sign in with, in this
 * app and in the web player, and the redirect URI to register at it, built from the public address.
 */
function IdentityProvider({ form, update, error, limits, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings.provider
  return (
    <SettingsGroup title={s.title}>
      <TextRow anchor="provider-setup">
        <p className="max-w-[60ch]">{s.intro}</p>
        {form.providerRedirectUri === '' ? (
          <div className="mt-3 max-w-[60ch]">
            <Notice tone="warn">{s.noPublicAddress}</Notice>
          </div>
        ) : (
          <>
            <Address label={s.redirectUri} value={form.providerRedirectUri} />
            <p className="mt-1 text-small text-ink-3">{s.redirectUriHelp}</p>
          </>
        )}
      </TextRow>
      <SettingRow
        kind="text"
        anchor="provider-issuer"
        label={s.issuer}
        help={s.issuerHelp}
        error={error('provider-issuer')}
        inputMode="url"
        value={form.providerIssuer}
        onChange={(providerIssuer) => update({ providerIssuer })}
        placeholder="https://id.example.org"
        maxLength={limits('providerIssuer').max}
        mono
        saved={saved.providerIssuer}
      />
      <SettingRow
        kind="text"
        anchor="provider-name"
        label={s.name}
        help={s.nameHelp}
        value={form.providerName}
        onChange={(providerName) => update({ providerName })}
        maxLength={limits('providerName').max}
        saved={saved.providerName}
      />
      <SettingRow
        kind="text"
        anchor="provider-client-id"
        label={s.clientId}
        help={s.clientIdHelp}
        value={form.providerClientId}
        onChange={(providerClientId) => update({ providerClientId })}
        maxLength={limits('providerClientId').max}
        mono
        saved={saved.providerClientId}
      />
      <SettingRow
        kind="secret"
        anchor="provider-client-secret"
        label={s.clientSecret}
        help={s.clientSecretHelp}
        saved={form.providerClientSecretSet}
        value={form.providerClientSecret}
        onValue={(providerClientSecret) => update({ providerClientSecret })}
      />
      <SettingRow
        kind="text"
        anchor="provider-user-claim"
        label={s.userClaim}
        help={s.userClaimHelp}
        value={form.providerUserClaim}
        onChange={(providerUserClaim) => update({ providerUserClaim })}
        maxLength={limits('providerUserClaim').max}
        mono
        saved={saved.providerUserClaim}
      />
      <SettingRow
        kind="switch"
        anchor="provider-create-accounts"
        label={s.createAccounts}
        help={s.createAccountsHelp}
        checked={form.providerCreateAccounts}
        onChange={(providerCreateAccounts) => update({ providerCreateAccounts })}
        saved={saved.providerCreateAccounts}
      />
      <SettingRow
        kind="text"
        anchor="provider-admin-claim"
        label={s.adminClaim}
        help={s.adminClaimHelp}
        value={form.providerAdminClaim}
        onChange={(providerAdminClaim) => update({ providerAdminClaim })}
        placeholder="groups"
        maxLength={limits('providerAdminClaim').max}
        mono
        saved={saved.providerAdminClaim}
      />
      <SettingRow
        kind="text"
        anchor="provider-admin-value"
        label={s.adminValue}
        help={s.adminValueHelp}
        value={form.providerAdminValue}
        onChange={(providerAdminValue) => update({ providerAdminValue })}
        maxLength={limits('providerAdminValue').max}
        saved={saved.providerAdminValue}
      />
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
function Https({ form, update, error, number, range, limits, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings.https
  const modes: HttpsMode[] = ['off', 'files', 'letsencrypt']
  const challenges: AcmeChallenge[] = ['http-01', 'tls-alpn-01']
  return (
    <SettingsGroup title={s.title}>
      <SettingRow
        kind="choice"
        anchor="https"
        label={s.mode}
        help={`${s.intro} ${s.modeHelp}`}
        error={error('https')}
        value={form.httpsMode}
        options={modes.map((mode) => ({ value: mode, label: s.modes[mode] }))}
        onChange={(httpsMode: HttpsMode) => update({ httpsMode })}
        saved={saved.httpsMode}
      />
      {form.httpsMode !== 'off' && (
        <>
          <SettingRow
            kind="number"
            anchor="https-port"
            label={s.port}
            help={s.portHelp(range('httpsPort'))}
            error={error('https-port')}
            {...minMax('httpsPort')}
            step={1}
            {...number('https-port', form.httpsPort, (value) =>
              update({ httpsPort: Math.trunc(value) }),
            )}
            saved={saved.httpsPort}
          />
          <SettingRow
            kind="switch"
            anchor="https-redirect"
            label={s.redirect}
            help={s.redirectHelp}
            checked={form.httpsRedirect}
            onChange={(httpsRedirect) => update({ httpsRedirect })}
            saved={saved.httpsRedirect}
          />
        </>
      )}
      {form.httpsMode === 'files' && (
        <>
          <SettingRow
            kind="text"
            anchor="https-certificate"
            label={s.certificate}
            help={s.certificateHelp}
            error={error('https-certificate')}
            value={form.httpsCertificate}
            onChange={(httpsCertificate) => update({ httpsCertificate })}
            placeholder="/certs/fullchain.pem"
            maxLength={limits('httpsCertificate').max}
            mono
            saved={saved.httpsCertificate}
          />
          <SettingRow
            kind="text"
            anchor="https-private-key"
            label={s.privateKey}
            help={s.privateKeyHelp}
            error={error('https-private-key')}
            value={form.httpsPrivateKey}
            onChange={(httpsPrivateKey) => update({ httpsPrivateKey })}
            placeholder="/certs/privkey.pem"
            maxLength={limits('httpsPrivateKey').max}
            mono
            saved={saved.httpsPrivateKey}
          />
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
          <SettingRow
            kind="text"
            anchor="acme-email"
            label={s.email}
            help={s.emailHelp}
            error={error('acme-email')}
            inputMode="email"
            value={form.acmeEmail}
            onChange={(acmeEmail) => update({ acmeEmail })}
            placeholder="admin@example.org"
            maxLength={limits('acmeEmail').max}
            saved={saved.acmeEmail}
          />
          <SettingRow
            kind="switch"
            anchor="acme-terms"
            label={s.terms}
            help={s.termsHelp}
            checked={form.acmeTerms}
            onChange={(acmeTerms) => update({ acmeTerms })}
            saved={saved.acmeTerms}
          >
            {error('acme-terms') && <FieldError>{error('acme-terms')}</FieldError>}
          </SettingRow>
          <SettingRow
            kind="choice"
            anchor="acme-challenge"
            label={s.challenge}
            help={s.challengeHelp}
            error={error('acme-challenge')}
            value={form.acmeChallenge}
            options={challenges.map((challenge) => ({
              value: challenge,
              label: s.challenges[challenge],
            }))}
            onChange={(acmeChallenge: AcmeChallenge) => update({ acmeChallenge })}
            saved={saved.acmeChallenge}
          />
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

function Tracking({ form, update, error, saved }: SectionFormApi) {
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
              {traktRedirectUri}
            </code>
          </p>
          <p className="mt-2 max-w-[60ch] text-ink-3">{s.traktOlderApps}</p>
        </TextRow>
        <SettingRow
          kind="text"
          anchor="trakt-client-id"
          label={s.traktClientId}
          help={s.traktClientIdHelp}
          error={error('trakt-client-id')}
          value={form.traktClientId}
          onChange={(traktClientId) => update({ traktClientId })}
          mono
          saved={saved.traktClientId}
        />
        <SettingRow
          kind="secret"
          anchor="trakt-client-secret"
          label={s.traktClientSecret}
          help={s.traktClientSecretHelp}
          saved={form.traktClientSecretSet}
          value={form.traktClientSecret}
          onValue={(traktClientSecret) => update({ traktClientSecret })}
          reveal={() => revealServerSecret('traktClientSecret')}
        />
      </SettingsGroup>
      <SettingsGroup title="Simkl">
        <TextRow anchor="simkl-setup">
          <p className="max-w-[60ch]">{s.simklSetup}</p>
        </TextRow>
        <SettingRow
          kind="text"
          anchor="simkl-client-id"
          label={s.simklClientId}
          help={s.simklClientIdHelp}
          error={error('simkl-client-id')}
          value={form.simklClientId}
          onChange={(simklClientId) => update({ simklClientId })}
          mono
          saved={saved.simklClientId}
        />
      </SettingsGroup>
      <SettingsGroup title="AniList">
        <TextRow anchor="anilist-setup">
          <p className="max-w-[60ch]">{s.aniListSetup}</p>
          <Address label={s.redirectUri} value={aniListPinUri} />
        </TextRow>
        <SettingRow
          kind="text"
          anchor="anilist-client-id"
          label={s.aniListClientId}
          help={s.aniListClientIdHelp}
          error={error('anilist-client-id')}
          value={form.aniListClientId}
          onChange={(aniListClientId) => update({ aniListClientId })}
          mono
          saved={saved.aniListClientId}
        />
        <SettingRow
          kind="secret"
          anchor="anilist-client-secret"
          label={s.aniListClientSecret}
          help={s.aniListClientSecretHelp}
          saved={form.aniListClientSecretSet}
          value={form.aniListClientSecret}
          onValue={(aniListClientSecret) => update({ aniListClientSecret })}
          reveal={() => revealServerSecret('aniListClientSecret')}
        />
      </SettingsGroup>
      <SettingsGroup title="MyAnimeList">
        <TextRow anchor="myanimelist-setup">
          <p className="max-w-[60ch]">{s.myAnimeListSetup}</p>
          <Address label={s.redirectUri} value={myAnimeListRedirect(form.publicAddress)} />
          <p className="mt-1 text-small text-ink-3">{s.myAnimeListRedirectHelp}</p>
        </TextRow>
        <SettingRow
          kind="text"
          anchor="myanimelist-client-id"
          label={s.myAnimeListClientId}
          help={s.myAnimeListClientIdHelp}
          error={error('myanimelist-client-id')}
          value={form.myAnimeListClientId}
          onChange={(myAnimeListClientId) => update({ myAnimeListClientId })}
          mono
          saved={saved.myAnimeListClientId}
        />
        <SettingRow
          kind="secret"
          anchor="myanimelist-client-secret"
          label={s.myAnimeListClientSecret}
          help={s.myAnimeListClientSecretHelp}
          saved={form.myAnimeListClientSecretSet}
          value={form.myAnimeListClientSecret}
          onValue={(myAnimeListClientSecret) => update({ myAnimeListClientSecret })}
          reveal={() => revealServerSecret('myAnimeListClientSecret')}
        />
      </SettingsGroup>
      <SettingsGroup title="Last.fm">
        <TextRow anchor="lastfm-setup">
          <p className="max-w-[60ch]">{s.lastFmSetup}</p>
        </TextRow>
        <SettingRow
          kind="text"
          anchor="lastfm-api-key"
          label={s.lastFmApiKey}
          help={s.lastFmApiKeyHelp}
          error={error('lastfm-api-key')}
          value={form.lastFmApiKey}
          onChange={(lastFmApiKey) => update({ lastFmApiKey })}
          mono
          saved={saved.lastFmApiKey}
        />
        <SettingRow
          kind="secret"
          anchor="lastfm-secret"
          label={s.lastFmSecret}
          help={s.lastFmSecretHelp}
          saved={form.lastFmSecretSet}
          value={form.lastFmSecret}
          onValue={(lastFmSecret) => update({ lastFmSecret })}
          reveal={() => revealServerSecret('lastFmSecret')}
        />
      </SettingsGroup>
    </>
  )
}

function LiveTv({ form, update, error, number, range, saved, minMax }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="number"
        anchor="live-tv-refresh-hours"
        label={s.liveTvRefreshHours}
        help={s.liveTvRefreshHoursHelp(range('liveTvRefreshHours'))}
        error={error('live-tv-refresh-hours')}
        {...minMax('liveTvRefreshHours')}
        step={1}
        {...number('live-tv-refresh-hours', form.liveTvRefreshHours, (value) =>
          update({ liveTvRefreshHours: Math.trunc(value) }),
        )}
        saved={saved.liveTvRefreshHours}
      />
    </SettingsGroup>
  )
}

function Recordings({ form, update, error, number, range, limits, saved, minMax }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  const host = computer(useServerSystem())
  const hours = Array.from({ length: 24 }, (_, hour) => ({
    value: hour,
    label: formatHour(hour, language),
  }))
  return (
    <>
      <SettingsGroup>
        <SettingRow
          kind="switch"
          anchor="recording"
          label={s.recording}
          help={s.recordingHelp}
          checked={form.recording}
          onChange={(recording) => update({ recording })}
          saved={saved.recording}
        />
        <SettingRow
          kind="text"
          anchor="recordings-folder"
          label={s.recordingsFolderLabel}
          help={s.recordingsFolderHelp[host]}
          error={error('recordings-folder')}
          value={form.recordingsFolder}
          onChange={(recordingsFolder) => update({ recordingsFolder })}
          placeholder={form.recordingsFolderDefault}
          disabled={!form.recording}
          maxLength={limits('recordingsFolder').max}
          mono
          saved={saved.recordingsFolder}
        />
        <SettingRow
          kind="number"
          anchor="recording-pre-padding"
          label={s.recordingPrePadding}
          help={s.recordingPrePaddingHelp(range('recordingPrePadding', 1 / 60))}
          error={error('recording-pre-padding')}
          {...minMax('recordingPrePadding', 1 / 60)}
          step={1}
          {...number('recording-pre-padding', Math.round(form.recordingPrePadding / 60), (value) =>
            update({ recordingPrePadding: Math.trunc(value) * 60 }),
          )}
          saved={saved.recordingPrePadding}
        />
        <SettingRow
          kind="number"
          anchor="recording-post-padding"
          label={s.recordingPostPadding}
          help={s.recordingPostPaddingHelp(range('recordingPostPadding', 1 / 60))}
          error={error('recording-post-padding')}
          {...minMax('recordingPostPadding', 1 / 60)}
          step={1}
          {...number(
            'recording-post-padding',
            Math.round(form.recordingPostPadding / 60),
            (value) => update({ recordingPostPadding: Math.trunc(value) * 60 }),
          )}
          saved={saved.recordingPostPadding}
        />
        <SettingRow
          kind="number"
          anchor="recording-retention-days"
          label={s.recordingRetentionDays}
          help={s.recordingRetentionDaysHelp(range('recordingRetentionDays'))}
          error={error('recording-retention-days')}
          {...minMax('recordingRetentionDays')}
          step={1}
          {...number('recording-retention-days', form.recordingRetentionDays, (value) =>
            update({ recordingRetentionDays: Math.trunc(value) }),
          )}
          saved={saved.recordingRetentionDays}
        />
        <SettingRow
          kind="switch"
          anchor="recording-commercials"
          label={s.recordingCommercials}
          help={s.recordingCommercialsHelp}
          checked={form.recordingCommercials}
          onChange={(recordingCommercials) => update({ recordingCommercials })}
          saved={saved.recordingCommercials}
        />
      </SettingsGroup>
      <SettingsGroup>
        <SettingRow
          kind="switch"
          anchor="convert-recordings"
          label={s.convertRecordings}
          help={s.convertRecordingsHelp}
          checked={form.convertRecordings}
          onChange={(convertRecordings) => update({ convertRecordings })}
          saved={saved.convertRecordings}
        />
        <SettingRow
          kind="choice"
          anchor="convert-recordings-between"
          label={s.convertRecordingsFrom}
          detail={s.convertRecordingsBetween}
          help={s.convertRecordingsBetweenHelp(
            formatHour(Number(form.bounds.convertRecordingsFrom?.default), language),
            formatHour(Number(form.bounds.convertRecordingsUntil?.default), language),
          )}
          error={error('convert-recordings-between')}
          value={form.convertRecordingsFrom}
          options={hours}
          onChange={(convertRecordingsFrom) => update({ convertRecordingsFrom })}
          disabled={!form.convertRecordings}
          saved={saved.convertRecordingsFrom}
        />
        <SettingRow
          kind="choice"
          label={s.convertRecordingsUntil}
          detail={s.convertRecordingsBetween}
          value={form.convertRecordingsUntil}
          options={hours}
          onChange={(convertRecordingsUntil) => update({ convertRecordingsUntil })}
          disabled={!form.convertRecordings}
          saved={saved.convertRecordingsUntil}
        />
        <SettingRow
          kind="switch"
          anchor="convert-recordings-on-processor"
          label={s.convertRecordingsOnProcessor}
          help={s.convertRecordingsOnProcessorHelp}
          checked={form.convertRecordingsOnProcessor}
          onChange={(convertRecordingsOnProcessor) => update({ convertRecordingsOnProcessor })}
          disabled={!form.convertRecordings}
          saved={saved.convertRecordingsOnProcessor}
        />
      </SettingsGroup>
    </>
  )
}

function Backups({ form, update, error, number, range, limits, saved, minMax }: SectionFormApi) {
  const { language, t } = useI18n()
  const s = t.settings
  const host = computer(useServerSystem())
  return (
    <>
      <SettingsGroup>
        <SettingRow
          kind="switch"
          anchor="backups"
          label={s.backups}
          help={s.backupsHelp}
          checked={form.backups}
          onChange={(backups) => update({ backups })}
          saved={saved.backups}
        />
        <SettingRow
          kind="text"
          anchor="backup-folder"
          label={s.backupFolderLabel}
          help={s.backupFolderHelp[host]}
          error={error('backup-folder')}
          value={form.backupFolder}
          onChange={(backupFolder) => update({ backupFolder })}
          placeholder={form.backupFolderDefault}
          disabled={!form.backups}
          maxLength={limits('backupFolder').max}
          mono
          saved={saved.backupFolder}
        />
        <SettingRow
          kind="choice"
          anchor="backup-hour"
          label={s.backupHour}
          help={s.backupHourHelp(formatHour(Number(form.bounds.backupHour?.default), language))}
          error={error('backup-hour')}
          value={form.backupHour}
          options={Array.from({ length: 24 }, (_, hour) => ({
            value: hour,
            label: formatHour(hour, language),
          }))}
          onChange={(backupHour) => update({ backupHour })}
          saved={saved.backupHour}
        />
        <SettingRow
          kind="number"
          anchor="backups-kept"
          label={s.backupsKept}
          help={s.backupsKeptHelp(range('backupsKept'))}
          error={error('backups-kept')}
          {...minMax('backupsKept')}
          step={1}
          {...number('backups-kept', form.backupsKept, (value) =>
            update({ backupsKept: Math.trunc(value) }),
          )}
          saved={saved.backupsKept}
        />
        <SettingRow
          kind="secret"
          anchor="backup-passphrase"
          label={s.backupPassphrase}
          help={s.backupPassphraseHelp(form.bounds.backupPassphrase?.min ?? 12)}
          error={error('backup-passphrase')}
          saved={form.backupPassphraseSet}
          value={form.backupPassphrase}
          onValue={(backupPassphrase) => update({ backupPassphrase })}
        >
          {form.backupPassphrase !== undefined && form.backupPassphrase !== '' && (
            <Notice tone="warn" className="max-w-[60ch]">
              {s.backupPassphraseNew}
            </Notice>
          )}
          {form.backupPassphraseSet && form.backupPassphrase === '' && (
            <Notice className="max-w-[60ch]">{s.backupPassphraseRemoved}</Notice>
          )}
        </SettingRow>
        {form.backups && <LastBackup />}
      </SettingsGroup>
      <SettingsGroup title={s.backupDestinations}>
        <ListRows anchor="backup-destinations" help={s.backupDestinationsHelp}>
          <BackupDestinations />
        </ListRows>
      </SettingsGroup>
    </>
  )
}

function WebPlayer({ form, update, error, limits, dirty, adopt, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  const custom = form.webTheme === 'custom'
  const shown = shownTexts(form)
  const inApp = jellyfinApp() !== null
  return (
    <>
      <SettingsGroup>
        {status.data?.webClient === true && (
          <PlainRow anchor="open-web-player">
            {/* A preview in a new tab, which keeps this page, but in place in a Jellyfin app: Android
                hands a new tab to the system browser. */}
            <ExternalButtonLink
              {...webPlayerLink(inApp)}
              icon={PlayIcon}
              iconEnd={inApp ? undefined : ArrowSquareOutIcon}
            >
              {s.openWebPlayer}
            </ExternalButtonLink>
          </PlainRow>
        )}
        <SettingRow
          kind="choice"
          anchor="web-theme"
          label={s.webTheme}
          help={s.webThemeHelp}
          error={error('web-theme')}
          value={form.webTheme}
          options={webThemes.map((value) => ({ value, label: s.webThemeChoices[value] }))}
          onChange={(webTheme: WebTheme) => update({ webTheme })}
          saved={saved.webTheme}
        >
          <p className="max-w-[60ch]">{s.webThemeReload}</p>
          {form.webTheme === 'lumaaglaass' && (
            <div className="mt-3">
              <EditAsCustom form={form} update={update} />
            </div>
          )}
        </SettingRow>
        <PlainRow anchor="custom-css">
          <CodeEditor
            label={s.customCss}
            help={form.webTheme === 'custom' ? s.customCssHelp : s.webThemeTexts[form.webTheme]}
            value={shown.css}
            onValue={(customCss) => update({ customCss })}
            maxBytes={limits('customCss').max ?? 0}
            readOnly={!custom}
            error={error('custom-css')}
          />
        </PlainRow>
        <PlainRow anchor="custom-js">
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
        </PlainRow>
        {custom && (
          <PlainRow anchor="theme-copy">
            <ThemeCopyPanel dirty={dirty} adopt={adopt} />
          </PlainRow>
        )}
        <PlainRow anchor="login-disclaimer">
          <CodeEditor
            label={s.loginDisclaimer}
            help={s.loginDisclaimerHelp}
            value={form.loginDisclaimer}
            onValue={(loginDisclaimer) => update({ loginDisclaimer })}
            maxBytes={limits('loginDisclaimer').max ?? 0}
            rows={4}
            error={error('login-disclaimer')}
          />
        </PlainRow>
        <SettingRow
          kind="switch"
          anchor="add-versions-to-open-page"
          label={s.addVersionsToOpenPage}
          help={s.addVersionsToOpenPageHelp}
          checked={form.addVersionsToOpenPage}
          onChange={(addVersionsToOpenPage) => update({ addVersionsToOpenPage })}
          saved={saved.addVersionsToOpenPage}
        />
        <SettingRow
          kind="switch"
          anchor="poster-ratings"
          label={s.posterRatings}
          help={s.posterRatingsHelp}
          checked={form.posterRatings}
          onChange={(posterRatings) => update({ posterRatings })}
          saved={saved.posterRatings}
        />
      </SettingsGroup>
      <SettingsGroup title={s.homeRows}>
        <ListRows anchor="home-rows" help={s.homeRowsHelp}>
          <HomeRows />
        </ListRows>
      </SettingsGroup>
    </>
  )
}

function DiagnosticsFields({ form, update, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <>
      <SettingsGroup>
        <SettingRow
          kind="switch"
          anchor="detailed-log"
          label={s.detailedLog}
          help={s.detailedLogHelp}
          checked={form.detailedLog}
          onChange={(detailedLog) => update({ detailedLog })}
          saved={saved.detailedLog}
        />
      </SettingsGroup>
      <Variables />
    </>
  )
}

function Notifications({ form, update, error, number, limits, saved, minMax }: SectionFormApi) {
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
        <SettingRow
          kind="text"
          anchor="public-address"
          label={s.publicAddress}
          help={s.publicAddressHelp}
          error={error('public-address')}
          value={form.publicAddress}
          onChange={(publicAddress) => update({ publicAddress })}
          placeholder={offered[0] ?? 'https://media.example.org'}
          maxLength={limits('publicAddress').max}
          mono
          saved={saved.publicAddress}
        >
          {offered.length > 0 && form.publicAddress === '' && (
            <div className="flex flex-wrap items-center gap-3">
              {offered.map((address) => (
                <Button key={address} size="sm" onClick={() => update({ publicAddress: address })}>
                  {s.https.useAddress(address)}
                </Button>
              ))}
              <span className="text-small text-ink-2">{s.https.offeredAddress}</span>
            </div>
          )}
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title={s.weeklySummary}>
        <TextRow anchor="weekly-summary">
          <p className="max-w-[60ch]">{s.weeklySummaryHelp}</p>
        </TextRow>
        <SettingRow
          kind="choice"
          anchor="weekly-summary-day"
          label={s.weeklySummaryDay}
          help={s.weeklySummaryDayHelp(weekday(Number(form.bounds.weeklySummaryDay?.default)))}
          error={error('weekly-summary-day')}
          value={form.weeklySummaryDay}
          options={[1, 2, 3, 4, 5, 6, 0].map((day) => ({ value: day, label: weekday(day) }))}
          onChange={(weeklySummaryDay) => update({ weeklySummaryDay })}
          saved={saved.weeklySummaryDay}
        />
        <SettingRow
          kind="choice"
          anchor="weekly-summary-hour"
          label={s.weeklySummaryHour}
          help={s.weeklySummaryHourHelp(
            formatHour(Number(form.bounds.weeklySummaryHour?.default), language),
          )}
          error={error('weekly-summary-hour')}
          value={form.weeklySummaryHour}
          options={Array.from({ length: 24 }, (_, hour) => ({
            value: hour,
            label: formatHour(hour, language),
          }))}
          onChange={(weeklySummaryHour) => update({ weeklySummaryHour })}
          saved={saved.weeklySummaryHour}
        />
        <PlainRow anchor="summary-preview">
          <h3 className="mb-3 text-h3 text-ink max-md:text-[18px]">{text.preview.title}</h3>
          <SummaryPreview />
        </PlainRow>
      </SettingsGroup>
      <SettingsGroup title={s.email}>
        <TextRow anchor="email-setup">
          <p className="max-w-[60ch]">{s.emailHelp}</p>
        </TextRow>
        <SettingRow
          kind="text"
          anchor="smtp-host"
          label={s.smtpHost}
          help={s.smtpHostHelp}
          error={error('smtp-host')}
          value={form.smtpHost}
          onChange={(smtpHost) => update({ smtpHost })}
          placeholder="smtp.example.org"
          maxLength={limits('smtpHost').max}
          mono
          saved={saved.smtpHost}
        />
        <SettingRow
          kind="number"
          anchor="smtp-port"
          label={s.smtpPort}
          help={s.smtpPortHelp}
          error={error('smtp-port')}
          {...minMax('smtpPort')}
          step={1}
          {...number('smtp-port', form.smtpPort, (value) =>
            update({ smtpPort: Math.trunc(value) }),
          )}
          saved={saved.smtpPort}
        />
        <SettingRow
          kind="choice"
          anchor="smtp-security"
          label={s.smtpSecurity}
          help={s.smtpSecurityHelp}
          error={error('smtp-security')}
          value={form.smtpSecurity}
          options={(['starttls', 'tls', 'none'] as const).map((value) => ({
            value,
            label: s.smtpSecurities[value],
          }))}
          onChange={(smtpSecurity) => update({ smtpSecurity })}
          saved={saved.smtpSecurity}
        />
        <SettingRow
          kind="text"
          anchor="smtp-user"
          label={s.smtpUser}
          help={s.smtpUserHelp}
          error={error('smtp-user')}
          value={form.smtpUser}
          onChange={(smtpUser) => update({ smtpUser })}
          maxLength={limits('smtpUser').max}
          saved={saved.smtpUser}
        />
        <SettingRow
          kind="secret"
          anchor="smtp-password"
          label={s.smtpPassword}
          help={s.smtpPasswordHelp}
          saved={form.smtpPasswordSet}
          value={form.smtpPassword}
          onValue={(smtpPassword) => update({ smtpPassword })}
        />
        <SettingRow
          kind="text"
          anchor="smtp-from"
          label={s.smtpFrom}
          help={s.smtpFromHelp}
          error={error('smtp-from')}
          inputMode="email"
          value={form.smtpFrom}
          onChange={(smtpFrom) => update({ smtpFrom })}
          placeholder="polyfin@example.org"
          maxLength={limits('smtpFrom').max}
          mono
          saved={saved.smtpFrom}
        />
        <SettingRow
          kind="text"
          anchor="smtp-from-name"
          label={s.smtpFromName}
          help={s.smtpFromNameHelp}
          error={error('smtp-from-name')}
          value={form.smtpFromName}
          onChange={(smtpFromName) => update({ smtpFromName })}
          placeholder={form.serverName}
          maxLength={limits('smtpFromName').max}
          saved={saved.smtpFromName}
        />
      </SettingsGroup>
      <SettingsGroup title={text.serverTargets}>
        <ListRows anchor="notification-targets" help={text.serverTargetsHelp}>
          <NotificationTargets scope="server" />
        </ListRows>
      </SettingsGroup>
    </>
  )
}
