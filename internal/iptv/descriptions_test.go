package iptv

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// describedShows adds an Xtream source of movies and count series, and
// returns it with its series' identifiers, in the provider's order.
func describedShows(t *testing.T, e env, count int) (*syntheticServer, accounts.ID, []string) {
	t.Helper()
	server, account := syntheticVOD(t, 3, count)
	on := true
	source := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Box", Account: account,
		Options: &OptionsPatch{Movies: &on, Series: &on}}, false)).ID
	ids := make([]string, len(server.data.series))
	for i, show := range server.data.series {
		ids[i] = titleID(source, typeSeries, fmt.Sprint(show.ID))
	}
	return server, source, ids
}

// A series' description follows whatever changes it, kept or not: its
// details stored when it is opened, a refresh that renames it, its
// category excluded then shown again, series turned off then on, which
// forgets its details, and the source's enrichment. Opening it again while
// its details are fresh asks the provider nothing. What a caller gets is
// its own to change.
func TestDescriptionsFollowTheirSource(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	server, source, ids := describedShows(t, e, 3)
	show := server.data.series[0]
	name, _ := titleName(show.Name)
	describe := func(opened bool) stremio.Meta {
		t.Helper()
		c := ctx
		if opened {
			c = Opening(ctx)
		}
		return must(e.service.Meta(c, source, ids[0]))
	}

	listed := describe(false)
	if listed.Name != name || len(listed.Videos) != 0 || listed.Description != "A show of the provider." || len(listed.Cast) == 0 {
		t.Fatalf("a series from its listing: %+v", listed)
	}
	listed.Cast[0], listed.Genres[0] = "Changed", "Changed"
	if again := describe(false); again.Cast[0] == "Changed" || again.Genres[0] == "Changed" {
		t.Errorf("a caller's change kept: %+v", again)
	}

	opened := describe(true)
	episodes := show.Seasons * show.Episodes
	if len(opened.Videos) != episodes || opened.Description != "A show of the provider, in detail." || server.count("get_series_info") != 1 {
		t.Fatalf("an opened series: %d episodes, %q, %d details asked", len(opened.Videos), opened.Description, server.count("get_series_info"))
	}
	if listed := describe(false); len(listed.Videos) != episodes || listed.Description != opened.Description {
		t.Errorf("listed after its details were stored: %d episodes, %q", len(listed.Videos), listed.Description)
	}
	opened.Videos[0].Title, opened.Extras.SeasonPosterByNumber["1"] = "Changed", "Changed"
	if again := describe(true); again.Videos[0].Title == "Changed" || again.Extras.SeasonPosterByNumber["1"] == "Changed" ||
		server.count("get_series_info") != 1 {
		t.Errorf("opened again: %q, %q, %d details asked", again.Videos[0].Title, again.Extras.SeasonPosterByNumber["1"], server.count("get_series_info"))
	}

	// Renamed by the provider: listed under its new name, its details
	// kept; opened, asked again.
	server.mu.Lock()
	server.data.series[0].Name, server.data.series[0].Modified = "Zorblat Renamed", show.Modified+1
	server.mu.Unlock()
	if err := e.service.Refresh(ctx, shared, source, false); err != nil {
		t.Fatal(err)
	}
	if listed := describe(false); listed.Name != "Zorblat Renamed" || len(listed.Videos) != episodes {
		t.Errorf("renamed: %q, %d episodes", listed.Name, len(listed.Videos))
	}
	if _ = describe(true); server.count("get_series_info") != 2 {
		t.Errorf("a renamed series' details asked %d times", server.count("get_series_info"))
	}

	// Its category excluded, then shown again.
	excluded := []string{"series:" + server.data.seriesGroups[show.Group]}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{VODExcluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Meta(ctx, source, ids[0]); !errors.Is(err, stremio.ErrNotFound) {
		t.Errorf("an excluded series described: %v", err)
	}
	none := []string{}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{VODExcluded: &none}}, false); err != nil {
		t.Fatal(err)
	}
	if listed := describe(false); listed.Name != "Zorblat Renamed" {
		t.Errorf("shown again: %q", listed.Name)
	}

	// Series off, then on: the details of the titles gone are gone.
	off, on := false, true
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Series: &off}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Meta(ctx, source, ids[0]); !errors.Is(err, stremio.ErrNotFound) {
		t.Errorf("a series described with series off: %v", err)
	}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Series: &on}}, false); err != nil {
		t.Fatal(err)
	}
	if listed := describe(false); len(listed.Videos) != 0 || listed.Description != "A show of the provider." {
		t.Errorf("series on again: %d episodes, %q", len(listed.Videos), listed.Description)
	}

	// The enrichment follows its toggle, in a request checking its
	// sources once too.
	for _, enriched := range []bool{true, false} {
		if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Enrichment: &enriched}}, false); err != nil {
			t.Fatal(err)
		}
		if got, once := e.service.Enrichment(ctx, source), e.service.Enrichment(CheckOnce(ctx), source); got != enriched || once != enriched {
			t.Errorf("enrichment %v: %v, checked once %v", enriched, got, once)
		}
	}
}

