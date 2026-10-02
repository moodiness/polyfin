package stremio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	manifest, err := ParseManifest([]byte(`{
		"id": "org.example", "version": "1.0.0", "name": "Example",
		"logo": "http://insecure.example/logo.png",
		"resources": ["catalog", {"name": "stream", "types": ["movie"], "idPrefixes": ["tt"]}],
		"catalogs": [
			{"type": "movie", "id": "top", "name": "Top", "extra": [{"name": "genre", "isRequired": true, "options": ["All", "Action"]}, {"name": "skip"}]},
			{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]},
			{"type": "series", "id": "legacy", "extraSupported": ["search", "genre"], "extraRequired": ["search"]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.ResourceNames(); strings.Join(got, ",") != "catalog,stream" {
		t.Errorf("resources: %v", got)
	}
	if manifest.Logo != "" {
		t.Errorf("an insecure logo URL was kept: %q", manifest.Logo)
	}
	for id, want := range map[string]bool{"top": true, "search": false, "legacy": false} {
		catalog := manifest.Catalogs[map[string]int{"top": 0, "search": 1, "legacy": 2}[id]]
		if catalog.Browsable() != want {
			t.Errorf("%s browsable = %v, want %v", id, catalog.Browsable(), want)
		}
	}
	if legacy := manifest.Catalogs[2]; legacy.Name != "legacy" || len(legacy.Extra) != 2 {
		t.Errorf("legacy extras not converted: %+v", legacy)
	}

	for _, invalid := range []string{`[]`, `{"id": "x", "name": "x", "version": "1"}`, `<html>`} {
		if _, err := ParseManifest([]byte(invalid)); !errors.Is(err, ErrInvalidManifest) {
			t.Errorf("%s: got %v", invalid, err)
		}
	}
}

func TestManifestURLs(t *testing.T) {
	for raw, want := range map[string]string{
		" stremio://addon.example/abc/manifest.json ": "https://addon.example/abc/manifest.json",
		"HTTPS://addon.example/manifest.json#x":       "https://addon.example/manifest.json",
		"http://192.168.1.2:3000/manifest.json":       "http://192.168.1.2:3000/manifest.json",
	} {
		if got, err := NormalizeManifestURL(raw); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"ftp://addon.example/manifest.json", "https://addon.example/catalog.json", "https://user:pw@addon.example/manifest.json", "manifest.json"} {
		if _, err := NormalizeManifestURL(raw); !errors.Is(err, ErrInvalidManifestURL) {
			t.Errorf("%q accepted", raw)
		}
	}
	if got := RedactManifestURL("https://addon.example/u/secret-token/manifest.json"); got != "https://addon.example/…/manifest.json" {
		t.Errorf("redacted: %q", got)
	}
}

func TestConfinedRequestsStayOffLocalNetworks(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "x", "name": "X", "version": "1", "resources": ["catalog"]}`))
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/secret-token/manifest.json"

	if _, err := client.Manifest(t.Context(), manifestURL, false); err != nil {
		t.Fatalf("trusted request to a local addon: %v", err)
	}
	_, err := client.Manifest(t.Context(), manifestURL, true)
	if !errors.Is(err, ErrPrivateNetwork) {
		t.Fatalf("confined request to a local addon: got %v", err)
	}

	addon.Close()
	_, err = client.Manifest(t.Context(), manifestURL, false)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("closed addon: got %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("the error exposes the manifest URL: %v", err)
	}
}
