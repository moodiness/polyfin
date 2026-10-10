package admin

import (
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

type userJSON struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	IsAdministrator bool                `json:"isAdministrator"`
	IsHidden        bool                `json:"isHidden"`
	IsDisabled      bool                `json:"isDisabled"`
	CreatedAt       time.Time           `json:"createdAt"`
	LastLoginAt     *time.Time          `json:"lastLoginAt"`
	LastActivityAt  *time.Time          `json:"lastActivityAt"`
	ParentalControl parentalControlJSON `json:"parentalControl"`
	// Transcoding is set when the user may have both video and audio
	// converted; Downloads, when they may download.
	Transcoding bool `json:"transcoding"`
	Downloads   bool `json:"downloads"`
	// PersonalAddons is set when the user may add and use their own addons.
	PersonalAddons bool `json:"personalAddons"`
	// BlockedUntil is when the block of the account for wrong passwords
	// ends, null when it is not blocked.
	BlockedUntil *time.Time `json:"blockedUntil"`
	// MaxPlaybacks is how many of the user's other devices may be playing
	// when one more starts, 0 for no limit; MaxBitrate, the highest bitrate
	// of their playback in bits per second, 0 for no limit. LiveTv lets them
	// watch Live TV; SyncPlay is CreateAndJoinGroups, JoinGroups or None;
	// RemoteControl lets them control other users' apps.
	MaxPlaybacks  int    `json:"maxPlaybacks"`
	MaxBitrate    int    `json:"maxBitrate"`
	LiveTv        bool   `json:"liveTv"`
	SyncPlay      string `json:"syncPlay"`
	RemoteControl bool   `json:"remoteControl"`
	// HiddenLibraries are the identifiers of the server's libraries the
	// user's apps do not show; BlockedGenres, the genres whose titles are
	// hidden; AccessSchedules, the hours the user may use the server in.
	HiddenLibraries []string             `json:"hiddenLibraries"`
	BlockedGenres   []string             `json:"blockedGenres"`
	AccessSchedules []accessScheduleJSON `json:"accessSchedules"`
	// CollectionManagement lets the user create, change and delete the
	// collections every user sees.
	CollectionManagement bool `json:"collectionManagement"`
	// LiveTvManagement lets the user schedule and delete Live TV
	// recordings.
	LiveTvManagement bool `json:"liveTvManagement"`
	// PasswordResetPin is the PIN the user asked for from a Jellyfin app's
	// forgotten password screen, while it is valid, null otherwise: their
	// password becomes the PIN once they enter it.
	PasswordResetPin *passwordResetPinJSON `json:"passwordResetPin"`
	// SubtitleManagement lets the user add subtitle files to titles from
	// their apps; ImageTag identifies their profile picture, served at
	// /UserImage, null without one.
	SubtitleManagement bool    `json:"subtitleManagement"`
	ImageTag           *string `json:"imageTag"`
	// QualityGroup is the tallest video the user is offered, in lines: 480,
	// 720, 1080, 1440 or 2160, or 0 for Original, no limit.
	QualityGroup int `json:"qualityGroup"`
}

