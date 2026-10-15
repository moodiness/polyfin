package https

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

// retryWait is how long after a failed attempt with Let's Encrypt the
// next one waits, unless the settings change what is asked: Let's Encrypt
// limits failed checks to a few an hour.
const retryWait = time.Hour

// issueTimeout bounds an attempt with Let's Encrypt.
const issueTimeout = 10 * time.Minute

// maxFailure is the longest error of an attempt kept, in bytes.
const maxFailure = 1000

// acmeState is how the certificate of Let's Encrypt stands: what Store
// holds, read once, whether an attempt is under way, and when the next
// one may start for the settings last tried.
type acmeState struct {
	loaded      bool
	stored      Stored
	issuing     bool
	attempted   string
	nextAttempt time.Time
}

// renewAt returns when a certificate is renewed: once a third of its
// lifetime remains.
func renewAt(leaf *x509.Certificate) time.Time {
	return leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) * 2 / 3)
}

// attemptKey tells apart what an attempt asks for: another key starts an
// attempt without waiting for retryWait.
func attemptKey(config Config, directory string) string {
	return strings.Join([]string{directory, config.Challenge, config.Email, strings.Join(config.Hosts, ",")}, "\n")
}

// renew serves the certificate of Let's Encrypt kept, and starts obtaining
// one when there is none, when it is for other names, from another ACME
// server or through another challenge, or once a third of its lifetime
// remains. The attempt runs in the background, and pokes the loop when it
// ends.
func (s *Service) renew(ctx context.Context, config Config) {
	s.loadStored(ctx, config)
	now := s.options.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := &s.acme
	certificate := state.stored.Certificate
	if certificate != nil {
		s.certificate.Store(certificate)
	}
	due := certificate == nil || !slices.Equal(state.stored.Hosts, config.Hosts) || state.stored.Directory != s.options.Directory ||
		state.stored.Challenge != config.Challenge || !now.Before(renewAt(certificate.Leaf))
	key := attemptKey(config, s.options.Directory)
	if !due || state.issuing || key == state.attempted && now.Before(state.nextAttempt) {
		return
	}
	state.issuing, state.attempted = true, key
	go s.issue(ctx, config, state.stored)
}

// loadStored reads what Store keeps, once.
func (s *Service) loadStored(ctx context.Context, config Config) {
	s.mu.Lock()
	loaded := s.acme.loaded
	s.mu.Unlock()
	if loaded {
		return
	}
	var stored Stored
	if s.options.Store != nil {
		var err error
		if stored, err = s.options.Store.Load(ctx); err != nil {
			s.options.Logger.Warn("The certificate of Let's Encrypt kept could not be read", "error", err)
			stored = Stored{}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acme.loaded, s.acme.stored = true, stored
	if stored.Failure != "" {
		// A restart waits as long as the attempt that failed would have.
		s.status.Renewal, s.status.Failed = stored.Failure, stored.FailedAt
		s.acme.attempted, s.acme.nextAttempt = attemptKey(config, s.options.Directory), stored.FailedAt.Add(retryWait)
	}
}

// issue obtains a certificate for config's host names, keeps it, and has
// the loop serve it; a failure is kept, and told in Status. The ACME
// directory and the challenge are kept with the certificate obtained only:
// after a failed attempt, the certificate it had is still due, and tried
// again after retryWait. An attempt canceled by Polyfin stopping did not
// fail: it keeps and tells nothing, and is made again at the next start.
func (s *Service) issue(ctx context.Context, config Config, stored Stored) {
	attemptCtx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()
	accountKey, certificate, err := s.obtain(attemptCtx, config, stored.AccountKey)
	if err != nil && ctx.Err() != nil {
		s.mu.Lock()
		s.acme.issuing = false
		s.mu.Unlock()
		return
	}
	now := s.options.now()
	stored.AccountKey = accountKey
	if err != nil {
		failure := err.Error()
		if len(failure) > maxFailure {
			failure = failure[:maxFailure]
		}
		stored.Failure, stored.FailedAt = failure, now
		s.options.Logger.Warn("A certificate could not be obtained from Let's Encrypt", "hosts", config.Hosts, "error", failure)
	} else {
		stored.Directory, stored.Challenge, stored.Hosts, stored.Certificate = s.options.Directory, config.Challenge, config.Hosts, certificate
		stored.Failure, stored.FailedAt = "", time.Time{}
		s.options.Logger.Info("A certificate was obtained from Let's Encrypt", "hosts", config.Hosts,
			"expires", certificate.Leaf.NotAfter.Format(time.RFC3339))
	}
	if s.options.Store != nil && accountKey != nil {
		saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		if err := s.options.Store.Save(saveCtx, stored); err != nil {
			s.options.Logger.Warn("The certificate of Let's Encrypt could not be kept in the database", "error", err)
		}
		cancelSave()
	}
	s.mu.Lock()
	s.acme.issuing, s.acme.stored = false, stored
	s.status.Renewal, s.status.Failed = stored.Failure, stored.FailedAt
	if err != nil {
		s.acme.nextAttempt = now.Add(retryWait)
	} else {
		s.acme.nextAttempt = time.Time{}
	}
	s.mu.Unlock()
	s.poke()
}

// obtain registers the account of accountKey, made anew when nil, then
// orders a certificate for config's host names, answering config's
// challenge for each, with a new ECDSA key. It returns the account key
// even when the order fails, to be used again.
func (s *Service) obtain(ctx context.Context, config Config, accountKey crypto.Signer) (crypto.Signer, *tls.Certificate, error) {
	if accountKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		accountKey = key
	}
	client := &acme.Client{Key: accountKey, DirectoryURL: s.options.Directory, HTTPClient: s.options.Client, UserAgent: "Polyfin"}
	account := &acme.Account{}
	if config.Email != "" {
		account.Contact = []string{"mailto:" + config.Email}
	}
	if _, err := client.Register(ctx, account, acme.AcceptTOS); errors.Is(err, acme.ErrAccountAlreadyExists) {
		if len(account.Contact) > 0 {
			if _, err := client.UpdateReg(ctx, account); err != nil {
				s.options.Logger.Debug("The email of the Let's Encrypt account could not be updated", "error", err)
			}
		}
	} else if err != nil {
		return accountKey, nil, fmt.Errorf("register the account: %w", err)
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(config.Hosts...))
	if err != nil {
		return accountKey, nil, fmt.Errorf("order the certificate: %w", err)
	}
	for _, url := range order.AuthzURLs {
		if err := s.authorize(ctx, client, url, config.Challenge); err != nil {
			return accountKey, nil, err
		}
	}
	if _, err := client.WaitOrder(ctx, order.URI); err != nil {
		return accountKey, nil, fmt.Errorf("order the certificate: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return accountKey, nil, err
	}
	request, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: config.Hosts[0]},
		DNSNames: config.Hosts}, key)
	if err != nil {
		return accountKey, nil, err
	}
	chain, _, err := client.CreateCertFromOrder(ctx, order, request, true)
	if err != nil {
		return accountKey, nil, fmt.Errorf("finalize the order: %w", err)
	}
	certificate, err := checkedCertificate(chain, key, config.Hosts)
	if err != nil {
		return accountKey, nil, err
	}
	return accountKey, certificate, nil
}

