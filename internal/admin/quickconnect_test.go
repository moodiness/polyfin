package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/moodiness/polyfin/internal/quickconnect"
)

// fromHome is where the tests' devices ask from: the local network, at
// another address than the tests' browsers.
var fromHome = quickconnect.Origin{Address: "192.168.1.20", Local: true}

// approval is the body of an approval as the page sends it: the code, and
// the code as the person entered it from their device's screen.
func approval(code string) map[string]any {
	return map[string]any{"code": code, "confirmation": code}
}

// pendingRequest fails the test unless the request still waits, approved
// for no one.
func pendingRequest(t *testing.T, api testAPI, request quickconnect.Request) {
	t.Helper()
	found, err := api.quickConnect.BySecret(request.Secret)
	if err != nil || found.State != quickconnect.Pending || found.User != nil {
		t.Fatalf("the request: %+v %v, want it pending", found, err)
	}
}

// A code alone does not approve, as a link that carries it would: the
// approval names the code a second time, as the person entered it, and is
// refused without it or with another, the request left waiting.
func TestQuickConnectApprovalNeedsTheCodeConfirmed(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	request, err := api.quickConnect.Initiate(fromHome, "tv-id", "Living room TV", "Jellyfin Android TV", "0.19.0")
	if err != nil {
		t.Fatal(err)
	}
	other := fmt.Sprintf("%06d", (mustNumber(t, request.Code)+1)%1_000_000)
	for name, body := range map[string]map[string]any{
		"the code alone":           {"code": request.Code},
		"an empty confirmation":    {"code": request.Code, "confirmation": ""},
		"another code confirmed":   {"code": request.Code, "confirmation": other},
		"a confirmation alone":     {"confirmation": request.Code},
		"an identity in its place": {"code": request.Code, "identity": map[string]string{"password": "correct horse"}},
	} {
		if status, answer, _ := member.call(http.MethodPost, "/quick-connect", body); status != http.StatusBadRequest || answer["error"] != "unconfirmed_code" {
			t.Errorf("%s: %d %v, want 400 unconfirmed_code", name, status, answer)
		}
	}
	pendingRequest(t, api, request)
	// Entered as the device shows it, spaced or not, it approves.
	spaced := request.Code[:3] + " " + request.Code[3:]
	if status, answer, _ := member.call(http.MethodPost, "/quick-connect", map[string]any{"code": request.Code, "confirmation": spaced}); status != http.StatusNoContent {
		t.Fatalf("the code confirmed: %d %v", status, answer)
	}
	approved, _ := api.quickConnect.BySecret(request.Secret)
	if approved.State != quickconnect.Approved || approved.User == nil || approved.User.String() != member.sessionUser()["id"] {
		t.Errorf("the request once confirmed: %+v", approved)
	}
}

// mustNumber reads a code's digits.
func mustNumber(t *testing.T, code string) int {
	t.Helper()
	number, err := strconv.Atoi(code)
	if err != nil {
		t.Fatal(err)
	}
	return number
}

// An account with two-step sign-in confirms who they are when it approves:
// the device signs in without the second step. Without a code of the
// authenticator app or the password, or with a wrong one, the approval is
// refused and the request left waiting.
func TestQuickConnectApprovalAsksATwoStepAccountWhoTheyAre(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("root", true)
	secret, recovery := turnOnTwoStep(t, administrator)
	request, err := api.quickConnect.Initiate(quickconnect.Origin{Address: "203.0.113.9"}, "other-id", "Living room TV", "Jellyfin Android TV", "0.19.0")
	if err != nil {
		t.Fatal(err)
	}
	// The page learns from the lookup that the approval asks it.
	if status, shown, _ := administrator.call(http.MethodGet, "/quick-connect/"+request.Code, nil); status != http.StatusOK || shown["identity"] != true {
		t.Fatalf("the lookup of a two-step account: %d %v", status, shown)
	}
	// The code, confirmed, is not enough.
	if status, body, _ := administrator.call(http.MethodPost, "/quick-connect", approval(request.Code)); status != http.StatusForbidden ||
		body["error"] != "identity_required" {
		t.Fatalf("an approval without a code of the app or the password: %d %v, want 403 identity_required", status, body)
	}
	pendingRequest(t, api, request)
	for name, wrong := range map[string]struct {
		identity map[string]string
		status   int
		code     string
	}{
		"a wrong code":     {map[string]string{"code": "000000"}, http.StatusBadRequest, "invalid_code"},
		"a wrong password": {map[string]string{"password": "wrong horse"}, http.StatusForbidden, "wrong_password"},
	} {
		body := approval(request.Code)
		body["identity"] = wrong.identity
		if status, answer, _ := administrator.call(http.MethodPost, "/quick-connect", body); status != wrong.status || answer["error"] != wrong.code {
			t.Errorf("%s: %d %v, want %d %s", name, status, answer, wrong.status, wrong.code)
		}
	}
	pendingRequest(t, api, request)

	// With a code of the app, it approves.
	body := approval(request.Code)
	body["identity"] = map[string]string{"code": code(t, secret, 1)}
	if status, answer, _ := administrator.call(http.MethodPost, "/quick-connect", body); status != http.StatusNoContent {
		t.Fatalf("an approval with a code of the app: %d %v", status, answer)
	}
	approved, _ := api.quickConnect.BySecret(request.Secret)
	if approved.State != quickconnect.Approved || approved.User == nil || approved.User.String() != administrator.sessionUser()["id"] {
		t.Errorf("the request once approved: %+v", approved)
	}
	// So does it with a recovery code, or the password.
	for name, identity := range map[string]map[string]string{"a recovery code": {"code": recovery[0]}, "the password": {"password": "correct horse"}} {
		next, err := api.quickConnect.Initiate(fromHome, "tv-"+name, "TV", "Swiftfin", "1.3")
		if err != nil {
			t.Fatal(err)
		}
		body := approval(next.Code)
		body["identity"] = identity
		if status, answer, _ := administrator.call(http.MethodPost, "/quick-connect", body); status != http.StatusNoContent {
			t.Errorf("an approval with %s: %d %v", name, status, answer)
		}
	}

	// An account without two-step sign-in is asked nothing more.
	member := api.signedIn("member", false)
	plain, err := api.quickConnect.Initiate(fromHome, "phone-id", "Phone", "Swiftfin", "1.3")
	if err != nil {
		t.Fatal(err)
	}
	if status, shown, _ := member.call(http.MethodGet, "/quick-connect/"+plain.Code, nil); status != http.StatusOK || shown["identity"] != false {
		t.Errorf("the lookup of an account without two-step sign-in: %d %v", status, shown)
	}
	if status, answer, _ := member.call(http.MethodPost, "/quick-connect", approval(plain.Code)); status != http.StatusNoContent {
		t.Errorf("its approval: %d %v", status, answer)
	}
}