type passwordResetPinJSON struct {
	Pin       string    `json:"pin"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// withPins adds to users the password reset PINs still valid.
func (h *handler) withPins(r *http.Request, users ...userJSON) ([]userJSON, error) {
	pins, err := h.Accounts.PasswordResetPINs(r.Context())
	if err != nil {
		return nil, err
	}
	for i, user := range users {
		id, _ := accounts.ParseID(user.ID)
		if pin, ok := pins[id]; ok {
			users[i].PasswordResetPin = &passwordResetPinJSON{Pin: pin.PIN, ExpiresAt: pin.ExpiresAt}
		}
	}
	return users, nil
}

// writeUser answers with user and their password reset PIN.
func (h *handler) writeUser(w http.ResponseWriter, r *http.Request, status int, user accounts.User) {
	result, err := h.withPins(r, newUserJSON(user))
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, result[0])
}

// accessScheduleJSON is a span of hours on a day, one of Jellyfin's
// DynamicDayOfWeek names (accounts.ScheduleDays).
type accessScheduleJSON struct {
	Day       string  `json:"day"`
	StartHour float64 `json:"startHour"`
	EndHour   float64 `json:"endHour"`
}

// parentalControlJSON is a user's parental control: the highest rating
// score (and subscore at that score) the user may reach, null for no
// limit, and the kinds of items (Jellyfin's UnratedItem names) hidden when
// unrated.
type parentalControlJSON struct {
	MaxRating    *int     `json:"maxRating"`
	MaxSubRating *int     `json:"maxSubRating"`
	BlockUnrated []string `json:"blockUnrated"`
}

func newUserJSON(user accounts.User) userJSON {
	return userJSON{
		ID:              user.ID.String(),
		Name:            user.Name,
		IsAdministrator: user.IsAdministrator,
		IsHidden:        user.IsHidden,
		IsDisabled:      user.IsDisabled,
		CreatedAt:       user.CreatedAt,
		LastLoginAt:     user.LastLoginAt,
		LastActivityAt:  user.LastActivityAt,
		ParentalControl: parentalControlJSON{
			MaxRating:    user.Parental.MaxRating,
			MaxSubRating: user.Parental.MaxSubRating,
			BlockUnrated: append([]string{}, user.Parental.BlockUnrated...),
		},
		Transcoding: user.VideoTranscoding && user.AudioTranscoding,
		Downloads:   user.ContentDownloading,
		// Whether the user may have their own addons, and their block.
		PersonalAddons: user.PersonalAddons,
		BlockedUntil:   blockedUntil(user),
		// The user's playback and access limits.
		MaxPlaybacks:  user.MaxPlaybacks,
		MaxBitrate:    user.MaxBitrate,
		LiveTv:        user.LiveTv,
		SyncPlay:      string(user.SyncPlay),
		RemoteControl: user.RemoteControl,
		// The user's content settings.
		HiddenLibraries: idStrings(user.HiddenLibraries),
		BlockedGenres:   append([]string{}, user.BlockedGenres...),
		AccessSchedules: schedulesJSON(user.AccessSchedules),
		// The user's permission to manage collections.
		CollectionManagement: user.CollectionManagement,
		// Subtitles and the profile picture.
		SubtitleManagement: user.SubtitleManagement,
		ImageTag:           imageTag(user),
		// Whether the user may schedule and delete recordings.
		LiveTvManagement: user.LiveTvManagement,
		// The tallest video the user is offered.
		QualityGroup: user.QualityGroup,
	}
}

// imageTag is the tag of user's profile picture, nil without one.
func imageTag(user accounts.User) *string {
	if user.ImageTag == "" {
		return nil
	}
	return &user.ImageTag
}

// blockedUntil is when the block of user's account ends, nil when it is not
// blocked.
func blockedUntil(user accounts.User) *time.Time {
	if !user.Blocked(time.Now()) {
		return nil
	}
	return user.BlockedUntil
}

func idStrings(ids []accounts.ID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, id.String())
	}
	return result
}

func schedulesJSON(schedules []accounts.AccessSchedule) []accessScheduleJSON {
	result := make([]accessScheduleJSON, 0, len(schedules))
	for _, s := range schedules {
		result = append(result, accessScheduleJSON{Day: s.Day, StartHour: s.StartHour, EndHour: s.EndHour})
	}
	return result
}

// libraryChoiceJSON is one of the server's libraries a user may see.
type libraryChoiceJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userContentChoices lists what the admin app offers for a user's content:
// the server's libraries, in order, and the genres the server's libraries
// can be narrowed to, which name their genre pages, sorted.
func (h *handler) userContentChoices(w http.ResponseWriter, r *http.Request) {
	libraries, err := library.ServerLibraries(r.Context(), h.Addons, h.Accounts.Settings().Language)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Libraries []libraryChoiceJSON `json:"libraries"`
		Genres    []string            `json:"genres"`
	}{Libraries: []libraryChoiceJSON{}, Genres: []string{}}
	for _, l := range libraries {
		result.Libraries = append(result.Libraries, libraryChoiceJSON{ID: l.ID.String(), Name: l.Name})
		for _, genre := range l.Genres {
			genre = strings.TrimSpace(genre)
			if genre != "" && !slices.ContainsFunc(result.Genres, func(known string) bool { return strings.EqualFold(known, genre) }) {
				result.Genres = append(result.Genres, genre)
			}
		}
	}
	slices.SortFunc(result.Genres, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	writeJSON(w, http.StatusOK, result)
}

// ratingJSON is a rating the admin app offers as a user's limit.
type ratingJSON struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	SubScore *int   `json:"subScore"`
}

// parentalRatings lists the ratings a user's limit can be set to, those
// Jellyfin apps offer, without the entry for unrated titles: blocking them
// is a choice of its own.
func (h *handler) parentalRatings(w http.ResponseWriter, _ *http.Request) {
	var result []ratingJSON
	for _, rating := range localization.Ratings() {
		if rating.Score != nil {
			result = append(result, ratingJSON{Name: rating.Name, Score: rating.Score.Score, SubScore: rating.Score.SubScore})
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type deviceJSON struct {
	ID             string    `json:"id"`
	DeviceName     string    `json:"deviceName"`
	Client         string    `json:"client"`
	ClientVersion  string    `json:"clientVersion"`
	RemoteAddress  string    `json:"remoteAddress"`
	CreatedAt      time.Time `json:"createdAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
}

