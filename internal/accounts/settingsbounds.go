package accounts

import "slices"

// DefaultSettings are the settings of a new server, those the columns of
// the settings default to: changing a default takes this and a migration
// setting the column's.
func DefaultSettings() Settings {
	return Settings{
		ServerName:          "Polyfin",
		QuickConnectEnabled: true,
		LegacyAuthorization: false,
		Language:            Languages[0],
		PrepareAhead:        true,
		Transcoding:         true,
		CatalogLimit:        DefaultCatalogLimit,
		ChannelLimit:        DefaultChannelLimit,

		SkipButtons:           true,
		SegmentOrder:          slices.Clone(SegmentSources),
		SegmentSourcesOff:     []string{},
		SimilarTitles:         true,
		PlayedPercent:         DefaultPlayedPercent,
		ResumePercent:         DefaultResumePercent,
		VersionListMinutes:    DefaultVersionListMinutes,
		CatalogRefreshMinutes: DefaultCatalogRefreshMinutes,

		PersonalAddons:     true,
		LoginAttempts:      DefaultLoginAttempts,
		InactiveDeviceDays: DefaultInactiveDeviceDays,
		DetailedLog:        false,

		AnalysisTimeout:     DefaultAnalysisTimeout,
		VersionAttempts:     DefaultVersionAttempts,
		PreferDirectPlay:    false,
		MaxConversions:      DefaultMaxConversions,
		MaxConversionHeight: ConversionHeights[0],

		EncoderPreset:          EncoderPresets[0],
		AllowHevcEncoding:      false,
		HardwareAcceleration:   HardwareAccelerations[0],
		HardwareDecodingCodecs: slices.Clone(HardwareDecodingCodecs),
		ToneMapping:            true,
		ToneMappingAlgorithm:   ToneMappingAlgorithms[0],
		DeinterlaceMethod:      DeinterlaceMethods[0],
		DeinterlaceDoubleRate:  false,
		DownmixAlgorithm:       DownmixAlgorithms[0],
		DownmixBoost:           DefaultDownmixBoost,
		MaxAudioChannels:       AudioChannelLimits[0],
		AheadSeconds:           DefaultAheadSeconds,

		Trickplay:          false,
		TrickplayInterval:  DefaultTrickplayInterval,
		TrickplayWidth:     DefaultTrickplayWidth,
		ChapterImages:      false,
		ThumbnailStorageGB: DefaultThumbnailStorageGB,

		RecordingPrePadding:    DefaultRecordingPrePadding,
		RecordingPostPadding:   DefaultRecordingPostPadding,
		RecordingRetentionDays: DefaultRecordingRetentionDays,
		LiveTvRefreshHours:     DefaultLiveTvRefreshHours,

		BackupHour:         DefaultBackupHour,
		BackupsKept:        DefaultBackupsKept,
		CollectionReadHour: DefaultCollectionReadHour,
	}
}

