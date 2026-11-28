package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// initiated opens a Quick Connect request as device through call.
func initiated(t *testing.T, call func(string, string, string, any) (int, []byte), device string) QuickConnectResult {
	t.Helper()
	status, body := call(http.MethodPost, "/QuickConnect/Initiate", app(device, ""), nil)
	var request QuickConnectResult
	if status != http.StatusOK || json.Unmarshal(body, &request) != nil || len(request.Code) != 6 || request.Secret == "" {
		t.Fatalf("initiate of %s: %d %s", device, status, body)
	}
	return request
}

// One address opens requests without signing in, a thousand as as many
// devices and a thousand as one: none is refused, and none takes the place
// of another address's. The TV that waited before them still waits, and
// another device still gets a code.
func TestQuickConnectLeavesEveryClientItsCode(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	busy, home := connected(t, s, "203.0.113.9:50000"), connected(t, s, "192.168.1.20:50000")
	tv := initiated(t, home, "living-room-tv")
	var last QuickConnectResult
	for i := range 1000 {
		last = initiated(t, busy, fmt.Sprintf("made-up-%d", i))
	}
	for range 1000 {
		last = initiated(t, busy, "made-up")
	}
	waits := func(request QuickConnectResult) int {
		status, _ := home(http.MethodGet, "/QuickConnect/Connect?secret="+request.Secret, "", nil)
		return status
	}
	if status := waits(tv); status != http.StatusOK {
		t.Errorf("the TV that waited before them: %d, want 200", status)
	}
	if status := waits(last); status != http.StatusOK {
		t.Errorf("the address's own last request: %d, want 200", status)
	}
	phone := initiated(t, connected(t, s, "198.51.100.7:50000"), "phone")
	if status := waits(phone); status != http.StatusOK {
		t.Errorf("another address's request after them: %d, want 200", status)
	}
}

// A code no app waits with counts as a wrong password does, toward the
// attempts of the user who gives it, wherever they connect from, and of
// their address, whoever connects from it: past the limit the answer is
// 429, for the right code too. The request waits on, and its owner
// approves it from elsewhere.
func TestUnknownQuickConnectCodesCountAsFailedAttemptsOfTheirUser(t *testing.T) {
	t.Parallel()
	const limit = 10
	s := newTestServer(t, limit)
	owner := s.user("owner", nil)
	s.user("member", nil)
	s.user("neighbour", nil)
	member, neighbour, phone := app("laptop", s.signIn("member", "laptop")), app("tablet", s.signIn("neighbour", "tablet")), app("phone", s.signIn("owner", "phone"))
	cafe, elsewhere, home := connected(t, s, "203.0.113.9:50000"), connected(t, s, "198.51.100.7:50000"), connected(t, s, "192.168.1.20:50000")
	tv := initiated(t, home, "tv")

	authorize := func(call func(string, string, string, any) (int, []byte), authorization, code string) (int, []byte) {
		return call(http.MethodPost, "/QuickConnect/Authorize?code="+code, authorization, nil)
	}
	wrong := 0
	for n := 0; wrong < limit; n++ {
		code := fmt.Sprintf("%06d", n)
		if code == tv.Code {
			continue
		}
		if status, body := authorize(cafe, member, code); status != http.StatusNotFound {
			t.Fatalf("unknown code %d: %d %s, want 404", wrong+1, status, body)
		}
		wrong++
	}
	for name, attempt := range map[string]struct {
		call          func(string, string, string, any) (int, []byte)
		authorization string
		code          string
	}{
		"another unknown code":                   {cafe, member, "999999"},
		"the waiting code":                       {cafe, member, tv.Code},
		"the waiting code from another address":  {elsewhere, member, tv.Code},
		"the waiting code by another user there": {cafe, neighbour, tv.Code},
		"an unknown code by another user there":  {cafe, neighbour, "999999"},
	} {
		if status, body := authorize(attempt.call, attempt.authorization, attempt.code); status != http.StatusTooManyRequests {
			t.Errorf("%s past the limit: %d %s, want 429", name, status, body)
		}
	}
	// The request still waits, for no one.
	status, body := home(http.MethodGet, "/QuickConnect/Connect?secret="+tv.Secret, "", nil)
	var polled QuickConnectResult
	if status != http.StatusOK || json.Unmarshal(body, &polled) != nil || polled.Authenticated {
		t.Fatalf("the request after the guesses: %d %s, want it waiting", status, body)
	}
	// Its owner approves it from home, and the TV signs in as them.
	if status, body := authorize(home, phone, tv.Code); status != http.StatusOK {
		t.Fatalf("the owner's approval: %d %s", status, body)
	}
	status, body = home(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app("tv", ""), map[string]string{"Secret": tv.Secret})
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.User.Id != owner.ID.String() {
		t.Errorf("the TV's sign-in: %d %s, want the owner", status, body)
	}
}

