package iptv

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// vodPlaylist is an M3U playlist as panels export them: live channels,
// movies and the episodes of series, by address and by file extension.
const vodPlaylist = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"zeb.zz\" group-title=\"News\",Zeb One\nhttps://tv.example/live/user/pass/1.ts\n" +
	"#EXTINF:-1 group-title=\"News\",Orbe Info\nhttps://tv.example/live/user/pass/2.m3u8\n" +
	"#EXTINF:-1 group-title=\"News\",Plain Channel\nhttps://tv.example/stream/3\n" +
	"#EXTINF:-1 tvg-logo=\"https://img.example/lumo.jpg\" group-title=\"VOD | Action\",FR - Lumo Movie (2018) 4K\nhttps://tv.example/movie/user/pass/7001.mkv\n" +
	"#EXTINF:-1 group-title=\"VOD | Action\",Quill Story - 2020\nhttps://tv.example/movie/user/pass/7002.mp4\n" +
	"#EXTINF:-1 group-title=\"VOD | Kids\",Tac Cartoon (2015)\nhttps://cdn.example/files/tac.cartoon.avi\n" +
	"#EXTINF:-1 group-title=\"Series | Drama\",Zeb Show S01 E01\nhttps://tv.example/series/user/pass/9001.mkv\n" +
	"#EXTINF:-1 group-title=\"Series | Drama\",Zeb Show S01E02 FHD\nhttps://tv.example/series/user/pass/9002.mkv\n" +
	"#EXTINF:-1 group-title=\"Series | Drama\",ZEB SHOW 2x01\nhttps://tv.example/series/user/pass/9003.mkv\n" +
	"#EXTINF:-1 group-title=\"Series | Kids\",Orb Saga - Season 1 Episode 3\nhttps://cdn.example/orb/saga.s1e3.mp4\n" +
	"#EXTINF:-1 group-title=\"Series | Kids\",Pif Tales\nhttps://tv.example/series/user/pass/9101.mkv\n" +
	"#EXTINF:-1 group-title=\"Series | Kids\",Pif Tales\nhttps://tv.example/series/user/pass/9102.mkv\n"

