package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/totp"
)

// The tests of a Silo TV signing in with a code, through Quick Connect
// (see silo.deviceLoginRoutes), against the Jellyfin API's sign-in.

// codeClock is the clock of a test's Quick Connect requests.
type codeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *codeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *codeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// codeServer serves Silo's apps, with Quick Connect requests on clock.
func codeServer(t *testing.T) (siloApps, *codeClock) {
	t.Helper()
	clock := &codeClock{now: time.Now()}
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.QuickConnect = quickconnect.NewAt(clock.Now)
	})
	allowSilo(t, s)
	return servingSilo(t, s), clock
}

// livingRoom is the identity of a TV app, sent with each of its requests.
var livingRoom = map[string]string{"X-Silo-Device-Id": "appletv-1", "X-Silo-Device-Name": "Living room",
	"X-Silo-Device-Platform": "tvos", "X-Silo-Client": "Silo for Apple TV", "X-Silo-Client-Version": "2.0"}

// tvCall sends a request as the TV does, without a session.
func (a siloApps) tvCall(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, a.site+path, reader)
	for name, value := range livingRoom {
		request.Header.Set(name, value)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

// startCode is a TV's request: its code and its secret.
func (a siloApps) startCode(t *testing.T) (code, secret string) {
	t.Helper()
	status, data := a.tvCall(t, http.MethodPost, "/api/v2/auth/device/start", map[string]string{"device_name": "Living room", "device_platform": "tvos"})
	if status != http.StatusCreated {
		t.Fatalf("start: %d %s", status, data)
	}
	code, secret, err := silotest.CheckDeviceStart(data)
	if err != nil {
		t.Fatalf("start: %v: %s", err, data)
	}
	return code, secret
}

// poll polls as the TV does: the request's state, and its session once
// approved.
func (a siloApps) poll(t *testing.T, secret string) (state, token string) {
	t.Helper()
	status, data := a.tvCall(t, http.MethodPost, "/api/v2/auth/device/poll", map[string]string{"device_code": secret})
	if status != http.StatusOK {
		t.Fatalf("poll: %d %s", status, data)
	}
	state, token, err := silotest.CheckDevicePoll(data)
	if err != nil {
		t.Fatalf("poll: %v: %s", err, data)
	}
	return state, token
}

// decide approves or denies a code from a phone signed in to Silo's app,
// answering the status and the error's code.
func (a siloApps) decide(t *testing.T, token, decision, code string) (int, string) {
	t.Helper()
	status, data := a.do(t, http.MethodPost, "/api/v2/auth/device/"+decision, token, "", map[string]string{"code": code})
	if status != http.StatusOK {
		return status, problemCode(data)
	}
	want := map[string]string{"approve": "approved", "deny": "denied"}[decision]
	if err := silotest.CheckDeviceDecision(data, want); err != nil {
		t.Errorf("%s: %v", decision, err)
	}
	return status, ""
}

// lookUp is a request as a phone signed in to Silo's app sees it.
func (a siloApps) lookUp(t *testing.T, token, code string) (int, string) {
	t.Helper()
	status, data := a.do(t, http.MethodGet, "/api/v2/auth/device?code="+code, token, "", nil)
	if status != http.StatusOK {
		return status, problemCode(data)
	}
	state, err := silotest.CheckDeviceLookup(data)
	if err != nil {
		t.Errorf("lookup: %v: %s", err, data)
	}
	return status, state
}

// A TV shows a code that a phone signed in to Silo's app approves, for
// its user only, once: the TV then gets the session a password gives, as
// a device of that user, and nothing more.
func TestSiloTVSignsInWithACode(t *testing.T) {
	t.Parallel()
	a, _ := codeServer(t)
	alice := a.user("alice", nil)
	a.user("bob", nil)
	phone, _ := a.signIn(t, "alice")
	other, _ := a.signIn(t, "bob")

	status, data := a.tvCall(t, http.MethodGet, "/api/v2/auth/device/capability", nil)
	if offered, cancel, err := silotest.CheckDeviceCapability(data); status != http.StatusOK || err != nil || !offered || !cancel {
		t.Fatalf("capability: %d %v %s", status, err, data)
	}
	code, secret := a.startCode(t)
	if state, _ := a.poll(t, secret); state != "pending" {
		t.Errorf("before approval: %s", state)
	}
	if status, state := a.lookUp(t, phone, code[:4]+"-"+code[4:]); status != http.StatusOK || state != "pending" {
		t.Errorf("lookup: %d %s", status, state)
	}
	if status, _ := a.do(t, http.MethodGet, "/api/v2/auth/device?code="+code, "", "", nil); status != http.StatusUnauthorized {
		t.Errorf("lookup without a session: %d", status)
	}
	if status, _ := a.do(t, http.MethodPost, "/api/v2/auth/device/approve", "", "", map[string]string{"code": code}); status != http.StatusUnauthorized {
		t.Errorf("approval without a session: %d", status)
	}
	if status, problem := a.decide(t, phone, "approve", code); status != http.StatusOK {
		t.Fatalf("approval: %d %s", status, problem)
	}
	// Another user cannot take it, nor turn it down.
	for _, decision := range []string{"approve", "deny"} {
		if status, problem := a.decide(t, other, decision, code); status != silotest.DeviceRefusals["decided"] || problem != "already_decided" {
			t.Errorf("%s by another user: %d %s", decision, status, problem)
		}
	}

	state, token := a.poll(t, secret)
	if state != "approved" || token == "" {
		t.Fatalf("after approval: %s", state)
	}
	device, user, err := a.store.DeviceByToken(t.Context(), token, "127.0.0.1")
	if err != nil || user.ID != alice.ID || device.DeviceID != "appletv-1" || device.DeviceName != "Living room" ||
		device.Client != "Silo for Apple TV" || device.ClientVersion != "2.0" {
		t.Errorf("the TV's session: %+v of %s, %v", device.DeviceInfo, user.Name, err)
	}
	var me struct {
		ID string `json:"id"`
	}
	a.get(t, "/api/v2/account/me", token, "", &me)
	if me.ID != alice.ID.String() {
		t.Errorf("signed in as %s", me.ID)
	}
	devices, err := a.store.Devices(t.Context(), alice.ID)
	if err != nil || len(devices) != 2 {
		t.Errorf("alice's devices: %d, %v", len(devices), err)
	}

	// The tokens are handed once.
	if state, token := a.poll(t, secret); state != "consumed" || token != "" {
		t.Errorf("polled again: %s", state)
	}
	if status, state := a.lookUp(t, phone, code); status != http.StatusOK || state != "consumed" {
		t.Errorf("lookup once used: %d %s", status, state)
	}
	if status, _ := a.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app("appletv-1", ""), map[string]string{"Secret": secret}); status != http.StatusNotFound {
		t.Errorf("Quick Connect's sign-in with the used secret: %d", status)
	}
	if status, problem := a.lookUp(t, phone, "12345678"); status != silotest.DeviceRefusals["unknown"] || problem != "not_found" {
		t.Errorf("an unknown code: %d %s", status, problem)
	}
}

