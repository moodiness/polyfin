package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// Clean up removed sources, whose every attempt meets a change of the
// addons it reads, leaves the cleanup to its next run: it says so at Info
// level, and its run completes, with no task failure logged.
func TestTheCleanupTaskLeavesSourcesChangingToTheNextRun(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var logged lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	lib := library.New(pool, addonStore, client, logger, store.Settings)
	folders := localfiles.New(pool, addonStore, lib, logger, store.Settings, nil)
	t.Cleanup(folders.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.Trim(strings.TrimSuffix(r.URL.Path, "manifest.json"), "/")
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: name, Name: name, Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
	}))
	t.Cleanup(server.Close)

	// As many addons as attempts, each changed by a connection of its own
	// whose change is not committed yet: each attempt waits for one.
	changes := map[uint32]pgx.Tx{}
	for _, name := range []string{"one", "two", "three"} {
		addon, err := addonStore.Install(ctx, addons.Shared(), server.URL+"/"+name+"/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := pgx.Connect(ctx, testdb.CopyURL(t, pool))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		change, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := change.Exec(ctx, "UPDATE addons SET manifest = manifest WHERE id = $1", addon.ID); err != nil {
			t.Fatal(err)
		}
		changes[conn.PgConn().PID()] = change
	}

	task := cleanupTask(lib, folders, logger)
	task.Triggers = nil
	registry := tasks.New(logger)
	registry.Register(task)
	registry.Start(ctx)
	id := tasks.ID(task.Key)
	if err := registry.Run(id); err != nil {
		t.Fatal(err)
	}
	// Each change commits once an attempt waits for it, which fails the
	// attempt.
	deadline := time.Now().Add(time.Minute)
	for len(changes) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d changes no attempt waited for; log %q", len(changes), logged.String())
		}
		var blocking []int32
		if err := pool.QueryRow(ctx, `SELECT coalesce(array_agg(b), '{}') FROM pg_stat_activity, unnest(pg_blocking_pids(pid)) b
			WHERE datname = current_database()`).Scan(&blocking); err != nil {
			t.Fatal(err)
		}
		for _, pid := range blocking {
			if change, ok := changes[uint32(pid)]; ok {
				if err := change.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				delete(changes, uint32(pid))
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	var info tasks.Info
	for info, _ = registry.Task(id, "en"); info.Last == nil; info, _ = registry.Task(id, "en") {
		if time.Now().After(deadline) {
			t.Fatalf("the run did not end; log %q", logged.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info.Last.Status != tasks.Completed || info.Last.Error != "" {
		t.Errorf("the run ended %+v", info.Last)
	}
	log := logged.String()
	if line := `level=INFO msg="The cleanup of removed sources waits for the next run, as sources changed meanwhile"`; !strings.Contains(log, line) {
		t.Errorf("log %q lacks %q", log, line)
	}
	if strings.Contains(log, "A task failed") {
		t.Errorf("log %q tells of a failure", log)
	}
}
