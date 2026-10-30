-- Each user's taste profile, built in the background from what they did
-- with movies and series (see package taste): profile is null until it is
-- first built, at built_at. taglines tells whether their titles' details
-- say why a title is for them (My account › Taste profile).
CREATE TABLE taste_profiles (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    profile jsonb,
    built_at timestamptz,
    taglines boolean NOT NULL DEFAULT true
);

-- A home row may list the user's top picks (picks).
ALTER TABLE home_rows
    DROP CONSTRAINT home_rows_kind_check,
    ADD CONSTRAINT home_rows_kind_check CHECK (kind IN ('catalog', 'collection', 'because', 'new', 'available', 'waiting', 'trakt', 'picks'));
