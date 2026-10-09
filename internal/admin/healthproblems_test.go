package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/tasks"
)

// downPinger is a database that does not answer.
type downPinger struct{}

func (downPinger) Ping(context.Context) error { return errors.New("connection refused") }

// The problems notifications tell are those System › Health shows, named
// in the server language: a failed task, keys stored unencrypted while no
// key is set; with the database down, that alone.
func TestHealthProblemsAreThoseTheHealthPageShows(t *testing.T) {
	registry := tasks.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.Register(tasks.Task{Key: "Refresh", Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{"en": {Name: "Refresh things"}, "fr": {Name: "Actualiser des choses"}},
		Run:  func(context.Context) error { return errors.New("addon down") }})
	registry.Start(t.Context())
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Tasks = registry
		o.Health.Secrets = func(context.Context) (secrets.Report, error) { return secrets.Report{Plaintext: 2}, nil }
		options = o
	})
	settings := api.store.Settings()
	settings.Language = "fr"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if err := registry.Run(tasks.ID("Refresh")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		info, _ := registry.Task(tasks.ID("Refresh"), "en")
		return info.Last != nil
	})

	problems, err := HealthProblems(*options)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []notifications.Problem{
		{Key: "task:Refresh", Severity: notifications.SeverityWarning, Text: "La tâche « Actualiser des choses » a échoué."},
		{Key: "secrets:plaintext", Severity: notifications.SeverityWarning,
			Text: "Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer."},
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems: %+v, want %+v", problems, want)
	}

	// With a key set, keys stored unencrypted are sealed at the next start:
	// not a problem.
	options.Health.SecretKey = true
	if problems, _ := HealthProblems(*options)(t.Context()); len(problems) != 1 || problems[0].Key != "task:Refresh" {
		t.Errorf("with a key: %+v", problems)
	}

	options.Database = downPinger{}
	if problems, _ := HealthProblems(*options)(t.Context()); len(problems) != 1 || problems[0].Key != "database" ||
		problems[0].Severity != notifications.SeverityError {
		t.Errorf("with the database down: %+v", problems)
	}
}
