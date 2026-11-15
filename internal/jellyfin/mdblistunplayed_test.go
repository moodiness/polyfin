package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/trackers"
)

// mdblistRecord is an episode of the fake MDBList's history: MDBList
// keeps a removed record, its date cleared, and tells when it changed.
type mdblistRecord struct {
	show          string
	season        int
	number        int
	lastWatchedAt *time.Time
	updatedAt     time.Time
}

// statefulMDBList is a fake MDBList whose history follows what it is
// sent, as MDBList's documentation describes it: a removal clears the
// record's date and keeps it, with a new update time.
type statefulMDBList struct {
	mu      sync.Mutex
	records []*mdblistRecord
	// watchedAt is the episode_watched_at of /sync/last_activities, and
	// moveOnRemove tells that a removal moves it.
	watchedAt    time.Time
	moveOnRemove bool
	removals     int
	sinces       []string
	url          string
}

func newStatefulMDBList(t *testing.T, moveOnRemove bool, records ...*mdblistRecord) *statefulMDBList {
	t.Helper()
	f := &statefulMDBList{records: records, moveOnRemove: moveOnRemove}
	for _, record := range records {
		if record.updatedAt.After(f.watchedAt) {
			f.watchedAt = record.updatedAt
		}
	}
	stamp := func(at time.Time) string { return at.UTC().Format(time.RFC3339) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user":
			_, _ = io.WriteString(w, `{"username":"member"}`)
			return
		case "/sync/last_activities":
			_ = json.NewEncoder(w).Encode(map[string]string{"watched_at": "", "episode_watched_at": stamp(f.watchedAt)})
			return
		case "/sync/watched":
			if r.Method != http.MethodGet {
				break
			}
			since := r.URL.Query().Get("since")
			f.sinces = append(f.sinces, since)
			var after time.Time
			if since != "" {
				var err error
				if after, err = time.Parse(time.RFC3339, since); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			var episodes []string
			for _, record := range f.records {
				if since != "" && !record.updatedAt.After(after) {
					continue
				}
				watched := "null"
				if record.lastWatchedAt != nil {
					watched = `"` + stamp(*record.lastWatchedAt) + `"`
				}
				episodes = append(episodes, fmt.Sprintf(`{"last_watched_at":%s,"updated_at":"%s","episode":{"season":%d,"number":%d,"show":{"ids":{"imdb":"%s"}}}}`,
					watched, stamp(record.updatedAt), record.season, record.number, record.show))
			}
			_, _ = io.WriteString(w, `{"movies":[],"episodes":[`+strings.Join(episodes, ",")+`],"pagination":{"has_more":false}}`)
			return
		case "/sync/watched/remove":
			var body struct {
				Shows []struct {
					IDs struct {
						IMDb string `json:"imdb"`
					} `json:"ids"`
					Seasons []struct {
						Number   int `json:"number"`
						Episodes []struct {
							Number int `json:"number"`
						} `json:"episodes"`
					} `json:"seasons"`
				} `json:"shows"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			now := time.Now()
			for _, show := range body.Shows {
				for _, season := range show.Seasons {
					for _, episode := range season.Episodes {
						for _, record := range f.records {
							if record.show == show.IDs.IMDb && record.season == season.Number && record.number == episode.Number {
								record.lastWatchedAt, record.updatedAt = nil, now
							}
						}
					}
				}
			}
			if f.moveOnRemove {
				f.watchedAt = now
			}
			f.removals++
			_, _ = io.WriteString(w, `{}`)
			return
		case "/sync/playback":
			_, _ = io.WriteString(w, `[]`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// watch adds a play of an episode on MDBList itself, now, which moves its
// last activities.
func (f *statefulMDBList) watch(show string, season, number int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	f.records = append(f.records, &mdblistRecord{show: show, season: season, number: number, lastWatchedAt: &now, updatedAt: now})
	f.watchedAt = now
}

func (f *statefulMDBList) removed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removals
}

// An episode marked unplayed in Polyfin is removed from MDBList's history,
// which keeps its record without a date: the next imports leave it
// unplayed, whether MDBList's last activities move at the removal or
// only at a later play of another title.
func TestAnEpisodeMarkedUnplayedStaysUnplayedAfterMDBListImports(t *testing.T) {
	t.Parallel()
	for _, moveOnRemove := range []bool{true, false} {
		t.Run(fmt.Sprintf("activity moves on removal %v", moveOnRemove), func(t *testing.T) {
			t.Parallel()
			watched := time.Now().AddDate(0, 0, -300).Truncate(time.Second)
			fake := newStatefulMDBList(t, moveOnRemove,
				&mdblistRecord{show: "tt0801", season: 1, number: 1, lastWatchedAt: &watched, updatedAt: watched})
			var tracker *trackers.Service
			s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
				tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
					URLs: map[string]string{trackers.MDBList: fake.url}, Titles: options.Library, UserData: options.UserData})
				options.Trackers = tracker
			})
			t.Cleanup(tracker.Close)
			member := s.user("member", nil)
			addon, err := s.addons.Install(t.Context(), addons.Shared(), freshAddon(t), false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
				{AddonID: addon.ID, CatalogType: "series", CatalogID: "fresh"},
			}); err != nil {
				t.Fatal(err)
			}
			token := s.signIn("member", "tv")
			tr := tracking{testServer: s, token: token, user: member.ID.String(), views: map[string]string{}}
			var page QueryResult
			s.get(t, "/UserViews", token, &page)
			for _, view := range page.Items {
				tr.views[view.Name] = view.Id
			}
			series := map[string]string{}
			episodes := map[string]string{}
			s.get(t, "/Items?ParentId="+tr.views["Fresh"], token, &page)
			for _, show := range page.Items {
				series[show.Name] = show.Id
				var listed QueryResult
				s.get(t, "/Shows/"+show.Id+"/Episodes", token, &listed)
				for _, episode := range listed.Items {
					episodes[episode.Name] = episode.Id
				}
			}
			upcoming := func() []string { t.Helper(); return tr.list(t, "/Shows/Upcoming") }
			nextUp := func() []string { t.Helper(); return tr.list(t, "/Shows/NextUp") }

			// The first import marks the episode played on its date.
			if _, err := tracker.ConnectKey(t.Context(), member.ID, trackers.MDBList, "mdblist-member"); err != nil {
				t.Fatal(err)
			}
			if _, err := tracker.SetImport(t.Context(), member.ID, trackers.MDBList, true); err != nil {
				t.Fatal(err)
			}
			waitMDBListImport(t, tracker, member.ID, time.Time{})
			if data := tr.userData(t, episodes["Fresh 1"]); !data.Played || data.LastPlayedDate == nil {
				t.Fatalf("after the first import: %+v", data)
			}
			if got := upcoming(); !slices.Contains(got, episodes["Fresh 3"]) {
				t.Fatalf("upcoming after the first import: %v", got)
			}

			// Marked unplayed as an app does, which MDBList hears of.
			tr.mark(t, http.MethodDelete, "/UserPlayedItems/"+episodes["Fresh 1"])
			deadline := time.Now().Add(10 * time.Second)
			for fake.removed() == 0 {
				if time.Now().After(deadline) {
					t.Fatal("the removal never reached MDBList")
				}
				time.Sleep(5 * time.Millisecond)
			}

			importNow := func() {
				t.Helper()
				started := time.Now()
				if _, err := tracker.ImportNow(t.Context(), member.ID, trackers.MDBList); err != nil {
					t.Fatal(err)
				}
				waitMDBListImport(t, tracker, member.ID, started)
			}
			importNow()
			if !moveOnRemove {
				// The activity moves at the next play on MDBList, of
				// another series, whose read holds the removed record too.
				fake.watch("tt0802", 1, 1)
				importNow()
				if data := tr.userData(t, episodes["Dateless 1"]); !data.Played {
					t.Errorf("the later play was not imported: %+v", data)
				}
			}
			fake.mu.Lock()
			t.Logf("since reads: %q", fake.sinces)
			fake.mu.Unlock()
			data := tr.userData(t, episodes["Fresh 1"])
			t.Logf("Fresh 1 after the imports: played %v, last played %v", data.Played, data.LastPlayedDate)
			t.Logf("upcoming %v, next up %v (Fresh 2 %s, Fresh 3 %s, Dateless 2 %s)", upcoming(), nextUp(),
				episodes["Fresh 2"], episodes["Fresh 3"], episodes["Dateless 2"])
			var resume QueryResult
			s.get(t, "/UserItems/Resume", token, &resume)
			t.Logf("continue watching: %d items", len(resume.Items))
			if data.Played {
				t.Errorf("the unplayed episode is played again: %+v", data)
			}
			if got := upcoming(); slices.Contains(got, episodes["Fresh 2"]) || slices.Contains(got, episodes["Fresh 3"]) {
				t.Errorf("the series is back in Upcoming: %v", got)
			}
			if got := nextUp(); slices.Contains(got, episodes["Fresh 2"]) {
				t.Errorf("the series is back in Next Up: %v", got)
			}
		})
	}
}

// waitMDBListImport waits until an MDBList import that ended after since
// is done.
func waitMDBListImport(t *testing.T, tracker *trackers.Service, user accounts.ID, since time.Time) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		statuses, err := tracker.Statuses(t.Context(), user)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range statuses {
			if status.Service == trackers.MDBList && !status.Importing && status.LastImport != nil && !status.LastImport.At.Before(since) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the MDBList import never ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
