package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Server configuration.
//
// Jellyfin apps read the server configuration whole, and its dashboard
// saves it whole. Polyfin answers with its own settings where Jellyfin has
// them: the server name, its language, the resume thresholds, Quick
// Connect and the legacy authorization; everything else is a new Jellyfin
// server's default. Saving applies those of Polyfin's settings and ignores
// the rest.

// MetadataOptions are Jellyfin's metadata settings of an item type.
type MetadataOptions struct {
	ItemType                 string
	DisabledMetadataSavers   []string
	LocalMetadataReaderOrder []string
	DisabledMetadataFetchers []string
	MetadataFetcherOrder     []string
	DisabledImageFetchers    []string
	ImageFetcherOrder        []string
}

// TrickplayOptions are Jellyfin's trickplay settings.
type TrickplayOptions struct {
	EnableHwAcceleration         bool
	EnableHwEncoding             bool
	EnableKeyFrameOnlyExtraction bool
	ScanBehavior                 string
	ProcessPriority              string
	Interval                     int
	WidthResolutions             []int
	TileWidth                    int
	TileHeight                   int
	Qscale                       int
	JpegQuality                  int
	ProcessThreads               int
}

// ServerConfiguration is Jellyfin's server configuration, in its order.
type ServerConfiguration struct {
	EnableMetrics                       bool
	EnableNormalizedItemByNameIds       bool
	IsPortAuthorized                    bool
	QuickConnectAvailable               bool
	EnableCaseSensitiveItemIds          bool
	DisableLiveTvChannelUserDataName    bool
	MetadataPath                        string
	PreferredMetadataLanguage           string
	MetadataCountryCode                 string
	SortReplaceCharacters               []string
	SortRemoveCharacters                []string
	SortRemoveWords                     []string
	MinResumePct                        int
	MaxResumePct                        int
	MinResumeDurationSeconds            int
	MinAudiobookResume                  int
	MaxAudiobookResume                  int
	InactiveSessionThreshold            int
	LibraryMonitorDelay                 int
	LibraryUpdateDuration               int
	CacheSize                           int
	ImageSavingConvention               string
	MetadataOptions                     []MetadataOptions
	IsStartupWizardCompleted            bool
	SkipDeserializationForBasicTypes    bool
	ServerName                          string
	UICulture                           string
	SaveMetadataHidden                  bool
	ContentTypes                        []struct{}
	RemoteClientBitrateLimit            int
	EnableFolderView                    bool
	EnableGroupingMoviesIntoCollections bool
	EnableGroupingShowsIntoCollections  bool
	DisplaySpecialsWithinSeasons        bool
	CodecsUsed                          []string
	PluginRepositories                  []struct{}
	EnableExternalContentInSuggestions  bool
	ImageExtractionTimeoutMs            int
	PathSubstitutions                   []struct{}
	EnableSlowResponseWarning           bool
	SlowResponseThresholdMs             int
	CorsHosts                           []string
	ActivityLogRetentionDays            int
	LibraryScanFanoutConcurrency        int
	LibraryMetadataRefreshConcurrency   int
	AllowClientLogUpload                bool
	DummyChapterDuration                int
	ChapterImageResolution              string
	ParallelImageEncodingLimit          int
	CastReceiverApplications            []CastReceiverApplication
	TrickplayOptions                    TrickplayOptions
	EnableLegacyAuthorization           bool
	LogFileRetentionDays                int
}

// metadataOptions are a new Jellyfin server's metadata settings.
func metadataOptions() []MetadataOptions {
	types := []struct {
		itemType          string
		metadata, artwork []string
	}{
		{itemType: "Book"}, {itemType: "Movie"},
		{itemType: "MusicVideo", metadata: []string{"The Open Movie Database"}, artwork: []string{"The Open Movie Database"}},
		{itemType: "Series"}, {itemType: "MusicAlbum", metadata: []string{"TheAudioDB"}},
		{itemType: "MusicArtist", metadata: []string{"TheAudioDB"}}, {itemType: "BoxSet"}, {itemType: "Season"}, {itemType: "Episode"},
	}
	options := make([]MetadataOptions, 0, len(types))
	for _, t := range types {
		options = append(options, MetadataOptions{ItemType: t.itemType, DisabledMetadataSavers: []string{}, LocalMetadataReaderOrder: []string{},
			DisabledMetadataFetchers: append([]string{}, t.metadata...), MetadataFetcherOrder: []string{},
			DisabledImageFetchers: append([]string{}, t.artwork...), ImageFetcherOrder: []string{}})
	}
	return options
}

