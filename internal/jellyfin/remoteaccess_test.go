package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
)

// connected calls s's handler as the client at address does with no proxy
// between them: the request's connection comes from address.
func connected(t *testing.T, s testServer, address string) func(method, target, authorization string, body any) (int, []byte) {
	return func(method, target, authorization string, body any) (int, []byte) {
		t.Helper()
		var reader io.Reader = http.NoBody
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		request := httptest.NewRequestWithContext(t.Context(), method, target, reader)
		request.RemoteAddr = address
		request.Header.Set("Range", "bytes=0-3")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		recorder := httptest.NewRecorder()
		s.handler.ServeHTTP(recorder, request)
		return recorder.Code, recorder.Body.Bytes()
	}
}

// A user without remote access signs in, is served and plays from the
// local network, the server's machine included, directly or behind a known
// proxy, and is refused from the internet, at sign-in, on every request
// and on every stream address. jellyfin-web's checkbox sets it through
// the user's policy, and each refused sign-in is in the activity log.
func TestRemoteAccess(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	p.testServer.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	root := p.signIn("root", "dashboard")
	knowProxies(t, p.testServer, "127.0.0.0/8")
	proxied := behindProxy(t, p.testServer)

	// jellyfin-web's user editor shows "Allow remote connections to this
	// server" only when the network configuration allows remote access.
	var network NetworkConfiguration
	if status := p.get(t, "/System/Configuration/network", root, &network); status != http.StatusOK || !network.EnableRemoteAccess ||
		!slices.Equal(network.KnownProxies, []string{"127.0.0.0/8"}) {
		t.Fatalf("network configuration: %d %+v", status, network)
	}
	// It reads the policy, unticks the checkbox and posts the policy back
	// whole.
	policy := func() map[string]any {
		t.Helper()
		var dto map[string]any
		if status := p.get(t, "/Users/"+p.user.ID.String(), root, &dto); status != http.StatusOK {
			t.Fatalf("user: %d", status)
		}
		return dto["Policy"].(map[string]any)
	}
	posted := policy()
	if posted["EnableRemoteAccess"] != true {
		t.Fatalf("a new user's remote access: %v", posted["EnableRemoteAccess"])
	}
	posted["EnableRemoteAccess"] = false
	if status, body := p.call(http.MethodPost, "/Users/"+p.user.ID.String()+"/Policy", app("dashboard", root), posted); status != http.StatusNoContent {
		t.Fatalf("posting the policy: %d %s", status, body)
	}
	if got := policy()["EnableRemoteAccess"]; got != false {
		t.Errorf("remote access read back: %v", got)
	}
	if stored, _ := p.store.User(t.Context(), p.user.ID); stored.RemoteAccess {
		t.Error("remote access still stored")
	}

	// The addresses the member plays from, granted from the local network:
	// a play session's direct stream, the same with the token, and a remux.
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{"MediaSourceId": p.versions[1].ID.String()})
	var direct playbackInfoResponse
	if err := json.Unmarshal(data, &direct); status != http.StatusOK || err != nil {
		t.Fatalf("playback info: %d %s", status, data)
	}
	stream := "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + direct.MediaSources[0].Id + "&tag=" + direct.MediaSources[0].ETag
	status, data = p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "EnableDirectPlay": false, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var remuxed playbackInfoResponse
	if err := json.Unmarshal(data, &remuxed); status != http.StatusOK || err != nil || remuxed.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("remux playback info: %d %s", status, data)
	}
	addresses := map[string]string{
		"a play session's stream": stream + "&playSessionId=" + direct.PlaySessionId,
		"a stream with the token": stream + "&ApiKey=" + p.token,
		"a download":              "/Items/" + p.movie + "/Download?ApiKey=" + p.token,
		"a remux's playlist":      remuxed.MediaSources[0].TranscodingUrl,
	}

	for _, client := range []struct {
		name  string
		call  func(method, target, authorization string, body any) (int, []byte)
		local bool
	}{
		{"the server's machine", connected(t, p.testServer, "127.0.0.1:50000"), true},
		{"the local network", connected(t, p.testServer, "192.168.1.20:50000"), true},
		{"the internet", connected(t, p.testServer, "203.0.113.9:50000"), false},
		{"the local network behind a known proxy", func(method, target, authorization string, body any) (int, []byte) {
			return proxied("192.168.1.30", method, target, authorization, body)
		}, true},
		{"the internet behind a known proxy", func(method, target, authorization string, body any) (int, []byte) {
			return proxied("203.0.113.7", method, target, authorization, body)
		}, false},
	} {
		want := map[bool]int{true: http.StatusOK, false: http.StatusForbidden}[client.local]
		status, body := client.call(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""), map[string]string{"Username": "member", "Pw": "correct horse"})
		if status != want {
			t.Errorf("%s: sign-in %d %s, want %d", client.name, status, body, want)
		}
		// Every endpoint, those served outside the allowed hours included.
		for _, path := range []string{"/Users/Me", "/UserViews", "/System/Info", "/Users/" + p.user.ID.String()} {
			if status, body := client.call(http.MethodGet, path, app("tv", p.token), nil); status != want || (!client.local && len(body) != 0) {
				t.Errorf("%s: %s %d, want %d", client.name, path, status, want)
			}
		}
		for name, target := range addresses {
			status, _ := client.call(http.MethodGet, target, "", nil)
			if served := status == http.StatusOK || status == http.StatusPartialContent; client.local != served ||
				!client.local && status != http.StatusUnauthorized {
				t.Errorf("%s: %s %d", client.name, name, status)
			}
		}
		// The administrator's own access is theirs.
		if status, _ := client.call(http.MethodGet, "/Users/Me", app("dashboard", root), nil); status != http.StatusOK {
			t.Errorf("%s: the administrator %d", client.name, status)
		}
	}

	// Each refused sign-in is a failed one about the member, with the
	// client's address.
	page, err := p.activity.Entries(t.Context(), activity.Query{Types: []string{"AuthenticationFailed"}})
	if err != nil {
		t.Fatal(err)
	}
	var refusedFrom []string
	for _, entry := range page.Entries {
		if entry.UserID == nil || *entry.UserID != p.user.ID || entry.ShortOverview == nil || entry.Severity != activity.Warning {
			t.Errorf("entry: %+v", entry)
			continue
		}
		refusedFrom = append(refusedFrom, *entry.ShortOverview)
	}
	if len(refusedFrom) != 2 || !slices.ContainsFunc(refusedFrom, func(s string) bool { return strings.Contains(s, "203.0.113.9") }) ||
		!slices.ContainsFunc(refusedFrom, func(s string) bool { return strings.Contains(s, "203.0.113.7") }) {
		t.Errorf("refused sign-ins: %q", refusedFrom)
	}

	// Behind a proxy that is not known, the internet looks local: the
	// limit cannot hold, which the admin app warns of.
	knowProxies(t, p.testServer)
	if status, _ := proxied("203.0.113.7", http.MethodGet, "/Users/Me", app("tv", p.token), nil); status != http.StatusOK {
		t.Errorf("the internet behind an unknown proxy: %d", status)
	}
}

