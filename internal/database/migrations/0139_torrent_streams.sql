-- Torrent streams of addons, played over one route alone: a network
-- interface every torrent socket is bound to, or a SOCKS5 proxy, its
-- password sealed with POLYFIN_SECRET_KEY as the other stored secrets.
-- Off by default, as is sharing (an upload limit of 0); a download limit of
-- 0 is none.
ALTER TABLE settings
    ADD COLUMN torrent_streams boolean NOT NULL DEFAULT false,
    ADD COLUMN torrent_interface text NOT NULL DEFAULT '',
    ADD COLUMN torrent_proxy text NOT NULL DEFAULT '',
    ADD COLUMN torrent_proxy_user text NOT NULL DEFAULT '',
    ADD COLUMN torrent_proxy_password text NOT NULL DEFAULT '',
    ADD COLUMN torrent_download_mbps integer NOT NULL DEFAULT 0 CHECK (torrent_download_mbps BETWEEN 0 AND 10000),
    ADD COLUMN torrent_upload_mbps integer NOT NULL DEFAULT 0 CHECK (torrent_upload_mbps BETWEEN 0 AND 10000),
    ADD CONSTRAINT torrent_route CHECK (torrent_interface = '' OR torrent_proxy = ''),
    ADD CONSTRAINT torrent_streams_route CHECK (NOT torrent_streams OR torrent_interface <> '' OR torrent_proxy <> '');

-- Playing torrent streams is a permission of its own, off for every user,
-- administrators included, and in every group.
ALTER TABLE users ADD COLUMN torrent_streams boolean NOT NULL DEFAULT false;
ALTER TABLE user_groups ADD COLUMN torrent_streams boolean NOT NULL DEFAULT false;
