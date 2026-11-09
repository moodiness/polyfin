package library

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// The descriptions of a metadata addon that gives TMDB's and TVDB's
// identifiers as moviedb_id and tvdb_id, trimmed: a movie, a series, and
// "IT" (1990), whose moviedb_id is its TVDB identifier copied.
const (
	shawshankDescribed = `{"id":"tt0111161","imdb_id":"tt0111161","type":"movie","name":"The Shawshank Redemption","moviedb_id":278}`
	survivorDescribed  = `{"id":"tt0239195","imdb_id":"tt0239195","type":"series","name":"Survivor","moviedb_id":14658,"tvdb_id":76733}`
	itDescribed        = `{"id":"tt0099864","imdb_id":"tt0099864","type":"series","name":"IT","moviedb_id":113981,"tvdb_id":113981}`
)

// described decodes descriptions as Polyfin reads an addon's catalog.
func described(t *testing.T, metas ...string) []stremio.Meta {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"metas":[`+strings.Join(metas, ",")+`]}`)
	}))
	defer server.Close()
	decoded, err := stremio.NewClient("test").Catalog(t.Context(), server.URL+"/manifest.json", "movie", "top", nil, false)
	if err != nil || len(decoded) != len(metas) {
		t.Fatalf("%v %+v", err, decoded)
	}
	return decoded
}

// savedTitles saves the movies and series of metas as listed.
func savedTitles(t *testing.T, e env, metas ...stremio.Meta) {
	t.Helper()
	for _, meta := range metas {
		kind := Kind(meta.Type)
		if err := e.service.save(t.Context(), []record{{ID: itemID(titleKey(kind, meta.ID)), Key: titleKey(kind, meta.ID), Kind: kind,
			Meta: &meta}}); err != nil {
			t.Fatal(err)
		}
	}
}

// A title's provider identifiers include those its description gives as
// moviedb_id and tvdb_id, but IT's moviedb_id, its TVDB identifier.
func TestProviderIDsOfDescribedIdentifiers(t *testing.T) {
	t.Parallel()
	metas := described(t, shawshankDescribed, survivorDescribed, itDescribed)
	for i, want := range []string{"map[Imdb:tt0111161 Tmdb:278]", "map[Imdb:tt0239195 Tmdb:14658 Tvdb:76733]", "map[Imdb:tt0099864 Tvdb:113981]"} {
		if got := fmt.Sprint(sourceIDs(metas[i])); got != want {
			t.Errorf("%s: %s, want %s", metas[i].Name, got, want)
		}
	}
}

// A ref by TMDB or TVDB only, as PublicMetaDB names titles, finds the
// title listed whose description gives that identifier as moviedb_id or
// tvdb_id, from the database's lookups; IT's moviedb_id is not one.
func TestResolveFindsTitlesByDescribedIdentifiers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	savedTitles(t, e, described(t, shawshankDescribed, survivorDescribed, itDescribed)...)
	refs := []TitleRef{{TMDB: 278}, {Series: true, TMDB: 14658}, {Series: true, TVDB: 76733}, {Series: true, TMDB: 113981},
		{Series: true, TVDB: 113981}}
	want := []string{"movie|tt0111161", "series|tt0239195", "series|tt0239195", "", "series|tt0099864"}
	titles, err := e.service.knownTitles(t.Context(), refs[:1])
	if err != nil || len(titles) != 1 || titles[0].stremioID != "tt0111161" || titles[0].tmdb != "278" {
		t.Errorf("known by TMDB: %+v %v", titles, err)
	}
	got, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, ref := range refs {
		switch {
		case want[i] == "" && len(got[i]) != 0:
			t.Errorf("%+v: %+v, want none", ref, got[i])
		case want[i] != "" && (len(got[i]) != 1 || got[i][0].ID != itemID(want[i])):
			t.Errorf("%+v: %+v, want %s", ref, got[i], want[i])
		}
	}
}

// A ref that names an IMDb identifier finds no title listed with another
// one, whatever TMDB or TVDB identifier its description gives: IT, which
// an addon describes with another series' TMDB identifier, is not marked
// for that series. A ref without an IMDb identifier, or a title without
// one, still matches by TMDB.
func TestResolveTakesNoTitleOfAnotherIMDbIdentifier(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	savedTitles(t, e, stremio.Meta{ID: "tt0099864", Type: "series", Name: "IT", ImdbID: "tt0099864", TmdbID: "113981"},
		stremio.Meta{ID: "kitsu:2", Type: "series", Name: "Own identifiers", TmdbID: "40"},
		stremio.Meta{ID: "tt0000041", Type: "movie", Name: "Listed by IMDb", TmdbID: "41"})
	for _, c := range []struct {
		ref  TitleRef
		want []string
	}{
		// First, as a ref of a title not listed records it, with the
		// TMDB identifier of the ref.
		{TitleRef{Series: true, TMDB: 113981}, []string{"series|tt0099864"}},
		{TitleRef{Series: true, IMDb: "tt0000113", TMDB: 113981}, []string{"series|tt0000113"}},
		{TitleRef{Series: true, IMDb: "tt0099864", TMDB: 19614}, []string{"series|tt0099864"}},
		{TitleRef{Series: true, IMDb: "tt0000040", TMDB: 40}, []string{"series|kitsu:2"}},
		// The Stremio identifier is the title's IMDb one.
		{TitleRef{IMDb: "tt0000042", TMDB: 41}, []string{"movie|tt0000042"}},
	} {
		got, err := e.service.Resolve(t.Context(), []TitleRef{c.ref})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, target := range got[0] {
			for _, id := range c.want {
				if target.ID == itemID(id) {
					ids = append(ids, id)
				}
			}
		}
		if len(got[0]) != len(c.want) || len(ids) != len(c.want) {
			t.Errorf("%+v: %+v, want %v", c.ref, got[0], c.want)
		}
	}
}

// A numbering tells whether a source told it: not for a series nothing
// numbers as TMDB does, which keeps its numbers, but for one whose
// numbering was kept, or which Polyfin names by its TMDB identifier.
func TestNumberingsTellTheirSource(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	listSeries(t, e, "tt0239195", weekly(1, 2, "2001-01-01"))
	savedTitles(t, e, stremio.Meta{ID: "tmdb:1399", Type: "series", Name: "By TMDB"})
	sourced := func(series string) bool {
		t.Helper()
		numbering, err := e.service.TMDBNumbering(t.Context(), itemID(titleKey(KindSeries, series)))
		if err != nil {
			t.Fatal(err)
		}
		return numbering.Sourced()
	}
	if sourced("tt0239195") || (EpisodeNumbering{}).Sourced() {
		t.Error("a numbering without a source tells one")
	}
	if !sourced("tmdb:1399") {
		t.Error("a series named by TMDB tells no source")
	}
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at)
		VALUES ('tt0239195', 14658, 2, 'null', now())`); err != nil {
		t.Fatal(err)
	}
	if !sourced("tt0239195") {
		t.Error("a kept numbering tells no source")
	}
}
