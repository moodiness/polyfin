package jellyfin

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// A finished recording tells CanDownload as /Items/{itemId}/Download
// serves it: true to a user allowed to download, in its details and in
// listings that ask for it, false to one who is not; one still being
// written has no file to download yet.
func TestRecordingsTellTheyCanBeDownloaded(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	s, _ := recordingServer(t, "ffprobe-not-installed", folder)
	tokens := map[bool]string{}
	users := map[bool]accounts.User{}
	for allowed, name := range map[bool]string{true: "downloader", false: "keeper"} {
		users[allowed] = s.user(name, func(c *accounts.UserChanges) { c.ContentDownloading = new(allowed) })
		tokens[allowed] = s.signIn(name, "web")
	}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), newTVAddon(t, false, "").url, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", tokens[true], &channels)
	channel, _ := parseGUID(channels.Items[0].Id)
	recording := func(name, status string) string {
		t.Helper()
		var id accounts.ID
		ended := "now()"
		if status == "InProgress" {
			ended = "NULL"
		}
		if err := s.pool.QueryRow(t.Context(), `INSERT INTO live_recordings (channel_id, name, start_at, end_at, started_at, ended_at, status, file)
			VALUES ($1, $2, now() - interval '1 minute', now(), now() - interval '1 minute', `+ended+`, $3, $4) RETURNING id`,
			channel, name, status, name+".mkv").Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, name+".mkv"), []byte(mediaBytes), 0o600); err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(id.String(), "-", "")
	}
	finished, underWay := recording("finished", "Completed"), recording("under-way", "InProgress")

	canDownload := func(dto BaseItemDto) any {
		if dto.CanDownload == nil {
			return nil
		}
		return *dto.CanDownload
	}
	for _, allowed := range []bool{true, false} {
		for id, want := range map[string]bool{finished: allowed, underWay: false} {
			var details BaseItemDto
			s.get(t, "/Users/"+users[allowed].ID.String()+"/Items/"+id, tokens[allowed], &details)
			if canDownload(details) != want {
				t.Errorf("details of %s, allowed %v: can download %v", details.Name, allowed, canDownload(details))
			}
		}
		var listed QueryResult
		s.get(t, "/LiveTv/Recordings?Fields=CanDownload", tokens[allowed], &listed)
		for _, item := range listed.Items {
			if want := item.Id == finished && allowed; canDownload(item) != want {
				t.Errorf("listed %s, allowed %v: can download %v", item.Name, allowed, canDownload(item))
			}
		}
		response, body := s.fetch(t, "/Items/"+finished+"/Download?ApiKey="+tokens[allowed])
		if allowed && (response.StatusCode != http.StatusOK || string(body) != mediaBytes) || !allowed && response.StatusCode != http.StatusForbidden {
			t.Errorf("download, allowed %v: %d %q", allowed, response.StatusCode, body)
		}
	}
}
