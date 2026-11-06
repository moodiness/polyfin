package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminAppUseIsTheUsersLastActivity(t *testing.T) {
	t.Parallel()
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { pool = deps.pool })
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	if _, err := pool.Exec(t.Context(), "UPDATE users SET last_activity_at = now() - interval '2 days'"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := member.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Fatalf("member's session: %d", status)
	}
	// The list itself is the administrator's use of the admin app.
	recent := time.Now().Add(-time.Hour)
	for _, user := range api.users(t, administrator) {
		at, _ := user["lastActivityAt"].(string)
		active, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || active.Before(recent) {
			t.Errorf("%s last active at %q", user["name"], at)
		}
	}
}
