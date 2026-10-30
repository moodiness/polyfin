package taste

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
)

// fakeTitles describes the titles it holds, of the kinds asked, in the
// order asked.
type fakeTitles map[accounts.ID]library.Item

func (f fakeTitles) ItemsOf(_ context.Context, _ accounts.User, ids []accounts.ID, kinds []library.Kind) ([]library.Item, error) {
	var items []library.Item
	for _, id := range ids {
		if item, ok := f[id]; ok && slices.Contains(kinds, item.Kind) {
			items = append(items, item)
		}
	}
	return items, nil
}

// A profile is built from the user's data: a movie watched again weighs
// more, a series counts its episodes finished, and a movie stopped early,
// a series whose every episode was, and a hidden title weigh against. It
// is kept, and read again after a restart, with the user's taglines.
func TestProfileIsBuiltFromTheUsersData(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	data := userdata.New(pool)
	rewatched, abandoned, hidden, show, dropped := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}, accounts.ID{4}, accounts.ID{5}
	titles := fakeTitles{
		rewatched: {ID: rewatched, Kind: library.KindMovie, Genres: []string{"Thriller"}, ProductionYear: 1994, Language: "French",
			People: []library.Person{{Name: "Some Director", Type: "Director"}, {Name: "Some Actor", Type: "Actor"}}},
		abandoned: {ID: abandoned, Kind: library.KindMovie, Genres: []string{"Musical"}},
		hidden:    {ID: hidden, Kind: library.KindMovie, Genres: []string{"Western"}},
		show:      {ID: show, Kind: library.KindSeries, Genres: []string{"Crime"}},
		dropped:   {ID: dropped, Kind: library.KindSeries, Genres: []string{"Soap"}},
	}
	day := 24 * time.Hour
	set := func(items []userdata.Item, change func(*userdata.Data)) {
		t.Helper()
		if _, err := data.Change(t.Context(), user.ID, items, change); err != nil {
			t.Fatal(err)
		}
	}
	at := func(ago time.Duration) *time.Time { return new(now.Add(-ago)) }
	set([]userdata.Item{{ID: rewatched}}, func(d *userdata.Data) { d.Played, d.PlayCount, d.LastPlayed = true, 3, at(day) })
	set([]userdata.Item{{ID: abandoned}}, func(d *userdata.Data) { d.LastPlayed = at(5 * day) })
	for i := range 3 {
		set([]userdata.Item{{ID: accounts.ID{40, byte(i)}, Series: show, Season: accounts.ID{41}}}, func(d *userdata.Data) {
			d.Played, d.PlayCount, d.LastPlayed = true, 1, at(day)
		})
	}
	set([]userdata.Item{{ID: accounts.ID{50}, Series: dropped, Season: accounts.ID{51}}}, func(d *userdata.Data) { d.LastPlayed = at(4 * day) })
	if err := data.Hide(t.Context(), user.ID, hidden, now.Add(-day)); err != nil {
		t.Fatal(err)
	}
	service := New(pool, users, data, titles, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.SetClock(func() time.Time { return now })
	profile, err := service.Build(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	decay := Activity{Finished: 1, At: now.Add(-day)}.Weight(now)
	for _, c := range []struct {
		facet Facet
		value string
		want  float64
	}{
		{FacetGenre, "thriller", 2 * decay},
		{FacetDirector, "some director", 2 * decay},
		{FacetCast, "some actor", 2 * decay},
		{FacetDecade, "1990s", 2 * decay},
		{FacetLanguage, "french", 2 * decay},
		{FacetGenre, "crime", 1.5 * decay},
		{FacetGenre, "musical", -Activity{Finished: 1, At: now.Add(-5 * day)}.Weight(now)},
		{FacetGenre, "soap", -Activity{Finished: 1, At: now.Add(-4 * day)}.Weight(now)},
		{FacetGenre, "western", -decay},
	} {
		if got := profile.weight(c.facet, c.value); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%s %s: %v, want %v", c.facet, c.value, got, c.want)
		}
	}
	if profile.Titles != 5 || profile.Liked != 2 || profile.Confident() {
		t.Errorf("titles %d, liked %d, confident %v", profile.Titles, profile.Liked, profile.Confident())
	}
	if err := service.SetTaglines(t.Context(), user.ID, false); err != nil {
		t.Fatal(err)
	}
	again := New(pool, users, data, titles, slog.New(slog.NewTextHandler(io.Discard, nil)))
	kept, err := again.Profile(t.Context(), user.ID)
	if err != nil || !kept.Built.Equal(profile.Built) || kept.weight(FacetGenre, "Thriller") != profile.weight(FacetGenre, "Thriller") {
		t.Errorf("kept: %+v %v", kept, err)
	}
	if on, err := again.Taglines(t.Context(), user.ID); err != nil || on {
		t.Errorf("taglines: %v %v", on, err)
	}
	// Building again keeps the taglines off.
	if _, err := again.Build(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	if on, _ := again.Taglines(t.Context(), user.ID); on {
		t.Error("building turned the taglines on")
	}
}
