package trackers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// PublicMetaDB names titles by TMDB only: a title listed with a
// description giving its TMDB identifier as moviedb_id, trimmed from a
// metadata addon's, is found by it, without asking PublicMetaDB to map it
// to IMDb.
func TestPublicMetaDBImportsFindTitlesByTheirMoviedbID(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"meta":{"id":"tt0111161","imdb_id":"tt0111161","type":"movie","name":"The Shawshank Redemption","moviedb_id":278}}`)
	}))
	defer addon.Close()
	meta, err := stremio.NewClient("test").Meta(t.Context(), addon.URL+"/manifest.json", "movie", "tt0111161", false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"kind": "movie", "meta": meta})
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	user := h.user(t, "ivy")
	h.connected(t, user, PublicMetaDB, "pm-ivy")
	if _, err := h.db.Exec(t.Context(), "INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'movie', $3)", itemOf("movie|tt0111161"),
		"movie|tt0111161", data); err != nil {
		t.Fatal(err)
	}
	h.f.reply("GET", "/publicmetadb/api/external/watched", http.StatusOK,
		`{"items":[{"tmdb_id":278,"media_type":"movie","watched_at":"2026-09-02 20:00:00.000Z"}],"totalPages":1}`)
	h.f.reply("GET", "/publicmetadb/api/external/resume", http.StatusOK, `{"items":[],"totalPages":1}`)
	if result := h.turnOn(t, user, PublicMetaDB); result.Played != 1 || result.Unmapped != 0 {
		t.Errorf("import: %+v", result)
	}
	if got := h.data(t, user, library.TitleRef{IMDb: "tt0111161"}); !got[0].Played {
		t.Errorf("the movie: %+v", got[0])
	}
	if mapped := h.f.sent("publicmetadb", "/api/external/mappings"); len(mapped) != 0 {
		t.Errorf("PublicMetaDB asked to map: %+v", mapped)
	}
	h.noWrites(t)
}
