package jellyfin

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"
)

// A title carries the streaming services TMDB names in the rating country
// as its tags; jellyfin-web's link of a tag lists its titles across the
// libraries, with a type Jellyfin ignores, and a library's filter menu
// offers them and narrows the library by them.
func TestStreamingServicesAreTagsAppsFilterBy(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	e.tmdb.mu.Lock()
	e.tmdb.details["/3/movie/9301"]["watch/providers"] = map[string]any{"results": map[string]any{
		"US": map[string]any{"flatrate": []any{map[string]any{"provider_id": 8, "provider_name": "Stream One"}},
			"rent": []any{map[string]any{"provider_id": 11, "provider_name": "Rent Four"}}},
	}}
	e.tmdb.mu.Unlock()
	movies := e.list(t)
	var harbor BaseItemDto
	// A page waits a moment for the details: on a loaded machine, they
	// show at a next open.
	for range 100 {
		if status := e.get(t, "/Items/"+movies["Bare Harbor"].Id, e.token, &harbor); status != http.StatusOK {
			t.Fatalf("Bare Harbor: %d", status)
		}
		if harbor.Tags != nil && len(*harbor.Tags) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if harbor.Tags == nil || !slices.Equal(*harbor.Tags, []string{"Stream One"}) {
		t.Fatalf("Bare Harbor's tags: %v", harbor.Tags)
	}

	for _, path := range []string{
		// jellyfin-web's link of a tag.
		"/Items?StartIndex=0&Limit=100&Fields=PrimaryImageAspectRatio%2CSortName&ImageTypeLimit=1&IncludeItemTypes=tag&Recursive=true" +
			"&SortBy=SortName&Tags=" + url.QueryEscape("Stream One"),
		"/Items?recursive=true&tags=stream%20one&includeItemTypes=Movie",
		"/Items?parentId=" + e.library + "&tags=" + url.QueryEscape("Stream One"),
	} {
		var page QueryResult
		if status := e.get(t, path, e.token, &page); status != http.StatusOK || len(page.Items) != 1 || page.Items[0].Name != "Bare Harbor" {
			t.Errorf("%s: %d %+v", path, status, page)
		}
	}
	var page QueryResult
	if status := e.get(t, "/Items?recursive=true&tags=Rent%20Four", e.token, &page); status != http.StatusOK || len(page.Items) != 0 {
		t.Errorf("a service to rent: %d %+v", status, page)
	}

	var filters struct{ Tags []string }
	if status := e.get(t, "/Items/Filters?parentId="+e.library, e.token, &filters); status != http.StatusOK ||
		!slices.Equal(filters.Tags, []string{"Stream One"}) {
		t.Errorf("/Items/Filters: %d %v", status, filters.Tags)
	}
	filters.Tags = nil
	if status := e.get(t, "/Items/Filters2?parentId="+e.library+"&includeItemTypes=Movie", e.token, &filters); status != http.StatusOK ||
		!slices.Equal(filters.Tags, []string{"Stream One"}) {
		t.Errorf("/Items/Filters2: %d %v", status, filters.Tags)
	}
}
