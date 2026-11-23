package library

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// shelfLocal is a local folder, Shelf, whose catalog lists titles, as
// stored: it counts the windows and the pages of it read. The files it
// holds are those of its titles.
type shelfLocal struct {
	Local
	titles         []stremio.Meta
	windows, pages atomic.Int32
}

func (l *shelfLocal) Catalog(_ context.Context, _ accounts.ID, _, _ string, skip int, _ string) ([]stremio.Meta, error) {
	l.pages.Add(1)
	return l.titles[min(skip, len(l.titles)):min(skip+100, len(l.titles))], nil
}

func (l *shelfLocal) CatalogWindow(_ context.Context, _ accounts.ID, _, _ string, start, count int) ([]stremio.Meta, error) {
	l.windows.Add(1)
	return l.titles[min(start, len(l.titles)):min(start+count, len(l.titles))], nil
}

func (l *shelfLocal) HeldTitles(_ context.Context, _ []accounts.ID, ids []string) (map[string]bool, error) {
	asked := map[string]bool{}
	for _, id := range ids {
		asked[id] = true
	}
	held := map[string]bool{}
	for _, t := range l.titles {
		if asked[t.ID] {
			held[t.ID] = true
		}
	}
	return held, nil
}

func (l *shelfLocal) Meta(context.Context, accounts.ID, string) (stremio.Meta, error) {
	return stremio.Meta{}, stremio.ErrNotFound
}

func (l *shelfLocal) Streams(context.Context, accounts.ID, string) ([]stremio.Stream, error) {
	return nil, nil
}
func (l *shelfLocal) Artwork() map[accounts.ID]Images { return nil }
func (l *shelfLocal) Changes() uint64                 { return 0 }

func (l *shelfLocal) HeldVideos(context.Context, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (l *shelfLocal) TitleDurations(context.Context, []string) (map[string]time.Duration, error) {
	return map[string]time.Duration{}, nil
}

func (l *shelfLocal) EpisodeDurations(context.Context, string) (map[[2]int]time.Duration, error) {
	return map[[2]int]time.Duration{}, nil
}

func (l *shelfLocal) Episodes(context.Context, []accounts.ID, string, *stremio.Meta) ([]stremio.Video, error) {
	return nil, nil
}

// scopeAddon serves Picks, the movies Harbor (tt11, made in the United
// States) and Lantern (tt12, in France), whose listing gives no country,
// and describes them, Shelf's Quarry (tt21, in France and Belgium) and the
// IPTV source's Ridge (tt31, in Japan), with their countries.
func scopeAddon() *fakeAddon {
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "scope", Name: "Scope", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		pageSize: 100,
	}
	for _, m := range []struct {
		id, name, year, country string
		listed                  bool
	}{
		{"tt11", "Harbor", "2001", "United States", true},
		{"tt12", "Lantern", "2005", "France", true},
		{"tt21", "Quarry", "1999", "France, Belgium", false},
		{"tt31", "Ridge", "2010", "Japan", false},
	} {
		meta := stremio.Meta{ID: m.id, Type: "movie", Name: m.name, ReleaseInfo: stremio.Text(m.year)}
		if m.listed {
			addon.catalogs["movie/picks"] = append(addon.catalogs["movie/picks"], meta)
		}
		meta.Country = stremio.Names(strings.Split(m.country, ", "))
		addon.metas["movie/"+m.id] = meta
	}
	return addon
}

// scopeEnv lists, besides scopeAddon's Picks, Shelf, a local folder, and
// Box, an Xtream account's movies, and the catalogs of the three a smart
// list may draw from.
type scopeEnv struct {
	env
	shelf    *shelfLocal
	catalogs []smartlists.Catalog
}

// newScopeEnv makes a scopeEnv of Box's movies and Shelf's titles, whose
// service's statements statements counts, when set.
func newScopeEnv(t *testing.T, movies []map[string]any, shelf []stremio.Meta, statements **statements) scopeEnv {
	t.Helper()
	e := scopeEnv{env: newEnv(t), shelf: &shelfLocal{titles: shelf}}
	e.service.smartWait = time.Minute
	ctx := t.Context()
	if statements != nil {
		*statements = counted(t, e.service)
		e.service.userData, e.service.watchlist = userdata.New(e.service.db), watchlist.New(e.service.db)
	}
	e.install(addons.Shared(), scopeAddon())
	e.service.UseLocal(e.shelf)
	if _, err := e.addons.Create(ctx, addons.Shared(), addons.KindLocal, "/media/shelf", func(accounts.ID) stremio.Manifest {
		return stremio.Manifest{ID: "polyfin.local.shelf", Version: "1", Name: "Shelf", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"movie"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "local", Name: "Shelf", Extra: []stremio.Extra{{Name: "search"}, {Name: "skip"}}}}}
	}, func(pgx.Tx, addons.Addon) error { return nil }); err != nil {
		t.Fatal(err)
	}
	channels := iptv.New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	e.service.UseIPTV(channels)
	provider := newStudioXtream(t, movies...)
	on, off := true, false
	if _, err := channels.Add(ctx, addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: provider.url, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{LiveTv: &off, Movies: &on}}, false); err != nil {
		t.Fatal(err)
	}
	choices, err := e.service.SmartCatalogs(ctx, e.admin, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{SmartSourceAddon, SmartSourceFolder, SmartSourceIPTV} {
		i := slices.IndexFunc(choices, func(c SmartCatalog) bool { return c.Source == source && c.Catalog.Type == "movie" })
		if i < 0 {
			t.Fatalf("no %s catalog among %+v", source, choices)
		}
		e.catalogs = append(e.catalogs, smartlists.Catalog{AddonID: choices[i].AddonID, Type: "movie", ID: choices[i].Catalog.ID})
	}
	return e
}

