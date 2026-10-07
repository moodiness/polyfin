// Package jellyfin serves the Jellyfin 12.2 HTTP API to Jellyfin apps.
package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/streamyfin"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/throttle"
	"github.com/moodiness/polyfin/internal/thumbnails"
	"github.com/moodiness/polyfin/internal/trackers"
	"github.com/moodiness/polyfin/internal/userdata"
)

const (
	// Version is the Jellyfin server version Polyfin reproduces.
	Version = "12.2.0"
	// productName identifies a Jellyfin-compatible server to apps; it is a
	// protocol value, not a claim to be Jellyfin.
	productName = "Jellyfin Server"
)

// Options are the dependencies of the Jellyfin API.
type Options struct {
	ServerID     string
	Accounts     *accounts.Store
	QuickConnect *quickconnect.Store
	// SignIns throttles failed password sign-ins per client address.
	SignIns *throttle.Failures
	// WebSocketPort is the port apps reach the server on.
	WebSocketPort int
	// Library browses the catalogs of users' addons.
	Library *library.Service
	// Stremio downloads artwork and subtitles referenced by addons.
	Stremio *stremio.Client
	// Playback analyzes, decides on and serves the versions of titles.
	Playback *playback.Service
	// Preferences stores the display preferences of Jellyfin apps and the
	// configuration of each user.
	Preferences *preferences.Store
	// UserData stores what each user did with each item.
	UserData *userdata.Store
	// Segments finds the parts of titles apps offer to skip; nil finds
	// none.
	Segments *mediasegments.Service
	// Playlists stores the playlists users make.
	Playlists *playlists.Store
	// Collections stores the collections users make.
	Collections *collections.Store
	Logger      *slog.Logger
	// Activity records what happens for the activity log; nil records
	// nothing.
	Activity *activity.Store
	// Tasks are the server's periodic jobs, shown as Jellyfin's scheduled
	// tasks; nil shows none.
	Tasks *tasks.Registry
	// Logs keeps the recent log lines administrators read; nil keeps none.
	Logs *logs.Ring
	// CacheDir is where sources and their remuxes are kept, and
	// RecordingsDir where recordings are, empty when none are made: the
	// folders storage information describes.
	CacheDir      string
	RecordingsDir string
	// FontsDir holds the fonts apps load to render subtitles whose own
	// fonts are missing; empty or missing, it offers none.
	FontsDir string
	// Thumbnails makes and keeps the scrubbing thumbnails and chapter
	// images of the versions played; nil makes none.
	Thumbnails *thumbnails.Service
	// Recordings schedules and keeps Live TV recordings; nil, or one
	// without a folder, records nothing.
	Recordings *recordings.Service
	// Trackers sends what users watch to the tracking services they
	// connected; nil sends nothing.
	Trackers *trackers.Service
	// Streamyfin keeps the rows of Streamyfin's home screen; nil keeps
	// Streamyfin's own.
	Streamyfin *streamyfin.Store
}

// Handler serves the Jellyfin API.
type Handler struct {
	Options
	routes   http.Handler
	images   imageCache
	sessions *playback.Sessions
	// subtitleFiles remembers the subtitle files described for each item,
	// for players that fetch them without credentials.
	subtitleFiles *cache.Cache[accounts.ID, []library.ExternalSubtitle]
	subtitleCache *cache.Cache[accounts.ID, subtitleText]
	// runtimes remembers the runtime of the version each report names.
	runtimes *cache.Cache[string, time.Duration]
	// configurations remembers each user's configuration, which item
	// listings read for every title.
	configurations *cache.Cache[accounts.ID, UserConfiguration]
	// sockets are the WebSockets apps keep open.
	sockets *sockets
	// syncPlay holds the groups apps watch together in.
	syncPlay *syncPlay
	// preparations bounds the preparations of playback made ahead of it.
	preparations *preparations
	// listings lists the versions of home rows' titles ahead of a play.
	listings *listings
	// watched are the title pages that follow their versions as addons
	// answer (see versionsChanged).
	watched *watchedPages
	// searches bounds how often users have titles' addons asked again
	// (see searchVersions).
	searches versionSearches
	// now tells the time users' allowed hours are checked against.
	now func() time.Time
	// newKeys remembers the API keys made from Jellyfin apps until a
	// listing shows them.
	newKeys newKeys
	// viewing remembers the item each device last reported showing.
	viewing *cache.Cache[accounts.ID, accounts.ID]
	// reasons remembers why each play session streamed over HLS, Jellyfin's
	// TranscodeReasons, for the admin dashboard.
	reasons *cache.Cache[string, []string]
}

