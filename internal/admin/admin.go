// Package admin serves the JSON API of the admin interface.
package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/throttle"
	"github.com/moodiness/polyfin/internal/trackers"
)

const (
	cookieName = "polyfin_session"
	cookiePath = "/admin"
	maxBody    = 64 << 10
	// maxSettingsBody fits the settings with their custom CSS and script at
	// their largest, written as JSON escapes them.
	maxSettingsBody = 4 << 20
	readyWait       = 2 * time.Second
)

// Pinger reports whether the database answers.
type Pinger interface {
	Ping(context.Context) error
}

// Options are the dependencies of the admin API.
type Options struct {
	Version      string
	ServerID     string
	Database     Pinger
	Accounts     *accounts.Store
	Addons       *addons.Store
	QuickConnect *quickconnect.Store
	SignIns      *throttle.Failures
	// SetupCode authorizes creating the first administrator. It is printed
	// in the server log at startup while no administrator exists.
	SetupCode string
	Logger    *slog.Logger
	// Now tells the time users' allowed hours are checked against; nil
	// means the system clock.
	Now func() time.Time
	// Guides fetches the XMLTV guides of live TV catalogs and maps their
	// channels.
	Guides Guides
	// Activity records what administrators change for the activity log;
	// nil records nothing.
	Activity *activity.Store
	// RecordingsDir is the folder Live TV recordings are written to, empty
	// when recording is off.
	RecordingsDir string
	// Acceleration is the GPU POLYFIN_HWACCEL asks for, which the settings
	// fall back on, and VAAPIDevice the render node POLYFIN_VAAPI_DEVICE
	// names, empty for any.
	Acceleration string
	VAAPIDevice  string
	// IPTV stores the IPTV sources, which the addon routes list among the
	// addons.
	IPTV *iptv.Service
	// Segments checks the keys of the segment databases before the settings
	// save them, and tells the order they are preferred in; nil saves no
	// key and asks no database.
	Segments *mediasegments.Service
	// WebClient tells whether Polyfin serves jellyfin-web at /web/.
	WebClient bool
	// Sessions are the playbacks under way, which the dashboard shows and
	// stops; nil shows none.
	Sessions Sessions
	// Tasks are the server's periodic jobs; nil shows none.
	Tasks *tasks.Registry
	// Logs keeps the recent log lines, redacted; nil keeps none.
	Logs *logs.Ring
	// Recordings schedules Live TV recordings; nil records nothing.
	Recordings *recordings.Service
	// Library names the channels of recordings.
	Library ItemReader
	// Health are what the health page reads.
	Health HealthSources
	// Variables are the POLYFIN_ environment variables in effect,
	// without secrets.
	Variables []config.Variable
	// Trackers connects users' accounts on tracking services; nil offers
	// none.
	Trackers *trackers.Service
}

type handler struct {
	Options
	now func() time.Time
	// checks spaces the checks of addons asked by hand.
	checks addonChecks
}

