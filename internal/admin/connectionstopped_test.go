package admin

import (
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/trackers"
)

// System › Health lists the app connections and tracking services that
// stopped, by user, then app or service, each opening its user's page;
// they are not told as health problems: their own event told them. A
// connection that works, or a service that only failed, is not listed.
func TestHealthListsStoppedConnections(t *testing.T) {
	t.Parallel()
	var options *Options
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		o.AppImport = appimport.New(appimport.Options{Settings: o.Accounts.Settings, Logger: logger, DB: deps.pool})
		t.Cleanup(o.AppImport.Close)
		o.Trackers = trackers.New(trackers.Options{DB: deps.pool, Settings: o.Accounts.Settings, Logger: logger})
		t.Cleanup(o.Trackers.Close)
		options, pool = o, deps.pool
	})
	admin := api.signedIn("root", true)
	api.signedIn("sam", false)
	api.signedIn("alex", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	id := func(name string) accounts.ID {
		return users[slices.IndexFunc(users, func(u accounts.User) bool { return u.Name == name })].ID
	}
	sam, alex := id("sam"), id("alex")
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO app_connections (user_id, app, sign_in, owned, connected_at, problem) VALUES
			($1, 'stremio', 'kept', true, now(), 'sign_in_refused'), ($1, 'nuvio', 'kept', true, now(), NULL),
			($2, 'nuvio', 'kept', true, now(), 'sign_in_refused')`, []any{sam, alex}},
		{`INSERT INTO tracking_connections (user_id, service, token, connected_at, problem) VALUES
			($1, 'trakt', 'kept', now(), 'reconnect'), ($1, 'mdblist', 'kept', now(), 'unreachable'),
			($2, 'simkl', 'kept', now(), NULL)`, []any{sam, alex}},
	} {
		if _, err := pool.Exec(t.Context(), statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	var answer struct {
		Problems []healthProblemJSON `json:"problems"`
	}
	if status := admin.raw(http.MethodGet, "/health/problems?language=en", "", &answer); status != http.StatusOK {
		t.Fatalf("problems: %d", status)
	}
	stopped := func(user accounts.ID, name, kind, service string) healthProblemJSON {
		return healthProblemJSON{Key: "connection_stopped:" + kind + ":" + service + ":" + user.String(), Code: problemConnectionStopped,
			Tone: "warning", To: "/users/" + user.String(), Transient: true, Name: service,
			Owner: &ownerJSON{ID: user.String(), Name: name}, Kind: kind}
	}
	want := []healthProblemJSON{
		stopped(alex, "alex", "app", "nuvio"),
		stopped(sam, "sam", "app", "stremio"),
		stopped(sam, "sam", "tracking", "trakt"),
	}
	if !reflect.DeepEqual(answer.Problems, want) {
		t.Errorf("problems:\n%+v\nwant\n%+v", answer.Problems, want)
	}
	if told, err := HealthProblems(*options)(t.Context()); err != nil || len(told) != 0 {
		t.Errorf("told: %+v %v", told, err)
	}
}