// New returns the Jellyfin API handler.
func New(options Options) *Handler {
	h := &Handler{
		Options:        options,
		sessions:       playback.NewSessions(),
		subtitleFiles:  cache.New[accounts.ID, []library.ExternalSubtitle](5000, 12*time.Hour).Sized(16<<20, cache.JSONSize),
		subtitleCache:  cache.New[accounts.ID, subtitleText](200, time.Hour).Sized(32<<20, subtitleText.size),
		runtimes:       cache.New[string, time.Duration](2000, 12*time.Hour),
		configurations: cache.New[accounts.ID, UserConfiguration](1000, 12*time.Hour),
		sockets:        newSockets(),
		preparations:   newPreparations(options.Logger),
		watched:        newWatchedPages(),
		now:            time.Now,
		viewing:        cache.New[accounts.ID, accounts.ID](5000, 12*time.Hour),
		reasons:        cache.New[string, []string](2000, 12*time.Hour),
	}
	h.syncPlay = newSyncPlay(h.canPlay, h.writeSyncPlay, options.Logger)
	h.listings = newListings(h.listVersions)
	if options.Library != nil {
		options.Library.OnVersionsChanged(h.versionsChanged)
	}
	options.Accounts.OnSignOut(h.signedOut)
	if options.Thumbnails != nil {
		options.Thumbnails.WatchPlaybacks(h.thumbnailPlaybacks)
	}
	rt := &router{unmatched: newUnmatchedRequests(options.Logger)}
	anonymous := func(method, pattern string, handler http.HandlerFunc) { rt.handle(method, pattern, handler) }
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	// Like Jellyfin, these answer a user outside their allowed hours too.
	signedInAnyHour := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticatedAnyHour(handler))
	}

	anonymous(http.MethodGet, "/System/Info/Public", h.publicSystemInfo)
	signedInAnyHour(http.MethodGet, "/System/Info", h.systemInfo)
	signedIn(http.MethodGet, "/System/Configuration/{key}", h.namedConfiguration)
	anonymous(http.MethodGet, "/System/Ping", h.ping)
	anonymous(http.MethodPost, "/System/Ping", h.ping)

	anonymous(http.MethodGet, "/Branding/Configuration", h.brandingConfiguration)
	anonymous(http.MethodGet, "/Branding/Css", h.brandingCSS)
	anonymous(http.MethodGet, "/Branding/Css.css", h.brandingCSS)

	anonymous(http.MethodGet, "/Users/Public", h.publicUsers)
	anonymous(http.MethodPost, "/Users/AuthenticateByName", h.authenticateByName)
	anonymous(http.MethodPost, "/Users/AuthenticateWithQuickConnect", h.authenticateWithQuickConnect)
	signedIn(http.MethodGet, "/Users/Me", h.currentUser)
	signedIn(http.MethodGet, "/Streamyfin/config", h.streamyfinConfigOf)
	signedIn(http.MethodGet, "/Users", h.users)
	signedInAnyHour(http.MethodGet, "/Users/{userId}", h.user)
	signedIn(http.MethodPost, "/Users/Password", h.changePassword)
	signedIn(http.MethodPost, "/Users/{userId}/Password", h.changePassword)
	signedIn(http.MethodPost, "/Users/{userId}/Policy", h.updatePolicy)

	anonymous(http.MethodGet, "/QuickConnect/Enabled", h.quickConnectEnabled)
	anonymous(http.MethodPost, "/QuickConnect/Initiate", h.quickConnectInitiate)
	anonymous(http.MethodGet, "/QuickConnect/Connect", h.quickConnectConnect)
	signedIn(http.MethodPost, "/QuickConnect/Authorize", h.quickConnectAuthorize)

	signedIn(http.MethodPost, "/Sessions/Logout", h.logout)
	signedIn(http.MethodPost, "/Sessions/Capabilities", h.capabilities)
	signedIn(http.MethodPost, "/Sessions/Capabilities/Full", h.fullCapabilities)

	h.browseRoutes(rt)
	h.musicRoutes(rt)
	h.audioRoutes(rt)
	h.personRoutes(rt)
	h.auxiliaryRoutes(rt)
	h.liveTvRoutes(rt)
	h.playbackRoutes(rt)
	h.userDataRoutes(rt)
	h.remoteSubtitleRoutes(rt)
	h.remoteRoutes(rt)
	h.configurationRoutes(rt)
	h.localizationRoutes(rt)
	signedIn(http.MethodGet, "/MediaSegments/{itemId}", h.mediaSegments)
	h.playlistRoutes(rt)
	h.syncPlayRoutes(rt)
	h.collectionRoutes(rt)
	h.administrationRoutes(rt)
	h.userImageRoutes(rt)
	h.appRoutes(rt)
	h.subtitleUploadRoutes(rt)
	h.absentRoutes(rt)
	h.metadataRoutes(rt)
	h.rootRoutes(rt)
	signedIn(http.MethodGet, "/Videos/{itemId}/Trickplay/{width}/{file}", h.trickplayFile)

	h.routes = cors(rt)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isSocket(r) {
		h.socket(w, r)
		return
	}
	h.routes.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// processingError answers like Jellyfin's exception middleware.
func processingError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("Error processing request."))
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	// An app that stops waiting cancels its request: nothing failed, and
	// nobody reads the answer.
	if r.Context().Err() != nil {
		h.Logger.Debug("An app abandoned a Jellyfin API request", "method", r.Method, "path", r.URL.Path)
		return
	}
	h.Logger.Error("Jellyfin API request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	processingError(w, http.StatusInternalServerError)
}
