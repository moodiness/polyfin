-- Sign-in with an OpenID Connect provider. The provider is its issuer,
-- the name its button shows, and Polyfin's client at it, whose secret is
-- sealed with POLYFIN_SECRET_KEY as the other stored secrets; it is used
-- once the issuer and the client ID are set. provider_user_claim names
-- the claim compared with account names, the verified email coming after
-- it; provider_create_accounts creates an account for someone no account
-- matches; provider_admin_claim and provider_admin_value, both set, make
-- the people whose claim holds the value administrators.
ALTER TABLE settings
    ADD COLUMN provider_issuer text NOT NULL DEFAULT '' CHECK (octet_length(provider_issuer) <= 500),
    ADD COLUMN provider_name text NOT NULL DEFAULT '' CHECK (octet_length(provider_name) <= 64),
    ADD COLUMN provider_client_id text NOT NULL DEFAULT '' CHECK (octet_length(provider_client_id) <= 255),
    ADD COLUMN provider_client_secret text NOT NULL DEFAULT '',
    ADD COLUMN provider_user_claim text NOT NULL DEFAULT 'preferred_username'
        CHECK (provider_user_claim <> '' AND octet_length(provider_user_claim) <= 64),
    ADD COLUMN provider_create_accounts boolean NOT NULL DEFAULT false,
    ADD COLUMN provider_admin_claim text NOT NULL DEFAULT '' CHECK (octet_length(provider_admin_claim) <= 64),
    ADD COLUMN provider_admin_value text NOT NULL DEFAULT '' CHECK (octet_length(provider_admin_value) <= 255);

-- An account is linked to the person the provider signed in first as it,
-- by the provider's issuer and subject, both set or neither, a person
-- linked to one account at most. provider_only refuses its password: it
-- signs in through the provider alone.
ALTER TABLE users
    ADD COLUMN provider_issuer text,
    ADD COLUMN provider_subject text,
    ADD COLUMN provider_only boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT users_provider_check CHECK ((provider_issuer IS NULL) = (provider_subject IS NULL));
CREATE UNIQUE INDEX users_provider_idx ON users (provider_issuer, provider_subject) WHERE provider_subject IS NOT NULL;

-- A device signed in through the provider, whose two-step sign-in the
-- provider asked for, opens the admin app without a code.
ALTER TABLE devices
    ADD COLUMN provider boolean NOT NULL DEFAULT false;
