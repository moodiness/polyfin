-- The last result of making a version's images: made (with the thumbnails
-- kept), cannot (its file cannot give them, for reason), failed (worth
-- trying again); kind tells a local folder's file, a link, or a torrent's
-- file. The title's versions page and Health read them across restarts.
CREATE TABLE thumbnail_results (
    version_id uuid PRIMARY KEY,
    item_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('local', 'link', 'torrent')),
    state text NOT NULL CHECK (state IN ('made', 'cannot', 'failed')),
    reason text NOT NULL DEFAULT '' CHECK (reason IN ('', 'no_index', 'unreadable', 'no_video', 'error')),
    thumbnails integer NOT NULL DEFAULT 0 CHECK (thumbnails >= 0),
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX thumbnail_results_item ON thumbnail_results (item_id);