// settingsJSON are the settings the admin interface reads and saves. A
// save leaving a setting out keeps its current value, so that a page or
// script older than it leaves it alone; one sending a setting that no
// longer exists, such as chapters or downloads, is not refused.
type settingsJSON struct {
	ServerName          string `json:"serverName"`
	QuickConnectEnabled bool   `json:"quickConnectEnabled"`
	LegacyAuthorization bool   `json:"legacyAuthorization"`
	Language            string `json:"language"`
	PrepareAhead        *bool  `json:"prepareAhead"`
	Transcoding         *bool  `json:"transcoding"`
	CatalogLimit        *int   `json:"catalogLimit"`
	ChannelLimit        *int   `json:"channelLimit"`
	UpdateCheck         *bool  `json:"updateCheck"`
	// The content settings keep their current values too when a PUT
	// leaves them out.
	SkipButtons *bool `json:"skipButtons"`
	// PublicMetaDBKeySet tells whether a PublicMetaDB key is saved; a PUT
	// ignores it. PublicMetaDBKey, which no answer holds, replaces the key
	// once PublicMetaDB accepts it; empty removes it, and a PUT leaving it
	// out keeps it.
	PublicMetaDBKeySet bool    `json:"publicMetaDbKeySet"`
	PublicMetaDBKey    *string `json:"publicMetaDbKey,omitempty"`
	// TheIntroDBKeySet and TheIntroDBKey are the same for TheIntroDB's
	// optional key.
	TheIntroDBKeySet bool    `json:"theIntroDbKeySet"`
	TheIntroDBKey    *string `json:"theIntroDbKey,omitempty"`
	// SegmentOrder is the order of preference of the segment databases,
	// every one of them once, and SegmentSourcesOff those never asked. A
	// PUT leaving either out keeps it; an empty order, which older pages
	// sent to follow POLYFIN_SEGMENTS, keeps it too.
	SegmentOrder          []string  `json:"segmentOrder"`
	SegmentSourcesOff     *[]string `json:"segmentSourcesOff"`
	SimilarTitles         *bool     `json:"similarTitles"`
	Lyrics                *bool     `json:"lyrics"`
	PlayedPercent         *int      `json:"playedPercent"`
	ResumePercent         *int      `json:"resumePercent"`
	VersionListMinutes    *int      `json:"versionListMinutes"`
	CatalogRefreshMinutes *int      `json:"catalogRefreshMinutes"`
	// The security settings keep their current values when a PUT leaves
	// them out, too.
	PersonalAddons     *bool `json:"personalAddons"`
	ServerImports      *bool `json:"serverImports"`
	LoginAttempts      *int  `json:"loginAttempts"`
	InactiveDeviceDays *int  `json:"inactiveDeviceDays"`
	DetailedLog        *bool `json:"detailedLog"`
	// AnalysisTimeout, VersionAttempts, PreferDirectPlay, MaxConversions
	// and MaxConversionHeight keep their current values when a PUT leaves
	// them out.
	AnalysisTimeout     *int  `json:"analysisTimeout"`
	VersionAttempts     *int  `json:"versionAttempts"`
	PreferDirectPlay    *bool `json:"preferDirectPlay"`
	MaxConversions      *int  `json:"maxConversions"`
	MaxConversionHeight *int  `json:"maxConversionHeight"`
	// The settings tuning conversions keep their current values when a
	// PUT leaves them out. ConversionHardware is what conversions run on;
	// a PUT cannot change it.
	EncoderPreset          *string   `json:"encoderPreset"`
	H264Quality            *int      `json:"h264Quality"`
	HevcQuality            *int      `json:"hevcQuality"`
	AllowHevcEncoding      *bool     `json:"allowHevcEncoding"`
	HardwareAcceleration   *string   `json:"hardwareAcceleration"`
	HardwareDecodingCodecs *[]string `json:"hardwareDecodingCodecs"`
	ToneMapping            *bool     `json:"toneMapping"`
	ToneMappingAlgorithm   *string   `json:"toneMappingAlgorithm"`
	ToneMappingPeak        *int      `json:"toneMappingPeak"`
	ToneMappingDesat       *float64  `json:"toneMappingDesat"`
	GPUToneMapping         *bool     `json:"gpuToneMapping"`
	// ProcessorToneMappingHeight is 0 for Automatic: ConversionHardware
	// tells the height a timing at startup chose.
	ProcessorToneMappingHeight *int                   `json:"processorToneMappingHeight"`
	DeinterlaceMethod          *string                `json:"deinterlaceMethod"`
	DeinterlaceDoubleRate      *bool                  `json:"deinterlaceDoubleRate"`
	DownmixAlgorithm           *string                `json:"downmixAlgorithm"`
	DownmixBoost               *float64               `json:"downmixBoost"`
	MaxAudioChannels           *int                   `json:"maxAudioChannels"`
	AudioBitratePerChannel     *int                   `json:"audioBitratePerChannel"`
	EncodingThreads            *int                   `json:"encodingThreads"`
	AheadSeconds               *int                   `json:"aheadSeconds"`
	ConversionHardware         conversionHardwareJSON `json:"conversionHardware"`
	// The thumbnail settings keep their current values when a PUT leaves
	// them out.
	Trickplay          *bool `json:"trickplay"`
	TrickplayInterval  *int  `json:"trickplayInterval"`
	TrickplayWidth     *int  `json:"trickplayWidth"`
	ChapterImages      *bool `json:"chapterImages"`
	ThumbnailStorageGB *int  `json:"thumbnailStorageGB"`
	// RecordingPrePadding and RecordingPostPadding, in seconds, and
	// RecordingRetentionDays keep their current values when a PUT leaves
	// them out.
	RecordingPrePadding    *int `json:"recordingPrePadding"`
	RecordingPostPadding   *int `json:"recordingPostPadding"`
	RecordingRetentionDays *int `json:"recordingRetentionDays"`
	// LiveTvRefreshHours keeps its current value when a PUT leaves it out.
	LiveTvRefreshHours *int `json:"liveTvRefreshHours"`
	// LocalScanHours keeps its current value when a PUT leaves it out.
	LocalScanHours *int `json:"localScanHours"`
	// CustomCss, CustomJs and LoginDisclaimer, for jellyfin-web, keep their
	// current values when a PUT leaves them out; empty clears them.
	CustomCss       *string `json:"customCss"`
	CustomJs        *string `json:"customJs"`
	LoginDisclaimer *string `json:"loginDisclaimer"`
	// TraktClientID, TraktClientSecret and SimklClientID keep their current
	// values when a PUT leaves them out; empty removes them. The secret is
	// never sent back: TraktClientSecretSet tells whether one is saved, and
	// a PUT ignores it.
	TraktClientID        *string `json:"traktClientId"`
	TraktClientSecret    *string `json:"traktClientSecret,omitempty"`
	TraktClientSecretSet bool    `json:"traktClientSecretSet"`
	SimklClientID        *string `json:"simklClientId"`
	// LastFMAPIKey and LastFMSecret, the Last.fm API account, are the same:
	// LastFMSecretSet tells whether a shared secret is saved.
	LastFMAPIKey    *string `json:"lastFmApiKey"`
	LastFMSecret    *string `json:"lastFmSecret,omitempty"`
	LastFMSecretSet bool    `json:"lastFmSecretSet"`
	// BackupHour, BackupsKept and CollectionReadHour keep their current
	// values when a PUT leaves them out.
	BackupHour         *int `json:"backupHour"`
	BackupsKept        *int `json:"backupsKept"`
	CollectionReadHour *int `json:"collectionReadHour"`
	// RemuxDB and RemuxDBURL keep their current values when a PUT leaves
	// them out.
	RemuxDB    *bool   `json:"remuxDb"`
	RemuxDBURL *string `json:"remuxDbUrl"`
	// CacheSizeGB, VAAPIDevice, Recording, RecordingsFolder, Backups and
	// BackupFolder keep their current values when a PUT leaves them out;
	// an empty folder is the default one. RecordingsFolderDefault and
	// BackupFolderDefault are the default folders, under the data folder,
	// and RenderNodes the render nodes the server has, sorted, found at
	// each request; a PUT ignores them.
	CacheSizeGB             *int     `json:"cacheSizeGb"`
	VAAPIDevice             *string  `json:"vaapiDevice"`
	Recording               *bool    `json:"recording"`
	RecordingsFolder        *string  `json:"recordingsFolder"`
	Backups                 *bool    `json:"backups"`
	BackupFolder            *string  `json:"backupFolder"`
	RecordingsFolderDefault string   `json:"recordingsFolderDefault"`
	BackupFolderDefault     string   `json:"backupFolderDefault"`
	RenderNodes             []string `json:"renderNodes"`
	// PublicAddress keeps its current value when a PUT leaves it out; empty
	// sends notifications without links.
	PublicAddress *string `json:"publicAddress"`
	// SMTPHost, SMTPPort, SMTPSecurity, SMTPUser, SMTPFrom and SMTPFromName,
	// the SMTP server email notifications go through, keep their current
	// values when a PUT leaves them out; an empty host offers no email
	// target. The password is the same, but never sent back:
	// SMTPPasswordSet tells whether one is saved, and a PUT ignores it.
	SMTPHost        *string `json:"smtpHost"`
	SMTPPort        *int    `json:"smtpPort"`
	SMTPSecurity    *string `json:"smtpSecurity"`
	SMTPUser        *string `json:"smtpUser"`
	SMTPPassword    *string `json:"smtpPassword,omitempty"`
	SMTPPasswordSet bool    `json:"smtpPasswordSet"`
	SMTPFrom        *string `json:"smtpFrom"`
	SMTPFromName    *string `json:"smtpFromName"`
	// PlaybackHistory and PlaybackHistoryDays, whether the videos played
	// are kept for the statistics and for how many days, keep their current
	// values when a PUT leaves them out.
	PlaybackHistory     *bool `json:"playbackHistory"`
	PlaybackHistoryDays *int  `json:"playbackHistoryDays"`
	// Bounds are what each setting accepts, and its default, by its name
	// here; a PUT ignores them.
	Bounds map[string]settingBoundsJSON `json:"bounds,omitempty"`
}

