-- The episodes local files hold that a metadata addon's description of
-- their series does not list, by season and number, as the series were
-- last described: the folders' pages count them. Their episodes are listed
-- all the same, named after their files.
CREATE TABLE local_unlisted_episodes (
    stremio_id text NOT NULL,
    season integer NOT NULL,
    episode integer NOT NULL,
    PRIMARY KEY (stremio_id, season, episode)
);

-- Whether a metadata addon of the server described the title a unit was
-- linked to, as the folder's kind, when its files were last matched to it:
-- a link to an identifier none describes, such as an episode's typed for
-- its series', is flagged on the folder's page.
ALTER TABLE local_links
    ADD COLUMN described boolean NOT NULL DEFAULT true;
