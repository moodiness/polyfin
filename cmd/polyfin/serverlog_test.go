package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// The server's log on standard error is the one System › Logs shows,
// line for line: the secrets of its records are hidden there too, and the
// setup code and the reset PIN, which administrators read there, stay.
func TestStandardErrorIsRedactedAsTheInAppLog(t *testing.T) {
	t.Parallel()
	stderr := &lockedBuffer{}
	log := newServerLog(stderr)
	log.logger.Warn("An addon could not be read", "error", &url.Error{Op: "Get", URL: "https://addon.example/DEBRID-key-1/manifest.json", Err: errors.New("EOF")})
	log.logger.Warn("PGPASSWORD stays in the environment of the programs Polyfin runs",
		"error", errors.New(`parse "postgresql://polyfin:PG-pass-2@db:5432/polyfin": invalid port`))
	log.logger.Info("A request failed", "header", "Authorization: Bearer TOKEN-3", "form", "client_secret=CS-4&code=AUTHCODE-5")
	log.logger.Warn("No administrator yet: open /admin/ and create one with this setup code", "setup_code", "ABCD-EFGH")
	log.logger.Info("A password reset PIN was made", "user", "root", "pin", "12-34-56")

	written := stderr.String()
	if written != string(log.recent.Contents()) {
		t.Errorf("standard error:\n%s\nSystem › Logs:\n%s", written, log.recent.Contents())
	}
	for _, secret := range []string{"DEBRID-key-1", "PG-pass-2", "TOKEN-3", "CS-4", "AUTHCODE-5"} {
		if strings.Contains(written, secret) {
			t.Errorf("standard error holds %q:\n%s", secret, written)
		}
	}
	for _, kept := range []string{"https://addon.example/", "setup_code=ABCD-EFGH", "pin=12-34-56", "level=WARN"} {
		if !strings.Contains(written, kept) {
			t.Errorf("standard error lost %q:\n%s", kept, written)
		}
	}
	if lines := strings.Count(written, "\n"); lines != 5 {
		t.Errorf("%d lines on standard error:\n%s", lines, written)
	}
}