// The lookup answers where the server saw the device ask from, beside the
// names the device gave, and how that place stands to the approver's: their
// own address, the local network with them, or another network.
func TestQuickConnectLookupTellsWhereARequestCameFrom(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	// The tests' browsers connect from 127.0.0.1, on the local network.
	for name, tc := range map[string]struct {
		from    quickconnect.Origin
		network string
	}{
		"the approver's own address": {quickconnect.Origin{Address: "127.0.0.1", Local: true}, "own"},
		"the local network":          {fromHome, "local"},
		"another network":            {quickconnect.Origin{Address: "203.0.113.9"}, "other"},
		"an IPv6 address elsewhere":  {quickconnect.Origin{Address: "2001:db8::1"}, "other"},
	} {
		request, err := api.quickConnect.Initiate(tc.from, "tv-"+name, "Living room TV", "Jellyfin Android TV", "0.19.0")
		if err != nil {
			t.Fatal(err)
		}
		status, shown, _ := member.call(http.MethodGet, "/quick-connect/"+request.Code, nil)
		if status != http.StatusOK || shown["address"] != tc.from.Address || shown["network"] != tc.network {
			t.Errorf("%s: %d %v, want address %s and network %s", name, status, shown, tc.from.Address, tc.network)
		}
		if shown["deviceName"] != "Living room TV" || shown["appName"] != "Jellyfin Android TV" || shown["appVersion"] != "0.19.0" || shown["requestedAt"] == nil {
			t.Errorf("%s: the device as it named itself: %v", name, shown)
		}
	}
}

// A code no device waits with counts as a wrong password does, toward the
// attempts of the user who gives it and of their address, on the lookup
// and on the approval alike: past the limit the answer is 429, for the
// right code too, and the request waits on as it was. A known code counts
// for nothing.
func TestUnknownQuickConnectCodesCountAsFailedAttempts(t *testing.T) {
	t.Parallel()
	const limit = 10
	api := newTestAPI(t, limit)
	member := api.signedIn("member", false)
	request, err := api.quickConnect.Initiate(fromHome, "tv-id", "Living room TV", "Jellyfin Android TV", "0.19.0")
	if err != nil {
		t.Fatal(err)
	}
	number := mustNumber(t, request.Code)
	unknown := func(n int) string { return fmt.Sprintf("%06d", (number+1+n)%1_000_000) }
	// Looking the waiting device up, as the page does at each visit, uses
	// no attempt.
	for n := range 3 * limit {
		if status, body, _ := member.call(http.MethodGet, "/quick-connect/"+request.Code, nil); status != http.StatusOK {
			t.Fatalf("lookup %d of the waiting device: %d %v", n+1, status, body)
		}
	}
	// Unknown codes, looked up or approved, are refused as unknown up to
	// the limit.
	for n := range limit {
		var status int
		var body map[string]any
		if n%2 == 0 {
			status, body, _ = member.call(http.MethodGet, "/quick-connect/"+unknown(n), nil)
		} else {
			status, body, _ = member.call(http.MethodPost, "/quick-connect", approval(unknown(n)))
		}
		if status != http.StatusNotFound || body["error"] != "unknown_code" {
			t.Fatalf("unknown code %d: %d %v, want 404 unknown_code", n+1, status, body)
		}
	}
	// Then every code waits, the right one included.
	for name, call := range map[string]func() (int, map[string]any, *http.Response){
		"the lookup of another unknown code": func() (int, map[string]any, *http.Response) {
			return member.call(http.MethodGet, "/quick-connect/"+unknown(limit), nil)
		},
		"the approval of another unknown code": func() (int, map[string]any, *http.Response) {
			return member.call(http.MethodPost, "/quick-connect", approval(unknown(limit)))
		},
		"the lookup of the waiting device": func() (int, map[string]any, *http.Response) {
			return member.call(http.MethodGet, "/quick-connect/"+request.Code, nil)
		},
		"the approval of the waiting device": func() (int, map[string]any, *http.Response) {
			return member.call(http.MethodPost, "/quick-connect", approval(request.Code))
		},
	} {
		status, body, response := call()
		if status != http.StatusTooManyRequests || body["error"] != "too_many_attempts" || response.Header.Get("Retry-After") == "" {
			t.Errorf("%s past the limit: %d %v, Retry-After %q, want 429 too_many_attempts", name, status, body, response.Header.Get("Retry-After"))
		}
	}
	pendingRequest(t, api, request)
}
