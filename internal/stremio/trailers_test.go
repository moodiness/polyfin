package stremio

import (
	"encoding/json"
	"slices"
	"testing"
)

// A description's trailers are read from its trailer streams first, then
// its trailers, each once: a YouTube identifier or page is a page of the
// video site; a link to another web address is a video to play, a stream
// of the addon, with the headers it gives; a trailer stream of another
// kind, or a source of another scheme, is none.
func TestTrailerLinks(t *testing.T) {
	t.Parallel()
	data := `{"id":"tt1","type":"movie","name":"One",
		"trailerStreams":[
			{"title":"Teaser","url":"https://cdn.example/teaser.mp4","behaviorHints":{"proxyHeaders":{"request":{"Referer":"https://cdn.example/"}}}},
			{"title":"Official","ytId":"abc"},
			{"title":"Torrent","infoHash":"0123456789abcdef0123456789abcdef01234567"}],
		"trailers":[
			{"source":"abc","type":"Trailer"},
			{"source":"https://youtu.be/def","type":"Clip"},
			{"source":"https://cdn.example/teaser.mp4","type":"Trailer"},
			{"source":"http://cdn.example/second.mp4","name":"Second"},
			{"source":"ftp://cdn.example/third.mp4"},
			{"source":""}]}`
	var d described
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		t.Fatal(err)
	}
	meta := d.meta()
	if len(meta.TrailerStreams) != 3 {
		t.Fatalf("trailer streams: %+v", meta.TrailerStreams)
	}
	type link struct{ name, page, url string }
	var got []link
	for _, l := range meta.TrailerLinks() {
		got = append(got, link{l.Name, l.Page, l.Stream.URL})
	}
	want := []link{
		{"Teaser", "", "https://cdn.example/teaser.mp4"},
		{"Official", "https://www.youtube.com/watch?v=abc", ""},
		{"Clip", "https://youtu.be/def", ""},
		{"Second", "", "http://cdn.example/second.mp4"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("links: %+v", got)
	}
	if headers := meta.TrailerLinks()[0].Stream.RequestHeaders(); headers["Referer"] != "https://cdn.example/" {
		t.Errorf("the teaser's headers: %v", headers)
	}
	if sites := meta.SiteTrailers(); !slices.Equal(sites, []Trailer{{Source: "https://www.youtube.com/watch?v=abc", Name: "Official"},
		{Source: "https://youtu.be/def", Name: "Clip"}}) {
		t.Errorf("site trailers: %+v", sites)
	}
}
