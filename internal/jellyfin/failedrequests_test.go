package jellyfin

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/logs"
)

// ringLogged is a test server that logs as the server does, to the ring
// administrators read.
func ringLogged(t *testing.T) (testServer, *logs.Ring) {
	t.Helper()
	ring := logs.NewRing(logs.Capacity)
	return newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.Logger, o.Logs = slog.New(slog.NewTextHandler(ring, nil)), ring
	}), ring
}

// away takes a table away from the test's database, as nothing does in
// production: what reads it fails, and everything else answers.
func (s testServer) away(t *testing.T, table string) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), "ALTER TABLE "+table+" RENAME TO "+table+"_away"); err != nil {
		t.Fatal(err)
	}
}

// A guest's playback that fails once its link is read is logged with the
// route of share links, never the path: the link's token is in it, and
// administrators share the log.
func TestAFailedShareLinkPlayIsLoggedByItsRoute(t *testing.T) {
	t.Parallel()
	s, ring := ringLogged(t)
	p := playingOn(t, s)
	p.setting(t, func(settings *accounts.Settings) { settings.ShareLinks = true })
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.CanShare = new(true) })
	mux := http.NewServeMux()
	mux.Handle("/share/", p.handler.Share())
	mux.Handle("/", p.handler)
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	shared := shareSetup{playbackSetup: p, site: site.URL}
	token, _ := shared.link(t, 1)
	if status, body := shared.guest(t, http.MethodGet, "/share/"+token, nil); status != http.StatusOK {
		t.Fatalf("the link's page: %d %s", status, body)
	}
	// The sharer's devices cannot be counted: the playback fails.
	s.away(t, "devices")
	if status, body := shared.guest(t, http.MethodPost, "/share/"+token+"/play", nil); status != http.StatusInternalServerError {
		t.Fatalf("the playback: %d %s", status, body)
	}
	kept := string(ring.Contents())
	if !strings.Contains(kept, `msg="Jellyfin API request failed" method=POST path=/share/{token}/play error=`) {
		t.Errorf("the failed playback is not logged with its route:\n%s", kept)
	}
	if strings.Contains(kept, token) {
		t.Errorf("the log holds the link's token:\n%s", strings.ReplaceAll(kept, token, "<the token>"))
	}
}

// An API key whose revocation fails is still good: the request is logged
// with its route, never the path, which is the key.
func TestAFailedAPIKeyRevocationIsLoggedByItsRoute(t *testing.T) {
	t.Parallel()
	s, ring := ringLogged(t)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := app("dashboard", s.signIn("root", "dashboard"))
	_, key, err := s.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	s.away(t, "api_keys")
	if status, body := s.call(http.MethodDelete, "/Auth/Keys/"+key, admin, nil); status != http.StatusInternalServerError {
		t.Fatalf("the revocation: %d %s", status, body)
	}
	kept := string(ring.Contents())
	if !strings.Contains(kept, `msg="Jellyfin API request failed" method=DELETE path=/Auth/Keys/{key} error=`) {
		t.Errorf("the failed revocation is not logged with its route:\n%s", kept)
	}
	if strings.Contains(kept, key) {
		t.Errorf("the log holds the key:\n%s", strings.ReplaceAll(kept, key, "<the key>"))
	}
}
