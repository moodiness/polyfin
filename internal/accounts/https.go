package accounts

import (
	"errors"
	"slices"
	"strings"
)

// The ways Polyfin serves HTTPS (see Settings.HTTPSMode), the first by
// default: not at all, with certificate files, or with a certificate from
// Let's Encrypt.
const (
	HTTPSOff         = "off"
	HTTPSFiles       = "files"
	HTTPSLetsEncrypt = "letsencrypt"
)

// HTTPSModes are the values of Settings.HTTPSMode.
var HTTPSModes = []string{HTTPSOff, HTTPSFiles, HTTPSLetsEncrypt}

// DefaultHTTPSPort is the port HTTPS listens on unless the settings choose
// another, Jellyfin's.
const DefaultHTTPSPort = 8920

// The challenges Let's Encrypt checks host names with (see
// Settings.ACMEChallenge), the first by default: a file Polyfin serves on
// its HTTP port, which the internet's port 80 reaches, or a certificate it
// presents on its HTTPS port, which the internet's port 443 reaches.
const (
	ChallengeHTTP01    = "http-01"
	ChallengeTLSALPN01 = "tls-alpn-01"
)

// ACMEChallenges are the values of Settings.ACMEChallenge.
var ACMEChallenges = []string{ChallengeHTTP01, ChallengeTLSALPN01}

// MaxACMEHosts is how many host names a certificate of Let's Encrypt holds
// at most, Let's Encrypt's own limit.
const MaxACMEHosts = 100

// MaxHTTPSFileBytes is the longest path of a certificate or private key
// file.
const MaxHTTPSFileBytes = MaxFolderBytes

var (
	// ErrInvalidHTTPSMode reports a Settings.HTTPSMode not among
	// HTTPSModes.
	ErrInvalidHTTPSMode = errors.New("invalid HTTPS mode")
	// ErrInvalidHTTPSPort reports a Settings.HTTPSPort that is not a TCP
	// port.
	ErrInvalidHTTPSPort = errors.New("invalid HTTPS port")
	// ErrInvalidHTTPSCertificate and ErrInvalidHTTPSPrivateKey report a
	// path that is not absolute, or none while HTTPS is served with files.
	ErrInvalidHTTPSCertificate = errors.New("invalid certificate file")
	ErrInvalidHTTPSPrivateKey  = errors.New("invalid private key file")
	// ErrInvalidACMEHosts reports a host name that is not a public one,
	// more than MaxACMEHosts of them, or none while Let's Encrypt is
	// chosen.
	ErrInvalidACMEHosts = errors.New("invalid host names for Let's Encrypt")
	// ErrInvalidACMEEmail reports a Settings.ACMEEmail that is not an
	// email address.
	ErrInvalidACMEEmail = errors.New("invalid email for Let's Encrypt")
	// ErrACMETermsRequired reports Let's Encrypt chosen without agreeing to
	// its terms.
	ErrACMETermsRequired = errors.New("terms of Let's Encrypt not agreed to")
	// ErrInvalidACMEChallenge reports a Settings.ACMEChallenge not among
	// ACMEChallenges.
	ErrInvalidACMEChallenge = errors.New("invalid challenge for Let's Encrypt")
)

// validHTTPS checks the HTTPS settings, and writes them as they are kept:
// the paths cleaned, the host names in lower case without a final dot,
// once each, none being an empty list. The files are not read: the admin
// API checks them (see admin's unreadableCertificate). The files and the
// host names are only required by the mode that uses them, the others
// being kept for when it is chosen again.
func validHTTPS(settings *Settings) error {
	if !slices.Contains(HTTPSModes, settings.HTTPSMode) {
		return ErrInvalidHTTPSMode
	}
	if settings.HTTPSPort < 1 || settings.HTTPSPort > 65535 {
		return ErrInvalidHTTPSPort
	}
	for _, file := range []struct {
		path *string
		err  error
	}{{&settings.HTTPSCertificate, ErrInvalidHTTPSCertificate}, {&settings.HTTPSPrivateKey, ErrInvalidHTTPSPrivateKey}} {
		cleaned, ok := CleanFolder(strings.TrimSpace(*file.path))
		if !ok || len(cleaned) > MaxHTTPSFileBytes || settings.HTTPSMode == HTTPSFiles && cleaned == "" {
			return file.err
		}
		*file.path = cleaned
	}
	if len(settings.ACMEHosts) > MaxACMEHosts {
		return ErrInvalidACMEHosts
	}
	hosts := make([]string, 0, len(settings.ACMEHosts))
	for _, host := range settings.ACMEHosts {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if !ValidACMEHost(host) {
			return ErrInvalidACMEHosts
		}
		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	if settings.HTTPSMode == HTTPSLetsEncrypt && len(hosts) == 0 {
		return ErrInvalidACMEHosts
	}
	settings.ACMEHosts = hosts
	settings.ACMEEmail = strings.TrimSpace(settings.ACMEEmail)
	if settings.ACMEEmail != "" && !ValidEmail(settings.ACMEEmail) {
		return ErrInvalidACMEEmail
	}
	if settings.HTTPSMode == HTTPSLetsEncrypt && !settings.ACMETerms {
		return ErrACMETermsRequired
	}
	if !slices.Contains(ACMEChallenges, settings.ACMEChallenge) {
		return ErrInvalidACMEChallenge
	}
	return nil
}

// ValidACMEHost reports whether host may be in a certificate of Let's
// Encrypt: a host name of two labels or more, of letters, digits and
// hyphens (see validHostName), without a final dot. A wildcard needs the
// DNS-01 challenge, which Polyfin does not answer.
func ValidACMEHost(host string) bool {
	return validHostName(host) && !strings.HasSuffix(host, ".") && strings.Contains(host, ".") && !strings.Contains(host, "_")
}
