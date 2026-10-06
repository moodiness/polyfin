package jellyfin

import (
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// absentRoutes registers dashboard features Polyfin does without: plugins,
// packages and their repositories, browsing the server's file system,
// library folders and metadata providers, Live TV tuners and listing
// providers, the startup wizard and backups. Each answers as a Jellyfin 12.2
// on which nothing of the kind is configured, so that apps probing them get
// empty lists and defaults rather than 404, and writes are refused the way
// Jellyfin refuses them for things that do not exist. No write here claims
// to have done anything it did not do.
func (h *Handler) absentRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	// Jellyfin's RequiresElevation, and FirstTimeSetupOrElevated once the
	// setup is complete (Polyfin's always is): an empty 403 for anyone but
	// an administrator.
	administrator := func(method, pattern string, handler http.HandlerFunc) {
		signedIn(method, pattern, func(w http.ResponseWriter, r *http.Request) {
			if !callerFrom(r.Context()).User.IsAdministrator {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		})
	}
	liveTv := func(method, pattern string, handler http.HandlerFunc) {
		signedIn(method, pattern, liveTvAccess(handler))
	}

	// Plugins: none is installed, so every plugin is unknown.
	administrator(http.MethodGet, "/Plugins", emptyList)
	administrator(http.MethodPost, "/Plugins/{pluginId}/{version}/Enable", unknownPlugin)
	administrator(http.MethodPost, "/Plugins/{pluginId}/{version}/Disable", unknownPlugin)
	administrator(http.MethodDelete, "/Plugins/{pluginId}/{version}", unknownPlugin)
	administrator(http.MethodDelete, "/Plugins/{pluginId}", unknownPlugin)
	administrator(http.MethodGet, "/Plugins/{pluginId}/Configuration", unknownPlugin)
	administrator(http.MethodPost, "/Plugins/{pluginId}/Configuration", unknownPlugin)
	administrator(http.MethodGet, "/Plugins/{pluginId}/{version}/Image", unknownPlugin)
	administrator(http.MethodPost, "/Plugins/{pluginId}/Manifest", unknownPlugin)

	// Packages: with no repository there is no package to show or install.
	// Cancelling an installation that is not running is done, as in Jellyfin.
	administrator(http.MethodGet, "/Packages", emptyList)
	administrator(http.MethodGet, "/Packages/{name}", notFound)
	administrator(http.MethodPost, "/Packages/Installed/{name}", notFound)
	administrator(http.MethodDelete, "/Packages/Installing/{packageId}", noContent)
	administrator(http.MethodGet, "/Repositories", emptyList)
	// Jellyfin saves the repository list. Polyfin installs no plugins and
	// keeps no repositories, so it refuses rather than pretend to save one.
	administrator(http.MethodPost, "/Repositories", refused)

	// The server's file system: Polyfin's libraries are not folders, so it
	// shows nothing of the host. Deliberate difference: Jellyfin lists its
	// drives and the directories asked for, and validates existing paths.
	administrator(http.MethodGet, "/Environment/DefaultDirectoryBrowser", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct{}{})
	})
	administrator(http.MethodGet, "/Environment/DirectoryContents", needsPath(emptyList))
	administrator(http.MethodGet, "/Environment/Drives", emptyList)
	administrator(http.MethodGet, "/Environment/ParentPath", needsPath(parentPath))
	administrator(http.MethodPost, "/Environment/ValidatePath", notFound)

	// Libraries: no physical folder, and no metadata provider to offer.
	administrator(http.MethodGet, "/Library/PhysicalPaths", emptyList)
	signedIn(http.MethodGet, "/Libraries/AvailableOptions", availableLibraryOptions)
	administrator(http.MethodGet, "/System/Configuration/MetadataOptions/Default", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, defaultMetadataOptions())
	})

	// Live TV tuners and listing providers: Polyfin's channels come from
	// the users' catalogs, never from a tuner or a listing provider.
	administrator(http.MethodPost, "/LiveTv/TunerHosts", jellyfinError(http.StatusNotFound))
	administrator(http.MethodDelete, "/LiveTv/TunerHosts", noContent)
	// Deliberate difference: Jellyfin lists its tuner types; Polyfin has none.
	liveTv(http.MethodGet, "/LiveTv/TunerHosts/Types", emptyList)
	administrator(http.MethodGet, "/LiveTv/Tuners/Discover", emptyList)
	administrator(http.MethodGet, "/LiveTv/Tuners/Discvover", emptyList)
	administrator(http.MethodPost, "/LiveTv/Tuners/{tunerId}/Reset", jellyfinError(http.StatusBadRequest))
	administrator(http.MethodPost, "/LiveTv/ListingProviders", jellyfinError(http.StatusNotFound))
	administrator(http.MethodDelete, "/LiveTv/ListingProviders", noContent)
	liveTv(http.MethodGet, "/LiveTv/ListingProviders/Default", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, defaultListingProvider())
	})
	liveTv(http.MethodGet, "/LiveTv/ListingProviders/Lineups", jellyfinError(http.StatusNotFound))
	// Jellyfin downloads this list from the listing provider; Polyfin
	// contacts no provider and fails as Jellyfin does when it cannot.
	administrator(http.MethodGet, "/LiveTv/ListingProviders/SchedulesDirect/Countries", jellyfinError(http.StatusInternalServerError))
	administrator(http.MethodGet, "/LiveTv/ChannelMappingOptions", jellyfinError(http.StatusInternalServerError))
	administrator(http.MethodPost, "/LiveTv/ChannelMappings", jellyfinError(http.StatusInternalServerError))

	// The startup wizard: Polyfin's setup is always complete.
	administrator(http.MethodGet, "/Startup/Configuration", h.startupConfiguration)
	administrator(http.MethodGet, "/Startup/User", h.startupUser)
	administrator(http.MethodGet, "/Startup/FirstUser", h.startupUser)
	// Completing a complete setup changes nothing and is done.
	administrator(http.MethodPost, "/Startup/Complete", noContent)
	// Jellyfin refuses to set the first user once it has a password, as
	// Polyfin's administrators always do. Deliberate difference: Jellyfin
	// still saves the wizard's configuration and remote access; Polyfin
	// refuses them, its settings being changed by /System/Configuration
	// and its administration app.
	administrator(http.MethodPost, "/Startup/User", refused)
	administrator(http.MethodPost, "/Startup/Configuration", refused)
	administrator(http.MethodPost, "/Startup/RemoteAccess", refused)

	// Backups: there are none. Deliberate difference: Jellyfin creates one
	// on request; Polyfin's database is backed up outside the server, so it
	// refuses.
	administrator(http.MethodGet, "/Backup", emptyList)
	administrator(http.MethodPost, "/Backup/Create", refused)
	administrator(http.MethodGet, "/Backup/Manifest", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := queryParam(r, "path"); !ok {
			validationProblem(w, map[string][]string{"path": {"A value for the 'path' parameter or property was not provided."}})
			return
		}
		notFoundProblem(w)
	})
	administrator(http.MethodPost, "/Backup/Restore", notFound)
}

