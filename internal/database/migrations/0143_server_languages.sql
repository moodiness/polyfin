-- The server speaks German, Spanish, Italian and Brazilian Portuguese too.
ALTER TABLE settings
    DROP CONSTRAINT settings_language_check,
    ADD CONSTRAINT settings_language_check CHECK (language IN ('en', 'fr', 'de', 'es', 'it', 'pt-BR'));
