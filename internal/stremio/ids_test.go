package stremio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A description's moviedb_id fills its TMDB identifier, and a series'
// tvdb_id its TVDB one, written as numbers or strings, when neither its
// Stremio identifier nor _tmdbId or _tvdbId gives them. A moviedb_id that
// is not a positive number, or equal to a TVDB identifier, is left out, as
// is a movie's tvdb_id; the fixtures are trimmed from a metadata addon's
// descriptions.
func TestDescriptionsGiveTMDBAndTVDBIdentifiers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, meta, tmdb, tvdb string
		fromMoviedb            bool
	}{
		{"a movie", `{"id":"tt0111161","imdb_id":"tt0111161","type":"movie","name":"The Shawshank Redemption","moviedb_id":278}`,
			"278", "", true},
		{"a series", `{"id":"tt0239195","imdb_id":"tt0239195","type":"series","name":"Survivor","moviedb_id":14658,"tvdb_id":76733,
			"videos":[{"id":"tt0239195:0:1","season":0,"number":1,"episode":1,"tvdb_id":5582324}]}`, "14658", "76733", true},
		{"identifiers as strings", `{"id":"tt0092455","type":"series","moviedb_id":"655","tvdb_id":"71470"}`, "655", "71470", true},
		{"a TVDB identifier of null", `{"id":"tt6468322","type":"series","name":"Money Heist","moviedb_id":71446,"tvdb_id":null,
			"videos":[{"id":"tt6468322:1:1","season":1,"episode":1,"number":1,"moviedb_id":2264480}]}`, "71446", "", true},
		{"no TVDB identifier", `{"id":"tt0386676","type":"series","moviedb_id":2316}`, "2316", "", true},
		{"a movie's TVDB identifier", `{"id":"tt10794054","type":"movie","moviedb_id":10,"tvdb_id":364390}`, "10", "", true},
		// IT (1990): its TVDB identifier copied, which names another
		// series on TMDB.
		{"a TVDB identifier copied", `{"id":"tt0099864","imdb_id":"tt0099864","type":"series","name":"IT","moviedb_id":113981,"tvdb_id":113981}`,
			"", "113981", false},
		{"a TVDB identifier copied from _tvdbId", `{"id":"tt1","type":"series","moviedb_id":"7","_tvdbId":7}`, "", "7", false},
		{"a TVDB identifier copied from the Stremio one", `{"id":"tvdb:8","type":"series","moviedb_id":8,"tvdb_id":9}`, "", "", false},
		{"_tmdbId and _tvdbId win", `{"id":"tt2","type":"series","_tmdbId":"20","_tvdbId":21,"moviedb_id":22,"tvdb_id":23}`,
			"20", "21", false},
		{"a Stremio TMDB identifier wins", `{"id":"tmdb:30","type":"movie","moviedb_id":31}`, "", "", false},
		{"an addon's own mark", `{"id":"tt3","type":"movie","_tmdbId":32,"_tmdbIdFromMoviedb":true}`, "32", "", false},
		{"zero", `{"id":"tt4","type":"series","moviedb_id":0,"tvdb_id":0}`, "", "", false},
		{"negative", `{"id":"tt5","type":"series","moviedb_id":-5,"tvdb_id":"-6"}`, "", "", false},
		{"not numbers", `{"id":"tt6","type":"series","moviedb_id":"abc","tvdb_id":1.5}`, "", "", false},
		{"of another type", `{"id":"tt7","type":"series","moviedb_id":{"id":1},"tvdb_id":[2],"name":"Odd"}`, "", "", false},
	} {
		var meta Meta
		if err := decodeMeta([]byte(c.meta), &meta); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(meta.TmdbID) != c.tmdb || string(meta.TvdbID) != c.tvdb || meta.TmdbFromMoviedb != c.fromMoviedb {
			t.Errorf("%s: TMDB %q, TVDB %q, from moviedb_id %v; want %q, %q, %v", c.name, meta.TmdbID, meta.TvdbID, meta.TmdbFromMoviedb,
				c.tmdb, c.tvdb, c.fromMoviedb)
		}
	}
}

// The identifiers are read from catalogs and descriptions alike, and the
// description Polyfin keeps writes them as _tmdbId and _tvdbId, with the
// mark of where the TMDB one came from, which it reads back.
func TestDescribedIdentifiersAreKept(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/movie/top.json":
			_, _ = w.Write([]byte(`{"metas":[{"imdb_id":"tt37287335","id":"tt37287335","name":"Obsession","type":"movie","moviedb_id":1339713}]}`))
		case "/meta/series/tt0239195.json":
			_, _ = w.Write([]byte(`{"meta":{"id":"tt0239195","imdb_id":"tt0239195","type":"series","name":"Survivor","moviedb_id":14658,
				"tvdb_id":76733}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer addon.Close()
	client := NewClient("test")
	manifestURL := addon.URL + "/manifest.json"
	metas, err := client.Catalog(t.Context(), manifestURL, "movie", "top", nil, false)
	if err != nil || len(metas) != 1 || metas[0].TmdbID != "1339713" || !metas[0].TmdbFromMoviedb {
		t.Fatalf("catalog: %v %+v", err, metas)
	}
	meta, err := client.Meta(t.Context(), manifestURL, "series", "tt0239195", false)
	if err != nil || meta.TmdbID != "14658" || meta.TvdbID != "76733" || !meta.TmdbFromMoviedb {
		t.Fatalf("meta: %v %+v", err, meta)
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"_tmdbId":"14658"`, `"_tvdbId":"76733"`, `"_tmdbIdFromMoviedb":true`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("kept as %s, without %s", encoded, field)
		}
	}
	var kept Meta
	if err := json.Unmarshal(encoded, &kept); err != nil || kept.TmdbID != "14658" || kept.TvdbID != "76733" || !kept.TmdbFromMoviedb {
		t.Errorf("read back: %v %+v", err, kept)
	}
}
