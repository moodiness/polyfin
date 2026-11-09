package library

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The titles the checks of moviedb_id correct here: "The Last of Us"
// (tt3581920), which a metadata addon gives moviedb_id 502119, a series
// TMDB does not know, and which TMDB knows as 100088; and "IT" (1990,
// tt0099864), which it gives moviedb_id 113981, another real show, and
// which TMDB knows as 19614.
const (
	repairLastOfUs                                  = "tt3581920"
	repairLastOfUsMoviedb, repairLastOfUsTMDB       = 502119, 100088
	repairIT                                        = "tt0099864"
	repairITMoviedb, repairITTMDB                   = 113981, 19614
	repairLastOfUsPreview                           = `{"id":"tt3581920","imdb_id":"tt3581920","type":"series","name":"The Last of Us","releaseInfo":"2023-","moviedb_id":502119}`
	repairLastOfUsDescribed                         = `{"id":"tt3581920","imdb_id":"tt3581920","type":"series","name":"The Last of Us","releaseInfo":"2023-","moviedb_id":502119,"videos":[{"id":"tt3581920:1:1","title":"Pilot","season":1,"episode":1,"released":"2023-01-15T00:00:00.000Z"}]}`
	repairCatalog, repairAddon, repairLastOfUsTitle = "Checked", "checked", "The Last of Us"
)

// repairEnv is a creditsEnv whose server also lists The Last of Us, as a
// metadata addon describes it, with moviedb_id 502119, in a library of
// series, "Checked". It is listed before the queue of TMDB's lookups is
// set (see UseTitleCredits), so that no check is made until a test sets
// it.
func repairEnv(t *testing.T) creditsEnv {
	t.Helper()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: repairAddon, Name: "Checked", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "checked", Name: repairCatalog}}},
		rawCatalogs: map[string]string{"series/checked": "[" + repairLastOfUsPreview + "]"},
		rawMetas:    map[string]string{"series/" + repairLastOfUs: repairLastOfUsDescribed},
	})
	listed := e.children(e.admin, repairCatalog, 0, 20).Items
	if len(listed) != 1 || listed[0].ProviderIDs["Tmdb"] != fmt.Sprint(repairLastOfUsMoviedb) {
		t.Fatalf("the listed series: %+v", listed)
	}
	e.titles[repairLastOfUsTitle] = listed[0].ID
	return e
}

// repairFound makes TMDB find the series of an IMDb identifier as id, and
// answer its credits: an aggregate cast of actors, in billing order.
func repairFound(e creditsEnv, imdb string, id int, actors ...string) {
	e.tmdb.mu.Lock()
	defer e.tmdb.mu.Unlock()
	e.tmdb.finds[imdb] = map[string]any{"tv_results": []any{map[string]any{"id": id}}}
	var cast []map[string]any
	for order, name := range actors {
		cast = append(cast, map[string]any{"id": 7000 + order, "name": name, "order": order,
			"roles": []any{map[string]any{"character": fmt.Sprintf("Character %d", order), "episode_count": 9}}})
	}
	e.tmdb.answers[fmt.Sprintf("/tv/%d", id)] = map[string]any{"id": id, "aggregate_credits": map[string]any{"cast": cast},
		"credits": map[string]any{"crew": []any{}}}
}

// repairEpisodes stores count episodes of the first season of series, as
// its addon lists them, and returns their items' identifiers.
func repairEpisodes(t *testing.T, e env, series string, count int) []accounts.ID {
	t.Helper()
	var records []record
	var ids []accounts.ID
	for number := 1; number <= count; number++ {
		video := stremio.Video{ID: fmt.Sprintf("%s:1:%d", series, number), Season: 1, Episode: stremio.Number(number),
			Name: fmt.Sprintf("Episode %d", number), Released: addonDate(fmt.Sprintf("2023-01-%02d", 8+7*number))}
		records = append(records, record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode, SeriesID: series,
			Season: 1, Video: &video})
		ids = append(ids, itemID(episodeKey(video.ID)))
	}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	return ids
}

