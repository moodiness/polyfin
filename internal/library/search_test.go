package library

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// searchEnv is a library whose movies, people, and episodes have names
// holding %, _ and \, with names that these would match as LIKE's
// wildcards, and the statements its service runs.
type searchEnv struct {
	languageEnv
	traced *statements
}

func newSearchEnv(t *testing.T) searchEnv {
	t.Helper()
	e := searchEnv{languageEnv: languageEnv{env: newEnv(t)}, traced: &statements{}}
	config := e.service.db.Config()
	config.ConnConfig.Tracer = e.traced
	traced, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	e.service.db = traced
	movie := func(id, name string, cast ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, Cast: stremio.Names(cast)}
	}
	movies := []stremio.Meta{
		movie("tt1", "50% Off", `Ana 100% Actor`, "Ben a_b Actor"),
		movie("tt2", "50 Off", "Ana 100 Actor", "Ben axb Actor"),
		movie("tt3", "Snake_Case", `Cleo Back\slash`),
		movie("tt4", "SnakeXCase", "Cleo Backslash"),
	}
	video := func(episode int, title string) stremio.Video {
		return stremio.Video{ID: "tt9:1:" + strconv.Itoa(episode), Title: title, Season: 1, Episode: stremio.Number(episode), Released: "2020-01-01T00:00:00Z"}
	}
	show := stremio.Meta{ID: "tt9", Type: "series", Name: "The Show", Videos: []stremio.Video{
		video(1, "Part 1%"), video(2, "Part 10"), video(3, "under_score"), video(4, "underXscore"),
		video(5, `back\slash`), video(6, "backslash"),
	}}
	row := func(m stremio.Meta) stremio.Meta {
		m.Videos, m.Cast = nil, nil
		return m
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {row(show)}},
		metas:    map[string]stremio.Meta{"series/tt9": show},
	}
	for _, m := range movies {
		addon.catalogs["movie/top"] = append(addon.catalogs["movie/top"], row(m))
		addon.metas["movie/"+m.ID] = m
	}
	e.install(addons.Shared(), addon)
	for _, id := range e.titleIDs("Top (Movies)") {
		if _, err := e.service.Item(t.Context(), e.admin, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.service.Episodes(t.Context(), e.admin, e.titleIDs("Top (Shows)")["The Show"], nil); err != nil {
		t.Fatal(err)
	}
	return e
}

// Searches by name find the text as written: %, _ and \ match themselves
// only, and the case does not count.
func TestNameSearchesMatchWildcardsLiterally(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	people := func(q PeopleQuery) []string {
		t.Helper()
		found, total, err := e.service.People(ctx, e.admin, q)
		if err != nil {
			t.Fatal(err)
		}
		if q.Start == 0 && q.Limit < 0 && total != len(found) {
			t.Errorf("%+v: %d people counted, %d listed", q, total, len(found))
		}
		return names(found)
	}
	for term, want := range map[string][]string{
		"100%": {"Ana 100% Actor"}, "A_B": {"Ben a_b Actor"}, `k\s`: {`Cleo Back\slash`}, "actor": {"Ana 100 Actor", "Ana 100% Actor", "Ben a_b Actor", "Ben axb Actor"},
	} {
		if got := people(PeopleQuery{NameContains: term, Limit: -1}); !slices.Equal(got, want) {
			t.Errorf("people containing %q: %v, want %v", term, got, want)
		}
	}
	if got, want := people(PeopleQuery{NameStartsWith: `cleo back\`, Limit: -1}), []string{`Cleo Back\slash`}; !slices.Equal(got, want) {
		t.Errorf("people starting with a backslash: %v, want %v", got, want)
	}
	if got, want := people(PeopleQuery{NameFrom: "b", NameBefore: "c", Limit: -1}), []string{"Ben a_b Actor", "Ben axb Actor"}; !slices.Equal(got, want) {
		t.Errorf("people from b, before c: %v, want %v", got, want)
	}
	if got, want := people(PeopleQuery{Start: 1, Limit: 2}), []string{"Ana 100% Actor", "Ben a_b Actor"}; !slices.Equal(got, want) {
		t.Errorf("the second page of 2: %v, want %v", got, want)
	}
	for term, want := range map[string][]string{"1%": {"Part 1%"}, "R_S": {"under_score"}, `k\s`: {`back\slash`}} {
		found, err := e.service.SearchEpisodes(ctx, e.admin, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(found); !slices.Equal(got, want) {
			t.Errorf("episodes containing %q: %v, want %v", term, got, want)
		}
	}
	for term, want := range map[string][]string{"0%": {"50% Off"}, "e_c": {"Snake_Case"}} {
		found, err := e.service.ShareTitles(ctx, e.admin, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(found); !slices.Equal(got, want) {
			t.Errorf("titles containing %q: %v, want %v", term, got, want)
		}
	}
}

// Every name search reads items through an index: with sequential scans
// turned off, a plan that still has one has no index to use. Plans are
// checked as prepared statements first run them, with their arguments,
// and as generic plans, which pgx's cached statements come to.
func TestNameSearchesReadItemsThroughIndexes(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	var trigrams bool
	if err := e.service.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')").Scan(&trigrams); err != nil {
		t.Fatal(err)
	}
	if !trigrams {
		t.Skip("the test server has no pg_trgm")
	}
	searches := map[string]func(){
		"people": func() {
			_, _, _ = e.service.People(ctx, e.admin, PeopleQuery{NameContains: "actor", Limit: 10})
		},
		"people starting with": func() {
			_, _, _ = e.service.People(ctx, e.admin, PeopleQuery{NameStartsWith: "ana", Limit: 10})
		},
		"episodes": func() { _, _ = e.service.SearchEpisodes(ctx, e.admin, "part", 10) },
		"titles":   func() { _, _ = e.service.ShareTitles(ctx, e.admin, "off", 10) },
	}
	for name, search := range searches {
		var checked int
		for _, s := range e.traced.run(search) {
			if !strings.Contains(s.sql, "->>'name'") {
				continue
			}
			checked++
			if text := explained(t, e.service.db, "EXPLAIN "+s.sql, s.args); strings.Contains(text, "Seq Scan on items") {
				t.Errorf("%s, with its arguments: %s\n%s", name, s.sql, text)
			}
			if text := explained(t, e.service.db, "EXPLAIN (GENERIC_PLAN) "+s.sql, nil); strings.Contains(text, "Seq Scan on items") {
				t.Errorf("%s, generic: %s\n%s", name, s.sql, text)
			}
		}
		if checked == 0 {
			t.Errorf("%s: no name search ran", name)
		}
	}
}

// explained is the plan of explain with sequential scans turned off; with
// no args, a generic plan's, its placeholders left unbound.
func explained(t *testing.T, db *pgxpool.Pool, explain string, args []any) string {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) }()
	if _, err := tx.Exec(t.Context(), "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if args == nil {
		// pgx would bind the placeholders: the server reads them as they
		// are.
		results, err := tx.Conn().PgConn().Exec(t.Context(), explain).ReadAll()
		if err != nil {
			t.Fatalf("%s: %v", explain, err)
		}
		for _, row := range results[0].Rows {
			lines = append(lines, string(row[0]))
		}
		return strings.Join(lines, "\n")
	}
	rows, err := tx.Query(t.Context(), explain, args...)
	if err != nil {
		t.Fatalf("%s: %v", explain, err)
	}
	if lines, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		t.Fatalf("%s: %v", explain, err)
	}
	return strings.Join(lines, "\n")
}

// The episodes a search finds are described from their series, read
// together: no series is read on its own, however many episodes match.
func TestEpisodeSearchReadsTheSeriesTogether(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	var found []Item
	ran := e.traced.run(func() {
		var err error
		if found, err = e.service.SearchEpisodes(t.Context(), e.admin, "a", 10); err != nil {
			t.Fatal(err)
		}
	})
	if len(found) < 4 {
		t.Fatalf("episodes found: %v", names(found))
	}
	var single int
	for _, s := range ran {
		if strings.Contains(s.sql, "FROM items WHERE id = $1") {
			single++
		}
	}
	if single > 0 {
		t.Errorf("%d items read one by one for %d episodes", single, len(found))
	}
}
