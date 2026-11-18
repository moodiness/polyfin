package admin

import (
	"errors"
	"maps"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/config"
)

// The conversion folder, the folder for files being read and the cache
// folder are refused when Polyfin cannot write into them, and nothing is
// saved; saved, the next start uses them, while the running one keeps
// its own.
func TestWorkFoldersMustBeWritable(t *testing.T) {
	t.Parallel()
	var dataDir string
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { dataDir = o.DataDir })
	administrator := api.signedIn("administrator", true)
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["cacheFolderDefault"] != filepath.Join(dataDir, "cache") ||
		body["conversionFolder"] != "" || body["readingFolder"] != "" || body["cacheFolder"] != "" {
		t.Errorf("default: %v %v %v %v", body["cacheFolderDefault"], body["conversionFolder"], body["readingFolder"], body["cacheFolder"])
	}
	for _, f := range []struct{ key, code string }{
		{"conversionFolder", "invalid_conversion_folder"},
		{"readingFolder", "invalid_reading_folder"},
		{"cacheFolder", "invalid_cache_folder"},
	} {
		missing := maps.Clone(base)
		maps.Copy(missing, map[string]any{f.key: filepath.Join(t.TempDir(), "missing"), "cacheSizeGb": 30})
		if status, body, _ := administrator.call(http.MethodPut, "/settings", missing); status != http.StatusBadRequest || body["error"] != f.code {
			t.Errorf("%s missing: %d %v", f.key, status, body)
		}
		relative := maps.Clone(base)
		relative[f.key] = "relative/path"
		if status, body, _ := administrator.call(http.MethodPut, "/settings", relative); status != http.StatusBadRequest || body["error"] != f.code {
			t.Errorf("%s relative: %d %v", f.key, status, body)
		}
	}
	if got := api.store.Settings(); got.ConversionFolder != "" || got.ReadingFolder != "" || got.CacheFolder != "" || got.CacheSizeGB != 10 {
		t.Errorf("refused folders saved: %+v", got)
	}

	conversion, reading, cache := t.TempDir(), t.TempDir(), t.TempDir()
	chosen := maps.Clone(base)
	maps.Copy(chosen, map[string]any{"conversionFolder": conversion, "readingFolder": reading, "cacheFolder": cache + string(filepath.Separator)})
	status, body, _ := administrator.call(http.MethodPut, "/settings", chosen)
	if status != http.StatusOK || body["conversionFolder"] != conversion || body["readingFolder"] != reading || body["cacheFolder"] != cache {
		t.Fatalf("saving: %d %v", status, body)
	}
	next := config.ChooseFolders(dataDir, api.store.Settings())
	if next.Conversion != conversion || next.Reading != reading || next.Cache != cache || next.Unwritable != nil {
		t.Errorf("next start: %+v", next)
	}
	// A page older than them leaves them as they are.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["cacheFolder"] != cache {
		t.Errorf("saving without them: %d %v", status, body)
	}
}

// A folder the settings chose that could not be written into at start is
// a problem of System › Health, which names it and links to its setting;
// Health measures the disk of each folder of the run, the data folder's
// cache in place of the unwritable one.
func TestUnwritableWorkFolderIsAHealthProblem(t *testing.T) {
	t.Parallel()
	place, cache := t.TempDir(), t.TempDir()
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health.Folders = config.Folders{Conversion: place, Reading: place, Cache: cache,
			Unwritable: []config.UnwritableFolder{{Name: config.ConversionFolder, Folder: "/mnt/missing", Err: errors.New("no such folder")}}}
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	var found map[string]any
	problems, _ := body["problems"].([]any)
	for _, p := range problems {
		if problem := p.(map[string]any); problem["code"] == "folder_unwritable" {
			found = problem
		}
	}
	if found == nil || found["folder"] != "conversion" || found["path"] != "/mnt/missing" || found["error"] != "no such folder" ||
		found["to"] != "/settings/conversion#conversion-folder" || found["tone"] != "warning" {
		t.Errorf("problem: %v", found)
	}

	var health healthJSON
	if status := administrator.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK {
		t.Fatalf("health: %d", status)
	}
	var disks []string
	for _, disk := range health.Disks {
		disks = append(disks, disk.Folder+" "+disk.Path)
	}
	if len(disks) != 2 || disks[0] != "cache "+cache || disks[1] != "conversion "+place {
		t.Errorf("disks: %v", disks)
	}
}

// A folder the settings chose that Polyfin left as it is at start, as it
// held files Polyfin did not make where a folder is emptied, is a problem
// of System › Health, which names the setting, the folder and what it
// holds, and links to the setting. The problem goes once the setting
// names another folder, which applies at the next start.
func TestOccupiedWorkFolderIsAHealthProblem(t *testing.T) {
	t.Parallel()
	place, downloads, other := t.TempDir(), t.TempDir(), t.TempDir()
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health.Folders = config.Folders{Conversion: place, Reading: place, Cache: place,
			Occupied: []config.OccupiedFolder{{Name: config.ReadingFolder, Folder: downloads, Holds: "torrents"}}}
	})
	administrator := api.signedIn("administrator", true)
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	choose := func(folder string) {
		t.Helper()
		chosen := maps.Clone(base)
		chosen["readingFolder"] = folder
		if status, body, _ := administrator.call(http.MethodPut, "/settings", chosen); status != http.StatusOK || body["readingFolder"] != folder {
			t.Fatalf("saving %s: %d %v", folder, status, body)
		}
	}
	occupied := func() map[string]any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		problems, _ := body["problems"].([]any)
		for _, p := range problems {
			if problem := p.(map[string]any); problem["code"] == "folder_occupied" {
				return problem
			}
		}
		return nil
	}
	choose(downloads)
	found := occupied()
	if found == nil || found["key"] != "folder_occupied:reading:torrents" || found["folder"] != "reading" || found["path"] != downloads || found["name"] != "torrents" ||
		found["to"] != "/settings/playback#reading-folder" || found["tone"] != "warning" {
		t.Errorf("problem: %v", found)
	}
	text := healthProblemJSON{Code: problemFolderOccupied, Folder: "reading", Path: downloads, Name: "torrents"}.text("en")
	if want := "The folder for files being read, " + downloads + ", already holds “torrents”, which Polyfin did not make: Polyfin leaves it as it is, " +
		"and keeps its own files in its data folder’s cache meanwhile. Choose another folder, or move “torrents” elsewhere, then restart Polyfin."; text != want {
		t.Errorf("text: %s", text)
	}
	for _, language := range []string{"fr", "de", "es", "it", "pt-BR"} {
		told := healthProblemJSON{Code: problemFolderOccupied, Folder: "reading", Path: downloads, Name: "torrents"}.text(language)
		if strings.Count(told, "torrents") != 2 || !strings.Contains(told, downloads) || strings.Contains(told, "%!") || told == text {
			t.Errorf("%s: %s", language, told)
		}
	}

	choose(other)
	if found := occupied(); found != nil {
		t.Errorf("with another folder chosen: %v", found)
	}
	choose("")
	if found := occupied(); found != nil {
		t.Errorf("with no folder chosen: %v", found)
	}
}
