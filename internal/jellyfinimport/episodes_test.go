package jellyfinimport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// placedEpisode is an episode of a fixture: as the source server numbers
// it, as the addon does (addonSeason, addonNumber, 0 for one the addon
// leaves out), the day it aired, its name on each side, and its TMDB
// identifier, which both sides give when set.
type placedEpisode struct {
	season, number           int
	addonSeason, addonNumber int
	day, name, addonName     string
	tmdb                     int
}

// weeklyEpisodes are count episodes of season, aired a week apart from
// first, which the addon numbers from addonFirst in addonSeason.
func weeklyEpisodes(season, count int, first string, addonSeason, addonFirst int) []placedEpisode {
	start, err := time.Parse(time.DateOnly, first)
	if err != nil {
		panic(err)
	}
	episodes := make([]placedEpisode, count)
	for i := range episodes {
		episodes[i] = placedEpisode{season: season, number: i + 1, addonSeason: addonSeason, addonNumber: addonFirst + i,
			day: start.AddDate(0, 0, 7*i).Format(time.DateOnly), name: fmt.Sprintf("Chapter %d.%d", addonSeason, addonFirst+i)}
	}
	return episodes
}

// releasedTogether are count episodes of season released on day, from the
// number first, which the addon numbers from 1 in addonSeason, named
// otherwise on each side, with their TMDB identifiers.
func releasedTogether(season, first, count int, day string, addonSeason int) []placedEpisode {
	episodes := make([]placedEpisode, count)
	for i := range episodes {
		n := first + i
		episodes[i] = placedEpisode{season: season, number: n, addonSeason: addonSeason, addonNumber: i + 1, day: day,
			name: fmt.Sprintf("Episodio %d", n), addonName: fmt.Sprintf("Part %d.%d", addonSeason, i+1), tmdb: 960000 + 100*season + n}
	}
	return episodes
}

// The report's series: Lupin as TMDB numbers it, against the addon's
// parts; a season the source splits as TheTVDB does, the addon not;
// Survivor's second season, of which the addon leaves out the eighth; a
// series the source shows in DVD order, where the second and third
// swap; and one numbered alike.
func lupinEpisodes() []placedEpisode {
	return slices.Concat(releasedTogether(1, 1, 5, "2021-01-08", 1), releasedTogether(1, 6, 5, "2021-06-11", 2),
		releasedTogether(2, 1, 7, "2023-10-05", 3), releasedTogether(3, 1, 8, "2026-10-23", 4))
}

func splitEpisodes() []placedEpisode {
	return slices.Concat(weeklyEpisodes(1, 10, "2015-01-04", 1, 1), weeklyEpisodes(2, 10, "2015-09-06", 1, 11),
		weeklyEpisodes(3, 8, "2016-09-04", 2, 1))
}

func survivorEpisodes() []placedEpisode {
	second := weeklyEpisodes(2, 17, "2001-01-28", 2, 1)
	for i := range second {
		switch e := &second[i]; {
		case e.number == 8:
			e.addonSeason, e.addonNumber, e.name = 0, 0, "Back from the Outback"
		case e.number > 8:
			e.addonNumber--
		}
	}
	return append(weeklyEpisodes(1, 13, "2000-05-31", 1, 1), second...)
}

func dvdEpisodes() []placedEpisode {
	episodes := weeklyEpisodes(1, 6, "2019-01-06", 1, 1)
	episodes[1].number, episodes[2].number = 3, 2
	return episodes
}

// placedSeries is a fixture's series: its identifier on the source
// server, its IMDb identifier, and its name.
type placedSeries struct {
	id, imdb, name string
	episodes       []placedEpisode
}

func placedFixtures() []placedSeries {
	return []placedSeries{
		{"s-lupin", "tt2531336", "Lupin", lupinEpisodes()},
		{"s-split", "tt0001200", "Split", splitEpisodes()},
		{"s-survivor", "tt0239195", "Survivor", survivorEpisodes()},
		{"s-dvd", "tt0000600", "On DVD", dvdEpisodes()},
		{"s-alike", "tt0000400", "Alike", weeklyEpisodes(1, 8, "2021-02-07", 1, 1)},
	}
}

