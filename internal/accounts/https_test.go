package accounts

import (
	"errors"
	"slices"
	"testing"
)

// HTTPS is off by default, on port 8920 without the redirect; each mode
// requires what it serves with, Let's Encrypt the agreement to its terms,
// and the host names are kept in lower case, once each.
func TestHTTPSSettings(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	base := store.Settings()
	if base.HTTPSMode != HTTPSOff || base.HTTPSPort != 8920 || base.HTTPSRedirect || base.ACMEChallenge != ChallengeHTTP01 {
		t.Errorf("defaults: %q %d %v %q", base.HTTPSMode, base.HTTPSPort, base.HTTPSRedirect, base.ACMEChallenge)
	}
	try := func(change func(*Settings)) (Settings, error) {
		t.Helper()
		changed := base
		change(&changed)
		return store.UpdateSettings(ctx, changed)
	}
	saved, err := try(func(s *Settings) {
		s.HTTPSMode, s.ACMEHosts, s.ACMETerms = HTTPSLetsEncrypt, []string{" Media.Example.org. ", "media.example.org", "www.example.org"}, true
		s.ACMEEmail, s.ACMEChallenge, s.HTTPSRedirect = " admin@example.org ", ChallengeTLSALPN01, true
	})
	if err != nil || !slices.Equal(saved.ACMEHosts, []string{"media.example.org", "www.example.org"}) || saved.ACMEEmail != "admin@example.org" {
		t.Errorf("Let's Encrypt: %v %q %v", saved.ACMEHosts, saved.ACMEEmail, err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got.HTTPSMode != HTTPSLetsEncrypt || !got.HTTPSRedirect || got.ACMEChallenge != ChallengeTLSALPN01 ||
		!slices.Equal(got.ACMEHosts, saved.ACMEHosts) {
		t.Errorf("read again: %+v", got)
	}
	saved, err = try(func(s *Settings) {
		s.HTTPSMode, s.HTTPSCertificate, s.HTTPSPrivateKey = HTTPSFiles, " "+systemPath("/certs/live/../live/fullchain.pem"), systemPath("/certs/live/privkey.pem")
	})
	if err != nil || saved.HTTPSCertificate != systemPath("/certs/live/fullchain.pem") {
		t.Errorf("files: %q %v", saved.HTTPSCertificate, err)
	}
	for name, c := range map[string]struct {
		change func(*Settings)
		want   error
	}{
		"unknown mode":           {func(s *Settings) { s.HTTPSMode = "on" }, ErrInvalidHTTPSMode},
		"port 0":                 {func(s *Settings) { s.HTTPSPort = 0 }, ErrInvalidHTTPSPort},
		"files without a key":    {func(s *Settings) { s.HTTPSMode, s.HTTPSCertificate = HTTPSFiles, systemPath("/certs/fullchain.pem") }, ErrInvalidHTTPSPrivateKey},
		"a relative certificate": {func(s *Settings) { s.HTTPSCertificate = "certs/fullchain.pem" }, ErrInvalidHTTPSCertificate},
		"no host names":          {func(s *Settings) { s.HTTPSMode, s.ACMETerms = HTTPSLetsEncrypt, true }, ErrInvalidACMEHosts},
		"terms not agreed":       {func(s *Settings) { s.HTTPSMode, s.ACMEHosts = HTTPSLetsEncrypt, []string{"media.example.org"} }, ErrACMETermsRequired},
		"a wildcard":             {func(s *Settings) { s.ACMEHosts = []string{"*.example.org"} }, ErrInvalidACMEHosts},
		"an IP address":          {func(s *Settings) { s.ACMEHosts = []string{"192.168.1.10"} }, ErrInvalidACMEHosts},
		"a single label":         {func(s *Settings) { s.ACMEHosts = []string{"polyfin"} }, ErrInvalidACMEHosts},
		"an email":               {func(s *Settings) { s.ACMEEmail = "admin" }, ErrInvalidACMEEmail},
		"an unknown challenge":   {func(s *Settings) { s.ACMEChallenge = "dns-01" }, ErrInvalidACMEChallenge},
	} {
		if _, err := try(c.change); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}
