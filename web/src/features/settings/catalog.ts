import type { Settings } from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import type en from '@/i18n/en/settings'
import type { RangeText } from '@/i18n/en/settings'

/** The messages a setting's words come from: the settings area file of one language. */
export type SettingsText = typeof en

/**
 * The bounds and default of a setting, by its name in Settings, as help texts show them; `scale`
 * converts the server's unit to the one the field shows.
 */
export type RangeOf = (name: keyof Settings, scale?: number) => RangeText

/**
 * One setting, wherever it is found: the section search, the command palette and the field
 * errors all read this list, so a setting is described once.
 */
export type SettingEntry = {
  section: SettingsSectionId
  /** The id of the setting's row on its section page, the `#anchor` of its address. */
  anchor: string
  label: (text: SettingsText) => string
  help?: (text: SettingsText, range: RangeOf) => string
  /** More words that find it: service names, technical terms. */
  keywords?: readonly string[]
  /** Server error codes about this setting, shown under it. */
  codes?: readonly string[]
}

const s = (text: SettingsText) => text.settings
const c = (text: SettingsText) => text.settings.conversion
const tr = (text: SettingsText) => text.settings.tracking

/** Every setting, in the order of the sections and of their pages. */
export const settingEntries: readonly SettingEntry[] = [
  // General
  {
    section: 'general',
    anchor: 'server-name',
    label: (t) => s(t).serverName,
    help: (t, r) => s(t).serverNameHelp(r('serverName')),
    codes: ['invalid_server_name'],
  },
  {
    section: 'general',
    anchor: 'language',
    label: (t) => s(t).language,
    help: (t) => s(t).languageHelp,
    codes: ['invalid_language'],
  },
  {
    section: 'general',
    anchor: 'quick-connect',
    label: (t) => s(t).quickConnect,
    help: (t) => s(t).quickConnectHelp,
  },
  {
    section: 'general',
    anchor: 'legacy-authorization',
    label: (t) => s(t).legacyAuthorization,
    help: (t) => `${s(t).legacyAuthorizationHelp} ${s(t).legacyWarning}`,
    keywords: ['X-Emby', 'api_key'],
  },
  // Playback
  {
    section: 'playback',
    anchor: 'prepare-ahead',
    label: (t) => s(t).prepareAhead,
    help: (t) => s(t).prepareAheadHelp,
  },
  {
    section: 'playback',
    anchor: 'prefer-direct-play',
    label: (t) => s(t).preferDirectPlay,
    help: (t) => s(t).preferDirectPlayHelp,
  },
  {
    section: 'playback',
    anchor: 'analysis-timeout',
    label: (t) => s(t).analysisTimeout,
    help: (t, r) => s(t).analysisTimeoutHelp(r('analysisTimeout')),
    codes: ['invalid_analysis_timeout'],
  },
  {
    section: 'playback',
    anchor: 'version-attempts',
    label: (t) => s(t).versionAttempts,
    help: (t, r) => s(t).versionAttemptsHelp(r('versionAttempts')),
    codes: ['invalid_version_attempts'],
  },
  // Conversion
  {
    section: 'conversion',
    anchor: 'transcoding',
    label: (t) => s(t).transcoding,
    help: (t) => s(t).transcodingHelp,
  },
  {
    section: 'conversion',
    anchor: 'max-conversions',
    label: (t) => s(t).maxConversions,
    help: (t, r) => s(t).maxConversionsHelp(r('maxConversions')),
    codes: ['invalid_max_conversions'],
  },
  {
    section: 'conversion',
    anchor: 'max-conversion-height',
    label: (t) => s(t).maxConversionHeight,
    help: (t) => s(t).maxConversionHeightHelp,
    codes: ['invalid_max_conversion_height'],
  },
  {
    section: 'conversion',
    anchor: 'hardware-acceleration',
    label: (t) => c(t).hardwareAcceleration,
    help: (t) => c(t).hardwareAccelerationHelp,
    keywords: ['gpu', 'nvidia', 'vaapi', 'POLYFIN_HWACCEL'],
    codes: ['invalid_hardware_acceleration'],
  },
  {
    section: 'conversion',
    anchor: 'detected-hardware',
    label: (t) => c(t).detected,
    help: (t) => `${c(t).noGpu} ${c(t).gpuToneMapping} ${c(t).processor}`,
    keywords: ['gpu', 'ffmpeg'],
  },
  {
    section: 'conversion',
    anchor: 'hardware-decoding',
    label: (t) => c(t).hardwareDecoding,
    help: (t) => `${c(t).hardwareDecodingHelp} ${Object.values(c(t).codecs).join(' ')}`,
    codes: ['invalid_hardware_decoding_codecs'],
  },
  {
    section: 'conversion',
    anchor: 'encoder-preset',
    label: (t) => c(t).encoderPreset,
    help: (t) => c(t).encoderPresetHelp,
    keywords: ['preset'],
    codes: ['invalid_encoder_preset'],
  },
  {
    section: 'conversion',
    anchor: 'video-quality',
    label: (t) => `${c(t).h264Quality}, ${c(t).hevcQuality}`,
    help: (t, r) => c(t).qualityHelp(r('h264Quality')),
    keywords: ['crf'],
    codes: ['invalid_video_quality'],
  },
  {
    section: 'conversion',
    anchor: 'allow-hevc-encoding',
    label: (t) => c(t).allowHevcEncoding,
    help: (t) => c(t).allowHevcEncodingHelp,
    keywords: ['h265'],
  },
  {
    section: 'conversion',
    anchor: 'tone-mapping',
    label: (t) => c(t).toneMapping,
    help: (t) => c(t).toneMappingHelp,
    keywords: ['hdr', 'sdr'],
  },
  {
    section: 'conversion',
    anchor: 'tone-mapping-algorithm',
    label: (t) => c(t).toneMappingAlgorithm,
    help: (t) => c(t).toneMappingAlgorithmHelp,
    codes: ['invalid_tone_mapping_algorithm'],
  },
  {
    section: 'conversion',
    anchor: 'tone-mapping-peak',
    label: (t) => `${c(t).toneMappingPeak}, ${c(t).toneMappingDesat}`,
    help: (t, r) => c(t).toneMappingPeakHelp(r('toneMappingPeak'), r('toneMappingDesat')),
    codes: ['invalid_tone_mapping_peak', 'invalid_tone_mapping_desat'],
  },
  {
    section: 'conversion',
    anchor: 'deinterlace-method',
    label: (t) => c(t).deinterlaceMethod,
    help: (t) => c(t).deinterlaceMethodHelp,
    keywords: ['yadif', 'bwdif'],
    codes: ['invalid_deinterlace_method'],
  },
  {
    section: 'conversion',
    anchor: 'deinterlace-double-rate',
    label: (t) => c(t).deinterlaceDoubleRate,
    help: (t) => c(t).deinterlaceDoubleRateHelp,
  },
  {
    section: 'conversion',
    anchor: 'downmix-algorithm',
    label: (t) => c(t).downmixAlgorithm,
    help: (t) => c(t).downmixAlgorithmHelp,
    keywords: ['downmix'],
    codes: ['invalid_downmix_algorithm'],
  },
  {
    section: 'conversion',
    anchor: 'downmix-boost',
    label: (t) => c(t).downmixBoost,
    help: (t, r) => c(t).downmixBoostHelp(r('downmixBoost')),
    codes: ['invalid_downmix_boost'],
  },
  {
    section: 'conversion',
    anchor: 'max-audio-channels',
    label: (t) => c(t).maxAudioChannels,
    help: (t) => c(t).maxAudioChannelsHelp,
    codes: ['invalid_max_audio_channels'],
  },
  {
    section: 'conversion',
    anchor: 'audio-bitrate-per-channel',
    label: (t) => c(t).audioBitratePerChannel,
    help: (t, r) => c(t).audioBitratePerChannelHelp(r('audioBitratePerChannel')),
    codes: ['invalid_audio_bitrate_per_channel'],
  },
  {
    section: 'conversion',
    anchor: 'encoding-threads',
    label: (t) => c(t).encodingThreads,
    help: (t, r) => c(t).encodingThreadsHelp(r('encodingThreads')),
    codes: ['invalid_encoding_threads'],
  },
  {
    section: 'conversion',
    anchor: 'ahead-seconds',
    label: (t) => c(t).aheadSeconds,
    help: (t, r) => c(t).aheadSecondsHelp(r('aheadSeconds')),
    keywords: ['throttle'],
    codes: ['invalid_ahead_seconds'],
  },
  // Content
  {
    section: 'content',
    anchor: 'skip-buttons',
    label: (t) => s(t).skipButtons,
    help: (t) => s(t).skipButtonsHelp,
  },
  {
    section: 'content',
    anchor: 'segment-order',
    label: (t) => s(t).segmentSources.label,
    help: (t) => s(t).segmentSources.help,
    keywords: ['TheIntroDB', 'IntroDB', 'PublicMetaDB', 'POLYFIN_SEGMENTS'],
    codes: ['invalid_segment_order', 'invalid_segment_sources_off'],
  },
  {
    section: 'content',
    anchor: 'publicmetadb-key',
    label: (t) => s(t).publicMetaDbKey,
    help: (t) => s(t).publicMetaDbKeyHelp,
    keywords: ['PublicMetaDB', 'api key'],
    codes: ['invalid_publicmetadb_key', 'publicmetadb_unreachable'],
  },
  {
    section: 'content',
    anchor: 'theintrodb-key',
    label: (t) => s(t).theIntroDbKey,
    help: (t) => s(t).theIntroDbKeyHelp,
    keywords: ['TheIntroDB', 'api key'],
    codes: ['invalid_theintrodb_key', 'theintrodb_unreachable'],
  },
  {
    section: 'content',
    anchor: 'similar-titles',
    label: (t) => s(t).similarTitles,
    help: (t) => s(t).similarTitlesHelp,
  },
  {
    section: 'content',
    anchor: 'played-percent',
    label: (t) => s(t).playedPercent,
    help: (t, r) => s(t).playedPercentHelp(r('playedPercent')),
    codes: ['invalid_played_percent'],
  },
  {
    section: 'content',
    anchor: 'resume-percent',
    label: (t) => s(t).resumePercent,
    help: (t, r) => s(t).resumePercentHelp(r('resumePercent')),
    codes: ['invalid_resume_percent', 'resume_not_below_played'],
  },
  // Catalogs
  {
    section: 'catalogs',
    anchor: 'catalog-limit',
    label: (t) => s(t).catalogLimit,
    help: (t, r) => s(t).catalogLimitHelp(r('catalogLimit')),
    codes: ['invalid_catalog_limit'],
  },
  {
    section: 'catalogs',
    anchor: 'channel-limit',
    label: (t) => s(t).channelLimit,
    help: (t, r) => s(t).channelLimitHelp(r('channelLimit')),
    codes: ['invalid_channel_limit'],
  },
  {
    section: 'catalogs',
    anchor: 'version-list-minutes',
    label: (t) => s(t).versionListMinutes,
    help: (t, r) => s(t).versionListMinutesHelp(r('versionListMinutes')),
    codes: ['invalid_version_list_minutes'],
  },
  {
    section: 'catalogs',
    anchor: 'catalog-refresh-minutes',
    label: (t) => s(t).catalogRefreshMinutes,
    help: (t, r) => s(t).catalogRefreshMinutesHelp(r('catalogRefreshMinutes')),
    codes: ['invalid_catalog_refresh_minutes'],
  },
  // Thumbnails
  {
    section: 'thumbnails',
    anchor: 'trickplay',
    label: (t) => s(t).trickplay,
    help: (t) => `${s(t).trickplayHelp} ${s(t).thumbnailsHelp}`,
    keywords: ['trickplay'],
  },
  {
    section: 'thumbnails',
    anchor: 'trickplay-interval',
    label: (t) => s(t).trickplayInterval,
    help: (t, r) => s(t).trickplayIntervalHelp(r('trickplayInterval')),
    codes: ['invalid_trickplay_interval'],
  },
  {
    section: 'thumbnails',
    anchor: 'trickplay-width',
    label: (t) => s(t).trickplayWidth,
    help: (t) => s(t).trickplayWidthHelp,
    codes: ['invalid_trickplay_width'],
  },
  {
    section: 'thumbnails',
    anchor: 'chapter-images',
    label: (t) => s(t).chapterImages,
    help: (t) => s(t).chapterImagesHelp,
  },
  {
    section: 'thumbnails',
    anchor: 'thumbnail-storage',
    label: (t) => s(t).thumbnailStorage,
    help: (t, r) => s(t).thumbnailStorageHelp(r('thumbnailStorageGB')),
    codes: ['invalid_thumbnail_storage_gb'],
  },
  // Security
  {
    section: 'security',
    anchor: 'personal-addons',
    label: (t) => s(t).personalAddons,
    help: (t) => s(t).personalAddonsHelp,
  },
  {
    section: 'security',
    anchor: 'login-attempts',
    label: (t) => s(t).loginAttempts,
    help: (t, r) => s(t).loginAttemptsHelp(r('loginAttempts')),
    codes: ['invalid_login_attempts'],
  },
  {
    section: 'security',
    anchor: 'inactive-device-days',
    label: (t) => s(t).inactiveDeviceDays,
    help: (t, r) => s(t).inactiveDeviceDaysHelp(r('inactiveDeviceDays')),
    codes: ['invalid_inactive_device_days'],
  },
  // Tracking
  {
    section: 'tracking',
    anchor: 'trakt-client-id',
    label: (t) => tr(t).traktClientId,
    help: (t) => `${tr(t).traktClientIdHelp} ${tr(t).traktSetup} ${tr(t).redirectUri}`,
    keywords: ['Trakt'],
    codes: ['invalid_trakt_app'],
  },
  {
    section: 'tracking',
    anchor: 'trakt-client-secret',
    label: (t) => tr(t).traktClientSecret,
    help: (t) => tr(t).traktClientSecretHelp,
    keywords: ['Trakt'],
  },
  {
    section: 'tracking',
    anchor: 'simkl-client-id',
    label: (t) => tr(t).simklClientId,
    help: (t) => `${tr(t).simklClientIdHelp} ${tr(t).simklSetup}`,
    keywords: ['Simkl'],
    codes: ['invalid_simkl_app'],
  },
  // Live TV
  {
    section: 'live-tv',
    anchor: 'live-tv-refresh-hours',
    label: (t) => s(t).liveTvRefreshHours,
    help: (t) => s(t).liveTvRefreshHoursHelp,
    keywords: ['iptv', 'xmltv', 'epg'],
    codes: ['invalid_live_tv_refresh_hours'],
  },
  // Recordings
  {
    section: 'recordings',
    anchor: 'recording-pre-padding',
    label: (t) => s(t).recordingPrePadding,
    help: (t, r) => s(t).recordingPrePaddingHelp(r('recordingPrePadding', 1 / 60)),
    codes: ['invalid_recording_padding'],
  },
  {
    section: 'recordings',
    anchor: 'recording-post-padding',
    label: (t) => s(t).recordingPostPadding,
    help: (t, r) => s(t).recordingPostPaddingHelp(r('recordingPostPadding', 1 / 60)),
    codes: ['invalid_recording_padding'],
  },
  {
    section: 'recordings',
    anchor: 'recording-retention-days',
    label: (t) => s(t).recordingRetentionDays,
    help: (t, r) => s(t).recordingRetentionDaysHelp(r('recordingRetentionDays')),
    codes: ['invalid_recording_retention_days'],
  },
  // Backups
  {
    section: 'backups',
    anchor: 'backup-hour',
    label: (t) => s(t).backupHour,
    help: (t, r) => s(t).backupHourHelp(r('backupHour').default),
    codes: ['invalid_backup_hour'],
  },
  {
    section: 'backups',
    anchor: 'backups-kept',
    label: (t) => s(t).backupsKept,
    help: (t, r) => s(t).backupsKeptHelp(r('backupsKept')),
    codes: ['invalid_backups_kept'],
  },
  {
    section: 'backups',
    anchor: 'last-backup',
    label: (t) => s(t).lastBackup,
    keywords: ['POLYFIN_BACKUP_DIR'],
  },
  // Web player
  {
    section: 'web-player',
    anchor: 'custom-css',
    label: (t) => s(t).customCss,
    help: (t) => s(t).customCssHelp,
    keywords: ['css', 'jellyfin-web'],
    codes: ['invalid_custom_css'],
  },
  {
    section: 'web-player',
    anchor: 'custom-js',
    label: (t) => s(t).customJs,
    help: (t) => `${s(t).customJsHelp} ${s(t).customJsWarning}`,
    keywords: ['javascript', 'js', 'jellyfin-web'],
    codes: ['invalid_custom_js'],
  },
  {
    section: 'web-player',
    anchor: 'login-disclaimer',
    label: (t) => s(t).loginDisclaimer,
    help: (t) => s(t).loginDisclaimerHelp,
    keywords: ['markdown', 'disclaimer'],
    codes: ['invalid_login_disclaimer'],
  },
  // Diagnostics
  {
    section: 'diagnostics',
    anchor: 'detailed-log',
    label: (t) => s(t).detailedLog,
    help: (t) => s(t).detailedLogHelp,
    keywords: ['debug'],
  },
  {
    section: 'diagnostics',
    anchor: 'environment-variables',
    label: (t) => t.settingsPage.sections.variables,
    help: (t) => t.settingsPage.variablesHelp,
    keywords: ['POLYFIN_', 'env', 'docker'],
  },
]

/** The anchor of an environment variable's row under Diagnostics. */
export function variableAnchor(name: string): string {
  return `variable-${name.toLowerCase()}`
}