// settingBoundsJSON is what a setting accepts, and its default (see
// accounts.SettingBounds): min and max for a number, or the length of a
// text, zero when 0 is accepted below min, and choices for a setting taking
// one of them, or a list holding some.
type settingBoundsJSON struct {
	Default any      `json:"default"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Zero    bool     `json:"zero,omitempty"`
	Choices []any    `json:"choices,omitempty"`
}

// settingsBounds are accounts.SettingsBounds as the admin API sends them.
var settingsBounds = func() map[string]settingBoundsJSON {
	result := map[string]settingBoundsJSON{}
	for name, bounds := range accounts.SettingsBounds() {
		entry := settingBoundsJSON{Default: bounds.Default, Zero: bounds.Zero, Choices: bounds.Choices}
		if bounds.Bounded {
			entry.Min, entry.Max = &bounds.Min, &bounds.Max
		}
		result[name] = entry
	}
	return result
}()

func newSettingsJSON(settings accounts.Settings) settingsJSON {
	return settingsJSON{
		ServerName:          settings.ServerName,
		QuickConnectEnabled: settings.QuickConnectEnabled,
		LegacyAuthorization: settings.LegacyAuthorization,
		Language:            settings.Language,
		PrepareAhead:        &settings.PrepareAhead,
		Transcoding:         &settings.Transcoding,
		CatalogLimit:        &settings.CatalogLimit,
		ChannelLimit:        &settings.ChannelLimit,
		UpdateCheck:         &settings.UpdateCheck,

		SkipButtons:           &settings.SkipButtons,
		PublicMetaDBKeySet:    settings.PublicMetaDBKey != "",
		TheIntroDBKeySet:      settings.TheIntroDBKey != "",
		SegmentOrder:          settings.SegmentOrder,
		SegmentSourcesOff:     &settings.SegmentSourcesOff,
		SimilarTitles:         &settings.SimilarTitles,
		Lyrics:                &settings.Lyrics,
		PlayedPercent:         &settings.PlayedPercent,
		ResumePercent:         &settings.ResumePercent,
		VersionListMinutes:    &settings.VersionListMinutes,
		CatalogRefreshMinutes: &settings.CatalogRefreshMinutes,
		PersonalAddons:        &settings.PersonalAddons,
		ServerImports:         &settings.ServerImports,
		LoginAttempts:         &settings.LoginAttempts,
		InactiveDeviceDays:    &settings.InactiveDeviceDays,
		DetailedLog:           &settings.DetailedLog,
		AnalysisTimeout:       &settings.AnalysisTimeout,
		VersionAttempts:       &settings.VersionAttempts,
		PreferDirectPlay:      &settings.PreferDirectPlay,
		MaxConversions:        &settings.MaxConversions,
		MaxConversionHeight:   &settings.MaxConversionHeight,

		EncoderPreset:              &settings.EncoderPreset,
		H264Quality:                &settings.H264Quality,
		HevcQuality:                &settings.HevcQuality,
		AllowHevcEncoding:          &settings.AllowHevcEncoding,
		HardwareAcceleration:       &settings.HardwareAcceleration,
		HardwareDecodingCodecs:     &settings.HardwareDecodingCodecs,
		ToneMapping:                &settings.ToneMapping,
		ToneMappingAlgorithm:       &settings.ToneMappingAlgorithm,
		ToneMappingPeak:            &settings.ToneMappingPeak,
		ToneMappingDesat:           &settings.ToneMappingDesat,
		GPUToneMapping:             &settings.GPUToneMapping,
		ProcessorToneMappingHeight: &settings.ProcessorToneMappingHeight,
		DeinterlaceMethod:          &settings.DeinterlaceMethod,
		DeinterlaceDoubleRate:      &settings.DeinterlaceDoubleRate,
		DownmixAlgorithm:           &settings.DownmixAlgorithm,
		DownmixBoost:               &settings.DownmixBoost,
		MaxAudioChannels:           &settings.MaxAudioChannels,
		AudioBitratePerChannel:     &settings.AudioBitratePerChannel,
		EncodingThreads:            &settings.EncodingThreads,
		AheadSeconds:               &settings.AheadSeconds,

		Trickplay:          &settings.Trickplay,
		TrickplayInterval:  &settings.TrickplayInterval,
		TrickplayWidth:     &settings.TrickplayWidth,
		ChapterImages:      &settings.ChapterImages,
		ThumbnailStorageGB: &settings.ThumbnailStorageGB,

		RecordingPrePadding:    &settings.RecordingPrePadding,
		RecordingPostPadding:   &settings.RecordingPostPadding,
		RecordingRetentionDays: &settings.RecordingRetentionDays,
		LiveTvRefreshHours:     &settings.LiveTvRefreshHours,
		LocalScanHours:         &settings.LocalScanHours,

		CustomCss:       &settings.CustomCss,
		CustomJs:        &settings.CustomJs,
		LoginDisclaimer: &settings.LoginDisclaimer,

		TraktClientID:        &settings.TraktClientID,
		TraktClientSecretSet: settings.TraktClientSecret != "",
		SimklClientID:        &settings.SimklClientID,
		LastFMAPIKey:         &settings.LastFMAPIKey,
		LastFMSecretSet:      settings.LastFMSecret != "",

		BackupHour:         &settings.BackupHour,
		BackupsKept:        &settings.BackupsKept,
		CollectionReadHour: &settings.CollectionReadHour,

		RemuxDB:    &settings.RemuxDB,
		RemuxDBURL: &settings.RemuxDBURL,

		CacheSizeGB:      &settings.CacheSizeGB,
		VAAPIDevice:      &settings.VAAPIDevice,
		Recording:        &settings.Recording,
		RecordingsFolder: &settings.RecordingsFolder,
		Backups:          &settings.Backups,
		BackupFolder:     &settings.BackupFolder,

		PublicAddress: &settings.PublicAddress,

		SMTPHost:        &settings.SMTPHost,
		SMTPPort:        &settings.SMTPPort,
		SMTPSecurity:    &settings.SMTPSecurity,
		SMTPUser:        &settings.SMTPUser,
		SMTPPasswordSet: settings.SMTPPassword != "",
		SMTPFrom:        &settings.SMTPFrom,
		SMTPFromName:    &settings.SMTPFromName,

		PlaybackHistory:     &settings.PlaybackHistory,
		PlaybackHistoryDays: &settings.PlaybackHistoryDays,
	}
}

// pathID parses an identifier path value, answering 404 when it is malformed.
func pathID(w http.ResponseWriter, r *http.Request, name string) (accounts.ID, bool) {
	id, err := accounts.ParseID(r.PathValue(name))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return id, false
	}
	return id, true
}

func (h *handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	err := h.Accounts.ChangePassword(r.Context(), session.User.ID, body.CurrentPassword, body.NewPassword, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.PasswordChanged(r.Context(), session.User)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) writeDevices(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	devices, err := h.Accounts.Devices(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]deviceJSON, 0, len(devices))
	for _, device := range devices {
		result = append(result, deviceJSON{
			ID:             device.ID.String(),
			DeviceName:     device.DeviceName,
			Client:         device.Client,
			ClientVersion:  device.ClientVersion,
			RemoteAddress:  device.RemoteAddress,
			CreatedAt:      device.CreatedAt,
			LastActivityAt: device.LastActivityAt,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) revokeDevice(w http.ResponseWriter, r *http.Request, user accounts.ID, name string) {
	device, ok := pathID(w, r, name)
	if !ok {
		return
	}
	err := h.Accounts.RevokeDevice(r.Context(), user, device)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) ownDevices(w http.ResponseWriter, r *http.Request) {
	h.writeDevices(w, r, sessionFrom(r.Context()).User.ID)
}

func (h *handler) revokeOwnDevice(w http.ResponseWriter, r *http.Request) {
	h.revokeDevice(w, r, sessionFrom(r.Context()).User.ID, "id")
}

func (h *handler) quickConnectRequest(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeError(w, http.StatusConflict, "quick_connect_disabled")
		return
	}
	request, err := h.QuickConnect.ByCode(r.PathValue("code"))
	if errors.Is(err, quickconnect.ErrUnknown) {
		writeError(w, http.StatusNotFound, "unknown_code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deviceName":  request.DeviceName,
		"appName":     request.AppName,
		"appVersion":  request.AppVersion,
		"requestedAt": request.CreatedAt,
	})
}

func (h *handler) quickConnectApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !h.Accounts.Settings().QuickConnectEnabled {
		writeError(w, http.StatusConflict, "quick_connect_disabled")
		return
	}
	if err := h.QuickConnect.Authorize(body.Code, sessionFrom(r.Context()).User.ID); err != nil {
		writeError(w, http.StatusNotFound, "unknown_code")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) users(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]userJSON, 0, len(users))
	for _, user := range users {
		result = append(result, newUserJSON(user))
	}
	if result, err = h.withPins(r, result...); err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Password        string `json:"password"`
		IsAdministrator bool   `json:"isAdministrator"`
		IsHidden        bool   `json:"isHidden"`
	}
	if !decode(w, r, &body) {
		return
	}
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{
		Name: body.Name, Password: body.Password, IsAdministrator: body.IsAdministrator, IsHidden: body.IsHidden,
	})
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.UserCreated(r.Context(), user)
	writeJSON(w, http.StatusCreated, newUserJSON(user))
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Name            *string              `json:"name"`
		Password        *string              `json:"password"`
		IsAdministrator *bool                `json:"isAdministrator"`
		IsHidden        *bool                `json:"isHidden"`
		IsDisabled      *bool                `json:"isDisabled"`
		ParentalControl *parentalControlJSON `json:"parentalControl"`
		// Transcoding sets both video and audio conversion.
		Transcoding *bool `json:"transcoding"`
		Downloads   *bool `json:"downloads"`
		// PersonalAddons lets the user add and use their own addons.
		PersonalAddons *bool `json:"personalAddons"`
		// The user's playback and access limits, see userJSON.
		MaxPlaybacks  *int                     `json:"maxPlaybacks"`
		MaxBitrate    *int                     `json:"maxBitrate"`
		LiveTv        *bool                    `json:"liveTv"`
		SyncPlay      *accounts.SyncPlayAccess `json:"syncPlay"`
		RemoteControl *bool                    `json:"remoteControl"`
		// The user's content settings; see userJSON.
		HiddenLibraries *[]string             `json:"hiddenLibraries"`
		BlockedGenres   *[]string             `json:"blockedGenres"`
		AccessSchedules *[]accessScheduleJSON `json:"accessSchedules"`
		// CollectionManagement lets the user manage collections.
		CollectionManagement *bool `json:"collectionManagement"`
		// SubtitleManagement, see userJSON.
		SubtitleManagement *bool `json:"subtitleManagement"`
		// LiveTvManagement, see userJSON.
		LiveTvManagement *bool `json:"liveTvManagement"`
		// QualityGroup, see userJSON.
		QualityGroup *int `json:"qualityGroup"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	var keep []byte
	if id == session.User.ID {
		keep = session.TokenHash
	}
	changes := accounts.UserChanges{
		Name:               body.Name,
		Password:           body.Password,
		IsAdministrator:    body.IsAdministrator,
		IsHidden:           body.IsHidden,
		IsDisabled:         body.IsDisabled,
		VideoTranscoding:   body.Transcoding,
		AudioTranscoding:   body.Transcoding,
		ContentDownloading: body.Downloads,
		PersonalAddons:     body.PersonalAddons,
		MaxPlaybacks:       body.MaxPlaybacks,
		MaxBitrate:         body.MaxBitrate,
		LiveTv:             body.LiveTv,
		SyncPlay:           body.SyncPlay,
		RemoteControl:      body.RemoteControl,
		BlockedGenres:      body.BlockedGenres,
		// The user's permission to manage collections.
		CollectionManagement: body.CollectionManagement,
		SubtitleManagement:   body.SubtitleManagement,
		LiveTvManagement:     body.LiveTvManagement,
		QualityGroup:         body.QualityGroup,
	}
	if p := body.ParentalControl; p != nil {
		changes.Parental = &accounts.ParentalControl{MaxRating: p.MaxRating, MaxSubRating: p.MaxSubRating, BlockUnrated: p.BlockUnrated}
	}
	if body.HiddenLibraries != nil {
		hidden := make([]accounts.ID, 0, len(*body.HiddenLibraries))
		for _, raw := range *body.HiddenLibraries {
			id, err := accounts.ParseID(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_hidden_libraries")
				return
			}
			hidden = append(hidden, id)
		}
		changes.HiddenLibraries = &hidden
	}
	if body.AccessSchedules != nil {
		schedules := make([]accounts.AccessSchedule, 0, len(*body.AccessSchedules))
		for _, s := range *body.AccessSchedules {
			schedules = append(schedules, accounts.AccessSchedule{Day: s.Day, StartHour: s.StartHour, EndHour: s.EndHour})
		}
		changes.AccessSchedules = &schedules
	}
	user, err := h.Accounts.UpdateUser(r.Context(), id, changes, keep)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if changes.Password != nil {
		h.Activity.PasswordChanged(r.Context(), user)
	}
	if changes != (accounts.UserChanges{Password: changes.Password}) {
		h.Activity.UserChanged(r.Context(), user)
	}
	h.writeUser(w, r, http.StatusOK, user)
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if err == nil {
		err = h.Accounts.DeleteUser(r.Context(), id)
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.UserDeleted(r.Context(), user.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) userDevices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.Accounts.User(r.Context(), id); accountError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeDevices(w, r, id)
}

func (h *handler) revokeUserDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	h.revokeDevice(w, r, id, "deviceId")
}

