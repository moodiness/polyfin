package https

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// testClock is a clock tests move.
type testClock struct{ now atomic.Int64 }

func newTestClock() *testClock {
	c := &testClock{}
	c.now.Store(time.Now().UnixNano())
	return c
}

func (c *testClock) Now() time.Time          { return time.Unix(0, c.now.Load()) }
func (c *testClock) set(t time.Time)         { c.now.Store(t.UnixNano()) }
func (c *testClock) advance(d time.Duration) { c.now.Add(int64(d)) }

// A certificate is obtained from the ACME server with each challenge, for
// every host name, served, kept in the database with its keys sealed,
// renewed once a third of its lifetime remains, and served again after a
// restart without a new order. A failed renewal is told, with the ACME
// server's error, and kept, the certificate it had staying served.
func TestLetsEncryptOrdersAndRenews(t *testing.T) {
	t.Parallel()
	for _, challenge := range []string{ChallengeHTTP01, ChallengeTLSALPN01} {
		t.Run(challenge, func(t *testing.T) {
			t.Parallel()
			clock := newTestClock()
			box, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
			if err != nil {
				t.Fatal(err)
			}
			pool := migrated.New(t)
			store := NewDatabaseStore(pool, box)
			settings := newTestSettings(Config{Mode: ModeLetsEncrypt, Port: 0, Hosts: []string{"media.example.org", "www.example.org"},
				Email: "admin@example.org", Challenge: challenge})

			// The HTTP port, which HTTP-01 checks reach, and a port that
			// answers nothing, for the host name that fails.
			var service atomic.Pointer[Service]
			plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				service.Load().Handler(tellsScheme).ServeHTTP(w, r)
			}))
			t.Cleanup(plain.Close)
			closed, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			nowhere := closed.Addr().String()
			if err := closed.Close(); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var httpsAddress string
			ca := newTestCA(t, clock.Now, 90*24*time.Hour, func(host, challengeType string) string {
				if host == "fail.example.org" {
					return nowhere
				}
				if challengeType == ChallengeHTTP01 {
					return plain.Listener.Addr().String()
				}
				mu.Lock()
				defer mu.Unlock()
				return httpsAddress
			})
			options := Options{Store: store, Directory: ca.directory(), Client: ca.server.Client(), now: clock.Now}
			startWith := func() (*Service, func()) {
				s, stop := start(t, options, settings, tellsScheme)
				service.Store(s)
				mu.Lock()
				httpsAddress = s.Status().Address
				mu.Unlock()
				return s, stop
			}
			s, stop := startWith()
			address := s.Status().Address
			waitFor(t, "the first certificate", func() bool { return s.Status().Serving })
			for _, host := range []string{"media.example.org", "www.example.org"} {
				if body := secureGet(t, address, host, ca.pool(), "/"); body != "https" {
					t.Errorf("%s: %q", host, body)
				}
			}
			first := served(address, "media.example.org")

			var accountKey, certificate, privateKey string
			if err := pool.QueryRow(t.Context(), "SELECT account_key, certificate, private_key FROM acme_certificate").Scan(&accountKey,
				&certificate, &privateKey); err != nil {
				t.Fatal(err)
			}
			if !secrets.Sealed(accountKey) || !secrets.Sealed(privateKey) || !strings.HasPrefix(certificate, "-----BEGIN CERTIFICATE-----") {
				t.Errorf("kept: account key sealed %v, private key sealed %v, certificate %.30q", secrets.Sealed(accountKey),
					secrets.Sealed(privateKey), certificate)
			}

			// Once a third of its lifetime remains, it is renewed.
			clock.set(first.NotBefore.Add(61 * 24 * time.Hour))
			waitFor(t, "the renewal", func() bool {
				leaf := served(address, "media.example.org")
				return leaf != nil && !leaf.Equal(first) && ca.issuedCount() == 2
			})
			renewed := served(address, "media.example.org")
			if status := s.Status(); status.Renewal != "" || !status.NotAfter.Equal(renewed.NotAfter) {
				t.Errorf("after the renewal: %+v", status)
			}

			// Started again, it serves what the database keeps.
			ca.mu.Lock()
			accounts := len(ca.accounts)
			ca.mu.Unlock()
			stop()
			restarted, _ := startWith()
			if status := restarted.Status(); !status.Serving || !status.NotAfter.Equal(renewed.NotAfter) {
				t.Errorf("restarted: %+v", status)
			}
			if leaf := served(restarted.Status().Address, "www.example.org"); leaf == nil || !leaf.Equal(renewed) {
				t.Error("the certificate kept is not served after a restart")
			}
			if ca.issuedCount() != 2 {
				t.Errorf("restarted: %d certificates issued", ca.issuedCount())
			}

			// A host name the ACME server cannot check: the attempt fails,
			// is told and kept, and the certificate it had stays served.
			settings.set(func(c *Config) { c.Hosts = append(c.Hosts, "fail.example.org") })
			waitFor(t, "the failed renewal", func() bool { return restarted.Status().Renewal != "" })
			if status := restarted.Status(); !strings.Contains(status.Renewal, "fail.example.org") || !status.NotAfter.Equal(renewed.NotAfter) {
				t.Errorf("after a failure: %+v", status)
			}
			var failure string
			if err := pool.QueryRow(t.Context(), "SELECT failure FROM acme_certificate").Scan(&failure); err != nil || failure == "" {
				t.Errorf("the failure is not kept: %q, %v", failure, err)
			}
			ca.mu.Lock()
			if len(ca.accounts) != accounts {
				t.Errorf("%d accounts registered, want %d: the account key is not used again", len(ca.accounts), accounts)
			}
			ca.mu.Unlock()
		})
	}
}

