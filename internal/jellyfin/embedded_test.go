package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// subtitledFile is a Matroska file with a French SRT track, an English ASS
// track shown by default, and the font it uses. codecsFile has text tracks
// FFmpeg names otherwise than their codec, a web page and cover art.
var (
	subtitledFile = filepath.Join("..", "container", "testdata", "subtitles.mkv")
	codecsFile    = filepath.Join("..", "playback", "testdata", "codecs.mkv")
)

// subtitledAddon serves one movie whose one stream is file.
func subtitledAddon(t *testing.T, file string) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3000", Type: "movie", Name: "Subtitled", Runtime: "12s", Year: "2024"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "subtitled", Name: "Subtitled", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source", URL: server.URL + "/files/movie.mkv", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv"}}}})
		case path == "/files/movie.mkv":
			http.ServeFile(w, r, file)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// playingFile installs an addon whose one movie has file as its one
// version, analyzed as ffprobe saw it in the .ffprobe.json beside it, and
// signs a member in from the device tv.
func playingFile(t *testing.T, file string) (s testServer, user accounts.User, token string, movie accounts.ID) {
	t.Helper()
	s = newTestServer(t, 10)
	user = s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), subtitledAddon(t, file), false); err != nil {
		t.Fatal(err)
	}
	token = s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie, _ = accounts.ParseID(page.Items[0].Id)
	versions, err := s.library.Versions(t.Context(), user, movie)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	probe, err := os.ReadFile(strings.TrimSuffix(file, ".mkv") + ".ffprobe.json")
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	return s, user, token, movie
}