// unblockUser ends the block of a user's account for wrong passwords.
func (h *handler) unblockUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.Unblock(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeUser(w, r, http.StatusOK, user)
}

// turnOffDownloads takes the permission to download away from every user,
// and answers with those who had it.
func (h *handler) turnOffDownloads(w http.ResponseWriter, r *http.Request) {
	changed, err := h.Accounts.TurnOffDownloads(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]userJSON, 0, len(changed))
	for _, user := range changed {
		h.Activity.UserChanged(r.Context(), user)
		result = append(result, newUserJSON(user))
	}
	if result, err = h.withPins(r, result...); err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.settingsJSON(h.Accounts.Settings()))
}

// settingsJSON describes settings with the default folders, the render
// nodes, what conversions run on, and the bounds of each setting.
func (h *handler) settingsJSON(settings accounts.Settings) settingsJSON {
	body := newSettingsJSON(settings)
	body.Bounds = settingsBounds
	body.RecordingsFolderDefault = accounts.DefaultRecordingsFolder(h.DataDir)
	body.BackupFolderDefault = accounts.DefaultBackupFolder(h.DataDir)
	body.RenderNodes = renderNodes()
	body.ConversionHardware = conversionHardwareJSON{Encoders: []string{}}
	if encoder := h.Health.Encoder; encoder != nil {
		if hw := encoder.Hardware(); hw != nil {
			body.ConversionHardware.GPU = newHardwareJSON(hw)
		}
		for _, name := range []string{"libx264", "libx265"} {
			if slices.Contains(encoder.Encoders(), name) {
				body.ConversionHardware.Encoders = append(body.ConversionHardware.Encoders, name)
			}
		}
		if encoder.HasFilters("zscale", "tonemap") {
			body.ConversionHardware.ToneMapping = true
			body.ConversionHardware.ToneMappingHeight = encoder.ToneMappedHeight()
		}
		body.ConversionHardware.Bwdif = encoder.HasFilters("bwdif")
	}
	return body
}