// A certificate obtained after a failed attempt is renewed on time, even
// within the hour a failure waits before it is tried again.
func TestRenewalFollowsASuccessAfterAFailure(t *testing.T) {
	t.Parallel()
	clock := newTestClock()
	settings := newTestSettings(Config{Mode: ModeLetsEncrypt, Port: 0, Hosts: []string{"media.example.org", "fail.example.org"},
		Challenge: ChallengeHTTP01})
	var service atomic.Pointer[Service]
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := service.Load(); s != nil {
			s.Handler(tellsScheme).ServeHTTP(w, r)
		}
	}))
	t.Cleanup(plain.Close)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	nowhere := closed.Addr().String()
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	// A lifetime short enough for the renewal to come within the hour.
	ca := newTestCA(t, clock.Now, 30*time.Minute, func(host, challengeType string) string {
		if host == "fail.example.org" {
			return nowhere
		}
		return plain.Listener.Addr().String()
	})
	s, _ := start(t, Options{Directory: ca.directory(), Client: ca.server.Client(), now: clock.Now}, settings, tellsScheme)
	service.Store(s)
	waitFor(t, "the failed attempt", func() bool { return s.Status().Renewal != "" })

	// The host name that failed is removed: a certificate is obtained at once.
	settings.set(func(c *Config) { c.Hosts = []string{"media.example.org"} })
	waitFor(t, "the certificate", func() bool { return s.Status().Serving && s.Status().Renewal == "" })
	first := served(s.Status().Address, "media.example.org")

	// Twenty minutes later, a third of its lifetime remains.
	clock.advance(21 * time.Minute)
	waitFor(t, "the renewal", func() bool {
		leaf := served(s.Status().Address, "media.example.org")
		return leaf != nil && !leaf.Equal(first) && ca.issuedCount() == 2
	})
}

// After a restart on another ACME directory, as from Let's Encrypt's
// staging environment to its production one, a failed first attempt keeps
// the certificate it had as coming from the old directory: the new one is
// tried again once the hour a failure waits is over, rather than at the
// old certificate's renewal.
func TestNewDirectoryIsTriedAgainAfterAFailure(t *testing.T) {
	t.Parallel()
	clock := newTestClock()
	box, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	pool := migrated.New(t)
	store := NewDatabaseStore(pool, box)
	settings := newTestSettings(Config{Mode: ModeLetsEncrypt, Port: 0, Hosts: []string{"media.example.org"}, Challenge: ChallengeHTTP01})
	var service atomic.Pointer[Service]
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := service.Load(); s != nil {
			s.Handler(tellsScheme).ServeHTTP(w, r)
		}
	}))
	t.Cleanup(plain.Close)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	nowhere := closed.Addr().String()
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	staging := newTestCA(t, clock.Now, 90*24*time.Hour, func(string, string) string { return plain.Listener.Addr().String() })
	var failing atomic.Bool
	failing.Store(true)
	production := newTestCA(t, clock.Now, 90*24*time.Hour, func(string, string) string {
		if failing.Load() {
			return nowhere
		}
		return plain.Listener.Addr().String()
	})

	s, stop := start(t, Options{Store: store, Directory: staging.directory(), Client: staging.server.Client(), now: clock.Now}, settings, tellsScheme)
	service.Store(s)
	waitFor(t, "the certificate from the first directory", func() bool { return s.Status().Serving })
	stop()

	// Restarted on another directory, whose first attempt fails.
	s, _ = start(t, Options{Store: store, Directory: production.directory(), Client: production.server.Client(), now: clock.Now}, settings,
		tellsScheme)
	service.Store(s)
	waitFor(t, "the failed attempt at the other directory", func() bool { return s.Status().Renewal != "" })

	// It works again, and the hour a failure waits is over.
	failing.Store(false)
	clock.advance(2 * time.Hour)
	waitFor(t, "a certificate from the other directory", func() bool { return production.issuedCount() == 1 && s.Status().Renewal == "" })
}

