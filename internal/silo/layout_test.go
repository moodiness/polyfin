package silo

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// Home's layout names Home's sections, in order, under the identifiers
// and types their own reads answer, which the TV apps mirror on the
// system's home screen; include_items also answers the first ones with
// their cards, cut by item_limit as their reads are.
func TestHomeLayoutNamesHomesSections(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	read := func(path string) []byte {
		t.Helper()
		w := h.do(http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	var home struct {
		Sections []struct {
			ID   string `json:"id"`
			Type string `json:"section_type"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(read("/api/v2/home/sections"), &home); err != nil {
		t.Fatal(err)
	}
	var want []silotest.LayoutEntry
	for _, sec := range home.Sections {
		want = append(want, silotest.LayoutEntry{ID: sec.ID, Type: sec.Type})
	}
	data := read("/api/v2/home/layout")
	rows, err := silotest.CheckHomeLayout(data, 0, 0)
	if err != nil || len(rows) != 2 || !slices.Equal(rows, want) {
		t.Fatalf("layout %v %s, want %v", err, data, want)
	}
	var layout struct {
		Included []json.RawMessage `json:"included_sections"`
	}
	if json.Unmarshal(data, &layout) != nil || layout.Included == nil || len(layout.Included) != 0 {
		t.Errorf("included sections without include_items: %s", data)
	}
	mirrored := 0
	for _, row := range rows {
		if silotest.WatchNextRows(row.Type) {
			mirrored++
			if err := silotest.CheckLayoutRow(read("/api/v2/home/sections/"+row.ID+"/items"), row); err != nil {
				t.Error(err)
			}
		}
	}
	if mirrored != 1 {
		t.Errorf("%d rows mirrored on the home screen, want Continue Watching", mirrored)
	}

	data = read("/api/v2/home/layout?include_items=1&item_limit=1")
	if _, err := silotest.CheckHomeLayout(data, 1, 1); err != nil {
		t.Errorf("layout with its first row: %v %s", err, data)
	}
	var included struct {
		Included []struct {
			ID         string            `json:"id"`
			Items      []json.RawMessage `json:"items"`
			TotalCount int               `json:"total_count"`
			HasMore    bool              `json:"has_more"`
		} `json:"included_sections"`
	}
	if json.Unmarshal(data, &included) != nil || len(included.Included) != 1 || included.Included[0].ID != rows[0].ID ||
		len(included.Included[0].Items) != 1 || included.Included[0].TotalCount != 2 || !included.Included[0].HasMore {
		t.Errorf("included: %s", data)
	}

	var capabilities struct {
		IncludeItemsMax int `json:"include_items_max"`
	}
	if err := json.Unmarshal(read("/api/v2/home/capabilities"), &capabilities); err != nil || capabilities.IncludeItemsMax != 4 {
		t.Errorf("include_items_max %d, %v", capabilities.IncludeItemsMax, err)
	}
	for _, query := range []string{"include_items=5", "include_items=-1", "include_items=two", "item_limit=0"} {
		if w := h.do(http.MethodGet, "/api/v2/home/layout?"+query, nil); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", query, w.Code, w.Body)
		}
	}
}
