package jellyfin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/trackers"
)

// A Recommended on Trakt row lists, for a user with a Trakt connection, the
// movies and series Trakt recommends to them, found by their identifiers,
// without those they played, hid or may not see. A user without a
// connection sees no row, no library and no folder of it.
func TestTraktRowListsWhatTraktRecommends(t *testing.T) {
	t.Parallel()
	// Trakt recommends the catalog's three movies, one it names by no
	// identifier, and the series.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/recommendations/movies":
			_, _ = io.WriteString(w, `[{"title":"Movie 2","year":2020,"ids":{"trakt":3,"imdb":"tt0002","tmdb":null}},
				{"title":"Unknown","year":2020,"ids":{"trakt":4,"imdb":null,"tmdb":null}},
				{"title":"Movie 0","year":2020,"ids":{"trakt":1,"imdb":"tt0000","tmdb":null}},
				{"title":"Movie 1","year":2020,"ids":{"trakt":2,"imdb":"tt0001","tmdb":null}}]`)
		case "/recommendations/shows":
			_, _ = io.WriteString(w, `[{"title":"Show","year":2010,"ids":{"trakt":5,"imdb":"tt0100","tvdb":20,"tmdb":null}}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	var tracker *trackers.Service
	s, member, admin, _, movies := homeRowsServer(t, func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.Trakt: fake.URL}, Titles: options.Library, UserData: options.UserData})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	s.setting(t, func(settings *accounts.Settings) {
		settings.TraktClientID, settings.TraktClientSecret = "trakt-client", "trakt-secret"
	})
	row := s.addRow(t, homerows.Row{Name: "Recommended on Trakt", Kind: homerows.KindTrakt, Place: homerows.PlaceTop, Everyone: true,
		Library: true})
	folder := customRowID(row.ID, accounts.ID{}).String()

	// The member connected Trakt.
	var me UserDto
	s.get(t, "/Users/Me", member, &me)
	memberID, _ := accounts.ParseID(me.Id)
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO tracking_connections (user_id, service, token, connected_at)
		VALUES ($1, 'trakt', 'trakt-member', now())`, memberID); err != nil {
		t.Fatal(err)
	}

	// The first read starts as the row first shows.
	var rows []CustomRowDto
	deadline := time.Now().Add(10 * time.Second)
	for {
		if rows = s.customRowsOf(t, member); len(rows) == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := rowSummary(rows); !slices.Equal(got, []string{"Recommended on Trakt @Top: Movie 2, Show, Movie 0, Movie 1"}) {
		t.Fatalf("the member's rows: %q", got)
	}
	if rows[0].Kind != "Trakt" || rows[0].Id != folder {
		t.Errorf("the row: %+v", rows[0])
	}
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	if !slices.Contains(itemNames(views.Items), "Recommended on Trakt") {
		t.Errorf("the member's libraries: %v", itemNames(views.Items))
	}

	// Without a connection: no row, no library, no folder.
	if got := rowSummary(s.customRowsOf(t, admin)); len(got) != 0 {
		t.Errorf("the administrator's rows: %q", got)
	}
	s.get(t, "/UserViews", admin, &views)
	if slices.Contains(itemNames(views.Items), "Recommended on Trakt") {
		t.Errorf("the administrator's libraries: %v", itemNames(views.Items))
	}
	var described BaseItemDto
	if status := s.get(t, "/Items/"+folder, admin, &described); status != http.StatusNotFound {
		t.Errorf("the folder for the administrator: %d %+v", status, described)
	}

	// The member plays Movie 0 and hides Movie 1; parental control then
	// hides the series, rated TV-14.
	if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+movies[0].Id, app("tv", member), nil); status != http.StatusOK {
		t.Fatalf("playing: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+movies[1].Id+"/Hidden", app("tv", member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Recommended on Trakt @Top: Movie 2, Show"}) {
		t.Errorf("played and hidden: %q", got)
	}
	if _, err := s.store.UpdateUser(t.Context(), memberID, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Recommended on Trakt @Top: Movie 2"}) {
		t.Errorf("with parental control: %q", got)
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+folder, member, &listed); status != http.StatusOK ||
		!slices.Equal(itemNames(listed.Items), []string{"Movie 2"}) || listed.TotalRecordCount != 1 {
		t.Errorf("the row's list: %d %v of %d", status, itemNames(listed.Items), listed.TotalRecordCount)
	}
}