// M3U entries are told apart by their address and name: live channels,
// movies by a /movie/ path or a video file, episodes by a /series/ path or
// numbers in their name; episodes group into series and seasons by name,
// whatever its case and dressing.
func TestM3UEntriesAreClassified(t *testing.T) {
	var entries []Entry
	if err := ParseM3U(strings.NewReader(vodPlaylist), func(e Entry) error { entries = append(entries, e); return nil }); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, fmt.Sprintf("%s|%s|%d|%d", or(e.Kind, "live"), e.Series, e.Season, e.Episode))
	}
	want := []string{"live||0|0", "live||0|0", "live||0|0", "movie||0|0", "movie||0|0", "movie||0|0",
		"episode|Zeb Show|1|1", "episode|Zeb Show|1|2", "episode|ZEB SHOW|2|1", "episode|Orb Saga|1|3", "episode|Pif Tales|0|0", "episode|Pif Tales|0|0"}
	if !slices.Equal(got, want) {
		t.Errorf("kinds\n%q\nwant\n%q", got, want)
	}
	var vod []vodEntry
	for i, e := range entries {
		if e.Kind != "" {
			vod = append(vod, vodEntry{key: fmt.Sprint("k", i), name: e.Name, group: e.Group, url: e.URL, kind: e.Kind, series: e.Series,
				season: e.Season, episode: e.Episode})
		}
	}
	var movies []string
	for _, m := range m3uMovies(slices.DeleteFunc(slices.Clone(vod), func(e vodEntry) bool { return e.kind != KindMovie })) {
		movies = append(movies, fmt.Sprintf("%s|%s|%d|%s|%s", m.Key, m.Name, m.Year, m.Quality, m.Category))
	}
	if want := []string{"7001|Lumo Movie|2018|4K|VOD | Action", "7002|Quill Story|2020||VOD | Action", "k5|Tac Cartoon|2015||VOD | Kids"}; !slices.Equal(movies, want) {
		t.Errorf("movies\n%q\nwant\n%q", movies, want)
	}
	series, episodes := m3uSeries(slices.DeleteFunc(slices.Clone(vod), func(e vodEntry) bool { return e.kind != KindEpisode }))
	var shows []string
	for _, s := range series {
		shows = append(shows, s.Name+"|"+s.Category)
	}
	if want := []string{"Zeb Show|Series | Drama", "Orb Saga|Series | Kids", "Pif Tales|Series | Kids"}; !slices.Equal(shows, want) {
		t.Errorf("series %q, want %q", shows, want)
	}
	var numbers []string
	for _, e := range episodes {
		numbers = append(numbers, fmt.Sprintf("%s:%dx%d:%s", e.key, e.season, e.episode, e.quality))
	}
	if want := []string{"9001:1x1:", "9002:1x2:FHD", "9003:2x1:", "k9:1x3:", "9101:1x1:", "9102:1x2:"}; !slices.Equal(numbers, want) {
		t.Errorf("episodes %q, want %q", numbers, want)
	}
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// With the default options a source imports exactly what it did before
// movies and series: the same manifest, and every entry as a channel,
// movies and episodes included.
func TestDefaultOptionsKeepTodaysSource(t *testing.T) {
	e := newEnv(t)
	list := newPlaylistServer(t, vodPlaylist)
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false))
	only := []string{prefix(addon.ID)}
	before := stremio.Manifest{ID: "polyfin.iptv." + addon.ID.String(), Version: "1", Name: "TV", Description: "M3U playlist", Types: []string{"tv"},
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"tv"}, IDPrefixes: only},
			{Name: "stream", Types: []string{"tv"}, IDPrefixes: only}},
		Catalogs: []stremio.Catalog{{Type: "tv", ID: catalogID, Name: "TV"}}, IDPrefixes: only}
	found := must(e.addons.Find(t.Context(), addon.ID))
	if !reflect.DeepEqual(found.Manifest, before) {
		t.Errorf("manifest %+v, want %+v", found.Manifest, before)
	}
	channels := must(e.service.Channels(t.Context(), addon.ID))
	if len(channels) != 12 || channels[3].Name != "FR - Lumo Movie (2018) 4K" {
		t.Errorf("channels: %q", names(channels))
	}
	libraries := must(e.addons.Libraries(t.Context(), addons.Shared()))
	if len(libraries) != 1 || libraries[0].Catalog.Type != "tv" {
		t.Errorf("libraries: %+v", libraries)
	}
	source := must(e.service.Source(t.Context(), addons.Shared(), addon.ID))
	if source.VOD != (VODCounts{}) || !source.Options.LiveTv || source.Options.Movies || source.Options.VODLibraries != LibrariesByType ||
		!source.Options.Enrichment {
		t.Errorf("source: %+v", source)
	}
}

// catalogs describes a source's catalogs, and libraryRows the scope's
// enabled libraries, as "type/id name".
func (e env) catalogs(source accounts.ID) []string {
	e.t.Helper()
	var result []string
	for _, c := range must(e.addons.Find(e.t.Context(), source)).Manifest.Catalogs {
		result = append(result, c.Type+"/"+c.ID+" "+c.Name)
	}
	return result
}

func (e env) libraryRows() []string {
	e.t.Helper()
	var result []string
	for _, l := range must(e.addons.Libraries(e.t.Context(), addons.Shared())) {
		if l.Enabled {
			result = append(result, l.Catalog.Type+"/"+l.Catalog.ID)
		}
	}
	return result
}

func syntheticVOD(t *testing.T, movies, series int) (*syntheticServer, Account) {
	t.Helper()
	server := newSyntheticServer(newSynthetic(6, 2).withVOD(movies, series), nil)
	server.Start()
	t.Cleanup(server.Close)
	return server, Account{Kind: addons.KindXtream, Server: server.URL, Username: "user", Password: "secret"}
}

