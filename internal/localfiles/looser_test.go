package localfiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// candidate is a title a search found: its name and year, its identifier
// numbered after its place.
type candidate struct {
	name string
	year int
}

// chosen tells what chooseItem keeps of titles for a file named title of
// year: "<identifier> by <how>", or the reason none is kept.
func chosen(title string, year int, exact bool, titles ...candidate) string {
	results := make([]library.RemoteResult, 0, len(titles))
	for i, t := range titles {
		id := fmt.Sprintf("tt%07d", i+1)
		results = append(results, library.RemoteResult{Item: library.Item{Kind: library.KindMovie, StremioID: id, Name: t.name,
			ProductionYear: t.year, ProviderIDs: map[string]string{"Imdb": id}}})
	}
	item, by, reason := chooseItem(library.KindMovie, title, year, results, exact)
	if reason != "" {
		return reason
	}
	return item.StremioID + " by " + by
}

// When no title of the very name and year is found, the one title of the
// year before or after is kept, flagged; two such titles keep none, nor
// does a year two apart, nor a unit matched by the exact rule only.
func TestYearOffByOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		year   int
		exact  bool
		titles []candidate
		want   string
	}{
		{"one title of the year after", 2011, false, []candidate{{"Harbor Lights", 2012}}, "tt0000001 by year"},
		{"one title of the year before", 2011, false, []candidate{{"Harbor Lights", 2010}, {"Harbor Lights", 1980}}, "tt0000001 by year"},
		{"titles of the years before and after", 2011, false, []candidate{{"Harbor Lights", 2010}, {"Harbor Lights", 2012}}, reasonAmbiguous},
		{"a title two years apart", 2011, false, []candidate{{"Harbor Lights", 2013}}, reasonOtherYear},
		{"the exact rule only", 2011, true, []candidate{{"Harbor Lights", 2012}}, reasonOtherYear},
		{"a title without a year", 2011, false, []candidate{{"Harbor Lights", 0}}, reasonOtherYear},
		{"a name without a year", 0, false, []candidate{{"Harbor Lights", 2012}}, "tt0000001 by search"},
	} {
		if got := chosen("Harbor Lights", tc.year, tc.exact, tc.titles...); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// Roman and Arabic numerals, and numbers written in words after "Part",
// are read alike in the file's name and in the titles found, both ways,
// and with a year one off too.
func TestNumeralsReadAlike(t *testing.T) {
	for _, tc := range []struct {
		file  string
		year  int
		title candidate
		want  string
	}{
		{"Harbor Lights 2", 2011, candidate{"Harbor Lights II", 2011}, "tt0000001 by numerals"},
		{"Harbor Lights II", 2011, candidate{"Harbor Lights 2", 2011}, "tt0000001 by numerals"},
		{"Harbor Lights XIV", 0, candidate{"Harbor Lights 14", 1999}, "tt0000001 by numerals"},
		{"Tidewater Part 1", 2011, candidate{"Tidewater: Part One", 2011}, "tt0000001 by numerals"},
		{"Tidewater - Part One", 2011, candidate{"Tidewater Part I", 2011}, "tt0000001 by numerals"},
		{"Tidewater Part III The End", 2011, candidate{"Tidewater: Part Three - The End", 2011}, "tt0000001 by numerals"},
		{"Harbor Lights V", 2011, candidate{"Harbor Lights 5", 2011}, "tt0000001 by numerals"},
		{"Harbor Lights 2", 2011, candidate{"Harbor Lights II", 2012}, "tt0000001 by year_numerals"},
		{"Harbor Lights 2", 2011, candidate{"Harbor Lights II", 2013}, reasonOtherYear},
		{"Harbor Lights 3", 2011, candidate{"Harbor Lights II", 2011}, reasonNotFound},
	} {
		if got := chosen(tc.file, tc.year, false, tc.title); got != tc.want {
			t.Errorf("%s (%d) for %+v: %s, want %s", tc.file, tc.year, tc.title, got, tc.want)
		}
		if got := chosen(tc.file, tc.year, true, tc.title); got != reasonNotFound && got != reasonOtherYear {
			t.Errorf("%s (%d) for %+v by the exact rule: %s", tc.file, tc.year, tc.title, got)
		}
	}
}

// Words that look like numerals are no numbers: a title of a single word
// such as "I", "X" or "XXX", a letter that starts a title, words written
// with other Roman letters, and numbers written in words elsewhere than
// after "Part".
func TestNumeralsFalseFriends(t *testing.T) {
	for _, tc := range []struct{ file, title string }{
		{"1", "I"},
		{"10", "X"},
		{"30", "XXX"},
		{"1 Robot", "I Robot"},
		{"5 for Vendetta", "V for Vendetta"},
		{"The 1009", "The Mix"},
		{"Liv and Maddie", "54 and Maddie"},
		{"Dix Hills", "509 Hills"},
		{"One Day", "1 Day"},
		{"Harbor Lights Two", "Harbor Lights 2"},
	} {
		if got := chosen(tc.file, 2011, false, candidate{tc.title, 2011}); got != reasonNotFound {
			t.Errorf("%s for %s: %s", tc.file, tc.title, got)
		}
	}
}

// A title of the very name and year always wins over those a looser rule
// keeps, and a looser rule keeps a title only when it is the only one left:
// the year one off and the numerals are tried together, both only after.
func TestExactBeforeLooser(t *testing.T) {
	for _, tc := range []struct {
		name   string
		file   string
		titles []candidate
		want   string
	}{
		{"the very year wins", "Harbor Lights", []candidate{{"Harbor Lights", 2012}, {"Harbor Lights", 2011}}, "tt0000002 by search"},
		{"the very name wins", "Harbor Lights 2", []candidate{{"Harbor Lights II", 2011}, {"Harbor Lights 2", 2011}}, "tt0000002 by search"},
		{"several exact ones stay ambiguous", "Harbor Lights", []candidate{{"Harbor Lights", 2011}, {"Harbor Lights", 2011}, {"Harbor Lights", 2012}},
			reasonAmbiguous},
		{"a year off and numerals at once", "Harbor Lights 2", []candidate{{"Harbor Lights 2", 2012}, {"Harbor Lights II", 2011}}, reasonAmbiguous},
		{"one rule before both", "Harbor Lights 2", []candidate{{"Harbor Lights II", 2012}, {"Harbor Lights 2", 2010}}, "tt0000002 by year"},
	} {
		if got := chosen(tc.file, 2011, false, tc.titles...); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// looserTitles are the titles the fake addon of the looser rules knows.
var looserTitles = []stremio.Meta{
	{ID: "tt1000001", Type: "movie", Name: "Harbor Lights", Year: "2012"},
	{ID: "tt1000002", Type: "movie", Name: "Tidewater: Part One", Year: "2015"},
	{ID: "tt1000003", Type: "movie", Name: "Gull Point II", Year: "2009"},
	{ID: "tt1000004", Type: "movie", Name: "Night Ferry", Year: "2000"},
	{ID: "tt1000005", Type: "movie", Name: "Night Ferry", Year: "2002"},
	{ID: "tt1000006", Type: "movie", Name: "Quay Street", Year: "1999"},
	{ID: "tt1000007", Type: "movie", Name: "Quay Street", Year: "2000"},
	{ID: "tt1000008", Type: "series", Name: "Lantern Coast", Year: "2012–2014"},
}

// newLooserAddon is a metadata addon whose movie and series searches find
// the looserTitles of the kind whose name starts with the term's first
// word, as addons find titles whose numerals or punctuation differ.
func newLooserAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			search := []stremio.Extra{{Name: "search", IsRequired: true}}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta.looser", Name: "Meta", Version: "1", Types: []string{"movie", "series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: search}, {Type: "series", ID: "top", Name: "Top", Extra: search}}})
		case strings.HasPrefix(path, "/catalog/"):
			typ, query, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/"), ".json"), "/top/search=")
			term, _ := url.PathUnescape(query)
			first, _, _ := strings.Cut(strings.ToLower(term), " ")
			metas := []stremio.Meta{}
			for _, meta := range looserTitles {
				if meta.Type == typ && first != "" && strings.HasPrefix(strings.ToLower(meta.Name), first) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/"):
			id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
			for _, meta := range looserTitles {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Files a looser rule matches are matched without anyone acting, counted
// apart in the folder's figures by rule and listed with their rule; an
// unlink leaves them to the exact rule, through rescans, until a link.
func TestLooserMatchesAreFlaggedAndUnlinked(t *testing.T) {
	t.Parallel()
	e := newEnvWith(t, newLooserAddon(t))
	ctx := t.Context()
	movies, _ := e.folder(KindMovies, map[string]int{
		"Harbor Lights (2011).mkv":      10,
		"Tidewater Part 1 (2015).mkv":   10,
		"Gull Point 2 (2008)/movie.mkv": 10,
		"Night Ferry (2001).mkv":        10,
		"Quay Street (1999).mkv":        10,
	})
	want := map[string]string{
		"Harbor Lights (2011).mkv":      "tt1000001 by year",
		"Tidewater Part 1 (2015).mkv":   "tt1000002 by numerals",
		"Gull Point 2 (2008)/movie.mkv": "tt1000003 by year_numerals",
		"Night Ferry (2001).mkv":        "unmatched: ambiguous",
		"Quay Street (1999).mkv":        "tt1000006 by search",
	}
	if got := e.matches(movies.ID); !maps.Equal(got, want) {
		t.Errorf("matched: %v", got)
	}
	folder, err := e.service.Folder(ctx, movies.ID)
	if err != nil || folder.MatchedBy != (Matches{Search: 1, Year: 1, Numerals: 1, YearNumerals: 1}) || folder.Matched != 4 {
		t.Errorf("figures: %+v %v", folder.MatchedBy, err)
	}
	looser, total, err := e.service.Looser(ctx, movies.ID)
	if err != nil || total != 3 || len(looser) != 3 {
		t.Fatalf("looser: %d %+v %v", total, looser, err)
	}
	if l := looser[1]; l.Unit != "Harbor Lights (2011).mkv" || l.By != byYear || l.Title != "Harbor Lights" || l.Year != 2011 ||
		l.StremioID != "tt1000001" || l.Name != "Harbor Lights" || l.TitleYear != 2012 || l.Files != 1 {
		t.Errorf("flagged: %+v", l)
	}

	// Unlinked, a file is matched by the exact rule only, and stays so
	// through rescans; a file the exact rule matched has nothing to unlink.
	if err := e.service.Unlink(ctx, movies.ID, "Harbor Lights (2011).mkv"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(ctx, movies.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.matches(movies.ID)["Harbor Lights (2011).mkv"]; got != "unmatched: other_year" {
		t.Errorf("unlinked: %s", got)
	}
	if err := e.service.Unlink(ctx, movies.ID, "Quay Street (1999).mkv"); !errors.Is(err, addons.ErrNotFound) {
		t.Errorf("unlinking an exact match: %v", err)
	}
	if folder, _ := e.service.Folder(ctx, movies.ID); folder.MatchedBy != (Matches{Search: 1, Numerals: 1, YearNumerals: 1}) {
		t.Errorf("figures once unlinked: %+v", folder.MatchedBy)
	}
	// A link wins; removed, the file is left to the exact rule again.
	if err := e.service.Link(ctx, movies.ID, "Harbor Lights (2011).mkv", "tt1000001"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Unlink(ctx, movies.ID, "Harbor Lights (2011).mkv"); err != nil {
		t.Fatal(err)
	}
	if got := e.matches(movies.ID)["Harbor Lights (2011).mkv"]; got != "unmatched: other_year" {
		t.Errorf("link removed: %s", got)
	}

	// A show's folder is unlinked with every episode.
	shows, _ := e.folder(KindShows, map[string]int{
		"Lantern Coast (2011)/Season 01/Lantern Coast S01E01.mkv": 10,
		"Lantern Coast (2011)/Season 01/Lantern Coast S01E02.mkv": 10,
	})
	if folder, _ := e.service.Folder(ctx, shows.ID); folder.MatchedBy != (Matches{Year: 2}) {
		t.Errorf("show figures: %+v", folder.MatchedBy)
	}
	if looser, _, _ := e.service.Looser(ctx, shows.ID); len(looser) != 1 || looser[0].Unit != "Lantern Coast (2011)" || looser[0].Files != 2 {
		t.Errorf("show flagged: %+v", looser)
	}
	if err := e.service.Unlink(ctx, shows.ID, "Lantern Coast (2011)"); err != nil {
		t.Fatal(err)
	}
	for rel, got := range e.matches(shows.ID) {
		if got != "unmatched: other_year" {
			t.Errorf("%s: %s", rel, got)
		}
	}
}

// A looser rule waits for every addon to answer: one that failed may know
// the title of the very name and year, which would win.
func TestLooserMatchesWaitForFailedSearches(t *testing.T) {
	t.Parallel()
	broken := &searchAddon{name: "Broken", extra: []stremio.Extra{{Name: "search", IsRequired: true}}, answer: failing(http.StatusInternalServerError)}
	e := newEnvWith(t, broken.start(t), newLooserAddon(t))
	folder, _ := e.folder(KindMovies, map[string]int{"Harbor Lights (2011).mkv": 1, "Quay Street (1999).mkv": 1})
	files := e.files(folder.ID)
	if files["Harbor Lights (2011).mkv"] != "unmatched: search_failed" || files["Quay Street (1999).mkv"] != "tt1000006" {
		t.Errorf("files: %v", files)
	}
}