// An M3U series' episodes follow its playlist.
func TestM3UDescriptionsFollowTheirPlaylist(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	list := newPlaylistServer(t, vodPlaylist)
	on := true
	source := must(e.service.Add(ctx, shared, NewSource{Name: "List", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Movies: &on, Series: &on}}, false)).ID
	show := must(e.service.Catalog(ctx, source, "series", "series.search", 0, "", "pif tales"))
	if len(show) != 1 {
		t.Fatalf("series found: %+v", show)
	}
	if meta := must(e.service.Meta(ctx, source, show[0].ID)); len(meta.Videos) != 2 {
		t.Fatalf("M3U episodes: %+v", meta.Videos)
	}
	list.set(vodPlaylist+"#EXTINF:-1 group-title=\"Series | Kids\",Pif Tales\nhttps://tv.example/series/user/pass/9103.mkv\n", 0)
	if err := e.service.Refresh(ctx, shared, source, false); err != nil {
		t.Fatal(err)
	}
	if meta := must(e.service.Meta(ctx, source, show[0].ID)); len(meta.Videos) != 3 {
		t.Errorf("M3U episodes after the playlist changed: %+v", meta.Videos)
	}
}

// A description kept is described again without reading it: one
// statement checks its source, once for all of them in a request marked
// by CheckOnce. The descriptions kept stay within their bound in entries,
// the least recently used evicted first, and in bytes.
func TestDescriptionsAreKeptWithinTheirBound(t *testing.T) {
	t.Parallel()
	statements := &statementCounter{}
	e := tracedEnv(t, statements)
	ctx := t.Context()
	server, source, ids := describedShows(t, e, 4)
	e.service.descriptions.once.Do(func() { e.service.descriptions.kept = newDescriptionCache(3, 1<<30) })
	sent := func(describe func()) int64 {
		t.Helper()
		before := statements.statements.Load()
		describe()
		return statements.statements.Load() - before
	}
	for _, id := range ids[:3] {
		if n := sent(func() { _ = must(e.service.Meta(ctx, source, id)) }); n != 2 {
			t.Errorf("a description read: %d statements", n)
		}
	}
	for _, id := range ids[:3] {
		if n := sent(func() { _ = must(e.service.Meta(ctx, source, id)) }); n != 1 {
			t.Errorf("a description kept: %d statements", n)
		}
	}
	once := CheckOnce(ctx)
	if n := sent(func() {
		for _, id := range ids[:3] {
			_ = must(e.service.Meta(once, source, id))
			_ = e.service.Enrichment(once, source)
		}
	}); n != 1 {
		t.Errorf("three descriptions kept, checked once: %d statements", n)
	}

	// A fourth evicts the least recently used, the first.
	_ = must(e.service.Meta(ctx, source, ids[3]))
	if kept := e.service.descriptions.cache().Len(); kept != 3 {
		t.Errorf("%d descriptions kept, at most 3", kept)
	}
	if n := sent(func() { _ = must(e.service.Meta(ctx, source, ids[0])) }); n != 2 {
		t.Errorf("the description evicted: %d statements", n)
	}

	// Bounded in bytes: two opened series, which fit each alone, not
	// together.
	var sizes []int
	for i := range 2 {
		_ = must(e.service.Meta(Opening(ctx), source, ids[i]))
		sizes = append(sizes, descriptionSize(must(e.service.description(ctx, source, typeSeries, fmt.Sprint(server.data.series[i].ID)))))
	}
	bound := max(sizes[0], sizes[1]) + min(sizes[0], sizes[1])/2
	bounded := newEnv(t)
	_, other, otherIDs := describedShows(t, bounded, 4)
	bounded.service.descriptions.once.Do(func() { bounded.service.descriptions.kept = newDescriptionCache(100, bound) })
	for _, id := range otherIDs[:2] {
		_ = must(bounded.service.Meta(Opening(ctx), other, id))
		_ = must(bounded.service.Meta(ctx, other, id))
	}
	if kept, bytes := bounded.service.descriptions.cache().Len(), bounded.service.descriptions.cache().Bytes(); kept != 1 || bytes > bound {
		t.Errorf("%d descriptions kept in %d bytes, at most %d; sizes %v", kept, bytes, bound, sizes)
	}
}

// A description's size counts its episodes and its texts.
func TestDescriptionSizeCountsEpisodes(t *testing.T) {
	t.Parallel()
	short := &description{title: Title{Type: typeSeries, Key: "1", Name: "Show"}}
	long := &description{title: short.title, details: &details{Plot: "A plot.", Episodes: slices.Repeat([]episodeDetails{{Key: "100", Title: "Part one", Plot: "Episode 1."}}, 2000)}}
	if s, l := descriptionSize(short), descriptionSize(long); s <= 0 || l < s+2000*150 || l > s+2000*300 {
		t.Errorf("sizes: %d without details, %d with 2,000 episodes", s, l)
	}
}