// serveDescriptions installs, in h's server, a metadata addon describing
// the series of fixtures by their IMDb identifiers, with each episode's
// TMDB identifier as a number, dating each the next day at 04:00 UTC.
func serveDescriptions(t *testing.T, h harness, fixtures []placedSeries) {
	t.Helper()
	metas := map[string]map[string]any{}
	for _, s := range fixtures {
		var videos []map[string]any
		for _, e := range s.episodes {
			if e.addonNumber == 0 {
				continue
			}
			day, err := time.Parse(time.DateOnly, e.day)
			if err != nil {
				t.Fatal(err)
			}
			video := map[string]any{"id": fmt.Sprintf("%s:%d:%d", s.imdb, e.addonSeason, e.addonNumber), "season": e.addonSeason,
				"episode": e.addonNumber, "name": cmpOr(e.addonName, e.name), "released": day.AddDate(0, 0, 1).Add(4 * time.Hour).Format(time.RFC3339)}
			if e.tmdb > 0 {
				video["moviedb_id"] = e.tmdb
			}
			videos = append(videos, video)
		}
		metas[s.imdb] = map[string]any{"id": s.imdb, "type": "series", "name": s.name, "videos": videos}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json")
		switch id, described := strings.CutPrefix(path, "meta/series/"); {
		case path == "manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt"}})
		case described && metas[id] != nil:
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": metas[id]})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	if _, err := h.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// jellyfinEpisode is the fixture's episode as Jellyfin lists it, dated at
// midnight in UTC+2, the evening before in UTC, with its TMDB identifier.
func jellyfinEpisode(s placedSeries, e placedEpisode, userData map[string]any) map[string]any {
	day, err := time.Parse(time.DateOnly, e.day)
	if err != nil {
		panic(err)
	}
	providers := map[string]string{}
	if e.tmdb > 0 {
		providers["Tmdb"] = strconv.Itoa(e.tmdb)
	}
	return map[string]any{"Id": fmt.Sprintf("%s-%d-%d", s.id, e.season, e.number), "Name": e.name, "Type": "Episode", "SeriesId": s.id,
		"SeriesName": s.name, "ParentIndexNumber": e.season, "IndexNumber": e.number, "ProviderIds": providers,
		"PremiereDate": day.Add(-2 * time.Hour).Format("2006-01-02T15:04:05.0000000Z"), "UserData": userData}
}

// placedPlay is an episode the user played, by its series and the
// source's numbers, and the addon's episode it marks, none (0) for one
// kept not found.
type placedPlay struct {
	series          string
	season, number  int
	addonSeason, at int
}

var placedPlays = []placedPlay{
	{"s-lupin", 1, 8, 2, 3}, {"s-lupin", 2, 1, 3, 1},
	{"s-split", 2, 3, 1, 13}, {"s-split", 3, 1, 2, 1},
	{"s-survivor", 2, 10, 2, 9}, {"s-survivor", 2, 8, 0, 0},
	{"s-dvd", 1, 2, 1, 3},
	{"s-alike", 1, 4, 1, 4},
}

// A Jellyfin or Emby server's episodes are placed among the addon's: by
// their own TMDB identifiers, else by the day they aired, rounded, and
// their names, against the episodes the server holds of each series,
// read once per series. Survivor's episode the addon leaves out is kept
// not found, never marked on another, and no episode the addon does not
// list is made up.
func TestServerEpisodesArePlacedAmongTheAddons(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{Jellyfin, Emby} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.f.emby = kind == Emby
			alice := h.user(t, "alice")
			fixtures := placedFixtures()
			serveDescriptions(t, h, fixtures)
			h.f.orders = map[string]string{"s-dvd": "dvd"}
			var played []map[string]any
			for _, s := range fixtures {
				h.f.series[s.id] = map[string]string{"Imdb": s.imdb}
				for _, e := range s.episodes {
					h.f.children[s.id] = append(h.f.children[s.id], jellyfinEpisode(s, e, nil))
					if slices.ContainsFunc(placedPlays, func(p placedPlay) bool {
						return p.series == s.id && p.season == e.season && p.number == e.number
					}) {
						played = append(played, jellyfinEpisode(s, e, map[string]any{"Played": true, "PlayCount": 1,
							"LastPlayedDate": "2026-09-01T20:00:00.0000000Z"}))
					}
				}
			}
			h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": played}
			if _, err := h.Start(t.Context(), Connection{Kind: kind, Address: h.f.url, Credentials: Credentials{Key: goodKey}}, accounts.User{},
				func(Server, []User, SignIn) ([]Target, error) {
					return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}}, nil
				}); err != nil {
				t.Fatal(err)
			}
			status := h.ended(t)
			if got := status.Users[0]; status.State != StateDone || got.Played != 7 || got.UnmatchedCount != 1 {
				t.Errorf("import: %+v", status)
			}
			if reasons := h.keptReasons(t, alice.ID, kind); len(reasons) != 1 || reasons["Survivor s2e8"] != ReasonEpisodeNotPlaced {
				t.Errorf("kept: %+v", reasons)
			}
			checkPlaced(t, h, alice.ID, fixtures)
			// Each series' episodes are read once, a page at a time, with
			// their days: Emby's when asked.
			pages := map[string]int{}
			for _, r := range h.f.sent() {
				if r.Method != http.MethodGet && !strings.HasSuffix(r.URL.Path, "/Users/AuthenticateByName") {
					t.Errorf("an import sent %s %s", r.Method, r.URL.Path)
				}
				query := r.URL.Query()
				if series := query.Get("ParentId"); series != "" {
					pages[series]++
					if query.Get("IncludeItemTypes") != "Episode" || strings.Contains(query.Get("Fields"), "PremiereDate") != h.f.emby {
						t.Errorf("episodes asked as %s", r.URL.RawQuery)
					}
				}
			}
			for _, s := range fixtures {
				if want := (len(s.episodes) + 1) / 2; pages[s.id] != want {
					t.Errorf("%s: %d pages of episodes, want %d", s.name, pages[s.id], want)
				}
			}
		})
	}
}

