package admin

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The confirmation of a removal counts what it loses: the home rows on the
// source's catalogs, its libraries, and the users hiding one of them. A
// smart list left with no catalog by the removal is kept, and Health
// tells of it.
func TestRemovingASourceCountsWhatIsLostAndFlagsTheListsEmptied(t *testing.T) {
	t.Parallel()
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { pool = deps.pool })
	administrator := api.signedIn("administrator", true)
	api.signedIn("member", false)
	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}
	id := addon["id"].(string)
	var libraries []map[string]any
	if status := administrator.raw(http.MethodGet, "/scopes/shared/libraries", "", &libraries); status != http.StatusOK || len(libraries) != 1 {
		t.Fatalf("libraries: %d %v", status, libraries)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE users SET hidden_libraries = ARRAY[$1::uuid] WHERE name = 'member'`, libraries[0]["itemId"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO home_rows (position, name, kind, place, addon_id, catalog_type, catalog_id)
		VALUES (1, 'Top', 'catalog', 'top', $1, 'movie', 'top')`, id); err != nil {
		t.Fatal(err)
	}
	status, list, _ := administrator.call(http.MethodPost, "/scopes/shared/smart-lists", map[string]any{
		"name": "Tops", "shownAs": "library", "sort": "name",
		"catalogs": []map[string]any{{"addonId": id, "type": "movie", "id": "top"}},
		"rules":    map[string]any{"field": "group", "match": "all"},
	})
	if status != http.StatusCreated {
		t.Fatalf("the smart list: %d %v", status, list)
	}

	_, loss, _ := administrator.call(http.MethodGet, "/scopes/shared/addons/"+id+"/removal", nil)
	if want := map[string]any{"homeRows": 1.0, "libraries": 1.0, "hiddenLibraries": 1.0, "recordings": 0.0}; !reflect.DeepEqual(loss, want) {
		t.Errorf("what the removal loses: %v", loss)
	}
	if status, _, _ := administrator.call(http.MethodGet, "/scopes/shared/addons/0123456789abcdef0123456789abcdef/removal", nil); status != http.StatusNotFound {
		t.Errorf("an unknown source: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/scopes/shared/addons/"+id, nil); status != http.StatusNoContent {
		t.Fatalf("removing: %d", status)
	}

	var lists []map[string]any
	if status := administrator.raw(http.MethodGet, "/scopes/shared/smart-lists", "", &lists); status != http.StatusOK || len(lists) != 1 ||
		len(lists[0]["catalogs"].([]any)) != 0 {
		t.Errorf("the lists left: %d %v", status, lists)
	}
	var answer struct {
		Problems []healthProblemJSON `json:"problems"`
	}
	if status := administrator.raw(http.MethodGet, "/health/problems?language=en", "", &answer); status != http.StatusOK {
		t.Fatalf("problems: %d", status)
	}
	listID := list["id"].(string)
	want := healthProblemJSON{Key: "smart_list_empty:" + listID, Code: problemSmartListEmpty, Tone: "warning", To: "/libraries/smart/" + listID, Name: "Tops"}
	if len(answer.Problems) != 1 || !reflect.DeepEqual(answer.Problems[0], want) {
		t.Fatalf("problems: %+v", answer.Problems)
	}
	for language, text := range map[string]string{
		"en": "The smart list Tops lists nothing: the catalogs it drew from are gone. Choose other catalogs for it.",
		"fr": "La liste intelligente Tops ne liste rien : les catalogues dont elle tirait ses titres ont disparu. Choisissez-lui d’autres catalogues.",
	} {
		if got := want.text(language); got != text {
			t.Errorf("%s: %q", language, got)
		}
	}
}
