-- Send apps to the source: every app that plays a version as it is may be
-- sent to its source, whose address is then told to the user. Off by
-- default: versions of IPTV sources are relayed, and those of the
-- server's addons are to users who are not administrators.
ALTER TABLE settings ADD COLUMN send_apps_to_source boolean NOT NULL DEFAULT false;
