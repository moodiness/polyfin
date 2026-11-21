package library

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// hintedEnv serves one movie from an addon whose subtitles answer the
// hints asked with: 1 for the title, 2 then 1 for a video it was given.
func hintedEnv(t *testing.T) followEnv {
	t.Helper()
	return repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		if strings.Contains(r.URL.EscapedPath(), "videoHash=") {
			answerNumbered(w, resource, []int{2, 1})
			return
		}
		answerNumbered(w, resource, []int{1})
	})
}

// subtitleRequests lists the addon's subtitle requests, their paths after
// the title's identifier.
func (e followEnv) subtitleRequests() []string {
	e.addon.mu.Lock()
	defer e.addon.mu.Unlock()
	var asked []string
	for _, request := range e.addon.requests {
		if rest, ok := strings.CutPrefix(request, "subtitles/movie/"); ok {
			_, extra, _ := strings.Cut(rest, "/")
			asked = append(asked, extra)
		}
	}
	return asked
}

// urls lists the addresses of subtitle files.
func urls(subtitles []ExternalSubtitle) []string {
	addresses := make([]string, len(subtitles))
	for i, subtitle := range subtitles {
		addresses[i] = subtitle.URL
	}
	return addresses
}

// Subtitle addons are told the video's hash, size and file name, as
// request properties, when they are known: the list they give for it is
// its own, kept apart from the title's, which is asked without any.
func TestSubtitleAddonsAreToldTheVideo(t *testing.T) {
	t.Parallel()
	e := hintedEnv(t)
	hints := SubtitleHints{Filename: "Movie (2008) 1080p.mkv", Size: 1_234_567, Hash: "8e245d9679d31e12"}
	subtitles, err := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints)
	if want := []string{"https://cdn.example/movie/2.srt", "https://cdn.example/movie/1.srt"}; err != nil || !slices.Equal(urls(subtitles), want) {
		t.Fatalf("for the video: %q %v, want %q", urls(subtitles), err, want)
	}
	if got, want := e.subtitleRequests(), []string{"videoHash=8e245d9679d31e12&videoSize=1234567&filename=Movie%20(2008)%201080p.mkv"}; !slices.Equal(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
	subtitles, err = e.service.Subtitles(t.Context(), e.member, e.movie)
	if want := []string{"https://cdn.example/movie/1.srt"}; err != nil || !slices.Equal(urls(subtitles), want) {
		t.Fatalf("for the title: %q %v, want %q", urls(subtitles), err, want)
	}
	// Each list is kept: asked again, neither asks the addon.
	if _, err := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints); err != nil {
		t.Fatal(err)
	}
	if got := e.subtitleRequests(); len(got) != 2 || got[1] != "" {
		t.Errorf("asked %q, want the hinted list then the title's once", got)
	}
	// The same file is the same subtitle in both lists.
	hinted, _ := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints)
	if subtitles[0].ID != hinted[1].ID {
		t.Errorf("the title's file %v is %v for the video", subtitles[0].ID, hinted[1].ID)
	}
}

// Of the hints, only those known are sent; none asks for the title's
// list, as before.
func TestSubtitleAddonsAreToldOnlyTheHintsKnown(t *testing.T) {
	t.Parallel()
	e := hintedEnv(t)
	for _, hints := range []SubtitleHints{{Filename: "Movie.mkv"}, {Size: 42}, {}} {
		if _, err := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := e.subtitleRequests(), []string{"filename=Movie.mkv", "videoSize=42", ""}; !slices.Equal(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
}

// A refresh of the title forgets the lists given for its videos too: the
// addon is asked again with the same hints.
func TestARefreshForgetsTheSubtitlesGivenForAVideo(t *testing.T) {
	t.Parallel()
	e := hintedEnv(t)
	hints := SubtitleHints{Hash: "0000000000030d43", Size: 200_000}
	for range 2 {
		if _, err := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(e.subtitleRequests()); got != 1 {
		t.Fatalf("%d requests before the refresh, want 1", got)
	}
	if err := e.service.Refresh(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	if n := e.service.subtitleLists.Len(); n != 0 {
		t.Errorf("%d subtitle lists kept after the refresh", n)
	}
	if _, err := e.service.SubtitlesFor(t.Context(), e.member, e.movie, hints); err != nil {
		t.Fatal(err)
	}
	if got := len(e.subtitleRequests()); got != 2 {
		t.Errorf("%d requests after the refresh, want 2", got)
	}
}

// subtitlesAsked reads back the hints a list is kept under.
func TestSubtitleListKeysCarryTheirHints(t *testing.T) {
	t.Parallel()
	hints := SubtitleHints{Filename: "a\x00b.mkv", Size: 7, Hash: "00000000000000ff"}
	key := subtitlesKey(accounts.ID{1}, target{metaType: "movie", id: "tt1"}, hints)
	id, extra := subtitlesAsked(key.id)
	if id != "tt1" || !slices.Equal(extra, hints.extra()) {
		t.Errorf("%q %v, want tt1 %v", id, extra, hints.extra())
	}
	if title := subtitlesKey(accounts.ID{1}, target{metaType: "movie", id: "tt1"}, SubtitleHints{}); title.id != "tt1" || !hintedOf(key, title) || hintedOf(title, title) {
		t.Errorf("the title's key %+v", title)
	}
	if id, extra := subtitlesAsked("tt1"); id != "tt1" || extra != nil {
		t.Errorf("without hints: %q %v", id, extra)
	}
	var want []stremio.ExtraValue
	if got := (SubtitleHints{}).extra(); !slices.Equal(got, want) {
		t.Errorf("no hints: %v", got)
	}
}