// Movies and series turned on bring their libraries, one per type then one
// per category; a library turned off stays off across refreshes; a
// category excluded loses its titles; live channels turned off take the
// Live TV catalog away but keep the line-up's edits for when they come
// back.
func TestVODLibrariesFollowOptions(t *testing.T) {
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	server, account := syntheticVOD(t, 12, 4)
	movies := true
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &movies}}, false))
	source := addon.ID
	if got := e.catalogs(source); !slices.Equal(got, []string{"tv/channels Box", "movie/movies Box"}) {
		t.Errorf("catalogs with movies: %q", got)
	}
	if got := e.libraryRows(); !slices.Equal(got, []string{"tv/channels", "movie/movies"}) {
		t.Errorf("libraries with movies: %q", got)
	}
	if server.count("get_series") != 0 || server.count("get_vod_streams") != 1 {
		t.Errorf("lists read: %v", server.requests)
	}
	series := true
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Series: &series}}, false); err != nil {
		t.Fatal(err)
	}
	if got := e.libraryRows(); !slices.Equal(got, []string{"tv/channels", "movie/movies", "series/series"}) || server.count("get_series") != 1 {
		t.Errorf("libraries with series: %q after %d series lists", got, server.count("get_series"))
	}
	genres := must(e.addons.Find(ctx, source)).Manifest.Catalogs[1].Extra
	if len(genres) != 3 || genres[1].Name != "genre" || !slices.Equal(genres[1].Options, server.data.vodGroups) {
		t.Errorf("movie catalog extras: %+v", genres)
	}

	// The movies library turned off stays off.
	if _, err := e.addons.SetLibraries(ctx, shared, []addons.LibraryChoice{{AddonID: source, CatalogType: "tv", CatalogID: "channels"},
		{AddonID: source, CatalogType: "series", CatalogID: "series"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Refresh(ctx, shared, source, false); err != nil {
		t.Fatal(err)
	}
	if got := e.libraryRows(); !slices.Equal(got, []string{"tv/channels", "series/series"}) {
		t.Errorf("libraries after a refresh: %q", got)
	}

	// One library per category, search catalogs aside.
	byCategory := LibrariesByGroup
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{VODLibraries: &byCategory}}, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"tv/channels Box"}
	for _, name := range server.data.vodGroups {
		want = append(want, "movie/"+categoryCatalogID(typeMovie, name)+" "+name)
	}
	want = append(want, "movie/movies.search Box")
	for _, name := range server.data.seriesGroups {
		want = append(want, "series/"+categoryCatalogID(typeSeries, name)+" "+name)
	}
	want = append(want, "series/series.search Box")
	if got := e.catalogs(source); !slices.Equal(got, want) {
		t.Errorf("catalogs by category\n%q\nwant\n%q", got, want)
	}
	if got := e.libraryRows(); len(got) != 1+len(server.data.vodGroups)+len(server.data.seriesGroups) || slices.Contains(got, "movie/movies.search") {
		t.Errorf("libraries by category: %q", got)
	}
	firstGroup := server.data.vodGroups[0]
	page := must(e.service.Catalog(ctx, source, "movie", categoryCatalogID(typeMovie, firstGroup), 0, "", ""))
	if len(page) != 4 || !slices.Equal(page[0].Genres[:1], []string{firstGroup}) {
		t.Errorf("a category's page: %+v", page)
	}

	// A category excluded loses its catalog and its titles, in searches too.
	excluded := []string{"movie:" + firstGroup}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{VODExcluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(e.catalogs(source), "movie/"+categoryCatalogID(typeMovie, firstGroup)+" "+firstGroup) {
		t.Error("an excluded category keeps its catalog")
	}
	if found := must(e.service.Catalog(ctx, source, "movie", "movies.search", 0, "", "zebzeb")); len(found) != 0 {
		t.Errorf("an excluded movie found: %+v", found)
	}
	if found := must(e.service.Catalog(ctx, source, "movie", "movies.search", 0, "", "Orbzeb 1")); len(found) != 1 || found[0].Name != "Orbzeb 1" {
		t.Errorf("a movie found: %+v", found)
	}
	counted := must(e.service.Source(ctx, shared, source)).VOD
	if counted.Movies != 12 || counted.ShownMovies != 8 || counted.Series != 4 || counted.ShownSeries != 4 || counted.MovieCategories != 3 {
		t.Errorf("counts: %+v", counted)
	}

	// Live channels off: no Live TV catalog, the line-up kept with its
	// edits; back on, the channels come back as edited.
	channel := e.lineup(source)[0]
	must(e.service.UpdateChannel(ctx, shared, source, channel.ID, ChannelChanges{Name: &Nullable[string]{Value: new("Renamed")}}))
	off := false
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{LiveTv: &off}}, false); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(e.libraryRows(), "tv/channels") || slices.ContainsFunc(e.catalogs(source), func(c string) bool { return strings.HasPrefix(c, "tv/") }) {
		t.Errorf("Live TV kept while off: %q", e.catalogs(source))
	}
	if metas := must(e.service.Channels(ctx, source)); len(metas) != 0 {
		t.Errorf("channels while off: %q", names(metas))
	}
	on := true
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{LiveTv: &on}}, false); err != nil {
		t.Fatal(err)
	}
	if metas := must(e.service.Channels(ctx, source)); len(metas) == 0 || metas[0].Name != "Renamed" {
		t.Errorf("channels back on: %q", names(metas))
	}
	if !slices.Contains(e.libraryRows(), "tv/channels") {
		t.Errorf("Live TV library back on: %q", e.libraryRows())
	}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{LiveTv: &off, Movies: &off, Series: &off}}, false); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("importing nothing: %v", err)
	}
}

