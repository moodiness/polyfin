-- The certificate Polyfin obtained through ACME keeps the challenge it was
-- obtained through, so that choosing another obtains a new one at once,
-- and its directory is the ACME server it came from, which a failed
-- attempt at another leaves unchanged. A certificate kept already was
-- obtained through the challenge the settings choose.
ALTER TABLE acme_certificate
    ADD COLUMN challenge text NOT NULL DEFAULT '';

UPDATE acme_certificate SET challenge = coalesce((SELECT acme_challenge FROM settings), 'http-01') WHERE certificate <> '';
