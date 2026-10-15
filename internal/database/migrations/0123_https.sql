-- Serving HTTPS: https_mode is off, files (the certificate and private
-- key files at https_certificate and https_private_key, paths in the
-- container) or letsencrypt (a certificate Polyfin obtains through ACME
-- for acme_hosts, with acme_email as the account's contact, once the
-- administrator agrees to the terms, through the challenge
-- acme_challenge); https_port is the TCP port HTTPS listens on, HTTP
-- keeping POLYFIN_LISTEN. https_redirect sends the requests of clients on
-- the internet that come over HTTP to HTTPS.
ALTER TABLE settings
    ADD COLUMN https_mode text NOT NULL DEFAULT 'off' CHECK (https_mode IN ('off', 'files', 'letsencrypt')),
    ADD COLUMN https_port integer NOT NULL DEFAULT 8920 CHECK (https_port BETWEEN 1 AND 65535),
    ADD COLUMN https_redirect boolean NOT NULL DEFAULT false,
    ADD COLUMN https_certificate text NOT NULL DEFAULT '',
    ADD COLUMN https_private_key text NOT NULL DEFAULT '',
    ADD COLUMN acme_hosts text[] NOT NULL DEFAULT '{}',
    ADD COLUMN acme_email text NOT NULL DEFAULT '',
    ADD COLUMN acme_terms boolean NOT NULL DEFAULT false,
    ADD COLUMN acme_challenge text NOT NULL DEFAULT 'http-01' CHECK (acme_challenge IN ('http-01', 'tls-alpn-01'));

-- The certificate Polyfin obtained through ACME, in a single row: the
-- account key it was obtained with, sealed with POLYFIN_SECRET_KEY as the
-- other secrets, the ACME directory the account is registered at, the
-- host names the certificate holds, the certificate with its chain (PEM)
-- and its private key (PEM, sealed). failure is the last renewal's error,
-- empty once one succeeds, at failed_at.
CREATE TABLE acme_certificate (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    account_key text NOT NULL DEFAULT '',
    directory text NOT NULL DEFAULT '',
    hosts text[] NOT NULL DEFAULT '{}',
    certificate text NOT NULL DEFAULT '',
    private_key text NOT NULL DEFAULT '',
    failure text NOT NULL DEFAULT '',
    failed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