// Listing movies and series never asks a provider for details; opening a
// title asks once, then the details are kept, until the series changes;
// requests to a host are paced; playing builds the provider's file
// addresses, with the quality the names give.
func TestXtreamDetailsAreLazyCachedAndPaced(t *testing.T) {
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	server, account := syntheticVOD(t, 250, 3)
	e.service.pacer = NewPacer(300 * time.Millisecond)
	on := true
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &on, Series: &on}}, false))
	source := addon.ID

	pages := 0
	for skip := 0; ; skip += vodPage {
		page := must(e.service.Catalog(ctx, source, "movie", "movies", skip, "", ""))
		if len(page) == 0 {
			break
		}
		for _, meta := range page {
			_ = must(e.service.Meta(ctx, source, meta.ID))
		}
		pages++
	}
	shows := must(e.service.Catalog(ctx, source, "series", "series", 0, "", ""))
	if pages != 3 || len(shows) != 3 {
		t.Fatalf("pages %d, series %d", pages, len(shows))
	}
	newest := must(e.service.Catalog(ctx, source, "movie", "movies", 0, "", ""))[0]
	if name, year := titleName(server.data.movies[249].Name); newest.Name != name || newest.Name != "Mirfen 249" || newest.ReleaseInfo != stremio.Text(fmt.Sprint(year)) ||
		year != 1999 || newest.Poster == "" || newest.TmdbID != "1249" {
		t.Errorf("newest movie first, from its listing: %+v", newest)
	}
	if listed := must(e.service.Meta(ctx, source, shows[0].ID)); len(listed.Videos) != 0 || listed.Description != "A show of the provider." {
		t.Errorf("a series from its listing: %+v", listed)
	}
	if server.count("get_vod_info") != 0 || server.count("get_series_info") != 0 {
		t.Fatalf("details asked for listings: %v", server.requests)
	}

	opened := Opening(ctx)
	movie := must(e.service.Meta(opened, source, newest.ID))
	if movie.Description != "The plot of "+server.data.movies[249].Name || movie.Runtime != "90 min" || !slices.Equal(movie.Cast, []string{"Dee Lum", "Eve Tac"}) ||
		!slices.Contains(movie.Genres, "Action") || movie.Trailers[0].Source != "abcdefghijk" || movie.Released != "2020-05-06T00:00:00.000Z" {
		t.Errorf("an opened movie: %+v", movie)
	}
	_ = must(e.service.Meta(opened, source, newest.ID))
	if again := must(e.service.Meta(ctx, source, newest.ID)); again.Description != movie.Description || server.count("get_vod_info") != 1 {
		t.Errorf("details asked %d times; kept: %q", server.count("get_vod_info"), again.Description)
	}

	// Three titles opened at once are asked one after the other.
	server.mu.Lock()
	server.at = nil
	server.mu.Unlock()
	var wg sync.WaitGroup
	for _, meta := range must(e.service.Catalog(ctx, source, "movie", "movies", 1, "", ""))[:3] {
		wg.Go(func() { _ = must(e.service.Meta(opened, source, meta.ID)) })
	}
	wg.Wait()
	if len(server.at) != 3 || server.at[2].Sub(server.at[0]) < 550*time.Millisecond {
		t.Errorf("details requests not paced: %v", server.at)
	}

	show := must(e.service.Meta(opened, source, shows[0].ID))
	spec := server.data.series[0]
	if len(show.Videos) != spec.Seasons*spec.Episodes || show.Videos[0].Title != "Part 1" || show.Videos[0].Runtime != "45 min" ||
		show.Extras == nil || show.Extras.SeasonPosterByNumber["1"] == "" || show.Description != "A show of the provider, in detail." {
		t.Errorf("an opened series: %+v", show)
	}

	streams := must(e.service.Streams(ctx, source, newest.ID))
	if len(streams) != 1 || streams[0].URL != fmt.Sprintf("%s/movie/user/secret/%d.%s", server.URL, server.data.movies[249].ID, server.data.movies[249].Extension) ||
		streams[0].Name != "Box" {
		t.Errorf("a movie's stream: %+v", streams)
	}
	fourK := must(e.service.Catalog(ctx, source, "movie", "movies.search", 0, "", "Daxfen 248"))
	if len(fourK) != 1 {
		t.Fatalf("search: %+v", fourK)
	}
	if streams := must(e.service.Streams(ctx, source, fourK[0].ID)); streams[0].Description != "4K" {
		t.Errorf("a 4K movie's stream: %+v", streams)
	}
	episode := show.Videos[len(show.Videos)-1]
	streams = must(e.service.Streams(ctx, source, episode.ID))
	if len(streams) != 1 || !strings.HasPrefix(streams[0].URL, server.URL+"/series/user/secret/") || !strings.HasSuffix(streams[0].URL, ".mkv") ||
		streams[0].Description != "FHD" {
		t.Errorf("an episode's stream: %+v", streams)
	}

	// A series whose listing changed is asked again when opened.
	server.data.series[0].Modified++
	if err := e.service.Refresh(ctx, shared, source, false); err != nil {
		t.Fatal(err)
	}
	_ = must(e.service.Meta(opened, source, shows[0].ID))
	if server.count("get_series_info") != 2 {
		t.Errorf("series details asked %d times", server.count("get_series_info"))
	}
}

