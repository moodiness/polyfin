package silotest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LayoutEntry is a Home row as the layout names it: the apps read its id
// and its section_type alone.
type LayoutEntry struct {
	ID, Type string
}

// WatchNextRows reports whether the apps mirror a Home row of a type on
// the system's home screen: Android TV's Watch Next row the Continue
// Watching and Next Up rows (WATCH_NEXT_SECTION_TYPES,
// androidTvApp/.../watchnext/WatchNextHomeSync.kt), Apple TV's Top Shelf
// those it shows (TopShelfRow.isShown, Shared/TopShelfModels.swift).
func WatchNextRows(sectionType string) bool {
	t := strings.ToLower(sectionType)
	return t == "continue_watching" || t == "next_up" || t == "in_progress" || strings.Contains(t, "next")
}

// CheckHomeLayout checks Home's layout (GET /api/v2/home/layout), asked
// with include_items include and item_limit limit, 0 for none, as the
// apps read it, and gives its rows:
//
//   - sections, each row with a non-blank id and section_type: Android
//     decodeHomeLayoutV2 (network/apiv2/HomeSectionsV2Api.kt) fails the
//     whole layout otherwise, and Apple TopShelfHomeLayout.Entry
//     (Shared/TopShelfModels.swift) requires both; an id used once, as
//     each row is then read by it;
//   - included_sections, the first include rows with their cards, in
//     order, each as its own read answers it (checkSection), as the
//     contract's getHomeLayout says.
func CheckHomeLayout(data []byte, include, limit int) ([]LayoutEntry, error) {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	c := &checker{}
	c.has(doc, "layout", map[string]string{"sections": "a", "included_sections": "a"})
	sections, _ := doc["sections"].([]any)
	var rows []LayoutEntry
	seen := map[string]bool{}
	for n, value := range sections {
		entry, _ := value.(object)
		where := fmt.Sprintf("sections[%d]", n)
		c.has(entry, where, map[string]string{"id": "s", "section_type": "s"})
		id, _ := entry["id"].(string)
		kind, _ := entry["section_type"].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(kind) == "" {
			c.fail("%s is blank: id %q, section_type %q", where, id, kind)
		}
		if seen[id] {
			c.fail("%s id %q twice", where, id)
		}
		seen[id] = true
		rows = append(rows, LayoutEntry{id, kind})
	}
	included, _ := doc["included_sections"].([]any)
	if len(included) > include {
		c.fail("%d included sections, asked %d", len(included), include)
	}
	for n, value := range included {
		section, _ := value.(object)
		where := fmt.Sprintf("included_sections[%d]", n)
		checkSection(c, section, where, limit)
		if n < len(rows) && (section["id"] != rows[n].ID || section["section_type"] != rows[n].Type) {
			c.fail("%s is %v %v, the layout's row %d %+v", where, section["id"], section["section_type"], n, rows[n])
		}
	}
	return rows, c.err()
}

// CheckLayoutRow checks the read of a row the layout named
// (GET /api/v2/home/sections/{id}/items) as the apps mirror it on the
// system's home screen: it answers the row asked (CheckSection), of the
// type the layout gave it. Android syncWatchNextHome gives up the whole
// sync on another type, keeping the Watch Next row as it was; Apple
// TopShelfHTTPClient.fetchRow (Shared/TopShelfHTTPClient.swift) leaves
// the row out of Top Shelf.
func CheckLayoutRow(data []byte, row LayoutEntry) error {
	if err := CheckSection(data, row.ID, 0); err != nil {
		return err
	}
	var section struct {
		Type string `json:"section_type"`
	}
	if err := json.Unmarshal(data, &section); err != nil {
		return err
	}
	if section.Type != row.Type {
		return fmt.Errorf("row %q reads as %q, the layout said %q", row.ID, section.Type, row.Type)
	}
	return nil
}
