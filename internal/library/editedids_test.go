package library

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// editIDs keeps an administrator's edit of the TMDB and TVDB identifiers
// of the title of kind that meta describes, as jellyfin-web's Edit
// metadata sends them, nil for a field not sent.
func editIDs(t *testing.T, s *Service, kind Kind, meta stremio.Meta, tmdbID, tvdbID *string) {
	t.Helper()
	base := Item{ID: itemID(titleKey(kind, meta.ID)), Kind: kind}
	s.fromMeta(&base, meta, "")
	if err := s.SaveOverrides(t.Context(), base, Overrides{TmdbID: tmdbID, TvdbID: tvdbID}); err != nil {
		t.Fatal(err)
	}
}

// An administrator's TMDB and TVDB identifiers win over every source of
// them, _tmdbId and moviedb_id alike, and a TMDB identifier set is no
// moviedb_id; the identifier a "tmdb:" or "tvdb:" title is stays its own.
func TestEditedIdentifiersWinOverEverySource(t *testing.T) {
	t.Parallel()
	metas := map[Kind][]stremio.Meta{
		KindMovie: {{ID: "tt0111161", Type: "movie", ImdbID: "tt0111161", TmdbID: "278", TmdbFromMoviedb: true}},
		KindSeries: {{ID: "tt0239195", Type: "series", ImdbID: "tt0239195", TmdbID: "14658", TvdbID: "76733"},
			{ID: "tmdb:1399", Type: "series", TvdbID: "121361"}, {ID: "tvdb:81189", Type: "series", TmdbID: "1396"}},
	}
	edits := map[Kind]Overrides{KindMovie: {TmdbID: new("680")}, KindSeries: {TmdbID: new("1400"), TvdbID: new("900")}}
	set := &overrideSet{fields: map[accounts.ID]Overrides{}}
	for kind, list := range metas {
		for _, meta := range list {
			set.fields[itemID(titleKey(kind, meta.ID))] = edits[kind]
		}
	}
	s := &Service{}
	s.overrides.Store(set)
	want := map[string]string{
		"tt0111161":  "map[Imdb:tt0111161 Tmdb:680]",
		"tt0239195":  "map[Imdb:tt0239195 Tmdb:1400 Tvdb:900]",
		"tmdb:1399":  "map[Tmdb:1399 Tvdb:900]",
		"tvdb:81189": "map[Tmdb:1400 Tvdb:81189]",
	}
	for kind, list := range metas {
		for _, meta := range list {
			item := Item{ID: itemID(titleKey(kind, meta.ID)), Kind: kind}
			s.fromMeta(&item, meta, "")
			if got := fmt.Sprint(item.ProviderIDs); got != want[meta.ID] || item.TMDBFromMoviedb {
				t.Errorf("%s: %s (from moviedb_id %v), want %s", meta.ID, got, item.TMDBFromMoviedb, want[meta.ID])
			}
			if got := fmt.Sprint(s.providerIDs(item.ID, meta)); got != want[meta.ID] {
				t.Errorf("%s, read by its item: %s, want %s", meta.ID, got, want[meta.ID])
			}
			if got, source := fmt.Sprint(item.sourceIDs), fmt.Sprint(sourceIDs(meta)); got != source {
				t.Errorf("%s: the source's %s, want %s", meta.ID, got, source)
			}
		}
	}
	// Without an edit, the moviedb_id stays one.
	unedited := Item{ID: itemID(titleKey(KindMovie, "tt0000001")), Kind: KindMovie}
	s.fromMeta(&unedited, stremio.Meta{ID: "tt0000001", TmdbID: "5", TmdbFromMoviedb: true}, "")
	if !unedited.TMDBFromMoviedb || unedited.ProviderIDs["Tmdb"] != "5" || unedited.sourceIDs != nil {
		t.Errorf("unedited: %+v", unedited)
	}
}

