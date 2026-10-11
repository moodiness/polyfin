-- The disc images a scan read in a local folder: what each is, kept until
-- it changes. path is the image's, in the folder, with slashes; size and
-- modified tell whether it changed since it was read. kind is the disc
-- it is an image of, "dvd" or "bluray", empty for no disc's; title is its
-- main title, a DVD's title or a Blu-ray disc's playlist, 0 for FFmpeg's
-- first title and -1 for libbluray's choice.
CREATE TABLE local_disc_images (
    addon_id uuid NOT NULL REFERENCES local_folders ON DELETE CASCADE,
    path text NOT NULL CHECK (path <> ''),
    size bigint NOT NULL CHECK (size >= 0),
    modified timestamptz NOT NULL,
    kind text NOT NULL CHECK (kind IN ('', 'dvd', 'bluray')),
    title integer NOT NULL CHECK (title >= -1),
    PRIMARY KEY (addon_id, path)
);