// A code turned down, withdrawn by its TV or expired gives nothing.
func TestSiloTVCodesDeniedWithdrawnOrExpired(t *testing.T) {
	t.Parallel()
	a, clock := codeServer(t)
	a.user("alice", nil)
	phone, _ := a.signIn(t, "alice")

	code, secret := a.startCode(t)
	if status, problem := a.decide(t, phone, "deny", code); status != http.StatusOK {
		t.Fatalf("denial: %d %s", status, problem)
	}
	if state, token := a.poll(t, secret); state != "denied" || token != "" {
		t.Errorf("denied: %s", state)
	}
	if status, problem := a.decide(t, phone, "approve", code); status != silotest.DeviceRefusals["decided"] || problem != "already_decided" {
		t.Errorf("approving a denied code: %d %s", status, problem)
	}

	code, secret = a.startCode(t)
	status, data := a.tvCall(t, http.MethodPost, "/api/v2/auth/device/cancel", map[string]string{"device_code": secret})
	if status != http.StatusOK || !bytes.Contains(data, []byte(`"status":"canceled"`)) {
		t.Errorf("cancel: %d %s", status, data)
	}
	if status, problem := a.decide(t, phone, "approve", code); status != silotest.DeviceRefusals["decided"] || problem != "already_decided" {
		t.Errorf("approving a withdrawn code: %d %s", status, problem)
	}
	if state, _ := a.poll(t, secret); state != "canceled" {
		t.Errorf("withdrawn: %s", state)
	}

	// Approved, but not collected before its end: Quick Connect's lifetime.
	code, secret = a.startCode(t)
	if status, problem := a.decide(t, phone, "approve", code); status != http.StatusOK {
		t.Fatalf("approval: %d %s", status, problem)
	}
	clock.add(quickconnect.Lifetime)
	if status, data := a.tvCall(t, http.MethodPost, "/api/v2/auth/device/poll", map[string]string{"device_code": secret}); status != silotest.DeviceRefusals["gone"] {
		t.Errorf("polling an expired code: %d %s", status, data)
	}
	if status, problem := a.decide(t, phone, "approve", code); status != silotest.DeviceRefusals["unknown"] || problem != "not_found" {
		t.Errorf("approving an expired code: %d %s", status, problem)
	}
}

