package admin

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestSettingsRecordings(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	recording := func(body map[string]any) [4]any {
		return [4]any{body["recordingPrePadding"], body["recordingPostPadding"], body["recordingRetentionDays"], body["recordingsFolder"]}
	}
	// Without POLYFIN_RECORDINGS_DIR, recording is off: no folder.
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); recording(body) != [4]any{0.0, 0.0, 0.0, ""} {
		t.Errorf("default settings: %v", body)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"recordingPrePadding": 120, "recordingPostPadding": 600, "recordingRetentionDays": 30,
		"recordingsFolder": "/elsewhere"})
	saved := [4]any{120.0, 600.0, 30.0, ""}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || recording(body) != saved {
		t.Fatalf("saving the recording settings: %d %v", status, body)
	}
	// A page or script older than them leaves them as they are.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || recording(body) != saved {
		t.Errorf("saving without them: %d %v", status, body)
	}
	if got := api.store.Settings(); got.RecordingPrePadding != 120 || got.RecordingPostPadding != 600 || got.RecordingRetentionDays != 30 {
		t.Errorf("stored: %+v", got)
	}
	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"recordingPrePadding", -1, "invalid_recording_padding"},
		{"recordingPostPadding", accounts.MaxRecordingPadding + 1, "invalid_recording_padding"},
		{"recordingRetentionDays", -1, "invalid_recording_retention_days"},
		{"recordingRetentionDays", accounts.MaxRecordingRetentionDays + 1, "invalid_recording_retention_days"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
}

// The folder the configuration names shows in the settings.
func TestSettingsShowTheRecordingsFolder(t *testing.T) {
	api := newTestAPI(t, 10)
	h := &handler{Options: Options{Accounts: api.store, RecordingsDir: "/recordings"}}
	response := httptest.NewRecorder()
	h.settings(response, httptest.NewRequest(http.MethodGet, "/admin/api/settings", nil))
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["recordingsFolder"] != "/recordings" {
		t.Errorf("settings with a recordings folder: %v %v", body, err)
	}
}

func TestAdministratorsLetUsersRecord(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	_, second, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "second", "password": "correct horse", "isAdministrator": true})
	if created["liveTvManagement"] != false || second["liveTvManagement"] != true {
		t.Fatalf("new users may record: %v, administrator %v", created["liveTvManagement"], second["liveTvManagement"])
	}
	path := "/users/" + created["id"].(string)
	if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"liveTvManagement": true}); status != http.StatusOK || updated["liveTvManagement"] != true {
		t.Fatalf("letting a user record: %d %v", status, updated)
	}
	if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"isHidden": true}); status != http.StatusOK || updated["liveTvManagement"] != true {
		t.Errorf("a change without it: %d %v", status, updated)
	}
}
