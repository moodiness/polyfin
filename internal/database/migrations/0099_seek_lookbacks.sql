-- How far before a segment's start FFmpeg starts reading a version it
-- seeks in by searching its timestamps, such as an MPEG-TS file, so as to
-- land on a keyframe before the segment, in milliseconds: learned by a
-- playback that landed past the segment, and kept for the next ones. It is
-- kept by what Polyfin knows the file by: kind 'local', a local folder's
-- file by its folder and path, file being its size and modification time
-- when it was learned; 'channel', an IPTV catch-up programme by its
-- channel; 'version', any other version by its identifier.
CREATE TABLE seek_lookbacks (
    kind text NOT NULL CHECK (kind IN ('local', 'channel', 'version')),
    name text NOT NULL CHECK (name <> ''),
    file text NOT NULL DEFAULT '',
    lookback_ms integer NOT NULL CHECK (lookback_ms > 0),
    learned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, name)
);
