package jellyfin

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// remoteApp is an app signed in on its own device, as a remote control
// test sets it up.
type remoteApp struct {
	token, session string
	// socket is the app's socket, nil when it keeps none.
	socket *appSocket
}

// declaring tells how an app declares media control, if it does.
type declaring int

const (
	noMediaControl declaring = iota
	queryMediaControl
	bodyMediaControl
)

func (s testServer) remoteApp(t *testing.T, user, device string, declared declaring, socket bool) remoteApp {
	t.Helper()
	a := remoteApp{token: s.signIn(user, device)}
	signedIn := app(device, a.token)
	switch declared {
	case queryMediaControl:
		if status, body := s.call(http.MethodPost, "/Sessions/Capabilities?playableMediaTypes=Video&supportedCommands=DisplayMessage,GoHome&supportsMediaControl=true", signedIn, nil); status != http.StatusNoContent {
			t.Fatalf("capabilities of %s: %d %s", device, status, body)
		}
	case bodyMediaControl:
		full := map[string]any{"PlayableMediaTypes": []string{"Audio", "Video"}, "SupportedCommands": []string{"SetVolume"}, "SupportsMediaControl": true}
		if status, body := s.call(http.MethodPost, "/Sessions/Capabilities/Full", signedIn, full); status != http.StatusNoContent {
			t.Fatalf("capabilities of %s: %d %s", device, status, body)
		}
	}
	if socket {
		var err error
		if a.socket, _, err = s.openSocket(t, a.token); err != nil {
			t.Fatal(err)
		}
		// The socket is known to the server before its first message.
		if message := a.socket.next(t); message.MessageType != "ForceKeepAlive" {
			t.Fatalf("first message of %s: %+v", device, message)
		}
	}
	own := s.sessions(t, a.token, "deviceId="+device+"-id")
	if len(own) != 1 {
		t.Fatalf("sessions of %s: %+v", device, own)
	}
	a.session = own[0].Id
	return a
}

func (s testServer) sessions(t *testing.T, token, query string) []SessionInfo {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Sessions?"+query, app("any", token), nil)
	if status != http.StatusOK {
		t.Fatalf("sessions with %q: %d %s", query, status, body)
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatal(err)
	}
	return sessions
}

func sessionIDs(sessions []SessionInfo) []string {
	result := make([]string, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, session.Id)
	}
	slices.Sort(result)
	return result
}

func sorted(values ...string) []string {
	slices.Sort(values)
	return values
}

func randomID() accounts.ID {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id
}

// command reads the next message as a command of the kind given, its data
// decoded into data.
func (a *appSocket) command(t *testing.T, kind string, data any) {
	t.Helper()
	message := a.next(t)
	if message.MessageType != kind || message.MessageId == "" {
		t.Fatalf("message %s %q, want %s: %+v", message.MessageType, message.MessageId, kind, message.Data)
	}
	encoded, _ := json.Marshal(message.Data)
	if err := json.Unmarshal(encoded, data); err != nil {
		t.Fatalf("%s data %s: %v", kind, encoded, err)
	}
}

