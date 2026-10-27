package silo

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// Contract is the commit of Silo-Server/silo-server whose API contract,
// contracts/api/v2/openapi.json, Polyfin follows; ContractDigest is that
// file's SHA-256, which system/info tells the apps.
const (
	Contract       = "6de6d5e5e285a9b423e95a1122cb39e5bb9ca93b"
	ContractDigest = "bbb759b43ac256f37a3cddd68c1c90aedeb9f788328b7702af4616273ca3bdd6"
)

// tokenLifetime is the lifetime the apps are told their access token has.
// Polyfin's sessions do not expire: the apps refresh it this often, which
// tells them when it was signed out.
const tokenLifetime = time.Hour

func (s *Server) accountRoutes() {
	s.mux.HandleFunc("GET /api/v2/system/info", s.systemInfo)
	s.mux.HandleFunc("GET /api/v2/system/identity", s.systemIdentity)
	s.mux.HandleFunc("GET /api/v2/system/setup", s.systemSetup)
	s.mux.HandleFunc("POST /api/v2/auth/login", s.login)
	s.mux.HandleFunc("POST /api/v2/auth/refresh", s.refresh)
	s.mux.HandleFunc("POST /api/v2/auth/logout", s.signedInAnyHour(s.logout))
	s.mux.HandleFunc("GET /api/v2/account/me", s.signedIn(s.me))
	s.mux.HandleFunc("GET /api/v2/profiles", s.signedIn(s.profiles))
}

type systemInfo struct {
	ServerVersion  string      `json:"server_version"`
	APIMajor       int         `json:"api_major"`
	ContractDigest string      `json:"contract_digest"`
	Links          systemLinks `json:"links"`
}

// systemLinks name the contract, the capability documents and the
// identity, all three of which the contract requires. Polyfin serves the
// identity and the capabilities of playback and search, not the contract
// nor a list of the capabilities.
type systemLinks struct {
	OpenAPI      string `json:"openapi"`
	Capabilities string `json:"capabilities"`
	Identity     string `json:"identity"`
}

// systemInfo is the apps' first call: an api_major of 2 tells them the
// server speaks the API they know.
func (s *Server) systemInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, systemInfo{ServerVersion: "polyfin-" + s.options.Version, APIMajor: 2,
		ContractDigest: ContractDigest, Links: systemLinks{OpenAPI: "/api/v2/openapi.json", Capabilities: "/api/v2/capabilities",
			Identity: "/api/v2/system/identity"}})
}

func (s *Server) systemIdentity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		ServerID string `json:"server_id"`
	}{s.options.ServerID})
}

// systemSetup tells whether the server still needs its first
// administrator, whom the admin app creates.
func (s *Server) systemSetup(w http.ResponseWriter, r *http.Request) {
	required, err := s.options.Accounts.SetupRequired(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		NeedsSetup      bool `json:"needs_setup"`
		WizardCompleted bool `json:"wizard_completed"`
	}{required, !required})
}

type account struct {
	ID                     string   `json:"id"`
	Username               string   `json:"username"`
	Email                  string   `json:"email"`
	Role                   string   `json:"role"`
	DownloadAllowed        bool     `json:"download_allowed"`
	PasswordChangeRequired bool     `json:"password_change_required"`
	Permissions            []string `json:"permissions"`
}

// accountOf describes a user. Downloads are not served to Silo's apps.
func accountOf(user accounts.User) account {
	role := "user"
	if user.IsAdministrator {
		role = "admin"
	}
	return account{ID: user.ID.String(), Username: user.Name, Role: role, Permissions: []string{}}
}

type tokenPair struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int64   `json:"expires_in"`
	User         account `json:"user"`
}

// deviceOf is the device a sign-in comes from, as the app names itself;
// a device that gives no identifier is a new one at each sign-in.
func deviceOf(r *http.Request) accounts.DeviceInfo {
	header := func(name string, size int) string {
		value := strings.TrimSpace(r.Header.Get(name))
		if len(value) > size || !utf8.ValidString(value) {
			return ""
		}
		return value
	}
	id := header("X-Silo-Device-Id", 128)
	if id == "" {
		random := make([]byte, 16)
		_, _ = rand.Read(random)
		id = hex.EncodeToString(random)
	}
	return accounts.DeviceInfo{
		DeviceID:      id,
		DeviceName:    cmp.Or(header("X-Silo-Device-Name", 128), appName),
		Client:        cmp.Or(header("X-Silo-Client", 64), appName),
		ClientVersion: cmp.Or(header("X-Silo-Client-Version", 64), "unknown"),
		RemoteAddress: forwarded.Client(r),
	}
}

