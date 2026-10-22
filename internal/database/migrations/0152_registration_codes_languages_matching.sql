-- Codes a visitor of /register types before registering, when the
-- settings ask for one; the server's default audio and subtitle languages,
-- for users who chose none; and exact title matching of releases.
CREATE TABLE registration_codes (
	code text PRIMARY KEY,
	created_at timestamptz NOT NULL DEFAULT now(),
	used_at timestamptz,
	used_by uuid REFERENCES users (id) ON DELETE SET NULL
);
ALTER TABLE settings
	ADD COLUMN registration_codes boolean NOT NULL DEFAULT false,
	ADD COLUMN default_audio_language text NOT NULL DEFAULT '',
	ADD COLUMN default_subtitle_language text NOT NULL DEFAULT '',
	ADD COLUMN exact_title_match boolean NOT NULL DEFAULT true;