// conversionHardwareJSON is what conversions run on: the GPU chosen, null
// for none, and, in software, the video encoders, the filters tone mapping
// HDR and the bwdif deinterlacer. ToneMappingHeight is the height
// Automatic caps HDR tone mapped on the processor at, as a timing at
// startup chose it (720 until it ends), 0 when the processor cannot tone
// map.
type conversionHardwareJSON struct {
	GPU               *hardwareJSON `json:"gpu"`
	Encoders          []string      `json:"encoders"`
	ToneMapping       bool          `json:"toneMapping"`
	ToneMappingHeight int           `json:"toneMappingHeight"`
	Bwdif             bool          `json:"bwdif"`
}

func (h *handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsJSON
	// The custom CSS and script make the largest bodies of the admin API.
	if !decodeUpTo(w, r, &body, maxSettingsBody) {
		return
	}
	current := h.Accounts.Settings()
	publicMetaDBKey, ok := h.segmentKey(w, r, mediasegments.PublicMetaDB, body.PublicMetaDBKey, current.PublicMetaDBKey)
	if !ok {
		return
	}
	theIntroDBKey, ok := h.segmentKey(w, r, mediasegments.TheIntroDB, body.TheIntroDBKey, current.TheIntroDBKey)
	if !ok {
		return
	}
	// An empty order is what older pages sent to follow POLYFIN_SEGMENTS,
	// which the settings now hold: it keeps the saved one.
	segmentOrder := current.SegmentOrder
	if len(body.SegmentOrder) > 0 {
		segmentOrder = body.SegmentOrder
	}
	next := accounts.Settings{
		ServerName:          body.ServerName,
		QuickConnectEnabled: body.QuickConnectEnabled,
		LegacyAuthorization: body.LegacyAuthorization,
		Language:            body.Language,
		PrepareAhead:        valueOr(body.PrepareAhead, current.PrepareAhead),
		Transcoding:         valueOr(body.Transcoding, current.Transcoding),
		CatalogLimit:        valueOr(body.CatalogLimit, current.CatalogLimit),
		ChannelLimit:        valueOr(body.ChannelLimit, current.ChannelLimit),
		UpdateCheck:         valueOr(body.UpdateCheck, current.UpdateCheck),

		SkipButtons:           valueOr(body.SkipButtons, current.SkipButtons),
		PublicMetaDBKey:       publicMetaDBKey,
		TheIntroDBKey:         theIntroDBKey,
		SegmentOrder:          segmentOrder,
		SegmentSourcesOff:     valueOr(body.SegmentSourcesOff, current.SegmentSourcesOff),
		SimilarTitles:         valueOr(body.SimilarTitles, current.SimilarTitles),
		Lyrics:                valueOr(body.Lyrics, current.Lyrics),
		PlayedPercent:         valueOr(body.PlayedPercent, current.PlayedPercent),
		ResumePercent:         valueOr(body.ResumePercent, current.ResumePercent),
		VersionListMinutes:    valueOr(body.VersionListMinutes, current.VersionListMinutes),
		CatalogRefreshMinutes: valueOr(body.CatalogRefreshMinutes, current.CatalogRefreshMinutes),
		PersonalAddons:        valueOr(body.PersonalAddons, current.PersonalAddons),
		ServerImports:         valueOr(body.ServerImports, current.ServerImports),
		LoginAttempts:         valueOr(body.LoginAttempts, current.LoginAttempts),
		InactiveDeviceDays:    valueOr(body.InactiveDeviceDays, current.InactiveDeviceDays),
		DetailedLog:           valueOr(body.DetailedLog, current.DetailedLog),
		AnalysisTimeout:       valueOr(body.AnalysisTimeout, current.AnalysisTimeout),
		VersionAttempts:       valueOr(body.VersionAttempts, current.VersionAttempts),
		PreferDirectPlay:      valueOr(body.PreferDirectPlay, current.PreferDirectPlay),
		MaxConversions:        valueOr(body.MaxConversions, current.MaxConversions),
		MaxConversionHeight:   valueOr(body.MaxConversionHeight, current.MaxConversionHeight),

		EncoderPreset:              valueOr(body.EncoderPreset, current.EncoderPreset),
		H264Quality:                valueOr(body.H264Quality, current.H264Quality),
		HevcQuality:                valueOr(body.HevcQuality, current.HevcQuality),
		AllowHevcEncoding:          valueOr(body.AllowHevcEncoding, current.AllowHevcEncoding),
		HardwareAcceleration:       valueOr(body.HardwareAcceleration, current.HardwareAcceleration),
		HardwareDecodingCodecs:     valueOr(body.HardwareDecodingCodecs, current.HardwareDecodingCodecs),
		ToneMapping:                valueOr(body.ToneMapping, current.ToneMapping),
		ToneMappingAlgorithm:       valueOr(body.ToneMappingAlgorithm, current.ToneMappingAlgorithm),
		ToneMappingPeak:            valueOr(body.ToneMappingPeak, current.ToneMappingPeak),
		ToneMappingDesat:           valueOr(body.ToneMappingDesat, current.ToneMappingDesat),
		GPUToneMapping:             valueOr(body.GPUToneMapping, current.GPUToneMapping),
		ProcessorToneMappingHeight: valueOr(body.ProcessorToneMappingHeight, current.ProcessorToneMappingHeight),
		DeinterlaceMethod:          valueOr(body.DeinterlaceMethod, current.DeinterlaceMethod),
		DeinterlaceDoubleRate:      valueOr(body.DeinterlaceDoubleRate, current.DeinterlaceDoubleRate),
		DownmixAlgorithm:           valueOr(body.DownmixAlgorithm, current.DownmixAlgorithm),
		DownmixBoost:               valueOr(body.DownmixBoost, current.DownmixBoost),
		MaxAudioChannels:           valueOr(body.MaxAudioChannels, current.MaxAudioChannels),
		AudioBitratePerChannel:     valueOr(body.AudioBitratePerChannel, current.AudioBitratePerChannel),
		EncodingThreads:            valueOr(body.EncodingThreads, current.EncodingThreads),
		AheadSeconds:               valueOr(body.AheadSeconds, current.AheadSeconds),

		Trickplay:          valueOr(body.Trickplay, current.Trickplay),
		TrickplayInterval:  valueOr(body.TrickplayInterval, current.TrickplayInterval),
		TrickplayWidth:     valueOr(body.TrickplayWidth, current.TrickplayWidth),
		ChapterImages:      valueOr(body.ChapterImages, current.ChapterImages),
		ThumbnailStorageGB: valueOr(body.ThumbnailStorageGB, current.ThumbnailStorageGB),

		RecordingPrePadding:    valueOr(body.RecordingPrePadding, current.RecordingPrePadding),
		RecordingPostPadding:   valueOr(body.RecordingPostPadding, current.RecordingPostPadding),
		RecordingRetentionDays: valueOr(body.RecordingRetentionDays, current.RecordingRetentionDays),
		LiveTvRefreshHours:     valueOr(body.LiveTvRefreshHours, current.LiveTvRefreshHours),
		LocalScanHours:         valueOr(body.LocalScanHours, current.LocalScanHours),

		CustomCss:       valueOr(body.CustomCss, current.CustomCss),
		CustomJs:        valueOr(body.CustomJs, current.CustomJs),
		LoginDisclaimer: valueOr(body.LoginDisclaimer, current.LoginDisclaimer),

		TraktClientID:     valueOr(body.TraktClientID, current.TraktClientID),
		TraktClientSecret: valueOr(body.TraktClientSecret, current.TraktClientSecret),
		SimklClientID:     valueOr(body.SimklClientID, current.SimklClientID),
		LastFMAPIKey:      valueOr(body.LastFMAPIKey, current.LastFMAPIKey),
		LastFMSecret:      valueOr(body.LastFMSecret, current.LastFMSecret),

		BackupHour:         valueOr(body.BackupHour, current.BackupHour),
		BackupsKept:        valueOr(body.BackupsKept, current.BackupsKept),
		CollectionReadHour: valueOr(body.CollectionReadHour, current.CollectionReadHour),

		RemuxDB:    valueOr(body.RemuxDB, current.RemuxDB),
		RemuxDBURL: valueOr(body.RemuxDBURL, current.RemuxDBURL),

		CacheSizeGB:      valueOr(body.CacheSizeGB, current.CacheSizeGB),
		VAAPIDevice:      valueOr(body.VAAPIDevice, current.VAAPIDevice),
		Recording:        valueOr(body.Recording, current.Recording),
		RecordingsFolder: valueOr(body.RecordingsFolder, current.RecordingsFolder),
		Backups:          valueOr(body.Backups, current.Backups),
		BackupFolder:     valueOr(body.BackupFolder, current.BackupFolder),

		PublicAddress: valueOr(body.PublicAddress, current.PublicAddress),

		SMTPHost:     valueOr(body.SMTPHost, current.SMTPHost),
		SMTPPort:     valueOr(body.SMTPPort, current.SMTPPort),
		SMTPSecurity: valueOr(body.SMTPSecurity, current.SMTPSecurity),
		SMTPUser:     valueOr(body.SMTPUser, current.SMTPUser),
		SMTPPassword: valueOr(body.SMTPPassword, current.SMTPPassword),
		SMTPFrom:     valueOr(body.SMTPFrom, current.SMTPFrom),
		SMTPFromName: valueOr(body.SMTPFromName, current.SMTPFromName),

		PlaybackHistory:     valueOr(body.PlaybackHistory, current.PlaybackHistory),
		PlaybackHistoryDays: valueOr(body.PlaybackHistoryDays, current.PlaybackHistoryDays),
	}
	if code := h.unwritableFolder(next, current); code != "" {
		writeError(w, http.StatusBadRequest, code)
		return
	}
	settings, err := h.Accounts.UpdateSettings(r.Context(), next)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.SettingsSaved(r.Context(), sessionFrom(r.Context()).User.Name)
	if !settings.QuickConnectEnabled {
		h.QuickConnect.Clear()
	}
	// A GPU or a render node chosen anew is detected, or switched to,
	// before the answer shows what was found.
	if encoder := h.Health.Encoder; encoder != nil &&
		(settings.HardwareAcceleration != current.HardwareAcceleration || settings.VAAPIDevice != current.VAAPIDevice) {
		encoder.SelectHardware(settings.HardwareAcceleration, settings.VAAPIDevice)
	}
	// Recording turned on, or moved, has the scheduler look at its timers.
	if settings.RecordingsDir(h.DataDir) != current.RecordingsDir(h.DataDir) {
		h.Recordings.Wake()
	}
	writeJSON(w, http.StatusOK, h.settingsJSON(settings))
}