// The secret signs in: an app that starts a request under one device ID
// and signs in under another, as Jellyfin for Android TV 0.20.1 does, is
// signed in, as the device ID it signs in with. That device signs in again
// with the secret until the request ends, and no other device does, the
// one that started it included.
func TestAnApprovedQuickConnectRequestSignsInTheDevicePresentingItsSecret(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	tv := initiated(t, s.call, "tv-started")
	secret := map[string]string{"Secret": tv.Secret}
	signIn := func(device string) (int, AuthenticationResult) {
		status, body := s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app(device, ""), secret)
		var result AuthenticationResult
		_ = json.Unmarshal(body, &result)
		return status, result
	}
	if status, _ := signIn("tv"); status != http.StatusNotFound {
		t.Errorf("the secret before approval: %d, want 404", status)
	}
	if err := s.handler.QuickConnect.Authorize(tv.Code, alice.ID); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		status, result := signIn("tv")
		if status != http.StatusOK || result.User.Id != alice.ID.String() || result.SessionInfo.DeviceId != "tv-id" || result.SessionInfo.DeviceName != "tv" {
			t.Errorf("the TV's sign-in %d under its other device ID: %d %+v", attempt+1, status, result.SessionInfo)
		}
	}
	for _, other := range []string{"laptop", "tv-started"} {
		if status, result := signIn(other); status != http.StatusNotFound || result.AccessToken != "" {
			t.Errorf("%s with the secret once the TV signed in: %d, want 404 and no token", other, status)
		}
	}
	devices, err := s.store.Devices(t.Context(), alice.ID)
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "tv-id" {
		t.Errorf("alice's devices: %+v, %v, want the TV alone", devices, err)
	}
}

// Devices that race with one approved secret: one of them signs in.
func TestQuickConnectSignsInOneOfTheDevicesRacingWithASecret(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	tv := initiated(t, s.call, "tv")
	if err := s.handler.QuickConnect.Authorize(tv.Code, alice.ID); err != nil {
		t.Fatal(err)
	}
	const racers = 8
	var signedIn atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			<-start
			status, _ := s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app(fmt.Sprintf("racer-%d", i), ""), map[string]string{"Secret": tv.Secret})
			if status == http.StatusOK {
				signedIn.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	if won := signedIn.Load(); won != 1 {
		t.Errorf("devices signed in with one secret at once: %d, want 1", won)
	}
	if devices, err := s.store.Devices(t.Context(), alice.ID); err != nil || len(devices) != 1 {
		t.Errorf("alice's devices: %d, %v, want 1", len(devices), err)
	}
}

// A password change signs the account out everywhere, the devices Quick
// Connect signed in included, and takes back what the account approved:
// the secret of a request approved before signs no device in after, used
// or not. A request approved after it signs in as before.
func TestAPasswordChangeEndsApprovedQuickConnectRequests(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	root := s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	used, unused := initiated(t, s.call, "tv"), initiated(t, s.call, "console")
	for _, request := range []QuickConnectResult{used, unused} {
		if err := s.handler.QuickConnect.Authorize(request.Code, root.ID); err != nil {
			t.Fatal(err)
		}
	}
	signIn := func(device string, request QuickConnectResult) (int, string) {
		status, body := s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app(device, ""), map[string]string{"Secret": request.Secret})
		var result AuthenticationResult
		_ = json.Unmarshal(body, &result)
		return status, result.AccessToken
	}
	keys := func(token string) int {
		status, _ := s.call(http.MethodGet, "/Auth/Keys", app("tv", token), nil)
		return status
	}
	status, token := signIn("tv", used)
	if status != http.StatusOK || keys(token) != http.StatusOK {
		t.Fatalf("the approved TV: %d, its token lists API keys: %d", status, keys(token))
	}

	next := "another horse entirely"
	if _, err := s.store.UpdateUser(t.Context(), root.ID, accounts.UserChanges{Password: &next}, nil); err != nil {
		t.Fatal(err)
	}
	if status := keys(token); status != http.StatusUnauthorized {
		t.Errorf("the TV's token after the password change: %d, want 401", status)
	}
	for device, request := range map[string]QuickConnectResult{"tv": used, "console": unused} {
		if status, token := signIn(device, request); status != http.StatusNotFound || token != "" {
			t.Errorf("the secret of %s after the password change: %d, want 404 and no token", device, status)
		}
		status, body := s.call(http.MethodGet, "/QuickConnect/Connect?secret="+request.Secret, "", nil)
		var polled QuickConnectResult
		if status != http.StatusOK || json.Unmarshal(body, &polled) != nil || polled.Authenticated {
			t.Errorf("the request of %s after the password change: %d %s, want it not approved", device, status, body)
		}
	}
	if devices, err := s.store.Devices(t.Context(), root.ID); err != nil || len(devices) != 0 {
		t.Errorf("devices after the password change: %d, %v", len(devices), err)
	}

	// The TV asks again, is approved again, and signs in.
	again := initiated(t, s.call, "tv")
	if err := s.handler.QuickConnect.Authorize(again.Code, root.ID); err != nil {
		t.Fatal(err)
	}
	if status, token := signIn("tv", again); status != http.StatusOK || keys(token) != http.StatusOK {
		t.Errorf("a request approved after the password change: %d", status)
	}
}
