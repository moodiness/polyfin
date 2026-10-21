-- Torrent streams may also go through a WireGuard tunnel Polyfin runs
-- itself: its configuration, a provider's .conf, is stored whole, sealed
-- with POLYFIN_SECRET_KEY as the other stored secrets, for its private and
-- preshared keys. One route alone still: an interface, a proxy or a
-- tunnel.
ALTER TABLE settings
    ADD COLUMN torrent_wireguard text NOT NULL DEFAULT '',
    DROP CONSTRAINT torrent_route,
    DROP CONSTRAINT torrent_streams_route,
    ADD CONSTRAINT torrent_route CHECK (num_nonnulls(NULLIF(torrent_interface, ''), NULLIF(torrent_proxy, ''), NULLIF(torrent_wireguard, '')) <= 1),
    ADD CONSTRAINT torrent_streams_route CHECK (NOT torrent_streams OR torrent_interface <> '' OR torrent_proxy <> '' OR torrent_wireguard <> '');
