package accounts

import (
	"net/netip"
	"slices"

	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/webtheme"
)

// LumaaGlaassCss and LumaaGlaassJs are the custom CSS and script Polyfin
// serves to the web player when its theme is WebThemeLumaaGlaass, the
// default: they load the LumaaGlaass theme Polyfin bundles (see webtheme),
// from the web client's own address, with its settings page for each user
// and its home carousel off. The page takes the script's options as its
// defaults. Their addresses carry the version of the bundled files, which
// changes with them; they are not stored, so a new copy of the theme takes
// no migration.
var LumaaGlaassCss = `@import url('` + webtheme.Address("assets/lumaaglaass.css") + `');`

var LumaaGlaassJs = `(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: false,
        collectionFilter: true
    };

    const id = 'lg-script';
    if (document.getElementById(id)) return;

    const script = document.createElement('script');
    script.id = id;
    script.src = '` + webtheme.Address("assets/lumaaglaass.js") + `';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();`

// DefaultSettings are the settings of a new server, those the columns of
// the settings default to: changing a default takes this and a migration
// setting the column's.
func DefaultSettings() Settings {
	return Settings{
		ServerName:          "Polyfin",
		QuickConnectEnabled: true,
		LegacyAuthorization: false,
		Language:            i18n.Languages[0],
		PrepareAhead:        true,
		Transcoding:         true,
		CatalogLimit:        DefaultCatalogLimit,
		ChannelLimit:        DefaultChannelLimit,
		UpdateCheck:         true,
		LocalDiscovery:      true,
		LocalAddress:        "",
		DLNA:                false,
		DLNAName:            "",
		DLNANewDevicesUser:  nil,
		DLNAMarkPlayed:      false,

		SkipButtons:           true,
		SegmentOrder:          slices.Clone(SegmentSources),
		SegmentSourcesOff:     []string{},
		AudioIntros:           true,
		SimilarTitles:         true,
		GenerateImages:        true,
		CriticRatingShows:     CriticRatingChoices[0],
		Lyrics:                true,
		EvenVolume:            true,
		MeasureLoudness:       MeasureLoudnessChoices[0],
		PlayedPercent:         DefaultPlayedPercent,
		ResumePercent:         DefaultResumePercent,
		VersionListMinutes:    DefaultVersionListMinutes,
		CatalogRefreshMinutes: DefaultCatalogRefreshMinutes,

		TwoStepAdministrators: false,

		PersonalAddons:     true,
		ServerImports:      true,
		ShareLinks:         false,
		LoginAttempts:      DefaultLoginAttempts,
		InactiveDeviceDays: DefaultInactiveDeviceDays,
		KnownProxies:       []netip.Prefix{},
		KnownProxyNames:    []string{},
		HTTPSMode:          HTTPSModes[0],
		HTTPSPort:          DefaultHTTPSPort,
		HTTPSRedirect:      false,
		HTTPSCertificate:   "",
		HTTPSPrivateKey:    "",
		ACMEHosts:          []string{},
		ACMEEmail:          "",
		ACMETerms:          false,
		ACMEChallenge:      ACMEChallenges[0],
		DetailedLog:        false,

		AnalysisTimeout:     DefaultAnalysisTimeout,
		VersionAttempts:     DefaultVersionAttempts,
		PreferDirectPlay:    false,
		VersionOrder:        []string{},
		VersionCodecOrder:   slices.Clone(VersionCodecs),
		VersionSourceOrder:  []ID{},
		HiddenVersionCodecs: []string{},
		HiddenVersionWords:  []string{},
		NameHowVersionsPlay: true,
		HiddenReleases:      []string{},
		SyncAddonSubtitles:  true,
		MaxConversions:      DefaultMaxConversions,
		MaxConversionHeight: ConversionHeights[0],
		ConvertedDownloads:  false,
		SubtitleOCR:         SubtitleOCRModes[0],
		SubtitleOCRTracks:   DefaultSubtitleOCRTracks,

		EncoderPreset:              EncoderPresets[0],
		AllowHevcEncoding:          false,
		AdaptiveQuality:            AdaptiveQualityOff,
		HardwareAcceleration:       HardwareAccelerations[0],
		HardwareDecodingCodecs:     slices.Clone(HardwareDecodingCodecs),
		ToneMapping:                true,
		ToneMappingAlgorithm:       ToneMappingAlgorithms[0],
		GPUToneMapping:             true,
		ProcessorToneMappingHeight: ToneMappingHeights[0],
		DeinterlaceMethod:          DeinterlaceMethods[0],
		DeinterlaceDoubleRate:      false,
		DownmixAlgorithm:           DownmixAlgorithms[0],
		DownmixBoost:               DefaultDownmixBoost,
		MaxAudioChannels:           AudioChannelLimits[0],
		AheadSeconds:               DefaultAheadSeconds,
		KeepConvertedVideo:         false,

		Trickplay:          false,
		TrickplayInterval:  DefaultTrickplayInterval,
		TrickplayWidth:     DefaultTrickplayWidth,
		ChapterImages:      false,
		ThumbnailStorageGB: DefaultThumbnailStorageGB,

		RecordingPrePadding:    DefaultRecordingPrePadding,
		RecordingPostPadding:   DefaultRecordingPostPadding,
		RecordingRetentionDays: DefaultRecordingRetentionDays,
		RecordingCommercials:   false,
		LiveTvRefreshHours:     DefaultLiveTvRefreshHours,
		LocalScanHours:         DefaultLocalScanHours,
		WatchlistCheckHours:    DefaultWatchlistCheckHours,
		WatchLocalFolders:      true,

		ConvertRecordings:            false,
		ConvertRecordingsFrom:        DefaultConvertRecordingsFrom,
		ConvertRecordingsUntil:       DefaultConvertRecordingsUntil,
		ConvertRecordingsOnProcessor: false,

		WebTheme:              WebThemeLumaaGlaass,
		CustomCss:             "",
		CustomJs:              "",
		AddVersionsToOpenPage: true,

		LastFMAPIKey: "",
		LastFMSecret: "",

		AniListClientID:         "",
		AniListClientSecret:     "",
		MyAnimeListClientID:     "",
		MyAnimeListClientSecret: "",

		BackupHour:            DefaultBackupHour,
		BackupsKept:           DefaultBackupsKept,
		CollectionReadHour:    DefaultCollectionReadHour,
		CollectionLibraryApps: []string{},

		RemuxDB:    false,
		RemuxDBURL: DefaultRemuxDBURL,

		CacheSizeGB:      DefaultCacheSizeGB,
		VAAPIDevice:      "",
		Recording:        false,
		RecordingsFolder: "",
		Backups:          false,
		BackupFolder:     "",
		BackupPassphrase: "",

		PublicAddress: "",

		SMTPHost:     "",
		SMTPPort:     DefaultSMTPPort,
		SMTPSecurity: SMTPSecurities[0],
		SMTPUser:     "",
		SMTPPassword: "",
		SMTPFrom:     "",
		SMTPFromName: "",

		PlaybackHistory:     true,
		PlaybackHistoryDays: DefaultPlaybackHistoryDays,

		WeeklySummaryDay:  DefaultWeeklySummaryDay,
		WeeklySummaryHour: DefaultWeeklySummaryHour,

		ImportEveryMinutes: DefaultImportEveryMinutes,

		Metrics:      false,
		MetricsToken: "",

		RestartConfirmed: false,
		UpdateToolURL:    "",
		UpdateToolToken:  "",

		TorrentStreams:       false,
		TorrentInterface:     "",
		TorrentProxy:         "",
		TorrentProxyUser:     "",
		TorrentProxyPassword: "",
		TorrentWireGuard:     "",
		TorrentDownloadMbps:  0,
		TorrentUploadMbps:    0,
		TorrentsAtOnce:       0,
		TorrentTrackers:      []string{},

		ConversionFolder: "",
		ReadingFolder:    "",
		CacheFolder:      "",
		SiloApps:         false,

		PosterRatings:          false,
		PosterBadges:           false,
		PosterBadgeSize:        PosterBadgeSizes[0],
		PosterTemplate:         "",
		ProviderIssuer:         "",
		ProviderName:           "",
		ProviderClientID:       "",
		ProviderClientSecret:   "",
		ProviderUserClaim:      "preferred_username",
		ProviderCreateAccounts: false,
		ProviderAdminClaim:     "",
		ProviderAdminValue:     "",
	}
}

