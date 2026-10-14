-- The rules ordering and hiding a title's versions: up to four criteria,
-- each grouping the versions and the next sorting within them (none keeps
-- the addons' order), the order of the video codecs for the codec
-- criterion, the size in GB above which versions are hidden (0 for no
-- limit), the video codecs hidden, and the words, matched without case in
-- a stream's labels, that hide it.
ALTER TABLE settings
    ADD COLUMN version_order text[] NOT NULL DEFAULT '{}'
        CHECK (cardinality(version_order) <= 4
            AND version_order <@ '{language,resolution-high,resolution-low,size-large,size-small,codec,hdr-first,hdr-last}'),
    ADD COLUMN version_codec_order text[] NOT NULL DEFAULT '{av1,hevc,vp9,h264,vc1,mpeg4,mpeg2video}'
        CHECK (version_codec_order <@ '{av1,hevc,vp9,h264,vc1,mpeg4,mpeg2video}'),
    ADD COLUMN hide_versions_larger_gb integer NOT NULL DEFAULT 0 CHECK (hide_versions_larger_gb BETWEEN 0 AND 1000),
    ADD COLUMN hidden_version_codecs text[] NOT NULL DEFAULT '{}'
        CHECK (hidden_version_codecs <@ '{av1,hevc,vp9,h264,vc1,mpeg4,mpeg2video}'),
    ADD COLUMN hidden_version_words text[] NOT NULL DEFAULT '{}' CHECK (cardinality(hidden_version_words) <= 50);
