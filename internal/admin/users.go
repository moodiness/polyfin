package admin

import (
	"encoding/json"
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
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/tmdb"
)

type userJSON struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	IsAdministrator bool       `json:"isAdministrator"`
	IsHidden        bool       `json:"isHidden"`
	IsDisabled      bool       `json:"isDisabled"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastLoginAt     *time.Time `json:"lastLoginAt"`
	LastActivityAt  *time.Time `json:"lastActivityAt"`
	// The settings a group holds; a member who follows their group has its
	// values.
	limitsJSON
	// BlockedUntil is when the block of the account for wrong passwords
	// ends, null when it is not blocked.
	BlockedUntil *time.Time `json:"blockedUntil"`
	// PasswordResetPin is the PIN the user asked for from a Jellyfin app's
	// forgotten password screen, while it is valid, null otherwise: their
	// password becomes the PIN once they enter it.
	PasswordResetPin *passwordResetPinJSON `json:"passwordResetPin"`
	// ImageTag identifies their profile picture, served at /UserImage,
	// null without one.
	ImageTag *string `json:"imageTag"`
	// MetadataLanguage is the language the user's titles are described in,
	// one of the server's addons' languages, empty for the server's
	// descriptions.
	MetadataLanguage string `json:"metadataLanguage"`
	// EndsOn is the date the account ends, YYYY-MM-DD in the server's time
	// zone, null for never; Ended is set once it has come, and the account
	// is refused as a disabled one is.
	EndsOn *string `json:"endsOn"`
	Ended  bool    `json:"ended"`
	// GroupID and GroupName are the user's group, null for none;
	// FollowGroup tells whether they follow it, set without a group.
	GroupID     *string `json:"groupId"`
	GroupName   *string `json:"groupName"`
	FollowGroup bool    `json:"followGroup"`
	// TwoStep tells whether the user signs in with a code from an
	// authenticator app after their password.
	TwoStep bool `json:"twoStep"`
}

// limitsJSON are the settings a group holds, as a user's JSON and a
// group's show them.
type limitsJSON struct {
	ParentalControl parentalControlJSON `json:"parentalControl"`
	// Transcoding is set when the user may have both video and audio
	// converted; Downloads, when they may download.
	Transcoding bool `json:"transcoding"`
	Downloads   bool `json:"downloads"`
	// PersonalAddons is set when the user may add and use their own addons.
	PersonalAddons bool `json:"personalAddons"`
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
	// SubtitleManagement lets the user add subtitle files to titles from
	// their apps.
	SubtitleManagement bool `json:"subtitleManagement"`
	// CanShare lets the user give a movie or an episode to someone
	// without an account, through a share link, while the server allows
	// them.
	CanShare bool `json:"canShare"`
	// QualityGroup is the tallest video the user is offered, in lines: 480,
	// 720, 1080, 1440 or 2160, or 0 for Original, no limit.
	QualityGroup int `json:"qualityGroup"`
}

func newLimitsJSON(limits accounts.Limits) limitsJSON {
	return limitsJSON{
		ParentalControl: parentalControlJSON{
			MaxRating:    limits.Parental.MaxRating,
			MaxSubRating: limits.Parental.MaxSubRating,
			BlockUnrated: append([]string{}, limits.Parental.BlockUnrated...),
		},
		Transcoding:          limits.VideoTranscoding && limits.AudioTranscoding,
		Downloads:            limits.ContentDownloading,
		PersonalAddons:       limits.PersonalAddons,
		MaxPlaybacks:         limits.MaxPlaybacks,
		MaxBitrate:           limits.MaxBitrate,
		LiveTv:               limits.LiveTv,
		SyncPlay:             string(limits.SyncPlay),
		RemoteControl:        limits.RemoteControl,
		HiddenLibraries:      idStrings(limits.HiddenLibraries),
		BlockedGenres:        append([]string{}, limits.BlockedGenres...),
		AccessSchedules:      schedulesJSON(limits.AccessSchedules),
		CollectionManagement: limits.CollectionManagement,
		LiveTvManagement:     limits.LiveTvManagement,
		SubtitleManagement:   limits.SubtitleManagement,
		CanShare:             limits.CanShare,
		QualityGroup:         limits.QualityGroup,
	}
}

// limitChanges are the settings a group holds as a change to a user or a
// group sets them; see limitsJSON.
type limitChanges struct {
	ParentalControl *parentalControlJSON `json:"parentalControl"`
	// Transcoding sets both video and audio conversion.
	Transcoding          *bool                    `json:"transcoding"`
	Downloads            *bool                    `json:"downloads"`
	PersonalAddons       *bool                    `json:"personalAddons"`
	MaxPlaybacks         *int                     `json:"maxPlaybacks"`
	MaxBitrate           *int                     `json:"maxBitrate"`
	LiveTv               *bool                    `json:"liveTv"`
	SyncPlay             *accounts.SyncPlayAccess `json:"syncPlay"`
	RemoteControl        *bool                    `json:"remoteControl"`
	HiddenLibraries      *[]string                `json:"hiddenLibraries"`
	BlockedGenres        *[]string                `json:"blockedGenres"`
	AccessSchedules      *[]accessScheduleJSON    `json:"accessSchedules"`
	CollectionManagement *bool                    `json:"collectionManagement"`
	SubtitleManagement   *bool                    `json:"subtitleManagement"`
	CanShare             *bool                    `json:"canShare"`
	LiveTvManagement     *bool                    `json:"liveTvManagement"`
	QualityGroup         *int                     `json:"qualityGroup"`
}

// changes returns what l sets, or answers the request with its error and
// returns false.
func (l limitChanges) changes(w http.ResponseWriter) (accounts.UserChanges, bool) {
	changes := accounts.UserChanges{
		VideoTranscoding:     l.Transcoding,
		AudioTranscoding:     l.Transcoding,
		ContentDownloading:   l.Downloads,
		PersonalAddons:       l.PersonalAddons,
		MaxPlaybacks:         l.MaxPlaybacks,
		MaxBitrate:           l.MaxBitrate,
		LiveTv:               l.LiveTv,
		SyncPlay:             l.SyncPlay,
		RemoteControl:        l.RemoteControl,
		BlockedGenres:        l.BlockedGenres,
		CollectionManagement: l.CollectionManagement,
		SubtitleManagement:   l.SubtitleManagement,
		LiveTvManagement:     l.LiveTvManagement,
		QualityGroup:         l.QualityGroup,
		CanShare:             l.CanShare,
	}
	if p := l.ParentalControl; p != nil {
		changes.Parental = &accounts.ParentalControl{MaxRating: p.MaxRating, MaxSubRating: p.MaxSubRating, BlockUnrated: p.BlockUnrated}
	}
	if l.HiddenLibraries != nil {
		hidden := make([]accounts.ID, 0, len(*l.HiddenLibraries))
		for _, raw := range *l.HiddenLibraries {
			id, err := accounts.ParseID(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_hidden_libraries")
				return changes, false
			}
			hidden = append(hidden, id)
		}
		changes.HiddenLibraries = &hidden
	}
	if l.AccessSchedules != nil {
		schedules := make([]accounts.AccessSchedule, 0, len(*l.AccessSchedules))
		for _, s := range *l.AccessSchedules {
			schedules = append(schedules, accounts.AccessSchedule{Day: s.Day, StartHour: s.StartHour, EndHour: s.EndHour})
		}
		changes.AccessSchedules = &schedules
	}
	return changes, true
}

type passwordResetPinJSON struct {
	Pin       string    `json:"pin"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// withPins adds to users the password reset PINs still valid, and the
