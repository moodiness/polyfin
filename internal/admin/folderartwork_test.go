package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
)

// A folder's titles show the artwork beside their files unless it is
// added or changed with useArtwork off; a change of it alone is one.
func TestFolderUseArtwork(t *testing.T) {
	var folders *localfiles.Service
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		folders = localfiles.New(deps.pool, deps.addons, lib, logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Folders = folders
	})
	admin := api.signedIn("root", true)
	folderOf := func(answer any) (id string, useArtwork bool) {
		t.Helper()
		var addon struct {
			ID     string `json:"id"`
			Folder struct {
				UseArtwork bool `json:"useArtwork"`
			} `json:"folder"`
		}
		encoded, _ := json.Marshal(answer)
		if err := json.Unmarshal(encoded, &addon); err != nil {
			t.Fatal(err)
		}
		return addon.ID, addon.Folder.UseArtwork
	}

	status, answer, _ := admin.call(http.MethodPost, "/scopes/shared/folders", map[string]any{"name": "Movies", "path": t.TempDir(),
		"kind": "movies"})
	if _, on := folderOf(answer); status != http.StatusCreated || !on {
		t.Errorf("added: %d %v", status, answer)
	}
	status, answer, _ = admin.call(http.MethodPost, "/scopes/shared/folders", map[string]any{"name": "Shows", "path": t.TempDir(),
		"kind": "shows", "useArtwork": false})
	id, on := folderOf(answer)
	if status != http.StatusCreated || on {
		t.Errorf("added off: %d %v", status, answer)
	}
	status, answer, _ = admin.call(http.MethodPatch, "/scopes/shared/folders/"+id, map[string]any{"useArtwork": true})
	if _, on := folderOf(answer); status != http.StatusOK || !on {
		t.Errorf("turned on: %d %v", status, answer)
	}
}