func TestControllableSessionsAreListedWithTheirCapabilities(t *testing.T) {
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	tv := s.remoteApp(t, "alice", "tv", queryMediaControl, true)
	box := s.remoteApp(t, "alice", "box", bodyMediaControl, true)
	// A socket without declared media control, or the reverse, is not
	// enough.
	phone := s.remoteApp(t, "alice", "phone", noMediaControl, true)
	tablet := s.remoteApp(t, "alice", "tablet", queryMediaControl, false)

	controllable := "controllableByUserId=" + alice.ID.String()
	listed := s.sessions(t, phone.token, controllable)
	if got := sessionIDs(listed); !slices.Equal(got, sorted(tv.session, box.session)) {
		t.Fatalf("controllable sessions %v, want tv and box", got)
	}
	for _, session := range listed {
		want := ClientCapabilities{PlayableMediaTypes: []string{"Video"}, SupportedCommands: []string{"DisplayMessage", "GoHome"}, SupportsMediaControl: true, SupportsPersistentIdentifier: true}
		if session.Id == box.session {
			want = ClientCapabilities{PlayableMediaTypes: []string{"Audio", "Video"}, SupportedCommands: []string{"SetVolume"}, SupportsMediaControl: true}
		}
		if !session.SupportsRemoteControl || !session.SupportsMediaControl || !reflect.DeepEqual(session.Capabilities, want) ||
			!slices.Equal(session.PlayableMediaTypes, want.PlayableMediaTypes) || !slices.Equal(session.SupportedCommands, want.SupportedCommands) {
			t.Errorf("session of %s: %+v", session.DeviceName, session)
		}
	}

	// Every session is still listed without the filter, those that cannot
	// be controlled saying so.
	all := s.sessions(t, phone.token, "")
	if got := sessionIDs(all); !slices.Equal(got, sorted(tv.session, box.session, phone.session, tablet.session)) {
		t.Fatalf("sessions %v", got)
	}
	for _, session := range all {
		if remote := session.Id == tv.session || session.Id == box.session; session.SupportsRemoteControl != remote || session.SupportsMediaControl != remote {
			t.Errorf("%s: remote control %v, media control %v", session.DeviceName, session.SupportsRemoteControl, session.SupportsMediaControl)
		}
		if session.Id == tablet.session && !session.Capabilities.SupportsMediaControl {
			t.Errorf("tablet's declared capabilities are lost: %+v", session.Capabilities)
		}
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "sessions-controllable.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, body := s.call(http.MethodGet, "/Sessions?"+controllable, app("phone", phone.token), nil)
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	// The recorded session had been paused by a command; these have not.
	for _, difference := range compareShapes("sessions-controllable", want, got, shapeRules{absent: map[string][]string{"LastPausedDate": {"*"}}}) {
		t.Error(difference)
	}

	// A session that closed its socket can no longer be controlled.
	_ = tv.socket.conn.CloseNow()
	deadline := time.Now().Add(3 * time.Second)
	for !slices.Equal(sessionIDs(s.sessions(t, phone.token, controllable)), []string{box.session}) {
		if time.Now().After(deadline) {
			t.Fatal("the tv is still controllable after closing its socket")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCommandsReachTheControlledApp(t *testing.T) {
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	tv := s.remoteApp(t, "alice", "tv", queryMediaControl, true)
	phone := s.remoteApp(t, "alice", "phone", noMediaControl, true)
	controller := alice.ID.String()
	movie, episode := randomID(), randomID()
	send := func(path string, body any) int {
		t.Helper()
		status, answer := s.call(http.MethodPost, "/Sessions/"+tv.session+path, app("phone", phone.token), body)
		if status != http.StatusNoContent && status/100 != 4 {
			t.Fatalf("%s: %d %s", path, status, answer)
		}
		return status
	}

	for _, tc := range []struct {
		path string
		body any
		kind string
		want any
	}{
		// Identifiers come back in Jellyfin's form, whatever the app sent;
		// positions go beyond 32 bits.
		{"/Playing?playCommand=PlayNext&itemIds=" + hyphenated(movie) + "," + episode.String() +
			"&startPositionTicks=60000000000&mediaSourceId=source&audioStreamIndex=1&subtitleStreamIndex=3&startIndex=1",
			nil, "Play", &PlayRequest{ItemIds: []string{movie.String(), episode.String()}, StartPositionTicks: new(int64(60_000_000_000)),
				PlayCommand: "PlayNext", ControllingUserId: controller, SubtitleStreamIndex: new(3), AudioStreamIndex: new(1),
				MediaSourceId: "source", StartIndex: new(1)}},
		{"/Playing?playCommand=playnow&itemIds=" + movie.String(), nil, "Play",
			&PlayRequest{ItemIds: []string{movie.String()}, PlayCommand: "PlayNow", ControllingUserId: controller}},
		{"/Playing/Pause", nil, "Playstate", &PlaystateRequest{Command: "Pause", ControllingUserId: controller}},
		{"/Playing/seek?seekPositionTicks=27000000000", nil, "Playstate",
			&PlaystateRequest{Command: "Seek", SeekPositionTicks: new(int64(27_000_000_000)), ControllingUserId: controller}},
		{"/Command/GoHome", nil, "GeneralCommand", &GeneralCommand{Name: "GoHome", ControllingUserId: controller, Arguments: map[string]string{}}},
		{"/Command", map[string]any{"Name": "SetVolume", "Arguments": map[string]string{"Volume": "40"}, "ControllingUserId": "someone else"}, "GeneralCommand",
			&GeneralCommand{Name: "SetVolume", ControllingUserId: controller, Arguments: map[string]string{"Volume": "40"}}},
		{"/Message", map[string]any{"Header": "Dinner", "Text": "Pause the movie", "TimeoutMs": 5000}, "GeneralCommand",
			&GeneralCommand{Name: "DisplayMessage", ControllingUserId: controller, Arguments: map[string]string{"Header": "Dinner", "Text": "Pause the movie", "TimeoutMs": "5000"}}},
		{"/Viewing?itemType=Movie&itemId=" + movie.String() + "&itemName=The%20Movie", nil, "GeneralCommand",
			&GeneralCommand{Name: "DisplayContent", ControllingUserId: controller, Arguments: map[string]string{"ItemType": "Movie", "ItemId": movie.String(), "ItemName": "The Movie"}}},
		{"/System/GoToSettings", nil, "GeneralCommand", &GeneralCommand{Name: "GoToSettings", ControllingUserId: controller, Arguments: map[string]string{}}},
	} {
		if status := send(tc.path, tc.body); status != http.StatusNoContent {
			t.Errorf("%s: %d", tc.path, status)
			continue
		}
		got := reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
		tv.socket.command(t, tc.kind, got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.path, got, tc.want)
		}
	}

	// Requests that do not bind are refused, and nothing is sent.
	for path, body := range map[string]any{
		"/Playing?playCommand=PlayNow":                                  nil,
		"/Playing?playCommand=PlayEverything&itemIds=" + movie.String(): nil,
		"/Playing/Jump":                        nil,
		"/Playing/Seek?seekPositionTicks=soon": nil,
		"/Command/Dance":                       nil,
		"/Command":                             map[string]any{"Name": "Dance"},
		"/Message":                             map[string]any{"Header": "Dinner"},
		"/Viewing?itemType=Movie":              nil,
	} {
		if status := send(path, body); status != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, status)
		}
	}
	for _, session := range []string{randomID().String(), "not-a-session"} {
		if status, _ := s.call(http.MethodPost, "/Sessions/"+session+"/Playing/Pause", app("phone", phone.token), nil); status != http.StatusNotFound {
			t.Errorf("unknown session %s: %d, want 404", session, status)
		}
	}
	tv.socket.quiet(t)
	// The controlling app is not told what it sent.
	phone.socket.quiet(t)
}

func TestCommandsToSessionsThatCannotBeControlledAreDropped(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	phone := s.remoteApp(t, "alice", "phone", noMediaControl, true)
	tablet := s.remoteApp(t, "alice", "tablet", queryMediaControl, false)
	for _, target := range []remoteApp{phone, tablet} {
		for _, path := range []string{"/Playing/Pause", "/Command/GoHome", "/Playing?playCommand=PlayNow&itemIds=" + randomID().String()} {
			if status, body := s.call(http.MethodPost, "/Sessions/"+target.session+path, app("phone", phone.token), nil); status != http.StatusNoContent {
				t.Errorf("%s to %s: %d %s", path, target.session, status, body)
			}
		}
	}
	phone.socket.quiet(t)

	// Once the tablet opens a socket, the commands reach it.
	socket, _, err := s.openSocket(t, tablet.token)
	if err != nil {
		t.Fatal(err)
	}
	socket.next(t)
	if status, _ := s.call(http.MethodPost, "/Sessions/"+tablet.session+"/Playing/Unpause", app("phone", phone.token), nil); status != http.StatusNoContent {
		t.Fatalf("unpause: %d", status)
	}
	var request PlaystateRequest
	socket.command(t, "Playstate", &request)
	if request.Command != "Unpause" {
		t.Errorf("tablet received %+v", request)
	}
}

func TestOnlyAdministratorsControlOtherUsersSessions(t *testing.T) {
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	bob := s.user("bob", nil)
	administrator := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	aliceTV := s.remoteApp(t, "alice", "tv", queryMediaControl, true)
	bobTV := s.remoteApp(t, "bob", "tv", queryMediaControl, true)
	console := s.remoteApp(t, "admin", "console", noMediaControl, false)

	// A member sees and controls their own sessions only, and may not ask
	// for those another user controls.
	if got := sessionIDs(s.sessions(t, aliceTV.token, "controllableByUserId="+alice.ID.String())); !slices.Equal(got, []string{aliceTV.session}) {
		t.Errorf("alice may control %v", got)
	}
	if status, _ := s.call(http.MethodGet, "/Sessions?controllableByUserId="+bob.ID.String(), app("tv", aliceTV.token), nil); status != http.StatusForbidden {
		t.Errorf("alice listing bob's controllable sessions: %d", status)
	}
	for _, path := range []string{"/Playing/Pause", "/Message", "/Playing?playCommand=PlayNow&itemIds=" + randomID().String()} {
		status, _ := s.call(http.MethodPost, "/Sessions/"+bobTV.session+path, app("tv", aliceTV.token), map[string]string{"Text": "hello"})
		if status != http.StatusForbidden {
			t.Errorf("alice sending %s to bob: %d, want 403", path, status)
		}
	}
	bobTV.socket.quiet(t)

	// An administrator controls everyone's sessions, and sees those a
	// member may control as the member does.
	if got := sessionIDs(s.sessions(t, console.token, "controllableByUserId="+administrator.ID.String())); !slices.Equal(got, sorted(aliceTV.session, bobTV.session)) {
		t.Errorf("the administrator may control %v", got)
	}
	if got := sessionIDs(s.sessions(t, console.token, "controllableByUserId="+bob.ID.String())); !slices.Equal(got, []string{bobTV.session}) {
		t.Errorf("bob may control %v", got)
	}
	if status, _ := s.call(http.MethodPost, "/Sessions/"+bobTV.session+"/Playing/Stop", app("console", console.token), nil); status != http.StatusNoContent {
		t.Fatalf("the administrator stopping bob's tv: %d", status)
	}
	var request PlaystateRequest
	bobTV.socket.command(t, "Playstate", &request)
	if request.Command != "Stop" || request.ControllingUserId != administrator.ID.String() {
		t.Errorf("bob's tv received %+v", request)
	}
	aliceTV.socket.quiet(t)
}
