package library

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// studioXtream is an Xtream account (user / secret) of movies only, in
// two categories, Action (1) and Drama (2), whose list can change.
type studioXtream struct {
	url    string
	mu     sync.Mutex
	movies []map[string]any
}

// xtreamMovie is a movie of an Xtream list, of TMDB identifier tmdb, none
// when 0, the newer the higher its stream identifier.
func xtreamMovie(stream int, name string, tmdb int, category string) map[string]any {
	movie := map[string]any{"name": name, "stream_id": stream, "category_id": category, "container_extension": "mkv",
		"added": fmt.Sprint(1700000000 + stream)}
	if tmdb > 0 {
		movie["tmdb"] = fmt.Sprint(tmdb)
	}
	return movie
}

func newStudioXtream(t *testing.T, movies ...map[string]any) *studioXtream {
	t.Helper()
	p := &studioXtream{movies: movies}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "1", "category_name": "Action"}, map[string]any{"category_id": "2", "category_name": "Drama"}}
		case "get_vod_streams":
			p.mu.Lock()
			answer = slices.Clone(p.movies)
			p.mu.Unlock()
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// set changes the account's movies, for its next refresh.
func (p *studioXtream) set(movies ...map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.movies = movies
}

// studioIndexEnv is a creditsEnv whose service counts its statements and
// lists the movies of an Xtream account, "Box", as a library, besides the
// addon's movies and series: Zeb Quest (TMDB 3001, of North Studio), Orb
// Tale (3002, of Harbor Network), Pax Road (3003, of North Studio and Bay
// Pictures, a Drama), Plain One (of no TMDB identifier) and fillers of
// none, enough for two pages.
type studioIndexEnv struct {
	creditsEnv
	statements *statements
	channels   *iptv.Service
	provider   *studioXtream
	source     accounts.ID
	box        accounts.ID
	// iptvTitles are the Xtream account's titles by name.
	iptvTitles map[string]accounts.ID
}

// studioFillers are the Xtream account's movies of no TMDB identifier, for
// a list longer than a page of Studios' reads (see movieIDsPage).
func studioFillers() []map[string]any {
	var fillers []map[string]any
	for i := range movieIDsPage + 20 {
		fillers = append(fillers, xtreamMovie(100+i, fmt.Sprintf("Filler %d", i), 0, "1"))
	}
	return fillers
}

func newStudioIndexEnv(t *testing.T) studioIndexEnv {
	t.Helper()
	e := studioIndexEnv{creditsEnv: newCreditsEnv(t, "k3y", time.Hour), iptvTitles: map[string]accounts.ID{}}
	e.service.UseTitleCredits(e.queue)
	e.statements = counted(t, e.service)
	e.channels = iptv.New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	e.service.UseIPTV(e.channels)

	north, harbor, bay := studio(71, "North Studio", "/north.png"), studio(73, "Harbor Network", ""), studio(75, "Bay Pictures", "/bay.png")
	e.tmdb.studios("/movie/1001", []any{}, []any{north})
	e.tmdb.studios("/movie/1012", []any{}, []any{north, harbor})
	e.tmdb.studios("/tv/2031", []any{studio(81, "North Studio", "/north-net.png")}, []any{})
	for id, companies := range map[int][]any{3001: {north}, 3002: {harbor}, 3003: {north, bay}, 3004: {bay}} {
		e.tmdb.movie("", id, billed([]int{6000}, nil), nil)
		e.tmdb.studios(fmt.Sprintf("/movie/%d", id), []any{}, companies)
	}
	for _, name := range []string{"Harbor Lights", "Kelp Bay", "Night Shift", "Tidewater"} {
		e.open(e.admin, name)
	}

	e.provider = newStudioXtream(t, append([]map[string]any{xtreamMovie(1, "Zeb Quest", 3001, "1"), xtreamMovie(2, "Orb Tale", 3002, "1"),
		xtreamMovie(3, "Pax Road", 3003, "2"), xtreamMovie(4, "Plain One", 0, "1")}, studioFillers()...)...)
	on, off := true, false
	addon, err := e.channels.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: e.provider.url, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{LiveTv: &off, Movies: &on}}, false)
	if err != nil {
		t.Fatal(err)
	}
	e.source = addon.ID
	libraries, err := e.service.Libraries(t.Context(), e.admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range libraries {
		if l.CollectionType == "movies" && l.Name != "Top" {
			e.box = l.ID
		}
	}
	if e.box == (accounts.ID{}) {
		t.Fatalf("no library of the Xtream account's movies in %v", names(libraries))
	}
	for _, name := range []string{"Zeb Quest", "Orb Tale", "Pax Road"} {
		e.openIPTV(name)
	}
	e.settle()
	return e
}

