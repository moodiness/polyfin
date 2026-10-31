-- The version an administrator pinned for a movie or an episode, which
-- every user's details and PlaybackInfo list first while its addon lists
-- it: version_id names the file of that title (see library.versionID).
-- name, file_name and size tell it as it was listed, for the Versions page
-- to show it once no addon lists it. A title is not referenced: movies and
-- episodes of addons are not all stored.
CREATE TABLE version_pins (
    item_id uuid PRIMARY KEY,
    version_id uuid NOT NULL,
    name text NOT NULL CHECK (octet_length(name) <= 1000),
    file_name text NOT NULL DEFAULT '' CHECK (octet_length(file_name) <= 1000),
    size bigint NOT NULL DEFAULT 0 CHECK (size >= 0),
    pinned_by uuid REFERENCES users (id) ON DELETE SET NULL,
    pinned_at timestamptz NOT NULL DEFAULT now()
);

-- The files administrators blocked: no version of them is listed, for the
-- title item_id names, or for every title when it is null. file is a hash
-- of the file's identity (see library.fileKey): its name and size, else
-- its link, which the hash keeps out of the database. name, file_name and
-- size tell it as it was listed, and reason why it was blocked.
CREATE TABLE version_blocks (
    id uuid PRIMARY KEY,
    item_id uuid,
    file text NOT NULL CHECK (octet_length(file) = 64),
    name text NOT NULL CHECK (octet_length(name) <= 1000),
    file_name text NOT NULL DEFAULT '' CHECK (octet_length(file_name) <= 1000),
    size bigint NOT NULL DEFAULT 0 CHECK (size >= 0),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 200),
    blocked_by uuid REFERENCES users (id) ON DELETE SET NULL,
    blocked_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (item_id, file)
);
