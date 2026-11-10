package jellyfin

import (
	"net/http"
	"slices"
	"testing"
)

// A movie's page shows TMDB's studios and tagline, read with its credits.
// The metadata editor's save of them unchanged keeps no edit of them, so
// that the movie follows TMDB; an edit of them wins.
func TestAMoviesPageShowsTMDBsStudiosAndTagline(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	e.tmdb.mu.Lock()
	details := e.tmdb.details["/3/movie/9301"]
	details["production_companies"] = []any{map[string]any{"name": "North Pictures"}, map[string]any{"name": "South Films"}}
	details["tagline"] = "A harbor at night."
	details["credits"] = map[string]any{"cast": []any{}, "crew": []any{}}
	e.tmdb.mu.Unlock()
	id := e.list(t)["Bare Harbor"].Id
	page := func() (studios, taglines []string) {
		t.Helper()
		var dto BaseItemDto
		if status := e.get(t, "/Items/"+id, e.token, &dto); status != http.StatusOK || dto.Studios == nil || dto.Taglines == nil {
			t.Fatalf("details: %d", status)
		}
		for _, studio := range *dto.Studios {
			studios = append(studios, studio.Name)
		}
		return studios, *dto.Taglines
	}
	studios, taglines := page()
	if !slices.Equal(studios, []string{"North Pictures", "South Films"}) || !slices.Equal(taglines, []string{"A harbor at night."}) {
		t.Fatalf("the page: %q %q", studios, taglines)
	}
	unchanged := editorBody(id, map[string]any{"Name": "Bare Harbor", "ProviderIds": map[string]string{"Imdb": "tt0893001"},
		"Studios": []map[string]string{{"Name": "North Pictures"}, {"Name": "South Films"}}, "Taglines": taglines})
	if status, body := e.call(http.MethodPost, "/Items/"+id, app("dashboard", e.token), unchanged); status != http.StatusNoContent {
		t.Fatalf("save: %d %s", status, body)
	}
	var edited int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM item_overrides WHERE item_id = $1 AND (fields ? 'studios' OR fields ? 'taglines')`,
		id).Scan(&edited); err != nil || edited != 0 {
		t.Errorf("the unchanged save kept %d edits: %v", edited, err)
	}
	edit := editorBody(id, map[string]any{"Name": "Bare Harbor", "ProviderIds": map[string]string{"Imdb": "tt0893001"},
		"Studios": []map[string]string{{"Name": "Own Studio"}}, "Taglines": []string{"Mine."}})
	if status, body := e.call(http.MethodPost, "/Items/"+id, app("dashboard", e.token), edit); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	if studios, taglines := page(); !slices.Equal(studios, []string{"Own Studio"}) || !slices.Equal(taglines, []string{"Mine."}) {
		t.Errorf("after the edit: %q %q", studios, taglines)
	}
}