// renderNodesPattern matches the render nodes VAAPI may open.
var renderNodesPattern = "/dev/dri/renderD*"

// renderNodes lists the render nodes the server has, sorted; none is an
// empty list.
func renderNodes() []string {
	nodes, _ := filepath.Glob(renderNodesPattern)
	if nodes == nil {
		return []string{}
	}
	slices.Sort(nodes)
	return nodes
}

// unwritableFolder checks the recordings and backup folders next turns to,
// against the current settings: recording turned on, or its folder
// changed while it is on, needs a folder Polyfin can write into, the
// default one being created first; likewise for backups. It returns the
// error code of the first that fails, empty when none does. A folder that
// is not a path is left to UpdateSettings to refuse.
func (h *handler) unwritableFolder(next, current accounts.Settings) string {
	for _, f := range []struct {
		on, wasOn                bool
		folder, was, defaultPath string
		code                     string
	}{
		{next.Recording, current.Recording, next.RecordingsFolder, current.RecordingsFolder, accounts.DefaultRecordingsFolder(h.DataDir),
			"invalid_recordings_folder"},
		{next.Backups, current.Backups, next.BackupFolder, current.BackupFolder, accounts.DefaultBackupFolder(h.DataDir),
			"invalid_backup_folder"},
	} {
		folder, ok := accounts.CleanFolder(f.folder)
		if !f.on || !ok || f.wasOn && folder == f.was {
			continue
		}
		dir, create := folder, false
		if dir == "" {
			dir, create = f.defaultPath, true
		}
		if err := config.PrepareFolder(dir, create); err != nil {
			h.Logger.Info("A folder chosen in the settings cannot be written into", "folder", dir, "error", err)
			return f.code
		}
	}
	return ""
}

