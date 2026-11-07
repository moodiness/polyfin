package importlinks

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

func TestParseReadsIdentifiersAndAddresses(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]Target{
		"tt0111161":                             {IMDb: "tt0111161"},
		" TT0111161 ":                           {IMDb: "tt0111161"},
		"tmdb:278":                              {TMDB: 278},
		"278":                                   {TMDB: 278},
		"https://www.imdb.com/title/tt0903747/": {IMDb: "tt0903747"},
		"imdb.com/title/tt0903747/episodes/?season=2":             {IMDb: "tt0903747"},
		"https://m.imdb.com/fr/title/tt0111161/?ref_=nv_sr":       {IMDb: "tt0111161"},
		"https://www.themoviedb.org/movie/278-harbor-lights":      {TMDB: 278, Kind: KindMovie},
		"https://www.themoviedb.org/tv/1396-night-shift/season/2": {TMDB: 1396, Kind: KindSeries},
		"http://themoviedb.org/tv/1396?language=fr":               {TMDB: 1396, Kind: KindSeries},
		"www.themoviedb.org/movie/550":                            {TMDB: 550, Kind: KindMovie},
	} {
		if got, err := Parse(input); err != nil || got != want {
			t.Errorf("%q: %+v %v, want %+v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "tt", "0", "tmdb:", "tmdb:-3", "kitsu:12", "https://www.imdb.com/name/nm0000151/",
		"https://www.themoviedb.org/person/287", "https://example.com/title/tt0111161/", "ftp://imdb.com/title/tt0111161/",
		"https://www.themoviedb.org/collection/10"} {
		if got, err := Parse(input); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("%q: %+v %v", input, got, err)
		}
	}
}

// titles finds the movies and series it knows by IMDb identifier, and by
// TMDB one through its IMDb one, as Resolve and Identify's lookup do.
type titles struct {
	mu sync.Mutex
	// kinds are the titles the addons describe, by IMDb identifier.
	kinds map[string]library.Kind
	// tmdb maps "<kind>:<tmdb>" to an IMDb identifier.
	tmdb  map[string]string
	names map[string]string
}

func (f *titles) target(ref library.TitleRef) []library.TitleTarget {
	f.mu.Lock()
	defer f.mu.Unlock()
	kind := library.KindMovie
	if ref.Series || ref.Episode {
		kind = library.KindSeries
	}
	imdb := ref.IMDb
	if imdb == "" {
		imdb = f.tmdb[string(kind)+":"+strconv.Itoa(ref.TMDB)]
	}
	if imdb == "" {
		return nil
	}
	if !ref.Episode {
		return []library.TitleTarget{{ID: itemID(imdb), Runtime: time.Hour}}
	}
	episode := imdb + ":" + strconv.Itoa(ref.Season) + ":" + strconv.Itoa(ref.Number)
	return []library.TitleTarget{{ID: itemID(episode), Series: itemID(imdb), Season: itemID(imdb + ":" + strconv.Itoa(ref.Season)),
		SeasonNumber: ref.Season, Number: ref.Number, Runtime: 40 * time.Minute}}
}

func (f *titles) Resolve(_ context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error) {
	found := make([][]library.TitleTarget, len(refs))
	for i, ref := range refs {
		found[i] = f.target(ref)
	}
	return found, nil
}

func (f *titles) RemoteLookup(_ context.Context, kind library.Kind, id string) (library.RemoteResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.kinds[id] != kind {
		return library.RemoteResult{}, false, nil
	}
	return library.RemoteResult{Item: library.Item{Name: f.names[id], ProductionYear: 1994,
		ProviderIDs: map[string]string{"Imdb": id}}}, true, nil
}

func (f *titles) LookUpIDs(ctx context.Context, kind library.Kind, ids map[string]string) (*library.RemoteResult, error) {
	f.mu.Lock()
	imdb := f.tmdb[string(kind)+":"+ids["Tmdb"]]
	f.mu.Unlock()
	if imdb == "" {
		return nil, nil
	}
	result, ok, err := f.RemoteLookup(ctx, kind, imdb)
	if !ok {
		return nil, err
	}
	result.Item.ProviderIDs["Tmdb"] = ids["Tmdb"]
	return &result, err
}

// itemID names the item of a title or episode in the tests.
func itemID(id string) accounts.ID {
	var item accounts.ID
	copy(item[:], id)
	return item
}

type harness struct {
	*Store
	data       *userdata.Store
	user, peer accounts.ID
}

