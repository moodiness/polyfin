package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// appOf is the Authorization header of an app that names itself and its
// device as given, with its token when it has one.
func appOf(client, device, deviceID, version, token string) string {
	header := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="%s"`, client, device, deviceID, version)
	if token != "" {
		header += fmt.Sprintf(`, Token="%s"`, token)
	}
	return header
}

// A sign-in whose app, device, identifier or version passes its bound is
// refused as one that names no device, before anything is stored, and so
// is a Quick Connect request; at the bounds the device signs in, and its
// row holds what it sent. A signed-in request that names a longer device
// is served for the device its token signed in.
func TestASignInNamesItsDeviceWithinBounds(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	password := map[string]string{"Username": "alice", "Pw": "correct horse"}
	long := func(bytes int) string { return strings.Repeat("N", bytes) }
	for name, header := range map[string]string{
		"a device name of 1 KB":      appOf("Test App", long(1<<10), "tv-id", "1.0.0", ""),
		"a device name past a bound": appOf("Test App", long(accounts.MaxDeviceTextBytes+1), "tv-id", "1.0.0", ""),
		"an app name past a bound":   appOf(long(accounts.MaxDeviceTextBytes+1), "TV", "tv-id", "1.0.0", ""),
		"a version past a bound":     appOf("Test App", "TV", "tv-id", long(accounts.MaxDeviceTextBytes+1), ""),
		"an identifier past a bound": appOf("Test App", "TV", long(accounts.MaxDeviceIDBytes+1), "1.0.0", ""),
	} {
		if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", header, password); status != http.StatusBadRequest {
			t.Errorf("a sign-in with %s: %d", name, status)
		}
		if status, _ := s.call(http.MethodPost, "/QuickConnect/Initiate", header, nil); status != http.StatusBadRequest {
			t.Errorf("a Quick Connect request with %s: %d", name, status)
		}
	}
	var devices, seen int
	if err := s.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM devices WHERE user_id = $1),
		(SELECT count(*) FROM seen_devices WHERE user_id = $1)`, alice.ID).Scan(&devices, &seen); err != nil {
		t.Fatal(err)
	}
	if devices != 0 || seen != 0 {
		t.Errorf("refused sign-ins left %d devices and %d identifiers seen", devices, seen)
	}

	atBounds := appOf(long(accounts.MaxDeviceTextBytes), long(accounts.MaxDeviceTextBytes), long(accounts.MaxDeviceIDBytes),
		long(accounts.MaxDeviceTextBytes), "")
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", atBounds, password)
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.AccessToken == "" {
		t.Fatalf("a sign-in at the bounds: %d", status)
	}
	stored, err := s.store.Devices(t.Context(), alice.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("%d devices, %v", len(stored), err)
	}
	if kept := stored[0]; len(kept.DeviceID) != accounts.MaxDeviceIDBytes || len(kept.DeviceName) != accounts.MaxDeviceTextBytes ||
		len(kept.Client) != accounts.MaxDeviceTextBytes || len(kept.ClientVersion) != accounts.MaxDeviceTextBytes {
		t.Errorf("the device at the bounds keeps %d, %d, %d and %d bytes", len(kept.DeviceID), len(kept.DeviceName), len(kept.Client),
			len(kept.ClientVersion))
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", appOf("Test App", long(4<<10), long(4<<10), "1.0.0", result.AccessToken), nil); status != http.StatusOK {
		t.Errorf("a signed-in request naming a longer device: %d", status)
	}
}

