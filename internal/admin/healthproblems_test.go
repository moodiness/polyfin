package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/tasks"
)

// switchPinger is a database that stops answering once down is set.
type switchPinger struct{ down *atomic.Bool }

func (p switchPinger) Ping(context.Context) error {
	if p.down.Load() {
		return errors.New("connection refused")
	}
	return nil
}

// The admin API tells the problems System › Health shows, errors first,
// with what the admin app names them by and the place that describes them;
// notifications tell the same, in the server language. Keys stored
// unencrypted stop counting once a key is set; with the database down, it
// alone is told.
func TestHealthProblemsAreThoseTheHealthPageShows(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": "own", "name": "Own TV", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`)
	}))
	t.Cleanup(addon.Close)
	registry := tasks.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.Register(tasks.Task{Key: "Refresh", Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{"en": {Name: "Refresh things"}, "fr": {Name: "Actualiser des choses"}},
		Run:  func(context.Context) error { return errors.New("addon down") }})
	registry.Start(t.Context())
	down := new(atomic.Bool)
	var options *Options
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Database = switchPinger{down}
		o.Tasks = registry
		o.Health = HealthSources{Addons: deps.client, Secrets: func(context.Context) (secrets.Report, error) {
			return secrets.Report{Plaintext: 2, Unreadable: []secrets.Unreadable{{Target: "Team pager"}}}, nil
		}}
		options, store = o, deps.addons
	})
	admin := api.signedIn("root", true)
	api.signedIn("sam", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sam := users[slices.IndexFunc(users, func(u accounts.User) bool { return u.Name == "sam" })]
	// Sam's own addon is on the local network, which a member's addons may
	// not reach: checking it records the failure.
	own, err := store.Install(t.Context(), addons.Personal(sam.ID), addon.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if status := admin.raw(http.MethodPost, "/health/addons/"+own.ID.String()+"/check", "", nil); status != http.StatusOK {
		t.Fatalf("check: %d", status)
	}
	if err := registry.Run(tasks.ID("Refresh")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		info, _ := registry.Task(tasks.ID("Refresh"), "en")
		return info.Last != nil
	})

	target := "Team pager"
	want := []healthProblemJSON{
		{Key: "secrets:unreadable", Code: problemSecretsUnreadable, Tone: "error", To: "/system/health#secrets",
			Secrets: []unreadableJSON{{Target: &target}}},
		{Key: "addon:" + own.ID.String(), Code: problemAddon, Tone: "warning", To: "/system/health#addons", Name: "Own TV",
			Owner: &ownerJSON{ID: sam.ID.String(), Name: "sam"}, Failure: "private_network"},
		{Key: "secrets:plaintext", Code: problemSecretsPlaintext, Tone: "warning", To: "/system/health#secrets"},
		{Key: "task:Refresh", Code: problemTask, Tone: "warning", To: "/system/schedule", Task: "Refresh things"},
	}
	problems := func(language string) []healthProblemJSON {
		t.Helper()
		var answer struct {
			Problems []healthProblemJSON `json:"problems"`
		}
		if status := admin.raw(http.MethodGet, "/health/problems?language="+language, "", &answer); status != http.StatusOK {
			t.Fatalf("problems: %d", status)
		}
		return answer.Problems
	}
	if got := problems("en"); !reflect.DeepEqual(got, want) {
		t.Errorf("problems:\n%+v\nwant\n%+v", got, want)
	}
	if got := problems("fr"); len(got) != 4 || got[3].Task != "Actualiser des choses" {
		t.Errorf("tasks in French: %+v", got)
	}

	settings := api.store.Settings()
	settings.Language = "fr"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantTold := []notifications.Problem{
		{Key: "secrets:unreadable", Severity: notifications.SeverityError,
			Text: "1 clé enregistrée ne peut pas être déchiffrée avec POLYFIN_SECRET_KEY et compte comme absente.", Page: "/system/health#secrets"},
		{Key: "addon:" + own.ID.String(), Severity: notifications.SeverityWarning, Text: "Own TV (Addons de sam) : est sur un réseau local",
			Page: "/system/health#addons"},
		{Key: "secrets:plaintext", Severity: notifications.SeverityWarning,
			Text: "Les clés et jetons sont enregistrés sans chiffrement. Définissez POLYFIN_SECRET_KEY pour les chiffrer.",
			Page: "/system/health#secrets"},
		{Key: "task:Refresh", Severity: notifications.SeverityWarning, Text: "La tâche « Actualiser des choses » a échoué.",
			Page: "/system/schedule"},
	}
	if !slices.Equal(told, wantTold) {
		t.Errorf("told:\n%+v\nwant\n%+v", told, wantTold)
	}

	// With a key set, keys stored unencrypted are sealed at the next start:
	// not a problem.
	withKey := *options
	withKey.Health.SecretKey = true
	if told, _ := HealthProblems(withKey)(t.Context()); len(told) != 3 || slices.ContainsFunc(told, func(p notifications.Problem) bool {
		return p.Key == "secrets:plaintext"
	}) {
		t.Errorf("with a key: %+v", told)
	}

	down.Store(true)
	if got := problems("en"); !reflect.DeepEqual(got, []healthProblemJSON{{Key: "database", Code: problemDatabase, Tone: "error",
		To: "/system/health#database"}}) {
		t.Errorf("with the database down: %+v", got)
	}
}

// A disk is short of room below 2 GB free, or below 5% of its size.
func TestDisksShortOfRoom(t *testing.T) {
	for _, disk := range []struct {
		free, used int64
		low        bool
	}{
		{1_500_000_000, 1_000_000_000, true},
		{10_000_000_000, 990_000_000_000, true},
		{10_000_000_000, 90_000_000_000, false},
		{-1, -1, false},
	} {
		if got := lowOnSpace(disk.free, disk.used); got != disk.low {
			t.Errorf("%d free, %d used: low %v, want %v", disk.free, disk.used, got, disk.low)
		}
	}
}
