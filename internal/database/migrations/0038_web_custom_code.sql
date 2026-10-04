-- What jellyfin-web shows of the server's own: Jellyfin's branding, the
-- CSS it applies to every page and the disclaimer under its sign-in form,
-- and a script of the administrator's, which Polyfin adds to its page. All
-- empty by default, which changes nothing. Sizes are in bytes.
ALTER TABLE settings
    ADD COLUMN custom_css text NOT NULL DEFAULT '' CHECK (octet_length(custom_css) <= 262144),
    ADD COLUMN custom_js text NOT NULL DEFAULT '' CHECK (octet_length(custom_js) <= 262144),
    ADD COLUMN login_disclaimer text NOT NULL DEFAULT '' CHECK (octet_length(login_disclaimer) <= 8192);
