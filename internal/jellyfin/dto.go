package jellyfin

import (
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The types below reproduce the JSON Jellyfin 12.1 returns, field for field.
// Jellyfin omits null values; Go nil slices would encode as null, so every
// list is initialized.

// Time encodes like .NET: UTC with seven fractional digits.
type Time time.Time

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format("2006-01-02T15:04:05.0000000Z") + `"`), nil
}

// UnmarshalJSON accepts any RFC 3339 date, as apps send dates with various
// precisions and offsets.
func (t *Time) UnmarshalJSON(data []byte) error {
	var parsed time.Time
	if err := parsed.UnmarshalJSON(data); err != nil {
		return err
	}
	*t = Time(parsed)
	return nil
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
	BlockedTags                      []string
	AllowedTags                      []string
	EnableUserPreferenceAccess       bool
	AccessSchedules                  []struct{}
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
		// Preferences are not stored yet: every user has a new Jellyfin
		// user's defaults.
		Configuration: UserConfiguration{
			PlayDefaultAudioTrack:      true,
			GroupedFolders:             []string{},
			SubtitleMode:               "Default",
			OrderedViews:               []string{},
			LatestItemsExcludes:        []string{},
			MyMediaExcludes:            []string{},
			HidePlayedInLatest:         true,
			RememberAudioSelections:    true,
			RememberSubtitleSelections: true,
			EnableNextEpisodeAutoPlay:  true,
			CastReceiverId:             castReceivers[0].Id,
		},
		// Capabilities Polyfin does not offer (deleting content, managing
		// collections, subtitles, lyrics or live TV) are reported as denied.
		Policy: UserPolicy{
			IsAdministrator:                  user.IsAdministrator,
			IsHidden:                         user.IsHidden,
			IsDisabled:                       user.IsDisabled,
			BlockedTags:                      []string{},
			AllowedTags:                      []string{},
			EnableUserPreferenceAccess:       true,
			AccessSchedules:                  []struct{}{},
			BlockUnratedItems:                []string{},
			EnableRemoteControlOfOtherUsers:  user.IsAdministrator,
			EnableSharedDeviceControl:        true,
			EnableRemoteAccess:               true,
			EnableLiveTvAccess:               true,
			EnableMediaPlayback:              true,
			EnableAudioPlaybackTranscoding:   true,
			EnableVideoPlaybackTranscoding:   true,
			EnablePlaybackRemuxing:           true,
			EnableContentDeletionFromFolders: []string{},
			EnableContentDownloading:         true,
			EnableSyncTranscoding:            true,
			EnableMediaConversion:            true,
			EnabledDevices:                   []string{},
			EnableAllDevices:                 true,
			EnabledChannels:                  []string{},
			EnableAllChannels:                true,
			EnabledFolders:                   []string{},
			EnableAllFolders:                 true,
			LoginAttemptsBeforeLockout:       -1,
			EnablePublicSharing:              true,
			BlockedMediaFolders:              []string{},
			BlockedChannels:                  []string{},
			AuthenticationProviderId:         "Jellyfin.Server.Implementations.Users.DefaultAuthenticationProvider",
			PasswordResetProviderId:          "Jellyfin.Server.Implementations.Users.DefaultPasswordResetProvider",
			SyncPlayAccess:                   "CreateAndJoinGroups",
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

func newSessionInfo(device accounts.Device, user accounts.User, serverID string) SessionInfo {
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
		SupportsMediaControl:  capabilities.SupportsMediaControl,
		SupportsRemoteControl: capabilities.SupportsMediaControl,
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