// SettingBounds are what a setting accepts, and its default.
type SettingBounds struct {
	// Default is the setting's value on a new server.
	Default any
	// Min and Max bound a number, the length of a text, in characters for
	// the server name and the backup passphrase's Min, and in bytes for
	// the others, or the number of entries of a list of addresses or apps.
	// Bounded tells whether they apply.
	Min, Max float64
	Bounded  bool
	// Zero tells that 0 is accepted too, below Min, turning the setting
	// off.
	Zero bool
	// Choices are the values the setting takes, or, for a list, those it
	// holds; nil when any value within the bounds is accepted.
	Choices []any
}

// SettingsBounds returns what each setting accepts, and its default, by
// the name the admin API gives it. The settings themselves check the same
// bounds (see UpdateSettings), as do the database's constraints.
func SettingsBounds() map[string]SettingBounds {
	d := DefaultSettings()
	between := func(min, max float64, value any) SettingBounds {
		return SettingBounds{Default: value, Min: min, Max: max, Bounded: true}
	}
	orZero := func(min, max float64, value any) SettingBounds {
		bounds := between(min, max, value)
		bounds.Zero = true
		return bounds
	}
	text := func(max int, value string) SettingBounds { return between(0, float64(max), value) }
	choice := func(value any, choices []any) SettingBounds { return SettingBounds{Default: value, Choices: choices} }
	is := func(value any) SettingBounds { return SettingBounds{Default: value} }
	return map[string]SettingBounds{
		"serverName":          between(1, maxNameLength, d.ServerName),
		"quickConnectEnabled": is(d.QuickConnectEnabled),
		"legacyAuthorization": is(d.LegacyAuthorization),
		"language":            choice(d.Language, anys(i18n.Languages)),
		"prepareAhead":        is(d.PrepareAhead),
		"transcoding":         is(d.Transcoding),
		"catalogLimit":        between(MinCatalogLimit, MaxCatalogLimit, d.CatalogLimit),
		"channelLimit":        between(MinChannelLimit, MaxChannelLimit, d.ChannelLimit),
		"updateCheck":         is(d.UpdateCheck),
		"localDiscovery":      is(d.LocalDiscovery),
		"localAddress":        text(MaxLocalAddressBytes, d.LocalAddress),
		"dlna":                is(d.DLNA),
		"dlnaName":            text(MaxDLNANameBytes, d.DLNAName),
		"dlnaNewDevicesUser":  is(d.DLNANewDevicesUser),
		"dlnaMarkPlayed":      is(d.DLNAMarkPlayed),

		"skipButtons":           is(d.SkipButtons),
		"publicMetaDbKey":       text(MaxSegmentKeyBytes, d.PublicMetaDBKey),
		"theIntroDbKey":         text(MaxSegmentKeyBytes, d.TheIntroDBKey),
		"segmentOrder":          choice(d.SegmentOrder, anys(SegmentSources)),
		"segmentSourcesOff":     choice(d.SegmentSourcesOff, anys(SegmentSources)),
		"audioIntros":           is(d.AudioIntros),
		"similarTitles":         is(d.SimilarTitles),
		"generateImages":        is(d.GenerateImages),
		"mdblistRatingsKey":     text(MaxRatingsKeyBytes, d.MDBListRatingsKey),
		"criticRatingShows":     choice(d.CriticRatingShows, anys(CriticRatingChoices)),
		"tmdbKey":               text(MaxTMDBKeyBytes, d.TMDBKey),
		"waitDigitalRelease":    is(d.WaitDigitalRelease),
		"lyrics":                is(d.Lyrics),
		"evenVolume":            is(d.EvenVolume),
		"measureLoudness":       choice(d.MeasureLoudness, anys(MeasureLoudnessChoices)),
		"playedPercent":         between(MinPlayedPercent, MaxPlayedPercent, d.PlayedPercent),
		"resumePercent":         between(MinResumePercent, MaxResumePercent, d.ResumePercent),
		"versionListMinutes":    between(MinVersionListMinutes, MaxVersionListMinutes, d.VersionListMinutes),
		"catalogRefreshMinutes": between(MinCatalogRefreshMinutes, MaxCatalogRefreshMinutes, d.CatalogRefreshMinutes),

		"twoStepAdministrators": is(d.TwoStepAdministrators),

		"personalAddons":     is(d.PersonalAddons),
		"serverImports":      is(d.ServerImports),
		"shareLinks":         is(d.ShareLinks),
		"loginAttempts":      orZero(MinLoginAttempts, MaxLoginAttempts, d.LoginAttempts),
		"inactiveDeviceDays": between(0, MaxInactiveDeviceDays, d.InactiveDeviceDays),
		"knownProxies":       between(0, MaxKnownProxies, d.KnownProxies),
		"httpsMode":          choice(d.HTTPSMode, anys(HTTPSModes)),
		"httpsPort":          between(1, 65535, d.HTTPSPort),
		"httpsRedirect":      is(d.HTTPSRedirect),
		"httpsCertificate":   text(MaxHTTPSFileBytes, d.HTTPSCertificate),
		"httpsPrivateKey":    text(MaxHTTPSFileBytes, d.HTTPSPrivateKey),
		"acmeHosts":          between(0, MaxACMEHosts, d.ACMEHosts),
		"acmeEmail":          text(MaxEmailBytes, d.ACMEEmail),
		"acmeTerms":          is(d.ACMETerms),
		"acmeChallenge":      choice(d.ACMEChallenge, anys(ACMEChallenges)),
		"detailedLog":        is(d.DetailedLog),

		"analysisTimeout":  between(MinAnalysisTimeout, MaxAnalysisTimeout, d.AnalysisTimeout),
		"versionAttempts":  between(MinVersionAttempts, MaxVersionAttempts, d.VersionAttempts),
		"preferDirectPlay": is(d.PreferDirectPlay),
		// Up to MaxVersionCriteria of them, one of each kind.
		"versionOrder":      choice(d.VersionOrder, anys(VersionCriteria)),
		"versionCodecOrder": choice(d.VersionCodecOrder, anys(VersionCodecs)),
		// The server's sources, each once.
		"versionSourceOrder":   between(0, MaxVersionSources, d.VersionSourceOrder),
		"hideVersionsLargerGb": orZero(1, MaxHideVersionsLargerGB, d.HideVersionsLargerGB),
		"hiddenVersionCodecs":  choice(d.HiddenVersionCodecs, anys(VersionCodecs)),
		"hiddenVersionWords":   between(0, MaxHiddenVersionWords, d.HiddenVersionWords),
		"nameHowVersionsPlay":  is(d.NameHowVersionsPlay),
		"hiddenReleases":       choice(d.HiddenReleases, anys(ReleaseKinds)),
		"syncAddonSubtitles":   is(d.SyncAddonSubtitles),
		"maxConversions":       between(MinMaxConversions, MaxMaxConversions, d.MaxConversions),
		"maxConversionHeight":  choice(d.MaxConversionHeight, anys(ConversionHeights)),
		"convertedDownloads":   is(d.ConvertedDownloads),
		"subtitleOcr":          choice(d.SubtitleOCR, anys(SubtitleOCRModes)),
		"subtitleOcrTracks":    between(MinSubtitleOCRTracks, MaxSubtitleOCRTracks, d.SubtitleOCRTracks),
		// "" or the ISO 639-2/B code of a language (see TrackLanguages).
		"defaultAudioLanguage":    choice(d.DefaultAudioLanguage, anys(TrackLanguages())),
		"defaultSubtitleLanguage": choice(d.DefaultSubtitleLanguage, anys(TrackLanguages())),

		"encoderPreset":              choice(d.EncoderPreset, anys(EncoderPresets)),
		"h264Quality":                orZero(MinVideoQuality, MaxVideoQuality, d.H264Quality),
		"hevcQuality":                orZero(MinVideoQuality, MaxVideoQuality, d.HevcQuality),
		"allowHevcEncoding":          is(d.AllowHevcEncoding),
		"adaptiveQuality":            choice(d.AdaptiveQuality, anys(AdaptiveQualities)),
		"hardwareAcceleration":       choice(d.HardwareAcceleration, anys(HardwareAccelerations)),
		"hardwareDecodingCodecs":     choice(d.HardwareDecodingCodecs, anys(HardwareDecodingCodecs)),
		"toneMapping":                is(d.ToneMapping),
		"toneMappingAlgorithm":       choice(d.ToneMappingAlgorithm, anys(ToneMappingAlgorithms)),
		"toneMappingPeak":            orZero(MinToneMappingPeak, MaxToneMappingPeak, d.ToneMappingPeak),
		"toneMappingDesat":           between(0, MaxToneMappingDesat, d.ToneMappingDesat),
		"gpuToneMapping":             is(d.GPUToneMapping),
		"processorToneMappingHeight": choice(d.ProcessorToneMappingHeight, anys(ToneMappingHeights)),
		"deinterlaceMethod":          choice(d.DeinterlaceMethod, anys(DeinterlaceMethods)),
		"deinterlaceDoubleRate":      is(d.DeinterlaceDoubleRate),
		"downmixAlgorithm":           choice(d.DownmixAlgorithm, anys(DownmixAlgorithms)),
		"downmixBoost":               between(MinDownmixBoost, MaxDownmixBoost, d.DownmixBoost),
		"maxAudioChannels":           choice(d.MaxAudioChannels, anys(AudioChannelLimits)),
		"audioBitratePerChannel":     orZero(MinAudioBitratePerChannel, MaxAudioBitratePerChannel, d.AudioBitratePerChannel),
		"encodingThreads":            between(0, MaxEncodingThreads, d.EncodingThreads),
		"aheadSeconds":               between(MinAheadSeconds, MaxAheadSeconds, d.AheadSeconds),
		"keepConvertedVideo":         is(d.KeepConvertedVideo),

		"trickplay":          is(d.Trickplay),
		"trickplayInterval":  between(MinTrickplayInterval, MaxTrickplayInterval, d.TrickplayInterval),
		"trickplayWidth":     choice(d.TrickplayWidth, anys(TrickplayWidths)),
		"chapterImages":      is(d.ChapterImages),
		"thumbnailStorageGB": between(MinThumbnailStorageGB, MaxThumbnailStorageGB, d.ThumbnailStorageGB),

		"recordingPrePadding":    between(0, MaxRecordingPadding, d.RecordingPrePadding),
		"recordingPostPadding":   between(0, MaxRecordingPadding, d.RecordingPostPadding),
		"recordingRetentionDays": between(0, MaxRecordingRetentionDays, d.RecordingRetentionDays),
		"recordingCommercials":   is(d.RecordingCommercials),
		"liveTvRefreshHours":     between(MinLiveTvRefreshHours, MaxLiveTvRefreshHours, d.LiveTvRefreshHours),
		"localScanHours":         between(0, MaxLocalScanHours, d.LocalScanHours),
		"watchlistCheckHours":    between(0, MaxWatchlistCheckHours, d.WatchlistCheckHours),
		"watchLocalFolders":      is(d.WatchLocalFolders),

		"convertRecordings":            is(d.ConvertRecordings),
		"convertRecordingsFrom":        between(0, 23, d.ConvertRecordingsFrom),
		"convertRecordingsUntil":       between(0, 23, d.ConvertRecordingsUntil),
		"convertRecordingsOnProcessor": is(d.ConvertRecordingsOnProcessor),

		"webTheme":                choice(d.WebTheme, anys(WebThemes)),
		"customCss":               text(MaxCustomCodeBytes, d.CustomCss),
		"customJs":                text(MaxCustomCodeBytes, d.CustomJs),
		"loginDisclaimer":         text(MaxLoginDisclaimerBytes, d.LoginDisclaimer),
		"addVersionsToOpenPage":   is(d.AddVersionsToOpenPage),
		"traktClientId":           text(MaxTrackingAppBytes, d.TraktClientID),
		"traktClientSecret":       text(MaxTrackingAppBytes, d.TraktClientSecret),
		"simklClientId":           text(MaxTrackingAppBytes, d.SimklClientID),
		"lastFmApiKey":            text(MaxTrackingAppBytes, d.LastFMAPIKey),
		"lastFmSecret":            text(MaxTrackingAppBytes, d.LastFMSecret),
		"aniListClientId":         text(MaxTrackingAppBytes, d.AniListClientID),
		"aniListClientSecret":     text(MaxTrackingAppBytes, d.AniListClientSecret),
		"myAnimeListClientId":     text(MaxTrackingAppBytes, d.MyAnimeListClientID),
		"myAnimeListClientSecret": text(MaxTrackingAppBytes, d.MyAnimeListClientSecret),

		"backupHour":            between(0, 23, d.BackupHour),
		"backupsKept":           between(MinBackupsKept, MaxBackupsKept, d.BackupsKept),
		"collectionReadHour":    between(-1, 23, d.CollectionReadHour),
		"collectionLibraryApps": between(0, MaxCollectionLibraryApps, d.CollectionLibraryApps),

		"remuxDb":    is(d.RemuxDB),
		"remuxDbUrl": text(MaxRemuxDBURLBytes, d.RemuxDBURL),

		"cacheSizeGb":      between(MinCacheSizeGB, MaxCacheSizeGB, d.CacheSizeGB),
		"vaapiDevice":      is(d.VAAPIDevice),
		"recording":        is(d.Recording),
		"recordingsFolder": text(MaxFolderBytes, d.RecordingsFolder),
		"backups":          is(d.Backups),
		"backupFolder":     text(MaxFolderBytes, d.BackupFolder),
		"backupPassphrase": orZero(MinBackupPassphraseRunes, MaxBackupPassphraseBytes, d.BackupPassphrase),

		"publicAddress": text(MaxPublicAddressBytes, d.PublicAddress),

		"smtpHost":     text(MaxSMTPHostBytes, d.SMTPHost),
		"smtpPort":     between(1, 65535, d.SMTPPort),
		"smtpSecurity": choice(d.SMTPSecurity, anys(SMTPSecurities)),
		"smtpUser":     text(MaxSMTPUserBytes, d.SMTPUser),
		"smtpPassword": text(MaxSMTPPasswordBytes, d.SMTPPassword),
		"smtpFrom":     text(MaxEmailBytes, d.SMTPFrom),
		"smtpFromName": text(MaxSMTPFromNameBytes, d.SMTPFromName),

		"playbackHistory":     is(d.PlaybackHistory),
		"playbackHistoryDays": between(1, MaxPlaybackHistoryDays, d.PlaybackHistoryDays),

		"weeklySummaryDay":  between(0, 6, d.WeeklySummaryDay),
		"weeklySummaryHour": between(0, 23, d.WeeklySummaryHour),

		"importEveryMinutes": choice(d.ImportEveryMinutes, anys(ImportEveryChoices)),

		"metrics":      is(d.Metrics),
		"metricsToken": text(MaxMetricsTokenBytes, d.MetricsToken),

		"restartConfirmed": is(d.RestartConfirmed),
		"updateToolUrl":    text(MaxUpdateToolURLBytes, d.UpdateToolURL),
		"updateToolToken":  text(MaxUpdateToolTokenBytes, d.UpdateToolToken),

		"torrentStreams":       is(d.TorrentStreams),
		"torrentInterface":     text(MaxTorrentInterfaceName, d.TorrentInterface),
		"torrentProxy":         text(MaxTorrentProxyBytes, d.TorrentProxy),
		"torrentProxyUser":     text(MaxTorrentProxyBytes, d.TorrentProxyUser),
		"torrentProxyPassword": text(MaxTorrentProxyBytes, d.TorrentProxyPassword),
		"torrentWireGuard":     text(torrent.MaxWireGuardConfigBytes, d.TorrentWireGuard),
		"torrentDownloadMbps":  between(0, MaxTorrentMbps, d.TorrentDownloadMbps),
		"torrentUploadMbps":    between(0, MaxTorrentMbps, d.TorrentUploadMbps),
		"torrentsAtOnce":       between(0, MaxTorrentsAtOnce, d.TorrentsAtOnce),
		"torrentTrackers":      between(0, MaxTorrentTrackers, d.TorrentTrackers),

		"conversionFolder": text(MaxFolderBytes, d.ConversionFolder),
		"readingFolder":    text(MaxFolderBytes, d.ReadingFolder),
		"cacheFolder":      text(MaxFolderBytes, d.CacheFolder),
		"siloApps":         is(d.SiloApps),

		"posterRatings":          is(d.PosterRatings),
		"posterBadges":           is(d.PosterBadges),
		"posterBadgeSize":        choice(d.PosterBadgeSize, anys(PosterBadgeSizes)),
		"posterTemplate":         text(MaxPosterTemplateBytes, d.PosterTemplate),
		"providerIssuer":         text(MaxProviderIssuerBytes, d.ProviderIssuer),
		"providerName":           text(MaxProviderNameBytes, d.ProviderName),
		"providerClientId":       text(MaxProviderClientBytes, d.ProviderClientID),
		"providerClientSecret":   text(MaxProviderClientBytes, d.ProviderClientSecret),
		"providerUserClaim":      between(1, MaxProviderClaimBytes, d.ProviderUserClaim),
		"providerCreateAccounts": is(d.ProviderCreateAccounts),
		"providerAdminClaim":     text(MaxProviderClaimBytes, d.ProviderAdminClaim),
		"providerAdminValue":     text(MaxProviderValueBytes, d.ProviderAdminValue),
	}
}

// anys returns values as a list of any, as SettingBounds.Choices holds
// them.
func anys[T any](values []T) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}