func emptyList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []struct{}{})
}

func noContent(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	notFoundProblem(w)
}

// refused answers a write Polyfin does not perform: an empty 403, as
// Jellyfin's Forbid().
func refused(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusForbidden)
}

// jellyfinError answers as Jellyfin's exception middleware does when the
// manager behind the route fails for want of the thing asked for.
func jellyfinError(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		processingError(w, status)
	}
}

// unknownPlugin answers for a plugin that is not installed, after checking
// the identifier as ASP.NET binds a Guid.
func unknownPlugin(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("pluginId")
	if _, ok := parseGUID(raw); !ok {
		validationProblem(w, map[string][]string{"pluginId": {notValid(raw)}})
		return
	}
	notFoundProblem(w)
}

// needsPath requires the path query parameter, as Jellyfin's [Required].
func needsPath(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if value, _ := queryParam(r, "path"); value == "" {
			validationProblem(w, map[string][]string{"path": {"The path field is required."}})
			return
		}
		next(w, r)
	}
}

// parentPath computes the parent of a path as .NET's
// Path.GetDirectoryName does on Linux, without looking at the disk.
func parentPath(w http.ResponseWriter, r *http.Request) {
	path := query(r, "path")
	var parent *string
	if path != "/" {
		value := ""
		if i := strings.LastIndexByte(path, '/'); i == 0 {
			value = "/"
		} else if i > 0 {
			value = path[:i]
		}
		parent = &value
	}
	writeJSON(w, http.StatusOK, parent)
}

type libraryOptionInfo struct {
	Name           string
	DefaultEnabled bool
}

type imageOption struct {
	Type     string
	Limit    int
	MinWidth int
}

type libraryTypeOptions struct {
	Type                 string
	MetadataFetchers     []libraryOptionInfo
	ImageFetchers        []libraryOptionInfo
	SimilarItemProviders []libraryOptionInfo
	SupportedImageTypes  []string
	DefaultImageOptions  []imageOption
}

type libraryOptionsResult struct {
	MetadataSavers        []libraryOptionInfo
	MetadataReaders       []libraryOptionInfo
	SubtitleFetchers      []libraryOptionInfo
	LyricFetchers         []libraryOptionInfo
	MediaSegmentProviders []libraryOptionInfo
	TypeOptions           []libraryTypeOptions
}

// representativeItemTypes are the item types Jellyfin offers options for,
// by library content type.
var representativeItemTypes = map[string][]string{
	"boxsets":     {"BoxSet"},
	"playlists":   {"Playlist"},
	"movies":      {"Movie"},
	"tvshows":     {"Series", "Season", "Episode"},
	"books":       {"Book", "AudioBook"},
	"music":       {"MusicArtist", "MusicAlbum", "Audio", "MusicVideo"},
	"homevideos":  {"Video", "Photo"},
	"photos":      {"Video", "Photo"},
	"musicvideos": {"MusicVideo"},
}