// repairLearnt stores what Polyfin learnt of a series by its TMDB
// identifier id: an alignment of its episodes on TMDB's numbering (the
// addon's second episode being TMDB's first), the segment databases'
// answers about each of its episodes, and its TMDB credits, saved on it.
func repairLearnt(t *testing.T, e env, series string, id int, title accounts.ID, episodes []accounts.ID) {
	t.Helper()
	e.exec([]any{`INSERT INTO episode_alignments (series, tmdb_id, episodes, places, asked_at) VALUES ($1, $2, $3, '[[1,1,1,2]]', now())`,
		series, id, len(episodes)},
		[]any{`INSERT INTO media_segments (item_id, answers) SELECT unnest($1::uuid[]), '{}'`, episodes},
		[]any{`INSERT INTO title_credits (kind, provider, id, tmdb_id, titles, asked_at) VALUES ('tv', 'tmdb', $1, $2, $3, now())`,
			fmt.Sprint(id), id, []accounts.ID{title}})
}

// repairCheck has the checks of the titles stored made: a pass (see
// moviedbChecksPass), then the queue settled.
func repairCheck(e creditsEnv) {
	e.t.Helper()
	if err := e.service.moviedbChecksPass(e.t.Context()); err != nil {
		e.t.Fatal(err)
	}
	e.settle()
}

// repairForgetVerdicts forgets the verdicts the service keeps in memory,
// which it reads again from the database when next needed, as after a
// restart.
func repairForgetVerdicts(e env) {
	e.service.moviedbChecks.mu.Lock()
	e.service.moviedbChecks.verdicts = nil
	e.service.moviedbChecks.mu.Unlock()
}

// repairStoredTMDB is the TMDB identifier the stored preview of a series
// gives, empty for none.
func repairStoredTMDB(e env, series string) string {
	e.t.Helper()
	var id string
	if err := e.service.db.QueryRow(e.t.Context(), "SELECT coalesce(data->'meta'->>'_tmdbId', '') FROM items WHERE key = $1",
		titleKey(KindSeries, series)).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// repairCreditsTitles lists the titles saved on the TMDB credits of the
// series of TMDB identifier id.
func repairCreditsTitles(e env, id int) []accounts.ID {
	e.t.Helper()
	var titles []accounts.ID
	if err := e.service.db.QueryRow(e.t.Context(), "SELECT coalesce((SELECT titles FROM title_credits WHERE kind = 'tv' AND provider = 'tmdb' AND id = $1), '{}')",
		fmt.Sprint(id)).Scan(&titles); err != nil {
		e.t.Fatal(err)
	}
	return titles
}

// A check that finds a series' moviedb_id wrong drops what was learnt by
// it: the alignment of its episodes, the segment databases' answers about
// them, and its place among the TMDB credits of the wrong identifier,
// whose credits are looked up under TMDB's. When TMDB knows no title of
// either, the identifier is dropped, and the alignment kept with it no
// longer numbers the episodes.
func TestAWrongMoviedbIdentifierDropsWhatWasLearntByIt(t *testing.T) {
	t.Parallel()
	for _, found := range []bool{true, false} {
		name := "replaced"
		if !found {
			name = "dropped"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := repairEnv(t)
			series := e.titles[repairLastOfUsTitle]
			episodes := repairEpisodes(t, e.env, repairLastOfUs, 2)
			repairLearnt(t, e.env, repairLastOfUs, repairLastOfUsMoviedb, series, episodes)
			if found {
				repairFound(e, repairLastOfUs, repairLastOfUsTMDB, "Tess Actor")
			}
			numbered := func() bool {
				t.Helper()
				numbering, err := e.service.TMDBNumbering(t.Context(), series)
				if err != nil {
					t.Fatal(err)
				}
				_, number, ok := numbering.ToTMDB(1, 2)
				return ok && number == 1
			}
			if !numbered() {
				t.Fatal("the kept alignment does not number the episodes before the check")
			}
			e.service.UseTitleCredits(e.queue)
			repairCheck(e)

			if n := e.count("SELECT count(*) FROM episode_alignments WHERE series = $1 AND tmdb_id = $2", repairLastOfUs, repairLastOfUsMoviedb); n != 0 {
				t.Errorf("the alignment by the wrong identifier is kept")
			}
			if n := e.count("SELECT count(*) FROM media_segments WHERE item_id = ANY($1::uuid[])", episodes); n != 0 {
				t.Errorf("%d answers of the segment databases are kept", n)
			}
			if titles := repairCreditsTitles(e.env, repairLastOfUsMoviedb); slices.Contains(titles, series) {
				t.Errorf("the series is kept on the wrong identifier's credits: %v", titles)
			}
			if !found {
				if id := repairStoredTMDB(e.env, repairLastOfUs); id != "" {
					t.Errorf("the stored TMDB identifier: %q, want none", id)
				}
				if numbered() {
					t.Error("the alignment by the dropped identifier still numbers the episodes")
				}
				return
			}
			if id := repairStoredTMDB(e.env, repairLastOfUs); id != fmt.Sprint(repairLastOfUsTMDB) {
				t.Errorf("the stored TMDB identifier: %q", id)
			}
			if asked := e.tmdb.requests(fmt.Sprintf("/3/tv/%d?", repairLastOfUsTMDB)); len(asked) != 1 {
				t.Errorf("the credits under TMDB's identifier were asked %v", asked)
			}
			if titles := repairCreditsTitles(e.env, repairLastOfUsTMDB); !slices.Contains(titles, series) {
				t.Errorf("the series is not on TMDB's identifier's credits: %v", titles)
			}
		})
	}
}

