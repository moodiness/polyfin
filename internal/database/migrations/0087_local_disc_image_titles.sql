-- The other movies a disc image holds: their titles, a DVD's titles or a
-- Blu-ray disc's playlists, besides its main one. What scans read of the
-- images before is forgotten, so that the next scan reads each image
-- again: its other movies, and the playlists of images with a UDF file
-- system only, which were left to libbluray.
ALTER TABLE local_disc_images ADD COLUMN titles integer[] NOT NULL DEFAULT '{}';
DELETE FROM local_disc_images;
