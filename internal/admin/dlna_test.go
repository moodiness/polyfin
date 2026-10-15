package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/dlna"
	"github.com/moodiness/polyfin/internal/library"
)

// Sharing over DLNA is off by default, under the server's name, new
// devices showing no one's libraries; a user who does not exist is
// refused, and null shows no one's again.
func TestSettingsDLNA(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if body["dlna"] != false || body["dlnaName"] != "" || body["dlnaNewDevicesUser"] != nil || body["dlnaMarkPlayed"] != false {
		t.Errorf("defaults: %v %v %v %v", body["dlna"], body["dlnaName"], body["dlnaNewDevicesUser"], body["dlnaMarkPlayed"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	with := func(values map[string]any) map[string]any {
		merged := map[string]any{}
		for k, v := range base {
			merged[k] = v
		}
		for k, v := range values {
			merged[k] = v
		}
		return merged
	}
	id := api.userID("administrator").String()
	status, body, _ := administrator.call(http.MethodPut, "/settings", with(map[string]any{"dlna": true, "dlnaName": " Living room ",
		"dlnaNewDevicesUser": id, "dlnaMarkPlayed": true}))
	if status != http.StatusOK || body["dlna"] != true || body["dlnaName"] != "Living room" || body["dlnaNewDevicesUser"] != id ||
		body["dlnaMarkPlayed"] != true {
		t.Fatalf("saving: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["dlnaNewDevicesUser"] != id ||
		body["dlna"] != true {
		t.Errorf("left out: %d %v %v", status, body["dlna"], body["dlnaNewDevicesUser"])
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(map[string]any{"dlnaNewDevicesUser": strings.Repeat("ab", 16)})); status != http.StatusBadRequest ||
		body["error"] != "invalid_dlna_user" {
		t.Errorf("unknown user: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(map[string]any{"dlnaName": strings.Repeat("a", 257)})); status != http.StatusBadRequest ||
		body["error"] != "invalid_dlna_name" {
		t.Errorf("long name: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(map[string]any{"dlnaNewDevicesUser": nil})); status != http.StatusOK ||
		body["dlnaNewDevicesUser"] != nil {
		t.Errorf("no one: %d %v", status, body["dlnaNewDevicesUser"])
	}
	if api.store.Settings().DLNANewDevicesUser != nil {
		t.Errorf("stored: %v", api.store.Settings().DLNANewDevicesUser)
	}
}

// noPlayer stands for the Jellyfin API's handler, which these tests never
// reach.
type noPlayer struct{ dlna.Player }

func (noPlayer) Allowed(accounts.User) bool { return true }

// The devices seen are listed with the profile they get; an administrator
// gives one a user, blocks it, forces a profile they added, and forgets
// it.
func TestDLNADevicesAndProfiles(t *testing.T) {
	t.Parallel()
	var service *dlna.Service
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		store := o.Accounts
		service = dlna.Start(t.Context(), dlna.Options{HTTP: &net.TCPAddr{Port: 8096}, ServerID: "0123", DB: deps.pool, Settings: store.Settings,
			Changed: store.SettingsChanged, User: store.User, Library: (*library.Service)(nil), Player: noPlayer{},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.DLNA = service
	})
	administrator := api.signedIn("administrator", true)
	settings := api.store.Settings()
	settings.DLNA = true
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	// A Samsung TV fetches the description.
	request := httptest.NewRequest(http.MethodGet, "/dlna/description.xml", nil)
	request.RemoteAddr = "192.168.1.30:50000"
	request.Header.Set("User-Agent", "SEC_HHP_[TV] Living Room/1.0 DLNADOC/1.50")
	request.Header.Set("X-AV-Client-Info", `av=5.0; cn="Samsung"; mn="UE55"; mv="1.0";`)
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("description: %d", recorder.Code)
	}
	list := func() []map[string]any {
		t.Helper()
		response, err := administrator.client.Get(api.url + "/admin/api/dlna/devices")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var devices []map[string]any
		_ = json.NewDecoder(response.Body).Decode(&devices)
		return devices
	}
	devices := list()
	if len(devices) != 1 || devices[0]["address"] != "192.168.1.30" || devices[0]["manufacturer"] != "Samsung" || devices[0]["model"] != "UE55" ||
		devices[0]["userId"] != nil || devices[0]["blocked"] != false || devices[0]["matchedProfile"] != "Samsung Smart TV" || devices[0]["profile"] != "" {
		t.Fatalf("devices: %v", devices)
	}
	device := devices[0]["id"].(string)
	user := api.userID("administrator").String()
	if status, body, _ := administrator.call(http.MethodPatch, "/dlna/devices/"+device, map[string]any{"userId": user, "blocked": true,
		"profile": "builtin:Sony Bravia"}); status != http.StatusOK || body["userId"] != user || body["userName"] != "administrator" ||
		body["blocked"] != true || body["matchedProfile"] != "Sony Bravia" {
		t.Fatalf("change: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, "/dlna/devices/"+device, map[string]any{"profile": "builtin:Nothing"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_dlna_profile" {
		t.Errorf("unknown profile: %d %v", status, body)
	}
	// Profiles: the built-in ones, then those added, in Jellyfin's XML.
	_, _, response := administrator.call(http.MethodGet, "/dlna/profiles", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("profiles: %d", response.StatusCode)
	}
	post := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), method, api.url+"/admin/api"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/xml")
		response, err := administrator.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var decoded map[string]any
		_ = json.NewDecoder(response.Body).Decode(&decoded)
		return response.StatusCode, decoded
	}
	if status, body := post(http.MethodPost, "/dlna/profiles", "<Profile><Name>Broken"); status != http.StatusBadRequest || body["error"] != "invalid_dlna_profile" {
		t.Errorf("broken XML: %d %v", status, body)
	}
	added := `<Profile><Name>Hall TV</Name><DirectPlayProfiles><DirectPlayProfile container="mp4" type="Video"/></DirectPlayProfiles></Profile>`
	status, body := post(http.MethodPost, "/dlna/profiles", added)
	if status != http.StatusCreated || body["name"] != "Hall TV" {
		t.Fatalf("added: %d %v", status, body)
	}
	profile := body["id"].(string)
	if status, body, _ := administrator.call(http.MethodPatch, "/dlna/devices/"+device, map[string]any{"profile": profile, "userId": nil}); status != http.StatusOK ||
		body["matchedProfile"] != "Hall TV" || body["userId"] != nil || body["blocked"] != true {
		t.Errorf("forced to the added profile: %d %v", status, body)
	}
	xml, err := administrator.client.Get(api.url + "/admin/api/dlna/profiles/" + profile)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(xml.Body)
	_ = xml.Body.Close()
	if string(data) != added {
		t.Errorf("profile XML: %s", data)
	}
	if status, _ := post(http.MethodDelete, "/dlna/profiles/"+profile, ""); status != http.StatusNoContent {
		t.Errorf("removing the profile: %d", status)
	}
	if devices := list(); devices[0]["profile"] != "" || devices[0]["matchedProfile"] != "Samsung Smart TV" {
		t.Errorf("a device forced to a removed profile: %v", devices[0])
	}
	if status, _ := post(http.MethodDelete, "/dlna/devices/"+device, ""); status != http.StatusNoContent || len(list()) != 0 {
		t.Errorf("forgetting the device: %d", status)
	}
	member := api.browser()
	if _, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"}); err != nil {
		t.Fatal(err)
	}
	member.call(http.MethodPost, "/session", map[string]string{"name": "member", "password": "correct horse"})
	if status, _, _ := member.call(http.MethodGet, "/dlna/devices", nil); status != http.StatusForbidden {
		t.Errorf("a member lists the devices: %d", status)
	}
}

// A DLNA port another program uses is a warning of System › Health,
// linked to the DLNA settings; turning sharing off ends it.
func TestHealthTellsATakenDLNAPort(t *testing.T) {
	t.Parallel()
	taken, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := taken.LocalAddr().(*net.UDPAddr).Port
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		settings := o.Accounts.Settings()
		settings.DLNA = true
		if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
		service := dlna.Start(t.Context(), dlna.Options{Port: port, HTTP: &net.TCPAddr{Port: 8096}, ServerID: "0123", DB: deps.pool,
			Settings: o.Accounts.Settings, Changed: o.Accounts.SettingsChanged, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.Health = HealthSources{DLNA: service.Status}
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	if len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "dlna:port", "code": "dlna_port", "tone": "warning",
		"to": "/settings/dlna#dlna", "transient": false, "port": float64(port)}) {
		t.Fatalf("problems: %v", found)
	}
}
