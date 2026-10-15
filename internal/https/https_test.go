package https

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/discovery"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// testSettings stands for the settings: Settings reads them, Changed tells
// their changes as accounts.Store.SettingsChanged does.
type testSettings struct {
	mu      sync.Mutex
	config  Config
	changed chan struct{}
}

func newTestSettings(config Config) *testSettings {
	return &testSettings{config: config, changed: make(chan struct{})}
}

func (s *testSettings) Settings() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}

func (s *testSettings) Changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

// set changes the settings, and tells it.
func (s *testSettings) set(change func(*Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&s.config)
	close(s.changed)
	s.changed = make(chan struct{})
}

// testCertificate is a self-signed certificate for names, as PEM files
// hold it, with its key, ECDSA or RSA.
type testCertificate struct {
	leaf           *x509.Certificate
	certificatePEM []byte
	privateKeyPEM  []byte
}

func newTestCertificate(t *testing.T, rsaKey bool, notAfter time.Time, names ...string) testCertificate {
	t.Helper()
	var key crypto.Signer
	var err error
	if rsaKey {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	c := testCertificate{leaf: leaf, certificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
	if rsaKey {
		// An RSA key as OpenSSL wrote them for long: PKCS #1.
		c.privateKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey))})
	} else {
		encoded, err := encodeKey(key)
		if err != nil {
			t.Fatal(err)
		}
		c.privateKeyPEM = encoded
	}
	return c
}

// write writes the certificate and its key at the paths given; empty
// leaves one alone.
func (c testCertificate) write(t *testing.T, certificatePath, keyPath string) {
	t.Helper()
	if keyPath != "" {
		writeFile(t, keyPath, c.privateKeyPEM)
	}
	if certificatePath != "" {
		writeFile(t, certificatePath, c.certificatePEM)
	}
}

// writeFile replaces a file as renewal tools do: a new file renamed over
// the former one, with a time of change of its own.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	temporary := path + ".new"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
}

// waitFor waits until done reports true, failing the test after a long
// while: CI runners can pause the service for seconds.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// served returns the certificate HTTPS presents at address to a client
// asking for name, without checking it; nil when the handshake fails.
func served(address, name string) *x509.Certificate {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", address, &tls.Config{ServerName: name, InsecureSkipVerify: true})
	if err != nil {
		return nil
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0]
}

