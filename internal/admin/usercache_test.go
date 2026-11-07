package admin

import (
	"context"
	"net/http"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// userReads counts the statements of the accounts store that read the
// users table.
type userReads struct{ n atomic.Int64 }

var readsUsers = regexp.MustCompile(`(?i)\b(from|join)\s+users\b`)

func (c *userReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if readsUsers.MatchString(data.SQL) {
		c.n.Add(1)
	}
	return ctx
}

func (c *userReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Once the user is read, the session check of every admin request reads
// no users row; disabling the user still signs them out at their next
// request.
func TestWarmAdminRequestsReadNoUser(t *testing.T) {
	t.Parallel()
	reads := &userReads{}
	var store *accounts.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		config := deps.pool.Config()
		config.ConnConfig.Tracer = reads
		counted, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(counted.Close)
		if store, err = accounts.Open(t.Context(), counted); err != nil {
			t.Fatal(err)
		}
		o.Accounts = store
	})
	b := api.signedIn("alice", false)
	if status, body, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Fatalf("session: %d %v", status, body)
	}
	reads.n.Store(0)
	for range 3 {
		if status, body, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusOK {
			t.Fatalf("session: %d %v", status, body)
		}
	}
	if n := reads.n.Load(); n != 0 {
		t.Errorf("3 warm admin requests read users %d times", n)
	}
	alice, err := store.Authenticate(t.Context(), "alice", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateUser(t.Context(), alice.ID, accounts.UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, body, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusUnauthorized {
		t.Errorf("session of a disabled user: %d %v", status, body)
	}
}
