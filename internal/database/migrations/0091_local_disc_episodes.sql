-- The episodes administrators gave the titles of shows' discs in a shows
-- folder: a Blu-ray disc's playlist, a DVD's title or a disc image's, by
-- the path its entry has in local_files, kept through every scan, even
-- while the disc is away.
CREATE TABLE local_disc_episodes (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    season integer NOT NULL CHECK (season >= 0),
    episode integer NOT NULL CHECK (episode >= 0),
    last_episode integer NOT NULL CHECK (last_episode >= episode),
    PRIMARY KEY (addon_id, path)
);

-- How long a disc's title plays, in seconds, as the disc's tables tell it;
-- null for other files, and for the titles of movies' discs stored before.
ALTER TABLE local_files ADD COLUMN length integer CHECK (length >= 0);

-- A title of a show's disc without its episodes is unmatched, its reason
-- disc_episode, until an administrator gives them or the disc's name does.
ALTER TABLE local_files DROP CONSTRAINT local_files_reason_check;
ALTER TABLE local_files ADD CONSTRAINT local_files_reason_check
    CHECK (reason IN ('', 'unreadable_name', 'not_found', 'ambiguous', 'other_year', 'search_failed', 'disc_title', 'disc_episode'));

-- How long each title of a disc image plays, in seconds, its main title's
-- first, then its other titles' in their order; empty for the images read
-- before.
ALTER TABLE local_disc_images ADD COLUMN lengths integer[] NOT NULL DEFAULT '{}';