// New returns the handler of every /admin/api/ route.
func New(options Options) http.Handler {
	h := &handler{Options: options, now: time.Now}
	if options.Now != nil {
		h.now = options.Now
	}
	if h.Health.Started.IsZero() {
		h.Health.Started = time.Now()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/status", h.status)
	mux.HandleFunc("POST /admin/api/setup", h.setup)
	mux.HandleFunc("POST /admin/api/session", h.signIn)
	mux.HandleFunc("POST /admin/api/session/jellyfin", h.signInWithJellyfin)

	mux.Handle("GET /admin/api/session", h.signedInAnyHour(h.session))
	mux.Handle("DELETE /admin/api/session", h.signedInAnyHour(h.signOut))
	mux.Handle("PUT /admin/api/account/password", h.signedIn(h.changePassword))
	mux.Handle("GET /admin/api/account/devices", h.signedIn(h.ownDevices))
	mux.Handle("DELETE /admin/api/account/devices/{id}", h.signedIn(h.revokeOwnDevice))
	mux.Handle("GET /admin/api/account/tracking", h.signedIn(h.ownTracking))
	mux.Handle("POST /admin/api/account/tracking/{service}", h.signedIn(h.connectTracking))
	mux.Handle("DELETE /admin/api/account/tracking/{service}", h.signedIn(h.disconnectTracking))
	mux.Handle("PATCH /admin/api/account/tracking/{service}", h.signedIn(h.setTrackingImport))
	mux.Handle("POST /admin/api/account/tracking/{service}/import", h.signedIn(h.importTracking))
	mux.Handle("GET /admin/api/quick-connect/{code}", h.signedIn(h.quickConnectRequest))
	mux.Handle("POST /admin/api/quick-connect", h.signedIn(h.quickConnectApprove))

	mux.Handle("GET /admin/api/users", h.administrator(h.users))
	mux.Handle("POST /admin/api/users", h.administrator(h.createUser))
	mux.Handle("PATCH /admin/api/users/{id}", h.administrator(h.updateUser))
	mux.Handle("DELETE /admin/api/users/{id}", h.administrator(h.deleteUser))
	mux.Handle("GET /admin/api/users/{id}/devices", h.administrator(h.userDevices))
	mux.Handle("DELETE /admin/api/users/{id}/devices/{deviceId}", h.administrator(h.revokeUserDevice))
	mux.Handle("GET /admin/api/parental-ratings", h.administrator(h.parentalRatings))
	mux.Handle("GET /admin/api/settings", h.administrator(h.settings))
	mux.Handle("PUT /admin/api/settings", h.administrator(h.updateSettings))
	mux.Handle("POST /admin/api/users/{id}/unblock", h.administrator(h.unblockUser))
	mux.Handle("GET /admin/api/user-content-choices", h.administrator(h.userContentChoices))
	mux.Handle("GET /admin/api/api-keys", h.administrator(h.apiKeys))
	mux.Handle("POST /admin/api/api-keys", h.administrator(h.createAPIKey))
	mux.Handle("DELETE /admin/api/api-keys/{id}", h.administrator(h.revokeAPIKey))
	mux.Handle("GET /admin/api/activity", h.administrator(h.recentActivity))
	mux.Handle("GET /admin/api/sessions", h.administrator(h.liveSessions))
	mux.Handle("POST /admin/api/sessions/{id}/stop", h.administrator(h.stopSession))
	mux.Handle("POST /admin/api/sessions/{id}/message", h.administrator(h.messageSession))
	mux.Handle("GET /admin/api/tasks", h.administrator(h.scheduledTasks))
	mux.Handle("POST /admin/api/tasks/{id}/run", h.administrator(h.runTask))
	mux.Handle("POST /admin/api/tasks/{id}/stop", h.administrator(h.stopTask))
	mux.Handle("GET /admin/api/timers", h.administrator(h.timers))
	mux.Handle("GET /admin/api/health", h.administrator(h.health))
	mux.Handle("POST /admin/api/health/addons/{id}/check", h.administrator(h.checkAddon))
	mux.Handle("GET /admin/api/sources", h.administrator(h.sources))
	mux.Handle("GET /admin/api/logs", h.administrator(h.logLines))
	mux.Handle("GET /admin/api/logs/download", h.administrator(h.downloadLog))
	mux.Handle("GET /admin/api/variables", h.administrator(h.variables))

	mux.Handle("GET /admin/api/scopes/{scope}/addons", h.signedIn(h.listAddons))
	mux.Handle("POST /admin/api/scopes/{scope}/addons", h.signedIn(h.installAddon))
	mux.Handle("PUT /admin/api/scopes/{scope}/addons/order", h.signedIn(h.reorderAddons))
	mux.Handle("PATCH /admin/api/scopes/{scope}/addons/{id}", h.signedIn(h.updateAddon))
	mux.Handle("POST /admin/api/scopes/{scope}/addons/{id}/refresh", h.signedIn(h.refreshAddon))
	mux.Handle("DELETE /admin/api/scopes/{scope}/addons/{id}", h.signedIn(h.removeAddon))
	mux.Handle("PUT /admin/api/scopes/{scope}/addons/{id}/settings", h.signedIn(h.saveAddonSettings))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv", h.signedIn(h.addSource))
	mux.Handle("PATCH /admin/api/scopes/{scope}/iptv/{id}", h.signedIn(h.updateSource))
	mux.Handle("GET /admin/api/scopes/{scope}/libraries", h.signedIn(h.listLibraries))
	mux.Handle("PUT /admin/api/scopes/{scope}/libraries", h.signedIn(h.saveLibraries))
	mux.Handle("GET /admin/api/account/addon-preferences", h.signedIn(h.addonPreferences))
	mux.Handle("PUT /admin/api/account/addon-preferences", h.signedIn(h.saveAddonPreferences))
	mux.Handle("PUT /admin/api/scopes/{scope}/guides", h.signedIn(h.saveGuide))
	mux.Handle("POST /admin/api/scopes/{scope}/guides/refresh", h.signedIn(h.refreshGuide))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/preview", h.signedIn(h.previewAccount))
	mux.Handle("GET /admin/api/scopes/{scope}/iptv/{id}/preview", h.signedIn(h.previewSource))
	mux.Handle("GET /admin/api/scopes/{scope}/iptv/{id}/categories", h.signedIn(h.listCategories))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/{id}/categories", h.signedIn(h.createCategory))
	mux.Handle("PUT /admin/api/scopes/{scope}/iptv/{id}/categories/order", h.signedIn(h.orderCategories))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/{id}/categories/bulk", h.signedIn(h.bulkCategories))
	mux.Handle("PATCH /admin/api/scopes/{scope}/iptv/{id}/categories/{cid}", h.signedIn(h.updateCategory))
	mux.Handle("DELETE /admin/api/scopes/{scope}/iptv/{id}/categories/{cid}", h.signedIn(h.deleteCategory))
	mux.Handle("GET /admin/api/scopes/{scope}/iptv/{id}/channels", h.signedIn(h.listChannels))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/{id}/channels/bulk", h.signedIn(h.bulkChannels))
	mux.Handle("GET /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}", h.signedIn(h.getChannel))
	mux.Handle("PATCH /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}", h.signedIn(h.updateChannel))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}/move", h.signedIn(h.moveChannel))
	mux.Handle("PUT /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}/streams", h.signedIn(h.setStreams))
	mux.Handle("POST /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}/streams", h.signedIn(h.addStream))
	mux.Handle("DELETE /admin/api/scopes/{scope}/iptv/{id}/channels/{chid}/streams/{sid}", h.signedIn(h.deleteStream))
	mux.Handle("GET /admin/api/scopes/{scope}/catalog-guides", h.signedIn(h.catalogGuides))
	mux.Handle("PUT /admin/api/scopes/{scope}/catalog-guides", h.signedIn(h.saveCatalogGuides))
	mux.Handle("POST /admin/api/scopes/{scope}/catalog-guides/refresh", h.signedIn(h.refreshCatalogGuides))
	mux.Handle("POST /admin/api/scopes/{scope}/catalog-guides/automap", h.signedIn(h.automapCatalog))
	mux.Handle("GET /admin/api/scopes/{scope}/catalog-guides/channels", h.signedIn(h.guideChannels))
	mux.Handle("GET /admin/api/scopes/{scope}/catalog-guides/mappings", h.signedIn(h.catalogMappings))
	mux.Handle("PUT /admin/api/scopes/{scope}/catalog-guides/mappings", h.signedIn(h.setMapping))
	mux.Handle("DELETE /admin/api/scopes/{scope}/catalog-guides/mappings", h.signedIn(h.clearMapping))

	mux.HandleFunc("/admin/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found")
	})
	return sameOrigin(mux)
}

