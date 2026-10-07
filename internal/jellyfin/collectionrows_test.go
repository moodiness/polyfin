package jellyfin

import (
	"net/http"
	"testing"
)

// A library of an addon's collections is told to apps as a library of
// mixed content, without content type: Jellyfin apps leave libraries of
// collections out of their home rows of latest items, and give one to the
// others. Its row lists its collections; and listed as jellyfin-web lists
// a library without type, by its folders, movies and series, it lists its
// collections too.
func TestCollectionLibrariesGetHomeRows(t *testing.T) {
	s, token, ids := browsing(t)
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	types := map[string]string{}
	for _, view := range views.Items {
		types[view.Name] = view.CollectionType
	}
	if len(types) != 3 || types["Groups"] != "" || types["Top"] != "movies" || types["Shows"] != "tvshows" {
		t.Errorf("content types: %v", types)
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?ParentId="+ids["Groups"], token, &latest); status != http.StatusOK || len(latest) != 1 || latest[0].Type != "BoxSet" {
		t.Errorf("the collection library's row: %d %+v", status, latest)
	}
	for _, query := range []string{
		"IncludeItemTypes=Folder,Movie,Series&Recursive=false",
		"IncludeItemTypes=BoxSet&Recursive=true",
		"ExcludeItemTypes=Folder",
	} {
		var listed QueryResult
		if status := s.get(t, "/Items?ParentId="+ids["Groups"]+"&"+query, token, &listed); status != http.StatusOK || len(listed.Items) != 1 || listed.Items[0].Type != "BoxSet" {
			t.Errorf("the collection library listed with %s: %d %+v", query, status, listed.Items)
		}
	}
}
