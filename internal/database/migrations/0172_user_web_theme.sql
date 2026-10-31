-- Each user's web player theme: one of the server's themes, as
-- settings.web_theme names them, empty for the server's own;
-- web_theme_locked keeps the user from changing it themselves, the
-- administrator having chosen it.
ALTER TABLE users
    ADD COLUMN web_theme text NOT NULL DEFAULT '' CHECK (web_theme IN ('', 'lumaaglaass', 'none', 'custom')),
    ADD COLUMN web_theme_locked boolean NOT NULL DEFAULT false;
