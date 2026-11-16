package jellyfin

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// searchedTitles are the titles of a guide's programmes that searches
// fold: accents, case, compatibility forms, a title-case digraph, a
// letter lower case keeps, punctuation, and LIKE's own characters.
var searchedTitles = []string{"Café Société", "KEPNOR Report", "Dr. Who?", "Ｆｕｌｌｗｉｄｔｈ News", "C'est la vie", "Kepnor_50%",
	"Straße", "Ǆemal", "Back\\slash"}

// A search finds the programmes of an XMLTV guide whose title, folded as
// Polyfin folds names (case and accents do not count, punctuation does),
// holds the term, as it did when it tested every programme's name: the
// same ones, in the order they air, whether their folded title was stored
// with the guide or is folded on the way, as for the programmes stored
// before Polyfin kept it.
func TestProgrammeSearchFoldsTitles(t *testing.T) {
	t.Parallel()
	addon := newTVAddon(t, true, "")
	s, token, _ := tuned(t, addon)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv><channel id="two-x"><display-name>Two</display-name></channel>`)
	for i, title := range searchedTitles {
		start := time.Duration(i+1) * 2 * time.Hour
		fmt.Fprintf(&body, `<programme channel="two-x" start="%s" stop="%s"><title>%s</title></programme>`, at(start), at(start+time.Hour),
			strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(title))
	}
	body.WriteString(`</tv>`)
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body.String()) }))
	t.Cleanup(guide.Close)
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil || len(libraries) != 1 {
		t.Fatal(libraries, err)
	}
	key := addons.LibraryKey{AddonID: libraries[0].AddonID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := s.addons.SetGuide(t.Context(), addons.Shared(), key, guide.URL+"/epg"); err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	var failure string
	if err := s.pool.QueryRow(t.Context(), "SELECT error FROM live_guides").Scan(&failure); err != nil || failure != "" {
		t.Fatalf("the guide was not stored: %q (%v)", failure, err)
	}

	// What each term finds: the titles whose folded form holds the folded
	// term, as the search tested them one by one.
	wanted := func(term string) []string {
		var titles []string
		for _, title := range searchedTitles {
			if strings.Contains(iptv.Fold(title), iptv.Fold(term)) {
				titles = append(titles, title)
			}
		}
		return titles
	}
	pinned := map[string][]string{
		"cafe":      {"Café Société"},
		"SOCIÉTÉ":   {"Café Société"},
		"kepnor":    {"KEPNOR Report", "Kepnor_50%"},
		"dr. who":   {"Dr. Who?"},
		"dr who":    nil,
		"fullwidth": {"Ｆｕｌｌｗｉｄｔｈ News"},
		"ｎｅｗｓ":      {"Ｆｕｌｌｗｉｄｔｈ News"},
		"c'est":     {"C'est la vie"},
		"cest":      nil,
		"_50%":      {"Kepnor_50%"},
		"%":         {"Kepnor_50%"},
		"r_5":       {"Kepnor_50%"},
		"r%":        nil,
		"STRAßE":    {"Straße"},
		"strasse":   nil,
		"dzemal":    {"Ǆemal"},
		`k\s`:       {`Back\slash`},
		`k\`:        {`Back\slash`},
	}
	search := func(term string) []string {
		t.Helper()
		var programs QueryResult
		s.get(t, "/Items?recursive=true&includeItemTypes=LiveTvProgram&searchTerm="+url.QueryEscape(term), token, &programs)
		return programNames(programs)
	}
	check := func(when string) {
		t.Helper()
		for term, pin := range pinned {
			if want := wanted(term); !slices.Equal(want, pin) {
				t.Fatalf("%q: the folding finds %q, the test pins %q", term, want, pin)
			}
			if got := search(term); !slices.Equal(got, pin) {
				t.Errorf("%s, %q found %q, want %q", when, term, got, pin)
			}
		}
	}
	check("folded as stored")
	var folded int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM live_guide_programmes WHERE search IS NOT NULL").Scan(&folded); err != nil ||
		folded != len(searchedTitles) {
		t.Fatalf("%d programmes stored with their folded title, want %d (%v)", folded, len(searchedTitles), err)
	}

	// Programmes stored without it are folded before a search reads them.
	if _, err := s.pool.Exec(t.Context(), "UPDATE live_guide_programmes SET search = NULL"); err != nil {
		t.Fatal(err)
	}
	if got := search("kepnor"); !slices.Equal(got, pinned["kepnor"]) {
		t.Errorf("before folding, kepnor found %q", got)
	}
	rows, err := s.pool.Query(t.Context(), "SELECT title, search FROM live_guide_programmes")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var title string
		var search *string
		if err := rows.Scan(&title, &search); err != nil {
			t.Fatal(err)
		}
		if search == nil || *search != iptv.Fold(title) {
			t.Errorf("%q folded as %v, want %q", title, search, iptv.Fold(title))
		}
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	check("folded on the way")
}

// A search finds the channels whose name, as an administrator may have
// changed it, holds the term, as it did, and records those it finds only:
// the others are recorded when a listing shows them.
func TestChannelSearchRecordsTheChannelsFound(t *testing.T) {
	t.Parallel()
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	search := func(term string) []string {
		t.Helper()
		var found QueryResult
		s.get(t, "/Items?recursive=true&includeItemTypes=LiveTvChannel&searchTerm="+url.QueryEscape(term), token, &found)
		var names []string
		for _, item := range found.Items {
			names = append(names, item.Name)
		}
		return names
	}
	recorded := func() []string {
		t.Helper()
		rows, err := s.pool.Query(t.Context(), "SELECT data->'meta'->>'name' FROM items WHERE kind = 'channel' ORDER BY 1")
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		return names
	}
	if got := search("TW"); !slices.Equal(got, []string{"Two"}) {
		t.Errorf("TW found %q", got)
	}
	if got := recorded(); !slices.Equal(got, []string{"Two"}) {
		t.Errorf("channels recorded by the search: %q, want the one found", got)
	}
	if got := search("o"); !slices.Equal(got, []string{"One", "Two"}) {
		t.Errorf("o found %q", got)
	}

	// A channel an administrator renamed is found by its new name only.
	channels, err := s.library.Channels(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	one := channels[slices.IndexFunc(channels, func(c library.Item) bool { return c.Name == "One" })]
	renamed := "Únited Twin"
	if err := s.library.SaveOverrides(t.Context(), one, library.Overrides{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if got := search("unit"); !slices.Equal(got, []string{"Únited Twin"}) {
		t.Errorf("unit found %q", got)
	}
	if got := search("tw"); !slices.Equal(got, []string{"Únited Twin", "Two"}) {
		t.Errorf("tw found %q", got)
	}
	if got := search("one"); len(got) != 0 {
		t.Errorf("one found %q after the rename", got)
	}
}
