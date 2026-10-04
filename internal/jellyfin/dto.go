package jellyfin

import (
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The types below reproduce the JSON Jellyfin 12.1 returns, field for field.
// Jellyfin omits null values; Go nil slices would encode as null, so every
// list is initialized.

// Time encodes like Jellyfin: UTC with up to seven fractional digits,
// trailing zeros dropped, but all seven when the fraction is zero.
type Time time.Time

func (t Time) MarshalJSON() ([]byte, error) {
	utc := time.Time(t).UTC()
	text := utc.Format("2006-01-02T15:04:05.0000000")
	if utc.Nanosecond()/100 != 0 {
		text = strings.TrimRight(text, "0")
	}
	return []byte(`"` + text + `Z"`), nil
}

// UnmarshalJSON accepts the dates apps send (see parseTime); null leaves
// the date unchanged.
func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	text, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	parsed, ok := parseTime(text)
	if !ok {
		return &time.ParseError{Layout: time.RFC3339, Value: text}
	}
	*t = Time(parsed)
	return nil
}

// parseTime reads a date as ASP.NET binds one: RFC 3339 with any precision,
// a date and time without a zone, read as UTC, or a date alone.
func parseTime(text string) (time.Time, bool) {
	text = strings.TrimSpace(text)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func optionalTime(t *time.Time) *Time {
	if t == nil {
		return nil
	}
	value := Time(*t)
	return &value
}

type PublicSystemInfo struct {
	LocalAddress           string
	ServerName             string
	Version                string
	ProductName            string
	OperatingSystem        string
	Id                     string
	StartupWizardCompleted bool
}

type CastReceiverApplication struct {
	Id   string
	Name string
}

type SystemInfo struct {
	OperatingSystemDisplayName string
	HasPendingRestart          bool
	IsShuttingDown             bool
	SupportsLibraryMonitor     bool
	WebSocketPortNumber        int
	CompletedInstallations     []struct{}
	CanSelfRestart             bool
	CanLaunchWebBrowser        bool
	ProgramDataPath            string
	WebPath                    string
	ItemsByNamePath            string
	CachePath                  string
	LogPath                    string
	InternalMetadataPath       string
	TranscodingTempPath        string
	CastReceiverApplications   []CastReceiverApplication
	HasUpdateAvailable         bool
	EncoderLocation            string
	SystemArchitecture         string
	PublicSystemInfo
}

// castReceivers are the Google Cast receiver applications Jellyfin apps
// offer, published by the Jellyfin project for any compatible server.
var castReceivers = []CastReceiverApplication{{Id: "F007D354", Name: "Stable"}, {Id: "6F511C87", Name: "Unstable"}}

type UserConfiguration struct {
	AudioLanguagePreference    *string `json:",omitempty"`
	PlayDefaultAudioTrack      bool
	SubtitleLanguagePreference string
	DisplayMissingEpisodes     bool
	GroupedFolders             []string
	SubtitleMode               string
	DisplayCollectionsView     bool
	EnableLocalPassword        bool
	OrderedViews               []string
	LatestItemsExcludes        []string
	MyMediaExcludes            []string
	HidePlayedInLatest         bool
	RememberAudioSelections    bool
	RememberSubtitleSelections bool
	EnableNextEpisodeAutoPlay  bool
	CastReceiverId             string
}

type UserPolicy struct {
	IsAdministrator                  bool
	IsHidden                         bool
	EnableCollectionManagement       bool
	EnableSubtitleManagement         bool
	EnableLyricManagement            bool
	IsDisabled                       bool
	MaxParentalRating                *int `json:",omitempty"`
	MaxParentalSubRating             *int `json:",omitempty"`
	BlockedTags                      []string
	AllowedTags                      []string
	EnableUserPreferenceAccess       bool
	AccessSchedules                  []AccessSchedule
	BlockUnratedItems                []string
	EnableRemoteControlOfOtherUsers  bool
	EnableSharedDeviceControl        bool
	EnableRemoteAccess               bool
	EnableLiveTvManagement           bool
	EnableLiveTvAccess               bool
	EnableMediaPlayback              bool
	EnableAudioPlaybackTranscoding   bool
	EnableVideoPlaybackTranscoding   bool
	EnablePlaybackRemuxing           bool
	ForceRemoteSourceTranscoding     bool
	EnableContentDeletion            bool
	EnableContentDeletionFromFolders []string
	EnableContentDownloading         bool
	EnableSyncTranscoding            bool
	EnableMediaConversion            bool
	EnabledDevices                   []string
	EnableAllDevices                 bool
	EnabledChannels                  []string
	EnableAllChannels                bool
	EnabledFolders                   []string
	EnableAllFolders                 bool
	InvalidLoginAttemptCount         int
	LoginAttemptsBeforeLockout       int
	MaxActiveSessions                int
	EnablePublicSharing              bool
	BlockedMediaFolders              []string
	BlockedChannels                  []string
	RemoteClientBitrateLimit         int
	AuthenticationProviderId         string
	PasswordResetProviderId          string
	SyncPlayAccess                   string
}

// AccessSchedule is Jellyfin's AccessSchedule, hours a user may use the
// server in. Id numbers the user's schedules from 1: Polyfin keeps them
// with the user, without identifiers of their own.
type AccessSchedule struct {
	Id        int
	UserId    string
	DayOfWeek string
	StartHour float64
	EndHour   float64
}

func accessSchedules(user accounts.User) []AccessSchedule {
	result := make([]AccessSchedule, 0, len(user.AccessSchedules))
	for i, schedule := range user.AccessSchedules {
		result = append(result, AccessSchedule{Id: i + 1, UserId: user.ID.String(), DayOfWeek: schedule.Day,
			StartHour: schedule.StartHour, EndHour: schedule.EndHour})
	}
	return result
}

type UserDto struct {
	Name                      string
	ServerId                  string
	Id                        string
	HasPassword               bool
	HasConfiguredPassword     bool
	HasConfiguredEasyPassword bool
	EnableAutoLogin           bool
	LastLoginDate             *Time `json:",omitempty"`
	LastActivityDate          *Time `json:",omitempty"`
	Configuration             UserConfiguration
	Policy                    UserPolicy
}

func newUserDto(user accounts.User, serverID string) UserDto {
	return UserDto{
		Name:                  user.Name,
		ServerId:              serverID,
		Id:                    user.ID.String(),
		HasPassword:           true,
		HasConfiguredPassword: true,
		LastLoginDate:         optionalTime(user.LastLoginAt),
		LastActivityDate:      optionalTime(user.LastActivityAt),
		// The configuration the user saved is filled in by userDto.
		Configuration: defaultUserConfiguration(),
		// Capabilities Polyfin does not offer (deleting content, managing
		// collections, lyrics or live TV) are reported as denied. Every user
		// may manage subtitles, as apps offer their subtitle search, which
		// lists the addons' subtitles, only to users allowed to. Jellyfin's
		// tags are not genres: titles in Polyfin have none, so none are
		// blocked or allowed; the genres a user blocks are Polyfin's own
		// setting. libraryAccess lists the server's libraries the user sees
		// when they hide some.
		Policy: UserPolicy{
			IsAdministrator:                 user.IsAdministrator,
			IsHidden:                        user.IsHidden,
			IsDisabled:                      user.IsDisabled,
			BlockedTags:                     []string{},
			AllowedTags:                     []string{},
			EnableUserPreferenceAccess:      true,
			AccessSchedules:                 accessSchedules(user),
			MaxParentalRating:               user.Parental.MaxRating,
			MaxParentalSubRating:            user.Parental.MaxSubRating,
			BlockUnratedItems:               append([]string{}, user.Parental.BlockUnrated...),
			EnableRemoteControlOfOtherUsers: user.RemoteControl,
			EnableSharedDeviceControl:       true,
			EnableRemoteAccess:              true,
			EnableLiveTvAccess:              user.LiveTv,
			EnableMediaPlayback:             true,
			// The user's own permissions; the server's switches apply on top.
			EnableAudioPlaybackTranscoding:   user.AudioTranscoding,
			EnableVideoPlaybackTranscoding:   user.VideoTranscoding,
			EnablePlaybackRemuxing:           true,
			EnableContentDeletionFromFolders: []string{},
			EnableContentDownloading:         user.ContentDownloading,
			EnableSubtitleManagement:         true,
			EnableSyncTranscoding:            true,
			EnableMediaConversion:            true,
			EnabledDevices:                   []string{},
			EnableAllDevices:                 true,
			EnabledChannels:                  []string{},
			EnableAllChannels:                true,
			EnabledFolders:                   []string{},
			EnableAllFolders:                 len(user.HiddenLibraries) == 0,
			LoginAttemptsBeforeLockout:       -1,
			EnablePublicSharing:              true,
			BlockedMediaFolders:              []string{},
			BlockedChannels:                  []string{},
			AuthenticationProviderId:         "Jellyfin.Server.Implementations.Users.DefaultAuthenticationProvider",
			PasswordResetProviderId:          "Jellyfin.Server.Implementations.Users.DefaultPasswordResetProvider",
			SyncPlayAccess:                   string(user.SyncPlay),
			// Jellyfin counts signed-in sessions against MaxActiveSessions and
			// limits remote clients only to RemoteClientBitrateLimit; Polyfin
			// counts the devices playing and limits all of the user's playback
			// (see Handler.playbackLimitReached and limitBitrate).
			MaxActiveSessions:        user.MaxPlaybacks,
			RemoteClientBitrateLimit: user.MaxBitrate,
		},
	}
}

type PlayerStateInfo struct {
	PositionTicks       *int64 `json:",omitempty"`
	CanSeek             bool
	IsPaused            bool
	IsMuted             bool
	VolumeLevel         *int   `json:",omitempty"`
	AudioStreamIndex    *int   `json:",omitempty"`
	SubtitleStreamIndex *int   `json:",omitempty"`
	MediaSourceId       string `json:",omitempty"`
	PlayMethod          string `json:",omitempty"`
	RepeatMode          string
	PlaybackOrder       string
}

type ClientCapabilities struct {
	PlayableMediaTypes           []string
	SupportedCommands            []string
	SupportsMediaControl         bool
	SupportsPersistentIdentifier bool
}

type SessionInfo struct {
	PlayState             PlayerStateInfo
	AdditionalUsers       []struct{}
	Capabilities          ClientCapabilities
	RemoteEndPoint        string
	PlayableMediaTypes    []string
	Id                    string
	UserId                string
	UserName              string
	Client                string
	LastActivityDate      Time
	LastPlaybackCheckIn   Time
	LastPausedDate        *Time `json:",omitempty"`
	DeviceName            string
	DeviceId              string
	ApplicationVersion    string
	IsActive              bool
	SupportsMediaControl  bool
	SupportsRemoteControl bool
	NowPlayingQueue       []struct{}
	NowPlayingItem        *BaseItemDto `json:",omitempty"`
	HasCustomDeviceName   bool
	ServerId              string
	SupportedCommands     []string
}

// newSessionInfo describes a device's session. Like Jellyfin, it reports
// media and remote control only when the device is controllable, as its
// declared capabilities alone do not say whether commands can reach it.
func newSessionInfo(device accounts.Device, user accounts.User, serverID string, controllable bool) SessionInfo {
	capabilities := device.Capabilities
	return SessionInfo{
		PlayState:       PlayerStateInfo{RepeatMode: "RepeatNone", PlaybackOrder: "Default"},
		AdditionalUsers: []struct{}{},
		Capabilities: ClientCapabilities{
			PlayableMediaTypes:           capabilities.PlayableMediaTypes,
			SupportedCommands:            capabilities.SupportedCommands,
			SupportsMediaControl:         capabilities.SupportsMediaControl,
			SupportsPersistentIdentifier: capabilities.SupportsPersistentIdentifier,
		},
		RemoteEndPoint:        device.RemoteAddress,
		PlayableMediaTypes:    capabilities.PlayableMediaTypes,
		Id:                    device.ID.String(),
		UserId:                user.ID.String(),
		UserName:              user.Name,
		Client:                device.Client,
		LastActivityDate:      Time(device.LastActivityAt),
		DeviceName:            device.DeviceName,
		DeviceId:              device.DeviceID,
		ApplicationVersion:    device.ClientVersion,
		IsActive:              true,
		SupportsMediaControl:  controllable,
		SupportsRemoteControl: controllable,
		NowPlayingQueue:       []struct{}{},
		ServerId:              serverID,
		SupportedCommands:     capabilities.SupportedCommands,
	}
}

type AuthenticationResult struct {
	User        UserDto
	SessionInfo SessionInfo
	AccessToken string
	ServerId    string
}

type QuickConnectResult struct {
	Authenticated bool
	Secret        string
	Code          string
	DeviceId      string
	DeviceName    string
	AppName       string
	AppVersion    string
	DateAdded     Time
}

type BrandingOptions struct {
	SplashscreenEnabled bool
}

// VirtualFolderInfo describes a library as Jellyfin's library settings do.
// Jellyfin leaves RefreshProgress out while a library is idle, which
// Polyfin's libraries always are.
type VirtualFolderInfo struct {
	Name               string
	Locations          []string
	CollectionType     string `json:",omitempty"`
	LibraryOptions     LibraryOptions
	ItemId             string
	PrimaryImageItemId string `json:",omitempty"`
	RefreshStatus      string
}

type LibraryOptions struct {
	Enabled                                 bool
	EnablePhotos                            bool
	EnableRealtimeMonitor                   bool
	EnableLUFSScan                          bool
	EnableChapterImageExtraction            bool
	ExtractChapterImagesDuringLibraryScan   bool
	EnableTrickplayImageExtraction          bool
	ExtractTrickplayImagesDuringLibraryScan bool
	PathInfos                               []struct{}
	SaveLocalMetadata                       bool
	EnableInternetProviders                 bool
	EnableAutomaticSeriesGrouping           bool
	EnableEmbeddedTitles                    bool
	EnableEmbeddedExtrasTitles              bool
	EnableEmbeddedEpisodeInfos              bool
	AutomaticRefreshIntervalDays            int
	SeasonZeroDisplayName                   string
	DisabledLocalMetadataReaders            []string
	DisabledSubtitleFetchers                []string
	SubtitleFetcherOrder                    []string
	DisabledMediaSegmentProviders           []string
	MediaSegmentProviderOrder               []string
	SkipSubtitlesIfEmbeddedSubtitlesPresent bool
	SkipSubtitlesIfAudioTrackMatches        bool
	RequirePerfectSubtitleMatch             bool
	SaveSubtitlesWithMedia                  bool
	SaveLyricsWithMedia                     bool
	SaveTrickplayWithMedia                  bool
	DisabledLyricFetchers                   []string
	LyricFetcherOrder                       []string
	PreferNonstandardArtistsTag             bool
	UseCustomTagDelimiters                  bool
	CustomTagDelimiters                     []string
	DelimiterWhitelist                      []string
	AutomaticallyAddToCollection            bool
	AllowEmbeddedSubtitles                  string
	TypeOptions                             []struct{}
}

// newLibraryOptions returns the options Jellyfin gives a library created
// without any. Polyfin's libraries are addon catalogs: they have no paths,
// and none of these options changes what they list.
func newLibraryOptions() LibraryOptions {
	return LibraryOptions{
		Enabled:                          true,
		EnablePhotos:                     true,
		PathInfos:                        []struct{}{},
		EnableAutomaticSeriesGrouping:    true,
		SeasonZeroDisplayName:            "Specials",
		DisabledLocalMetadataReaders:     []string{},
		DisabledSubtitleFetchers:         []string{},
		SubtitleFetcherOrder:             []string{},
		DisabledMediaSegmentProviders:    []string{},
		MediaSegmentProviderOrder:        []string{},
		SkipSubtitlesIfAudioTrackMatches: true,
		RequirePerfectSubtitleMatch:      true,
		SaveSubtitlesWithMedia:           true,
		DisabledLyricFetchers:            []string{},
		LyricFetcherOrder:                []string{},
		CustomTagDelimiters:              []string{"/", "|", ";", `\`},
		DelimiterWhitelist:               []string{},
		AllowEmbeddedSubtitles:           "AllowAll",
		TypeOptions:                      []struct{}{},
	}
}
