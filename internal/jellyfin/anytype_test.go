package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// sportsAddon is a fake addon of a type of its own, sport, as Stremio apps
// show such types: catalogs Live Now, Today and Football; a match on now,
// whose stream is an HLS playlist that does not end, and a recap whose
// details list one video, a file.
func sportsAddon(t *testing.T, liveDir, recap string) string {
	t.Helper()
	var server *httptest.Server
	match := stremio.Meta{ID: "live:match1", Type: "sport", Name: "Harbour City v Northfield", Description: "Live"}
	cup := func() stremio.Meta {
		return stremio.Meta{ID: "recap:cup1", Type: "sport", Name: "Cup final recap", Videos: []stremio.Video{{ID: "recap:cup1:1", Title: "Highlights",
			Released: "2026-10-01T20:00:00.000Z", Streams: []stremio.Stream{{Name: "Sports", URL: server.URL + "/files/recap.mp4"}}}}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		answer := func(value any) { _ = json.NewEncoder(w).Encode(value) }
		switch {
		case path == "/manifest.json":
			answer(stremio.Manifest{ID: "org.example.sports", Name: "Sports", Version: "1", Types: []string{"sport"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}}, IDPrefixes: []string{"live:", "recap:"},
				Catalogs: []stremio.Catalog{{Type: "sport", ID: "live", Name: "Live Now"}, {Type: "sport", ID: "today", Name: "Today"},
					{Type: "sport", ID: "football", Name: "Football"}}})
		case path == "/catalog/sport/live.json", path == "/catalog/sport/today.json":
			answer(map[string]any{"metas": []stremio.Meta{match}})
		case path == "/catalog/sport/football.json":
			answer(map[string]any{"metas": []stremio.Meta{{ID: "recap:cup1", Type: "sport", Name: "Cup final recap"}}})
		case path == "/meta/sport/live%3Amatch1.json":
			answer(map[string]any{"meta": match})
		case path == "/meta/sport/recap%3Acup1.json":
			answer(map[string]any{"meta": cup()})
		case path == "/stream/sport/live%3Amatch1.json":
			answer(map[string]any{"streams": []stremio.Stream{{Name: "Sports", Title: "Live", URL: server.URL + "/live/match.m3u8"}}})
		case strings.HasPrefix(path, "/stream/"):
			answer(map[string]any{"streams": []stremio.Stream{}})
		case strings.HasPrefix(path, "/live/"):
			http.ServeFile(w, r, filepath.Join(liveDir, strings.TrimPrefix(path, "/live/")))
		case path == "/files/recap.mp4":
			http.ServeFile(w, r, recap)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Catalogs of a type Polyfin has no use of its own for, such as sport,
// are libraries of videos once enabled, as other catalogs are: a title is
// a video, or a folder when its details list videos. A video whose stream
// has no end plays live, as a channel does: an infinite stream without a
// duration, converted as it comes, which keeps no resume point and is not
// marked played. A finished file plays as a video.
func TestCatalogsOfAnyTypeListVideosAndLiveOnesPlayLive(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	liveDir := t.TempDir()
	// A live playlist, written as it plays: no #EXT-X-ENDLIST, and its
	// oldest segments deleted as new ones come.
	ctx, stop := context.WithCancel(context.Background())
	streaming := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine", "-c:v", "libx264", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "4", "-hls_flags", "delete_segments", filepath.Join(liveDir, "match.m3u8"))
	if err := streaming.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(); _ = streaming.Wait() })
	eventually(t, "the live playlist", func() bool {
		data, _ := os.ReadFile(filepath.Join(liveDir, "match.m3u8"))
		return strings.Count(string(data), "#EXTINF") >= 2
	})
	recap := filepath.Join(t.TempDir(), "recap.mp4")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=20:size=64x48:rate=4",
		"-c:v", "mpeg4", recap).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), sportsAddon(t, liveDir, recap), false)
	if err != nil {
		t.Fatal(err)
	}
	tr := tracking{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String()}
	views := func() map[string]BaseItemDto {
		var result QueryResult
		s.get(t, "/UserViews", tr.token, &result)
		named := map[string]BaseItemDto{}
		for _, view := range result.Items {
			named[view.Name] = view
		}
		return named
	}
	// Added, the addon enables none of its catalogs.
	if named := views(); len(named) != 0 {
		t.Errorf("libraries after adding the addon: %v", named)
	}
	var choices []addons.LibraryChoice
	for _, catalog := range addon.Manifest.Catalogs {
		choices = append(choices, addons.LibraryChoice{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID})
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
	named := views()
	for _, name := range []string{"Live Now", "Today", "Football"} {
		if view, ok := named[name]; !ok || view.CollectionType != "homevideos" {
			t.Errorf("the library %s: %+v", name, view)
		}
	}

	// The recap is a folder of its video, the match a video.
	var football, live, recapVideos QueryResult
	s.get(t, "/Items?ParentId="+named["Football"].Id+"&Recursive=false&IncludeItemTypes=Folder,Video", tr.token, &football)
	if len(football.Items) != 1 || football.Items[0].Type != "Folder" || football.Items[0].Name != "Cup final recap" {
		t.Fatalf("the Football library: %+v", football.Items)
	}
	s.get(t, "/Items?ParentId="+football.Items[0].Id, tr.token, &recapVideos)
	if len(recapVideos.Items) != 1 || recapVideos.Items[0].Type != "Video" || recapVideos.Items[0].Name != "Highlights" {
		t.Fatalf("the recap's videos: %+v", recapVideos.Items)
	}
	s.get(t, "/Items?ParentId="+named["Live Now"].Id+"&Recursive=false&IncludeItemTypes=Folder,Video", tr.token, &live)
	if len(live.Items) != 1 || live.Items[0].Type != "Video" || live.Items[0].Name != "Harbour City v Northfield" {
		t.Fatalf("the Live Now library: %+v", live.Items)
	}
	match := live.Items[0].Id

	// Its details opened prepare its play ahead, as a channel's: a file's
	// analysis would wait for an end that never comes, then fail it.
	s.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	if status := s.get(t, "/Users/"+tr.user+"/Items/"+match, tr.token, nil); status != http.StatusOK {
		t.Fatalf("the match's details: %d", status)
	}
	matchID, _ := accounts.ParseID(match)
	eventually(t, "the match prepared", func() bool {
		versions, _ := s.library.KnownVersions(t.Context(), member, matchID)
		if len(versions) == 0 {
			return false
		}
		analysis, ok := s.handler.Playback.Analyzed(t.Context(), versions[0].ID)
		return ok && playback.Endless(analysis)
	})
	s.settle(t)

	// The recap's video plays as a file: its duration is known.
	if answer := s.playbackInfo(t, "tv", tr.token, recapVideos.Items[0].Id, nil); len(answer.MediaSources) != 1 ||
		answer.MediaSources[0].IsInfiniteStream || answer.MediaSources[0].RunTimeTicks == nil {
		t.Errorf("the recap's video: %+v", answer)
	}

	// The match plays live: an infinite stream without a duration, which
	// Swiftfin's native player takes converted into HLS segments as they
	// come.
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "swiftfin-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	answer := s.playbackInfo(t, "tv", tr.token, match, map[string]any{"DeviceProfile": json.RawMessage(profile), "IsPlayback": true})
	if len(answer.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo of the match: %+v", answer)
	}
	source := answer.MediaSources[0]
	if !source.IsInfiniteStream || source.RunTimeTicks != nil || source.TranscodingUrl == "" {
		t.Fatalf("the match: %+v", source)
	}
	get := func(path string) (int, string) {
		status, body := s.call(http.MethodGet, path, "", nil)
		return status, string(body)
	}
	status, master := get(source.TranscodingUrl)
	if status != http.StatusOK || !strings.Contains(master, "live.m3u8?") {
		t.Fatalf("the match's master playlist: %d %s", status, master)
	}
	base := source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "master.m3u8")]
	query := source.TranscodingUrl[strings.Index(source.TranscodingUrl, "?"):]
	status, media := get(base + "live.m3u8" + query)
	if status != http.StatusOK || strings.Contains(media, "#EXT-X-ENDLIST") || !strings.Contains(media, "#EXTINF") {
		t.Fatalf("the match's live playlist: %d %s", status, media)
	}
	segments := slices.DeleteFunc(strings.Split(media, "\n"), func(line string) bool { return !strings.HasPrefix(line, "hls/live/") })
	if len(segments) == 0 {
		t.Fatalf("no segment: %s", media)
	}
	if status, segment := get(base + segments[0]); status != http.StatusOK || len(segment) == 0 {
		t.Errorf("a segment of the match: %d", status)
	}

	// A position reported for the match keeps no resume point, and does
	// not mark it played.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": match, "MediaSourceId": source.Id, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": match, "MediaSourceId": source.Id, "PositionTicks": 1_800_000_000})
	if data := tr.userData(t, match); data.Played || data.PlaybackPositionTicks != 0 || data.PlayCount != 0 {
		t.Errorf("the match's user data after it played: %+v", data)
	}
}
