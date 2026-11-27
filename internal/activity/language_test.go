package activity

import (
	"io"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// An entry is named in the language it is read in, whichever the server
// spoke when it was written: its name, its short overview, the names of
// sections and secrets it holds, its numbers and where its title lies. Its
// name is still written in the server language. An entry written before,
// or with a phrase this version does not know, reads as it was written;
// one read in no language reads in the server's of the time.
func TestEntriesAreNamedInTheReadersLanguage(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	var language atomic.Value
	language.Store("fr")
	store := New(pool, func() accounts.Settings { return accounts.Settings{Language: language.Load().(string)} },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := pool.Exec(ctx, `INSERT INTO activity_log (name, short_overview, type) VALUES ('Connexion de Alex', 'Adresse : 192.0.2.9', 'AuthenticationSucceeded')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activity_log (name, type, name_phrase) VALUES ('Later entry', 'Later', '{"key": "later"}')`); err != nil {
		t.Fatal(err)
	}
	sam, root := accounts.User{ID: accounts.ID{1}, Name: "Sam"}, accounts.User{ID: accounts.ID{2}, Name: "Root"}
	item := accounts.ID{3}
	store.SignedIn(ctx, sam, "192.0.2.1")
	store.GroupChanged(ctx, accounts.Group{Name: "Kids"}, []string{accounts.SectionHours, accounts.SectionLibraries, "unknown"})
	store.SecretRevealed(ctx, root, "tmdbKey", false)
	store.LibraryOrderApplied(ctx, root.Name, 3)
	store.PlaybackStarted(ctx, sam, "Amélie", item.String(), "TV")

	type entry struct{ name, short, title string }
	want := map[string][]entry{
		"en": {
			{"Sam is playing Amélie on TV", "", "Amélie"},
			{"Root applied the order of the libraries to every user (3 with an order of their own)", "", ""},
			{"Root revealed the TMDB key", "", ""},
			{"Group Kids was changed: Allowed hours, Visible libraries, unknown", "", ""},
			{"Sam signed in", "Address: 192.0.2.1", ""},
		},
		"fr": {
			{"Sam regarde Amélie sur TV", "", "Amélie"},
			{"Root a appliqué l’ordre des bibliothèques à tous les utilisateurs (3 avec un ordre à eux)", "", ""},
			{"Root a affiché la clé TMDB", "", ""},
			{"Groupe Kids modifié\u202f: Horaires autorisés, Bibliothèques visibles, unknown", "", ""},
			{"Connexion de Sam", "Adresse\u202f: 192.0.2.1", ""},
		},
		"de": {
			{"Sam schaut Amélie auf TV", "", "Amélie"},
			{"Root hat die Reihenfolge der Bibliotheken auf alle Benutzer angewendet (3 mit eigener Reihenfolge)", "", ""},
			{"Root hat den TMDB-Schlüssel angezeigt", "", ""},
			{"Gruppe Kids wurde geändert: Erlaubte Zeiten, Sichtbare Bibliotheken, unknown", "", ""},
			{"Sam hat sich angemeldet", "Adresse: 192.0.2.1", ""},
		},
	}
	// Written before, and by a later version: as written, in any language.
	before := []entry{{"Later entry", "", ""}, {"Connexion de Alex", "Adresse : 192.0.2.9", ""}}
	read := func(query Query) []entry {
		t.Helper()
		page, err := store.Entries(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		var got []entry
		for _, e := range page.Entries {
			var short, title string
			if e.ShortOverview != nil {
				short = *e.ShortOverview
			}
			if e.TitleStart != nil {
				title = string([]rune(e.Name)[*e.TitleStart:*e.TitleEnd])
			}
			got = append(got, entry{e.Name, short, title})
		}
		return got
	}
	check := func(what string, got, want []entry) {
		t.Helper()
		want = append(append([]entry{}, want...), before...)
		if len(got) != len(want) {
			t.Fatalf("%s: %d entries, want %d: %q", what, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: %+q, want %+q", what, got[i], want[i])
			}
		}
	}
	for _, code := range []string{"en", "fr", "de"} {
		check(code, read(Query{Language: code}), want[code])
	}
	language.Store("de")
	check("the server language", read(Query{}), want["de"])

	var written []string
	rows, err := pool.Query(ctx, "SELECT name FROM activity_log WHERE name_phrase IS NOT NULL AND type <> 'Later' ORDER BY id DESC")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		written = append(written, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var french []string
	for _, e := range want["fr"] {
		french = append(french, e.name)
	}
	if !slices.Equal(written, french) {
		t.Errorf("names written: %q, want them in French, the server language then", written)
	}
}
