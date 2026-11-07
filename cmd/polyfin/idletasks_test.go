package main

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/taste"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// statements keeps the SQL statements a pool sends, their spaces folded.
type statements struct {
	mu   sync.Mutex
	sent []string
}

func (s *statements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, strings.Join(strings.Fields(data.SQL), " "))
	return ctx
}

func (s *statements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// take returns the statements sent since the last take.
func (s *statements) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent := s.sent
	s.sent = nil
	return sent
}

// idleTask is a background task's tick, and the statements it may send
// when there is nothing to do, by their start.
type idleTask struct {
	key     string
	tick    func(ctx context.Context) error
	allowed []string
}

// idleServer is a server whose Live TV, share links and local folders are
// off, with a user who played a movie and saved it to their watchlist,
// both checked once: the tasks' ticks then find nothing to do. sent is
// the tracer of pool.
func idleServer(t *testing.T, pool *pgxpool.Pool, sent *statements) []idleTask {
	t.Helper()
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	lib := library.New(pool, addonStore, client, logger, store.Settings)
	channels := iptv.New(pool, addonStore, client, logger, store.Settings)
	lib.UseIPTV(channels)
	folders := localfiles.New(pool, addonStore, lib, logger, store.Settings, nil)
	t.Cleanup(folders.Close)
	data := userdata.New(pool)
	tastes := taste.New(pool, store, data, lib, logger)
	lists := watchlist.New(pool)
	checker := watchlist.NewChecker(watchlist.CheckerOptions{Store: lists, Users: store.Users,
		Hours: func() int { return store.Settings().WatchlistCheckHours },
		Find: func(context.Context, accounts.User, accounts.ID) (watchlist.Found, error) {
			return watchlist.Missing, nil
		}, Logger: logger})

	users, err := store.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		user, err := store.CreateUser(ctx, accounts.NewUser{Name: "member", Password: "correct horse"})
		if err != nil {
			t.Fatal(err)
		}
		movie := accounts.ID{1}
		if _, err := data.Change(ctx, user.ID, []userdata.Item{{ID: movie}}, func(d *userdata.Data) {
			d.Played, d.PlayCount, d.LastPlayed = true, 1, new(time.Now())
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := lists.Save(ctx, user.ID, watchlist.Import, []watchlist.Title{{ID: movie}}, false); err != nil {
			t.Fatal(err)
		}
	}
	// The addons are served from memory once their changes are followed,
	// as the server does.
	go addonStore.Watch(ctx, logger)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		sent.take()
		if _, err := addonStore.Addons(ctx, addons.Shared()); err != nil {
			t.Fatal(err)
		}
		if len(sent.take()) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the addons are still read from the database")
		}
	}
	// What a first run does is done.
	if err := tastes.BuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check(ctx); err != nil {
		t.Fatal(err)
	}
	return []idleTask{
		// The first tick sweeps past programmes, as one tick an hour does.
		{"RefreshLiveTvGuides", func(ctx context.Context) error {
			if err := channels.RefreshDue(ctx, false); err != nil {
				return err
			}
			return lib.RefreshGuides(ctx, false)
		}, []string{
			"SELECT a.id, a.kind, a.manifest_url, a.manifest, a.enabled, a.refreshed_at, a.owner_id IS NOT NULL AND NOT coalesce(u.is_administrator, false), i.checked_at, i.next_try_at, i.error FROM iptv_sources i ",
			"SELECT a.id, a.kind, a.manifest_url, a.manifest, a.enabled, a.refreshed_at, a.owner_id, coalesce(u.is_administrator, true), l.catalog_type, l.catalog_id FROM libraries l ",
			"SELECT id, next_try_at FROM live_guides WHERE next_try_at IS NOT NULL",
			"SELECT EXISTS (SELECT 1 FROM items WHERE kind IN ('program', 'replay'))",
		}},
		{"RecordExpiredShareLinks", func(ctx context.Context) error {
			_, err := store.ExpiredShareLinks(ctx)
			return err
		}, nil},
		{"ScanLocalFolders", func(ctx context.Context) error { return folders.ScanDue(ctx, false) }, nil},
		// A user whose data did not change keeps their profile.
		{"BuildTasteProfiles", tastes.BuildAll, []string{
			"SELECT id, name, ",
			"SELECT item_id, coalesce(series_id, '00000000-0000-0000-0000-000000000000'), coalesce(season_id, '00000000-0000-0000-0000-000000000000'), played, play_count, position_ticks, runtime_ticks, favorite, rating, last_played_at, resume_removed_at FROM user_data WHERE user_id = $1 AND (played OR favorite ",
			"SELECT item_id, hidden_at FROM hidden_titles WHERE user_id = $1 ",
		}},
		// No title is due.
		{"CheckWatchlists", checker.Check, []string{
			"SELECT id, name, ",
			"SELECT w.item_id FROM watchlist w ",
		}},
	}
}

// With Live TV, share links and local folders off, and nothing changed
// since the last run, a tick of each background task sends only the
// statements that find there is nothing to do.
func TestIdleTicksSendOnlyTheirChecks(t *testing.T) {
	t.Parallel()
	sent := &statements{}
	config := migrated.New(t).Config()
	config.ConnConfig.Tracer = sent
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, task := range idleServer(t, pool, sent) {
		sent.take()
		if err := task.tick(t.Context()); err != nil {
			t.Fatalf("%s: %v", task.key, err)
		}
		for _, statement := range sent.take() {
			if !slices.ContainsFunc(task.allowed, func(allowed string) bool { return strings.HasPrefix(statement, allowed) }) {
				t.Errorf("%s sent %q", task.key, statement)
			}
		}
	}
}