// login signs a device in with a password, as AuthenticateByName signs a
// Jellyfin app in: the same throttle, the same refusals. An account with
// two-step sign-in is refused as a wrong password is, as Jellyfin apps
// are: Silo's apps have no field for the code. It signs a TV in with a
// code instead (see deviceLoginRoutes).
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string  `json:"username"`
		Password string  `json:"password"`
		Provider *string `json:"provider"`
	}
	if !decode(w, r, &body) {
		return
	}
	switch {
	case body.Username == "" || len(body.Username) > 254:
		validationProblem(w, "body.username", "required", "A username is required.")
		return
	case body.Password == "" || len(body.Password) > 1024:
		validationProblem(w, "body.password", "required", "A password is required.")
		return
	case body.Provider != nil && *body.Provider != "":
		validationProblem(w, "body.provider", "unsupported", "This server signs in with a password only.")
		return
	}
	device := deviceOf(r)
	var result struct{ AccessToken string }
	a, err := s.callAs(r, device, "", http.MethodPost, "/Users/AuthenticateByName",
		map[string]string{"Username": body.Username, "Pw": body.Password}, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	switch {
	case a.status == http.StatusUnauthorized:
		problem(w, http.StatusUnauthorized, "invalid_token", "Invalid username or password.")
		return
	case a.status == http.StatusForbidden:
		problem(w, http.StatusForbidden, "permission_denied", "This account may not sign in now.")
		return
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
		return
	}
	_, user, err := s.options.Accounts.DeviceByToken(r.Context(), result.AccessToken, forwarded.Client(r))
	if errors.Is(err, accounts.ErrNotFound) {
		// The account was disabled, or ended, since it was authenticated.
		problem(w, http.StatusUnauthorized, "invalid_token", "Invalid username or password.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenPair{AccessToken: result.AccessToken, RefreshToken: result.AccessToken,
		ExpiresIn: int64(tokenLifetime / time.Second), User: accountOf(user)})
}

// refresh tells the app its session still stands. Polyfin's sessions do
// not expire: the refresh token is the session's token, given back.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.RefreshToken == "" {
		validationProblem(w, "body.refresh_token", "required", "A refresh token is required.")
		return
	}
	_, _, err := s.options.Accounts.DeviceByToken(r.Context(), body.RefreshToken, forwarded.Client(r))
	if errors.Is(err, accounts.ErrNotFound) {
		problem(w, http.StatusUnauthorized, "invalid_grant", "The session ended.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}{body.RefreshToken, int64(tokenLifetime / time.Second), body.RefreshToken})
}

// logout signs the device out, as a Jellyfin app's sign-out does, at any
// hour: the Jellyfin API's sign-out refuses a user outside their hours.
func (s *Server) logout(w http.ResponseWriter, r *http.Request, v viewer) {
	if err := s.options.Accounts.SignOutDevice(r.Context(), v.token); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, v viewer) {
	writeJSON(w, http.StatusOK, accountOf(v.user))
}

type profile struct {
	ID                         string   `json:"id"`
	Name                       string   `json:"name"`
	Avatar                     string   `json:"avatar"`
	AvatarSource               string   `json:"avatar_source"`
	HasPin                     bool     `json:"has_pin"`
	IsChild                    bool     `json:"is_child"`
	IsPrimary                  bool     `json:"is_primary"`
	MaxContentRating           string   `json:"max_content_rating"`
	MaxAdvisoryAge             *int     `json:"max_advisory_age"`
	RequireAdvisoryAge         bool     `json:"require_advisory_age"`
	QualityPreference          string   `json:"quality_preference"`
	MaxPlaybackQuality         string   `json:"max_playback_quality"`
	Language                   string   `json:"language"`
	PreferredMetadataLanguage  string   `json:"preferred_metadata_language"`
	SubtitleLanguage           string   `json:"subtitle_language"`
	SubtitleMode               string   `json:"subtitle_mode"`
	ShowForcedSubtitles        bool     `json:"show_forced_subtitles"`
	AutoSkipIntro              bool     `json:"auto_skip_intro"`
	AutoSkipCredits            bool     `json:"auto_skip_credits"`
	AutoSkipRecap              bool     `json:"auto_skip_recap"`
	AutoPlayNextPreview        bool     `json:"auto_play_next_preview"`
	LibraryRestrictionsEnabled bool     `json:"library_restrictions_enabled"`
	AllowedLibraryIDs          []string `json:"allowed_library_ids"`
	CreatedAt                  string   `json:"created_at"`
	UpdatedAt                  string   `json:"updated_at"`
}

// profiles lists the one profile each user has, named after them. The
// user's limits are the server's to apply, wherever they browse: the
// profile itself restricts nothing more.
func (s *Server) profiles(w http.ResponseWriter, _ *http.Request, v viewer) {
	created := instant(v.user.CreatedAt)
	writeJSON(w, http.StatusOK, struct {
		Items                       []profile `json:"items"`
		AvatarUploadEnabled         bool      `json:"avatar_upload_enabled"`
		MaxAdvisoryAgeSupported     bool      `json:"max_advisory_age_supported"`
		RequireAdvisoryAgeSupported bool      `json:"require_advisory_age_supported"`
	}{Items: []profile{{ID: v.user.ID.String(), Name: v.user.Name, AvatarSource: "none", IsPrimary: true,
		QualityPreference: "auto", AllowedLibraryIDs: []string{}, CreatedAt: created, UpdatedAt: created}}})
}

// instant writes a time as the API does: RFC 3339 in UTC, to the
// millisecond.
func instant(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
