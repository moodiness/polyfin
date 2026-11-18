package silo

import (
	"net/http"
	"strings"
	"testing"
)

// A request that fails is logged with its route, as the Jellyfin API and
// the admin app log theirs: the pattern, never the path the app asked.
func TestAFailedRequestIsLoggedByItsRoute(t *testing.T) {
	t.Parallel()
	id := shelfID(1)
	h := newHarness(t, func(w http.ResponseWriter, _ call) {
		jsonAnswer(w, map[string]any{"Id": id, "Name": "Tidewater", "Type": "Movie", "MediaSources": []any{map[string]any{"Id": testVersion}}})
	})
	// PostgreSQL is away: the versions' numbers cannot be read.
	h.server.options.DB.Close()
	if w := h.do(http.MethodGet, "/api/v2/catalog/items/"+id, nil); w.Code != http.StatusInternalServerError {
		t.Fatalf("the title without a database: %d %s", w.Code, w.Body)
	}
	if log := h.log.String(); !strings.Contains(log, `msg="A request of Silo's apps failed" method=GET path=/api/v2/catalog/items/{id} error=`) ||
		strings.Contains(log, id) {
		t.Errorf("log: %s", log)
	}
}
