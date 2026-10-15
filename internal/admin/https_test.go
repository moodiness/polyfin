package admin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/https"
)

// writeCertificate writes a self-signed certificate for name and its
// ECDSA key into dir, and returns their paths.
func writeCertificate(t *testing.T, dir, name string) (certificatePath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificatePath, keyPath = filepath.Join(dir, name+".pem"), filepath.Join(dir, name+".key")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certificatePath, keyPath
}

// Saving certificate files checks that they can be read and belong
// together: missing, unreadable or mismatched files are refused, and
// nothing is saved. Saving the same files again checks nothing, so that a
// renewal under way elsewhere never stops other settings from being saved.
func TestCertificateFilesAreCheckedAtSave(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	dir := t.TempDir()
	certificate, key := writeCertificate(t, dir, "media.example.org")
	otherCertificate, otherKey := writeCertificate(t, dir, "other.example.org")
	garbage := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The fields a PUT always sends, with those changed.
	save := func(changed map[string]any) (int, map[string]any) {
		t.Helper()
		body := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
		maps.Copy(body, changed)
		status, answer, _ := administrator.call(http.MethodPut, "/settings", body)
		return status, answer
	}
	put := func(certificatePath, keyPath string) (int, map[string]any) {
		t.Helper()
		return save(map[string]any{"httpsMode": "files", "httpsPort": 8920, "httpsCertificate": certificatePath, "httpsPrivateKey": keyPath})
	}
	for _, c := range []struct {
		certificate, key, code string
	}{
		{certificate, otherKey, "https_files_mismatch"},
		{otherCertificate, key, "https_files_mismatch"},
		{filepath.Join(dir, "absent.pem"), key, "https_certificate_missing"},
		{certificate, filepath.Join(dir, "absent.key"), "https_private_key_missing"},
		{garbage, key, "https_certificate_unreadable"},
		{certificate, garbage, "https_private_key_unreadable"},
		{"certs/fullchain.pem", key, "invalid_https_certificate"},
		{certificate, "", "invalid_https_private_key"},
	} {
		if status, body := put(c.certificate, c.key); status != http.StatusBadRequest || body["error"] != c.code {
			t.Errorf("%s, %s: %d %v, want %s", c.certificate, c.key, status, body, c.code)
		}
	}
	if settings := api.store.Settings(); settings.HTTPSMode != "off" || settings.HTTPSCertificate != "" {
		t.Fatalf("refused files saved: %+v", settings)
	}
	status, body := put(certificate, key)
	if status != http.StatusOK || body["httpsMode"] != "files" || body["httpsCertificate"] != certificate || body["httpsPrivateKey"] != key {
		t.Fatalf("matching files: %d %v", status, body)
	}
	// The key replaced by another tool, not yet with its certificate.
	data, err := os.ReadFile(otherKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if status, body := save(map[string]any{"serverName": "Living room"}); status != http.StatusOK ||
		body["serverName"] != "Living room" {
		t.Errorf("other settings, the same files: %d %v", status, body)
	}
	// Let's Encrypt needs host names and the agreement to its terms.
	for _, c := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"httpsMode": "letsencrypt", "acmeHosts": []string{}, "acmeTerms": true}, "invalid_acme_hosts"},
		{map[string]any{"httpsMode": "letsencrypt", "acmeHosts": []string{"media.example.org"}, "acmeTerms": false}, "acme_terms_required"},
		{map[string]any{"httpsMode": "letsencrypt", "acmeHosts": []string{"*.example.org"}, "acmeTerms": true}, "invalid_acme_hosts"},
		{map[string]any{"acmeChallenge": "dns-01"}, "invalid_acme_challenge"},
		{map[string]any{"httpsPort": 70000}, "invalid_https_port"},
	} {
		if status, body := save(c.body); status != http.StatusBadRequest || body["error"] != c.code {
			t.Errorf("%v: %d %v, want %s", c.body, status, body, c.code)
		}
	}
	status, body = save(map[string]any{"httpsMode": "letsencrypt",
		"acmeHosts": []string{"Media.example.org"}, "acmeEmail": "admin@example.org", "acmeTerms": true, "acmeChallenge": "tls-alpn-01",
		"httpsRedirect": true})
	if status != http.StatusOK || !reflect.DeepEqual(body["acmeHosts"], []any{"media.example.org"}) || body["acmeChallenge"] != "tls-alpn-01" ||
		body["httpsRedirect"] != true {
		t.Errorf("Let's Encrypt: %d %v", status, body)
	}
}

