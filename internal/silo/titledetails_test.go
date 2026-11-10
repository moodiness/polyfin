package silo

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// A title's detail gives its first tagline, the one from TMDB, and its
// card its studios, in order, as the Jellyfin API gives them.
func TestADetailGivesTheTaglineAndStudios(t *testing.T) {
	t.Parallel()
	id := shelfID(1)
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		switch {
		case c.path == "/Items/"+id && strings.Contains(c.query.Get("Fields"), "Taglines") && strings.Contains(c.query.Get("Fields"), "Studios"):
			jsonAnswer(w, map[string]any{"Id": id, "Name": "Tidewater", "Type": "Series",
				"Taglines": []string{"Every tide turns.", "87% for you"},
				"Studios":  []any{map[string]any{"Name": "Coast TV", "Id": "a"}, map[string]any{"Name": "Harbor Works", "Id": "b"}}})
		default:
			jsonAnswer(w, map[string]any{"Items": []any{}, "TotalRecordCount": 0})
		}
	})
	w := h.do(http.MethodGet, "/api/v2/catalog/items/"+id, nil)
	var detail struct {
		Tagline string   `json:"tagline"`
		Studios []string `json:"studios"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil {
		t.Fatalf("detail: %d %s", w.Code, w.Body)
	}
	if detail.Tagline != "Every tide turns." || !slices.Equal(detail.Studios, []string{"Coast TV", "Harbor Works"}) {
		t.Errorf("detail: %s", w.Body)
	}
}
