package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

func TestRootFolderCountsAndSuggestionsAnswerAsJellyfin(t *testing.T) {
	e := newEdited(t)
	var views QueryResult
	e.get(t, "/UserViews", e.member, &views)
	for _, path := range []string{"/Items/Root", "/Users/" + e.memberID + "/Items/Root", "/Items/" + rootFolderID.String()} {
		status, body := e.call(http.MethodGet, path, app("tv", e.member), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		// Polyfin's root folder has no path on disk.
		matchesFixture(t, "root-folder", body, shapeRules{absent: map[string][]string{"Path": {"root-folder"}, "ChannelId": {"root-folder"},
			"ParentId": {"root-folder"}}, returned: map[string][]string{"CollectionType": {"root-folder"}}})
		var root BaseItemDto
		_ = json.Unmarshal(body, &root)
		if root.Type != "UserRootFolder" || root.Name != "Media Folders" || root.Id != rootFolderID.String() || *root.ChildCount != len(views.Items) {
			t.Errorf("%s: %s", path, body)
		}
	}
	var children QueryResult
	if status := e.get(t, "/Items?ParentId="+rootFolderID.String(), e.member, &children); status != http.StatusOK ||
		!slices.Equal(itemNames(children.Items), itemNames(views.Items)) {
		t.Errorf("root's children: %d %v, views %v", status, itemNames(children.Items), itemNames(views.Items))
	}

	status, body := e.call(http.MethodGet, "/Items/Counts", app("tv", e.member), nil)
	if status != http.StatusOK {
		t.Fatalf("counts: %d %s", status, body)
	}
	matchesFixture(t, "item-counts", body, shapeRules{})
	var counts ItemCounts
	_ = json.Unmarshal(body, &counts)
	if counts.MovieCount != 3 || counts.SeriesCount != 1 || counts.ItemCount != 4 {
		t.Errorf("counts: %s", body)
	}
	// The child does not count what they may not see.
	e.get(t, "/Items/Counts?userId="+e.childID, e.child, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 0 {
		t.Errorf("child's counts: %+v", counts)
	}
	if status, _ := e.call(http.MethodPost, "/Users/"+e.memberID+"/FavoriteItems/"+e.ids["Allowed"], app("tv", e.member), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	e.get(t, "/Items/Counts?isFavorite=true", e.member, &counts)
	if counts.MovieCount != 1 || counts.ItemCount != 1 {
		t.Errorf("favorite counts: %+v", counts)
	}
	e.get(t, "/Items/Counts?isFavorite=false", e.member, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 1 {
		t.Errorf("counts of the others: %+v", counts)
	}

	for _, path := range []string{"/Items/Suggestions?type=Movie&limit=2", "/Users/" + e.memberID + "/Suggestions?type=Movie&limit=2"} {
		var suggestions QueryResult
		if status := e.get(t, path, e.member, &suggestions); status != http.StatusOK || len(suggestions.Items) != 2 ||
			slices.ContainsFunc(suggestions.Items, func(d BaseItemDto) bool { return d.Type != "Movie" }) {
			t.Errorf("%s: %d %v", path, status, suggestions.Items)
		}
	}
	var all QueryResult
	e.get(t, "/Items/Suggestions?mediaType=Video&enableTotalRecordCount=true", e.member, &all)
	if all.TotalRecordCount != 3 || len(all.Items) != 3 {
		t.Errorf("video suggestions: %d %v", all.TotalRecordCount, itemNames(all.Items))
	}
}
