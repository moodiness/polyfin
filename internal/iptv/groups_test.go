package iptv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// A group's episode identifier names its leader, whatever it holds, and
// its numbers.
func TestGroupEpisodeIdentifiersAreReadBack(t *testing.T) {
	t.Parallel()
	source := accounts.ID{1}
	for _, leader := range []string{"81", "a:b", "x"} {
		id := groupEpisodeID(source, leader, episodeNumbers{2, 13})
		rest, ok := strings.CutPrefix(id, prefix(source)+"gep:")
		got, season, episode, parsed := groupEpisodeParts(rest)
		if !ok || !parsed || got != leader || season != 2 || episode != 13 {
			t.Errorf("%q: %q %d %d %v", id, got, season, episode, parsed)
		}
	}
	for _, rest := range []string{"", "81", "81:1", ":1:2", "81:x:2", "81:1:-2"} {
		if _, _, _, ok := groupEpisodeParts(rest); ok {
			t.Errorf("%q read", rest)
		}
	}
}

// A group's episodes are matched by numbers: each once, from the first
// entry that has it; an episode without a number, or a second of the
// same numbers in one entry, keeps its entry's identifier.
func TestGroupEpisodesAreMatchedByNumber(t *testing.T) {
	t.Parallel()
	source := accounts.ID{1}
	first := groupMember{title: Title{Key: "81"}, details: &details{Episodes: []episodeDetails{
		{Key: "a1", Season: 1, Episode: 1, Title: "First"}, {Key: "a2", Season: 1, Episode: 2}, {Key: "a2b", Season: 1, Episode: 2}}}}
	second := groupMember{title: Title{Key: "82"}, details: &details{Episodes: []episodeDetails{
		{Key: "b3", Season: 1, Episode: 3}, {Key: "b1", Season: 1, Episode: 1, Title: "Other"}, {Key: "b0", Season: 1}, {Key: "c1", Season: 2, Episode: 1}}}}
	var got []string
	for _, v := range groupVideos(source, "81", []groupMember{first, second, {title: Title{Key: "83"}}}) {
		got = append(got, fmt.Sprintf("%dx%d %s %s", v.Season, v.Episode, v.ID[len(prefix(source)):], v.Title))
	}
	want := []string{"1x0 ep:82:b0 ", "1x1 gep:81:1:1 First", "1x2 gep:81:1:2 ", "1x2 ep:81:a2b ", "1x3 gep:81:1:3 ", "2x1 gep:81:2:1 "}
	if !slices.Equal(got, want) {
		t.Errorf("videos\n%q\nwant\n%q", got, want)
	}
}

// Versions are told apart by quality and category, then by name, then by
// the provider's identifier.
func TestGroupVersionsAreLabelledApart(t *testing.T) {
	t.Parallel()
	versions := []groupVersion{
		{title: Title{Key: "1", Name: "Dune", Category: "4K"}, quality: "4K"},
		{title: Title{Key: "2", Name: "Dune", Category: "Movies"}, quality: "FHD"},
		{title: Title{Key: "3", Name: "Dune Part One", Category: "Movies"}, quality: "FHD"},
		{title: Title{Key: "4", Name: "Dune", Category: ""}, quality: ""},
		{title: Title{Key: "5", Name: "Dune", Category: ""}, quality: ""},
	}
	var got []string
	for _, s := range labelled(versions) {
		got = append(got, s.Description)
	}
	want := []string{"4K · 4K", "FHD · Movies · Dune", "FHD · Movies · Dune Part One", "#4", "#5"}
	if !slices.Equal(got, want) {
		t.Errorf("labels %q, want %q", got, want)
	}
}

// groupsProvider is an Xtream account (user / secret) of movies, each by
// its key with its name, category and TMDB id, which may change.
type groupsProvider struct {
	url    string
	mu     sync.Mutex
	movies map[string][3]string
	order  []string
}

func (p *groupsProvider) set(key, name, category, tmdb string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.movies[key]; !ok {
		p.order = append(p.order, key)
	}
	p.movies[key] = [3]string{name, category, tmdb}
}

