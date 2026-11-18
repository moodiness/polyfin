package admin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/secrets"
)

// signingAPI is a test API that makes new signing secrets in its own
// database, as main has it do, and counts its restarts.
type signingAPI struct {
	testAPI
	pool     *pgxpool.Pool
	restarts chan struct{}
}

func newSigningAPI(t *testing.T) signingAPI {
	t.Helper()
	s := signingAPI{restarts: make(chan struct{}, 10)}
	s.testAPI = newTestAPI(t, 10, func(o *Options, deps testDeps) {
		s.pool = deps.pool
		o.Restart = func() { s.restarts <- struct{}{} }
		o.NewSigningSecret = func(ctx context.Context) error { return database.ReplaceSecret(ctx, deps.pool) }
		o.Health.Started = time.Now().Add(-time.Hour)
	})
	return s
}

// stored reads the signing secret the database stores.
func (s signingAPI) stored(t *testing.T) []byte {
	t.Helper()
	secret, err := database.Secret(t.Context(), s.pool)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// A new signing secret is made for an administrator who confirms it with
// their password, and Polyfin restarts to sign with it, with what a
// restart asks: anyway while nothing known starts Polyfin again, and once
// a minute, restarts included. Who did is recorded. A request refused
// leaves the secret as it was, and restarts nothing.
func TestANewSigningSecretIsAnAdministratorsConfirmedAction(t *testing.T) {
	t.Parallel()
	api := newSigningAPI(t)
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	right := map[string]any{"password": "correct horse", "anyway": true}
	first := api.stored(t)

	if status, _, _ := member.call(http.MethodPost, "/server/signing-secret", right); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", map[string]any{"password": "correct horse"}); status != http.StatusConflict ||
		body["error"] != "restart_unconfirmed" {
		t.Errorf("without anyway: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", map[string]any{"password": "wrong horse", "anyway": true}); status != http.StatusForbidden ||
		body["error"] != "wrong_password" {
		t.Errorf("wrong password: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", map[string]any{"anyway": true}); status != http.StatusForbidden {
		t.Errorf("no password: %d %v", status, body)
	}
	if len(api.restarts) != 0 || len(activityNames(t, admin, "ServerRestarted")) != 0 || !bytes.Equal(api.stored(t), first) {
		t.Fatal("a refused request made a new secret, or restarted Polyfin")
	}

	var state serverJSON
	admin.raw(http.MethodGet, "/server", "", &state)
	status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", right)
	if status != http.StatusAccepted || body["startedAt"] != state.StartedAt.Format(time.RFC3339Nano) {
		t.Fatalf("new signing secret: %d %v", status, body)
	}
	<-api.restarts
	second := api.stored(t)
	if len(second) != 32 || bytes.Equal(second, first) {
		t.Errorf("the stored secret after the action: %d bytes, the same as before: %v", len(second), bytes.Equal(second, first))
	}
	if names := activityNames(t, admin, "ServerRestarted"); len(names) != 1 || names[0] != "root made a new signing secret and restarted Polyfin" {
		t.Errorf("activity: %v", names)
	}
	// Once a minute, as a restart and with it.
	for _, path := range []string{"/server/signing-secret", "/server/restart"} {
		status, body, response := admin.call(http.MethodPost, path, right)
		if status != http.StatusTooManyRequests || body["error"] != "too_soon" || response.Header.Get("Retry-After") == "" {
			t.Errorf("%s again at once: %d %v", path, status, body)
		}
	}
	if len(api.restarts) != 0 || !bytes.Equal(api.stored(t), second) {
		t.Error("a request refused as too soon made a new secret, or restarted Polyfin")
	}
	later := time.Now().Add(time.Minute + time.Second)
	api.clock.Store(&later)

	// Confirmed in the settings, the request needs no anyway.
	settings := api.store.Settings()
	settings.RestartConfirmed = true
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", map[string]any{"password": "correct horse"}); status != http.StatusAccepted {
		t.Fatalf("confirmed: %d %v", status, body)
	}
	<-api.restarts
	if third := api.stored(t); len(third) != 32 || bytes.Equal(third, second) || bytes.Equal(third, first) {
		t.Error("the second action left the stored secret as it was")
	}
}

// What Polyfin signed before a new signing secret is refused by the
// Polyfin started again after it, which makes its signers from the stored
// secret as main does: grants, live links, and the marks of images and
// configure links alike, with POLYFIN_SECRET_KEY and without. What it
// signs itself verifies.
func TestANewSigningSecretEndsWhatWasSigned(t *testing.T) {
	t.Parallel()
	key, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	for name, box := range map[string]*secrets.Box{"with a key": key, "without a key": nil} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := newSigningAPI(t)
			admin := api.signedIn("root", true)
			// The signer of a Polyfin that starts now.
			started := func() playback.Signer { return playback.NewSigner(box.SigningSecret(api.stored(t))) }
			grant := playback.Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Relay: true}
			const target, purpose, value = "https://source.example/segment.ts", "purpose", "value"

			before := started()
			token := before.Sign(grant)
			link, mark := before.Link(token, target), before.Mark(purpose, value)
			if again := started(); !again.VerifyLink(token, target, link) || !again.VerifyMark(purpose, value, mark) {
				t.Fatal("a restart alone ends what was signed")
			}

			if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", map[string]any{"password": "correct horse", "anyway": true}); status != http.StatusAccepted {
				t.Fatalf("new signing secret: %d %v", status, body)
			}
			<-api.restarts
			after := started()
			if got, err := after.Verify(token); !errors.Is(err, playback.ErrInvalidGrant) {
				t.Errorf("a grant signed before: %+v %v", got, err)
			}
			if after.VerifyLink(token, target, link) {
				t.Error("a live link signed before still verifies")
			}
			if after.VerifyMark(purpose, value, mark) {
				t.Error("a mark signed before still verifies")
			}
			token = after.Sign(grant)
			if got, err := after.Verify(token); err != nil || got != grant {
				t.Errorf("a grant signed after: %+v %v", got, err)
			}
			if !after.VerifyLink(token, target, after.Link(token, target)) || !after.VerifyMark(purpose, value, after.Mark(purpose, value)) {
				t.Error("a link or a mark signed after is refused")
			}
		})
	}
}

