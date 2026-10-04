package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// lineupProvider serves a playlist whose first channel comes in three
// qualities, the best of which is broken, then two plain channels.
func lineupProvider(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get.php":
			_, _ = fmt.Fprintf(w, "#EXTM3U\n"+
				"#EXTINF:-1 tvg-chno=\"20\" tvg-logo=\"%[1]s/zeb.png\" group-title=\"News\",FR: Zeb One SD\n%[1]s/live/zeb-sd.m3u8\n"+
				"#EXTINF:-1 tvg-chno=\"21\" group-title=\"News\",FR: Zeb One FHD\n%[1]s/live/zeb-fhd.m3u8\n"+
				"#EXTINF:-1 tvg-chno=\"22\" group-title=\"News\",FR: Zeb One HD\n%[1]s/live/zeb-hd.m3u8\n"+
				"#EXTINF:-1 tvg-chno=\"30\" group-title=\"Kids\",Orbe Junior\n%[1]s/live/orbe.m3u8\n"+
				"#EXTINF:-1 tvg-chno=\"40\" group-title=\"Docs\",Quill Docs\n%[1]s/live/quill.m3u8\n"+
				"#EXTINF:-1 tvg-chno=\"41\" tvg-logo=\"%[1]s/lumo.png\" group-title=\"Docs\",Lumo Docs\n%[1]s/live/lumo.m3u8\n", server.URL)
		case "/live/zeb-fhd.m3u8":
			http.Error(w, "gone", http.StatusNotFound)
		case "/zeb.png", "/lumo.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		default:
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:1\n#EXTINF:2,\nseg1.ts\n")
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/get.php?username=user&password=secret"
}

// analyze records an analysis of a channel stream, of the height given.
func analyze(t *testing.T, s testServer, version library.Version, height int) {
	t.Helper()
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Format, analysis.Duration, analysis.Remote = "hls", 0, true
	for i := range analysis.Streams {
		if analysis.Streams[i].Type == "video" {
			analysis.Streams[i].Height, analysis.Streams[i].Width = height, height*16/9
		}
	}
	data, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
}