func newGroupsProvider(t *testing.T) *groupsProvider {
	t.Helper()
	p := &groupsProvider{movies: map[string][3]string{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var answer any
		switch r.URL.Query().Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1}}
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "A", "category_name": "4K Movies"}, map[string]any{"category_id": "B", "category_name": "Movies"}}
		case "get_vod_streams":
			p.mu.Lock()
			var movies []any
			for _, key := range p.order {
				m := p.movies[key]
				movies = append(movies, map[string]any{"name": m[0], "stream_id": key, "category_id": m[1], "tmdb": m[2], "container_extension": "mkv"})
			}
			p.mu.Unlock()
			answer = movies
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// A source's groups follow its list and its administrator: listed once,
// in the category of their face; a leader kept apart is its own title
// again, with what users did with the group, while the others form a new
// group, to which what users did with them moves; a title whose TMDB id
// changes leaves its group.
func TestTitleGroupsFollowTheListAndTheAdministrator(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	p := newGroupsProvider(t)
	p.set("51", "EN - Dune (2021) 4K", "A", "438631")
	p.set("52", "EN - Dune (2021) FHD", "B", "438631")
	p.set("53", "EN - Dune (2021) HD", "B", "438631")
	p.set("54", "EN - Arrival (2016)", "B", "329865")
	on := true
	source := must(e.service.Add(ctx, addons.Shared(), NewSource{Name: "Box", Account: Account{Kind: addons.KindXtream, Server: p.url,
		Username: "user", Password: "secret"}, Options: &OptionsPatch{Movies: &on, TitleVersions: &on, VODLibraries: new(LibrariesByGroup)}},
		false)).ID
	listed := func(category string) []string {
		t.Helper()
		metas, _ := must2(e.service.CatalogWindow(ctx, source, typeMovie, categoryCatalogID(typeMovie, category), "", "", 0, 10))
		var ids []string
		for _, m := range metas {
			ids = append(ids, m.ID[len(prefix(source)):]+" "+m.Name)
		}
		return ids
	}
	if a, b := listed("4K Movies"), listed("Movies"); !slices.Equal(a, []string{"vod:51 Dune"}) || !slices.Equal(b, []string{"vod:54 Arrival"}) {
		t.Errorf("catalogs: %q and %q", a, b)
	}
	if c := must(e.service.vodCounts(ctx, source)); c.Movies != 4 || c.ShownMovies != 2 {
		t.Errorf("counts: %+v", c)
	}
	streams := must(e.service.Streams(ctx, source, titleID(source, typeMovie, "51")))
	var labels []string
	for _, s := range streams {
		labels = append(labels, s.Description)
	}
	if !slices.Equal(labels, []string{"4K · 4K Movies", "FHD · Movies", "HD · Movies"}) {
		t.Errorf("streams: %q", labels)
	}
	if s, err := e.service.Streams(ctx, source, titleID(source, typeMovie, "52")); len(s) != 0 || err != nil {
		t.Errorf("a member's own streams: %+v %v", s, err)
	}
	if _, err := e.service.Meta(ctx, source, titleID(source, typeMovie, "53")); err != stremio.ErrNotFound {
		t.Errorf("a member's own description: %v", err)
	}
	if id := must(e.service.Grouped(ctx, source, titleID(source, typeMovie, "53"))); id != titleID(source, typeMovie, "51") {
		t.Errorf("a member is grouped as %q", id)
	}

	// What a user did with the group stays with the leader kept apart;
	// what they did with a member moves to the new group.
	member := must(e.users.CreateUser(ctx, accounts.NewUser{Name: "member", Password: "correct horse"}))
	store := userdata.New(e.service.db)
	at := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	item := func(key string) accounts.ID { return titleItem(source, typeMovie, key).ID }
	_ = must(store.Change(ctx, member.ID, []userdata.Item{{ID: item("51")}}, func(d *userdata.Data) { d.MarkPlayed(&at, at) }))
	_ = must(store.Change(ctx, member.ID, []userdata.Item{{ID: item("53")}}, func(d *userdata.Data) { d.Favorite = true }))
	if err := e.service.KeepApart(ctx, source, typeMovie, "51", true); err != nil {
		t.Fatal(err)
	}
	if a, b := listed("4K Movies"), listed("Movies"); !slices.Equal(a, []string{"vod:51 Dune"}) || !slices.Equal(b, []string{"vod:52 Dune", "vod:54 Arrival"}) {
		t.Errorf("catalogs once the leader is kept apart: %q and %q", a, b)
	}
	data := must(store.Get(ctx, member.ID, []accounts.ID{item("51"), item("52")}))
	if !data[item("51")].Played || data[item("52")].Played || !data[item("52")].Favorite {
		t.Errorf("data once the leader is kept apart: %+v", data)
	}
	_, _, entries := must3(e.service.TitleEntries(ctx, titleID(source, typeMovie, "52")))
	var keys []string
	for _, entry := range entries {
		keys = append(keys, fmt.Sprintf("%s %v %v %v", entry.Key, entry.Leader, entry.Member, entry.Apart))
	}
	if !slices.Equal(keys, []string{"52 true true false", "53 false true false", "51 false false true"}) {
		t.Errorf("entries: %q", keys)
	}

	// Another TMDB id: the title leaves its group, which lasts with its
	// leader alone.
	p.set("53", "EN - Dune (2021) HD", "B", "1")
	if err := e.service.Refresh(ctx, addons.Shared(), source, false); err != nil {
		t.Fatal(err)
	}
	if b := listed("Movies"); !slices.Equal(b, []string{"vod:52 Dune", "vod:53 Dune", "vod:54 Arrival"}) {
		t.Errorf("catalog once a TMDB id changed: %q", b)
	}
	if s := must(e.service.Streams(ctx, source, titleID(source, typeMovie, "52"))); len(s) != 1 {
		t.Errorf("the group's streams: %+v", s)
	}
}

