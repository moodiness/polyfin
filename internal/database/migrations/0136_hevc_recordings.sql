-- Finished recordings can be converted to HEVC, one at a time, between two
-- hours of the night, to take less space. Off by default, as is converting
-- them on the processor when the server has no graphics card.
ALTER TABLE settings
    ADD COLUMN convert_recordings boolean NOT NULL DEFAULT false,
    ADD COLUMN convert_recordings_from integer NOT NULL DEFAULT 1 CHECK (convert_recordings_from BETWEEN 0 AND 23),
    ADD COLUMN convert_recordings_until integer NOT NULL DEFAULT 6 CHECK (convert_recordings_until BETWEEN 0 AND 23),
    ADD COLUMN convert_recordings_on_processor boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT convert_recordings_window CHECK (convert_recordings_from <> convert_recordings_until);

-- Where a recording is in its conversion to HEVC: not tried yet (''), its
-- copy checked and waiting for its playback to stop (pending), replaced by
-- its copy (converted), or kept as it was (kept), never tried again.
-- original_size is the size of the file a converted recording replaced.
ALTER TABLE live_recordings
    ADD COLUMN hevc text NOT NULL DEFAULT '' CHECK (hevc IN ('', 'pending', 'converted', 'kept')),
    ADD COLUMN original_size bigint;
