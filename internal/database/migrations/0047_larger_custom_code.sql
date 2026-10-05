-- The web client's custom CSS and script may take up to 2 MiB each, enough
-- for a whole theme pasted in. Sizes are in bytes.
ALTER TABLE settings
    DROP CONSTRAINT settings_custom_css_check,
    DROP CONSTRAINT settings_custom_js_check,
    ADD CONSTRAINT settings_custom_css_check CHECK (octet_length(custom_css) <= 2097152),
    ADD CONSTRAINT settings_custom_js_check CHECK (octet_length(custom_js) <= 2097152);
