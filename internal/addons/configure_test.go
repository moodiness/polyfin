package addons

import (
	"testing"

	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A Stremio or Eclipse addon whose manifest declares itself configurable,
// at an address ending in manifest.json, has a configure page; IPTV
// sources, tuners and local folders never do.
func TestOnlyDeclaredStremioAndEclipseAddonsAreConfigurable(t *testing.T) {
	t.Parallel()
	declared := stremio.Manifest{BehaviorHints: stremio.ManifestBehavior{Configurable: true}}
	music := &eclipse.Manifest{}
	for _, test := range []struct {
		name  string
		addon Addon
		want  string
	}{
		{"stremio, declared", Addon{Kind: KindStremio, ManifestURL: "https://addon.example/abc/manifest.json?key=1", Manifest: declared},
			"https://addon.example/abc/configure?key=1"},
		{"eclipse, declared", Addon{Kind: KindEclipse, Music: music, ManifestURL: "https://music.example/t/manifest.json", Manifest: declared},
			"https://music.example/t/configure"},
		{"stremio, not declared", Addon{Kind: KindStremio, ManifestURL: "https://addon.example/manifest.json"}, ""},
		{"stremio, another address", Addon{Kind: KindStremio, ManifestURL: "https://addon.example/abc/addon.json", Manifest: declared}, ""},
		{"m3u", Addon{Kind: KindM3U, ManifestURL: "https://iptv.example/manifest.json", Manifest: declared}, ""},
		{"xtream", Addon{Kind: KindXtream, ManifestURL: "https://iptv.example/manifest.json", Manifest: declared}, ""},
		{"tuner", Addon{Kind: KindHDHomeRun, ManifestURL: "http://192.168.1.9/manifest.json", Manifest: declared}, ""},
		{"local folder", Addon{Kind: KindLocal, ManifestURL: "/media/manifest.json", Manifest: declared}, ""},
	} {
		got, ok := test.addon.ConfigureURL()
		if got != test.want || ok != (test.want != "") || test.addon.Configurable() != ok {
			t.Errorf("%s: got %q, %v", test.name, got, ok)
		}
	}
}