// openIPTV opens one of the Xtream account's titles, listed first so
// that it is found again, and waits for its credits.
func (e studioIndexEnv) openIPTV(name string) {
	e.t.Helper()
	page, err := e.service.Children(e.t.Context(), e.admin, e.box, 0, 1000, "")
	if err != nil {
		e.t.Fatal(err)
	}
	for _, item := range page.Items {
		e.iptvTitles[item.Name] = item.ID
	}
	item, err := e.service.Item(e.t.Context(), e.admin, e.iptvTitles[name])
	if err != nil {
		e.t.Fatal(err)
	}
	e.service.AwaitTitleCredits(e.t.Context(), item)
}

// studios lists the studios Studios lists, each with its counts and logo.
func (e studioIndexEnv) studios(user accounts.User, parent accounts.ID) []string {
	e.t.Helper()
	studios, err := e.service.Studios(e.t.Context(), user, parent)
	if err != nil {
		e.t.Fatal(err)
	}
	return studioLines(studios)
}

func studioLines(studios []Studio) []string {
	lines := []string{}
	for _, s := range studios {
		lines = append(lines, fmt.Sprintf("%s %d/%d %s", s.Name, s.Movies, s.Series, s.Logo))
	}
	return lines
}

// walked lists the studios as Studios listed them before the studio
// index: from every page of the libraries, read through Children.
func (e studioIndexEnv) walked(user accounts.User, parent accounts.ID) []string {
	e.t.Helper()
	ctx := e.t.Context()
	v, err := e.service.view(ctx, user)
	if err != nil {
		e.t.Fatal(err)
	}
	titles := WithKinds(ctx, func(kind Kind) bool { return kind == KindMovie || kind == KindSeries })
	var items []Item
	for _, l := range v.libraries {
		if l.item.CollectionType != "movies" && l.item.CollectionType != "tvshows" || parent != (accounts.ID{}) && l.item.ID != parent {
			continue
		}
		for start := 0; start < CreditsPassMost; {
			page, err := e.service.Children(titles, user, l.item.ID, start, min(movieIDsPage, CreditsPassMost-start), "")
			if err != nil {
				e.t.Fatal(err)
			}
			for _, item := range page.Items {
				if item.Kind == KindMovie || item.Kind == KindSeries {
					items = append(items, item)
				}
			}
			start += len(page.Items)
			if len(page.Items) == 0 || !page.More && start >= page.Total {
				break
			}
		}
	}
	byName := map[string]*Studio{}
	seen := map[accounts.ID]bool{}
	for i, shown := range e.service.studiosShown(ctx, items) {
		if seen[items[i].ID] {
			continue
		}
		seen[items[i].ID] = true
		counted := map[string]bool{}
		for _, name := range shown {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" || counted[key] {
				continue
			}
			counted[key] = true
			if byName[key] == nil {
				byName[key] = &Studio{Name: strings.TrimSpace(name)}
			}
			if items[i].Kind == KindMovie {
				byName[key].Movies++
			} else {
				byName[key].Series++
			}
		}
	}
	var studios []Studio
	var listed []string
	for _, s := range byName {
		listed = append(listed, s.Name)
	}
	logos, err := e.service.studioLogos(ctx, listed)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, s := range byName {
		s.Logo = logos[strings.ToLower(s.Name)]
		studios = append(studios, *s)
	}
	slices.SortFunc(studios, func(a, b Studio) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	return studioLines(studios)
}

// same checks that Studios lists for user, of parent, what the walk
// through Children lists, twice, and returns it.
func (e studioIndexEnv) same(what string, user accounts.User, parent accounts.ID) []string {
	e.t.Helper()
	got, again := e.studios(user, parent), e.studios(user, parent)
	want := e.walked(user, parent)
	if !slices.Equal(got, want) || !slices.Equal(again, want) {
		e.t.Errorf("%s:\n got %q\nthen %q\nwant %q", what, got, again, want)
	}
	return got
}

// statementsOf returns the statements Studios runs for user, of parent.
func (e studioIndexEnv) statementsOf(user accounts.User, parent accounts.ID) []string {
	e.t.Helper()
	var sqls []string
	for _, s := range e.statements.run(func() { e.studios(user, parent) }) {
		sqls = append(sqls, strings.Join(strings.Fields(s.sql), " "))
	}
	return sqls
}

// statementsWith returns the statements of sqls that contain each of parts.
func statementsWith(sqls []string, parts ...string) []string {
	return slices.DeleteFunc(slices.Clone(sqls), func(sql string) bool {
		return slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(sql, part) })
	})
}

