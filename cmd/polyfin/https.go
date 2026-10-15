package main

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/https"
	"github.com/moodiness/polyfin/internal/secrets"
)

// acmeTimeout bounds each request to the ACME server.
const acmeTimeout = 30 * time.Second

// newHTTPS returns what serves HTTPS next to HTTP, on the same address,
// as the settings choose, the certificate of Let's Encrypt being kept in
// the database, its keys sealed with box.
func newHTTPS(cfg config.Config, store *accounts.Store, pool *pgxpool.Pool, box *secrets.Box, logger *slog.Logger) *https.Service {
	host, _, _ := net.SplitHostPort(cfg.Listen)
	return https.New(https.Options{
		Host: host,
		Settings: func() https.Config {
			settings := store.Settings()
			return https.Config{Mode: settings.HTTPSMode, Port: settings.HTTPSPort, Redirect: settings.HTTPSRedirect,
				Certificate: settings.HTTPSCertificate, PrivateKey: settings.HTTPSPrivateKey,
				Hosts: settings.ACMEHosts, Email: settings.ACMEEmail, Challenge: settings.ACMEChallenge}
		},
		Changed:           store.SettingsChanged,
		Store:             https.NewDatabaseStore(pool, box),
		Directory:         cfg.ACMEDirectory,
		Client:            acmeClient(cfg.ACMECAFile, logger),
		ReadHeaderTimeout: readHeaderTimeout,
		Logger:            logger,
	})
}

// acmeClient returns the HTTP client that talks to the ACME server,
// trusting the certificate authorities of caFile besides the system's. A
// file that cannot be read is logged, and the system's are trusted alone.
func acmeClient(caFile string, logger *slog.Logger) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		data, err := os.ReadFile(caFile)
		switch {
		case err != nil:
			logger.Warn("POLYFIN_ACME_CA_FILE cannot be read: the system's certificate authorities are trusted alone", "error", err)
		case !roots.AppendCertsFromPEM(data):
			logger.Warn("POLYFIN_ACME_CA_FILE holds no PEM certificate: the system's certificate authorities are trusted alone",
				"file", caFile)
		default:
			transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
		}
	}
	return &http.Client{Transport: transport, Timeout: acmeTimeout}
}