// uiCulture is the culture Jellyfin names a server language by.
func uiCulture(language string) string {
	if language == "en" {
		return "en-US"
	}
	return language
}

// cultureLanguage is the server language of a culture, empty when Polyfin
// does not speak it.
func cultureLanguage(culture string) string {
	language, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(culture)), "-")
	if accounts.ValidLanguage(language) {
		return language
	}
	return ""
}

// serverConfiguration answers the server configuration to any signed-in
// user, as Jellyfin does.
func (h *Handler) serverConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, ServerConfiguration{
		EnableNormalizedItemByNameIds:      true,
		IsPortAuthorized:                   true,
		QuickConnectAvailable:              settings.QuickConnectEnabled,
		EnableCaseSensitiveItemIds:         true,
		DisableLiveTvChannelUserDataName:   true,
		PreferredMetadataLanguage:          "en",
		MetadataCountryCode:                "US",
		SortReplaceCharacters:              []string{".", "+", "%"},
		SortRemoveCharacters:               []string{",", "&", "-", "{", "}", "'"},
		SortRemoveWords:                    []string{"the", "a", "an"},
		MinResumePct:                       settings.ResumePercent,
		MaxResumePct:                       settings.PlayedPercent,
		MinResumeDurationSeconds:           int(userdata.MinResumeDuration.Seconds()),
		MinAudiobookResume:                 5,
		MaxAudiobookResume:                 5,
		LibraryMonitorDelay:                60,
		LibraryUpdateDuration:              30,
		CacheSize:                          runtime.NumCPU() * 100,
		ImageSavingConvention:              "Legacy",
		MetadataOptions:                    metadataOptions(),
		IsStartupWizardCompleted:           true,
		SkipDeserializationForBasicTypes:   true,
		ServerName:                         settings.ServerName,
		UICulture:                          uiCulture(settings.Language),
		ContentTypes:                       []struct{}{},
		DisplaySpecialsWithinSeasons:       true,
		CodecsUsed:                         []string{},
		PluginRepositories:                 []struct{}{},
		EnableExternalContentInSuggestions: true,
		PathSubstitutions:                  []struct{}{},
		EnableSlowResponseWarning:          true,
		SlowResponseThresholdMs:            500,
		CorsHosts:                          []string{"*"},
		ActivityLogRetentionDays:           int(activity.Retention.Hours() / 24),
		AllowClientLogUpload:               true,
		ChapterImageResolution:             "MatchSource",
		CastReceiverApplications:           castReceivers,
		TrickplayOptions: TrickplayOptions{ScanBehavior: "NonBlocking", ProcessPriority: "BelowNormal", Interval: 10000,
			WidthResolutions: []int{320}, TileWidth: 10, TileHeight: 10, Qscale: 4, JpegQuality: 90, ProcessThreads: 1},
		EnableLegacyAuthorization: settings.LegacyAuthorization,
		LogFileRetentionDays:      3,
	})
}

// updateServerConfiguration applies the parts of a configuration Polyfin
// has: the server name, its language (UICulture, when Polyfin speaks it),
// the resume thresholds, Quick Connect and the legacy authorization.
// Unlike Jellyfin, which replaces the configuration whole, a part left out
// is kept, as is an empty server name; MinResumeDurationSeconds is fixed.
// Values Polyfin's settings refuse are refused, as the admin interface
// refuses them.
func (h *Handler) updateServerConfiguration(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "configuration", errs)
	if !ok {
		return
	}
	var body struct {
		ServerName                *string
		UICulture                 *string
		MinResumePct              *int
		MaxResumePct              *int
		QuickConnectAvailable     *bool
		EnableLegacyAuthorization *bool
	}
	if raw != nil && json.Unmarshal(raw, &body) != nil {
		errs.add("$", "The JSON value could not be converted.")
		errs.add("configuration", "The configuration field is required.")
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	settings := h.Accounts.Settings()
	if body.ServerName != nil && strings.TrimSpace(*body.ServerName) != "" {
		settings.ServerName = *body.ServerName
	}
	if body.UICulture != nil {
		if language := cultureLanguage(*body.UICulture); language != "" {
			settings.Language = language
		}
	}
	if body.MinResumePct != nil {
		settings.ResumePercent = *body.MinResumePct
	}
	if body.MaxResumePct != nil {
		settings.PlayedPercent = *body.MaxResumePct
	}
	if body.QuickConnectAvailable != nil {
		settings.QuickConnectEnabled = *body.QuickConnectAvailable
	}
	if body.EnableLegacyAuthorization != nil {
		settings.LegacyAuthorization = *body.EnableLegacyAuthorization
	}
	if _, err := h.Accounts.UpdateSettings(r.Context(), settings); err != nil {
		for _, refused := range []struct {
			err     error
			field   string
			message string
		}{
			{accounts.ErrInvalidServerName, "ServerName", "The server name must have at most 64 printable characters."},
			{accounts.ErrInvalidResumePercent, "MinResumePct", "MinResumePct must be from 0 to 50."},
			{accounts.ErrInvalidPlayedPercent, "MaxResumePct", "MaxResumePct must be from 50 to 100."},
			{accounts.ErrResumeNotBelowPlayed, "MinResumePct", "MinResumePct must be below MaxResumePct."},
		} {
			if errors.Is(err, refused.err) {
				validationProblem(w, map[string][]string{refused.field: {refused.message}})
				return
			}
		}
		h.internalError(w, r, err)
		return
	}
	h.Activity.SettingsSaved(r.Context(), actor(callerFrom(r.Context())))
	// As when the admin interface turns it off, pending requests end.
	if !h.Accounts.Settings().QuickConnectEnabled {
		h.QuickConnect.Clear()
	}
	w.WriteHeader(http.StatusNoContent)
}

