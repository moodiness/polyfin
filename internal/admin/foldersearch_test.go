package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A local folder whose titles' searches failed tells it: each file left
// unmatched names the addon that failed and why, the folder counts them
// and tells when they are searched again, and System › Health and its
// notifications tell it while it lasts.
func TestFolderSearchFailuresTold(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta", Name: "Meta", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
			return
		}
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(addon.Close)
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		if _, err := deps.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
			t.Fatal(err)
		}
		o.Folders = localfiles.New(deps.pool, deps.addons, lib, logger, o.Accounts.Settings, nil)
		t.Cleanup(o.Folders.Close)
		options = o
	})
	admin := api.signedIn("root", true)
	dir := t.TempDir()
	for _, name := range []string{"The General (1926).mkv", "Nosferatu (1922).mkv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	if status := admin.raw(http.MethodPost, "/scopes/shared/folders", `{"name": "Shelf", "kind": "movies", "path": "`+dir+`"}`,
		&created); status != http.StatusCreated {
		t.Fatalf("folder: %d", status)
	}
	folder := func() map[string]any {
		t.Helper()
		var list []map[string]any
		if status := admin.raw(http.MethodGet, "/scopes/shared/addons", "", &list); status != http.StatusOK {
			t.Fatalf("addons: %d", status)
		}
		i := slices.IndexFunc(list, func(a map[string]any) bool { return a["id"] == created.ID })
		f, _ := list[i]["folder"].(map[string]any)
		return f
	}
	waitUntil(t, func() bool {
		f := folder()
		return f["scannedAt"] != nil && f["scanning"] == false
	})

	f := folder()
	if f["unmatched"] != 2.0 || f["searchFailed"] != 2.0 || f["searchRetryAt"] == nil {
		t.Errorf("folder: unmatched %v, searchFailed %v, searchRetryAt %v", f["unmatched"], f["searchFailed"], f["searchRetryAt"])
	}
	if failure, _ := f["searchFailure"].(map[string]any); failure["addon"] != "Meta" || failure["failure"] != "rate_limited" {
		t.Errorf("folder's failure: %v", f["searchFailure"])
	}
	var unmatched struct {
		Files []map[string]any `json:"files"`
	}
	if status := admin.raw(http.MethodGet, "/scopes/shared/folders/"+created.ID+"/unmatched", "", &unmatched); status != http.StatusOK ||
		len(unmatched.Files) != 2 {
		t.Fatalf("unmatched: %d %v", status, unmatched.Files)
	}
	for _, file := range unmatched.Files {
		if search, _ := file["search"].(map[string]any); file["reason"] != "search_failed" || search["addon"] != "Meta" ||
			search["failure"] != "rate_limited" {
			t.Errorf("%v: %v %v", file["path"], file["reason"], file["search"])
		}
	}

	var problems struct {
		Problems []map[string]any `json:"problems"`
	}
	if status := admin.raw(http.MethodGet, "/health/problems?language=en", "", &problems); status != http.StatusOK {
		t.Fatalf("problems: %d", status)
	}
	i := slices.IndexFunc(problems.Problems, func(p map[string]any) bool { return p["code"] == "folder_search" })
	if i < 0 {
		t.Fatalf("problems: %v", problems.Problems)
	}
	if p := problems.Problems[i]; p["key"] != "folder_search:"+created.ID || p["tone"] != "warning" || p["to"] != "/sources/shared/"+created.ID ||
		p["name"] != "Shelf" || p["addon"] != "Meta" || p["failure"] != "rate_limited" {
		t.Errorf("problem: %v", p)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	i = slices.IndexFunc(told, func(p notifications.Problem) bool { return p.Key == "folder_search:"+created.ID })
	if i < 0 || !strings.HasPrefix(told[i].Text, "Shelf: the titles of its files could not be searched (Meta: asked to slow down (too many requests)).") {
		t.Errorf("told: %+v", told)
	}
}