// An account with two-step sign-in, which Silo's apps cannot sign in with
// a password, signs a TV in with a code approved on the admin app's Quick
// Connect page, which asked for its code. A disabled account gets no
// session from a code approved before.
func TestSiloTVCodesForTwoStepAndDisabledAccounts(t *testing.T) {
	t.Parallel()
	a, _ := codeServer(t)
	alice := a.user("alice", nil)
	carol := a.user("carol", nil)
	setup, err := a.store.StartTwoStep(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	totpCode, _ := totp.Code(setup.Secret, time.Now())
	if _, err := a.store.ConfirmTwoStep(t.Context(), alice.ID, totpCode); err != nil {
		t.Fatal(err)
	}
	if status, data := a.do(t, http.MethodPost, "/api/v2/auth/login", "", "", map[string]string{"username": "alice", "password": "correct horse"}); status != http.StatusUnauthorized {
		t.Fatalf("a password alone: %d %s", status, data)
	}
	code, secret := a.startCode(t)
	// As the admin app approves it, for the user signed in there.
	if err := a.handler.QuickConnect.Authorize(code, alice.ID); err != nil {
		t.Fatal(err)
	}
	state, token := a.poll(t, secret)
	if state != "approved" {
		t.Fatalf("two-step account: %s", state)
	}
	var me struct {
		ID string `json:"id"`
	}
	a.get(t, "/api/v2/account/me", token, "", &me)
	if me.ID != alice.ID.String() {
		t.Errorf("signed in as %s", me.ID)
	}

	code, secret = a.startCode(t)
	if err := a.handler.QuickConnect.Authorize(code, carol.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.UpdateUser(t.Context(), carol.ID, accounts.UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if state, token := a.poll(t, secret); state != "denied" || token != "" {
			t.Errorf("a disabled account: %s", state)
		}
	}
	if devices, _ := a.store.Devices(t.Context(), carol.ID); len(devices) != 0 {
		t.Errorf("devices of a disabled account: %d", len(devices))
	}
}

// The apps offer a TV's code only while Quick Connect is on, and Silo's
// apps allowed. While Quick Connect is off, a code shown before is
// approved by no one.
func TestSiloTVCodesFollowTheSettings(t *testing.T) {
	t.Parallel()
	a, _ := codeServer(t)
	a.user("alice", nil)
	phone, _ := a.signIn(t, "alice")
	start := map[string]string{"device_name": "Living room"}
	for _, tc := range []struct {
		name                  string
		quickConnect, siloApp bool
	}{{"both on", true, true}, {"Quick Connect off", false, true}, {"Silo's apps off", true, false}} {
		a.setting(t, func(settings *accounts.Settings) {
			settings.QuickConnectEnabled, settings.SiloApps = tc.quickConnect, tc.siloApp
		})
		status, data := a.tvCall(t, http.MethodGet, "/api/v2/auth/device/capability", nil)
		offered, cancel, err := silotest.CheckDeviceCapability(data)
		switch {
		case !tc.siloApp:
			// The apps find no Silo server at all.
			if status == http.StatusOK {
				t.Errorf("%s: capability %d %s", tc.name, status, data)
			}
		case status != http.StatusOK || err != nil || offered != tc.quickConnect || cancel != tc.quickConnect:
			t.Errorf("%s: capability %d %v %s", tc.name, status, err, data)
		case !tc.quickConnect && !bytes.Contains(data, []byte(`"state":"disabled"`)):
			t.Errorf("%s: capability %s", tc.name, data)
		}
		want := http.StatusCreated
		if !tc.quickConnect || !tc.siloApp {
			want = silotest.DeviceRefusals["off"]
		}
		if status, data := a.tvCall(t, http.MethodPost, "/api/v2/auth/device/start", start); status != want {
			t.Errorf("%s: start %d %s", tc.name, status, data)
		}
	}

	a.setting(t, func(settings *accounts.Settings) { settings.QuickConnectEnabled, settings.SiloApps = true, true })
	code, secret := a.startCode(t)
	a.setting(t, func(settings *accounts.Settings) { settings.QuickConnectEnabled = false })
	if status, problem := a.decide(t, phone, "approve", code); status != silotest.DeviceRefusals["unknown"] {
		t.Errorf("approving with Quick Connect off: %d %s", status, problem)
	}
	a.setting(t, func(settings *accounts.Settings) { settings.QuickConnectEnabled = true })
	if state, token := a.poll(t, secret); state != "pending" || token != "" {
		t.Errorf("a code approved while Quick Connect was off: %s", state)
	}
}
