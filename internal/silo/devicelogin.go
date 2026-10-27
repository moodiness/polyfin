package silo

import (
	"cmp"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

// Signing a TV in with a code, through Quick Connect.
//
// Silo's TV apps show a code, with a QR code of the page that approves it,
// and poll until it is decided; a phone signed in to Silo's app approves
// it too. Polyfin makes each of their requests a Quick Connect request, in
// the store Jellyfin apps' requests are in: approved on the admin app's
// Quick Connect page (where /activate leads), by Jellyfin's
// /QuickConnect/Authorize, or by a signed-in Silo app, for the signed-in
// user only, once; denied by a signed-in Silo app; withdrawn by the TV;
// ended by Quick Connect's lifetime; and signed in to by Jellyfin's
// /Users/AuthenticateWithQuickConnect, which refuses a disabled or ended
// account. The code has the eight digits Silo's apps take; the TV's
// secret, its device_code, signs in once.
//
// Quick Connect's setting turns it on and off. Polyfin does not approve
// Jellyfin apps' 6-digit codes from Silo's apps (jellyfin_quick_connect),
// tell the TV that its code was opened (opened_signal), nor hand off a
// phone's playback to a TV (remote_playback_handoff).

func (s *Server) deviceLoginRoutes() {
	s.mux.HandleFunc("GET /api/v2/auth/device/capability", s.deviceCapability)
	s.mux.HandleFunc("POST /api/v2/auth/device/start", s.deviceStart)
	s.mux.HandleFunc("POST /api/v2/auth/device/poll", s.devicePoll)
	s.mux.HandleFunc("POST /api/v2/auth/device/cancel", s.deviceCancel)
	s.mux.HandleFunc("GET /api/v2/auth/device", s.signedIn(s.deviceLookup))
	s.mux.HandleFunc("POST /api/v2/auth/device/approve", s.signedIn(s.deviceApprove))
	s.mux.HandleFunc("POST /api/v2/auth/device/deny", s.signedIn(s.deviceDeny))
}

// pollInterval is how often the TV is asked to poll, in seconds.
const pollInterval = 5

// activatePath is the page the TV names, which leads to the admin app's
// Quick Connect page (see package server).
const activatePath = "/activate"

// deviceStates name the states of a request as the apps read them.
var deviceStates = map[quickconnect.State]string{quickconnect.Pending: "pending", quickconnect.Approved: "approved",
	quickconnect.Denied: "denied", quickconnect.Canceled: "canceled", quickconnect.Collected: "consumed"}

// deviceCapability tells the apps whether a TV signs in with a code: while
// Quick Connect is on. A device may then withdraw its request.
func (s *Server) deviceCapability(w http.ResponseWriter, _ *http.Request) {
	state := "disabled"
	if s.options.Accounts.Settings().QuickConnectEnabled {
		state = "available"
	}
	writeJSON(w, http.StatusOK, struct {
		State                 string `json:"state"`
		Revision              string `json:"revision"`
		RemotePlaybackHandoff bool   `json:"remote_playback_handoff"`
		JellyfinQuickConnect  bool   `json:"jellyfin_quick_connect"`
		ProtocolVersions      []int  `json:"protocol_versions"`
		Cancel                bool   `json:"cancel"`
		OpenedSignal          bool   `json:"opened_signal"`
	}{State: state, Revision: revision + "-" + state, ProtocolVersions: []int{}, Cancel: state == "available"})
}

// deviceLoginOff answers a request while Quick Connect is off, as Silo
// answers a server without device sign-in: the apps then ask for a
// password. It reports whether it did.
func (s *Server) deviceLoginOff(w http.ResponseWriter) bool {
	if s.options.Accounts.Settings().QuickConnectEnabled {
		return false
	}
	problem(w, http.StatusNotFound, "not_found", "Signing in with a code is off on this server.")
	return true
}

// deviceRequest is a request of Silo's apps by its secret, or false after
// answering that there is none: it expired, or never was.
func (s *Server) deviceRequest(w http.ResponseWriter, secret string) (quickconnect.Request, bool) {
	request, err := s.options.QuickConnect.BySecret(secret)
	if err != nil || request.Kind != quickconnect.Silo {
		problem(w, http.StatusNotFound, "not_found", "This sign-in request expired.")
		return quickconnect.Request{}, false
	}
	return request, true
}

// userCode writes a code as Silo writes its own, in two groups of four
// digits.
func userCode(code string) string {
	return code[:4] + "-" + code[4:]
}

// verificationBase is the address the TV tells people to open: the public
// address the settings give, else the one the TV reached.
func verificationBase(r *http.Request, settings accounts.Settings) string {
	if address := strings.TrimRight(settings.PublicAddress, "/"); address != "" {
		return address
	}
	scheme := "http"
	if r.TLS != nil || forwarded.ViaProxy(r) && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// deviceStart opens a TV's request, with the name and platform it gives,
// for a sign-in. A remote playback's is not served.
func (s *Server) deviceStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceName     string `json:"device_name"`
		DevicePlatform string `json:"device_platform"`
		ClientPurpose  string `json:"client_purpose"`
		Temporary      bool   `json:"temporary"`
	}
	if !decode(w, r, &body) {
		return
	}
	body.DeviceName, body.DevicePlatform = strings.TrimSpace(body.DeviceName), strings.TrimSpace(body.DevicePlatform)
	switch {
	case len(body.DeviceName) > 128 || !utf8.ValidString(body.DeviceName):
		validationProblem(w, "body.device_name", "invalid", "The device name is too long.")
		return
	case len(body.DevicePlatform) > 64 || !utf8.ValidString(body.DevicePlatform):
		validationProblem(w, "body.device_platform", "invalid", "The device platform is too long.")
		return
	case body.ClientPurpose != "" && body.ClientPurpose != "device_login" || body.Temporary:
		problem(w, http.StatusNotImplemented, "capability_unsupported", "This server does not hand playback off to a TV.")
		return
	}
	if s.deviceLoginOff(w) {
		return
	}
	device := deviceOf(r)
	if r.Header.Get("X-Silo-Device-Name") == "" && body.DeviceName != "" {
		device.DeviceName = body.DeviceName
	}
	platform := cmp.Or(body.DevicePlatform, strings.TrimSpace(r.Header.Get("X-Silo-Device-Platform")))
	if len(platform) > 64 || !utf8.ValidString(platform) {
		platform = ""
	}
	request, err := s.options.QuickConnect.InitiateSilo(device.DeviceID, device.DeviceName, platform, device.Client, device.ClientVersion)
	if errors.Is(err, quickconnect.ErrFull) {
		problem(w, http.StatusServiceUnavailable, "unavailable", "Too many devices are signing in; try again later.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	activate := verificationBase(r, s.options.Accounts.Settings()) + activatePath
	writeJSON(w, http.StatusCreated, struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		MatchCode               string `json:"match_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresAt               string `json:"expires_at"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int    `json:"interval"`
		DeviceName              string `json:"device_name"`
		DevicePlatform          string `json:"device_platform"`
		ClientPurpose           string `json:"client_purpose"`
		Temporary               bool   `json:"temporary"`
	}{request.Secret, userCode(request.Code), "", activate, activate + "?code=" + request.Code, instant(request.ExpiresAt()),
		int64(quickconnect.Lifetime / time.Second), pollInterval, request.DeviceName, request.DevicePlatform, "device_login", false})
}

// devicePoll is a TV's poll: an approved request gives it the session a
// password sign-in gives, once, made by Quick Connect's sign-in; a denied
// or used one gives nothing.
type devicePoll struct {
	Status       string     `json:"status"`
	PollAfter    int        `json:"poll_after"`
	Opened       bool       `json:"opened"`
	ProfileID    string     `json:"profile_id"`
	ProfileToken string     `json:"profile_token"`
	Temporary    bool       `json:"temporary"`
	ExpiresAt    string     `json:"expires_at,omitempty"`
	Tokens       *tokenPair `json:"tokens,omitempty"`
}

func (s *Server) devicePoll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.DeviceCode == "" {
		validationProblem(w, "body.device_code", "required", "A device code is required.")
		return
	}
	if s.deviceLoginOff(w) {
		return
	}
	request, ok := s.deviceRequest(w, body.DeviceCode)
	if !ok {
		return
	}
	switch request.State {
	case quickconnect.Pending:
		writeJSON(w, http.StatusOK, devicePoll{Status: "pending", PollAfter: pollInterval, ExpiresAt: instant(request.ExpiresAt())})
	case quickconnect.Approved:
		s.collect(w, r, request)
	default:
		writeJSON(w, http.StatusOK, devicePoll{Status: deviceStates[request.State], PollAfter: pollInterval})
	}
}