// segmentKeyErrors are the error codes of the keys of the segment
// databases: malformed or refused, then not checked.
var segmentKeyErrors = map[string][2]string{
	mediasegments.PublicMetaDB: {"invalid_publicmetadb_key", "publicmetadb_unreachable"},
	mediasegments.TheIntroDB:   {"invalid_theintrodb_key", "theintrodb_unreachable"},
}

// segmentKey is the key of a segment database a settings PUT saves: the
// current one when the body leaves it out, none for an empty one, else the
// key sent once the database accepted it. A key that is malformed or that
// the database refuses is answered 400, and one that the database could
// not be asked about 502, ok then being false.
func (h *handler) segmentKey(w http.ResponseWriter, r *http.Request, database string, sent *string, current string) (key string, ok bool) {
	if sent == nil {
		return current, true
	}
	if *sent == "" {
		return "", true
	}
	codes := segmentKeyErrors[database]
	key = strings.TrimSpace(*sent)
	if key == "" || !accounts.ValidSegmentKey(key) {
		writeError(w, http.StatusBadRequest, codes[0])
		return "", false
	}
	if h.Segments == nil {
		writeError(w, http.StatusBadGateway, codes[1])
		return "", false
	}
	switch err := h.Segments.CheckKey(r.Context(), database, key); {
	case errors.Is(err, mediasegments.ErrKeyRefused):
		writeError(w, http.StatusBadRequest, codes[0])
		return "", false
	case err != nil:
		h.Logger.Warn("A segment database could not check the key", "database", database, "error", err)
		writeError(w, http.StatusBadGateway, codes[1])
		return "", false
	}
	return key, true
}

// valueOr is the value value points to, else fallback.
func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
