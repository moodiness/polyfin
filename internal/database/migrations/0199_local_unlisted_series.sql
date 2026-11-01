-- The episodes a metadata addon's description of a series lists, by
-- season and number, as the series was last described, kept for the
-- series whose local files hold episodes it does not list (see
-- local_unlisted_episodes): the folders' pages show the description's
-- numbers next to the files'.
CREATE TABLE local_unlisted_series (
    stremio_id text PRIMARY KEY,
    seasons integer[] NOT NULL,
    episodes integer[] NOT NULL
);