// SettingBounds are what a setting accepts, and its default.
type SettingBounds struct {
	// Default is the setting's value on a new server.
	Default any
	// Min and Max bound a number, or the length of a text: in characters
	// for the server name, in bytes for the others. Bounded tells whether
	// they apply.
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
		"language":            choice(d.Language, anys(Languages)),
		"prepareAhead":        is(d.PrepareAhead),
		"transcoding":         is(d.Transcoding),
		"catalogLimit":        between(MinCatalogLimit, MaxCatalogLimit, d.CatalogLimit),
		"channelLimit":        between(MinChannelLimit, MaxChannelLimit, d.ChannelLimit),

		"skipButtons":           is(d.SkipButtons),
		"publicMetaDbKey":       text(MaxSegmentKeyBytes, d.PublicMetaDBKey),
		"theIntroDbKey":         text(MaxSegmentKeyBytes, d.TheIntroDBKey),
		"segmentOrder":          choice(d.SegmentOrder, anys(SegmentSources)),
		"segmentSourcesOff":     choice(d.SegmentSourcesOff, anys(SegmentSources)),
		"similarTitles":         is(d.SimilarTitles),
		"playedPercent":         between(MinPlayedPercent, MaxPlayedPercent, d.PlayedPercent),
		"resumePercent":         between(MinResumePercent, MaxResumePercent, d.ResumePercent),
		"versionListMinutes":    between(MinVersionListMinutes, MaxVersionListMinutes, d.VersionListMinutes),
		"catalogRefreshMinutes": between(MinCatalogRefreshMinutes, MaxCatalogRefreshMinutes, d.CatalogRefreshMinutes),

		"personalAddons":     is(d.PersonalAddons),
		"loginAttempts":      orZero(MinLoginAttempts, MaxLoginAttempts, d.LoginAttempts),
		"inactiveDeviceDays": between(0, MaxInactiveDeviceDays, d.InactiveDeviceDays),
		"detailedLog":        is(d.DetailedLog),

		"analysisTimeout":     between(MinAnalysisTimeout, MaxAnalysisTimeout, d.AnalysisTimeout),
		"versionAttempts":     between(MinVersionAttempts, MaxVersionAttempts, d.VersionAttempts),
		"preferDirectPlay":    is(d.PreferDirectPlay),
		"maxConversions":      between(MinMaxConversions, MaxMaxConversions, d.MaxConversions),
		"maxConversionHeight": choice(d.MaxConversionHeight, anys(ConversionHeights)),

		"encoderPreset":          choice(d.EncoderPreset, anys(EncoderPresets)),
		"h264Quality":            orZero(MinVideoQuality, MaxVideoQuality, d.H264Quality),
		"hevcQuality":            orZero(MinVideoQuality, MaxVideoQuality, d.HevcQuality),
		"allowHevcEncoding":      is(d.AllowHevcEncoding),
		"hardwareAcceleration":   choice(d.HardwareAcceleration, anys(HardwareAccelerations)),
		"hardwareDecodingCodecs": choice(d.HardwareDecodingCodecs, anys(HardwareDecodingCodecs)),
		"toneMapping":            is(d.ToneMapping),
		"toneMappingAlgorithm":   choice(d.ToneMappingAlgorithm, anys(ToneMappingAlgorithms)),
		"toneMappingPeak":        orZero(MinToneMappingPeak, MaxToneMappingPeak, d.ToneMappingPeak),
		"toneMappingDesat":       between(0, MaxToneMappingDesat, d.ToneMappingDesat),
		"deinterlaceMethod":      choice(d.DeinterlaceMethod, anys(DeinterlaceMethods)),
		"deinterlaceDoubleRate":  is(d.DeinterlaceDoubleRate),
		"downmixAlgorithm":       choice(d.DownmixAlgorithm, anys(DownmixAlgorithms)),
		"downmixBoost":           between(MinDownmixBoost, MaxDownmixBoost, d.DownmixBoost),
		"maxAudioChannels":       choice(d.MaxAudioChannels, anys(AudioChannelLimits)),
		"audioBitratePerChannel": orZero(MinAudioBitratePerChannel, MaxAudioBitratePerChannel, d.AudioBitratePerChannel),
		"encodingThreads":        between(0, MaxEncodingThreads, d.EncodingThreads),
		"aheadSeconds":           between(MinAheadSeconds, MaxAheadSeconds, d.AheadSeconds),

		"trickplay":          is(d.Trickplay),
		"trickplayInterval":  between(MinTrickplayInterval, MaxTrickplayInterval, d.TrickplayInterval),
		"trickplayWidth":     choice(d.TrickplayWidth, anys(TrickplayWidths)),
		"chapterImages":      is(d.ChapterImages),
		"thumbnailStorageGB": between(MinThumbnailStorageGB, MaxThumbnailStorageGB, d.ThumbnailStorageGB),

		"recordingPrePadding":    between(0, MaxRecordingPadding, d.RecordingPrePadding),
		"recordingPostPadding":   between(0, MaxRecordingPadding, d.RecordingPostPadding),
		"recordingRetentionDays": between(0, MaxRecordingRetentionDays, d.RecordingRetentionDays),
		"liveTvRefreshHours":     between(MinLiveTvRefreshHours, MaxLiveTvRefreshHours, d.LiveTvRefreshHours),

		"customCss":         text(MaxCustomCodeBytes, d.CustomCss),
		"customJs":          text(MaxCustomCodeBytes, d.CustomJs),
		"loginDisclaimer":   text(MaxLoginDisclaimerBytes, d.LoginDisclaimer),
		"traktClientId":     text(MaxTrackingAppBytes, d.TraktClientID),
		"traktClientSecret": text(MaxTrackingAppBytes, d.TraktClientSecret),
		"simklClientId":     text(MaxTrackingAppBytes, d.SimklClientID),

		"backupHour":         between(0, 23, d.BackupHour),
		"backupsKept":        between(MinBackupsKept, MaxBackupsKept, d.BackupsKept),
		"collectionReadHour": between(-1, 23, d.CollectionReadHour),
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
