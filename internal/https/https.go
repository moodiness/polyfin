// Package https serves Polyfin over HTTPS, on a port of its own next to
// HTTP, while the settings turn it on: with certificate files, read again
// when they change, or with a certificate it obtains and renews from Let's
// Encrypt, or another ACME server, through the HTTP-01 or TLS-ALPN-01
// challenge. Whatever happens to HTTPS, HTTP keeps working: a port it
// cannot listen on, files it cannot read and renewals that fail are told
// in System › Health, never stop Polyfin. Requests from the internet that
// come over HTTP may be sent to HTTPS (see Service.Handler).
package https

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"
)

// The ways HTTPS is served, as accounts.HTTPSModes names them.
const (
	ModeOff         = "off"
	ModeFiles       = "files"
	ModeLetsEncrypt = "letsencrypt"
)

// The challenges an ACME server checks host names with, as
// accounts.ACMEChallenges names them.
const (
	ChallengeHTTP01    = "http-01"
	ChallengeTLSALPN01 = "tls-alpn-01"
)

// LetsEncrypt is the directory of Let's Encrypt's ACME server.
const LetsEncrypt = "https://acme-v02.api.letsencrypt.org/directory"

// ExpiresSoon is how close to its end a certificate served is told in
// System › Health.
const ExpiresSoon = 14 * 24 * time.Hour

// checkEvery is how often the files are looked at for a change, a port
// that could not be listened on is tried again, and the certificate of
// Let's Encrypt is checked for renewal.
const checkEvery = time.Minute

// shutdownWait is how long the connections of a port HTTPS leaves have to
// end before they are closed.
const shutdownWait = 5 * time.Second

// Config is what the settings choose, read at each change of them.
type Config struct {
	// Mode is one of ModeOff, ModeFiles and ModeLetsEncrypt.
	Mode string
	// Port is the TCP port HTTPS listens on.
	Port int
	// Redirect sends the requests of clients on the internet that come
	// over HTTP to HTTPS (see Service.Handler), on the port of
	// PublicAddress, the address people open Polyfin at, when it is an
	// https:// one.
	Redirect      bool
	PublicAddress string
	// Certificate and PrivateKey are the paths of the PEM files served
	// with ModeFiles: the certificate with its chain, and its RSA or ECDSA
	// key.
	Certificate, PrivateKey string
	// Hosts are the host names the certificate of Let's Encrypt is for,
	// Email the account's contact, empty for none, and Challenge how the
	// names are checked: ChallengeHTTP01 or ChallengeTLSALPN01.
	Hosts     []string
	Email     string
	Challenge string
}

// Options are what New needs.
type Options struct {
	// Host is the address HTTPS listens on, HTTP's: empty for every one.
	Host string
	// Settings returns what the settings choose; Changed tells when they
	// change.
	Settings func() Config
	Changed  func() <-chan struct{}
	// Store keeps the account and the certificate of Let's Encrypt; nil
	// keeps them in memory only.
	Store Store
	// Directory is the ACME server's directory, LetsEncrypt when empty,
	// and Client the HTTP client that talks to it; nil uses
	// http.DefaultClient.
	Directory string
	Client    *http.Client
	// ReadHeaderTimeout is HTTP's, given to the HTTPS server too.
	ReadHeaderTimeout time.Duration
	Logger            *slog.Logger
	// checkEvery replaces checkEvery in tests; now replaces time.Now.
	checkEvery time.Duration
	now        func() time.Time
}

// The problems of certificate files (see Status.Files).
const (
	// FilesMissing: a file does not exist.
	FilesMissing = "missing"
	// FilesUnreadable: a file cannot be read, or holds no PEM certificate
	// or key Polyfin reads.
	FilesUnreadable = "unreadable"
	// FilesMismatch: the private key is not the certificate's.
	FilesMismatch = "mismatch"
)