// collect signs the TV of an approved request in, as Jellyfin apps sign in
// with Quick Connect: as the device it named when it started.
func (s *Server) collect(w http.ResponseWriter, r *http.Request, request quickconnect.Request) {
	device := accounts.DeviceInfo{DeviceID: request.DeviceID, DeviceName: request.DeviceName, Client: request.AppName,
		ClientVersion: request.AppVersion}
	var result struct{ AccessToken string }
	a, err := s.callAs(r, device, "", http.MethodPost, "/Users/AuthenticateWithQuickConnect",
		map[string]string{"Secret": request.Secret}, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	denied := devicePoll{Status: "denied", PollAfter: pollInterval}
	switch {
	case a.status == http.StatusForbidden:
		// The account was disabled, or ended, since it approved the code.
		writeJSON(w, http.StatusOK, denied)
		return
	case a.status == http.StatusNotFound || a.status == http.StatusUnauthorized:
		// Another poll collected it, it expired, or Quick Connect was
		// turned off: say where it stands now.
		if request, ok := s.deviceRequest(w, request.Secret); ok {
			writeJSON(w, http.StatusOK, devicePoll{Status: deviceStates[request.State], PollAfter: pollInterval})
		}
		return
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
		return
	}
	_, user, err := s.options.Accounts.DeviceByToken(r.Context(), result.AccessToken, forwarded.Client(r))
	if errors.Is(err, accounts.ErrNotFound) {
		writeJSON(w, http.StatusOK, denied)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, devicePoll{Status: "approved", PollAfter: pollInterval, Tokens: &tokenPair{AccessToken: result.AccessToken,
		RefreshToken: result.AccessToken, ExpiresIn: int64(tokenLifetime / time.Second), User: accountOf(user)}})
}

// deviceCancel withdraws a TV's request, which it no longer shows: its code
// can no longer be approved, nor its approval used.
func (s *Server) deviceCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.DeviceCode == "" {
		validationProblem(w, "body.device_code", "required", "A device code is required.")
		return
	}
	if s.deviceLoginOff(w) {
		return
	}
	if _, ok := s.deviceRequest(w, body.DeviceCode); !ok {
		return
	}
	state, err := s.options.QuickConnect.Cancel(body.DeviceCode)
	if err != nil {
		problem(w, http.StatusNotFound, "not_found", "This sign-in request expired.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{deviceStates[state]})
}

// deviceByCode is the request of Silo's apps behind a code, or false after
// answering that there is none.
func (s *Server) deviceByCode(w http.ResponseWriter, code string) (quickconnect.Request, bool) {
	if s.deviceLoginOff(w) {
		return quickconnect.Request{}, false
	}
	request, err := s.options.QuickConnect.ByCode(code)
	if err != nil || request.Kind != quickconnect.Silo {
		problem(w, http.StatusNotFound, "not_found", "No TV is waiting with this code.")
		return quickconnect.Request{}, false
	}
	return request, true
}

// deviceLookup describes a TV's request to the signed-in user about to
// decide it, and then tells them where it stands.
func (s *Server) deviceLookup(w http.ResponseWriter, r *http.Request, _ viewer) {
	request, ok := s.deviceByCode(w, r.URL.Query().Get("code"))
	if !ok {
		return
	}
	code := ""
	if request.State == quickconnect.Pending {
		code = userCode(request.Code)
	}
	settings := s.options.Accounts.Settings()
	writeJSON(w, http.StatusOK, struct {
		Status         string `json:"status"`
		UserCode       string `json:"user_code"`
		MatchCode      string `json:"match_code"`
		DeviceName     string `json:"device_name"`
		DevicePlatform string `json:"device_platform"`
		IPAddressHint  string `json:"ip_address_hint"`
		RequestedAt    string `json:"requested_at"`
		ExpiresAt      string `json:"expires_at"`
		ClientPurpose  string `json:"client_purpose"`
		Temporary      bool   `json:"temporary"`
		ServerID       string `json:"server_id"`
		ServerName     string `json:"server_name"`
	}{deviceStates[request.State], code, "", request.DeviceName, request.DevicePlatform, "", instant(request.CreatedAt),
		instant(request.ExpiresAt()), "device_login", false, s.options.ServerID, settings.ServerName})
}

// deviceApprove approves a TV's code for the signed-in user, once: the TV
// then signs in as them.
func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request, v viewer) {
	s.decideDevice(w, r, "approved", func(code string) error { return s.options.QuickConnect.Authorize(code, v.user.ID) })
}

// deviceDeny turns a TV's code down: the TV says the sign-in was declined.
func (s *Server) deviceDeny(w http.ResponseWriter, r *http.Request, _ viewer) {
	s.decideDevice(w, r, "denied", s.options.QuickConnect.Deny)
}

// decideDevice decides, with decide, the code the request's body names,
// and answers the state it took. A token of a browser link names nothing:
// Polyfin's links carry the code.
func (s *Server) decideDevice(w http.ResponseWriter, r *http.Request, status string, decide func(code string) error) {
	var body struct {
		Code  string `json:"code"`
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Code == "" {
		validationProblem(w, "body.code", "required", "The code the TV shows is required.")
		return
	}
	request, ok := s.deviceByCode(w, body.Code)
	if !ok {
		return
	}
	err := decide(request.Code)
	switch {
	case errors.Is(err, quickconnect.ErrDecided):
		problem(w, http.StatusConflict, "already_decided", "This code was already approved, declined or withdrawn.")
		return
	case err != nil:
		problem(w, http.StatusNotFound, "not_found", "No TV is waiting with this code.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{status})
}
