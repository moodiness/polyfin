package admin

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/logs"
)

// A request that fails is logged with the route it went to, never its
// path: an invite's token, which makes an account, is in the path of the
// invite's page, and administrators share the log. The API logs here as
// the server does, to the ring they read.
func TestAFailedInviteRequestIsLoggedByItsRoute(t *testing.T) {
	t.Parallel()
	ring := logs.NewRing(logs.Capacity)
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Logger, o.Logs, pool = slog.New(slog.NewTextHandler(ring, nil)), ring, deps.pool
	})
	_ = api.signedIn("root", true)
	users, err := api.store.Users(t.Context())
	if err != nil || len(users) != 1 {
		t.Fatalf("users: %v %v", users, err)
	}
	_, token, err := api.store.CreateInvite(t.Context(), accounts.NewInvite{MaxUses: 5, CreatedBy: users[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if status, body, _ := api.browser().call(http.MethodGet, "/invite/"+token, nil); status != http.StatusOK {
		t.Fatalf("the invite's page: %d %v", status, body)
	}
	// PostgreSQL restarts, or is out of connections: the guest's requests
	// fail, the page's and the account's.
	pool.Close()
	if status, body, _ := api.browser().call(http.MethodGet, "/invite/"+token, nil); status != http.StatusInternalServerError {
		t.Fatalf("the invite's page without a database: %d %v", status, body)
	}
	account := map[string]string{"name": "guest", "password": "correct horse battery"}
	if status, body, _ := api.browser().call(http.MethodPost, "/invite/"+token, account); status != http.StatusInternalServerError {
		t.Fatalf("the invite's account without a database: %d %v", status, body)
	}
	kept := string(ring.Contents())
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if !strings.Contains(kept, `msg="Admin API request failed" method=`+method+` path=/admin/api/invite/{token} error=`) {
			t.Errorf("the failed %s is not logged with its route:\n%s", method, kept)
		}
	}
	if strings.Contains(kept, token) {
		t.Errorf("the log holds the invite's token:\n%s", strings.ReplaceAll(kept, token, "<the token>"))
	}
}