// An edit keeps a TMDB or TVDB identifier typed, without spaces or leading
// zeros, unless it is the source's or empty, which follows the source
// again; what is no identifier, a movie's TVDB identifier, the identity of
// a "tmdb:" or "tvdb:" title and an episode's identifiers change nothing.
func TestEditsKeepOnlyIdentifiersOtherThanTheSources(t *testing.T) {
	t.Parallel()
	movie := Item{Kind: KindMovie, StremioID: "tt0111161", ProviderIDs: map[string]string{"Imdb": "tt0111161", "Tmdb": "278"}}
	series := Item{Kind: KindSeries, StremioID: "tt0239195", ProviderIDs: map[string]string{"Tmdb": "1400", "Tvdb": "900"},
		sourceIDs: map[string]string{"Tmdb": "14658", "Tvdb": "76733"}}
	byTMDB := Item{Kind: KindSeries, StremioID: "tmdb:1399", ProviderIDs: map[string]string{"Tmdb": "1399"}}
	byTVDB := Item{Kind: KindSeries, StremioID: "tvdb:81189", ProviderIDs: map[string]string{"Tvdb": "81189"}}
	episode := Item{Kind: KindEpisode, StremioID: "tt0239195:1:1"}
	current := Overrides{TmdbID: new("1400"), TvdbID: new("900")}
	show := func(value *string) string {
		if value == nil {
			return "-"
		}
		return *value
	}
	for _, c := range []struct {
		name       string
		base       Item
		tmdb, tvdb *string
		want       string
	}{
		{"typed", movie, new(" 0680 "), nil, "680 -"},
		{"the source's", movie, new("278"), nil, "- -"},
		{"empty", series, new(""), new(" "), "- -"},
		{"the source's of an edited series", series, new("14658"), new("76733"), "- -"},
		{"not sent", series, nil, nil, "1400 900"},
		{"the shown edit", series, new("1400"), new("900"), "1400 900"},
		{"changed", series, new("1401"), new("901"), "1401 901"},
		{"text", series, new("abc"), new("12a"), "1400 900"},
		{"zero and negative", series, new("0"), new("-5"), "1400 900"},
		{"beyond PostgreSQL's integer", series, new("2147483648"), new("1e3"), "1400 900"},
		{"PostgreSQL's largest integer", series, new("2147483647"), nil, "2147483647 900"},
		{"a movie's TVDB identifier", movie, nil, new("900"), "- -"},
		{"a tmdb: title's identity", byTMDB, new("1"), new("2"), "- 2"},
		{"a tvdb: title's identity", byTVDB, new("1"), new("2"), "1 -"},
		{"an episode", episode, new("1"), new("2"), "- -"},
	} {
		kept := current
		if c.base.Kind == KindMovie {
			kept.TvdbID = nil
		}
		if c.base.Kind != KindSeries || c.base.StremioID != "tt0239195" {
			kept = Overrides{}
		}
		got := (Overrides{TmdbID: c.tmdb, TvdbID: c.tvdb}).against(c.base, kept)
		if s := show(got.TmdbID) + " " + show(got.TvdbID); s != c.want {
			t.Errorf("%s: %s, want %s", c.name, s, c.want)
		}
	}
}