// Like Jellyfin's, the remote check covers administrators, signing in and
// requests alike, never API keys; a policy that leaves the setting out
// grants it; and the sign-in screen's users from the internet leave out
// those who may not sign in from there.
func TestRemoteAccessOfAdministratorsKeysAndPublicUsers(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	root := s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator, c.IsHidden = new(true), new(false) })
	member := s.user("member", func(c *accounts.UserChanges) { c.IsHidden = new(false) })
	rootToken := s.signIn("root", "dashboard")
	_, key, err := s.store.CreateAPIKey(t.Context(), "Monitor")
	if err != nil {
		t.Fatal(err)
	}
	internet, home := connected(t, s, "203.0.113.9:50000"), connected(t, s, "10.0.0.5:50000")
	publicNames := func(call func(string, string, string, any) (int, []byte)) []string {
		t.Helper()
		_, body := call(http.MethodGet, "/Users/Public", "", nil)
		var users []UserDto
		_ = json.Unmarshal(body, &users)
		var names []string
		for _, user := range users {
			names = append(names, user.Name)
		}
		return names
	}
	if _, err := s.store.UpdateUser(t.Context(), member.ID, accounts.UserChanges{RemoteAccess: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if got := publicNames(internet); !slices.Equal(got, []string{"root"}) {
		t.Errorf("public users from the internet: %v", got)
	}
	if got := publicNames(home); !slices.Equal(got, []string{"member", "root"}) && !slices.Equal(got, []string{"root", "member"}) {
		t.Errorf("public users from the local network: %v", got)
	}

	// From the internet, the administrator cannot take their own remote
	// access away, which would lock them out at once: 409 with a message.
	// From the local network they can.
	posted := map[string]any{"IsAdministrator": true, "EnableRemoteAccess": false}
	status, body := internet(http.MethodPost, "/Users/"+root.ID.String()+"/Policy", app("dashboard", rootToken), posted)
	var message string
	if status != http.StatusConflict || json.Unmarshal(body, &message) != nil || message == "" {
		t.Errorf("their own from the internet: %d %s", status, body)
	}
	if stored, _ := s.store.User(t.Context(), root.ID); !stored.RemoteAccess {
		t.Fatal("the refused policy was stored")
	}
	if status, body := home(http.MethodPost, "/Users/"+root.ID.String()+"/Policy", app("dashboard", rootToken), posted); status != http.StatusNoContent {
		t.Fatalf("posting the policy: %d %s", status, body)
	}
	if status, _ := internet(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""), map[string]string{"Username": "root", "Pw": "correct horse"}); status != http.StatusForbidden {
		t.Errorf("administrator signing in from the internet: %d", status)
	}
	if status, _ := internet(http.MethodGet, "/System/Info", app("dashboard", rootToken), nil); status != http.StatusForbidden {
		t.Errorf("administrator's app from the internet: %d", status)
	}
	if status, _ := home(http.MethodGet, "/System/Info", app("dashboard", rootToken), nil); status != http.StatusOK {
		t.Errorf("administrator's app from the local network: %d", status)
	}
	if status, _ := internet(http.MethodGet, "/System/Info", `MediaBrowser Token="`+key+`"`, nil); status != http.StatusOK {
		t.Errorf("an API key from the internet: %d", status)
	}
	// A policy that leaves it out grants it, as Jellyfin's does.
	if status, body := home(http.MethodPost, "/Users/"+root.ID.String()+"/Policy", app("dashboard", rootToken), map[string]any{"IsAdministrator": true}); status != http.StatusNoContent {
		t.Fatalf("posting a policy without it: %d %s", status, body)
	}
	if status, _ := internet(http.MethodGet, "/System/Info", app("dashboard", rootToken), nil); status != http.StatusOK {
		t.Errorf("administrator's app from the internet once granted again: %d", status)
	}
}