// secureGet requests path over HTTPS at address for name, trusting roots
// only, and returns the body.
func secureGet(t *testing.T, address, name string, roots *x509.CertPool, path string) string {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: name, RootCAs: roots}}}
	defer client.CloseIdleConnections()
	response, err := client.Get("https://" + address + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// tellsScheme answers whether the request came over HTTPS.
var tellsScheme = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if r.TLS != nil {
		_, _ = io.WriteString(w, "https")
		return
	}
	_, _ = io.WriteString(w, "http")
})

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// start starts a service with settings, on a free port of the loopback
// interface, stopped at the end of the test, or before by stop.
func start(t *testing.T, options Options, settings *testSettings, handler http.Handler) (s *Service, stop func()) {
	t.Helper()
	options.Host = "127.0.0.1"
	options.Settings, options.Changed = settings.Settings, settings.Changed
	options.Logger = testLogger(t)
	if options.checkEvery == 0 {
		options.checkEvery = 20 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	s = New(options)
	s.Start(ctx, handler)
	stop = func() {
		cancel()
		if err := s.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(stop)
	return s, stop
}

// HTTPS serves the certificate files given, then, without a restart, those
// that replace them. While they cannot be read, mismatched or missing, the
// last good certificate stays served and the status tells why.
func TestFilesAreServedAndReadAgainWithoutRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certificatePath, keyPath := filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
	first := newTestCertificate(t, false, time.Now().Add(90*24*time.Hour), "media.example.org")
	first.write(t, certificatePath, keyPath)
	settings := newTestSettings(Config{Mode: ModeFiles, Port: 0, Certificate: certificatePath, PrivateKey: keyPath})
	s, _ := start(t, Options{}, settings, tellsScheme)

	status := s.Status()
	if status.Address == "" || !status.Serving || status.Files != "" || status.NotAfter.Unix() != first.leaf.NotAfter.Unix() {
		t.Fatalf("status: %+v", status)
	}
	roots := x509.NewCertPool()
	roots.AddCert(first.leaf)
	if body := secureGet(t, status.Address, "media.example.org", roots, "/System/Info/Public"); body != "https" {
		t.Errorf("served %q", body)
	}

	// Another tool renews the certificate, with an RSA key: the key first,
	// then the certificate.
	second := newTestCertificate(t, true, time.Now().Add(60*24*time.Hour), "media.example.org", "www.example.org")
	second.write(t, certificatePath, keyPath)
	waitFor(t, "the renewed certificate", func() bool {
		leaf := served(status.Address, "media.example.org")
		return leaf != nil && leaf.Equal(second.leaf)
	})
	if status := s.Status(); status.Files != "" || len(status.Names) != 2 {
		t.Errorf("after the renewal: %+v", status)
	}

	// A certificate written without its key: the files no longer match.
	first.write(t, certificatePath, "")
	waitFor(t, "mismatched files", func() bool { return s.Status().Files == FilesMismatch })
	if leaf := served(status.Address, "media.example.org"); leaf == nil || !leaf.Equal(second.leaf) {
		t.Error("the last good certificate is not served while the files do not match")
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a missing file", func() bool {
		status := s.Status()
		return status.Files == FilesMissing && status.File == keyPath
	})
	if leaf := served(status.Address, "media.example.org"); leaf == nil || !leaf.Equal(second.leaf) {
		t.Error("the last good certificate is not served while a file is missing")
	}
	first.write(t, "", keyPath)
	waitFor(t, "the files again", func() bool {
		leaf := served(status.Address, "media.example.org")
		return leaf != nil && leaf.Equal(first.leaf) && s.Status().Files == ""
	})

	// Turned off, the port is left at once.
	settings.set(func(c *Config) { c.Mode = ModeOff })
	waitFor(t, "HTTPS off", func() bool { return s.Status().Address == "" })
	if leaf := served(status.Address, "media.example.org"); leaf != nil {
		t.Error("HTTPS still served once off")
	}
}

// Certificate files are read with their chain and an RSA or ECDSA key;
// those that cannot be read, or do not belong together, are refused with
// why.
func TestLoadFilesTellsWhatFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ecdsaCertificate := newTestCertificate(t, false, time.Now().Add(time.Hour), "media.example.org")
	rsaCertificate := newTestCertificate(t, true, time.Now().Add(time.Hour), "media.example.org")
	path := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if data != nil {
			writeFile(t, p, data)
		}
		return p
	}
	ecdsaPEM, ecdsaKey := path("ecdsa.pem", ecdsaCertificate.certificatePEM), path("ecdsa.key", ecdsaCertificate.privateKeyPEM)
	rsaPEM, rsaKey := path("rsa.pem", rsaCertificate.certificatePEM), path("rsa.key", rsaCertificate.privateKeyPEM)
	garbage := path("garbage.pem", []byte("not a certificate"))
	for _, c := range []struct {
		certificate, key string
		problem, file    string
	}{
		{ecdsaPEM, ecdsaKey, "", ""},
		{rsaPEM, rsaKey, "", ""},
		{ecdsaPEM, rsaKey, FilesMismatch, rsaKey},
		{rsaPEM, ecdsaKey, FilesMismatch, ecdsaKey},
		{path("absent.pem", nil), ecdsaKey, FilesMissing, filepath.Join(dir, "absent.pem")},
		{ecdsaPEM, path("absent.key", nil), FilesMissing, filepath.Join(dir, "absent.key")},
		{garbage, ecdsaKey, FilesUnreadable, garbage},
		{ecdsaPEM, garbage, FilesUnreadable, garbage},
		{ecdsaPEM, ecdsaPEM, FilesUnreadable, ecdsaPEM},
	} {
		certificate, err := LoadFiles(c.certificate, c.key)
		var problem *FilesError
		switch {
		case c.problem == "" && (err != nil || certificate.Leaf == nil):
			t.Errorf("%s, %s: %v", c.certificate, c.key, err)
		case c.problem != "" && (!errors.As(err, &problem) || problem.Problem != c.problem || problem.File != c.file):
			t.Errorf("%s, %s: %v, want %s of %s", c.certificate, c.key, err, c.problem, c.file)
		}
	}
}

