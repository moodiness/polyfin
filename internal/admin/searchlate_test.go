package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// An addon whose search catalog the searches leave out, as it did not
// answer a search in time, is a warning of System › Health that comes and
// goes with the searches: it is not told to notifications.
func TestHealthTellsASearchLeftOut(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": "late", "name": "Late", "version": "1.0.0", "resources": ["catalog", "meta"],
			"catalogs": [{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]}]}`)
	}))
	t.Cleanup(server.Close)
	var late atomic.Value
	late.Store(accounts.ID{})
	var options *Options
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Health = HealthSources{SearchesLeaveOut: func(id accounts.ID) bool { return id == late.Load().(accounts.ID) }}
		options, store = o, deps.addons
	})
	addon, err := store.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	administrator := api.signedIn("administrator", true)
	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		return found
	}
	if found := problems(); len(found) != 0 {
		t.Fatalf("problems while searches ask every addon: %v", found)
	}

	late.Store(addon.ID)
	if found := problems(); len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "search_late:" + addon.ID.String(),
		"code": "search_late", "tone": "warning", "to": "/system/health#addons", "transient": true, "name": "Late"}) {
		t.Fatalf("problems: %v", found)
	}
	if told, err := HealthProblems(*options)(t.Context()); err != nil || len(told) != 0 {
		t.Errorf("notifications: %+v %v", told, err)
	}
}
