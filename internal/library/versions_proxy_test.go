package library

import (
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Only a server Stremio addon's direct link is read through the source
// proxy: not a torrent, a user's addon, IPTV or a local file.
func TestVersionProxySource(t *testing.T) {
	t.Parallel()
	stream := stremio.Stream{URL: "https://example.org/movie.mkv"}
	stremioAddon := func(kind string, confined bool) installed {
		return installed{addon: addons.Addon{Kind: kind, Manifest: stremio.Manifest{Name: "Addon"}}, confined: confined}
	}
	for _, tc := range []struct {
		name   string
		entry  installed
		stream stremio.Stream
		want   bool
	}{
		{"server Stremio addon", stremioAddon(addons.KindStremio, false), stream, true},
		{"user's Stremio addon", stremioAddon(addons.KindStremio, true), stream, false},
		{"IPTV source", stremioAddon(addons.KindM3U, false), stream, false},
		{"torrent", stremioAddon(addons.KindStremio, false), stremio.Stream{InfoHash: strings.Repeat("a", 40)}, false},
	} {
		version := newVersion(target{}, streamSource{installed: tc.entry}, tc.stream)
		if version.ProxySource != tc.want {
			t.Errorf("%s: ProxySource = %t, want %t", tc.name, version.ProxySource, tc.want)
		}
	}
}