// Status is how HTTPS stands, for System › Health.
type Status struct {
	// Mode is the mode the settings choose, and Port the port they
	// choose; PortFailed tells that it cannot be listened on, with
	// PortError, and Address is the address listened on, empty for none.
	Mode       string
	Port       int
	PortFailed bool
	PortError  string
	Address    string
	// Files is a problem of certificate files, empty for none, the file
	// it is about being File; FilesError tells it in English.
	Files      string
	File       string
	FilesError string
	// Renewal is the error of the last attempt to obtain or renew the
	// certificate of Let's Encrypt, empty once one succeeds, at Failed.
	Renewal string
	Failed  time.Time
	// Serving tells that a certificate is served, for Names, from
	// NotBefore to NotAfter.
	Serving   bool
	Names     []string
	NotBefore time.Time
	NotAfter  time.Time
}

// ExpiresSoon reports whether the certificate served ends within
// ExpiresSoon of now, or has ended.
func (s Status) ExpiresSoon(now time.Time) bool {
	return s.Serving && s.NotAfter.Sub(now) < ExpiresSoon
}

// Service serves HTTPS as the settings choose.
type Service struct {
	options Options
	// handler serves the requests that come over HTTPS: HTTP's own.
	handler http.Handler
	// certificate is the certificate served, nil for none.
	certificate atomic.Pointer[tls.Certificate]
	// serving tells that HTTPS listens and has a certificate, for the
	// redirect, with the settings it was applied with.
	serving atomic.Pointer[Config]

	// The loop's own state: the server listening, on port, with its
	// listener.
	server   *http.Server
	listener net.Listener
	port     int
	// mode is the mode certificate was obtained in, and files what the
	// certificate files were when last read.
	mode  string
	files filesStamp

	mu     sync.Mutex
	status Status
	// challenges answers the challenges of the ACME server under way:
	// the key authorizations of HTTP-01 by token, the certificates of
	// TLS-ALPN-01 by host name.
	tokens map[string]string
	alpn   map[string]*tls.Certificate
	acme   acmeState

	wake chan struct{}
	done chan struct{}
}

// New returns a service that serves nothing until Start.
func New(options Options) *Service {
	if options.Directory == "" {
		options.Directory = LetsEncrypt
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	if options.checkEvery == 0 {
		options.checkEvery = checkEvery
	}
	if options.now == nil {
		options.now = time.Now
	}
	return &Service{options: options, tokens: map[string]string{}, alpn: map[string]*tls.Certificate{},
		wake: make(chan struct{}, 1), done: make(chan struct{})}
}

// Start serves HTTPS with handler, HTTP's own, as the settings choose now,
// then follows them until ctx ends, when it stops listening (see
// Shutdown). It is called once.
func (s *Service) Start(ctx context.Context, handler http.Handler) {
	s.handler = handler
	// The channel is taken before the settings are read, so that no
	// change is missed.
	var changed <-chan struct{}
	if s.options.Changed != nil {
		changed = s.options.Changed()
	}
	s.apply(ctx)
	go s.run(ctx, changed)
}

// run applies the settings at each change of them, at each end of an
// attempt with Let's Encrypt, and every checkEvery, until ctx ends.
func (s *Service) run(ctx context.Context, changed <-chan struct{}) {
	defer close(s.done)
	ticker := time.NewTicker(s.options.checkEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopListening()
			return
		case <-changed:
			changed = s.options.Changed()
		case <-ticker.C:
		case <-s.wake:
		}
		s.apply(ctx)
	}
}

