package admin

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/database"
)

// collationProblems are the collation_version problems the admin API
// tells, in language.
func collationProblems(t *testing.T, admin browser, language string) []healthProblemJSON {
	t.Helper()
	var answer struct {
		Problems []healthProblemJSON `json:"problems"`
	}
	if status := admin.raw(http.MethodGet, "/health/problems?language="+language, "", &answer); status != http.StatusOK {
		t.Fatalf("problems: %d", status)
	}
	var found []healthProblemJSON
	for _, p := range answer.Problems {
		if p.Code == problemCollationVersion {
			found = append(found, p)
		}
	}
	return found
}

// A collation version that changed is a warning naming the database and
// both versions; equal versions, versions unknown, or a query that fails
// are no problem.
func TestChangedCollationVersionIsAHealthProblem(t *testing.T) {
	t.Parallel()
	var collation atomic.Pointer[database.Collation]
	var failing atomic.Bool
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health.Collation = func(context.Context) (database.Collation, error) {
			if failing.Load() {
				return database.Collation{}, errors.New("could not load locale")
			}
			return *collation.Load(), nil
		}
	})
	admin := api.signedIn("root", true)
	for _, c := range []database.Collation{
		{Database: "polyfin", Provider: "libc", Recorded: "2.41", Actual: "2.41"},
		{Database: "polyfin", Provider: "libc", Recorded: "", Actual: "2.41"},
		{Database: "polyfin", Provider: "libc", Recorded: "2.36", Actual: ""},
	} {
		collation.Store(&c)
		if got := collationProblems(t, admin, "en"); len(got) != 0 {
			t.Errorf("for %+v: %+v", c, got)
		}
	}
	collation.Store(&database.Collation{Database: "polyfin", Provider: "libc", Recorded: "2.36", Actual: "2.41"})
	want := []healthProblemJSON{{Key: "collation_version", Code: problemCollationVersion, Tone: "warning", To: "/system/health",
		Name: "polyfin", Previous: "2.36", Version: "2.41", Kind: "libc"}}
	if got := collationProblems(t, admin, "en"); !reflect.DeepEqual(got, want) {
		t.Errorf("problems = %+v, want %+v", got, want)
	}
	failing.Store(true)
	if got := collationProblems(t, admin, "en"); len(got) != 0 {
		t.Errorf("with the query failing: %+v", got)
	}
}

// On the test database, whose versions match, Health tells nothing of its
// collation.
func TestUnchangedCollationVersionIsNoHealthProblem(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Health.Collation = func(ctx context.Context) (database.Collation, error) { return database.ReadCollation(ctx, deps.pool) }
	})
	if got := collationProblems(t, api.signedIn("root", true), "en"); len(got) != 0 {
		t.Errorf("problems = %+v", got)
	}
}

// The problem tells the versions, what an index out of order does, and
// the commands that repair it, which name the database as an identifier.
func TestCollationVersionText(t *testing.T) {
	t.Parallel()
	p := healthProblemJSON{Code: problemCollationVersion, Name: "polyfin", Previous: "2.36", Version: "2.41", Kind: "libc"}
	for language, want := range map[string]string{
		"en": "The collation version of the database polyfin changed: PostgreSQL recorded 2.36, and the system’s collation library now provides 2.41. " +
			"Its text indexes may be out of order, which can hide rows or duplicate them. " +
			`To repair them, stop Polyfin, then, connected as the database’s owner, run REINDEX DATABASE "polyfin"; and then ALTER DATABASE "polyfin" REFRESH COLLATION VERSION;`,
		"fr": "La version de collation de la base de données polyfin a changé\u202f: PostgreSQL a enregistré 2.36, et la bibliothèque de collation du système fournit maintenant 2.41. " +
			"Ses index de texte peuvent ne plus être dans l’ordre, ce qui peut masquer des lignes ou les dupliquer. " +
			`Pour les réparer, arrêtez Polyfin, puis, connecté en tant que propriétaire de la base, exécutez REINDEX DATABASE "polyfin"; puis ALTER DATABASE "polyfin" REFRESH COLLATION VERSION;`,
	} {
		if got := p.text(language); got != want {
			t.Errorf("in %s:\n%s\nwant\n%s", language, got, want)
		}
	}
	for name, want := range map[string][2]string{
		"polyfin":    {`REINDEX DATABASE "polyfin";`, `ALTER DATABASE "polyfin" REFRESH COLLATION VERSION;`},
		`Media "TV"`: {`REINDEX DATABASE "Media ""TV""";`, `ALTER DATABASE "Media ""TV""" REFRESH COLLATION VERSION;`},
	} {
		if reindex, refresh := collationCommands(name); reindex != want[0] || refresh != want[1] {
			t.Errorf("commands for %s: %s %s, want %s %s", name, reindex, refresh, want[0], want[1])
		}
	}
	for _, language := range []string{"de", "es", "it", "pt-BR"} {
		got := p.text(language)
		for _, part := range []string{"polyfin", "2.36", "2.41", `REINDEX DATABASE "polyfin";`, `ALTER DATABASE "polyfin" REFRESH COLLATION VERSION;`} {
			if !strings.Contains(got, part) {
				t.Errorf("in %s, %q lacks %s", language, got, part)
			}
		}
	}
}