// A line-up shows in apps as the administrator arranged it: only enabled
// channels of enabled categories, in category then channel order, with
// their fixed or provider numbers and their categories as genres. A merged
// channel offers each enabled stream as a version, best first; playing it
// falls back past a broken stream, and a quality group picks the stream
// that fits.
func TestLineupsShowAndPlayAsArranged(t *testing.T) {
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	merged := iptv.ChannelsMerged
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider",
		Account: iptv.Account{Kind: addons.KindM3U, URL: lineupProvider(t)}, Options: &iptv.OptionsPatch{Channels: &merged}}, false)
	if err != nil {
		t.Fatal(err)
	}
	categories, err := s.iptv.Categories(t.Context(), addons.Shared(), addon.ID, "")
	if err != nil || len(categories) != 3 {
		t.Fatalf("categories: %+v %v", categories, err)
	}
	news, kids, docs := categories[0], categories[1], categories[2]
	// Kids first, Docs off, News renamed; Quill moved into News, numbered.
	if err := s.iptv.OrderCategories(t.Context(), addons.Shared(), addon.ID, []accounts.ID{kids.ID, news.ID, docs.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.iptv.UpdateCategory(t.Context(), addons.Shared(), addon.ID, docs.ID, iptv.CategoryChanges{Enabled: new(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.iptv.UpdateCategory(t.Context(), addons.Shared(), addon.ID, news.ID, iptv.CategoryChanges{Name: &iptv.Nullable[string]{Value: new("Headlines")}}); err != nil {
		t.Fatal(err)
	}
	_, lineup, err := s.iptv.ListChannels(t.Context(), addons.Shared(), addon.ID, iptv.ChannelFilter{})
	if err != nil || len(lineup) != 4 {
		t.Fatalf("line-up: %+v %v", lineup, err)
	}
	zeb, quill, lumo := lineup[1], lineup[2], lineup[3]
	if _, err := s.iptv.UpdateChannel(t.Context(), addons.Shared(), addon.ID, quill.ID, iptv.ChannelChanges{
		Category: &iptv.Nullable[accounts.ID]{Value: &news.ID}, Number: &iptv.Nullable[int]{Value: new(7)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.iptv.MoveChannel(t.Context(), addons.Shared(), addon.ID, quill.ID, &zeb.ID); err != nil {
		t.Fatal(err)
	}

	var channels QueryResult
	s.get(t, "/LiveTv/Channels?fields=Genres", token, &channels)
	var got []string
	for _, c := range channels.Items {
		got = append(got, fmt.Sprintf("%s #%s %v", c.Name, c.Number, *c.Genres))
	}
	want := []string{"Orbe Junior #30 [Kids]", "Quill Docs #7 [Headlines]", "Zeb One #20 [Headlines]"}
	if !slices.Equal(got, want) {
		t.Errorf("channels %q, want %q", got, want)
	}
	if channels.Items[2].Id != zeb.ID.String() {
		t.Errorf("a channel's item is its line-up id: %s, want %s", channels.Items[2].Id, zeb.ID)
	}

	// The merged channel's details list its streams, best first.
	var details BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+zeb.ID.String(), token, &details)
	if details.MediaSources == nil {
		t.Fatal("no media sources")
	}
	var names []string
	for _, source := range *details.MediaSources {
		names = append(names, source.Name)
	}
	if len(names) != 3 || (*details.MediaSources)[0].Id != zeb.ID.String() {
		t.Errorf("versions of the merged channel: %q %+v", names, *details.MediaSources)
	}
	versions, err := s.library.Versions(t.Context(), member, zeb.ID)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if versions[0].Height != 1080 || versions[1].Height != 720 || versions[2].Height != 576 && versions[2].Height != 480 {
		t.Errorf("versions best first: %d %d %d", versions[0].Height, versions[1].Height, versions[2].Height)
	}

	// The best stream is broken: playback falls back to the next one.
	analyze(t, s, versions[1], 720)
	analyze(t, s, versions[2], 480)
	source := livePlaybackInfo(t, s, token, zeb.ID.String(), "jellyfin-web-chrome")
	if source.ETag != versions[1].ID.String() || source.Id != versions[1].ID.String() {
		t.Errorf("played %s, want the second stream %s", source.ETag, versions[1].ID)
	}

	// A user of the SD quality group plays the stream that fits.
	height := 480
	s.user("small", func(c *accounts.UserChanges) { c.QualityGroup = &height })
	small := s.signIn("small", "tv")
	if source := livePlaybackInfo(t, s, small, zeb.ID.String(), "jellyfin-web-chrome"); source.ETag != versions[2].ID.String() {
		t.Errorf("the SD group played %s, want %s", source.ETag, versions[2].ID)
	}

	// A disabled stream is no longer offered; a disabled channel no longer
	// listed, nor played.
	settings := []iptv.StreamSetting{}
	full, _ := s.iptv.Channel(t.Context(), addons.Shared(), addon.ID, zeb.ID)
	for i, stream := range full.Streams {
		settings = append(settings, iptv.StreamSetting{ID: stream.ID, Enabled: i != 1})
	}
	if _, err := s.iptv.SetStreams(t.Context(), addons.Shared(), addon.ID, zeb.ID, settings); err != nil {
		t.Fatal(err)
	}
	if versions, _ := s.library.Versions(t.Context(), member, zeb.ID); len(versions) != 2 {
		t.Errorf("versions once one is disabled: %d", len(versions))
	}
	if _, err := s.iptv.UpdateChannel(t.Context(), addons.Shared(), addon.ID, zeb.ID, iptv.ChannelChanges{Enabled: new(false)}); err != nil {
		t.Fatal(err)
	}
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Errorf("channels once one is disabled: %d", len(channels.Items))
	}
	if status, _ := s.call(http.MethodPost, "/Items/"+zeb.ID.String()+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a disabled channel: %d", status)
	}
	// The logos of channels apps do not list, listed before or never,
	// still show in the admin app.
	for _, id := range []accounts.ID{zeb.ID, lumo.ID} {
		response, err := http.Get(s.url + "/Items/" + id.String() + "/Images/Primary")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" {
			t.Errorf("logo of a hidden channel: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
		}
	}
}
