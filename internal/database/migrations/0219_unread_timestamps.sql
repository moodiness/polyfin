-- Timestamps only their default ever wrote and nothing read. A torrent's
-- subtitle list is kept for good, as the files of a torrent never change,
-- so listed_at was never meant to make it expire.
ALTER TABLE live_series_episodes DROP COLUMN recorded_at;
ALTER TABLE recording_commercial_reads DROP COLUMN read_at;
ALTER TABLE seen_devices DROP COLUMN first_seen_at;
ALTER TABLE subtitle_syncs DROP COLUMN synced_at;
ALTER TABLE torrent_subtitle_lists DROP COLUMN listed_at;
ALTER TABLE version_speech DROP COLUMN found_at;
ALTER TABLE server_identity DROP COLUMN created_at;
