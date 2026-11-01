package library

import (
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An addon refusing a description answers with an error described as a
// title of another identifier and type: it is a failed description, kept
// neither in memory nor in the database, and the title is described once
// the addon answers again.
func TestAnErrorDescriptionIsNotKept(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	const id = "library.item.1"
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"other"},
			Resources: []stremio.Resource{{Name: "meta"}}},
		rawMetas: map[string]string{"other/" + id: `{"id": "addonerror.%7B%7D", "name": "[x] Addon - Error",
			"description": "Rate Limit Exceeded", "type": "movie"}`},
	}
	server := httptest.NewServer(addon)
	t.Cleanup(server.Close)
	installedAddon, err := e.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	entry := installed{addon: installedAddon}
	if meta, err := e.service.meta(t.Context(), entry, "other", id); err == nil {
		t.Errorf("the error described: %+v", meta)
	}
	key := metaKey{installedAddon.ID, "other", id}
	if _, ok := e.service.metas.Get(key); ok {
		t.Error("the error is kept in memory")
	}
	var rows int
	if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM metas WHERE addon_id = $1", installedAddon.ID).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d descriptions kept in the database, %v", rows, err)
	}
	addon.mu.Lock()
	addon.rawMetas["other/"+id] = `{"id": "` + id + `", "name": "Item", "type": "other"}`
	addon.mu.Unlock()
	if meta, err := e.service.meta(t.Context(), entry, "other", id); err != nil || meta.Name != "Item" {
		t.Errorf("described again: %v %+v", err, meta)
	}
}
