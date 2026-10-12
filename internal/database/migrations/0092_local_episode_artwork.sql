-- The image of an episode a scan found beside its file in a shows folder:
-- file is the path of the episode's file in local_files, whose unit the
-- row has; its image is the Primary one of each episode the file holds.
-- Null for the images of units and seasons.
ALTER TABLE local_artwork
    ADD COLUMN file text CHECK (file <> ''),
    ADD CHECK (file IS NULL OR season IS NULL AND image_type = 'Primary'),
    DROP CONSTRAINT local_artwork_addon_id_unit_season_image_type_key,
    ADD UNIQUE NULLS NOT DISTINCT (addon_id, unit, season, image_type, file);