// list saves a list of the server's drawing from the three catalogs.
func (e scopeEnv) list(name string, rules smartlists.Rule, sort string) smartlists.List {
	e.t.Helper()
	list, err := e.service.smart.Create(e.t.Context(), smartlists.List{Name: name, ShownAs: smartlists.ShownAsLibrary, Catalogs: e.catalogs,
		Rules: rules, Sort: sort})
	if err != nil {
		e.t.Fatal(err)
	}
	return list
}

// A list draws from an addon's catalog, a local folder and an IPTV
// source's movies at once, and judges their titles by their tags, their
// countries, their critics' and Metacritic scores, the user's watchlist
// and their local files; sorted by date played, it lists the titles the
// user played the latest first, then the others as their catalogs list
// them. A local folder's title and an IPTV source's are described by the
// addon that describes their IMDb identifier; an IPTV title without one
// by its listing alone.
func TestASmartListDrawsFromAddonsFoldersAndIPTVAndJudgesTheirTitles(t *testing.T) {
	t.Parallel()
	shelf := []stremio.Meta{{ID: "tt21", Type: "movie", Name: "Quarry", Year: "1999"}, {ID: "tt12", Type: "movie", Name: "Lantern", Year: "2005"}}
	ridge, plain := xtreamMovie(1, "Ridge", 0, "1"), xtreamMovie(2, "Plain Box", 0, "1")
	ridge["imdb"] = "tt31"
	e := newScopeEnv(t, []map[string]any{ridge, plain}, shelf, nil)
	ctx := t.Context()
	scores := ratings.New(e.service.db, "http://127.0.0.1:1", "test", slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	t.Cleanup(scores.Close)
	e.service.UseRatings(scores)
	e.setting(func(s *accounts.Settings) { s.MDBListRatingsKey = "k3y" })
	if _, err := e.service.db.Exec(ctx, `INSERT INTO title_ratings (kind, provider, id, found, critics, metacritic, asked_at) VALUES
		('movie', 'imdb', 'tt11', true, 91, 80, now()), ('movie', 'imdb', 'tt12', true, 40, NULL, now()),
		('movie', 'imdb', 'tt21', true, 75, 70, now()), ('movie', 'imdb', 'tt31', true, NULL, 60, now())`); err != nil {
		t.Fatal(err)
	}

	everything := e.list("Everything", group(smartlists.MatchAll), smartlists.SortCatalog)
	page := e.listing(e.member, everything)
	if got, want := names(page.Items), []string{"Harbor", "Quarry", "Plain Box", "Lantern", "Ridge"}; !slices.Equal(got, want) {
		t.Fatalf("every title: %v, want %v", got, want)
	}
	byName := map[string]Item{}
	for _, item := range page.Items {
		byName[item.Name] = item
	}
	for _, name := range []string{"Harbor", "Ridge"} {
		if err := e.service.SaveOverrides(ctx, byName[name], Overrides{Tags: []string{"Award"}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Lantern", "Ridge"} {
		id := byName[name].ID
		if err := e.service.watchlist.Add(ctx, e.member.ID, []watchlist.Group{{Title: id, Items: []accounts.ID{id}}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	played := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	for i, name := range []string{"Ridge", "Harbor", "Quarry"} {
		at := played.Add(time.Duration(i) * time.Hour)
		if _, err := e.service.userData.Change(ctx, e.member.ID, []userdata.Item{{ID: byName[name].ID}}, func(d *userdata.Data) {
			d.Played, d.LastPlayed = true, &at
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		name string
		rule smartlists.Rule
		want []string
	}{
		{"tagged", smartlists.Rule{Field: smartlists.FieldTag, Value: "award"}, []string{"Harbor", "Ridge"}},
		{"made in France", smartlists.Rule{Field: smartlists.FieldCountry, Value: "FR"}, []string{"Lantern", "Quarry"}},
		{"praised by critics", smartlists.Rule{Field: smartlists.FieldCritics, Min: new(70.0)}, []string{"Harbor", "Quarry"}},
		{"Metacritic", smartlists.Rule{Field: smartlists.FieldMetacritic, Min: new(65.0)}, []string{"Harbor", "Quarry"}},
		{"not Metacritic", smartlists.Rule{Field: smartlists.FieldMetacritic, Min: new(65.0), Not: true}, []string{"Ridge"}},
		{"on the watchlist", smartlists.Rule{Field: smartlists.FieldState, Value: smartlists.StateWatchlist}, []string{"Lantern", "Ridge"}},
		{"with a local file", smartlists.Rule{Field: smartlists.FieldLocalFile}, []string{"Lantern", "Quarry"}},
		{"without a local file", smartlists.Rule{Field: smartlists.FieldLocalFile, Not: true}, []string{"Harbor", "Plain Box", "Ridge"}},
	} {
		list := e.list(c.name, group(smartlists.MatchAll, c.rule), smartlists.SortName)
		if got := e.settled(e.member, list); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	history := e.list("History", group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldState, Value: smartlists.StatePlayed}),
		smartlists.SortDatePlayed)
	if got, want := names(e.listing(e.member, history).Items), []string{"Quarry", "Harbor", "Ridge"}; !slices.Equal(got, want) {
		t.Errorf("watch history: %v, want %v", got, want)
	}
	byDate := e.list("By date played", group(smartlists.MatchAll), smartlists.SortDatePlayed)
	if got, want := names(e.listing(e.member, byDate).Items), []string{"Quarry", "Harbor", "Ridge", "Plain Box", "Lantern"}; !slices.Equal(got, want) {
		t.Errorf("by date played: %v, want %v", got, want)
	}
	if got := names(e.listing(e.admin, history).Items); len(got) != 0 {
		t.Errorf("another user's history: %v", got)
	}
	if pages := e.shelf.pages.Load(); pages != 0 {
		t.Errorf("Shelf read in %d pages, want whole windows", pages)
	}
}

// A list drawing from 10,000 stored IPTV titles and 10,000 titles of a
// local folder, besides an addon's, reads each of them in one query and
// looks up what its rules and sort need of them in a few statements, not
// one per title: its listing stays within a bounded number of statements,
// and reads every catalog within the 3 seconds a listing waits for them,
// complete. The time it took is logged: a loaded test database, or the
// race detector, slows it apart from the queries. The IPTV source's titles
// are stored in one statement, as its refresh stores them, rather than
// fetched and parsed from its provider, which the test does not measure.
func TestASmartListOfTenThousandStoredTitlesIsAFewQueries(t *testing.T) {
	t.Parallel()
	const stored = 10000
	shelf := make([]stremio.Meta, 0, stored)
	for i := range stored {
		shelf = append(shelf, stremio.Meta{ID: fmt.Sprintf("tt8%06d", i), Type: "movie", Name: fmt.Sprintf("Shelf %05d", i)})
	}
	first := xtreamMovie(1, "Box 00000", 0, "1")
	first["imdb"] = "tt9000000"
	var counter *statements
	e := newScopeEnv(t, []map[string]any{first}, shelf, &counter)
	box := e.catalogs[2].AddonID
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO iptv_titles (addon_id, type, key, position, name, category, added, imdb, search)
		SELECT $1, 'movie', 'seeded' || i, 1 + i, 'Box ' || lpad(i::text, 5, '0'), c.category, to_timestamp(1700000000 + i),
			'tt9' || lpad(i::text, 6, '0'), 'box ' || lpad(i::text, 5, '0')
		FROM generate_series(1, $2::integer - 1) AS i, (SELECT category FROM iptv_titles WHERE addon_id = $1 LIMIT 1) AS c`,
		box, stored); err != nil {
		t.Fatal(err)
	}
	e.service.smartWait = smartWait
	// Every rule is looked up for every title; the last one, which every
	// title matches, has the list keep them all: both sources' and the
	// addon's two.
	list := e.list("Large", group(smartlists.MatchAny,
		smartlists.Rule{Field: smartlists.FieldTag, Value: "Award"},
		smartlists.Rule{Field: smartlists.FieldCritics, Min: new(70.0)},
		smartlists.Rule{Field: smartlists.FieldState, Value: smartlists.StateWatchlist},
		smartlists.Rule{Field: smartlists.FieldLocalFile, Not: true},
		smartlists.Rule{Field: smartlists.FieldType, Value: "movie"},
	), smartlists.SortDatePlayed)

	var preview SmartPreview
	var took time.Duration
	seen := counter.run(func() {
		start := time.Now()
		var err error
		preview, err = e.service.PreviewSmartList(t.Context(), e.member, list)
		took = time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
	})
	if want := 2*stored + 2; preview.Listed != want || !preview.Complete || preview.Unknown != 0 {
		t.Errorf("listed %d titles (complete %v, %d unknown), want %d", preview.Listed, preview.Complete, preview.Unknown, want)
	}
	t.Logf("%d statements in %v", len(seen), took)
	if len(seen) > 30 {
		var sqls []string
		for _, s := range seen {
			sql := strings.Join(strings.Fields(s.sql), " ")
			sqls = append(sqls, sql[:min(len(sql), 100)])
		}
		t.Errorf("%d statements, want at most 30:\n%s", len(seen), strings.Join(sqls, "\n"))
	}
	if windows, pages := e.shelf.windows.Load(), e.shelf.pages.Load(); windows != 1 || pages != 0 {
		t.Errorf("Shelf read in %d windows and %d pages, want 1 window", windows, pages)
	}
}
