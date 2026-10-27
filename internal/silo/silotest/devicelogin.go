package silotest

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Signing a TV in with a code (/api/v2/auth/device/...): the TV apps'
// sign-in screens (Apple QRLoginViewModel, Android DeviceSignInMachine and
// DeviceLoginRepository), and the phone apps' "Sign in a TV" (Apple
// TVApprovalModel and PairingDeviceAPI, Android DevicePairingViewModel),
// at the commits of the apps internal/silo/followed.json records.

// deviceStates are the states the apps know a request in: Apple
// APIv2DevicePoll.validated and DeviceLoginStatus, Android
// DeviceLoginStatus.fromWire.
var deviceStates = []string{"pending", "approved", "denied", "expired", "consumed", "canceled"}

// instantOf reports whether text is a time the apps read: Apple
// HTTPClient.makeJSONDecoder takes ISO 8601 with or without fractional
// seconds, Android parseRfc3339ToEpochMillis RFC 3339.
func instantOf(text any) bool {
	s, ok := text.(string)
	if !ok {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, s)
	return err == nil
}

// CheckDeviceCapability checks the device sign-in document
// (GET /api/v2/auth/device/capability) as the TV apps read it before
// showing a code, and reports whether they offer it, and withdraw the
// codes they no longer show. Apple APIv2DeviceCapability and Android
// DeviceCapabilityV2 require the state, the revision, the handoff and the
// protocol versions; both offer device sign-in only in the state available
// (offersDeviceSignIn, deviceLoginAvailable), and withdraw codes when it
// says cancel too (supportsCancel; Android DeviceCapabilityV2.domain).
func CheckDeviceCapability(data []byte) (offered, cancel bool, err error) {
	doc, err := decode(data)
	if err != nil {
		return false, false, err
	}
	c := &checker{}
	c.has(doc, "capability", map[string]string{"state": "s", "revision": "s", "remote_playback_handoff": "b", "protocol_versions": "a"})
	for _, key := range []string{"cancel", "opened_signal", "jellyfin_quick_connect"} {
		if value, ok := doc[key]; ok {
			if _, isBool := value.(bool); !isBool {
				c.fail("capability.%s is not a boolean: %v", key, value)
			}
		}
	}
	offered = doc["state"] == "available"
	return offered, offered && doc["cancel"] == true, c.err()
}

// normalizedCode is a code as the phone apps compare it: its letters and
// digits, in capitals (Apple DeviceUserCode.normalized, Android
// DeviceCodeFormat.normalize).
func normalizedCode(code string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, code))
}

// CheckDeviceStart checks a TV's request (POST /api/v2/auth/device/start)
// as the TV apps show it, and returns the code a person types and the
// secret the TV polls with.
//
//   - Apple APIv2DeviceStart requires every field, Android
//     DeviceLoginStartResponse all but the purpose and temporary.
//   - The phone's "Sign in a TV" takes eight ASCII letters or digits:
//     Apple DeviceUserCode.isComplete, Android DeviceCodeFormat.LENGTH.
//   - The QR code's address opens the approval: Android
//     DeviceLoginRouteParser takes an http(s) address whose path ends in
//     /activate or /device, with the code in its code parameter.
//   - The TV polls no more often than once a second, until the code's
//     lifetime runs out (Apple QRLoginViewModel.Timing.minimumPoll and
//     showSession, Android DeviceLoginPoller).
func CheckDeviceStart(data []byte) (code, secret string, err error) {
	doc, err := decode(data)
	if err != nil {
		return "", "", err
	}
	c := &checker{}
	c.has(doc, "start", map[string]string{"device_code": "s", "user_code": "s", "match_code": "s", "verification_uri": "s",
		"verification_uri_complete": "s", "expires_at": "s", "expires_in": "i", "interval": "i", "device_name": "s",
		"device_platform": "s", "client_purpose": "s", "temporary": "b"})
	secret, _ = doc["device_code"].(string)
	shown, _ := doc["user_code"].(string)
	code = normalizedCode(shown)
	if len(code) != 8 || strings.ContainsFunc(code, func(r rune) bool { return r > unicode.MaxASCII }) {
		c.fail("user_code %q is not eight ASCII letters or digits", shown)
	}
	if secret == "" || normalizedCode(secret) == code {
		c.fail("device_code %q is not a secret of its own", secret)
	}
	if !instantOf(doc["expires_at"]) {
		c.fail("expires_at %v", doc["expires_at"])
	}
	if n, _ := doc["expires_in"].(float64); n <= 0 {
		c.fail("expires_in %v", doc["expires_in"])
	}
	if n, _ := doc["interval"].(float64); n < 1 {
		c.fail("interval %v", doc["interval"])
	}
	if doc["client_purpose"] != "device_login" || doc["temporary"] != false {
		c.fail("a request for %v, temporary %v", doc["client_purpose"], doc["temporary"])
	}
	complete, _ := doc["verification_uri_complete"].(string)
	address, parseErr := url.Parse(complete)
	path := ""
	if parseErr == nil {
		path = strings.ToLower(strings.TrimRight(address.Path, "/"))
	}
	if parseErr != nil || address.Scheme != "http" && address.Scheme != "https" || address.Host == "" ||
		!strings.HasSuffix(path, "/activate") && !strings.HasSuffix(path, "/device") ||
		normalizedCode(address.Query().Get("code")) != code {
		c.fail("verification_uri_complete %q does not open the approval of %q", complete, code)
	}
	return code, secret, c.err()
}