// authorize answers challenge for the authorization at url, unless it is
// valid already, and waits until the ACME server checked it.
func (s *Service) authorize(ctx context.Context, client *acme.Client, url, challengeType string) error {
	authorization, err := client.GetAuthorization(ctx, url)
	if err != nil {
		return fmt.Errorf("read an authorization: %w", err)
	}
	if authorization.Status == acme.StatusValid {
		return nil
	}
	host := authorization.Identifier.Value
	var challenge *acme.Challenge
	for _, offered := range authorization.Challenges {
		if offered.Type == challengeType {
			challenge = offered
			break
		}
	}
	if challenge == nil {
		return fmt.Errorf("%s: the ACME server does not offer the %s challenge", host, challengeType)
	}
	switch challengeType {
	case ChallengeHTTP01:
		response, err := client.HTTP01ChallengeResponse(challenge.Token)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.tokens[challenge.Token] = response
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.tokens, challenge.Token)
			s.mu.Unlock()
		}()
	case ChallengeTLSALPN01:
		certificate, err := client.TLSALPN01ChallengeCert(challenge.Token, host)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.alpn[host] = &certificate
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.alpn, host)
			s.mu.Unlock()
		}()
	}
	if _, err := client.Accept(ctx, challenge); err != nil {
		return fmt.Errorf("%s: %w", host, err)
	}
	// The ACME server's error names the host.
	if _, err := client.WaitAuthorization(ctx, authorization.URI); err != nil {
		return err
	}
	return nil
}

// checkedCertificate makes the certificate the ACME server issued, with
// its chain, once it holds key's public key and every host name.
func checkedCertificate(chain [][]byte, key crypto.Signer, hosts []string) (*tls.Certificate, error) {
	if len(chain) == 0 {
		return nil, errors.New("the ACME server issued no certificate")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, fmt.Errorf("the ACME server issued a certificate Polyfin cannot read: %w", err)
	}
	if public, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool }); !ok || !public.Equal(key.Public()) {
		return nil, errors.New("the ACME server issued a certificate for another key")
	}
	for _, host := range hosts {
		if err := leaf.VerifyHostname(host); err != nil {
			return nil, fmt.Errorf("the ACME server issued a certificate without %s", host)
		}
	}
	return &tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leaf}, nil
}

// answerChallenge answers the ACME server's HTTP-01 check of a token under
// way, and reports whether it did.
func (s *Service) answerChallenge(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.tokens[token]
	return response, ok
}
