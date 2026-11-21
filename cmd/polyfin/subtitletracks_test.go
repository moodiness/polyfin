package main

import (
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/tasks"
)

// Clean the kept subtitle tracks is a Maintenance task named in the
// server's six languages, each telling the days a track is kept, which
// runs at start and every day.
func TestCleanTheKeptSubtitleTracksTask(t *testing.T) {
	t.Parallel()
	task := subtitleTracksTask(nil, slog.New(slog.DiscardHandler))
	if task.Key != "CleanSubtitleTracks" || task.Category != tasks.CategoryMaintenance || task.Limits.Locked != "" ||
		task.Limits.MinInterval != time.Hour {
		t.Errorf("not an unlocked Maintenance task: %+v", task)
	}
	if want := []tasks.Trigger{tasks.AtStartup(), tasks.Every(24 * time.Hour)}; !slices.Equal(task.Triggers, want) {
		t.Errorf("triggers %+v, want %+v", task.Triggers, want)
	}
	days := strconv.Itoa(playback.TrackKeepDays)
	for _, language := range []string{"en", "fr", "de", "es", "it", "pt-BR"} {
		if text := task.Text[language]; text.Name == "" || !strings.Contains(text.Description, days) {
			t.Errorf("the %s text does not tell the %s days: %+v", language, days, text)
		}
	}
	if name := task.Text["fr"].Name; name != "Nettoyer les pistes de sous-titres gardées" {
		t.Errorf("French name %q", name)
	}
}
