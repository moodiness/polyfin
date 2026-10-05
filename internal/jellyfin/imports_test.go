package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/trackers"
)

// manySeries is the number of series of seriesAddon.
const manySeries = 120

// seriesAddon serves a catalog of manySeries series of two released
// episodes each, and counts the descriptions of series it is asked for.
func seriesAddon(t *testing.T, metas *atomic.Int32) string {
	t.Helper()
	id := func(i int) string { return fmt.Sprintf("tt9%06d", i) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "many", Name: "Many", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "many", Name: "Many"}}})
		case strings.HasPrefix(path, "/catalog/series/many"):
			var list []stremio.Meta
			for i := range manySeries {
				list = append(list, stremio.Meta{ID: id(i), Type: "series", Name: fmt.Sprintf("Series %d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": list})
		case strings.HasPrefix(path, "/meta/series/"):
			metas.Add(1)
			series := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/series/"), ".json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: series, Type: "series", Name: series,
				Videos: []stremio.Video{
					{ID: series + ":1:1", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"},
					{ID: series + ":1:2", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00.000Z"},
				}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

func TestALargeImportedHistoryDoesNotFloodTheAddons(t *testing.T) {
	var metas atomic.Int32
	addonURL := seriesAddon(t, &metas)
	// MDBList's history: the first episode of every series, the most
	// recent first.
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = io.WriteString(w, `{"username":"member"}`)
		case "/sync/watched":
			var episodes []string
			for i := range manySeries {
				episodes = append(episodes, fmt.Sprintf(`{"last_watched_at":"%s","episode":{"season":1,"number":1,"show":{"ids":{"imdb":"tt9%06d"}}}}`,
					time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i)*time.Hour).Format(time.RFC3339), i))
			}
			_, _ = io.WriteString(w, `{"episodes":[`+strings.Join(episodes, ",")+`],"pagination":{"has_more":false}}`)
		case "/sync/playback":
			_, _ = io.WriteString(w, `[]`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(service.Close)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: service.URL}, Titles: options.Library, UserData: options.UserData})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), addonURL, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "series", CatalogID: "many"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id+"&Limit=200", token, &page)
	if len(page.Items) != manySeries {
		t.Fatalf("%d series listed", len(page.Items))
	}

	if _, err := tracker.ConnectKey(t.Context(), member.ID, trackers.MDBList, "mdblist-member"); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.SetImport(t.Context(), member.ID, trackers.MDBList, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		statuses, err := tracker.Statuses(t.Context(), member.ID)
		if err != nil {
			t.Fatal(err)
		}
		if last := statuses[2].LastImport; last != nil && !statuses[2].Importing {
			if last.Played != manySeries {
				t.Fatalf("import: %+v", last)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the import never ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := metas.Load()

	// Next Up offers the next episode of the series played last, without
	// describing every series of the history.
	var next QueryResult
	if status := s.get(t, "/Shows/NextUp", token, &next); status != http.StatusOK || len(next.Items) == 0 ||
		next.Items[0].SeriesName != "tt9000000" || next.Items[0].IndexNumber == nil || *next.Items[0].IndexNumber != 2 {
		t.Fatalf("next up: %d %+v", status, next.Items)
	}
	s.get(t, "/Shows/Upcoming", token, &next)
	var resume QueryResult
	s.get(t, "/UserItems/Resume", token, &resume)
	if asked := metas.Load() - before; asked == 0 || asked > 50 {
		t.Errorf("%d series described for Next Up, Upcoming and Continue Watching", asked)
	}
}
