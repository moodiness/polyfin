package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// People were kept by name. Those an addon gave a TMDB identifier move to
// the item of that identifier, two names of one identifier merging into
// one person, with their photo, what TMDB told of them, the credits of
// both, a user's favorite and an administrator's edit; each name then
// points to that person. People of no known identifier stay as they were.
func TestPeopleOfATMDBIdentifierMoveToIt(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "title_credits" })
	if at < 0 {
		t.Fatal("no title_credits migration")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	// The identifiers of items, as the library derives them from their keys.
	itemID := func(key string) string {
		sum := sha256.Sum256([]byte("polyfin:item:" + key))
		return hex.EncodeToString(sum[:4]) + "-" + hex.EncodeToString(sum[4:6]) + "-" + hex.EncodeToString(sum[6:8]) + "-" +
			hex.EncodeToString(sum[8:10]) + "-" + hex.EncodeToString(sum[10:16])
	}
	ana, anaShort, cleo := itemID("person|ana actor"), itemID("person|a. actor"), itemID("person|cleo actor")
	people := map[string]string{
		ana: `{"kind": "person", "person": {"name": "Ana Actor", "image": "https://img.example/ana.jpg"}, "tmdbId": "5001",
			"tmdb": {"id": 5001, "foundAt": "2026-10-01T00:00:00Z", "biographies": {"en": "Ana was born."}, "imdb": "nm5001"},
			"credits": {"11111111-1111-1111-1111-111111111111": ["Actor"]}}`,
		anaShort: `{"kind": "person", "person": {"name": "A. Actor"}, "tmdbId": "5001", "confined": true,
			"credits": {"11111111-1111-1111-1111-111111111111": ["Writer"], "22222222-2222-2222-2222-222222222222": ["Actor"]}}`,
		cleo: `{"kind": "person", "person": {"name": "Cleo Actor"}, "tmdb": {"id": 6002, "foundAt": "2026-10-01T00:00:00Z"},
			"credits": {"33333333-3333-3333-3333-333333333333": ["Actor"]}}`,
	}
	keys := map[string]string{ana: "person|ana actor", anaShort: "person|a. actor", cleo: "person|cleo actor"}
	for id, data := range people {
		if _, err := pool.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'person', $3)`, id, keys[id], data); err != nil {
			t.Fatal(err)
		}
	}
	var user string
	if err := pool.QueryRow(ctx, `INSERT INTO users (name, password_hash) VALUES ('fan', 'x') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_data (user_id, item_id, favorite) VALUES ($1, $2, true)`, user, anaShort); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO item_overrides (item_id, fields) VALUES ($1, '{"Name": "Ana A. Actor"}')`, ana); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}

	moved := itemID("persontmdb|5001")
	var key string
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT key, data FROM items WHERE id = $1`, moved).Scan(&key, &data); err != nil {
		t.Fatalf("no person of TMDB identifier 5001: %v", err)
	}
	var person struct {
		Person struct {
			Name, Image string
		}
		TMDBID   string `json:"tmdbId"`
		TMDB     map[string]any
		Credits  map[string][]string
		Confined bool
		SameAs   string `json:"sameAs"`
	}
	if err := json.Unmarshal(data, &person); err != nil {
		t.Fatal(err)
	}
	slices.Sort(person.Credits["11111111-1111-1111-1111-111111111111"])
	if key != "persontmdb|5001" || person.TMDBID != "5001" || person.Person.Image != "https://img.example/ana.jpg" ||
		person.TMDB["imdb"] != "nm5001" || person.TMDB["biographies"].(map[string]any)["en"] != "Ana was born." ||
		len(person.Credits) != 2 || !slices.Equal(person.Credits["11111111-1111-1111-1111-111111111111"], []string{"Actor", "Writer"}) ||
		person.Confined || person.SameAs != "" {
		t.Errorf("the person of 5001: %s %s", key, data)
	}
	for _, name := range []string{ana, anaShort} {
		var sameAs, tmdbID string
		var credits bool
		if err := pool.QueryRow(ctx, `SELECT data->>'sameAs', data->>'tmdbId', data ? 'credits' FROM items WHERE id = $1`, name).
			Scan(&sameAs, &tmdbID, &credits); err != nil {
			t.Fatal(err)
		}
		if sameAs != moved || tmdbID != "5001" || credits {
			t.Errorf("the name's record %s points to %s (%s), credits %v", name, sameAs, tmdbID, credits)
		}
	}
	var favorite, edited bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_data WHERE item_id = $1 AND favorite),
		EXISTS (SELECT 1 FROM item_overrides WHERE item_id = $1)`, moved).Scan(&favorite, &edited); err != nil {
		t.Fatal(err)
	}
	if !favorite || !edited {
		t.Errorf("the favorite moved: %v, the edit: %v", favorite, edited)
	}
	var unchanged []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM items WHERE id = $1`, cleo).Scan(&unchanged); err != nil {
		t.Fatal(err)
	}
	var was, is map[string]any
	_ = json.Unmarshal([]byte(people[cleo]), &was)
	_ = json.Unmarshal(unchanged, &is)
	if a, b := mustJSON(t, was), mustJSON(t, is); a != b {
		t.Errorf("a person of no identifier the addon gave changed: %s", unchanged)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
