-- An account is linked to a person at the identity provider from its own
-- session, or, while an administrator allows it, at the next sign-in
-- through the provider of the person whose claim or verified email is the
-- account's name. provider_link_allowed is that permission: off until an
-- administrator gives it, and off again once the link is made.
ALTER TABLE users
    ADD COLUMN provider_link_allowed boolean NOT NULL DEFAULT false;