func setup(t *testing.T) harness {
	t.Helper()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	f := &titles{
		kinds: map[string]library.Kind{"tt0111161": library.KindMovie, "tt0903747": library.KindSeries},
		tmdb:  map[string]string{"movie:278": "tt0111161", "series:1396": "tt0903747"},
		names: map[string]string{"tt0111161": "Harbor Lights", "tt0903747": "Night Shift"},
	}
	data := userdata.New(pool)
	store := New(Options{DB: pool, Settings: users.Settings, Titles: f, UserData: data, Watchlist: watchlist.New(pool)})
	return harness{Store: store, data: data, user: user.ID, peer: peer.ID}
}

func (h harness) data1(t *testing.T, id string) userdata.Data {
	t.Helper()
	got, err := h.data.Get(t.Context(), h.user, []accounts.ID{itemID(id)})
	if err != nil {
		t.Fatal(err)
	}
	return got[itemID(id)]
}

var played = time.Date(2026, 3, 4, 20, 0, 0, 0, time.UTC)

func TestALinkAppliesWhatWasKeptAtOnce(t *testing.T) {
	t.Parallel()
	h := setup(t)
	entries := []Entry{
		{Key: "tmdb:9999", Label: Label{Kind: KindMovie, Name: "Harbor movie", IDs: "tmdb:9999"}, Reason: "not_found",
			Played: true, PlayedAt: &played, Plays: 3, Rating: new(8.0)},
		{Key: "kitsu:1", Label: Label{Kind: KindEpisode, Name: "Chemistry", IDs: "kitsu:1", Season: 2, Episode: 5}, Reason: "not_found",
			Resume: &userdata.ImportedResume{Position: 20 * time.Minute, At: played}},
		{Key: "kitsu:1", Label: Label{Kind: KindEpisode, Name: "Chemistry", IDs: "kitsu:1", Season: 2, Episode: 4}, Reason: "not_found",
			Played: true},
		{Key: "kitsu:1", Label: Label{Kind: KindSeries, Name: "Chemistry", IDs: "kitsu:1"}, Reason: "not_found",
			Watchlist: watchlist.Import},
	}
	if err := h.Save(t.Context(), h.user, "nuvio", entries, true); err != nil {
		t.Fatal(err)
	}
	if err := h.Save(t.Context(), h.peer, "nuvio", entries[:1], true); err != nil {
		t.Fatal(err)
	}
	list, err := h.List(t.Context(), h.user, "nuvio")
	if err != nil || len(list) != 4 || list[0].Name != "Chemistry" || list[0].Kind != KindSeries || list[1].Episode != 4 ||
		list[3].Rating == nil || *list[3].Rating != 8 || list[3].Plays != 3 || list[0].Link != nil {
		t.Fatalf("list: %+v %v", list, err)
	}

	// A series for a movie, a movie for an episode's series: refused.
	for key, input := range map[string]string{"tmdb:9999": "https://www.themoviedb.org/tv/1396", "kitsu:1": "tt0111161"} {
		if _, err := h.Link(t.Context(), h.user, h.user, "nuvio", key, input); !errors.Is(err, ErrKindMismatch) {
			t.Errorf("%s to %s: %v", key, input, err)
		}
	}
	if _, err := h.Link(t.Context(), h.user, h.user, "nuvio", "tmdb:9999", "tmdb:12345"); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("unknown TMDB: %v", err)
	}
	if _, err := h.Link(t.Context(), h.user, h.user, "nuvio", "tmdb:9999", "not an id"); !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("invalid: %v", err)
	}
	if _, err := h.Link(t.Context(), h.user, h.user, "stremio", "tmdb:9999", "tt0111161"); !errors.Is(err, ErrUnknownEntry) {
		t.Errorf("another source: %v", err)
	}
	if d := h.data1(t, "tt0111161"); d.Played {
		t.Fatalf("a refused link applied: %+v", d)
	}

	link, err := h.Link(t.Context(), h.user, h.user, "nuvio", "tmdb:9999", "278")
	if err != nil || link.IMDb != "tt0111161" || link.TMDB != 278 || link.Name != "Harbor Lights" || link.Year != 1994 ||
		link.LinkedBy == nil || *link.LinkedBy != h.user || link.LinkedByName != "alice" {
		t.Fatalf("movie link: %+v %v", link, err)
	}
	if d := h.data1(t, "tt0111161"); !d.Played || d.PlayCount != 3 || d.LastPlayed == nil || !d.LastPlayed.Equal(played) ||
		d.Rating == nil || *d.Rating != 8 {
		t.Errorf("movie data: %+v", d)
	}
	if _, err := h.Link(t.Context(), h.user, h.user, "nuvio", "kitsu:1", "https://www.imdb.com/title/tt0903747/"); err != nil {
		t.Fatal(err)
	}
	if d := h.data1(t, "tt0903747:2:4"); !d.Played {
		t.Errorf("episode 4: %+v", d)
	}
	if d := h.data1(t, "tt0903747:2:5"); d.Played || d.Position != 20*time.Minute || d.Runtime != 40*time.Minute {
		t.Errorf("episode 5: %+v", d)
	}
	// The series is on the watchlist, which shows it once its episodes
	// are no longer started.
	var lists []string
	if err := h.db.QueryRow(t.Context(), `SELECT array_agg(list) FROM watchlist WHERE user_id = $1 AND item_id = $2`, h.user,
		itemID("tt0903747")).Scan(&lists); err != nil || len(lists) != 1 || lists[0] != watchlist.Import {
		t.Errorf("watchlist: %v %v", lists, err)
	}
	// The peer's entry of the same key is theirs: nothing of it applied.
	if got, err := h.data.Get(t.Context(), h.peer, []accounts.ID{itemID("tt0111161")}); err != nil || got[itemID("tt0111161")].Played {
		t.Errorf("peer: %+v %v", got, err)
	}

	counts, err := h.Counts(t.Context(), h.user)
	if err != nil || len(counts) != 1 || counts[0] != (Count{Source: "nuvio", NotFound: 0, Linked: 4}) {
		t.Errorf("counts: %+v %v", counts, err)
	}
	linked, err := h.Linked(t.Context(), h.user, "nuvio")
	if err != nil || len(linked) != 2 {
		t.Fatalf("linked: %+v %v", linked, err)
	}
	if ref, ok := linked.Ref("kitsu:1", Label{Kind: KindEpisode, Season: 3, Episode: 1}); !ok ||
		ref != (library.TitleRef{Episode: true, IMDb: "tt0903747", Season: 3, Number: 1, Name: "Night Shift"}) {
		t.Errorf("episode ref: %+v %v", ref, ok)
	}
	if _, ok := linked.Ref("kitsu:1", Label{Kind: KindMovie}); ok {
		t.Error("a movie ref of a series link")
	}

	// A whole import that no longer holds them keeps the linked entries.
	if err := h.Save(t.Context(), h.user, "nuvio", nil, true); err != nil {
		t.Fatal(err)
	}
	if list, err := h.List(t.Context(), h.user, "nuvio"); err != nil || len(list) != 4 || list[0].Link == nil {
		t.Fatalf("after a whole import: %+v %v", list, err)
	}

	// Removed, the link applies no more; what it set stays, and the next
	// whole import drops the entry it no longer holds.
	if err := h.Unlink(t.Context(), h.user, "nuvio", "tmdb:9999"); err != nil {
		t.Fatal(err)
	}
	if err := h.Unlink(t.Context(), h.user, "nuvio", "tmdb:9999"); !errors.Is(err, ErrUnknownEntry) {
		t.Errorf("unlinked twice: %v", err)
	}
	if linked, err := h.Linked(t.Context(), h.user, "nuvio"); err != nil || len(linked) != 1 {
		t.Errorf("linked after removal: %+v %v", linked, err)
	}
	if d := h.data1(t, "tt0111161"); !d.Played {
		t.Errorf("unlinking took back: %+v", d)
	}
	if err := h.Save(t.Context(), h.user, "nuvio", nil, true); err != nil {
		t.Fatal(err)
	}
	if list, err := h.List(t.Context(), h.user, "nuvio"); err != nil || len(list) != 3 {
		t.Errorf("after removal: %+v %v", list, err)
	}
}