// Imports find a title by the TMDB and TVDB identifiers an administrator
// set, and no longer by its source's, until the edit is cleared.
func TestImportsFindTitlesByTheirEditedIdentifiers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	metas := described(t, shawshankDescribed, survivorDescribed)
	savedTitles(t, e, metas...)
	shawshank, survivor := "movie|tt0111161", "series|tt0239195"
	resolved := func(refs ...TitleRef) []string {
		t.Helper()
		got, err := e.service.Resolve(t.Context(), refs)
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for i, targets := range got {
			key := ""
			for _, target := range targets {
				for _, k := range []string{shawshank, survivor} {
					if target.ID == itemID(k) {
						key = k
					}
				}
			}
			if key == "" && len(targets) > 0 {
				key = fmt.Sprintf("%+v", refs[i])
			}
			keys = append(keys, key)
		}
		return keys
	}
	refs := []TitleRef{{TMDB: 680}, {TMDB: 278}, {Series: true, TMDB: 1400}, {Series: true, TMDB: 14658}, {Series: true, TVDB: 900},
		{Series: true, TVDB: 76733}}
	if got := resolved(refs...); !slices.Equal(got, []string{"", shawshank, "", survivor, "", survivor}) {
		t.Fatalf("before the edits: %q", got)
	}
	editIDs(t, e.service, KindMovie, metas[0], new("680"), nil)
	editIDs(t, e.service, KindSeries, metas[1], new("1400"), new("900"))
	titles, err := e.service.knownTitles(t.Context(), refs[:1])
	if err != nil || len(titles) != 1 || titles[0].stremioID != "tt0111161" || titles[0].tmdb != "680" {
		t.Errorf("known by the edited TMDB identifier: %+v %v", titles, err)
	}
	if got := resolved(refs...); !slices.Equal(got, []string{shawshank, "", survivor, "", survivor, ""}) {
		t.Errorf("after the edits: %q", got)
	}
	// By its IMDb identifier, the title is found whatever the others.
	if got := resolved(TitleRef{IMDb: "tt0111161", TMDB: 278}); !slices.Equal(got, []string{shawshank}) {
		t.Errorf("by IMDb: %q", got)
	}
	editIDs(t, e.service, KindMovie, metas[0], new(""), nil)
	if got := resolved(refs[:2]...); !slices.Equal(got, []string{"", shawshank}) {
		t.Errorf("after clearing: %q", got)
	}
}

