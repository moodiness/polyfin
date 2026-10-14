package letterboxd

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// exportZip zips files, by their path in the ZIP.
func exportZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		f, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// sampleExport is a member's export, with invented movies and links: The
// Paper Lantern watched twice in the diary and rated 3.5 stars; Quiet
// Harbour only in watched.csv; Northern Static rated 4 stars; Copper
// Fields rated in the diary only; Glass Orchard on the watchlist; Lantern
// Bay liked; Twin Rivers, of which the addons know two movies of 2014;
// Unknown Shore, which they do not know; and Broken Compass, whose search
// fails. The files carry columns of their own, in their own order, and
// the likes folder files the member liked besides movies, not read.
var sampleExport = map[string]string{
	"profile.csv": "Date Joined,Username\n2019-02-03,sample\n",
	"watched.csv": "\ufeffDate,Name,Year,Letterboxd URI\n" +
		"2020-01-01,The Paper Lantern,1987,https://boxd.it/aaa1\n" +
		"2022-05-06,Quiet Harbour,1999,https://boxd.it/aaa2\n" +
		"2021-07-08,Northern Static,2005,https://boxd.it/aaa3\n" +
		"2021-07-09,Twin Rivers,2014,https://boxd.it/aaa5\n" +
		"2021-07-10,Unknown Shore,2001,https://boxd.it/aaa6\n" +
		"2021-07-11,Broken Compass,1990,javascript:alert(1)\n" +
		"2021-07-12,Lantern Bay,2018,https://boxd.it/aaa7\n",
	"diary.csv": "Date,Name,Year,Letterboxd URI,Rating,Rewatch,Tags,Watched Date,Mood\n" +
		"2021-03-05,The Paper Lantern,1987,https://boxd.it/diary1,,,,2021-03-04,calm\n" +
		"2023-06-11,The Paper Lantern,1987,https://boxd.it/diary2,3,Yes,,2023-06-10,calm\n" +
		"2022-09-02,Copper Fields,2003,https://boxd.it/diary3,2.5,,,2022-09-01,\n",
	"ratings.csv": "Name,Year,Rating,Date,Letterboxd URI\n" +
		"The Paper Lantern,1987,3.5,2023-06-11,https://boxd.it/aaa1\n" +
		"Northern Static,2005,4,2021-07-08,https://boxd.it/aaa3\n",
	"watchlist.csv":     "Date,Name,Year,Letterboxd URI\n2024-01-02,Glass Orchard,2011,https://boxd.it/aaa4\n",
	"likes/films.csv":   "Date,Name,Year,Letterboxd URI\n2021-07-12,Lantern Bay,2018,https://boxd.it/aaa7\n",
	"likes/reviews.csv": "Date,Name,Year,Letterboxd URI\n2021-07-12,Glass Orchard,2011,https://boxd.it/review\n",
}

// The movies the fake addons find, by IMDb identifier.
var addonMovies = []library.Item{
	{Name: "The Paper Lantern", ProductionYear: 1987, StremioID: "tt7100001"},
	// Another movie of the name, of another year, leaves the one of 1987.
	{Name: "The Paper Lantern", ProductionYear: 2019, StremioID: "tt7100011"},
	{Name: "Quiet Harbour", ProductionYear: 1999, StremioID: "tt7100002"},
	{Name: "Northern Static", ProductionYear: 2005, StremioID: "tt7100003"},
	{Name: "Glass Orchard", ProductionYear: 2011, StremioID: "tt7100004"},
	{Name: "Twin Rivers", ProductionYear: 2014, StremioID: "tt7100005"},
	{Name: "Twin Rivers", ProductionYear: 2014, StremioID: "tt7100006"},
	{Name: "Lantern Bay", ProductionYear: 2018, StremioID: "tt7100007"},
	{Name: "Copper Fields", ProductionYear: 2003, StremioID: "tt7100008"},
}

// fakeTitles searches addonMovies, failing for Broken Compass, and counts
// the searches by name; titles are resolved by the library.
type fakeTitles struct {
	*library.Service
	mu       sync.Mutex
	searches map[string]int
}

