-- Subtitle files beside local videos may be pictures too: PGS (.sup) files,
-- and VobSub pairs, stored by their .idx, whose .sub is pair, beside it.
-- A pair's size is both files' and its modification time the later one.
ALTER TABLE local_subtitles
    DROP CONSTRAINT local_subtitles_format_check,
    ADD CONSTRAINT local_subtitles_format_check CHECK (format IN ('srt', 'ass', 'ssa', 'vtt', 'sup', 'idx')),
    ADD COLUMN pair text NOT NULL DEFAULT '',
    ADD CONSTRAINT local_subtitles_pair_check CHECK ((format = 'idx') = (pair <> ''));
