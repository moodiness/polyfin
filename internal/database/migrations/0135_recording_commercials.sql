-- Whether the commercial breaks of finished Live TV recordings are looked
-- for. Off by default: off, recordings are not read.
ALTER TABLE settings
    ADD COLUMN recording_commercials boolean NOT NULL DEFAULT false;

-- The recordings read for commercial breaks, those where none was found
-- included, by the recording's identifier: they are kept with it when its
-- file is replaced, and deleted with it.
CREATE TABLE recording_commercial_reads (
    recording_id uuid PRIMARY KEY REFERENCES live_recordings (id) ON DELETE CASCADE,
    read_at timestamptz NOT NULL DEFAULT now()
);

-- The commercial breaks found in a recording, from start_ms to end_ms of
-- its file.
CREATE TABLE recording_commercials (
    recording_id uuid NOT NULL REFERENCES recording_commercial_reads (recording_id) ON DELETE CASCADE,
    start_ms bigint NOT NULL CHECK (start_ms >= 0),
    end_ms bigint NOT NULL,
    CHECK (start_ms < end_ms),
    PRIMARY KEY (recording_id, start_ms)
);