// defaultImageOptions is Jellyfin's TypeOptions.DefaultImageOptions.
var defaultImageOptions = map[string][]imageOption{
	"Movie":       {{"Backdrop", 1, 1280}, {"Art", 0, 0}, {"Disc", 0, 0}, {"Primary", 1, 0}, {"Banner", 0, 0}, {"Thumb", 1, 0}, {"Logo", 1, 0}},
	"MusicVideo":  {{"Backdrop", 1, 1280}, {"Art", 0, 0}, {"Disc", 0, 0}, {"Primary", 1, 0}, {"Banner", 0, 0}, {"Thumb", 1, 0}, {"Logo", 1, 0}},
	"Series":      {{"Backdrop", 1, 1280}, {"Art", 0, 0}, {"Primary", 1, 0}, {"Banner", 1, 0}, {"Thumb", 1, 0}, {"Logo", 1, 0}},
	"MusicAlbum":  {{"Backdrop", 0, 1280}, {"Disc", 0, 0}},
	"MusicArtist": {{"Backdrop", 1, 1280}, {"Banner", 0, 0}, {"Art", 0, 0}, {"Logo", 1, 0}},
	"BoxSet":      {{"Backdrop", 1, 1280}, {"Primary", 1, 0}, {"Thumb", 1, 0}, {"Logo", 1, 0}, {"Art", 0, 0}, {"Disc", 0, 0}, {"Banner", 0, 0}},
	"Season":      {{"Backdrop", 0, 1280}, {"Primary", 1, 0}, {"Banner", 0, 0}, {"Thumb", 0, 0}},
	"Episode":     {{"Backdrop", 0, 1280}, {"Primary", 1, 0}},
}

// availableLibraryOptions answers Jellyfin's library options for a content
// type with every provider list empty. Deliberate difference: Jellyfin
// lists its metadata, image and similarity providers and its NFO reader
// and saver; Polyfin's metadata comes from addons, so it has none.
func availableLibraryOptions(w http.ResponseWriter, r *http.Request) {
	types, ok := representativeItemTypes[strings.ToLower(query(r, "libraryContentType"))]
	if !ok {
		types = []string{"Series", "Season", "Episode", "Movie"}
	}
	none := []libraryOptionInfo{}
	result := libraryOptionsResult{
		MetadataSavers: none, MetadataReaders: none, SubtitleFetchers: none,
		LyricFetchers: none, MediaSegmentProviders: none,
		TypeOptions: make([]libraryTypeOptions, 0, len(types)),
	}
	for _, kind := range types {
		images := defaultImageOptions[kind]
		if images == nil {
			images = []imageOption{}
		}
		result.TypeOptions = append(result.TypeOptions, libraryTypeOptions{
			Type: kind, MetadataFetchers: none, ImageFetchers: none, SimilarItemProviders: none,
			SupportedImageTypes: []string{}, DefaultImageOptions: images,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// defaultMetadataOptions is Jellyfin's new MetadataOptions, whose null
// ItemType Jellyfin leaves out.
func defaultMetadataOptions() any {
	return struct {
		DisabledMetadataSavers   []string
		LocalMetadataReaderOrder []string
		DisabledMetadataFetchers []string
		MetadataFetcherOrder     []string
		DisabledImageFetchers    []string
		ImageFetcherOrder        []string
	}{[]string{}, []string{}, []string{}, []string{}, []string{}, []string{}}
}

// listingsProviderInfo is the part of Jellyfin's ListingsProviderInfo a new
// provider carries.
type listingsProviderInfo struct {
	EnabledTuners    []string
	EnableAllTuners  bool
	NewsCategories   []string
	SportsCategories []string
	KidsCategories   []string
	MovieCategories  []string
	ChannelMappings  []struct{}
}

// defaultListingProvider is Jellyfin's new ListingsProviderInfo. Deliberate
// difference: Jellyfin's kids categories also hold a platform's name, left
// out here.
func defaultListingProvider() listingsProviderInfo {
	return listingsProviderInfo{
		EnabledTuners:    []string{},
		EnableAllTuners:  true,
		NewsCategories:   []string{"news", "journalism", "documentary", "current affairs"},
		SportsCategories: []string{"sports", "basketball", "baseball", "football"},
		KidsCategories:   []string{"kids", "family", "children", "childrens"},
		MovieCategories:  []string{"movie"},
		ChannelMappings:  []struct{}{},
	}
}

// startupConfiguration answers the wizard's configuration from Polyfin's
// settings, as /System/Configuration does.
func (h *Handler) startupConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, struct {
		ServerName                string
		UICulture                 string
		MetadataCountryCode       string
		PreferredMetadataLanguage string
	}{settings.ServerName, uiCulture(settings.Language), "US", "en"})
}

// startupUser names the first user: Polyfin's earliest administrator, as
// Jellyfin's first user is the administrator its wizard created.
func (h *Handler) startupUser(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	slices.SortStableFunc(users, func(a, b accounts.User) int {
		if a.IsAdministrator != b.IsAdministrator {
			if a.IsAdministrator {
				return -1
			}
			return 1
		}
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	name := ""
	if len(users) > 0 {
		name = users[0].Name
	}
	writeJSON(w, http.StatusOK, struct{ Name string }{name})
}
