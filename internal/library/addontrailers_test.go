package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// An addon trailer's item leads back to its title's, and differs from
// every other trailer's of the title; its Stremio identifier tells its
// number, which picks its stream from the description, and no addon
// serves it.
func TestAddonTrailerIdentifiers(t *testing.T) {
	t.Parallel()
	title := itemID(titleKey(KindMovie, "tt1"))
	seen := map[string]bool{title.String(): true}
	for n := range maxAddonTrailers {
		id := addonTrailerID(title, n)
		if seen[id.String()] || addonTrailerID(id, n) != title {
			t.Errorf("trailer %d: %s", n, id)
		}
		seen[id.String()] = true
	}
	meta := stremio.Meta{TrailerStreams: []stremio.Stream{{Title: "Teaser", URL: "https://cdn.example/a.mp4"}, {YouTubeID: "abc"}},
		Trailers: []stremio.Trailer{{Source: "https://cdn.example/b.mp4", Name: "Second"}}}
	if streams := videoStreams(meta, TrailerIDPrefix+"1"); len(streams) != 1 || streams[0].URL != "https://cdn.example/b.mp4" {
		t.Errorf("the second trailer's stream: %+v", streams)
	}
	for _, id := range []string{TrailerIDPrefix + "2", TrailerIDPrefix + "x", TrailerIDPrefix + "-1"} {
		if streams := videoStreams(meta, id); streams != nil {
			t.Errorf("%s: %+v", id, streams)
		}
	}
	if serves(installed{}, "stream", "movie", TrailerIDPrefix+"0") {
		t.Error("an addon serves an addon trailer")
	}
}