// A title's page has its moviedb_id checked first, and waits for it: The
// Last of Us opens with TMDB's identifier, and the credits read by it,
// TMDB being asked nothing by the wrong one.
func TestATitlePageWaitsForItsMoviedbCheck(t *testing.T) {
	t.Parallel()
	e := repairEnv(t)
	repairFound(e, repairLastOfUs, repairLastOfUsTMDB, "Tess Actor", "Bill Actor")
	e.service.UseTitleCredits(e.queue)
	item := e.open(e.admin, repairLastOfUsTitle)
	if got := item.ProviderIDs["Tmdb"]; got != fmt.Sprint(repairLastOfUsTMDB) {
		t.Errorf("the page's TMDB identifier: %q", got)
	}
	if got := cast(item.People, "Actor"); !slices.Equal(got, []string{"Tess Actor", "Bill Actor"}) {
		t.Errorf("the page's actors: %v", got)
	}
	if asked := e.tmdb.requests(fmt.Sprintf("/3/tv/%d", repairLastOfUsMoviedb)); len(asked) != 0 {
		t.Errorf("TMDB was asked by the wrong identifier: %v", asked)
	}
}

// The daily pass checks the titles stored whose TMDB identifier is their
// moviedb_id, never described again: IT is replaced by TMDB's identifier.
// It corrects a title a kept verdict found wrong, still showing the wrong
// identifier, asking TMDB nothing about it. A second pass asks nothing.
func TestTheDailyPassChecksTheStoredMoviedbIdentifiers(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	repairFound(e, repairIT, repairITTMDB)
	savedTitles(t, e.env,
		stremio.Meta{ID: repairIT, Type: "series", Name: "IT", ImdbID: repairIT, ReleaseInfo: "1990", TmdbID: stremio.Text(fmt.Sprint(repairITMoviedb)),
			TmdbFromMoviedb: true},
		stremio.Meta{ID: repairLastOfUs, Type: "series", Name: "The Last of Us", ImdbID: repairLastOfUs, ReleaseInfo: "2023-",
			TmdbID: stremio.Text(fmt.Sprint(repairLastOfUsMoviedb)), TmdbFromMoviedb: true})
	// A check found The Last of Us' moviedb_id wrong, and TMDB's right,
	// before its preview was corrected.
	e.exec([]any{`INSERT INTO moviedb_id_checks (kind, tmdb_id, imdb_id, year, wrong, replaced_by, checked_at)
		VALUES ('tv', $1, $3, 0, true, $2, now()), ('tv', $2, $3, 0, false, NULL, now())`, repairLastOfUsMoviedb, repairLastOfUsTMDB, repairLastOfUs})
	repairForgetVerdicts(e.env)
	e.service.UseTitleCredits(e.queue)
	repairCheck(e)
	if id := repairStoredTMDB(e.env, repairIT); id != fmt.Sprint(repairITTMDB) {
		t.Errorf("IT's stored TMDB identifier: %q", id)
	}
	if asked := e.tmdb.requests("/3/find/" + repairIT); len(asked) != 1 {
		t.Errorf("TMDB was asked IT %v", asked)
	}
	if id := repairStoredTMDB(e.env, repairLastOfUs); id != fmt.Sprint(repairLastOfUsTMDB) {
		t.Errorf("The Last of Us' stored TMDB identifier: %q", id)
	}
	for _, prefix := range []string{"/3/find/" + repairLastOfUs, fmt.Sprintf("/3/tv/%d", repairLastOfUsMoviedb)} {
		if asked := e.tmdb.requests(prefix); len(asked) != 0 {
			t.Errorf("TMDB was asked The Last of Us' check: %v", asked)
		}
	}
	before := e.tmdb.requests("/")
	repairCheck(e)
	if after := e.tmdb.requests("/"); len(after) != len(before) {
		t.Errorf("the second pass asked %v", after[len(before):])
	}
}

