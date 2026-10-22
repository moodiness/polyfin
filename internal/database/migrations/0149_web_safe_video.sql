-- The web player (jellyfin-web, in a browser) is offered no HEVC, Dolby
-- Vision or AV1 video, which browsers claim and then often show black:
-- such versions play converted. On by default.
ALTER TABLE settings ADD COLUMN web_safe_video boolean NOT NULL DEFAULT true;