// Studios lists what it listed when it read every page of the libraries
// through Children: for every library, of one library, for a restricted
// user and for a user who hid titles, from the index or not.
func TestStudiosListWhatTheWalkListed(t *testing.T) {
	t.Parallel()
	e := newStudioIndexEnv(t)
	want := []string{"Bay Pictures 1/0 /bay.png", "Harbor Network 2/0 ", "North Studio 4/1 /north-net.png"}
	if got := e.same("every library", e.admin, accounts.ID{}); !slices.Equal(got, want) {
		t.Errorf("every library: %q, want %q", got, want)
	}
	if got := e.same("the Xtream account's library", e.admin, e.box); !slices.Equal(got,
		[]string{"Bay Pictures 1/0 /bay.png", "Harbor Network 1/0 ", "North Studio 2/0 /north-net.png"}) {
		t.Errorf("the Xtream account's library: %q", got)
	}
	e.same("the addon's movies", e.admin, e.library(e.admin, "Top").ID)
	e.same("a member", e.member, accounts.ID{})
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	e.same("a restricted user", child, accounts.ID{})
	for _, hidden := range []accounts.ID{e.iptvTitles["Orb Tale"], e.titles["Kelp Bay"]} {
		if err := e.service.userData.Hide(t.Context(), e.member.ID, hidden, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.same("a member who hid two titles", e.member, accounts.ID{}); slices.Contains(got, "Harbor Network 2/0 ") {
		t.Errorf("a member who hid two titles: %q", got)
	}
}

// The studio index follows the Xtream account's list as it is refreshed
// and its options, the details kept of a title newly opened, and an
// administrator's edit of a title's studios. A listing from the index
// reads the account's version alone; one after a change reads and counts
// its titles once, for every page; neither saves the titles.
func TestTheStudioIndexFollowsChanges(t *testing.T) {
	t.Parallel()
	e := newStudioIndexEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	e.same("first", e.admin, e.box)
	sqls := e.statementsOf(e.admin, e.box)
	if read := statementsWith(sqls, "iptv_titles"); len(read) != 0 {
		t.Errorf("titles read from the index: %q", read)
	}
	if version := statementsWith(sqls, "FROM iptv_sources"); len(version) != 1 {
		t.Errorf("versions read: %q", version)
	}
	if saved := statementsWith(sqls, "INTO items"); len(saved) != 0 {
		t.Errorf("titles saved: %q", saved)
	}

	// Orb Tale leaves the list, Quo Vale (Bay Pictures) joins it.
	e.provider.set(append([]map[string]any{xtreamMovie(1, "Zeb Quest", 3001, "1"), xtreamMovie(3, "Pax Road", 3003, "2"),
		xtreamMovie(4, "Plain One", 0, "1"), xtreamMovie(5, "Quo Vale", 3004, "1")}, studioFillers()...)...)
	if err := e.channels.Refresh(ctx, shared, e.source, false); err != nil {
		t.Fatal(err)
	}
	sqls = e.statementsOf(e.admin, e.box)
	if counts := statementsWith(sqls, "count(*)", "iptv_titles"); len(counts) != 1 {
		t.Errorf("counts of the titles after a refresh: %q", counts)
	}
	if read := statementsWith(sqls, "iptv_titles"); len(read) != 2 {
		t.Errorf("titles read after a refresh: %q", read)
	}
	if saved := statementsWith(sqls, "INTO items"); len(saved) != 0 {
		t.Errorf("titles saved: %q", saved)
	}
	want := []string{"Bay Pictures 1/0 /bay.png", "North Studio 2/0 /north-net.png"}
	if got := e.same("after a refresh", e.admin, e.box); !slices.Equal(got, want) {
		t.Errorf("after a refresh: %q, want %q", got, want)
	}

	// Quo Vale's details are kept once it is opened.
	e.openIPTV("Quo Vale")
	e.settle()
	want = []string{"Bay Pictures 2/0 /bay.png", "North Studio 2/0 /north-net.png"}
	if got := e.same("Quo Vale opened", e.admin, e.box); !slices.Equal(got, want) {
		t.Errorf("Quo Vale opened: %q, want %q", got, want)
	}

	// An administrator sets Zeb Quest's studios.
	base, err := e.service.Original(ctx, e.admin, e.iptvTitles["Zeb Quest"])
	if err != nil {
		t.Fatal(err)
	}
	if err := e.service.SaveOverrides(ctx, base, Overrides{Studios: []string{"Edited Studio"}}); err != nil {
		t.Fatal(err)
	}
	want = []string{"Bay Pictures 2/0 /bay.png", "Edited Studio 1/0 ", "North Studio 1/0 /north-net.png"}
	if got := e.same("Zeb Quest edited", e.admin, e.box); !slices.Equal(got, want) {
		t.Errorf("Zeb Quest edited: %q, want %q", got, want)
	}

	// The Drama category left out takes Pax Road away.
	excluded := []string{"movie:Drama"}
	if err := e.channels.Update(ctx, shared, e.source, iptv.Changes{Options: &iptv.OptionsPatch{VODExcluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	want = []string{"Bay Pictures 1/0 /bay.png", "Edited Studio 1/0 "}
	if got := e.same("Drama left out", e.admin, e.box); !slices.Equal(got, want) {
		t.Errorf("Drama left out: %q, want %q", got, want)
	}
}
