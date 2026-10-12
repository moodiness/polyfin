-- A home video's identifier is the stremio_id of its local_files row,
-- kept as it is through scans: those of the videos found before are the
-- ones their folder and path gave, and a video found since has one of its
-- own, or the one of the video gone it is (see localfiles.storeVideo).
--
-- fingerprint is a SHA-256 hash of the first and last MiB of a home video,
-- of all of it when smaller, read with size when the video is found or
-- changes (see localfiles.videoFingerprint); null until read.
ALTER TABLE local_videos ADD COLUMN fingerprint bytea;
CREATE INDEX local_videos_contents ON local_videos (size, fingerprint) WHERE fingerprint IS NOT NULL;

-- The home videos gone from every folder of the server, by identifier,
-- with their size and fingerprint, and when their file was first found
-- (local_files.added_at). A video found with the same contents takes the
-- identifier back (see localfiles.recognize); they are forgotten 30 days
-- after gone_at.
CREATE TABLE local_videos_gone (
    video_id text PRIMARY KEY,
    size bigint NOT NULL CHECK (size >= 0),
    fingerprint bytea NOT NULL,
    added_at timestamptz,
    gone_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX local_videos_gone_contents ON local_videos_gone (size, fingerprint);

-- A home video's file is remembered when its row goes, whatever removes
-- it: a scan that no longer finds it, a folder given another path or
-- removed.
CREATE FUNCTION remember_local_video() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO local_videos_gone (video_id, size, fingerprint, added_at)
        SELECT OLD.stremio_id, v.size, v.fingerprint, OLD.added_at FROM local_videos v
        WHERE v.addon_id = OLD.addon_id AND v.path = OLD.path AND v.fingerprint IS NOT NULL
        ON CONFLICT (video_id) DO UPDATE SET size = EXCLUDED.size, fingerprint = EXCLUDED.fingerprint, added_at = EXCLUDED.added_at,
            gone_at = now();
    RETURN OLD;
END
$$;
CREATE TRIGGER local_videos_gone BEFORE DELETE ON local_files
    FOR EACH ROW WHEN (OLD.matched_by = 'own') EXECUTE FUNCTION remember_local_video();