// Titles keep their identifiers when lists are downloaded again in
// another order, and M3U titles when the account's credentials change.
func TestTitlesKeepIdentifiersAcrossRefreshes(t *testing.T) {
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	server, account := syntheticVOD(t, 20, 2)
	on := true
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &on, Series: &on}}, false))
	ids := func(source accounts.ID, typ, catalog string) []string {
		var result []string
		for _, meta := range must(e.service.Catalog(ctx, source, typ, catalog, 0, "", "")) {
			result = append(result, meta.Name+"="+meta.ID)
		}
		slices.Sort(result)
		return result
	}
	before := ids(addon.ID, "movie", "movies")
	slices.Reverse(server.data.movies)
	if err := e.service.Refresh(ctx, shared, addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if after := ids(addon.ID, "movie", "movies"); !slices.Equal(before, after) {
		t.Errorf("movie identifiers changed:\n%q\n%q", before, after)
	}

	list := newPlaylistServer(t, vodPlaylist)
	m3u := must(e.service.Add(ctx, shared, NewSource{Name: "List", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Movies: &on, Series: &on}}, false))
	movies, shows := ids(m3u.ID, "movie", "movies"), ids(m3u.ID, "series", "series")
	if len(movies) != 3 || len(shows) != 3 {
		t.Fatalf("M3U titles: %q %q", movies, shows)
	}
	if channels := must(e.service.Channels(ctx, m3u.ID)); len(channels) != 3 {
		t.Errorf("M3U channels besides its titles: %q", names(channels))
	}
	list.set(strings.ReplaceAll(vodPlaylist, "/user/pass/", "/user/newpass/"), 0)
	if err := e.service.Update(ctx, shared, m3u.ID, Changes{Account: &Account{URL: list.url + "&v=2"}}, false); err != nil {
		t.Fatal(err)
	}
	if after := ids(m3u.ID, "movie", "movies"); !slices.Equal(movies, after) {
		t.Errorf("M3U movie identifiers changed:\n%q\n%q", movies, after)
	}
	if after := ids(m3u.ID, "series", "series"); !slices.Equal(shows, after) {
		t.Errorf("M3U series identifiers changed:\n%q\n%q", shows, after)
	}
	show := must(e.service.Catalog(ctx, m3u.ID, "series", "series.search", 0, "", "zeb show"))
	if len(show) != 1 {
		t.Fatalf("series found: %+v", show)
	}
	meta := must(e.service.Meta(ctx, m3u.ID, show[0].ID))
	if len(meta.Videos) != 3 || meta.Videos[2].Season != 2 {
		t.Errorf("M3U episodes: %+v", meta.Videos)
	}
	streams := must(e.service.Streams(ctx, m3u.ID, meta.Videos[1].ID))
	if len(streams) != 1 || streams[0].URL != "https://tv.example/series/user/newpass/9002.mkv" || streams[0].Description != "FHD" {
		t.Errorf("M3U episode stream: %+v", streams)
	}
}