// CheckDevicePoll checks a TV's poll (POST /api/v2/auth/device/poll) as
// the TV apps read it, and returns its state and, once approved, the
// access token. Apple APIv2DevicePoll and Android DevicePollV2 require the
// state, poll_after, the profile and temporary; Apple validated() refuses
// a state it does not know, tokens on any state but approved, and an
// approval without an access token, a refresh token and a user; the TV's
// sign-in takes no temporary session (QRLoginViewModel.pollCode), and
// Android TokenPairV2 requires the user's id, name, email and role.
func CheckDevicePoll(data []byte) (state, token string, err error) {
	doc, err := decode(data)
	if err != nil {
		return "", "", err
	}
	c := &checker{}
	c.has(doc, "poll", map[string]string{"status": "s", "poll_after": "i", "profile_id": "s", "profile_token": "s", "temporary": "b"})
	state, _ = doc["status"].(string)
	if !slices.Contains(deviceStates, state) {
		c.fail("status %q", state)
	}
	if value, ok := doc["expires_at"]; ok && !instantOf(value) {
		c.fail("expires_at %v", value)
	}
	tokens, present := doc["tokens"].(object)
	switch {
	case state != "approved" && doc["tokens"] != nil:
		c.fail("tokens on a %s request", state)
	case state == "approved" && !present:
		c.fail("an approval without tokens")
	case state == "approved":
		c.has(tokens, "poll.tokens", map[string]string{"access_token": "s", "refresh_token": "s", "expires_in": "i", "user": "o"})
		user, _ := tokens["user"].(object)
		c.has(user, "poll.tokens.user", map[string]string{"id": "s", "username": "s", "email": "s", "role": "s"})
		token, _ = tokens["access_token"].(string)
		if refresh, _ := tokens["refresh_token"].(string); token == "" || refresh == "" || user["id"] == "" {
			c.fail("tokens %v", tokens)
		}
		if doc["temporary"] != false {
			c.fail("a temporary session")
		}
	}
	return state, token, c.err()
}

// CheckDeviceLookup checks a request as the phone apps show it before
// approving it (GET /api/v2/auth/device), and returns its state. Apple
// APIv2DeviceLookup requires every field but the times; TVApprovalModel
// offers no approval of a request whose purpose is not device_login, or
// which is temporary.
func CheckDeviceLookup(data []byte) (string, error) {
	doc, err := decode(data)
	if err != nil {
		return "", err
	}
	c := &checker{}
	c.has(doc, "lookup", map[string]string{"status": "s", "user_code": "s", "match_code": "s", "device_name": "s",
		"device_platform": "s", "ip_address_hint": "s", "client_purpose": "s", "temporary": "b"})
	state, _ := doc["status"].(string)
	if !slices.Contains(deviceStates, state) {
		c.fail("status %q", state)
	}
	for _, key := range []string{"expires_at", "requested_at"} {
		if value, ok := doc[key]; ok && !instantOf(value) {
			c.fail("%s %v", key, value)
		}
	}
	if doc["client_purpose"] != "device_login" || doc["temporary"] != false {
		c.fail("a request for %v, temporary %v", doc["client_purpose"], doc["temporary"])
	}
	return state, c.err()
}

// CheckDeviceDecision checks the answer of an approval or a denial: Apple
// PairingDeviceAPI.approve and deny want the state the decision gives.
func CheckDeviceDecision(data []byte, want string) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	if doc["status"] != want {
		return fmt.Errorf("status %v, want %s", doc["status"], want)
	}
	return nil
}

// DeviceRefusals are the statuses both apps read the same way, by what
// they mean to them:
//
//   - "off": a start refused while the server signs no TV in with a code,
//     which makes the TV ask for a password (Apple
//     QRLoginViewModel.startResult takes 403 and 404, Android
//     DeviceLoginErrorKind 404 and 501);
//   - "gone": a poll of a request the server no longer has, which makes
//     the TV show a new code (Apple QRLoginViewModel.pollResult,
//     DeviceLoginPoller.classify; Android DeviceLoginErrorKind.NotFound);
//   - "unknown": a code the phone looks up or decides that no TV waits
//     with (Apple TVApprovalModel.failure, Android
//     DevicePairingViewModel.pairingError);
//   - "decided": a code already approved, declined or withdrawn (the same).
var DeviceRefusals = map[string]int{"off": 404, "gone": 404, "unknown": 404, "decided": 409}