// checkPlaced checks that user's played marks are on the addon's episodes
// placedPlays names, on no other episode of the fixtures' series, and
// that only the episodes the addon lists are recorded.
func checkPlaced(t *testing.T, h harness, user accounts.ID, fixtures []placedSeries) {
	t.Helper()
	for _, s := range fixtures {
		rows, err := h.pool.Query(t.Context(), `SELECT i.key, coalesce(d.played, false) FROM items i
			LEFT JOIN user_data d ON d.item_id = i.id AND d.user_id = $2
			WHERE i.kind = 'episode' AND i.data->>'seriesId' = $1`, s.imdb, user)
		if err != nil {
			t.Fatal(err)
		}
		marked := map[string]bool{}
		for rows.Next() {
			var key string
			var played bool
			if err := rows.Scan(&key, &played); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(s.episodes, func(e placedEpisode) bool {
				return e.addonNumber > 0 && key == fmt.Sprintf("episode|%s:%d:%d", s.imdb, e.addonSeason, e.addonNumber)
			}) {
				t.Errorf("%s: %s recorded, which the addon does not list", s.name, key)
			}
			marked[key] = played
		}
		rows.Close()
		for key, played := range marked {
			wanted := slices.ContainsFunc(placedPlays, func(p placedPlay) bool {
				return p.series == s.id && p.at > 0 && key == fmt.Sprintf("episode|%s:%d:%d", s.imdb, p.addonSeason, p.at)
			})
			if played != wanted {
				t.Errorf("%s: %s played %v, want %v", s.name, key, played, wanted)
			}
		}
		for _, p := range placedPlays {
			if key := fmt.Sprintf("episode|%s:%d:%d", s.imdb, p.addonSeason, p.at); p.series == s.id && p.at > 0 && !marked[key] {
				t.Errorf("%s: S%dE%d not marked on %s", s.name, p.season, p.number, key)
			}
		}
	}
}