// Health warns of a certificate that expires within 14 days, an error once
// it expired, of certificate files that cannot be served, of a renewal
// that failed, with Let's Encrypt's error, and of a port HTTPS cannot
// listen on; notifications tell them, in French too. HTTPS off tells
// nothing.
func TestHealthTellsHTTPSProblems(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	status := https.Status{Mode: https.ModeLetsEncrypt, Port: 8920, PortFailed: true, Files: https.FilesMismatch, File: "/certs/privkey.pem",
		Renewal: "acme: authorization error for media.example.org: connection refused", Serving: true,
		NotAfter: now.Add(10 * 24 * time.Hour)}
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Now = func() time.Time { return now }
		o.Health = HealthSources{HTTPS: func() https.Status { return status }}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems", nil)
	expires := now.Add(10 * 24 * time.Hour).Format(time.RFC3339)
	want := []any{
		map[string]any{"key": "https:port", "code": "https_port", "tone": "warning", "to": "/settings/security#https-port", "transient": false,
			"port": float64(8920)},
		map[string]any{"key": "https:files", "code": "https_files", "tone": "warning", "to": "/settings/security#https-certificate",
			"transient": false, "failure": "mismatch", "file": "/certs/privkey.pem"},
		map[string]any{"key": "https:renewal", "code": "https_renewal", "tone": "warning", "to": "/settings/security#https", "transient": false,
			"error": "acme: authorization error for media.example.org: connection refused"},
		map[string]any{"key": "https:expiry", "code": "https_expiry", "tone": "warning", "to": "/settings/security#https", "transient": false,
			"expires": expires},
	}
	if found, _ := body["problems"].([]any); !reflect.DeepEqual(found, want) {
		t.Fatalf("problems:\n%v\nwant\n%v", found, want)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 4 || !strings.Contains(told[3].Text, "expires on 2026-10-25") ||
		!strings.Contains(told[2].Text, "connection refused") || !strings.Contains(told[0].Text, "TCP port 8920") {
		t.Errorf("notifications: %+v %v", told, err)
	}

	// Expired, and told in French.
	status = https.Status{Mode: https.ModeFiles, Port: 8920, Serving: true, NotAfter: now.Add(-time.Hour)}
	settings := api.store.Settings()
	settings.Language = "fr"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	told, err = HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || told[0].Severity != "error" || told[0].Text != "Le certificat HTTPS a expiré le 2026-10-15." {
		t.Errorf("expired: %+v %v", told, err)
	}
	// Far from its end, nothing.
	status = https.Status{Mode: https.ModeFiles, Port: 8920, Serving: true, NotAfter: now.Add(15 * 24 * time.Hour)}
	if told, err := HealthProblems(*options)(t.Context()); err != nil || len(told) != 0 {
		t.Errorf("valid for 15 days: %+v %v", told, err)
	}
	status = https.Status{Mode: https.ModeOff, PortFailed: true}
	if told, err := HealthProblems(*options)(t.Context()); err != nil || len(told) != 0 {
		t.Errorf("off: %+v %v", told, err)
	}
}

// Over HTTPS, the admin app's session cookie is marked secure, as behind
// a proxy that says so.
func TestSessionCookieIsSecureOverHTTPS(t *testing.T) {
	t.Parallel()
	for target, secure := range map[string]bool{"https://media.example.org:8920/admin/api/session": true,
		"http://192.168.1.10:8096/admin/api/session": false} {
		w := httptest.NewRecorder()
		setCookie(w, httptest.NewRequest(http.MethodPost, target, nil), "token", time.Now().Add(time.Hour))
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Secure != secure {
			t.Errorf("%s: %+v", target, cookies)
		}
	}
}