// A device whose row passes the bounds, kept from before them, stays
// signed in: its token is served, administrators list it as it is, and
// sign it out from the list of devices. Signing in again as it names
// itself is refused.
func TestADeviceKeptPastTheBoundsStaysSignedIn(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	alice := s.user("alice", nil)
	admin := app("dashboard", s.signIn("root", "dashboard"))
	token := s.signIn("alice", "old")
	name := strings.Repeat("N", 1<<10)
	commands, err := json.Marshal(accounts.Capabilities{PlayableMediaTypes: []string{}, SupportedCommands: slices.Repeat([]string{name}, 100)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(t.Context(), "UPDATE devices SET device_name = $2, client = $2, capabilities = $3 WHERE user_id = $1",
		alice.ID, name, commands); err != nil {
		t.Fatal(err)
	}

	asItNamesItself := appOf(name, name, "old-id", "1.0.0", token)
	if status, _ := s.call(http.MethodGet, "/Users/Me", asItNamesItself, nil); status != http.StatusOK {
		t.Fatalf("the token of a row past the bounds: %d", status)
	}
	status, body := s.call(http.MethodGet, "/Devices", admin, nil)
	var listed struct{ Items []DeviceInfoDto }
	if status != http.StatusOK || json.Unmarshal(body, &listed) != nil {
		t.Fatalf("the list of devices: %d", status)
	}
	found := slices.IndexFunc(listed.Items, func(d DeviceInfoDto) bool { return d.Id == "old-id" })
	if found < 0 || listed.Items[found].Name != name || len(listed.Items[found].Capabilities.SupportedCommands) != 100 {
		t.Errorf("the row past the bounds is not listed as it is")
	}
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", appOf(name, name, "old-id", "1.0.0", ""),
		map[string]string{"Username": "alice", "Pw": "correct horse"}); status != http.StatusBadRequest {
		t.Errorf("signing in again under the long names: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", asItNamesItself, nil); status != http.StatusOK {
		t.Errorf("after the refused sign-in, the token: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Devices?id=old-id", admin, nil); status != http.StatusNoContent {
		t.Fatalf("signing the device out: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", asItNamesItself, nil); status != http.StatusUnauthorized {
		t.Errorf("the token once signed out: %d", status)
	}
}

// Capabilities longer than Jellyfin's media types and commands make are a
// bad request, in a body as in the query, and neither they nor the device
// profile sent with them are kept. The capabilities apps send are kept as
// before.
func TestCapabilitiesPastTheirBoundsAreRefused(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	token := s.signIn("alice", "tv")
	tv := app("tv", token)
	kept := func() accounts.Device {
		t.Helper()
		devices, err := s.store.Devices(t.Context(), alice.ID)
		if err != nil || len(devices) != 1 {
			t.Fatalf("%d devices, %v", len(devices), err)
		}
		return devices[0]
	}
	// What jellyfin-web sends, in part.
	usual := ClientCapabilities{PlayableMediaTypes: []string{"Audio", "Video"},
		SupportedCommands:    []string{"MoveUp", "MoveDown", "Select", "Back", "VolumeUp", "VolumeDown", "Mute", "Unmute", "ToggleMute", "SetVolume", "DisplayMessage"},
		SupportsMediaControl: true, SupportsPersistentIdentifier: false}
	if status, body := s.call(http.MethodPost, "/Sessions/Capabilities/Full", tv, usual); status != http.StatusNoContent {
		t.Fatalf("usual capabilities: %d %s", status, body)
	}
	same := func() bool {
		capabilities := kept().Capabilities
		return slices.Equal(capabilities.PlayableMediaTypes, usual.PlayableMediaTypes) &&
			slices.Equal(capabilities.SupportedCommands, usual.SupportedCommands) && capabilities.SupportsMediaControl &&
			!capabilities.SupportsPersistentIdentifier
	}
	if !same() {
		t.Fatalf("usual capabilities are kept as %+v", kept().Capabilities)
	}

	tooMany := slices.Repeat([]string{"Mute"}, accounts.MaxCapabilityEntries+1)
	profile := &playback.DeviceProfile{Name: "sent with capabilities past the bounds"}
	for name, body := range map[string]any{
		"a megabyte in one command": map[string]any{"SupportedCommands": []string{strings.Repeat("C", maxBody-128)}},
		"a command past the bound":  map[string]any{"SupportedCommands": []string{strings.Repeat("C", accounts.MaxCapabilityBytes+1)}},
		"too many commands":         map[string]any{"SupportedCommands": tooMany, "DeviceProfile": profile},
		"too many media types":      map[string]any{"PlayableMediaTypes": tooMany, "DeviceProfile": profile},
	} {
		if status, _ := s.call(http.MethodPost, "/Sessions/Capabilities/Full", tv, body); status != http.StatusBadRequest {
			t.Errorf("%s in a body: %d", name, status)
		}
	}
	for name, query := range map[string]string{
		"too many commands":        "supportedCommands=" + strings.Join(tooMany, ","),
		"too many media types":     "playableMediaTypes=" + strings.Join(tooMany, ","),
		"a command past the bound": "supportedCommands=" + strings.Repeat("C", accounts.MaxCapabilityBytes+1),
	} {
		if status, _ := s.call(http.MethodPost, "/Sessions/Capabilities?"+query, tv, nil); status != http.StatusBadRequest {
			t.Errorf("%s in the query: %d", name, status)
		}
	}
	if !same() {
		t.Errorf("after refused capabilities, the device keeps %+v", kept().Capabilities)
	}
	if play, ok := s.handler.devicePlayOf(t.Context(), kept().ID); ok && play.profile != nil {
		t.Errorf("the profile sent with refused capabilities was kept: %+v", play.profile)
	}

	atBounds := slices.Repeat([]string{strings.Repeat("C", accounts.MaxCapabilityBytes)}, accounts.MaxCapabilityEntries)
	if status, _ := s.call(http.MethodPost, "/Sessions/Capabilities?supportedCommands="+strings.Join(atBounds, ","), tv, nil); status != http.StatusNoContent {
		t.Errorf("capabilities at the bounds: %d", status)
	}
	if capabilities := kept().Capabilities; !slices.Equal(capabilities.SupportedCommands, atBounds) {
		t.Errorf("capabilities at the bounds keep %d commands", len(capabilities.SupportedCommands))
	}
}

// The device a user signs in past their bound signs out the one used
// longest ago, whose token is then refused. A device that asks for
// something, as one playing does, is not the one, even signed in first.
// The user never holds more than the bound, and another user's devices
// stay.
func TestTheDevicePastAUsersBoundSignsOutTheOneUsedLongestAgo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	s.user("bob", nil)
	bobs := s.signIn("bob", "device-0")
	tokens := make([]string, accounts.MaxUserDevices)
	for i := range tokens {
		tokens[i] = s.signIn("alice", fmt.Sprintf("device-%d", i))
		// Each was last used a minute after the one before, the first
		// two hours ago.
		if _, err := s.pool.Exec(t.Context(), `UPDATE devices SET last_activity_at = now() - interval '2 hours' + $3::int * interval '1 minute'
			WHERE user_id = $1 AND device_id = $2`, alice.ID, fmt.Sprintf("device-%d-id", i), i); err != nil {
			t.Fatal(err)
		}
	}
	held := func() int {
		t.Helper()
		var n int
		if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM devices WHERE user_id = $1", alice.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	me := func(device int) int {
		status, _ := s.call(http.MethodGet, "/Users/Me", app(fmt.Sprintf("device-%d", device), tokens[device]), nil)
		return status
	}
	if n := held(); n != accounts.MaxUserDevices {
		t.Fatalf("at the bound, the user holds %d devices", n)
	}
	if status := me(0); status != http.StatusOK {
		t.Fatalf("the first device: %d", status)
	}

	for i, signedOut := range []int{1, 2} {
		s.signIn("alice", fmt.Sprintf("one-more-%d", i))
		if n := held(); n != accounts.MaxUserDevices {
			t.Errorf("past the bound, the user holds %d devices", n)
		}
		if status := me(signedOut); status != http.StatusUnauthorized {
			t.Errorf("the device used longest ago, device %d: %d", signedOut, status)
		}
	}
	if first, next := me(0), me(3); first != http.StatusOK || next != http.StatusOK {
		t.Errorf("the device that asked since: %d, the next one used: %d", first, next)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("device-0", bobs), nil); status != http.StatusOK {
		t.Errorf("another user's device: %d", status)
	}
}

// One device's capabilities leave what the tokens of the others resolved
// to, of its user or of another: their next request does not look their
// token up again, by the statements the accounts send, and its own does,
// once, and tells its new capabilities.
func TestCapabilitiesLeaveOtherDevicesSignInsResolved(t *testing.T) {
	t.Parallel()
	counter := &statementCounter{}
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		config := pool.Config()
		config.ConnConfig.Tracer = counter
		counted, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(counted.Close)
		if options.Accounts, err = accounts.Open(t.Context(), counted); err != nil {
			t.Fatal(err)
		}
	})
	// lookups counts the statements sent since the last reset that
	// resolve a token to its device.
	lookups := func() int {
		counter.mu.Lock()
		defer counter.mu.Unlock()
		n := 0
		for sql, sent := range counter.sent {
			if strings.Contains(sql, "WHERE d.token_hash = $1") {
				n += sent
			}
		}
		return n
	}
	s.user("alice", nil)
	s.user("bob", nil)
	tv := app("tv", s.signIn("alice", "tv"))
	others := map[string]string{"the user's other device": app("phone", s.signIn("alice", "phone")), "another user's device": app("tablet", s.signIn("bob", "tablet"))}
	asks := func(device string) {
		t.Helper()
		if status, body := s.call(http.MethodGet, "/Users/Me", device, nil); status != http.StatusOK {
			t.Fatalf("/Users/Me: %d %s", status, body)
		}
	}
	for name, device := range others {
		counter.reset()
		asks(device)
		if n := lookups(); n != 1 {
			t.Fatalf("the first request of %s looked its token up %d times", name, n)
		}
	}
	asks(tv)

	if status, body := s.call(http.MethodPost, "/Sessions/Capabilities/Full", tv,
		ClientCapabilities{PlayableMediaTypes: []string{"Video"}, SupportedCommands: []string{"Mute"}}); status != http.StatusNoContent {
		t.Fatalf("capabilities: %d %s", status, body)
	}
	for name, device := range others {
		counter.reset()
		asks(device)
		if n := lookups(); n != 0 {
			t.Errorf("after a device's capabilities, the next request of %s looked its token up %d times", name, n)
		}
	}
	counter.reset()
	status, body := s.call(http.MethodGet, "/Sessions?deviceId=tv-id", tv, nil)
	var sessions []SessionInfo
	if status != http.StatusOK || json.Unmarshal(body, &sessions) != nil || len(sessions) != 1 {
		t.Fatalf("the device's session: %d %s", status, body)
	}
	if n := lookups(); n != 1 || !slices.Equal(sessions[0].Capabilities.SupportedCommands, []string{"Mute"}) {
		t.Errorf("the device's own next request looked its token up %d times, and tells the commands %v", n,
			sessions[0].Capabilities.SupportedCommands)
	}
}