// A title not found keeps its episodes as the server holds them, and a
// link made by hand places them as the import does: an episode of a series
// no identifier found is placed by its TMDB identifier once linked, and
// one the import could not place is not marked by its numbers.
func TestALinkPlacesEpisodesAsTheImportDoes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice")
	fixtures := placedFixtures()
	serveDescriptions(t, h, fixtures)
	lupin, survivor := fixtures[0], fixtures[2]
	// The source knows Lupin by a TVDB identifier only, which no addon
	// describes.
	lupin.id = "s-lupin-tvdb"
	h.f.series = map[string]map[string]string{lupin.id: {"Tvdb": "999999"}, survivor.id: {"Imdb": survivor.imdb}}
	var played []map[string]any
	for _, s := range []placedSeries{lupin, survivor} {
		for _, e := range s.episodes {
			h.f.children[s.id] = append(h.f.children[s.id], jellyfinEpisode(s, e, nil))
		}
	}
	userData := map[string]any{"Played": true, "PlayCount": 1, "LastPlayedDate": "2026-09-01T20:00:00.0000000Z"}
	played = append(played, jellyfinEpisode(lupin, lupin.episodes[7], userData), jellyfinEpisode(survivor, survivor.episodes[13+7], userData))
	h.f.items["jf-alice"] = map[string][]map[string]any{"IsPlayed": played}
	if got := h.imported(t, Target{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}).Users[0]; got.Played != 0 || got.UnmatchedCount != 2 {
		t.Fatalf("import: %+v", got)
	}
	keys := map[string]string{}
	for _, row := range h.kept(t, alice.ID, Jellyfin) {
		keys[row.Name] = row.Key
		if row.Held == nil || row.Name == "Lupin" && (row.Held.TMDB != lupin.episodes[7].tmdb || row.Held.Otherwise) ||
			row.Name == "Survivor" && (!row.Held.Otherwise || row.Reason != ReasonEpisodeNotPlaced) {
			t.Errorf("kept: %+v %+v", row, row.Held)
		}
	}
	for name, imdb := range map[string]string{"Lupin": lupin.imdb, "Survivor": survivor.imdb} {
		if _, err := h.links.Link(t.Context(), alice.ID, alice.ID, string(Jellyfin), keys[name], imdb); err != nil {
			t.Fatal(err)
		}
	}
	marks := func(ref library.TitleRef) bool { t.Helper(); return h.dataOf(t, alice.ID, ref).Played }
	if !marks(library.TitleRef{Episode: true, IMDb: lupin.imdb, Season: 2, Number: 3}) {
		t.Error("Lupin's S1E8, linked, is not the addon's S2E3")
	}
	if n := countRecorded(t, h, survivor.imdb); n != 0 {
		t.Errorf("Survivor's S2E8, linked, recorded %d episodes", n)
	}
	// The next import places the linked series' episodes against the
	// server's list, as the link did.
	if got := h.imported(t, Target{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}).Users[0]; got.Played != 0 || got.UpToDate != 1 ||
		got.UnmatchedCount != 1 {
		t.Errorf("again: %+v", got)
	}
}

