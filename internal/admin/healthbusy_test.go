package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/database"
)

// While every connection of the database's pool is in use, the health
// answers once its first database check waits out its deadline: the
// database reachable but busy, what the other checks tell left out, and
// no failure.
func TestHealthFindsTheDatabaseBusyWhileItsPoolIsFull(t *testing.T) {
	t.Parallel()
	var small *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		config := deps.pool.Config()
		config.MaxConns, config.MinConns = 1, 0
		pool, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		small = pool
		o.Database = pool
		o.Health = HealthSources{DatabaseSize: func(ctx context.Context) (int64, error) { return database.Size(ctx, pool) }}
	})
	admin := api.signedIn("root", true)

	var health healthJSON
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK ||
		!health.Database.Reachable || health.Database.Busy || health.Database.Size == nil {
		t.Fatalf("health: %d %+v", status, health.Database)
	}

	held, err := small.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	health = healthJSON{}
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK ||
		!health.Database.Reachable || !health.Database.Busy || health.Database.Size != nil {
		t.Errorf("health while the pool is full: %d %+v", status, health.Database)
	}
}
