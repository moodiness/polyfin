package library

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// watchlistToday is the day the tests of watchlists run on.
var watchlistToday = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

// watchlistAddon serves Movies, three movies, and Shows: Show, with a
// special, two regular seasons listed out of order and an episode to
// come; Soon, whose only episode airs next week; Bare, which lists none.
func watchlistAddon() *fakeAddon {
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "wl", Name: "Watchlist addon", Version: "1", Types: []string{"movie", "series"},
			IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "movies", Name: "Movies"}, {Type: "series", ID: "shows", Name: "Shows"}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
	}
	add := func(catalog string, meta stremio.Meta) {
		meta.Poster = "https://images.example/" + meta.ID + ".jpg"
		row := meta
		row.Videos = nil
		addon.catalogs[catalog] = append(addon.catalogs[catalog], row)
		addon.metas[meta.Type+"/"+meta.ID] = meta
	}
	for _, m := range []struct{ id, name string }{{"tt41", "First"}, {"tt42", "Second"}, {"tt43", "Third"}} {
		add("movie/movies", stremio.Meta{ID: m.id, Type: "movie", Name: m.name})
	}
	add("series/shows", stremio.Meta{ID: "tt31", Type: "series", Name: "Show", Videos: []stremio.Video{
		episode("tt31", 0, 1, "2024-01-01"), episode("tt31", 2, 1, "2025-03-01"), episode("tt31", 1, 2, "2024-02-08"),
		episode("tt31", 1, 1, "2024-02-01"), episode("tt31", 2, 2, "2026-12-01"),
	}})
	add("series/shows", stremio.Meta{ID: "tt32", Type: "series", Name: "Soon", Videos: []stremio.Video{episode("tt32", 1, 1, "2026-11-06")}})
	add("series/shows", stremio.Meta{ID: "tt33", Type: "series", Name: "Bare"})
	return addon
}

// watchlistEnv installs watchlistAddon on watchlistToday, the clock it
// returns, and lists its catalogs, as a member browsing does.
func watchlistEnv(t *testing.T) (env, *atomic.Pointer[time.Time]) {
	t.Helper()
	e := newEnv(t)
	clock := &atomic.Pointer[time.Time]{}
	clock.Store(&watchlistToday)
	e.service.SetClock(func() time.Time { return *clock.Load() })
	e.install(addons.Shared(), watchlistAddon())
	e.children(e.member, "Movies", 0, 10)
	e.children(e.member, "Shows", 0, 10)
	return e, clock
}

func showEpisode(season, number int) accounts.ID {
	return itemID(episodeKey(fmt.Sprintf("tt31:%d:%d", season, number)))
}

func seriesID(stremioID string) accounts.ID { return itemID(titleKey(KindSeries, stremioID)) }
func movieID(stremioID string) accounts.ID  { return itemID(titleKey(KindMovie, stremioID)) }

// A playlist takes a series' aired episodes of its regular seasons, in
// the series' order, without the specials or those to come; a season its
// aired episodes, a season of specials too.
func TestPlaylistEpisodesTakeTheAiredEpisodes(t *testing.T) {
	t.Parallel()
	e, _ := watchlistEnv(t)
	ids := func(season *accounts.ID) []accounts.ID {
		t.Helper()
		episodes, err := e.service.PlaylistEpisodes(t.Context(), e.member, seriesID("tt31"), season)
		if err != nil {
			t.Fatal(err)
		}
		result := make([]accounts.ID, len(episodes))
		for i, episode := range episodes {
			result[i] = episode.ID
		}
		return result
	}
	if got, want := ids(nil), []accounts.ID{showEpisode(1, 1), showEpisode(1, 2), showEpisode(2, 1)}; !slices.Equal(got, want) {
		t.Errorf("the series: %v, want %v", got, want)
	}
	second, specials := itemID(seasonKey("tt31", 2)), itemID(seasonKey("tt31", 0))
	if got, want := ids(&second), []accounts.ID{showEpisode(2, 1)}; !slices.Equal(got, want) {
		t.Errorf("season 2: %v, want %v", got, want)
	}
	if got, want := ids(&specials), []accounts.ID{showEpisode(0, 1)}; !slices.Equal(got, want) {
		t.Errorf("the specials: %v, want %v", got, want)
	}
}

