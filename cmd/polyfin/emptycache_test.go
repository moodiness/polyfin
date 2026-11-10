package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/tasks"
)

// Empty the cache of files being read is a Maintenance task named in the
// server's six languages, with no trigger until the settings give it an
// hour. Run by hand, it empties the cache and logs the bytes it freed.
func TestEmptyTheSourceCacheTask(t *testing.T) {
	t.Parallel()
	// A part a previous run kept: 100 bytes of the first chunk, read an
	// hour ago.
	dir := t.TempDir()
	folder := filepath.Join(dir, accounts.ID{1}.String()+".blocks")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	read := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	state := fmt.Sprintf(`{"link":"digest","size":100,"blocks":[1],"chunks":[{"index":0,"read":%q}]}`, read)
	for name, content := range map[string]string{"state": state, "0": strings.Repeat("x", 100)} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cache, err := source.New(dir, func() source.Policy { return source.Policy{Limit: 1 << 30, Keep: 24 * time.Hour} }, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if used := cache.Usage().Used; used != 100 {
		t.Fatalf("%d bytes kept, want 100", used)
	}

	var logged lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	task := emptySourceCacheTask(cache, logger)
	if task.Category != tasks.CategoryMaintenance || len(task.Triggers) != 0 || task.Limits.Locked != "" || task.Limits.MinInterval != 6*time.Hour {
		t.Errorf("not a Maintenance task without triggers by default: %+v", task)
	}
	for _, language := range []string{"en", "fr", "de", "es", "it", "pt-BR"} {
		if text := task.Text[language]; text.Name == "" || text.Description == "" {
			t.Errorf("no %s text: %+v", language, text)
		}
	}
	if name := task.Text["fr"].Name; name != "Vider le cache des fichiers lus" {
		t.Errorf("French name %q", name)
	}
	registry := tasks.New(logger)
	registry.Register(task)
	registry.Start(t.Context())
	id := tasks.ID(task.Key)
	if info, _ := registry.Task(id, "en"); info.Next != nil || len(info.Triggers) != 0 {
		t.Errorf("a trigger by default: %+v", info)
	}
	if err := registry.SetTriggers(t.Context(), id, []tasks.Trigger{tasks.Daily(4 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if info, _ := registry.Task(id, "en"); info.Next == nil || info.Next.Hour() != 4 {
		t.Errorf("no trigger at 4: %+v", info)
	}

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
	if used := cache.Usage().Used; used != 0 {
		t.Errorf("%d bytes kept once emptied", used)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("the source's folder stays: %v", err)
	}
	if line := `level=INFO msg="The cache of files being read was emptied" freed_bytes=100`; !strings.Contains(logged.String(), line) {
		t.Errorf("log %q lacks %q", logged.String(), line)
	}
}
