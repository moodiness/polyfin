package secrets_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/trackers"
)

// The plaintext values the test stores, which must never be found in the
// database once a key is set, nor in the log.
var plaintexts = []string{"pm-plain-key", "trakt-plain-secret", "access-plain", "refresh-plain", "mdblist-plain", "target-plain-token", "ntfy-plain-token",
	"share-plain-password"}

func newBox(t *testing.T, key string) *secrets.Box {
	t.Helper()
	parsed, err := secrets.ParseKey(key)
	if err != nil {
		t.Fatal(err)
	}
	box, err := secrets.New(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// stored returns every secret value as the database holds it.
func stored(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	var values []string
	var pm, tidb, trakt string
	if err := pool.QueryRow(t.Context(), "SELECT publicmetadb_key, theintrodb_key, trakt_client_secret FROM settings").Scan(&pm, &tidb, &trakt); err != nil {
		t.Fatal(err)
	}
	values = append(values, pm, tidb, trakt)
	rows, err := pool.Query(t.Context(), "SELECT token, refresh_token FROM tracking_connections ORDER BY service")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var token, refresh string
		if err := rows.Scan(&token, &refresh); err != nil {
			t.Fatal(err)
		}
		values = append(values, token, refresh)
	}
	targets, err := pool.Query(t.Context(), "SELECT secret FROM notification_targets ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer targets.Close()
	for targets.Next() {
		var secret string
		if err := targets.Scan(&secret); err != nil {
			t.Fatal(err)
		}
		values = append(values, secret)
	}
	shares, err := pool.Query(t.Context(), "SELECT share_password FROM local_folders ORDER BY addon_id")
	if err != nil {
		t.Fatal(err)
	}
	defer shares.Close()
	for shares.Next() {
		var password string
		if err := shares.Scan(&password); err != nil {
			t.Fatal(err)
		}
		values = append(values, password)
	}
	return values
}

// noPlaintext fails when the database holds one of the plaintexts.
func noPlaintext(t *testing.T, pool *pgxpool.Pool, when string) {
	t.Helper()
	for _, value := range stored(t, pool) {
		if value != "" && !secrets.Sealed(value) {
			t.Errorf("%s: stored unsealed: %q", when, value)
		}
	}
}

// lockedLog is a log the test reads.
type lockedLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *lockedLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Reset()
}

func TestStoredSecretsAreSealedOnceAndUnreadableOnesCountAsUnset(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// A server that ran without a key: its secrets are plaintext.
	plain, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := plain.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	settings := plain.Settings()
	settings.PublicMetaDBKey, settings.TraktClientID, settings.TraktClientSecret = "pm-plain-key", "trakt-id", "trakt-plain-secret"
	if _, err := plain.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tracking_connections (user_id, service, token, refresh_token, connected_at)
		VALUES ($1, 'trakt', 'access-plain', 'refresh-plain', now()), ($1, 'mdblist', 'mdblist-plain', '', now())`, alice.ID); err != nil {
		t.Fatal(err)
	}
	// A user's Discord target and the server's ntfy target, with a token.
	if _, err := pool.Exec(ctx, `INSERT INTO notification_targets (user_id, kind, name, address, topic, secret)
		VALUES ($1, 'discord', 'Family', 'https://hooks.example.org', '', 'https://hooks.example.org/api/webhooks/1/target-plain-token'),
			(NULL, 'ntfy', 'Phone', 'https://ntfy.example.org', 'alerts', 'ntfy-plain-token')`, alice.ID); err != nil {
		t.Fatal(err)
	}
	// A network share's password.
	if _, err := pool.Exec(ctx, `WITH share AS (INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES (NULL, 'local', 'smb://nas.example/media', '{"name": "NAS"}', 1) RETURNING id)
		INSERT INTO local_folders (addon_id, kind, share_user, share_password) SELECT id, 'movies', 'reader', 'share-plain-password' FROM share`); err != nil {
		t.Fatal(err)
	}
	log := &lockedLog{}
	logger := slog.New(slog.NewTextHandler(log, nil))
	var none *secrets.Box
	if err := none.Prepare(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}
	if report, _ := none.Inspect(ctx, pool); report.Plaintext != 8 || len(report.Unreadable) != 0 {
		t.Errorf("without a key: %+v", report)
	}
	if !strings.Contains(log.String(), "level=WARN") || !strings.Contains(log.String(), "POLYFIN_SECRET_KEY") {
		t.Errorf("no warning that keys are stored unencrypted: %s", log)
	}

	// Started with a key, every secret is sealed, once.
	key := newBox(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	log.reset()
	if err := key.Prepare(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}
	noPlaintext(t, pool, "after sealing")
	first := stored(t, pool)
	if err := key.Prepare(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}
	if again := stored(t, pool); !slices.Equal(again, first) {
		t.Error("sealing again changed the stored values")
	}
	if strings.Count(log.String(), "Stored secrets encrypted") != 1 || strings.Contains(log.String(), "level=WARN") ||
		strings.Contains(log.String(), "level=ERROR") {
		t.Errorf("log: %s", log)
	}
	if report, _ := key.Inspect(ctx, pool); report.Plaintext != 0 || len(report.Unreadable) != 0 {
		t.Errorf("with the key: %+v", report)
	}

	// The settings and the connections read them back.
	sealed, err := accounts.Open(ctx, pool, accounts.Sealing(key))
	if err != nil {
		t.Fatal(err)
	}
	if got := sealed.Settings(); got.PublicMetaDBKey != "pm-plain-key" || got.TraktClientSecret != "trakt-plain-secret" {
		t.Errorf("settings: %q, %q", got.PublicMetaDBKey, got.TraktClientSecret)
	}
	tracking := trackers.New(trackers.Options{DB: pool, Settings: sealed.Settings, Secrets: key, Logger: logger})
	t.Cleanup(tracking.Close)
	if got, err := tracking.Key(ctx, alice.ID, trackers.MDBList); err != nil || got != "mdblist-plain" {
		t.Errorf("MDBList key: %q, %v", got, err)
	}
	// New values are written sealed.
	settings = sealed.Settings()
	settings.TheIntroDBKey = "tidb-new-key"
	if _, err := sealed.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	mdblist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"username":"alice"}`)
	}))
	t.Cleanup(mdblist.Close)
	connecting := trackers.New(trackers.Options{DB: pool, Settings: sealed.Settings, Secrets: key, Logger: logger,
		URLs: map[string]string{trackers.MDBList: mdblist.URL}})
	t.Cleanup(connecting.Close)
	if _, err := connecting.ConnectKey(ctx, alice.ID, trackers.MDBList, "mdblist-new-key"); err != nil {
		t.Fatal(err)
	}
	noPlaintext(t, pool, "after new values")
	for _, value := range stored(t, pool) {
		if strings.Contains(value, "new-key") {
			t.Errorf("stored: %q", value)
		}
	}

	// With another key, or none, the sealed secrets count as not set, and
	// one error names them, never their values.
	for name, wrong := range map[string]*secrets.Box{"another key": newBox(t, strings.Repeat("ab", 32)), "no key": nil} {
		log.reset()
		if err := wrong.Prepare(ctx, pool, logger); err != nil {
			t.Fatal(err)
		}
		if errors := strings.Count(log.String(), "level=ERROR"); errors != 1 {
			t.Errorf("%s: %d errors logged: %s", name, errors, log)
		}
		for _, named := range []string{"setting publicMetaDbKey", "setting theIntroDbKey", "setting traktClientSecret",
			"trakt connection of alice", "mdblist connection of alice", "notification target Family of alice", "server notification target Phone",
			"password of the share NAS"} {
			if !strings.Contains(log.String(), named) {
				t.Errorf("%s: %q not named: %s", name, named, log)
			}
		}
		report, err := wrong.Inspect(ctx, pool)
		if err != nil || len(report.Unreadable) != 8 {
			t.Errorf("%s: %+v %v", name, report, err)
		}
		store, err := accounts.Open(ctx, pool, accounts.Sealing(wrong))
		if err != nil {
			t.Fatal(err)
		}
		if got := store.Settings(); got.PublicMetaDBKey != "" || got.TheIntroDBKey != "" || got.TraktClientSecret != "" || got.TraktAvailable() {
			t.Errorf("%s: unreadable settings read as set", name)
		}
		lost := trackers.New(trackers.Options{DB: pool, Settings: store.Settings, Secrets: wrong, Logger: logger})
		statuses, err := lost.Statuses(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range statuses {
			if status.Connected {
				t.Errorf("%s: %s counts as connected", name, status.Service)
			}
		}
		if _, err := lost.Key(ctx, alice.ID, trackers.MDBList); err == nil {
			t.Errorf("%s: an unreadable key was answered", name)
		}
		lost.Close()

		// Saving other settings keeps what the right key opens.
		settings := store.Settings()
		settings.ServerName = "Den " + name
		if _, err := store.UpdateSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		if reopened, err := accounts.Open(ctx, pool, accounts.Sealing(key)); err != nil || reopened.Settings().PublicMetaDBKey != "pm-plain-key" {
			t.Errorf("%s: saving other settings lost the key", name)
		}
	}
	for _, value := range plaintexts {
		if strings.Contains(log.String(), value) {
			t.Errorf("the log holds %q", value)
		}
	}

	// A key entered again replaces the unreadable one.
	other := newBox(t, strings.Repeat("ab", 32))
	store, err := accounts.Open(ctx, pool, accounts.Sealing(other))
	if err != nil {
		t.Fatal(err)
	}
	settings = store.Settings()
	settings.PublicMetaDBKey = "pm-entered-again"
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if reopened, _ := accounts.Open(ctx, pool, accounts.Sealing(other)); reopened.Settings().PublicMetaDBKey != "pm-entered-again" {
		t.Error("the key entered again is not read back")
	}
	report, _ := other.Inspect(ctx, pool)
	if slices.ContainsFunc(report.Unreadable, func(u secrets.Unreadable) bool { return u.Setting == "publicMetaDbKey" }) {
		t.Errorf("after entering it again: %+v", report.Unreadable)
	}
	noPlaintext(t, pool, "at the end")
}