// A watchlist takes a movie or an episode itself, a series or a season
// as a playlist does; a series none of whose episodes aired, one that
// lists none, or a title the library does not know waits.
func TestWatchlistEntriesOfEachKind(t *testing.T) {
	t.Parallel()
	e, _ := watchlistEnv(t)
	second := itemID(seasonKey("tt31", 2))
	// The member opened Show, as they do before adding one of its episodes.
	if _, err := e.service.Episodes(t.Context(), e.member, seriesID("tt31"), nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		title accounts.ID
		want  watchlist.Made
	}{
		"a movie":             {movieID("tt41"), watchlist.Made{Items: []accounts.ID{movieID("tt41")}, Known: true}},
		"an episode":          {showEpisode(2, 2), watchlist.Made{Items: []accounts.ID{showEpisode(2, 2)}, Series: seriesID("tt31"), Known: true}},
		"a series":            {seriesID("tt31"), watchlist.Made{Items: []accounts.ID{showEpisode(1, 1), showEpisode(1, 2), showEpisode(2, 1)}, Series: seriesID("tt31"), Known: true}},
		"a season":            {second, watchlist.Made{Items: []accounts.ID{showEpisode(2, 1)}, Series: seriesID("tt31"), Known: true}},
		"a series to come":    {seriesID("tt32"), watchlist.Made{}},
		"a series of nothing": {seriesID("tt33"), watchlist.Made{}},
		"an unknown title":    {movieID("tt99"), watchlist.Made{}},
	} {
		got, err := e.service.WatchlistEntries(t.Context(), e.member, c.title)
		if err != nil || !slices.Equal(got.Items, c.want.Items) || got.Series != c.want.Series || got.Known != c.want.Known {
			t.Errorf("%s: %+v %v, want %+v", name, got, err, c.want)
		}
	}
}

// What a list brings adds only what is left to watch: a movie started or
// played adds nothing, a series or a season its aired episodes not
// played, the one in progress kept; with nothing left, a title adds
// nothing and waits no more.
func TestWatchlistEntriesAreWhatIsLeftToWatch(t *testing.T) {
	t.Parallel()
	e, _ := watchlistEnv(t)
	ctx := t.Context()
	change := func(ids []accounts.ID, change func(*userdata.Data)) {
		t.Helper()
		items := make([]userdata.Item, len(ids))
		for i, id := range ids {
			items[i] = userdata.Item{ID: id}
		}
		if _, err := e.service.userData.Change(ctx, e.member.ID, items, change); err != nil {
			t.Fatal(err)
		}
	}
	change([]accounts.ID{movieID("tt41"), showEpisode(1, 1)}, func(d *userdata.Data) { d.Played = true })
	change([]accounts.ID{movieID("tt42"), showEpisode(1, 2)}, func(d *userdata.Data) { d.Position, d.Runtime = 10*time.Minute, time.Hour })
	second := itemID(seasonKey("tt31", 2))
	check := func(name string, title accounts.ID, want watchlist.Made) {
		t.Helper()
		got, err := e.service.WatchlistEntries(ctx, e.member, title)
		if err != nil || !slices.Equal(got.Items, want.Items) || got.Series != want.Series || got.Known != want.Known {
			t.Errorf("%s: %+v %v, want %+v", name, got, err, want)
		}
	}
	show := seriesID("tt31")
	check("a played movie", movieID("tt41"), watchlist.Made{Known: true})
	check("a started movie", movieID("tt42"), watchlist.Made{Known: true})
	check("a movie not started", movieID("tt43"), watchlist.Made{Items: []accounts.ID{movieID("tt43")}, Known: true})
	check("a series partly watched", show, watchlist.Made{Items: []accounts.ID{showEpisode(1, 2), showEpisode(2, 1)}, Series: show, Known: true})
	check("a played episode", showEpisode(1, 1), watchlist.Made{Series: show, Known: true})
	check("an episode in progress", showEpisode(1, 2), watchlist.Made{Items: []accounts.ID{showEpisode(1, 2)}, Series: show, Known: true})

	change([]accounts.ID{showEpisode(1, 2), showEpisode(2, 1)}, func(d *userdata.Data) { d.Played = true })
	check("a series watched whole", show, watchlist.Made{Series: show, Known: true})
	check("a season watched whole", second, watchlist.Made{Series: show, Known: true})

	// Added by hand, the played movie and the watched series come whole.
	if err := e.service.AddToWatchlist(ctx, e.member.ID, []watchlist.Group{
		{Title: movieID("tt41"), Items: []accounts.ID{movieID("tt41")}},
		{Title: show, Series: show, Items: []accounts.ID{showEpisode(1, 1), showEpisode(1, 2), showEpisode(2, 1)}},
	}, 0); err != nil {
		t.Fatal(err)
	}
	held, err := e.service.WatchlistHeld(ctx, e.member.ID)
	if err != nil || len(held) != 4 {
		t.Errorf("added by hand: %+v %v", held, err)
	}
}

