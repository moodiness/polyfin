-- Invite links: each lets up to max_uses guests create a member account
-- for themselves, until expires_at (never when null), or until revoked. The
-- link's token is the invite's id followed by a random secret, which is
-- kept only as its SHA-256 hash. A new account copies the settings of
-- model_user_id when set; deleting the model revokes the invite.
CREATE TABLE invites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    secret_hash bytea NOT NULL CHECK (length(secret_hash) = 32),
    max_uses integer NOT NULL CHECK (max_uses BETWEEN 1 AND 100),
    uses integer NOT NULL DEFAULT 0 CHECK (uses BETWEEN 0 AND max_uses),
    expires_at timestamptz,
    model_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE INDEX invites_model_user ON invites (model_user_id) WHERE model_user_id IS NOT NULL;
