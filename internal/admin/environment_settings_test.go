package admin

import (
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// environmentSettings are the settings that were options of the
// environment, as the admin API names them.
var environmentSettings = []string{"cacheSizeGb", "vaapiDevice", "recording", "recordingsFolder", "backups", "backupFolder"}

// The settings that were options of the environment are answered with the
// default folders under the data folder and the render nodes found, which
// a PUT ignores; a PUT saves them, and keeps those it leaves out.
func TestSettingsFromTheEnvironment(t *testing.T) {
	nodes := t.TempDir()
	for _, name := range []string{"renderD129", "renderD128", "card0"} {
		if err := os.WriteFile(filepath.Join(nodes, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pattern := renderNodesPattern
	renderNodesPattern = filepath.Join(nodes, "renderD*")
	t.Cleanup(func() { renderNodesPattern = pattern })
	var dataDir string
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { dataDir = o.DataDir })
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	want := map[string]any{"cacheSizeGb": 10.0, "vaapiDevice": "", "recording": false, "recordingsFolder": "", "backups": false, "backupFolder": "",
		"recordingsFolderDefault": filepath.Join(dataDir, "recordings"), "backupFolderDefault": filepath.Join(dataDir, "backups"),
		"renderNodes": []any{filepath.Join(nodes, "renderD128"), filepath.Join(nodes, "renderD129")}}
	for name, value := range want {
		if !reflect.DeepEqual(body[name], value) {
			t.Errorf("%s: %#v, want %#v", name, body[name], value)
		}
	}

	recordings, backups := t.TempDir(), t.TempDir()
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	put := maps.Clone(base)
	maps.Copy(put, map[string]any{"cacheSizeGb": 25, "vaapiDevice": "/dev/dri/renderD129", "recording": true, "recordingsFolder": recordings + "/",
		"backups": true, "backupFolder": backups,
		"recordingsFolderDefault": "/elsewhere", "backupFolderDefault": "/elsewhere", "renderNodes": []string{"/dev/dri/renderD1"}})
	saved := map[string]any{"cacheSizeGb": 25.0, "vaapiDevice": "/dev/dri/renderD129", "recording": true, "recordingsFolder": recordings,
		"backups": true, "backupFolder": backups}
	check := func(when string, body map[string]any) {
		t.Helper()
		for _, name := range environmentSettings {
			if !reflect.DeepEqual(body[name], saved[name]) {
				t.Errorf("%s: %s %#v, want %#v", when, name, body[name], saved[name])
			}
		}
		for _, name := range []string{"recordingsFolderDefault", "backupFolderDefault", "renderNodes"} {
			if !reflect.DeepEqual(body[name], want[name]) {
				t.Errorf("%s: %s %#v changed", when, name, body[name])
			}
		}
	}
	status, body, _ := administrator.call(http.MethodPut, "/settings", put)
	if status != http.StatusOK {
		t.Fatalf("saving: %d %v", status, body)
	}
	check("saved", body)
	if got := api.store.Settings(); got.CacheSizeGB != 25 || got.RecordingsFolder != recordings || !got.Backups {
		t.Errorf("stored: %+v", got)
	}
	// A page older than them leaves them as they are.
	status, body, _ = administrator.call(http.MethodPut, "/settings", base)
	if status != http.StatusOK {
		t.Fatalf("saving without them: %d %v", status, body)
	}
	check("saved without them", body)

	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"cacheSizeGb", 0, "invalid_cache_size"},
		{"cacheSizeGb", 2001, "invalid_cache_size"},
		{"vaapiDevice", "/dev/dri/card0", "invalid_vaapi_device"},
		{"recordingsFolder", "relative/path", "invalid_recordings_folder"},
		{"backupFolder", "relative/path", "invalid_backup_folder"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
}

// Recording or backups turned on into the default folder create it; into
// a folder that does not exist, they are refused, and nothing is saved.
func TestSettingsFoldersMustBeWritable(t *testing.T) {
	var dataDir string
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { dataDir = o.DataDir })
	administrator := api.signedIn("administrator", true)
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	for _, f := range []struct {
		on, folder, code, name string
	}{
		{"recording", "recordingsFolder", "invalid_recordings_folder", "recordings"},
		{"backups", "backupFolder", "invalid_backup_folder", "backups"},
	} {
		missing := maps.Clone(base)
		maps.Copy(missing, map[string]any{f.on: true, f.folder: filepath.Join(t.TempDir(), "missing"), "cacheSizeGb": 30})
		if status, body, _ := administrator.call(http.MethodPut, "/settings", missing); status != http.StatusBadRequest || body["error"] != f.code {
			t.Errorf("%s into a missing folder: %d %v", f.on, status, body)
		}
		if got := api.store.Settings(); got.Recording || got.Backups || got.RecordingsFolder != "" || got.BackupFolder != "" || got.CacheSizeGB != 10 {
			t.Errorf("%s into a missing folder saved: %+v", f.on, got)
		}

		folder := filepath.Join(dataDir, f.name)
		if _, err := os.Stat(folder); !os.IsNotExist(err) {
			t.Fatalf("%s exists already: %v", folder, err)
		}
		turnOn := maps.Clone(base)
		turnOn[f.on] = true
		if status, body, _ := administrator.call(http.MethodPut, "/settings", turnOn); status != http.StatusOK || body[f.on] != true {
			t.Errorf("%s into the default folder: %d %v", f.on, status, body)
		}
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			t.Errorf("the default %s folder was not made: %v", f.name, err)
		}
		turnOff := maps.Clone(base)
		turnOff[f.on] = false
		if status, body, _ := administrator.call(http.MethodPut, "/settings", turnOff); status != http.StatusOK {
			t.Errorf("%s off: %d %v", f.on, status, body)
		}
	}
}
