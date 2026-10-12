-- Whether the intros of local episodes are looked for in their audio,
-- where neither their chapters nor the segment databases give one.
ALTER TABLE settings
    ADD COLUMN audio_intros boolean NOT NULL DEFAULT true;

-- The fingerprints of the audio of local files, by the file's path, as it
-- was at its size and modification time: part is the part of the file
-- they are of, from start_ms; frames are 4 bytes each, little-endian.
CREATE TABLE audio_fingerprints (
    path text NOT NULL,
    part text NOT NULL CHECK (part <> ''),
    size bigint NOT NULL,
    modified timestamptz NOT NULL,
    start_ms bigint NOT NULL CHECK (start_ms >= 0),
    frames bytea NOT NULL,
    PRIMARY KEY (path, part)
);

-- The segments the comparison of a season's files found in each, by the
-- file's path, as it was at its size and modification time: start_ms and
-- end_ms are null when none was found. season tells the files of the
-- season it was compared with (see package audiosegments).
CREATE TABLE audio_segments (
    path text NOT NULL,
    type text NOT NULL CHECK (type <> ''),
    size bigint NOT NULL,
    modified timestamptz NOT NULL,
    season text NOT NULL,
    start_ms bigint,
    end_ms bigint,
    CHECK ((start_ms IS NULL) = (end_ms IS NULL) AND (start_ms IS NULL OR 0 <= start_ms AND start_ms < end_ms)),
    PRIMARY KEY (path, type)
);