// A series' alignment of its episodes on TMDB's numbering is dropped when
// its TMDB identifier is edited, and made again with the identifier set;
// one kept without a TMDB identifier is not taken once the edit is
// cleared.
func TestAnEditedTMDBIdentifierRealignsEpisodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	survivor := survivorSecondSeason()
	f := newAlignedTMDB(t, map[int][]airedFixture{survivorTMDB: survivor})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	listSeries(t, e, survivorIMDb, survivor)
	series := itemID(titleKey(KindSeries, survivorIMDb))
	meta := stremio.Meta{ID: survivorIMDb, Type: "series", Name: "Series " + survivorIMDb}
	// An alignment made with another series' identifier, which a series
	// of no TMDB identifier keeps.
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at)
		VALUES ($1, 1434, 29, '[[1,1,1,2]]', now())`, survivorIMDb); err != nil {
		t.Fatal(err)
	}
	numbering := func() EpisodeNumbering {
		t.Helper()
		n, err := e.service.TMDBNumbering(t.Context(), series)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, number, ok := numbering().ToTMDB(1, 2); !ok || number != 1 {
		t.Fatalf("the kept alignment is not taken: S1E%d %v", number, ok)
	}
	kept := func() int {
		t.Helper()
		var id int
		if err := e.service.db.QueryRow(t.Context(), "SELECT tmdb_id FROM episode_alignments WHERE series = $1", survivorIMDb).Scan(&id); err != nil {
			return 0
		}
		return id
	}
	editIDs(t, e.service, KindSeries, meta, new(fmt.Sprint(survivorTMDB)), nil)
	if id := kept(); id != 0 {
		t.Errorf("the alignment is kept after the edit, of %d", id)
	}
	checkNumbering(t, "edited", numbering(), survivor)
	if id := kept(); id != survivorTMDB {
		t.Errorf("the alignment made again is of %d", id)
	}
	if asked := f.requests(survivorTMDB); len(asked) == 0 {
		t.Error("TMDB was not asked the edited series")
	}
	editIDs(t, e.service, KindSeries, meta, new(""), nil)
	if id := kept(); id != 0 {
		t.Errorf("the alignment is kept after clearing, of %d", id)
	}
	if n := numbering(); n.Sourced() {
		t.Errorf("a series of no TMDB identifier is numbered: %+v", n)
	}
}

// An edit outlives its title's descriptions stored again, and a restart.
func TestEditedIdentifiersOutliveRefreshesAndRestarts(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	metas := described(t, survivorDescribed)
	savedTitles(t, e, metas...)
	editIDs(t, e.service, KindSeries, metas[0], new("1400"), new("900"))
	id := itemID(titleKey(KindSeries, metas[0].ID))
	check := func(when string, s *Service) {
		t.Helper()
		r, err := s.load(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(s.providerIDs(id, *r.Meta)); got != "map[Imdb:tt0239195 Tmdb:1400 Tvdb:900]" {
			t.Errorf("%s: %s", when, got)
		}
		titles, err := s.knownTitles(t.Context(), []TitleRef{{Series: true, TVDB: 900}})
		if err != nil || len(titles) != 1 || titles[0].tmdb != "1400" {
			t.Errorf("%s, known: %+v %v", when, titles, err)
		}
	}
	check("edited", e.service)
	// The addon describes the title again: its row is stored anew.
	savedTitles(t, e, metas...)
	check("described again", e.service)
	check("after a restart", e.restarted().service)
}

// A title's poster address and TMDB credits follow its edited TMDB
// identifier: the item leaves the credits of its former one, and takes
// those of the new one, which its people become.
func TestPostersAndCreditsFollowAnEditedTMDBIdentifier(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.setting(func(s *accounts.Settings) { s.PosterTemplate = "http://192.168.1.3/{type}/{tmdb_id}.jpg" })
	harbor := e.open(e.admin, "Harbor Lights")
	if got := cast(harbor.People, "Actor"); len(got) < 4 || got[0] != "Ana Actor" {
		t.Fatalf("Harbor Lights' actors: %v", got)
	}
	extra := itemID(personTMDBKey("8011"))
	if got := e.titlesOf(e.admin, extra, KindMovie); !slices.Equal(got, []string{"Harbor Lights"}) {
		t.Fatalf("Billed Extra 8011's movies: %v", got)
	}
	base, err := e.service.Original(t.Context(), e.admin, harbor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.service.SaveOverrides(t.Context(), base, Overrides{TmdbID: new("1012")}); err != nil {
		t.Fatal(err)
	}
	e.settle()
	var titles []accounts.ID
	if err := e.service.db.QueryRow(t.Context(), "SELECT titles FROM title_credits WHERE kind = 'movie' AND provider = 'imdb' AND id = 'tt0882001'").
		Scan(&titles); err != nil || slices.Contains(titles, harbor.ID) {
		t.Errorf("the former credits' titles: %v %v", titles, err)
	}
	if asked := e.tmdb.requests("/3/movie/1012/credits"); len(asked) != 1 {
		t.Errorf("TMDB was asked %v", asked)
	}
	harbor = e.open(e.admin, "Harbor Lights")
	if got := cast(harbor.People, "Actor"); !slices.Equal(got, []string{"Ana Actor"}) {
		t.Errorf("Harbor Lights' actors after the edit: %v", got)
	}
	// The extra, whom no title credits now, is no longer recorded.
	if page, err := e.service.PersonTitles(t.Context(), e.admin, extra, []Kind{KindMovie}, 20); err != nil && !errors.Is(err, ErrNotFound) ||
		err == nil && len(page.Items) != 0 {
		t.Errorf("Billed Extra 8011's movies after the edit: %+v %v", page.Items, err)
	}
	if harbor.Images.Primary != "http://192.168.1.3/movie/1012.jpg" || harbor.ProviderIDs["Tmdb"] != "1012" {
		t.Errorf("Harbor Lights' poster %q and identifiers %v", harbor.Images.Primary, harbor.ProviderIDs)
	}
}