func TestAnIncrementalImportAddsToItsList(t *testing.T) {
	t.Parallel()
	h := setup(t)
	earlier := played.Add(-time.Hour)
	first := Entry{Key: "movie:imdb:tt0000001", Label: Label{Kind: KindMovie, IDs: "tt0000001"}, Reason: "not_found", Played: true,
		PlayedAt: &earlier, Plays: 2}
	if err := h.Save(t.Context(), h.user, "trakt", []Entry{first}, false); err != nil {
		t.Fatal(err)
	}
	second := Entry{Key: "movie:imdb:tt0000002", Label: Label{Kind: KindMovie, Name: "Two", IDs: "tt0000002"}, Reason: "not_found",
		Resume: &userdata.ImportedResume{Percent: 40, At: played}}
	again := first
	again.PlayedAt, again.Plays = &played, 1
	if err := h.Save(t.Context(), h.user, "trakt", []Entry{second, again}, false); err != nil {
		t.Fatal(err)
	}
	list, err := h.List(t.Context(), h.user, "trakt")
	if err != nil || len(list) != 2 || list[0].Name != "Two" || list[0].Resume == nil || list[0].Resume.Percent != 40 ||
		list[1].Plays != 2 || list[1].PlayedAt == nil || !list[1].PlayedAt.Equal(played) {
		t.Fatalf("list: %+v %v", list, err)
	}
}
