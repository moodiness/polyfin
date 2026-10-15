-- The language of a server addon's descriptions, as its manifest address
-- configures it, empty for none; and the metadata language of a user, one
-- of those languages, empty for the server's descriptions.
ALTER TABLE addons ADD COLUMN language text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN metadata_language text NOT NULL DEFAULT '';