// TestBrowsersGetTheTracksInsideFilesWithTheirFonts plays the file as
// jellyfin-web does: its tracks go out as files, read whole through the
// file's index, the ASS one as its script, with the font it uses.
func TestBrowsersGetTheTracksInsideFilesWithTheirFonts(t *testing.T) {
	s, user, token, id := playingFile(t, subtitledFile)
	movie := id.String()

	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token),
		map[string]any{"UserId": user.ID.String(), "DeviceProfile": json.RawMessage(profile)})
	var response playbackInfoResponse
	if err := json.Unmarshal(body, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	source := response.MediaSources[0]
	tracks := map[int]playback.MediaStream{}
	for _, stream := range source.MediaStreams {
		tracks[stream.Index] = stream
	}
	srt, ass := tracks[1], tracks[2]
	if srt.DeliveryMethod != "External" || !strings.Contains(srt.DeliveryUrl, "/Subtitles/1/0/Stream.vtt?ApiKey=") {
		t.Errorf("SRT track: %s %s", srt.DeliveryMethod, srt.DeliveryUrl)
	}
	if ass.DeliveryMethod != "External" || !strings.Contains(ass.DeliveryUrl, "/Subtitles/2/0/Stream.ass?ApiKey=") {
		t.Errorf("ASS track: %s %s", ass.DeliveryMethod, ass.DeliveryUrl)
	}
	if source.DefaultSubtitleStreamIndex == nil || *source.DefaultSubtitleStreamIndex != 2 {
		t.Errorf("default subtitle: %v", source.DefaultSubtitleStreamIndex)
	}
	wantFont := MediaAttachment{Codec: "ttf", Index: 3, FileName: "Dummy.ttf", MimeType: "application/x-truetype-font",
		DeliveryUrl: "/Videos/" + hyphenated(id) + "/" + source.Id + "/Attachments/3?ApiKey=" + token}
	if len(source.MediaAttachments) != 1 || source.MediaAttachments[0] != wantFont {
		t.Errorf("attachments: %+v\nwant %+v", source.MediaAttachments, wantFont)
	}

	// jellyfin-web downloads a text track as JSON, by swapping .vtt for .js.
	status, _, events := fetchText(t, s.url+strings.Replace(srt.DeliveryUrl, ".vtt", ".js", 1))
	original, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "subtitles.srt"))
	cues, _ := subtitles.Parse(original)
	if status != http.StatusOK || events != string(subtitles.TrackEvents(cues)) {
		t.Errorf("SRT track as JSON: %d %s", status, events)
	}
	// The ASS track comes as its script, which libass renders.
	status, header, script := fetchText(t, s.url+ass.DeliveryUrl)
	want, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "subtitles.ass"))
	wantScript, _ := subtitles.ParseScript(want)
	if status != http.StatusOK || header.Get("Content-Type") != "text/x-ssa" || script != string(wantScript.Bytes()) {
		t.Errorf("ASS track: %d %s\n%s\nwant\n%s", status, header.Get("Content-Type"), script, wantScript.Bytes())
	}
	// Apps that cannot render ASS get its text.
	if status, _, text := fetchText(t, s.url+strings.Replace(ass.DeliveryUrl, ".ass", ".vtt", 1)); status != http.StatusOK || strings.Contains(text, `{\`) {
		t.Errorf("ASS track as WebVTT: %d %s", status, text)
	}
	// The font, signed for the caller like the tracks.
	dummy, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "Dummy.ttf"))
	status, header, font := fetchText(t, s.url+wantFont.DeliveryUrl)
	if status != http.StatusOK || header.Get("Content-Type") != "application/x-truetype-font" || font != string(dummy) {
		t.Errorf("font: %d %s %d bytes", status, header.Get("Content-Type"), len(font))
	}
	unsigned, _, _ := strings.Cut(wantFont.DeliveryUrl, "?")
	if status, _, _ := fetchText(t, s.url+unsigned); status != http.StatusUnauthorized {
		t.Errorf("font without a token: %d", status)
	}

	answers := recordedAnswers(t)
	for name, path := range map[string]string{
		"Attachment wrong index":        strings.Replace(wantFont.DeliveryUrl, "/Attachments/3?", "/Attachments/99?", 1),
		"Attachment wrong media source": strings.Replace(wantFont.DeliveryUrl, source.Id, strings.Repeat("ab", 16), 1),
	} {
		recorded := answers[name]
		status, _, body := fetchText(t, s.url+path)
		var message string
		_ = json.Unmarshal([]byte(body), &message)
		var wantMessage string
		_ = json.Unmarshal(recorded.Body, &wantMessage)
		// The messages name the media source asked for.
		replace := func(text string) string {
			return strings.Fields(text)[0] + " " + strings.Join(strings.Fields(text)[2:], " ")
		}
		if status != recorded.Status || replace(message) != replace(wantMessage) {
			t.Errorf("%s: %d %q, want %d %q", name, status, message, recorded.Status, wantMessage)
		}
	}
	if status, _, body := fetchText(t, s.url+strings.Replace(wantFont.DeliveryUrl, hyphenated(id), hyphenated(accounts.ID{9}), 1)); status != http.StatusNotFound || !strings.Contains(body, `"title":"Not Found"`) {
		t.Errorf("wrong item: %d %s", status, body)
	}
}

// jellyfin-web loads fonts from the attachment route. Anything else a
// file carries goes out as a download, sandboxed, so that a page in a file
// cannot run as Polyfin's; cover art is not extracted, as from Jellyfin.
func TestAttachedFilesOtherThanFontsAreDownloads(t *testing.T) {
	s, user, token, id := playingFile(t, codecsFile)
	status, body := s.call(http.MethodPost, "/Items/"+id.String()+"/PlaybackInfo", app("tv", token), map[string]any{"UserId": user.ID.String()})
	var response playbackInfoResponse
	if err := json.Unmarshal(body, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	urls := map[string]string{}
	for _, attachment := range response.MediaSources[0].MediaAttachments {
		urls[attachment.FileName] = attachment.DeliveryUrl
	}

	page, _ := os.ReadFile(filepath.Join("..", "playback", "testdata", "page.html"))
	status, header, served := fetchText(t, s.url+urls["page.html"])
	if status != http.StatusOK || served != string(page) || header.Get("Content-Type") != "application/octet-stream" ||
		header.Get("Content-Disposition") != "attachment" || !strings.HasPrefix(header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("web page: %d %v", status, header)
	}

	recorded := recordedAnswers(t)["Attachment cover.jpg signed"]
	status, _, served = fetchText(t, s.url+urls["cover.jpg"])
	var message string
	_ = json.Unmarshal([]byte(served), &message)
	if status != recorded.Status || !strings.HasPrefix(message, "Attachment with stream index 5 can't be extracted for MediaSource ") {
		t.Errorf("cover art: %d %q, want %d", status, message, recorded.Status)
	}
}
