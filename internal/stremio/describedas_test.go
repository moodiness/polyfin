package stremio

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// An addon may describe a title by another identifier than the one asked.
// Its description is returned under the identifier asked, by which the
// title is recorded, so that what is recorded from it names that title;
// the identifier the addon gave still tells the title's IMDb or TMDB one.
func TestADescriptionIsReturnedUnderTheIdentifierAsked(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/meta/series/own:1.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt0000009", "type": "series", "name": "Show", "videos": [{"id": "tt0000009:1:1", "season": 1, "episode": 1}]}}`))
		case "/meta/series/own:2.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tmdb:77", "type": "series", "name": "Show"}}`))
		case "/meta/series/tt0000003.json":
			_, _ = w.Write([]byte(`{"meta": {"id": "tt0000003", "type": "series", "name": "Show"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	client := NewClient("test")
	manifest := addon.URL + "/manifest.json"

	meta, err := client.Meta(t.Context(), manifest, "series", "own:1", false)
	if err != nil || meta.ID != "own:1" || meta.ImdbID != "tt0000009" {
		t.Errorf("a description by an IMDb identifier: id %q, IMDb %q, %v; want own:1 and tt0000009", meta.ID, meta.ImdbID, err)
	}
	if len(meta.Videos) != 1 || meta.Videos[0].ID != "tt0000009:1:1" {
		t.Errorf("its videos keep their identifiers: %+v", meta.Videos)
	}
	meta, err = client.Meta(t.Context(), manifest, "series", "own:2", false)
	if err != nil || meta.ID != "own:2" || meta.TmdbID != "77" {
		t.Errorf("a description by a TMDB identifier: id %q, TMDB %q, %v; want own:2 and 77", meta.ID, meta.TmdbID, err)
	}
	meta, err = client.Meta(t.Context(), manifest, "series", "tt0000003", false)
	if err != nil || meta.ID != "tt0000003" || meta.ImdbID != "" || meta.TmdbID != "" {
		t.Errorf("a description by the identifier asked: %+v %v", meta, err)
	}
}