// countRecorded counts the episodes Polyfin keeps of the series imdb.
func countRecorded(t *testing.T, h harness, imdb string) int {
	t.Helper()
	var count int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE kind = 'episode' AND data->>'seriesId' = $1`, imdb).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// A Plex server's episodes are placed by their own Guid identifiers and
// originallyAvailableAt, which the episodes of the show sections give,
// asked with includeGuids: the owner's from the sections, another
// account's history through the episodes' rating keys.
func TestPlexEpisodesArePlacedByTheirGuidsAndDays(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	p := smallPlex(t)
	owner, carol := h.user(t, "owner"), h.user(t, "carol")
	fixtures := placedFixtures()
	serveDescriptions(t, h, fixtures)
	lupin, survivor := fixtures[0], fixtures[2]
	shows := []map[string]any{}
	var episodes []map[string]any
	key := func(s placedSeries, e placedEpisode) string { return fmt.Sprintf("%s-%d-%d", s.id, e.season, e.number) }
	watched := map[string]bool{key(lupin, lupin.episodes[7]): true, key(survivor, survivor.episodes[13+9]): true}
	for _, s := range []placedSeries{lupin, survivor} {
		shows = append(shows, map[string]any{"ratingKey": s.id, "type": "show", "title": s.name, "Guid": guids("imdb://" + s.imdb)})
		for _, e := range s.episodes {
			episode := map[string]any{"ratingKey": key(s, e), "type": "episode", "title": e.name, "grandparentRatingKey": s.id,
				"grandparentTitle": s.name, "parentIndex": e.season, "index": e.number, "originallyAvailableAt": e.day}
			if e.tmdb > 0 {
				episode["Guid"] = guids("tmdb://" + strconv.Itoa(e.tmdb))
			}
			if watched[key(s, e)] {
				episode["viewCount"], episode["lastViewedAt"] = 1, at("2026-09-04T20:00:00Z").Unix()
			}
			episodes = append(episodes, episode)
		}
	}
	p.items["2"] = map[string][]map[string]any{"2": shows, "4": episodes}
	p.history["5"] = []map[string]any{
		{"ratingKey": key(lupin, lupin.episodes[10]), "type": "episode", "title": "Episodio 1", "grandparentKey": "/library/metadata/" + lupin.id,
			"grandparentTitle": "Lupin", "parentIndex": 2, "index": 1, "viewedAt": at("2026-09-07T20:00:00Z").Unix(), "accountID": 5},
	}
	if _, err := h.Start(t.Context(), Connection{Kind: Plex, Address: p.url, Credentials: Credentials{Key: plexToken}}, accounts.User{},
		func(Server, []User, SignIn) ([]Target, error) {
			return []Target{{JellyfinID: "1", User: owner.ID, UserName: "owner"}, {JellyfinID: "5", User: carol.ID, UserName: "carol"}}, nil
		}); err != nil {
		t.Fatal(err)
	}
	if status := h.ended(t); status.State != StateDone || status.Users[0].Played != 2 || status.Users[1].Played != 1 {
		t.Fatalf("import: %+v", status)
	}
	for name, c := range map[string]struct {
		user accounts.ID
		ref  library.TitleRef
	}{
		"the owner's Lupin S1E8":     {owner.ID, library.TitleRef{Episode: true, IMDb: lupin.imdb, Season: 2, Number: 3}},
		"the owner's Survivor S2E10": {owner.ID, library.TitleRef{Episode: true, IMDb: survivor.imdb, Season: 2, Number: 9}},
		"Carol's Lupin S2E1":         {carol.ID, library.TitleRef{Episode: true, IMDb: lupin.imdb, Season: 3, Number: 1}},
	} {
		if d := h.dataOf(t, c.user, c.ref); !d.Played {
			t.Errorf("%s: %+v", name, d)
		}
	}
	if n := countRecorded(t, h, lupin.imdb); n != 2 {
		t.Errorf("Lupin: %d episodes recorded, want 2", n)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.requests {
		if r.URL.Query().Get("type") == "4" && r.URL.Query().Get("includeGuids") != "1" {
			t.Errorf("episodes listed without their Guid lists: %s", r.URL)
		}
	}
}

// Plex's guids and days, and the server's display orders, are read as
// each server writes them.
func TestServerEpisodeFieldsAreRead(t *testing.T) {
	t.Parallel()
	var p plexItemJSON
	if err := json.Unmarshal([]byte(`{"ratingKey":"7","type":"episode","parentIndex":1,"index":2,"title":"Second",
		"guid":"plex://episode/5d9c","Guid":[{"id":"imdb://tt0900002"},{"id":"tmdb://1234"},{"id":"tvdb://5678"}],
		"originallyAvailableAt":"2021-06-11"}`), &p); err != nil {
		t.Fatal(err)
	}
	held := p.held()
	if len(held) != 1 || held[0].Season != 1 || held[0].Number != 2 || held[0].TMDB != 1234 || held[0].TVDB != 5678 || held[0].Name != "Second" ||
		!held[0].Aired.Equal(time.Date(2021, 6, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Plex episode: %+v", held)
	}
	// An older agent's episode guid names its show: no identifier of the
	// episode's own.
	older := plexItemJSON{Type: "episode", ParentIndex: new(1), Index: new(2), PlexGUID: "com.plexapp.agents.thetvdb://81189/1/2?lang=en"}
	if held := older.held(); len(held) != 1 || held[0].TVDB != 0 || held[0].TMDB != 0 {
		t.Errorf("older agent's episode: %+v", held)
	}
	// Emby writes PremiereDate as Jellyfin does, and a file holding two
	// episodes gives the second its day, not its identifiers.
	var item itemJSON
	if err := json.Unmarshal([]byte(`{"Id":"e","Type":"Episode","Name":"Pilot","ParentIndexNumber":1,"IndexNumber":1,"IndexNumberEnd":2,
		"PremiereDate":"2021-06-10T22:00:00.0000000Z","ProviderIds":{"Tmdb":"1234","Tvdb":"5678"},"DisplayOrder":"Dvd"}`), &item); err != nil {
		t.Fatal(err)
	}
	held = heldEpisodes(item)
	if len(held) != 2 || held[0].TMDB != 1234 || held[1].TMDB != 0 || held[1].Name != "" ||
		!held[1].Aired.Equal(time.Date(2021, 6, 10, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("Jellyfin episode: %+v", held)
	}
	for order, other := range map[string]bool{"": false, "Aired": false, "aired": false, "official": false, "dvd": true, "Dvd": true,
		"absolute": true, "originalAirDate": true} {
		if otherOrder(order) != other {
			t.Errorf("%q: other order %v", order, !other)
		}
	}
}
