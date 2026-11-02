-- How the files of a show, in a shows folder, are numbered, as an
-- administrator chose it: 'per_season', 'continuing' from season 1,
-- 'absolute', or 'order', in the TMDB episode group episode_group. A
-- show without a row is numbered as its catalog: its files' numbers are
-- the catalog's.
CREATE TABLE local_show_numbering (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    stremio_id text NOT NULL CHECK (stremio_id <> ''),
    numbering text NOT NULL CHECK (numbering IN ('per_season', 'continuing', 'absolute', 'order')),
    episode_group text NOT NULL DEFAULT '',
    CHECK ((numbering = 'order') = (episode_group <> '')),
    PRIMARY KEY (addon_id, stremio_id)
);

-- The season and episodes a show's file tells, by its name or as an
-- administrator gave them, apart from its season, episode and
-- last_episode, the catalog's episodes it is a version of once its
-- show's numbering maps it; NULL for a file named by an absolute number
-- stored before, whose season its path tells. unmapped tells why the
-- show's numbering could not place the file, which is then an episode of
-- its own ('mapped' and 'unmapped' being its numbering), empty otherwise.
ALTER TABLE local_files
    ADD COLUMN file_season integer,
    ADD COLUMN file_episode integer,
    ADD COLUMN file_last_episode integer,
    ADD COLUMN unmapped text NOT NULL DEFAULT '';

UPDATE local_files SET file_season = season, file_episode = episode, file_last_episode = last_episode
    WHERE numbering = '' AND season IS NOT NULL;
