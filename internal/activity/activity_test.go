package activity

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// statements counts the statements a pool sends.
type statements struct{ n atomic.Int64 }

func (c *statements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *statements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Listing all the entries, as the admin app's Recent activity does every
// few seconds, counts them once: the total then follows each entry
// recorded and each sweep. A filtered listing still counts its own.
func TestTheTotalFollowsTheEntries(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	counter := &statements{}
	config := migrated.New(t).Config()
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	now := time.Now()
	store := New(pool, func() accounts.Settings { return accounts.Settings{Language: "en"} }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	store.now = func() time.Time { return now }
	user := accounts.User{ID: accounts.ID{1}, Name: "sam"}
	total := func(when string, want int) {
		t.Helper()
		page, err := store.Entries(ctx, Query{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		var counted int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM activity_log").Scan(&counted); err != nil {
			t.Fatal(err)
		}
		if page.Total != want || counted != want {
			t.Errorf("%s: total %d, %d in the log, want %d", when, page.Total, counted, want)
		}
	}
	listed := func(when string, query Query, want int64) {
		t.Helper()
		before := counter.n.Load()
		if _, err := store.Entries(ctx, query); err != nil {
			t.Fatal(err)
		}
		if sent := counter.n.Load() - before; sent != want {
			t.Errorf("%s: %d statements, want %d", when, sent, want)
		}
	}

	total("empty", 0)
	store.now = func() time.Time { return now.Add(-Retention - time.Hour) }
	store.SignInFailed(ctx, "sam", "192.0.2.1")
	store.now = func() time.Time { return now }
	store.SignedIn(ctx, user, "192.0.2.1")
	store.SignedIn(ctx, user, "192.0.2.2")
	total("after 3 entries", 3)
	listed("all the entries", Query{Limit: 10}, 1)
	listed("by type", Query{Limit: 10, Types: []string{"AuthenticationSucceeded"}}, 2)

	swept, err := store.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if swept != 1 {
		t.Errorf("swept %d", swept)
	}
	total("after the sweep", 2)
	listed("all the entries after the sweep", Query{Limit: 10}, 1)
}