// A secret that could not be stored restarts nothing; a Polyfin that
// offers no restart, or cannot store a secret, offers no new one.
func TestASigningSecretThatCannotBeStoredRestartsNothing(t *testing.T) {
	t.Parallel()
	right := map[string]any{"password": "correct horse", "anyway": true}
	restarts := make(chan struct{}, 10)
	failing := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Restart = func() { restarts <- struct{}{} }
		o.NewSigningSecret = func(context.Context) error { return errors.New("the database is away") }
		o.Health.Started = time.Now().Add(-time.Hour)
	})
	admin := failing.signedIn("root", true)
	if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", right); status != http.StatusInternalServerError {
		t.Errorf("a secret that cannot be stored: %d %v", status, body)
	}
	if len(restarts) != 0 || len(activityNames(t, admin, "ServerRestarted")) != 0 {
		t.Error("Polyfin restarted without a new secret")
	}

	for name, configure := range map[string]func(*Options, testDeps){
		"no restart": func(o *Options, deps testDeps) {
			o.NewSigningSecret = func(ctx context.Context) error { return database.ReplaceSecret(ctx, deps.pool) }
		},
		"no way to store one": func(o *Options, _ testDeps) { o.Restart = func() { restarts <- struct{}{} } },
	} {
		admin := newTestAPI(t, 10, configure).signedIn("root", true)
		if status, body, _ := admin.call(http.MethodPost, "/server/signing-secret", right); status != http.StatusNotFound {
			t.Errorf("%s: %d %v", name, status, body)
		}
	}
	if len(restarts) != 0 {
		t.Error("Polyfin restarted without a new secret")
	}
}
