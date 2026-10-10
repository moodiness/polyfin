-- Where HDR is tone mapped to SDR: on a GPU that can (NVIDIA's, and
-- Intel's through VAAPI), unless gpu_tone_mapping is off, and otherwise
-- on the processor, up to processor_tone_mapping_height, 0 letting Polyfin
-- choose from a timing at startup.
ALTER TABLE settings
    ADD COLUMN gpu_tone_mapping boolean NOT NULL DEFAULT true,
    ADD COLUMN processor_tone_mapping_height integer NOT NULL DEFAULT 0
        CHECK (processor_tone_mapping_height IN (0, 720, 1080, 1440, 2160));