// A port HTTPS cannot listen on is told, and tried again: HTTPS starts
// once it is free.
func TestPortThatCannotBeListenedOnIsToldAndTriedAgain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certificatePath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	newTestCertificate(t, false, time.Now().Add(time.Hour), "media.example.org").write(t, certificatePath, keyPath)
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := taken.Addr().(*net.TCPAddr).Port
	settings := newTestSettings(Config{Mode: ModeFiles, Port: port, Certificate: certificatePath, PrivateKey: keyPath})
	s, _ := start(t, Options{}, settings, tellsScheme)
	if status := s.Status(); !status.PortFailed || status.Port != port || status.PortError == "" || status.Address != "" {
		t.Errorf("a port in use: %+v", status)
	}
	if err := taken.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the port once free", func() bool {
		status := s.Status()
		return !status.PortFailed && status.Address == "127.0.0.1:"+strconv.Itoa(port)
	})
}

// Over HTTP, while the redirect is on and HTTPS served, clients on the
// internet get a 308 to the same address on https:// and the HTTPS port;
// the local network, the ACME server's checks, and requests a known proxy
// received over HTTPS are never redirected.
func TestInternetClientsAreRedirectedToHTTPS(t *testing.T) {
	t.Parallel()
	s := New(Options{})
	s.tokens["token-1"] = "token-1.thumbprint"
	proxies := forwarded.New(func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("192.168.1.2/32")} }, nil)
	handler := proxies.Handler(s.Handler(tellsScheme))
	call := func(method, remote, target string, header map[string]string, secure bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		r.Host = "media.example.org:8096"
		r.RemoteAddr = remote
		for name, value := range header {
			r.Header.Set(name, value)
		}
		if secure {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	internet := "203.0.113.7:51000"

	// Not served yet: nothing is redirected.
	if w := call(http.MethodGet, internet, "/web/", nil, false); w.Code != http.StatusOK {
		t.Errorf("before HTTPS is served: %d", w.Code)
	}
	s.serving.Store(&Config{Mode: ModeFiles, Port: 8920, Redirect: true})
	for _, c := range []struct {
		name, method, remote, target string
		header                       map[string]string
		secure                       bool
		location                     string
	}{
		{"internet", http.MethodGet, internet, "/web/index.html?x=1", nil, false, "https://media.example.org:8920/web/index.html?x=1"},
		{"internet, a POST", http.MethodPost, internet, "/Users/AuthenticateByName", nil, false, "https://media.example.org:8920/Users/AuthenticateByName"},
		{"internet, IPv6", http.MethodGet, "[2001:db8::7]:51000", "/", nil, false, "https://media.example.org:8920/"},
		{"internet over HTTPS", http.MethodGet, internet, "/web/", nil, true, ""},
		{"private IPv4", http.MethodGet, "192.168.1.20:51000", "/web/", nil, false, ""},
		{"private IPv4, 10/8", http.MethodGet, "10.1.2.3:51000", "/web/", nil, false, ""},
		{"loopback", http.MethodGet, "127.0.0.1:51000", "/web/", nil, false, ""},
		{"IPv6 unique local", http.MethodGet, "[fd00::5]:51000", "/web/", nil, false, ""},
		{"IPv6 link-local", http.MethodGet, "[fe80::1]:51000", "/web/", nil, false, ""},
		{"IPv6 loopback", http.MethodGet, "[::1]:51000", "/web/", nil, false, ""},
		{"ACME check of a token under way", http.MethodGet, internet, "/.well-known/acme-challenge/token-1", nil, false, ""},
		{"ACME check of another token", http.MethodGet, internet, "/.well-known/acme-challenge/token-2", nil, false, ""},
		{"known proxy, over HTTPS", http.MethodGet, "192.168.1.2:40000", "/web/",
			map[string]string{"X-Forwarded-For": "203.0.113.7", "X-Forwarded-Proto": "https"}, false, ""},
		{"known proxy, over HTTP", http.MethodGet, "192.168.1.2:40000", "/web/",
			map[string]string{"X-Forwarded-For": "203.0.113.7", "X-Forwarded-Proto": "http"}, false, "https://media.example.org:8920/web/"},
		{"known proxy, a local client", http.MethodGet, "192.168.1.2:40000", "/web/",
			map[string]string{"X-Forwarded-For": "192.168.1.30"}, false, ""},
		{"unknown proxy saying HTTPS", http.MethodGet, "203.0.113.9:40000", "/web/",
			map[string]string{"X-Forwarded-For": "192.168.1.30", "X-Forwarded-Proto": "https"}, false, "https://media.example.org:8920/web/"},
	} {
		w := call(c.method, c.remote, c.target, c.header, c.secure)
		switch {
		case c.location != "" && (w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != c.location):
			t.Errorf("%s: %d %q, want 308 to %s", c.name, w.Code, w.Header().Get("Location"), c.location)
		case c.location == "" && w.Code != http.StatusOK:
			t.Errorf("%s: %d %q, want no redirect", c.name, w.Code, w.Header().Get("Location"))
		}
	}
	if w := call(http.MethodGet, internet, "/.well-known/acme-challenge/token-1", nil, false); w.Body.String() != "token-1.thumbprint" {
		t.Errorf("the ACME check got %q", w.Body.String())
	}
	s.serving.Store(&Config{Mode: ModeFiles, Port: 443, Redirect: true})
	if w := call(http.MethodGet, internet, "/web/", nil, false); w.Header().Get("Location") != "https://media.example.org/web/" {
		t.Errorf("to port 443: %q", w.Header().Get("Location"))
	}
	s.serving.Store(&Config{Mode: ModeFiles, Port: 8920, Redirect: false})
	if w := call(http.MethodGet, internet, "/web/", nil, false); w.Code != http.StatusOK {
		t.Errorf("redirect off: %d", w.Code)
	}
}

// While HTTPS is served, discovery still announces the HTTP address: a
// certificate for a public name does not match the server's address on
// the local network, where HTTP is never redirected.
func TestDiscoveryAnnouncesHTTPWhileHTTPSIsServed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certificatePath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	newTestCertificate(t, false, time.Now().Add(time.Hour), "media.example.org").write(t, certificatePath, keyPath)
	settings := newTestSettings(Config{Mode: ModeFiles, Port: 0, Redirect: true, Certificate: certificatePath, PrivateKey: keyPath})
	s, _ := start(t, Options{}, settings, tellsScheme)
	if !s.Status().Serving {
		t.Fatalf("HTTPS not served: %+v", s.Status())
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	local := discovery.Start(ctx, discovery.Options{Port: 0, HTTP: &net.TCPAddr{Port: 8096}, ServerID: "0123456789abcdef0123456789abcdef",
		Settings: func() discovery.Settings { return discovery.Settings{Enabled: true, Name: "Polyfin"} },
		Changed:  func() <-chan struct{} { return nil }, Logger: testLogger(t)})
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(local.Status().Port))))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("who is JellyfinServer?")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatal(err)
	}
	var answer discovery.Answer
	if err := json.Unmarshal(buffer[:n], &answer); err != nil || answer.Address != "http://127.0.0.1:8096" {
		t.Errorf("announced %s, %v", buffer[:n], err)
	}
}