// mediaFolders lists the caller's libraries as Jellyfin's MediaFolders,
// which tools sync libraries from: those the user sees, or the server's
// for an API key without a user. Libraries are never hidden, which
// isHidden filters on.
func (h *Handler) mediaFolders(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	hidden, hiddenSet := b.bool(r, "isHidden")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	views := []BaseItemDto{}
	if !hiddenSet || !hidden {
		var err error
		if views, err = h.libraryViews(r, callerFrom(r.Context()).User); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: views, TotalRecordCount: len(views)})
}

// FolderStorageDto describes the disk a folder is on.
type FolderStorageDto struct {
	Path        string
	FreeSpace   int64
	UsedSpace   int64
	StorageType string `json:",omitempty"`
	DeviceId    string `json:",omitempty"`
}

// LibraryStorageDto describes the folders of a library.
type LibraryStorageDto struct {
	Id      string
	Name    string
	Folders []FolderStorageDto
}

// SystemStorageDto is Jellyfin's storage information.
type SystemStorageDto struct {
	ProgramDataFolder      FolderStorageDto
	WebFolder              FolderStorageDto
	ImageCacheFolder       FolderStorageDto
	CacheFolder            FolderStorageDto
	LogFolder              FolderStorageDto
	InternalMetadataFolder FolderStorageDto
	TranscodingTempFolder  FolderStorageDto
	Libraries              []LibraryStorageDto
}

// recordingsLibrary identifies the recordings folder among the libraries
// of the storage information.
const recordingsLibrary = "5f8d0b9f2a6e4c1b9d7e3a4c6b8f0e12"

// folderStorage describes the disk path is on; a path that is not set, or
// cannot be measured, has the space Jellyfin gives an unknown folder.
func folderStorage(path string) FolderStorageDto {
	folder := FolderStorageDto{Path: path, FreeSpace: -1, UsedSpace: -1}
	if path == "" {
		return folder
	}
	if free, used, mount, ok := diskspace.Measure(path); ok {
		folder.FreeSpace, folder.UsedSpace, folder.StorageType, folder.DeviceId = free, used, "Fixed", mount
	}
	return folder
}

// storage describes the folders Polyfin writes to: its cache, where
// sources and remuxes are kept, its segments, where conversions are, and,
// when recordings are made, their folder, as a library. Polyfin keeps no
// program data, web files, logs, metadata or artwork on disk: those
// folders are left empty and unmeasured.
func (h *Handler) storage(w http.ResponseWriter, _ *http.Request) {
	result := SystemStorageDto{
		ProgramDataFolder:      folderStorage(""),
		WebFolder:              folderStorage(""),
		ImageCacheFolder:       folderStorage(""),
		CacheFolder:            folderStorage(h.CacheDir),
		LogFolder:              folderStorage(""),
		InternalMetadataFolder: folderStorage(""),
		TranscodingTempFolder:  folderStorage(""),
		Libraries:              []LibraryStorageDto{},
	}
	if h.CacheDir != "" {
		result.TranscodingTempFolder = folderStorage(filepath.Join(h.CacheDir, "segments"))
	}
	if dir := h.Recordings.Dir(); dir != "" {
		name := "Recordings"
		if h.Accounts.Settings().Language == "fr" {
			name = "Enregistrements"
		}
		result.Libraries = append(result.Libraries, LibraryStorageDto{Id: recordingsLibrary, Name: name,
			Folders: []FolderStorageDto{folderStorage(dir)}})
	}
	writeJSON(w, http.StatusOK, result)
}
