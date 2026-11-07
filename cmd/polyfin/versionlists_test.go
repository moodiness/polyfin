package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// lockedBuffer is a log the server's goroutines write while the test
// reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Forget the version lists is a Library task, run by hand only, named in
// the server's six languages. Run from the schedule, it forgets the
// version lists, in memory and saved, and logs how many it forgot.
func TestForgetTheVersionListsTask(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	movie := stremio.Meta{ID: "tt0001", Type: "movie", Name: "First"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "reels", Name: "Reels", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}, {Name: "stream", IDPrefixes: []string{"tt"}}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt0001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt0001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "First", URL: "https://cdn.example/first.mkv"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	if _, err := addonStore.Install(ctx, addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	var logged lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	lib := library.New(pool, addonStore, client, logger, store.Settings)
	lib.SetFollowUps()
	libraries, err := lib.Libraries(ctx, user)
	if err != nil || len(libraries) != 1 {
		t.Fatalf("libraries: %v %v", libraries, err)
	}
	page, err := lib.Children(ctx, user, libraries[0].ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the library: %v %v", page.Items, err)
	}
	title := page.Items[0].ID
	if versions, err := lib.Versions(ctx, user, title); err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	saved := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM stream_lists").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := saved(); n != 1 {
		t.Fatalf("%d lists saved, want 1", n)
	}

	task := forgetVersionListsTask(lib, logger)
	if task.Category != tasks.CategoryLibrary || task.Interval != 0 || task.Daily != nil || task.AtStart {
		t.Errorf("not a Library task run by hand only: %+v", task)
	}
	for _, language := range []string{"en", "fr", "de", "es", "it", "pt-BR"} {
		if text := task.Text[language]; text.Name == "" || text.Description == "" {
			t.Errorf("no %s text: %+v", language, text)
		}
	}
	registry := tasks.New(logger)
	registry.Register(task)
	registry.Start(ctx)
	id := tasks.ID(task.Key)
	if err := registry.Run(id); err != nil {
		t.Fatal(err)
	}
	var last *tasks.Result
	for deadline := time.Now().Add(5 * time.Second); last == nil; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the task to end")
		}
		info, _ := registry.Task(id, "en")
		last = info.Last
	}
	if last.Status != tasks.Completed {
		t.Fatalf("the task ended %s: %s", last.Status, last.Error)
	}
	if versions, err := lib.KnownVersions(ctx, user, title); err != nil || len(versions) != 0 {
		t.Errorf("versions known once forgotten: %v %v", versions, err)
	}
	if n := saved(); n != 0 {
		t.Errorf("%d lists saved once forgotten", n)
	}
	if line := `level=INFO msg="Version lists forgotten" lists=1`; !strings.Contains(logged.String(), line) {
		t.Errorf("log %q lacks %q", logged.String(), line)
	}
}
