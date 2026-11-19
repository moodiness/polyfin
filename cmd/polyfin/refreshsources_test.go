package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// Refresh the sources is a Library task named in the server's six
// languages, run every day at 3:30 by default, on a server upgraded too:
// daily, weekly or every hour at most once its triggers are changed. Run,
// it downloads the addons' manifests again and logs what it did.
func TestRefreshTheSourcesTask(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	users, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var added atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		catalogs := []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}
		if added.Load() {
			catalogs = append(catalogs, stremio.Catalog{Type: "movie", ID: "new", Name: "New"})
		}
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "reels", Name: "Reels", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: catalogs})
	}))
	t.Cleanup(server.Close)
	sources := addons.New(pool, stremio.NewClient("test"))
	installed, err := sources.Install(ctx, addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	var logged lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))

	task := refreshSourcesTask(sources, users, logger)
	if task.Key != "RefreshSources" || task.Category != tasks.CategoryLibrary || task.Limits.Locked != "" ||
		task.Limits.MinInterval != time.Hour || task.Limits.KeepOne ||
		len(task.Limits.Types) != 3 || !slices.Equal(task.Triggers, []tasks.Trigger{tasks.Daily(3*time.Hour + 30*time.Minute)}) {
		t.Errorf("not a Library task every day at 3:30 by default: %+v", task)
	}
	for _, language := range []string{"en", "fr", "de", "es", "it", "pt-BR"} {
		if text := task.Text[language]; text.Name == "" || text.Description == "" {
			t.Errorf("no %s text: %+v", language, text)
		}
	}
	if name := task.Text["fr"].Name; name != "Actualiser les sources" {
		t.Errorf("French name %q", name)
	}
	// A server upgraded: its database has the triggers an administrator
	// changed and the last runs of the tasks it had, none of this one.
	store := tasks.NewStore(pool)
	if err := store.SaveTriggers(ctx, "ReadCollections", []tasks.Trigger{tasks.Daily(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(ctx, "WarmLibraries", tasks.Result{Start: time.Now(), End: time.Now(), Status: tasks.Completed}); err != nil {
		t.Fatal(err)
	}
	registry := tasks.New(logger)
	registry.Register(task)
	if err := registry.Load(ctx, store); err != nil {
		t.Fatal(err)
	}
	// Stopped once read, so that the scheduled run never comes.
	scheduling, stop := context.WithCancel(ctx)
	registry.Start(scheduling)
	info, _ := registry.Task(tasks.ID(task.Key), "en")
	stop()
	if info.Custom || !slices.Equal(info.Triggers, task.Triggers) || info.Next == nil || info.Next.Hour() != 3 || info.Next.Minute() != 30 ||
		!info.Next.After(time.Now()) {
		t.Errorf("not every day at 3:30 on a server upgraded: %+v", info)
	}
	// Checked apart: a trigger saved would schedule a run.
	for _, refused := range [][]tasks.Trigger{{tasks.Every(30 * time.Minute)}, {tasks.AtStartup()}} {
		if err := task.Limits.Check(refused); !errors.Is(err, tasks.ErrInvalidTriggers) {
			t.Errorf("triggers %+v: %v", refused, err)
		}
	}
	if err := task.Limits.Check([]tasks.Trigger{tasks.Every(time.Hour), tasks.Daily(4 * time.Hour)}); err != nil {
		t.Errorf("an hourly and a daily trigger: %v", err)
	}

	added.Store(true)
	if err := task.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if addon, err := sources.Find(ctx, installed.ID); err != nil || len(addon.Manifest.Catalogs) != 2 {
		t.Errorf("the addon after the run: %+v %v", addon.Manifest.Catalogs, err)
	}
	if line := `level=INFO msg="Sources refreshed" refreshed=1 held=0 failed=0 skipped=0`; !strings.Contains(logged.String(), line) {
		t.Errorf("log %q lacks %q", logged.String(), line)
	}
}
