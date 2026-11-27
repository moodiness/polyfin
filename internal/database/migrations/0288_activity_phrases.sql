-- The phrase that names an entry of the activity log, and the one of its
-- short overview, as the key of a text in package activity and the words
-- that fill it, so that each reader reads the entry in their own
-- language: {"key": "signedIn", "args": [{"text": "Sam"}]}. name and
-- short_overview are still written, in the server language; entries
-- written before keep only them, and are read as they were written.
ALTER TABLE activity_log
    ADD COLUMN name_phrase jsonb CHECK (jsonb_typeof(name_phrase) = 'object'),
    ADD COLUMN short_overview_phrase jsonb CHECK (jsonb_typeof(short_overview_phrase) = 'object');