// names of their groups.
func (h *handler) withPins(r *http.Request, users ...userJSON) ([]userJSON, error) {
	pins, err := h.Accounts.PasswordResetPINs(r.Context())
	if err != nil {
		return nil, err
	}
	var names map[string]string
	for i, user := range users {
		id, _ := accounts.ParseID(user.ID)
		if pin, ok := pins[id]; ok {
			users[i].PasswordResetPin = &passwordResetPinJSON{Pin: pin.PIN, ExpiresAt: pin.ExpiresAt}
		}
		if user.GroupID == nil {
			continue
		}
		if names == nil {
			groups, err := h.Accounts.Groups(r.Context())
			if err != nil {
				return nil, err
			}
			names = make(map[string]string, len(groups))
			for _, group := range groups {
				names[group.ID.String()] = group.Name
			}
		}
		if name, ok := names[*user.GroupID]; ok {
			users[i].GroupName = &name
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
	result := userJSON{
		ID:               user.ID.String(),
		Name:             user.Name,
		IsAdministrator:  user.IsAdministrator,
		IsHidden:         user.IsHidden,
		IsDisabled:       user.IsDisabled,
		CreatedAt:        user.CreatedAt,
		LastLoginAt:      user.LastLoginAt,
		LastActivityAt:   user.LastActivityAt,
		limitsJSON:       newLimitsJSON(user.Limits()),
		BlockedUntil:     blockedUntil(user),
		ImageTag:         imageTag(user),
		MetadataLanguage: user.MetadataLanguage,
		// When the account ends.
		EndsOn:      endsOn(user),
		Ended:       user.Ended(time.Now()),
		FollowGroup: user.FollowGroup || user.GroupID == nil,
		TwoStep:     user.TwoStep,
	}
	if user.GroupID != nil {
		result.GroupID = new(user.GroupID.String())
	}
	return result
}

// endsOn is the date user's account ends, nil for never.
func endsOn(user accounts.User) *string {
	if user.EndsOn == nil {
		return nil
	}
	return new(user.EndsOn.Format(time.DateOnly))
}

// endDate reads the end date a change sets: a YYYY-MM-DD date, or null for
// never, which is the zero time.
func endDate(raw json.RawMessage) (time.Time, bool) {
	if string(raw) == "null" {
		return time.Time{}, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return time.Time{}, false
	}
	date, err := time.Parse(time.DateOnly, text)
	return date, err == nil && !date.IsZero()
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
	libraries, err := library.ServerLibraries(r.Context(), h.Addons, h.SmartLists, h.Accounts.Settings().Language)
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
	// LocalDiscovery and LocalAddress keep their current values when a
	// PUT leaves them out; an empty address announces the interface's.
	LocalDiscovery *bool   `json:"localDiscovery"`
	LocalAddress   *string `json:"localAddress"`
	// The DLNA settings keep their current values too when a PUT leaves
	// them out. DLNANewDevicesUser is a user's ID, or null for no one.
	DLNA               *bool           `json:"dlna"`
	DLNAName           *string         `json:"dlnaName"`
	DLNANewDevicesUser json.RawMessage `json:"dlnaNewDevicesUser"`
	DLNAMarkPlayed     *bool           `json:"dlnaMarkPlayed"`
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
	// sent to follow POLYFIN_SEGMENTS, keeps it too. MeasureLoudness is
	// "off", "local" or "all".
	SegmentOrder          []string  `json:"segmentOrder"`
	SegmentSourcesOff     *[]string `json:"segmentSourcesOff"`
	AudioIntros           *bool     `json:"audioIntros"`
	SimilarTitles         *bool     `json:"similarTitles"`
	GenerateImages        *bool     `json:"generateImages"`
	Lyrics                *bool     `json:"lyrics"`
	EvenVolume            *bool     `json:"evenVolume"`
	MeasureLoudness       *string   `json:"measureLoudness"`
	PlayedPercent         *int      `json:"playedPercent"`
	ResumePercent         *int      `json:"resumePercent"`
	VersionListMinutes    *int      `json:"versionListMinutes"`
	CatalogRefreshMinutes *int      `json:"catalogRefreshMinutes"`
	// MDBListRatingsKeySet and MDBListRatingsKey are the same for the
	// server's MDBList key for ratings, which MDBList checks too.
	// CriticRatingShows is "critics" or "audience".
	MDBListRatingsKeySet bool    `json:"mdblistRatingsKeySet"`
	MDBListRatingsKey    *string `json:"mdblistRatingsKey,omitempty"`
	CriticRatingShows    *string `json:"criticRatingShows"`
	// TMDBKeySet and TMDBKey are the same for the server's TMDB key, which
	// TMDB checks too.
	TMDBKeySet bool    `json:"tmdbKeySet"`
	TMDBKey    *string `json:"tmdbKey,omitempty"`
	// The security settings keep their current values when a PUT leaves
	// them out, too.
	PersonalAddons     *bool `json:"personalAddons"`
	ServerImports      *bool `json:"serverImports"`
	ShareLinks         *bool `json:"shareLinks"`
	LoginAttempts      *int  `json:"loginAttempts"`
	InactiveDeviceDays *int  `json:"inactiveDeviceDays"`
	DetailedLog        *bool `json:"detailedLog"`
	// TwoStepAdministrators requires two-step sign-in of administrators.
	TwoStepAdministrators *bool `json:"twoStepAdministrators"`
	// KnownProxies lists addresses, networks such as 172.18.0.0/16, and
	// host names, the names after the rest.
	KnownProxies *[]string `json:"knownProxies"`
	// The HTTPS settings keep their current values when a PUT leaves them
	// out. httpsMode is "off", "files" or "letsencrypt"; acmeChallenge
	// "http-01" or "tls-alpn-01".
	HTTPSMode        *string   `json:"httpsMode"`
	HTTPSPort        *int      `json:"httpsPort"`
	HTTPSRedirect    *bool     `json:"httpsRedirect"`
	HTTPSCertificate *string   `json:"httpsCertificate"`
	HTTPSPrivateKey  *string   `json:"httpsPrivateKey"`
	ACMEHosts        *[]string `json:"acmeHosts"`
	ACMEEmail        *string   `json:"acmeEmail"`
	ACMETerms        *bool     `json:"acmeTerms"`
	ACMEChallenge    *string   `json:"acmeChallenge"`
	// AnalysisTimeout, VersionAttempts, PreferDirectPlay,
	// SyncAddonSubtitles, MaxConversions and MaxConversionHeight keep their
	// current values when a PUT leaves them out.
	AnalysisTimeout  *int  `json:"analysisTimeout"`
	VersionAttempts  *int  `json:"versionAttempts"`
	PreferDirectPlay *bool `json:"preferDirectPlay"`
	// The version rules keep their current values when a PUT leaves them
	// out: VersionOrder holds up to four criteria (see
	// accounts.VersionCriteria), VersionCodecOrder every video codec once,
	// and HiddenVersionWords the words, one per entry.
	VersionOrder         *[]string `json:"versionOrder"`
	VersionCodecOrder    *[]string `json:"versionCodecOrder"`
	HideVersionsLargerGB *int      `json:"hideVersionsLargerGb"`
	HiddenVersionCodecs  *[]string `json:"hiddenVersionCodecs"`
	HiddenVersionWords   *[]string `json:"hiddenVersionWords"`
	// NameHowVersionsPlay keeps its current value when a PUT leaves it out.
	NameHowVersionsPlay *bool `json:"nameHowVersionsPlay"`
	SyncAddonSubtitles  *bool `json:"syncAddonSubtitles"`
	MaxConversions      *int  `json:"maxConversions"`
	MaxConversionHeight *int  `json:"maxConversionHeight"`
	ConvertedDownloads  *bool `json:"convertedDownloads"`
	// SubtitleOCR ("off", "played" or "scans") and SubtitleOCRTracks keep
	// their current values when a PUT leaves them out.
	SubtitleOCR       *string `json:"subtitleOcr"`
	SubtitleOCRTracks *int    `json:"subtitleOcrTracks"`
	// The settings tuning conversions keep their current values when a
	// PUT leaves them out. ConversionHardware is what conversions run on;
	// a PUT cannot change it.
	EncoderPreset          *string   `json:"encoderPreset"`
	H264Quality            *int      `json:"h264Quality"`
	HevcQuality            *int      `json:"hevcQuality"`
	AllowHevcEncoding      *bool     `json:"allowHevcEncoding"`
	AdaptiveQuality        *string   `json:"adaptiveQuality"`
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
	KeepConvertedVideo         *bool                  `json:"keepConvertedVideo"`
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
	// WatchLocalFolders keeps its current value when a PUT leaves it out.
	WatchLocalFolders *bool `json:"watchLocalFolders"`
	// CustomCss, CustomJs and LoginDisclaimer, for jellyfin-web, keep their
	// current values when a PUT leaves them out; empty clears them.
	CustomCss       *string `json:"customCss"`
	CustomJs        *string `json:"customJs"`
	LoginDisclaimer *string `json:"loginDisclaimer"`
	// AddVersionsToOpenPage keeps its current value when a PUT leaves it
	// out.
	AddVersionsToOpenPage *bool `json:"addVersionsToOpenPage"`
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
	// The AniList and MyAnimeList apps are the same: AniListClientSecretSet
	// and MyAnimeListClientSecretSet tell whether their secrets are saved.
	AniListClientID            *string `json:"aniListClientId"`
	AniListClientSecret        *string `json:"aniListClientSecret,omitempty"`
	AniListClientSecretSet     bool    `json:"aniListClientSecretSet"`
	MyAnimeListClientID        *string `json:"myAnimeListClientId"`
	MyAnimeListClientSecret    *string `json:"myAnimeListClientSecret,omitempty"`
	MyAnimeListClientSecretSet bool    `json:"myAnimeListClientSecretSet"`
	// BackupHour, BackupsKept, CollectionReadHour and CollectionLibraryApps
	// keep their current values when a PUT leaves them out.
	BackupHour            *int      `json:"backupHour"`
	BackupsKept           *int      `json:"backupsKept"`
	CollectionReadHour    *int      `json:"collectionReadHour"`
	CollectionLibraryApps *[]string `json:"collectionLibraryApps"`
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
	// WeeklySummaryDay, 0 for Sunday to 6 for Saturday, and
	// WeeklySummaryHour, 0 to 23, when the weekly summary is sent in the
	// server's time zone, keep their current values when a PUT leaves them
	// out.
	WeeklySummaryDay  *int `json:"weeklySummaryDay"`
	WeeklySummaryHour *int `json:"weeklySummaryHour"`
	// ImportEveryMinutes is how often connected accounts are imported
	// again, one of the bounds' choices; it keeps its current value when a
	// PUT leaves it out.
	ImportEveryMinutes *int `json:"importEveryMinutes"`
	// Metrics, whether /metrics answers, keeps its current value when a PUT
	// leaves it out. MetricsTokenSet tells whether Polyfin made the token
	// it needs, which no answer holds; a PUT ignores it (see
	// newMetricsToken).
	Metrics         *bool `json:"metrics"`
	MetricsTokenSet bool  `json:"metricsTokenSet"`
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
		LocalDiscovery:      &settings.LocalDiscovery,
		LocalAddress:        &settings.LocalAddress,
		DLNA:                &settings.DLNA,
		DLNAName:            &settings.DLNAName,
		DLNANewDevicesUser:  dlnaUserJSON(settings.DLNANewDevicesUser),
		DLNAMarkPlayed:      &settings.DLNAMarkPlayed,

		SkipButtons:           &settings.SkipButtons,
		PublicMetaDBKeySet:    settings.PublicMetaDBKey != "",
		TheIntroDBKeySet:      settings.TheIntroDBKey != "",
		SegmentOrder:          settings.SegmentOrder,
		SegmentSourcesOff:     &settings.SegmentSourcesOff,
		AudioIntros:           &settings.AudioIntros,
		SimilarTitles:         &settings.SimilarTitles,
		GenerateImages:        &settings.GenerateImages,
		MDBListRatingsKeySet:  settings.MDBListRatingsKey != "",
		CriticRatingShows:     &settings.CriticRatingShows,
		TMDBKeySet:            settings.TMDBKey != "",
		Lyrics:                &settings.Lyrics,
		EvenVolume:            &settings.EvenVolume,
		MeasureLoudness:       &settings.MeasureLoudness,
		PlayedPercent:         &settings.PlayedPercent,
		ResumePercent:         &settings.ResumePercent,
		VersionListMinutes:    &settings.VersionListMinutes,
		CatalogRefreshMinutes: &settings.CatalogRefreshMinutes,
		PersonalAddons:        &settings.PersonalAddons,
		ServerImports:         &settings.ServerImports,
		ShareLinks:            &settings.ShareLinks,
		LoginAttempts:         &settings.LoginAttempts,
		TwoStepAdministrators: &settings.TwoStepAdministrators,
		InactiveDeviceDays:    &settings.InactiveDeviceDays,
		KnownProxies:          new(accounts.FormatKnownProxies(settings.KnownProxies, settings.KnownProxyNames)),
		HTTPSMode:             &settings.HTTPSMode,
		HTTPSPort:             &settings.HTTPSPort,
		HTTPSRedirect:         &settings.HTTPSRedirect,
		HTTPSCertificate:      &settings.HTTPSCertificate,
		HTTPSPrivateKey:       &settings.HTTPSPrivateKey,
		ACMEHosts:             &settings.ACMEHosts,
		ACMEEmail:             &settings.ACMEEmail,
		ACMETerms:             &settings.ACMETerms,
		ACMEChallenge:         &settings.ACMEChallenge,
		DetailedLog:           &settings.DetailedLog,
		AnalysisTimeout:       &settings.AnalysisTimeout,
		VersionAttempts:       &settings.VersionAttempts,
		PreferDirectPlay:      &settings.PreferDirectPlay,
		VersionOrder:          &settings.VersionOrder,
		VersionCodecOrder:     &settings.VersionCodecOrder,
		HideVersionsLargerGB:  &settings.HideVersionsLargerGB,
		HiddenVersionCodecs:   &settings.HiddenVersionCodecs,
		HiddenVersionWords:    &settings.HiddenVersionWords,
		NameHowVersionsPlay:   &settings.NameHowVersionsPlay,
		SyncAddonSubtitles:    &settings.SyncAddonSubtitles,
		MaxConversions:        &settings.MaxConversions,
		MaxConversionHeight:   &settings.MaxConversionHeight,
		ConvertedDownloads:    &settings.ConvertedDownloads,
		SubtitleOCR:           &settings.SubtitleOCR,
		SubtitleOCRTracks:     &settings.SubtitleOCRTracks,

		EncoderPreset:              &settings.EncoderPreset,
		H264Quality:                &settings.H264Quality,
		HevcQuality:                &settings.HevcQuality,
		AllowHevcEncoding:          &settings.AllowHevcEncoding,
		AdaptiveQuality:            &settings.AdaptiveQuality,
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
		KeepConvertedVideo:         &settings.KeepConvertedVideo,

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
		WatchLocalFolders:      &settings.WatchLocalFolders,

		CustomCss:             &settings.CustomCss,
		CustomJs:              &settings.CustomJs,
		LoginDisclaimer:       &settings.LoginDisclaimer,
		AddVersionsToOpenPage: &settings.AddVersionsToOpenPage,

		TraktClientID:        &settings.TraktClientID,
		TraktClientSecretSet: settings.TraktClientSecret != "",
		SimklClientID:        &settings.SimklClientID,
		LastFMAPIKey:         &settings.LastFMAPIKey,
		LastFMSecretSet:      settings.LastFMSecret != "",

		AniListClientID:            &settings.AniListClientID,
		AniListClientSecretSet:     settings.AniListClientSecret != "",
		MyAnimeListClientID:        &settings.MyAnimeListClientID,
		MyAnimeListClientSecretSet: settings.MyAnimeListClientSecret != "",

		BackupHour:            &settings.BackupHour,
		BackupsKept:           &settings.BackupsKept,
		CollectionReadHour:    &settings.CollectionReadHour,
		CollectionLibraryApps: &settings.CollectionLibraryApps,

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

		WeeklySummaryDay:  &settings.WeeklySummaryDay,
		WeeklySummaryHour: &settings.WeeklySummaryHour,

		ImportEveryMinutes: &settings.ImportEveryMinutes,

		Metrics:         &settings.Metrics,
		MetricsTokenSet: settings.MetricsToken != "",
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

// groupParam reads the group a request names: nil when it names none, the
// zero ID for an empty string, No group. It answers an identifier that is
// not one with invalid_group, and returns false.
func groupParam(w http.ResponseWriter, raw *string) (*accounts.ID, bool) {
	if raw == nil {
		return nil, true
	}
	if *raw == "" {
		return &accounts.ID{}, true
	}
	id, err := accounts.ParseID(*raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_group")
		return nil, false
	}
	return &id, true
}

// groupName is the name of the group id, empty when it no longer exists.
func (h *handler) groupName(r *http.Request, id *accounts.ID) string {
	if id == nil {
		return ""
	}
	group, err := h.Accounts.Group(r.Context(), *id)
	if err != nil {
		return ""
	}
	return group.Name
}

// recordMembership records the changes to a user's group from before to
// after: leaving a group, joining one, or no longer following it.
func (h *handler) recordMembership(r *http.Request, before, after accounts.User) {
	same := func(a, b *accounts.ID) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	if !same(before.GroupID, after.GroupID) {
		if before.GroupID != nil {
			h.Activity.GroupMemberRemoved(r.Context(), after, h.groupName(r, before.GroupID))
		}
		if after.GroupID != nil {
			h.Activity.GroupMemberAdded(r.Context(), after, h.groupName(r, after.GroupID))
		}
		return
	}
	if after.GroupID != nil && before.FollowGroup && !after.FollowGroup {
		h.Activity.GroupMemberDetached(r.Context(), after, h.groupName(r, after.GroupID), "")
	}
}

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Password        string `json:"password"`
		IsAdministrator bool   `json:"isAdministrator"`
		IsHidden        bool   `json:"isHidden"`
		// GroupID is the group the account joins and follows, empty or
		// left out for none.
		GroupID *string `json:"groupId"`
	}
	if !decode(w, r, &body) {
		return
	}
	group, ok := groupParam(w, body.GroupID)
	if !ok {
		return
	}
	if group != nil && *group == (accounts.ID{}) {
		group = nil
	}
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{
		Name: body.Name, Password: body.Password, IsAdministrator: body.IsAdministrator, IsHidden: body.IsHidden, Group: group,
	})
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.UserCreated(r.Context(), user)
	if user.GroupID != nil {
		h.Activity.GroupMemberAdded(r.Context(), user, h.groupName(r, user.GroupID))
	}
	h.writeUser(w, r, http.StatusCreated, user)
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Name            *string `json:"name"`
		Password        *string `json:"password"`
		IsAdministrator *bool   `json:"isAdministrator"`
		IsHidden        *bool   `json:"isHidden"`
		IsDisabled      *bool   `json:"isDisabled"`
		// The settings a group holds, see userJSON; refused while the user
		// follows their group, unless followGroup turns that off.
		limitChanges
		// EndsOn, see userJSON: a date, or null for never.
		EndsOn json.RawMessage `json:"endsOn"`
		// GroupID is the group the user joins and follows, empty for none;
		// FollowGroup has them follow it, or keep its values as their own.
		GroupID     *string `json:"groupId"`
		FollowGroup *bool   `json:"followGroup"`
		// MetadataLanguage, see userJSON.
		MetadataLanguage *string `json:"metadataLanguage"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	var keep []byte
	if id == session.User.ID {
		keep = session.TokenHash
	}
	changes, ok := body.changes(w)
	if !ok {
		return
	}
	changes.MetadataLanguage = body.MetadataLanguage
	if body.MetadataLanguage != nil && !h.metadataLanguageOffered(w, r, *body.MetadataLanguage) {
		return
	}
	changes.Name = body.Name
	changes.Password = body.Password
	changes.IsAdministrator = body.IsAdministrator
	changes.IsHidden = body.IsHidden
	changes.IsDisabled = body.IsDisabled
	changes.FollowGroup = body.FollowGroup
	if changes.Group, ok = groupParam(w, body.GroupID); !ok {
		return
	}
	if body.EndsOn != nil {
		end, ok := endDate(body.EndsOn)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_ends_on")
			return
		}
		changes.EndsOn = &end
	}
	before, err := h.Accounts.User(r.Context(), id)
	var user accounts.User
	if err == nil {
		user, err = h.Accounts.UpdateUser(r.Context(), id, changes, keep)
	}
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
	h.recordMembership(r, before, user)
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

// turnOffDownloads takes the permission to download away from every user
// and every group, and answers with the users who had it.
func (h *handler) turnOffDownloads(w http.ResponseWriter, r *http.Request) {
	changed, groups, err := h.Accounts.TurnOffDownloads(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, group := range groups {
		h.Activity.GroupChanged(r.Context(), group, []string{accounts.SectionAccess})
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

// settingsApps answers the apps an administrator may choose among in the
// settings: those every user is signed in with (see accounts.DeviceApps).
func (h *handler) settingsApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Accounts.DeviceApps(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"apps": apps})
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
	dlnaUser, ok := h.dlnaNewDevicesUser(w, r, body.DLNANewDevicesUser, current.DLNANewDevicesUser)
	if !ok {
		return
	}
	theIntroDBKey, ok := h.segmentKey(w, r, mediasegments.TheIntroDB, body.TheIntroDBKey, current.TheIntroDBKey)
	if !ok {
		return
	}
	ratingsKey, ok := h.ratingsKey(w, r, body.MDBListRatingsKey, current.MDBListRatingsKey)
	if !ok {
		return
	}
	tmdbKey, ok := h.tmdbKey(w, r, body.TMDBKey, current.TMDBKey)
	if !ok {
		return
	}
	// An empty order is what older pages sent to follow POLYFIN_SEGMENTS,
	// which the settings now hold: it keeps the saved one.
	segmentOrder := current.SegmentOrder
	if len(body.SegmentOrder) > 0 {
		segmentOrder = body.SegmentOrder
	}
	knownProxies, knownProxyNames := current.KnownProxies, current.KnownProxyNames
	if body.KnownProxies != nil {
		proxies, names, err := accounts.ParseKnownProxies(*body.KnownProxies)
		if accountError(w, err) {
			return
		}
		knownProxies, knownProxyNames = proxies, names
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
		LocalDiscovery:      valueOr(body.LocalDiscovery, current.LocalDiscovery),
		LocalAddress:        valueOr(body.LocalAddress, current.LocalAddress),
		DLNA:                valueOr(body.DLNA, current.DLNA),
		DLNAName:            valueOr(body.DLNAName, current.DLNAName),
		DLNANewDevicesUser:  dlnaUser,
		DLNAMarkPlayed:      valueOr(body.DLNAMarkPlayed, current.DLNAMarkPlayed),

		SkipButtons:           valueOr(body.SkipButtons, current.SkipButtons),
		PublicMetaDBKey:       publicMetaDBKey,
		TheIntroDBKey:         theIntroDBKey,
		SegmentOrder:          segmentOrder,
		SegmentSourcesOff:     valueOr(body.SegmentSourcesOff, current.SegmentSourcesOff),
		AudioIntros:           valueOr(body.AudioIntros, current.AudioIntros),
		SimilarTitles:         valueOr(body.SimilarTitles, current.SimilarTitles),
		GenerateImages:        valueOr(body.GenerateImages, current.GenerateImages),
		MDBListRatingsKey:     ratingsKey,
		CriticRatingShows:     valueOr(body.CriticRatingShows, current.CriticRatingShows),
		TMDBKey:               tmdbKey,
		Lyrics:                valueOr(body.Lyrics, current.Lyrics),
		EvenVolume:            valueOr(body.EvenVolume, current.EvenVolume),
		MeasureLoudness:       valueOr(body.MeasureLoudness, current.MeasureLoudness),
		PlayedPercent:         valueOr(body.PlayedPercent, current.PlayedPercent),
		ResumePercent:         valueOr(body.ResumePercent, current.ResumePercent),
		VersionListMinutes:    valueOr(body.VersionListMinutes, current.VersionListMinutes),
		CatalogRefreshMinutes: valueOr(body.CatalogRefreshMinutes, current.CatalogRefreshMinutes),
		PersonalAddons:        valueOr(body.PersonalAddons, current.PersonalAddons),
		ServerImports:         valueOr(body.ServerImports, current.ServerImports),
		ShareLinks:            valueOr(body.ShareLinks, current.ShareLinks),
		LoginAttempts:         valueOr(body.LoginAttempts, current.LoginAttempts),
		TwoStepAdministrators: valueOr(body.TwoStepAdministrators, current.TwoStepAdministrators),
		InactiveDeviceDays:    valueOr(body.InactiveDeviceDays, current.InactiveDeviceDays),
		KnownProxies:          knownProxies,
		KnownProxyNames:       knownProxyNames,
		HTTPSMode:             valueOr(body.HTTPSMode, current.HTTPSMode),
		HTTPSPort:             valueOr(body.HTTPSPort, current.HTTPSPort),
		HTTPSRedirect:         valueOr(body.HTTPSRedirect, current.HTTPSRedirect),
		HTTPSCertificate:      valueOr(body.HTTPSCertificate, current.HTTPSCertificate),
		HTTPSPrivateKey:       valueOr(body.HTTPSPrivateKey, current.HTTPSPrivateKey),
		ACMEHosts:             valueOr(body.ACMEHosts, current.ACMEHosts),
		ACMEEmail:             valueOr(body.ACMEEmail, current.ACMEEmail),
		ACMETerms:             valueOr(body.ACMETerms, current.ACMETerms),
		ACMEChallenge:         valueOr(body.ACMEChallenge, current.ACMEChallenge),
		DetailedLog:           valueOr(body.DetailedLog, current.DetailedLog),
		AnalysisTimeout:       valueOr(body.AnalysisTimeout, current.AnalysisTimeout),
		VersionAttempts:       valueOr(body.VersionAttempts, current.VersionAttempts),
		PreferDirectPlay:      valueOr(body.PreferDirectPlay, current.PreferDirectPlay),
		VersionOrder:          valueOr(body.VersionOrder, current.VersionOrder),
		VersionCodecOrder:     valueOr(body.VersionCodecOrder, current.VersionCodecOrder),
		HideVersionsLargerGB:  valueOr(body.HideVersionsLargerGB, current.HideVersionsLargerGB),
		HiddenVersionCodecs:   valueOr(body.HiddenVersionCodecs, current.HiddenVersionCodecs),
		HiddenVersionWords:    valueOr(body.HiddenVersionWords, current.HiddenVersionWords),
		NameHowVersionsPlay:   valueOr(body.NameHowVersionsPlay, current.NameHowVersionsPlay),
		SyncAddonSubtitles:    valueOr(body.SyncAddonSubtitles, current.SyncAddonSubtitles),
		MaxConversions:        valueOr(body.MaxConversions, current.MaxConversions),
		MaxConversionHeight:   valueOr(body.MaxConversionHeight, current.MaxConversionHeight),
		ConvertedDownloads:    valueOr(body.ConvertedDownloads, current.ConvertedDownloads),
		SubtitleOCR:           valueOr(body.SubtitleOCR, current.SubtitleOCR),
		SubtitleOCRTracks:     valueOr(body.SubtitleOCRTracks, current.SubtitleOCRTracks),

		EncoderPreset:              valueOr(body.EncoderPreset, current.EncoderPreset),
		H264Quality:                valueOr(body.H264Quality, current.H264Quality),
		HevcQuality:                valueOr(body.HevcQuality, current.HevcQuality),
		AllowHevcEncoding:          valueOr(body.AllowHevcEncoding, current.AllowHevcEncoding),
		AdaptiveQuality:            valueOr(body.AdaptiveQuality, current.AdaptiveQuality),
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
		KeepConvertedVideo:         valueOr(body.KeepConvertedVideo, current.KeepConvertedVideo),

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
		WatchLocalFolders:      valueOr(body.WatchLocalFolders, current.WatchLocalFolders),

		CustomCss:             valueOr(body.CustomCss, current.CustomCss),
		CustomJs:              valueOr(body.CustomJs, current.CustomJs),
		LoginDisclaimer:       valueOr(body.LoginDisclaimer, current.LoginDisclaimer),
		AddVersionsToOpenPage: valueOr(body.AddVersionsToOpenPage, current.AddVersionsToOpenPage),

		TraktClientID:     valueOr(body.TraktClientID, current.TraktClientID),
		TraktClientSecret: valueOr(body.TraktClientSecret, current.TraktClientSecret),
		SimklClientID:     valueOr(body.SimklClientID, current.SimklClientID),
		LastFMAPIKey:      valueOr(body.LastFMAPIKey, current.LastFMAPIKey),
		LastFMSecret:      valueOr(body.LastFMSecret, current.LastFMSecret),

		AniListClientID:         valueOr(body.AniListClientID, current.AniListClientID),
		AniListClientSecret:     valueOr(body.AniListClientSecret, current.AniListClientSecret),
		MyAnimeListClientID:     valueOr(body.MyAnimeListClientID, current.MyAnimeListClientID),
		MyAnimeListClientSecret: valueOr(body.MyAnimeListClientSecret, current.MyAnimeListClientSecret),

		BackupHour:            valueOr(body.BackupHour, current.BackupHour),
		BackupsKept:           valueOr(body.BackupsKept, current.BackupsKept),
		CollectionReadHour:    valueOr(body.CollectionReadHour, current.CollectionReadHour),
		CollectionLibraryApps: valueOr(body.CollectionLibraryApps, current.CollectionLibraryApps),

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

		WeeklySummaryDay:  valueOr(body.WeeklySummaryDay, current.WeeklySummaryDay),
		WeeklySummaryHour: valueOr(body.WeeklySummaryHour, current.WeeklySummaryHour),

		ImportEveryMinutes: valueOr(body.ImportEveryMinutes, current.ImportEveryMinutes),

		Metrics:      valueOr(body.Metrics, current.Metrics),
		MetricsToken: current.MetricsToken,
	}
	if code := h.unwritableFolder(next, current); code != "" {
		writeError(w, http.StatusBadRequest, code)
		return
	}
	if code := h.unreadableCertificate(next, current); code != "" {
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

// ratingsKey is the MDBList key for ratings a settings PUT saves, as
// segmentKey tells the key of a segment database: one MDBList refuses, or
// that is malformed, is answered 400 invalid_mdblist_ratings_key, and one
// MDBList could not be asked about 502 mdblist_unreachable.
func (h *handler) ratingsKey(w http.ResponseWriter, r *http.Request, sent *string, current string) (key string, ok bool) {
	if sent == nil {
		return current, true
	}
	key = strings.TrimSpace(*sent)
	if *sent == "" || key == current {
		return key, true
	}
	if key == "" || !accounts.ValidRatingsKey(key) {
		writeError(w, http.StatusBadRequest, "invalid_mdblist_ratings_key")
		return "", false
	}
	switch err := h.Ratings.CheckKey(r.Context(), key); {
	case errors.Is(err, ratings.ErrKeyRefused):
		writeError(w, http.StatusBadRequest, "invalid_mdblist_ratings_key")
		return "", false
	case err != nil:
		h.Logger.Warn("MDBList could not check the key for ratings", "error", err)
		writeError(w, http.StatusBadGateway, "mdblist_unreachable")
		return "", false
	}
	return key, true
}

// tmdbKey is the TMDB key a settings PUT saves, as ratingsKey tells the
// MDBList key: one TMDB refuses, or that is malformed, is answered 400
// invalid_tmdb_key, and one TMDB could not be asked about 502
// tmdb_unreachable.
func (h *handler) tmdbKey(w http.ResponseWriter, r *http.Request, sent *string, current string) (key string, ok bool) {
	if sent == nil {
		return current, true
	}
	key = strings.TrimSpace(*sent)
	if *sent == "" || key == current {
		return key, true
	}
	if key == "" || !accounts.ValidTMDBKey(key) {
		writeError(w, http.StatusBadRequest, "invalid_tmdb_key")
		return "", false
	}
	if h.TMDB == nil {
		writeError(w, http.StatusBadGateway, "tmdb_unreachable")
		return "", false
	}
	switch err := h.TMDB.CheckKey(r.Context(), key); {
	case errors.Is(err, tmdb.ErrKeyRefused):
		writeError(w, http.StatusBadRequest, "invalid_tmdb_key")
		return "", false
	case err != nil:
		h.Logger.Warn("TMDB could not check the key", "error", err)
		writeError(w, http.StatusBadGateway, "tmdb_unreachable")
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