// A group ends once its first entry shows under another TMDB id, as when
// it is kept apart: each entry is its own title again, the first keeping
// what users did with the group. A group none of whose entries is shown
// lasts, so that they come back under its identifiers.
func TestTitleGroupsEndWithTheirFirstEntrysTMDBID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	p := newGroupsProvider(t)
	p.set("61", "EN - Orb Tale 4K", "A", "77")
	p.set("62", "EN - Orb Tale FHD", "B", "77")
	on := true
	source := must(e.service.Add(ctx, addons.Shared(), NewSource{Name: "Box", Account: Account{Kind: addons.KindXtream, Server: p.url,
		Username: "user", Password: "secret"}, Options: &OptionsPatch{Movies: &on, TitleVersions: &on}}, false)).ID
	listed := func() []string {
		t.Helper()
		metas, _ := must2(e.service.CatalogWindow(ctx, source, typeMovie, typeCatalogID(typeMovie), "", "", 0, 10))
		var ids []string
		for _, m := range metas {
			ids = append(ids, m.ID[len(prefix(source)):])
		}
		slices.Sort(ids)
		return ids
	}
	refresh := func() {
		t.Helper()
		if err := e.service.Refresh(ctx, addons.Shared(), source, false); err != nil {
			t.Fatal(err)
		}
	}
	if ids := listed(); !slices.Equal(ids, []string{"vod:61"}) {
		t.Fatalf("grouped: %q", ids)
	}

	// None shown: the group lasts, and takes its entries back as they were.
	p.set("61", "EN - Orb Tale 4K", "C", "77")
	p.set("62", "EN - Orb Tale FHD", "C", "77")
	refresh()
	if err := e.service.Update(ctx, addons.Shared(), source, Changes{Options: &OptionsPatch{VODExcluded: &[]string{"movie:"}}}, false); err != nil {
		t.Fatal(err)
	}
	if ids := listed(); len(ids) != 0 {
		t.Fatalf("nothing shown: %q", ids)
	}
	p.set("61", "EN - Orb Tale 4K", "B", "77")
	refresh()
	if ids := listed(); !slices.Equal(ids, []string{"vod:61"}) {
		t.Errorf("shown again: %q", ids)
	}

	// The first entry under another TMDB id: each is its own title.
	p.set("61", "EN - Orb Tale 4K", "B", "78")
	p.set("62", "EN - Orb Tale FHD", "B", "77")
	refresh()
	if ids := listed(); !slices.Equal(ids, []string{"vod:61", "vod:62"}) {
		t.Errorf("the first entry's TMDB id changed: %q", ids)
	}
	if s := must(e.service.Streams(ctx, source, titleID(source, typeMovie, "61"))); len(s) != 1 {
		t.Errorf("the first entry's streams: %+v", s)
	}
	_, _, entries := must3(e.service.TitleEntries(ctx, titleID(source, typeMovie, "62")))
	if len(entries) != 1 || entries[0].Member || !entries[0].Listed {
		t.Errorf("the other entry: %+v", entries)
	}
}

func must3[T, U, V any](t T, u U, v V, err error) (T, U, V) {
	if err != nil {
		panic(err)
	}
	return t, u, v
}