func (f *fakeTitles) RemoteSearch(_ context.Context, kind library.Kind, name string, _ int) ([]library.RemoteResult, error) {
	f.mu.Lock()
	f.searches[name]++
	f.mu.Unlock()
	if name == "Broken Compass" {
		return nil, errors.New("the addon did not answer")
	}
	var results []library.RemoteResult
	for _, m := range addonMovies {
		if kind == library.KindMovie && strings.Contains(strings.ToLower(m.Name), strings.ToLower(name)) {
			m.Kind, m.ProviderIDs = library.KindMovie, map[string]string{"Imdb": m.StremioID}
			results = append(results, library.RemoteResult{Item: m, Provider: "Meta"})
		}
	}
	return results, nil
}

func (f *fakeTitles) RemoteSearches(context.Context, library.Kind) (bool, error) { return true, nil }

func (f *fakeTitles) searched(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.searches[name]
}

type harness struct {
	*Service
	titles *fakeTitles
	data   *userdata.Store
	lists  *watchlist.Store
	user   accounts.User
}

func newHarness(t *testing.T) harness {
	t.Helper()
	pool := migrated.New(t)
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := stremio.NewClient("test")
	lib := library.New(pool, addons.New(pool, client), client, logger, store.Settings)
	titles := &fakeTitles{Service: lib, searches: map[string]int{}}
	data, lists := userdata.New(pool), watchlist.New(pool)
	s := New(Options{Titles: titles, UserData: data, Watchlist: lists, Logger: logger})
	t.Cleanup(s.Close)
	user, err := store.CreateUser(t.Context(), accounts.NewUser{Name: "mira", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return harness{Service: s, titles: titles, data: data, lists: lists, user: user}
}

// imported imports export with choice into the user's account, and waits
// for the import to end, done.
func (h harness) imported(t *testing.T, export []byte, choice Choice) *Status {
	t.Helper()
	parsed, err := Parse(export)
	if err != nil {
		t.Fatal(err)
	}
	started, err := h.Start(h.user, h.user, parsed, choice)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != StateRunning {
		t.Fatalf("started: %+v", started)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		current := h.Current(h.user.ID)
		if current.State != StateRunning {
			if current.State != StateDone || current.ID != started.ID {
				t.Fatalf("import: %+v", current)
			}
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %+v", current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// item is the item of the movie imdb names.
func (h harness) item(t *testing.T, imdb string) accounts.ID {
	t.Helper()
	found, err := h.titles.Resolve(t.Context(), []library.TitleRef{{IMDb: imdb}})
	if err != nil || len(found[0]) != 1 {
		t.Fatalf("%s: %v %v", imdb, found, err)
	}
	return found[0][0].ID
}

// data is the user's data of the movies imdbs name, by identifier.
func (h harness) dataOf(t *testing.T, imdbs ...string) map[string]userdata.Data {
	t.Helper()
	ids := make([]accounts.ID, len(imdbs))
	for i, imdb := range imdbs {
		ids[i] = h.item(t, imdb)
	}
	got, err := h.data.Get(t.Context(), h.user.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]userdata.Data{}
	for i, imdb := range imdbs {
		result[imdb] = got[ids[i]]
	}
	return result
}

func noon(day string) time.Time {
	at, _ := time.Parse("2006-01-02", day)
	return at.Add(12 * time.Hour)
}

// An export's movies are found by name and year, one search per name:
// those watched are played, at their latest diary date, else watched.csv's,
// as many times as the diary logs them; ratings are stars × 2, the
// diary's when ratings.csv lacks the movie, and a rating Polyfin has
// stays; the watchlist goes to the Watchlist until started; the movies
// liked become favorites only with the switch on; the movies not found are
// listed with why and their link; and importing again adds nothing.
func TestImport(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// The member already rated Northern Static in Polyfin.
	if _, err := h.data.Change(t.Context(), h.user.ID, []userdata.Item{{ID: h.item(t, "tt7100003")}}, func(d *userdata.Data) {
		d.Rating = new(6.0)
	}); err != nil {
		t.Fatal(err)
	}
	export := exportZip(t, sampleExport)

	first := h.imported(t, export, Choice{Watchlist: true, Ratings: true})
	if first.Movies != 9 || first.Searched != 9 || first.Played != 5 || first.Watchlist != 1 || first.Ratings != 2 ||
		first.Favorites != 0 {
		t.Errorf("first import: %+v", first)
	}
	for name, searched := range map[string]int{"The Paper Lantern": 1, "Twin Rivers": 1, "Broken Compass": 1} {
		if got := h.titles.searched(name); got != searched {
			t.Errorf("%s searched %d times", name, got)
		}
	}
	want := []Unmatched{
		{Name: "Twin Rivers", Year: 2014, Link: "https://boxd.it/aaa5", Reason: ReasonAmbiguous},
		{Name: "Unknown Shore", Year: 2001, Link: "https://boxd.it/aaa6", Reason: ReasonNotFound},
		// Its link is not a web address: it is not shown.
		{Name: "Broken Compass", Year: 1990, Reason: ReasonSearchFailed},
	}
	if first.UnmatchedCount != len(want) || len(first.Unmatched) != len(want) {
		t.Fatalf("unmatched: %+v", first.Unmatched)
	}
	for i := range want {
		if first.Unmatched[i] != want[i] {
			t.Errorf("unmatched %d: %+v, want %+v", i, first.Unmatched[i], want[i])
		}
	}

	got := h.dataOf(t, "tt7100001", "tt7100002", "tt7100003", "tt7100004", "tt7100007", "tt7100008", "tt7100011")
	lantern := got["tt7100001"]
	if !lantern.Played || lantern.PlayCount != 2 || lantern.LastPlayed == nil || !lantern.LastPlayed.Equal(noon("2023-06-10")) {
		t.Errorf("The Paper Lantern, watched twice: %+v", lantern)
	}
	if lantern.Rating == nil || *lantern.Rating != 7 || lantern.Likes() == nil || !*lantern.Likes() {
		t.Errorf("The Paper Lantern, 3.5 stars: %+v", lantern)
	}
	harbour := got["tt7100002"]
	if !harbour.Played || harbour.PlayCount != 1 || harbour.LastPlayed == nil || !harbour.LastPlayed.Equal(noon("2022-05-06")) ||
		harbour.Rating != nil {
		t.Errorf("Quiet Harbour, in watched.csv only: %+v", harbour)
	}
	if static := got["tt7100003"]; !static.Played || static.Rating == nil || *static.Rating != 6 {
		t.Errorf("Northern Static keeps Polyfin's rating: %+v", static)
	}
	if copper := got["tt7100008"]; !copper.Played || copper.Rating == nil || *copper.Rating != 5 {
		t.Errorf("Copper Fields, rated in the diary: %+v", copper)
	}
	if orchard := got["tt7100004"]; orchard.Played || orchard.Favorite {
		t.Errorf("Glass Orchard, on the watchlist: %+v", orchard)
	}
	if bay := got["tt7100007"]; !bay.Played || bay.Favorite {
		t.Errorf("Lantern Bay, liked, with the switch off: %+v", bay)
	}
	if other := got["tt7100011"]; other.Played || other.Rating != nil {
		t.Errorf("the other movie of the name: %+v", other)
	}
	orchard := h.item(t, "tt7100004")
	if listed, err := h.lists.Titles(t.Context(), h.user.ID); err != nil || len(listed) != 1 || listed[0] != orchard {
		t.Errorf("watchlist: %v %v", listed, err)
	}

	again := h.imported(t, export, Choice{Watchlist: true, Ratings: true})
	if again.Played != 0 || again.Watchlist != 0 || again.Ratings != 0 || again.Favorites != 0 || again.UnmatchedCount != 3 {
		t.Errorf("importing again: %+v", again)
	}
	if again := h.dataOf(t, "tt7100001")["tt7100001"]; again.PlayCount != 2 || !again.LastPlayed.Equal(noon("2023-06-10")) {
		t.Errorf("The Paper Lantern, imported again: %+v", again)
	}

	liked := h.imported(t, export, Choice{LikesAsFavorites: true})
	if liked.Movies != 8 || liked.Favorites != 1 || liked.Played != 0 || liked.Ratings != 0 || liked.Watchlist != 0 {
		t.Errorf("with the liked movies: %+v", liked)
	}
	if bay := h.dataOf(t, "tt7100007")["tt7100007"]; !bay.Favorite {
		t.Errorf("Lantern Bay, liked, with the switch on: %+v", bay)
	}

	// Glass Orchard leaves the watchlist once started.
	if _, err := h.data.Change(t.Context(), h.user.ID, []userdata.Item{{ID: orchard}}, func(d *userdata.Data) {
		d.Start(time.Now(), true)
		d.Position, d.Runtime = 10*time.Minute, time.Hour
	}); err != nil {
		t.Fatal(err)
	}
	if listed, err := h.lists.Titles(t.Context(), h.user.ID); err != nil || len(listed) != 0 {
		t.Errorf("watchlist once started: %v %v", listed, err)
	}
}

// One import into an account runs at a time.
func TestImportRunningRefusesAnother(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	release := make(chan struct{})
	blocking := &blockingTitles{fakeTitles: h.titles, release: release}
	h.Service.titles = blocking
	parsed, err := Parse(exportZip(t, sampleExport))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Start(h.user, h.user, parsed, Choice{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Start(h.user, h.user, parsed, Choice{}); !errors.Is(err, ErrRunning) {
		t.Errorf("a second import: %v", err)
	}
	if !h.Running(h.user.ID) {
		t.Error("not running")
	}
	close(release)
}

// blockingTitles holds each search until release is closed, then fails it.
type blockingTitles struct {
	*fakeTitles
	release chan struct{}
}

func (b *blockingTitles) RemoteSearch(ctx context.Context, _ library.Kind, _ string, _ int) ([]library.RemoteResult, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil, errors.New("held")
}

// Only the files of an export are read, by their columns' names, under
// the folder that holds them; a ZIP without watched.csv nor diary.csv,
// a file that is not a ZIP, and a file without the Name column are
// refused.
func TestParse(t *testing.T) {
	t.Parallel()
	zipped := map[string]string{}
	for name, content := range sampleExport {
		zipped["letterboxd-sample-2026-10-01/"+name] = content
	}
	// A diary of deleted entries, in a folder of its own, is not read.
	zipped["letterboxd-sample-2026-10-01/deleted/diary.csv"] = "Date,Name,Year,Letterboxd URI,Rating,Rewatch,Tags,Watched Date\n" +
		"2020-01-01,Removed Movie,2000,https://boxd.it/gone,,,,2020-01-01\n"
	export, err := Parse(exportZip(t, zipped))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]Movie{}
	for _, m := range export.Movies {
		names[m.Name] = m
	}
	if len(export.Movies) != 9 || names["Removed Movie"].Name != "" {
		t.Fatalf("movies: %+v", export.Movies)
	}
	if m := names["The Paper Lantern"]; m.Year != 1987 || m.Link != "https://boxd.it/aaa1" || m.Plays != 2 || !m.Watched ||
		!m.PlayedAt.Equal(noon("2023-06-10")) || m.Rating == nil || *m.Rating != 7 {
		t.Errorf("The Paper Lantern: %+v", m)
	}
	if m := names["Copper Fields"]; m.Link != "https://boxd.it/diary3" || m.Plays != 1 || m.Rating == nil || *m.Rating != 5 {
		t.Errorf("Copper Fields, in the diary only: %+v", m)
	}
	if m := names["Glass Orchard"]; !m.Watchlist || m.Watched || m.Liked || m.AddedAt == nil || !m.AddedAt.Equal(noon("2024-01-02")) {
		t.Errorf("Glass Orchard: %+v", m)
	}
	if m := names["Lantern Bay"]; !m.Liked || !m.Watched {
		t.Errorf("Lantern Bay: %+v", m)
	}

	for name, files := range map[string]map[string]string{
		"no watched.csv nor diary.csv": {"profile.csv": sampleExport["profile.csv"], "ratings.csv": sampleExport["ratings.csv"]},
		"a watched.csv without names":  {"watched.csv": "Date,Title,Year\n2020-01-01,Somewhere,2000\n"},
		"too deep":                     {"a/b/watched.csv": sampleExport["watched.csv"]},
	} {
		if _, err := Parse(exportZip(t, files)); !errors.Is(err, ErrInvalidExport) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Parse([]byte("Date,Name,Year\n")); !errors.Is(err, ErrInvalidExport) {
		t.Errorf("not a ZIP: %v", err)
	}
}