// VOD previews count each category's titles, for a new account and for a
// source, sharing one download per list for a few minutes with the source
// added next.
func TestVODPreviews(t *testing.T) {
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	server, account := syntheticVOD(t, 30, 5)
	total, categories := must2(e.service.PreviewAccount(ctx, account, PreviewMovies, "", false))
	if total != 30 || len(categories) != 3 || categories[0].Key != "movie:"+server.data.vodGroups[0] || categories[0].Channels != 10 {
		t.Errorf("movie preview: %d %+v", total, categories)
	}
	if _, found := must2(e.service.PreviewAccount(ctx, account, PreviewMovies, "action", false)); len(found) != 1 {
		t.Errorf("searched preview: %+v", found)
	}
	on := true
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &on}}, false))
	if server.count("get_vod_streams") != 1 {
		t.Errorf("movie list downloaded %d times", server.count("get_vod_streams"))
	}
	excluded := []string{"series:" + server.data.seriesGroups[1]}
	if err := e.service.Update(ctx, shared, addon.ID, Changes{Options: &OptionsPatch{VODExcluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	total, categories = must2(e.service.Preview(ctx, shared, addon.ID, PreviewSeries, ""))
	if total != 5 || len(categories) != 2 || categories[0].Excluded || !categories[1].Excluded || server.count("get_series") != 1 {
		t.Errorf("series preview of a source: %d %+v after %d downloads", total, categories, server.count("get_series"))
	}
	if _, _, err := e.service.Preview(ctx, shared, addon.ID, "planet", ""); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("a bad preview: %v", err)
	}
	list := newPlaylistServer(t, vodPlaylist)
	total, categories = must2(e.service.PreviewAccount(ctx, Account{Kind: addons.KindM3U, URL: list.url}, PreviewSeries, "", false))
	if total != 3 || len(categories) != 2 || categories[1].Channels != 2 {
		t.Errorf("M3U series preview: %d %+v", total, categories)
	}
}