// Quick Connect checks both requests: the approval comes from where the
// approver may connect, and the app signing in from where the user may.
func TestQuickConnectFromOutsideTheLocalNetwork(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", func(c *accounts.UserChanges) { c.RemoteAccess = new(false) })
	phone := s.signIn("member", "phone")
	internet, home := connected(t, s, "203.0.113.9:50000"), connected(t, s, "192.168.1.20:50000")
	initiate := func(call func(string, string, string, any) (int, []byte), device string) QuickConnectResult {
		t.Helper()
		status, body := call(http.MethodPost, "/QuickConnect/Initiate", app(device, ""), nil)
		var initiated QuickConnectResult
		if status != http.StatusOK || json.Unmarshal(body, &initiated) != nil {
			t.Fatalf("initiate: %d %s", status, body)
		}
		return initiated
	}

	// The member's phone, outside, cannot approve.
	away := initiate(internet, "tv")
	if status, _ := internet(http.MethodPost, "/QuickConnect/Authorize?code="+away.Code, app("phone", phone), nil); status != http.StatusForbidden {
		t.Errorf("approving from the internet: %d", status)
	}
	// Approved from home, a TV outside is still refused, and its request
	// stays approved for a retry from home.
	if status, _ := home(http.MethodPost, "/QuickConnect/Authorize?code="+away.Code, app("phone", phone), nil); status != http.StatusOK {
		t.Fatalf("approving from the local network: %d", status)
	}
	secret := map[string]string{"Secret": away.Secret}
	if status, _ := internet(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app("tv", ""), secret); status != http.StatusForbidden {
		t.Errorf("signing in from the internet: %d", status)
	}
	status, body := home(http.MethodPost, "/Users/AuthenticateWithQuickConnect", app("tv", ""), secret)
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.User.Id != member.ID.String() {
		t.Errorf("signing in from the local network: %d %s", status, body)
	}
}
