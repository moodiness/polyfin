package stremio

import (
	"encoding/json"
	"testing"
)

// The configure page is where Stremio opens it: the manifest URL with
// manifest.json replaced by configure, the query kept.
func TestConfigureURLReplacesManifestJSONAndKeepsTheQuery(t *testing.T) {
	t.Parallel()
	for manifestURL, want := range map[string]string{
		"https://addon.example/manifest.json":                        "https://addon.example/configure",
		"https://addon.example/u/secret-token/manifest.json":         "https://addon.example/u/secret-token/configure",
		"https://addon.example/manifest.json?token=a%2Fb&lang=fr":    "https://addon.example/configure?token=a%2Fb&lang=fr",
		"http://192.168.1.2:3000/a%2Fb/manifest.json?x=1":            "http://192.168.1.2:3000/a%2Fb/configure?x=1",
		"https://addon.example/manifest.json/settings/manifest.json": "https://addon.example/manifest.json/settings/configure",
	} {
		if got, ok := ConfigureURL(manifestURL); !ok || got != want {
			t.Errorf("%s: got %q, %v; want %q", manifestURL, got, ok, want)
		}
	}
	for _, manifestURL := range []string{"https://addon.example/catalog.json", "https://addon.example/", "https://addon.example/mymanifest.json",
		"/srv/media/movies", "manifest.json", ""} {
		if got, ok := ConfigureURL(manifestURL); ok {
			t.Errorf("%s: got %q", manifestURL, got)
		}
	}
}

// A manifest declaring itself configurable keeps it as it is stored.
func TestTheConfigurableHintIsKept(t *testing.T) {
	t.Parallel()
	for manifest, want := range map[string]bool{
		`{"id": "x", "name": "X", "version": "1", "resources": ["stream"], "behaviorHints": {"configurable": true}}`:  true,
		`{"id": "x", "name": "X", "version": "1", "resources": ["stream"], "behaviorHints": {"configurable": false}}`: false,
		`{"id": "x", "name": "X", "version": "1", "resources": ["stream"]}`:                                           false,
	} {
		parsed, err := ParseManifest([]byte(manifest))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(parsed)
		if err != nil {
			t.Fatal(err)
		}
		var stored Manifest
		if err := json.Unmarshal(encoded, &stored); err != nil {
			t.Fatal(err)
		}
		if stored.BehaviorHints.Configurable != want {
			t.Errorf("%s: stored %s", manifest, encoded)
		}
	}
}
