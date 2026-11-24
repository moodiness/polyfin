package jellyfin

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// tonePlaylist and toneManifest are the HLS playlist and the DASH manifest
// fakeEclipse gives as the streams of the tracks its manifests name.
const (
	tonePlaylist = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n#EXTINF:10,\nsegment0.aac\n#EXT-X-ENDLIST\n"
	toneManifest = `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT10S">` +
		`<Period><AdaptationSet mimeType="audio/mp4"><Representation id="0" codecs="mp4a.40.2" bandwidth="128000">` +
		`<BaseURL>tone.m4a</BaseURL></Representation></AdaptationSet></Period></MPD>`
)

// Tracks download with the user's Can download permission, as Jellyfin's
// audio does, and tell it in CanDownload: from the permission alone in a
// listing that knows no stream of the track yet, and once the stream is
// known, only when /Items/{itemId}/Download serves it as one file. A
// stream that is an HLS playlist or a DASH manifest is no such file.
func TestTracksDownloadWithThePermission(t *testing.T) {
	t.Parallel()
	// Binary, as media is: text is taken for a playlist.
	tone := "fLaC\x00\x00\x00\x22\x10\x00\x10\x00"
	flac := filepath.Join(t.TempDir(), "tone.flac")
	if err := os.WriteFile(flac, []byte(tone), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, flac)
	addon.manifests = map[string]string{"t2": "hls", "t3": "dash"}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false); err != nil {
		t.Fatal(err)
	}
	permitted := map[bool]accounts.User{
		true:  s.user("downloader", func(c *accounts.UserChanges) { c.ContentDownloading = new(true) }),
		false: s.user("listener", func(c *accounts.UserChanges) { c.ContentDownloading = new(false) }),
	}
	tokens := map[bool]string{true: s.signIn("downloader", "web"), false: s.signIn("listener", "web")}
	var views QueryResult
	s.get(t, "/UserViews", tokens[true], &views)
	var albums string
	for _, view := range views.Items {
		if view.Name == "New Releases" {
			albums = view.Id
		}
	}
	listed := func(allowed bool) map[string]BaseItemDto {
		t.Helper()
		var page QueryResult
		s.get(t, "/Items?ParentId="+albums+"&IncludeItemTypes=Audio&Recursive=true&Fields=CanDownload", tokens[allowed], &page)
		songs := map[string]BaseItemDto{}
		for _, song := range page.Items {
			songs[song.Name] = song
		}
		if len(songs) != 3 {
			t.Fatalf("songs: %+v", page.Items)
		}
		return songs
	}
	canDownload := func(dto BaseItemDto) any {
		if dto.CanDownload == nil {
			return nil
		}
		return *dto.CanDownload
	}
	details := func(allowed bool, id string) BaseItemDto {
		t.Helper()
		var dto BaseItemDto
		s.get(t, "/Users/"+permitted[allowed].ID.String()+"/Items/"+id, tokens[allowed], &dto)
		return dto
	}
	download := func(allowed bool, id string) (int, string) {
		t.Helper()
		response, body := s.fetch(t, "/Items/"+id+"/Download?ApiKey="+tokens[allowed])
		return response.StatusCode, string(body)
	}

	// No stream is known yet: the permission alone tells.
	before := addon.streams.Load()
	for _, allowed := range []bool{true, false} {
		for name, song := range listed(allowed) {
			if canDownload(song) != allowed {
				t.Errorf("listed %s, allowed %v: can download %v", name, allowed, canDownload(song))
			}
		}
	}
	if asked := addon.streams.Load() - before; asked != 0 {
		t.Errorf("a listing asked the addon for %d streams", asked)
	}

	// A file downloads, for the user allowed to; its details and the
	// listings that know it now agree.
	file := listed(true)["First Light"].Id
	for _, allowed := range []bool{true, false} {
		if dto := details(allowed, file); canDownload(dto) != allowed {
			t.Errorf("details, allowed %v: can download %v", allowed, canDownload(dto))
		}
		status, body := download(allowed, file)
		if allowed && (status != http.StatusOK || body != tone) || !allowed && status != http.StatusForbidden {
			t.Errorf("download, allowed %v: %d %q", allowed, status, body)
		}
		if song := listed(allowed)["First Light"]; canDownload(song) != allowed {
			t.Errorf("listed once known, allowed %v: can download %v", allowed, canDownload(song))
		}
	}

	// A playlist or a manifest is not one file: refused as a DVD is,
	// whether the app is kept from the source or sent to it, and
	// CanDownload false once it is known.
	for name, manifest := range map[string]string{"Second Wind": tonePlaylist, "Rude Words": toneManifest} {
		id := listed(true)[name].Id
		if dto := details(true, id); canDownload(dto) != false {
			t.Errorf("details of %s: can download %v", name, canDownload(dto))
		}
		if status, body := download(true, id); status != http.StatusBadRequest || body == manifest {
			t.Errorf("download of %s, kept from the source: %d %q", name, status, body)
		}
		for _, allowed := range []bool{true, false} {
			if song := listed(allowed)[name]; canDownload(song) != false {
				t.Errorf("listed %s once known, allowed %v: can download %v", name, allowed, canDownload(song))
			}
		}
	}
	s.setting(t, func(settings *accounts.Settings) { settings.SendToSource = accounts.SendToSourceEveryone })
	for name, manifest := range map[string]string{"Second Wind": tonePlaylist, "Rude Words": toneManifest} {
		if status, body := download(true, listed(true)[name].Id); status != http.StatusBadRequest || body == manifest {
			t.Errorf("download of %s, sent to the source: %d %q", name, status, body)
		}
	}
}
