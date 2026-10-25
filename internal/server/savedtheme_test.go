package server

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// The copy of a theme's files is served from its folder only: its files,
// cached for good with the type of their extension and a policy that keeps
// them from running anything opened on their own; nothing else, nor a path
// out of the folder, under the web client's address and its base path.
func TestSavedThemeIsServedFromItsFolderOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "themes")
	id := "0123456789abcdef"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"001-theme.css": "body {}", "002-x.woff2": "wOF2", "003-back.svg": "<svg/>"}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, id, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "secret.css"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "secret.css"), filepath.Join(dir, id, "004-link.css")); err != nil {
		t.Fatal(err)
	}
	h := New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           webFiles,
		SetupRequired: setupState{}.SetupRequired,
		SavedThemes:   dir,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for name, want := range map[string]string{"001-theme.css": "text/css; charset=utf-8", "002-x.woff2": "font/woff2", "003-back.svg": "image/svg+xml"} {
		for _, prefix := range []string{"/web/", "/jellyfin/web/"} {
			response := get(h, prefix+"polyfin/themes/saved/"+id+"/"+name)
			header := response.Header()
			if response.Code != http.StatusOK || response.Body.String() != files[name] || header.Get("Content-Type") != want ||
				header.Get("Cache-Control") != immutableCache || header.Get("Content-Security-Policy") != savedThemePolicy ||
				header.Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("%s%s: %d %q %v", prefix, name, response.Code, response.Body.String(), header)
			}
		}
	}
	for _, target := range []string{
		"/web/polyfin/themes/saved/" + id + "/004-link.css",
		"/web/polyfin/themes/saved/" + id + "/..%2f..%2fsecret.css",
		"/web/polyfin/themes/saved/" + id + "/missing.css",
		"/web/polyfin/themes/saved/" + id + "/005-missing.css",
		"/web/polyfin/themes/saved/" + id,
		"/web/polyfin/themes/saved/",
		"/web/polyfin/themes/saved/abc/001-theme.css",
	} {
		if response := get(h, target); response.Code != http.StatusNotFound || response.Body.String() == "secret" {
			t.Errorf("%s: %d %q", target, response.Code, response.Body.String())
		}
	}
	// Without a folder, nothing is served.
	if response := get(webHandler(setupState{}), "/web/polyfin/themes/saved/"+id+"/001-theme.css"); response.Code != http.StatusNotFound {
		t.Errorf("without a folder: %d", response.Code)
	}
}

// A file of the copy kept that is missing from its folder sends the browser
// to where it came from, without being kept; a file present is served, and
// a name with no original answers 404.
func TestAMissingSavedThemeFileGoesWhereItCameFrom(t *testing.T) {
	dir := t.TempDir()
	id := "0123456789abcdef"
	if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id, "001-theme.css"), []byte("body {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	originals := map[string]string{
		id + "/001-theme.css": "https://themes.example/theme.css",
		id + "/002-x.woff2":   "https://themes.example/fonts/x.woff2",
	}
	h := New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           webFiles,
		SetupRequired: setupState{}.SetupRequired,
		SavedThemes:   dir,
		SavedThemeOriginal: func(id, name string) (string, bool) {
			address, ok := originals[id+"/"+name]
			return address, ok
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if response := get(h, "/web/polyfin/themes/saved/"+id+"/001-theme.css"); response.Code != http.StatusOK || response.Body.String() != "body {}" {
		t.Errorf("a file present: %d %q", response.Code, response.Body.String())
	}
	for _, prefix := range []string{"/web/", "/jellyfin/web/"} {
		response := get(h, prefix+"polyfin/themes/saved/"+id+"/002-x.woff2")
		if response.Code != http.StatusFound || response.Header().Get("Location") != "https://themes.example/fonts/x.woff2" ||
			response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s, a missing file: %d %v", prefix, response.Code, response.Header())
		}
	}
	for _, name := range []string{"004-unknown.css"} {
		if response := get(h, "/web/polyfin/themes/saved/"+id+"/"+name); response.Code != http.StatusNotFound {
			t.Errorf("%s: %d %v", name, response.Code, response.Header())
		}
	}
}