// Through the library, a list's series becomes its aired regular-season
// episodes in a member's watchlist; a series none of whose episodes aired
// waits, and its episodes come once one airs, then no more.
func TestAWatchlistSeriesBecomesItsAiredEpisodes(t *testing.T) {
	t.Parallel()
	e, clock := watchlistEnv(t)
	ctx := t.Context()
	store := watchlist.New(e.service.db).WithEntries(func(ctx context.Context, _, title accounts.ID) (watchlist.Made, error) {
		return e.service.WatchlistEntries(ctx, e.member, title)
	}).WithClock(func() time.Time { return *clock.Load() })
	at := func(day int) *time.Time { return new(time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC)) }
	if _, err := store.Save(ctx, e.member.ID, watchlist.Tracking("trakt"), []watchlist.Title{
		{ID: seriesID("tt31"), AddedAt: at(1)}, {ID: seriesID("tt32"), AddedAt: at(3)}, {ID: movieID("tt41"), AddedAt: at(2)},
	}, true); err != nil {
		t.Fatal(err)
	}
	want := []accounts.ID{movieID("tt41"), showEpisode(1, 1), showEpisode(1, 2), showEpisode(2, 1)}
	titles := func(step string, want []accounts.ID) {
		t.Helper()
		if got, err := store.Titles(ctx, e.member.ID); err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s: %v %v, want %v", step, got, err, want)
		}
	}
	titles("imported", want)

	// Forty days later, Soon's first episode and Show's next one have
	// aired: Soon's comes, Show's does not.
	clock.Store(new(watchlistToday.AddDate(0, 0, 40)))
	if err := store.ExpandAll(ctx); err != nil {
		t.Fatal(err)
	}
	want = append([]accounts.ID{itemID(episodeKey("tt32:1:1"))}, want...)
	titles("once aired", want)
	if _, err := store.Save(ctx, e.member.ID, watchlist.Tracking("trakt"), []watchlist.Title{
		{ID: seriesID("tt31"), AddedAt: at(1)}, {ID: seriesID("tt32"), AddedAt: at(3)}, {ID: movieID("tt41"), AddedAt: at(2)},
	}, true); err != nil {
		t.Fatal(err)
	}
	titles("imported again", want)
}

// Each member who sees a playlist gets their own image of it, from its
// first titles, a series by its poster, under the name they see it by:
// none for one under parental control, none shown to one who hid a title
// it shows. A title taken off the watchlist leaves its image at once, and
// the images go with the playlist.
func TestEachViewerGetsTheirOwnPlaylistMosaic(t *testing.T) {
	t.Parallel()
	e, _ := watchlistEnv(t)
	ctx := t.Context()
	playlist, exists, err := e.service.Watchlist(ctx, e.member.ID)
	if err != nil || !exists {
		t.Fatalf("the member's watchlist: %v %v", exists, err)
	}
	if err := e.service.AddToWatchlist(ctx, e.member.ID, []watchlist.Group{
		{Title: movieID("tt41"), Items: []accounts.ID{movieID("tt41")}},
		{Title: seriesID("tt31"), Series: seriesID("tt31"), Items: []accounts.ID{showEpisode(1, 1)}},
		{Title: movieID("tt42"), Items: []accounts.ID{movieID("tt42")}},
	}, 0); err != nil {
		t.Fatal(err)
	}
	first := []accounts.ID{movieID("tt41"), seriesID("tt31"), movieID("tt42")}
	image := func(user accounts.User, name string, ids []accounts.ID) string {
		t.Helper()
		return e.service.PlaylistImage(ctx, user, playlist, name, ids)
	}
	mine := image(e.member, "Watchlist", first)
	theirs := image(e.admin, "Watchlist (member)", first)
	if mine == "" || theirs == "" || mine == theirs {
		t.Fatalf("images: %q, %q", mine, theirs)
	}
	art, ok := e.service.PersonalArtwork(ctx, playlist, ImageTag(mine))
	if !ok || art.Generated == nil || !slices.Equal(art.Generated.Posters, []string{
		"https://images.example/tt41.jpg", "https://images.example/tt31.jpg", "https://images.example/tt42.jpg",
	}) {
		t.Errorf("the member's image: %+v %v", art.Generated, ok)
	}

	kid := e.restrict("kid", accounts.ParentalControl{MaxRating: new(13)})
	if got := image(kid, "Watchlist (member)", first); got != "" {
		t.Errorf("an image under parental control: %q", got)
	}
	hider, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "hider", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.service.userData.Hide(ctx, hider.ID, movieID("tt42"), watchlistToday); err != nil {
		t.Fatal(err)
	}
	if got := image(hider, "Watchlist (member)", first); got != "" {
		t.Errorf("an image showing a title the viewer hid: %q", got)
	}

	// A title taken off leaves the image at its next listing.
	if err := e.service.RemoveFromWatchlist(ctx, e.member.ID, []accounts.ID{movieID("tt41")}, nil); err != nil {
		t.Fatal(err)
	}
	after := image(e.member, "Watchlist", first[1:])
	art, ok = e.service.PersonalArtwork(ctx, playlist, ImageTag(after))
	if after == mine || !ok || art.Generated == nil || slices.Contains(art.Generated.Posters, "https://images.example/tt41.jpg") {
		t.Errorf("the image once a title left: %+v %v", art.Generated, ok)
	}

	var rows int
	if err := e.service.db.QueryRow(ctx, "SELECT count(*) FROM personal_images WHERE playlist_id = $1", playlist).Scan(&rows); err != nil || rows != 3 {
		t.Errorf("%d images kept, %v; want the member's, the administrator's and the hider's", rows, err)
	}
	if err := e.service.DeleteWatchlist(ctx, e.member.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.service.db.QueryRow(ctx, "SELECT count(*) FROM personal_images").Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d images left with the playlist, %v", rows, err)
	}
}