// The IMDb check of imports no longer refuses The Last of Us for TMDB's
// identifier once the check of its moviedb_id corrected it.
func TestACheckedMoviedbIdentifierNoLongerContradictsItsIMDbIdentifier(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	repairFound(e, repairLastOfUs, repairLastOfUsTMDB)
	savedTitles(t, e.env, stremio.Meta{ID: repairLastOfUs, Type: "series", Name: "The Last of Us", ImdbID: repairLastOfUs,
		TmdbID: stremio.Text(fmt.Sprint(repairLastOfUsMoviedb)), TmdbFromMoviedb: true})
	ref := TitleRef{Episode: true, TMDB: repairLastOfUsTMDB, Season: 1, Number: 1}
	got, err := e.service.ContradictedIMDb(t.Context(), ref, []string{repairLastOfUs})
	if want := map[string]bool{repairLastOfUs: true}; err != nil || !maps.Equal(got, want) {
		t.Errorf("before the check: %v %v, want %v", got, err, want)
	}
	e.service.UseTitleCredits(e.queue)
	repairCheck(e)
	got, err = e.service.ContradictedIMDb(t.Context(), ref, []string{repairLastOfUs})
	if err != nil || len(got) != 0 {
		t.Errorf("after the check: %v %v, want none", got, err)
	}
}

// A verdict outlives the key: once the key is removed, the addon's
// descriptions of The Last of Us, read anew, and its catalog's still show
// TMDB's identifier, read from the verdicts kept, TMDB being asked
// nothing.
func TestAMoviedbVerdictOutlivesTheKey(t *testing.T) {
	t.Parallel()
	e := repairEnv(t)
	repairFound(e, repairLastOfUs, repairLastOfUsTMDB)
	e.service.UseTitleCredits(e.queue)
	repairCheck(e)
	if id := repairStoredTMDB(e.env, repairLastOfUs); id != fmt.Sprint(repairLastOfUsTMDB) {
		t.Fatalf("the stored TMDB identifier after the check: %q", id)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	repairForgetVerdicts(e.env)
	asked := e.tmdb.requests("/")
	addon := e.installedAddon(repairAddon)
	if err := e.service.forgetMeta(t.Context(), metaKey{addon.ID, "series", repairLastOfUs}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.service.meta(t.Context(), installed{addon: addon}, "series", repairLastOfUs)
	if err != nil || meta.TmdbID != stremio.Text(fmt.Sprint(repairLastOfUsTMDB)) || !meta.TmdbFromMoviedb {
		t.Errorf("the description read anew: %q %v %v", meta.TmdbID, meta.TmdbFromMoviedb, err)
	}
	listed := e.children(e.admin, repairCatalog, 0, 20).Items
	if len(listed) != 1 || listed[0].ProviderIDs["Tmdb"] != fmt.Sprint(repairLastOfUsTMDB) {
		t.Errorf("the listed series: %+v", listed)
	}
	if after := e.tmdb.requests("/"); len(after) != len(asked) {
		t.Errorf("TMDB was asked without a key: %v", after[len(asked):])
	}
}