// NewSetupCode returns a random code formatted XXXX-XXXX, without letters
// and digits that are easy to confuse.
func NewSetupCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 0, 9)
	for i := range 8 {
		if i == 4 {
			code = append(code, '-')
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		code = append(code, alphabet[n.Int64()])
	}
	return string(code)
}

func normalizeSetupCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

// sameOrigin refuses state-changing requests sent by another site. The
// session cookie is SameSite=Strict as well; this also covers browsers
// that would send it.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
					writeError(w, http.StatusForbidden, "cross_origin")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	return decodeUpTo(w, r, into, maxBody)
}

func decodeUpTo(w http.ResponseWriter, r *http.Request, into any, limit int64) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func (h *handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	// The admin app leaving a page cancels the requests it polls with:
	// nothing failed, and nobody reads the answer.
	if r.Context().Err() != nil {
		h.Logger.Debug("The admin app abandoned a request", "method", r.Method, "path", r.URL.Path)
		return
	}
	h.Logger.Error("Admin API request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal")
}

// accountError answers the client-facing account errors and reports
// whether err was one of them.
func accountError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{accounts.ErrInvalidName, http.StatusBadRequest, "invalid_name"},
		{accounts.ErrInvalidPassword, http.StatusBadRequest, "invalid_password"},
		{accounts.ErrInvalidServerName, http.StatusBadRequest, "invalid_server_name"},
		{accounts.ErrInvalidLanguage, http.StatusBadRequest, "invalid_language"},
		{accounts.ErrInvalidCatalogLimit, http.StatusBadRequest, "invalid_catalog_limit"},
		{accounts.ErrInvalidChannelLimit, http.StatusBadRequest, "invalid_channel_limit"},
		{accounts.ErrInvalidPlayedPercent, http.StatusBadRequest, "invalid_played_percent"},
		{accounts.ErrInvalidResumePercent, http.StatusBadRequest, "invalid_resume_percent"},
		{accounts.ErrResumeNotBelowPlayed, http.StatusBadRequest, "resume_not_below_played"},
		{accounts.ErrInvalidVersionListMinutes, http.StatusBadRequest, "invalid_version_list_minutes"},
		{accounts.ErrInvalidCatalogRefreshMinutes, http.StatusBadRequest, "invalid_catalog_refresh_minutes"},
		{accounts.ErrInvalidPublicMetaDBKey, http.StatusBadRequest, "invalid_publicmetadb_key"},
		{accounts.ErrInvalidTheIntroDBKey, http.StatusBadRequest, "invalid_theintrodb_key"},
		{accounts.ErrInvalidSegmentOrder, http.StatusBadRequest, "invalid_segment_order"},
		{accounts.ErrInvalidLoginAttempts, http.StatusBadRequest, "invalid_login_attempts"},
		{accounts.ErrInvalidInactiveDeviceDays, http.StatusBadRequest, "invalid_inactive_device_days"},
		{accounts.ErrInvalidAnalysisTimeout, http.StatusBadRequest, "invalid_analysis_timeout"},
		{accounts.ErrInvalidVersionAttempts, http.StatusBadRequest, "invalid_version_attempts"},
		{accounts.ErrInvalidMaxConversions, http.StatusBadRequest, "invalid_max_conversions"},
		{accounts.ErrInvalidMaxConversionHeight, http.StatusBadRequest, "invalid_max_conversion_height"},
		{accounts.ErrInvalidEncoderPreset, http.StatusBadRequest, "invalid_encoder_preset"},
		{accounts.ErrInvalidVideoQuality, http.StatusBadRequest, "invalid_video_quality"},
		{accounts.ErrInvalidHardwareAcceleration, http.StatusBadRequest, "invalid_hardware_acceleration"},
		{accounts.ErrInvalidHardwareDecodingCodecs, http.StatusBadRequest, "invalid_hardware_decoding_codecs"},
		{accounts.ErrInvalidToneMappingAlgorithm, http.StatusBadRequest, "invalid_tone_mapping_algorithm"},
		{accounts.ErrInvalidToneMappingPeak, http.StatusBadRequest, "invalid_tone_mapping_peak"},
		{accounts.ErrInvalidToneMappingDesat, http.StatusBadRequest, "invalid_tone_mapping_desat"},
		{accounts.ErrInvalidDeinterlaceMethod, http.StatusBadRequest, "invalid_deinterlace_method"},
		{accounts.ErrInvalidDownmixAlgorithm, http.StatusBadRequest, "invalid_downmix_algorithm"},
		{accounts.ErrInvalidDownmixBoost, http.StatusBadRequest, "invalid_downmix_boost"},
		{accounts.ErrInvalidMaxAudioChannels, http.StatusBadRequest, "invalid_max_audio_channels"},
		{accounts.ErrInvalidAudioBitrate, http.StatusBadRequest, "invalid_audio_bitrate_per_channel"},
		{accounts.ErrInvalidEncodingThreads, http.StatusBadRequest, "invalid_encoding_threads"},
		{accounts.ErrInvalidAheadSegments, http.StatusBadRequest, "invalid_ahead_segments"},
		{accounts.ErrInvalidTrickplayInterval, http.StatusBadRequest, "invalid_trickplay_interval"},
		{accounts.ErrInvalidTrickplayWidth, http.StatusBadRequest, "invalid_trickplay_width"},
		{accounts.ErrInvalidThumbnailStorage, http.StatusBadRequest, "invalid_thumbnail_storage_gb"},
		{accounts.ErrInvalidRecordingPadding, http.StatusBadRequest, "invalid_recording_padding"},
		{accounts.ErrInvalidRecordingRetentionDays, http.StatusBadRequest, "invalid_recording_retention_days"},
		{accounts.ErrInvalidLiveTvRefreshHours, http.StatusBadRequest, "invalid_live_tv_refresh_hours"},
		{accounts.ErrInvalidCustomCss, http.StatusBadRequest, "invalid_custom_css"},
		{accounts.ErrInvalidCustomJs, http.StatusBadRequest, "invalid_custom_js"},
		{accounts.ErrInvalidLoginDisclaimer, http.StatusBadRequest, "invalid_login_disclaimer"},
		{accounts.ErrInvalidTraktApp, http.StatusBadRequest, "invalid_trakt_app"},
		{accounts.ErrInvalidSimklApp, http.StatusBadRequest, "invalid_simkl_app"},
		{accounts.ErrInvalidParentalControl, http.StatusBadRequest, "invalid_parental_control"},
		{accounts.ErrInvalidMaxPlaybacks, http.StatusBadRequest, "invalid_max_playbacks"},
		{accounts.ErrInvalidMaxBitrate, http.StatusBadRequest, "invalid_max_bitrate"},
		{accounts.ErrInvalidSyncPlay, http.StatusBadRequest, "invalid_sync_play"},
		{accounts.ErrInvalidQualityGroup, http.StatusBadRequest, "invalid_quality_group"},
		{accounts.ErrInvalidBlockedGenres, http.StatusBadRequest, "invalid_blocked_genres"},
		{accounts.ErrInvalidAccessSchedule, http.StatusBadRequest, "invalid_access_schedules"},
		{accounts.ErrNameTaken, http.StatusConflict, "name_taken"},
		{accounts.ErrLastAdministrator, http.StatusConflict, "last_administrator"},
		{accounts.ErrSetupComplete, http.StatusConflict, "setup_complete"},
		{accounts.ErrNotFound, http.StatusNotFound, "not_found"},
		{accounts.ErrWrongPassword, http.StatusForbidden, "wrong_password"},
		{accounts.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
		{accounts.ErrDisabled, http.StatusForbidden, "account_disabled"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// throttled answers 429 when the client exhausted its failed attempts.
func (h *handler) throttled(w http.ResponseWriter, key string) bool {
	allowed, wait := h.SignIns.Allowed(key)
	if allowed {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too_many_attempts")
	return true
}

func setupCodeMatches(expected, given string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(normalizeSetupCode(expected)), []byte(normalizeSetupCode(given))) == 1
}