// Shutdown waits until the service stops, once the context Start was
// given ends, its server having closed its connections.
func (s *Service) Shutdown(ctx context.Context) error {
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// apply listens and finds the certificate as config chooses: the port
// first, which the TLS-ALPN-01 challenge needs.
func (s *Service) apply(ctx context.Context) {
	config := s.options.Settings()
	config.Hosts = slices.Clone(config.Hosts)
	if config.Mode != s.mode {
		// A certificate of another mode is not served.
		s.certificate.Store(nil)
		s.files = filesStamp{}
		s.mode = config.Mode
		s.mu.Lock()
		s.status.Files, s.status.File, s.status.FilesError = "", "", ""
		s.mu.Unlock()
	}
	s.listen(config)
	switch config.Mode {
	case ModeFiles:
		s.readFiles(config)
	case ModeLetsEncrypt:
		s.renew(ctx, config)
	}
	s.report(config)
}

// listen listens on config's port while HTTPS is on, and stops listening
// on the one it leaves. A port that cannot be listened on is told in
// Status, and tried again at the next apply.
func (s *Service) listen(config Config) {
	if config.Mode == ModeOff || s.port != config.Port {
		s.stopListening()
	}
	if config.Mode == ModeOff || s.server != nil {
		return
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(s.options.Host, strconv.Itoa(config.Port)))
	s.mu.Lock()
	failedBefore := s.status.PortFailed && s.status.Port == config.Port
	s.status.PortFailed, s.status.PortError = err != nil, ""
	if err != nil {
		s.status.PortError = err.Error()
	}
	s.mu.Unlock()
	if err != nil {
		if !failedBefore {
			s.options.Logger.Warn("HTTPS cannot listen on its port: HTTP keeps working", "port", config.Port, "error", err)
		}
		return
	}
	server := &http.Server{
		Handler: s.handler,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: s.getCertificate,
			// The ACME server's TLS-ALPN-01 checks ask for acme-tls/1.
			NextProtos: []string{"h2", "http/1.1", acme.ALPNProto},
		},
		ReadHeaderTimeout: s.options.ReadHeaderTimeout,
		// Clients on the internet that fail their handshakes are many,
		// and none of Polyfin's concern.
		ErrorLog: slog.NewLogLogger(s.options.Logger.Handler(), slog.LevelDebug),
	}
	s.server, s.listener, s.port = server, listener, config.Port
	s.mu.Lock()
	s.status.Address = listener.Addr().String()
	s.mu.Unlock()
	go func() {
		if err := server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			s.options.Logger.Warn("HTTPS stopped serving", "port", config.Port, "error", err)
		}
	}()
	s.options.Logger.Info("HTTPS is served", "address", listener.Addr().String())
}

// stopListening closes the port listened on, at once, and lets its
// connections end for shutdownWait.
func (s *Service) stopListening() {
	if s.server == nil {
		return
	}
	server := s.server
	_ = s.listener.Close()
	s.server, s.listener, s.port = nil, nil, 0
	s.mu.Lock()
	s.status.Address = ""
	s.mu.Unlock()
	s.serving.Store(nil)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
	}
}

// report updates the status, and what the redirect reads.
func (s *Service) report(config Config) {
	certificate := s.certificate.Load()
	s.mu.Lock()
	s.status.Mode, s.status.Port = config.Mode, config.Port
	s.status.Serving, s.status.Names, s.status.NotBefore, s.status.NotAfter = false, nil, time.Time{}, time.Time{}
	if certificate != nil && certificate.Leaf != nil {
		s.status.Serving = true
		s.status.Names = slices.Clone(certificate.Leaf.DNSNames)
		s.status.NotBefore, s.status.NotAfter = certificate.Leaf.NotBefore, certificate.Leaf.NotAfter
	}
	if config.Mode != ModeLetsEncrypt {
		s.status.Renewal, s.status.Failed = "", time.Time{}
	}
	s.mu.Unlock()
	if s.server != nil && certificate != nil {
		s.serving.Store(&config)
	} else {
		s.serving.Store(nil)
	}
}

// Status returns how HTTPS stands.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	status.Names = slices.Clone(status.Names)
	return status
}

// errNoCertificate refuses the handshakes made while no certificate is
// served.
var errNoCertificate = errors.New("no certificate")

// getCertificate gives the certificate served, or, to the ACME server's
// TLS-ALPN-01 checks, the challenge's for the name they ask for.
func (s *Service) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == acme.ALPNProto {
		s.mu.Lock()
		certificate := s.alpn[hello.ServerName]
		s.mu.Unlock()
		if certificate == nil {
			return nil, errNoCertificate
		}
		return certificate, nil
	}
	if certificate := s.certificate.Load(); certificate != nil {
		return certificate, nil
	}
	return nil, errNoCertificate
}

// poke has the loop apply the settings again soon.
func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