// An attempt canceled by Polyfin stopping is no failure: nothing is kept
// nor told, and the next start tries again at once.
func TestStoppingDuringAnAttemptKeepsNoFailure(t *testing.T) {
	t.Parallel()
	clock := newTestClock()
	box, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	pool := migrated.New(t)
	store := NewDatabaseStore(pool, box)
	settings := newTestSettings(Config{Mode: ModeLetsEncrypt, Port: 0, Hosts: []string{"media.example.org"}, Challenge: ChallengeHTTP01})
	var service atomic.Pointer[Service]
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := service.Load(); s != nil {
			s.Handler(tellsScheme).ServeHTTP(w, r)
		}
	}))
	t.Cleanup(plain.Close)
	// A port that takes the ACME server's check and never answers it,
	// holding the attempt until Polyfin stops.
	hang, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hang.Close() })
	checking := make(chan net.Conn, 1)
	go func() {
		for {
			conn, err := hang.Accept()
			if err != nil {
				return
			}
			select {
			case checking <- conn:
			default:
				_ = conn.Close()
			}
		}
	}()
	var hanging atomic.Bool
	hanging.Store(true)
	ca := newTestCA(t, clock.Now, 90*24*time.Hour, func(string, string) string {
		if hanging.Load() {
			return hang.Addr().String()
		}
		return plain.Listener.Addr().String()
	})
	options := Options{Store: store, Directory: ca.directory(), Client: ca.server.Client(), now: clock.Now}
	s, stop := start(t, options, settings, tellsScheme)
	service.Store(s)
	conn := <-checking
	// Closed before the ACME server stops, which waits for the check.
	t.Cleanup(func() { _ = conn.Close() })
	stop()
	waitFor(t, "the attempt's end", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.acme.issuing
	})
	var failure string
	if err := pool.QueryRow(t.Context(), "SELECT coalesce((SELECT failure FROM acme_certificate), '')").Scan(&failure); err != nil || failure != "" {
		t.Errorf("a failure is kept: %q, %v", failure, err)
	}
	if renewal := s.Status().Renewal; renewal != "" {
		t.Errorf("a failure is told: %q", renewal)
	}

	hanging.Store(false)
	restarted, _ := start(t, options, settings, tellsScheme)
	service.Store(restarted)
	waitFor(t, "a certificate after the restart", func() bool { return ca.issuedCount() == 1 && restarted.Status().Serving })
}

// Choosing another challenge obtains a new certificate at once, through it.
func TestChangingTheChallengeObtainsANewCertificate(t *testing.T) {
	t.Parallel()
	clock := newTestClock()
	settings := newTestSettings(Config{Mode: ModeLetsEncrypt, Port: 0, Hosts: []string{"media.example.org"}, Challenge: ChallengeHTTP01})
	var service atomic.Pointer[Service]
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := service.Load(); s != nil {
			s.Handler(tellsScheme).ServeHTTP(w, r)
		}
	}))
	t.Cleanup(plain.Close)
	var checked sync.Map
	ca := newTestCA(t, clock.Now, 90*24*time.Hour, func(host, challengeType string) string {
		checked.Store(challengeType, true)
		if challengeType == ChallengeHTTP01 {
			return plain.Listener.Addr().String()
		}
		return service.Load().Status().Address
	})
	s, _ := start(t, Options{Directory: ca.directory(), Client: ca.server.Client(), now: clock.Now}, settings, tellsScheme)
	service.Store(s)
	waitFor(t, "the first certificate", func() bool { return s.Status().Serving && ca.issuedCount() == 1 })
	settings.set(func(c *Config) { c.Challenge = ChallengeTLSALPN01 })
	waitFor(t, "a certificate through TLS-ALPN-01", func() bool { return ca.issuedCount() == 2 })
	if _, ok := checked.Load(ChallengeTLSALPN01); !ok {
		t.Error("TLS-ALPN-01 was not checked")
	}
}
