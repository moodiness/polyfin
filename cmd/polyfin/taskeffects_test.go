package main

import (
	"log/slog"
	"testing"

	"github.com/moodiness/polyfin/internal/tasks"
)

// The summaries and the watchlist check, which notify users, say that
// they send, so that the admin app asks before running them; adding the
// episodes of watchlist series sends nothing.
func TestTasksThatNotifySaySo(t *testing.T) {
	t.Parallel()
	registry := tasks.New(slog.New(slog.DiscardHandler))
	registerSummaryTasks(registry, nil)
	registerWatchlistTasks(registry, nil, nil, nil, nil, slog.New(slog.DiscardHandler))
	want := map[string]tasks.Effect{
		"SendWeeklySummary":  tasks.Sends,
		"SendMonthlySummary": tasks.Sends,
		"SendYearlySummary":  tasks.Sends,
		"CheckWatchlists":    tasks.Sends,
		"ExpandWatchlists":   "",
	}
	infos := registry.Tasks("en")
	if len(infos) != len(want) {
		t.Fatalf("%d tasks, want %d", len(infos), len(want))
	}
	for _, info := range infos {
		if effect, ok := want[info.Key]; !ok || info.Effect != effect {
			t.Errorf("%s: effect %q, want %q", info.Key, info.Effect, effect)
		}
	}
}
