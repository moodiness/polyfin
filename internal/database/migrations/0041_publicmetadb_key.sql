-- The PublicMetaDB API key the server asks for skip segments with, empty
-- while none is saved, which leaves PublicMetaDB unasked. Keys are printable
-- ASCII; the size is in bytes.
ALTER TABLE settings
    ADD COLUMN publicmetadb_key text NOT NULL DEFAULT '' CHECK (octet_length(publicmetadb_key) <= 256 AND publicmetadb_key ~ '^[ -~]*$');
