-- Whether the data of a language Tesseract lacks is downloaded the first
-- time a picture subtitle track needs it.
ALTER TABLE settings
    ADD COLUMN subtitle_ocr_download boolean NOT NULL DEFAULT false;
