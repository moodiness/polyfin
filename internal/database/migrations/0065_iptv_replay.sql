-- Replay: the past programmes a provider keeps (its catch-up archive). An
-- entry of a list tells how its archive is reached: catchup is the kind of
-- address ('' for none; 'xc' for an Xtream Codes timeshift), catchup_days
-- how many days back it reaches, catchup_source the address template an M3U
-- playlist gives, which may embed credentials as url does.
ALTER TABLE iptv_entries
    ADD COLUMN catchup text NOT NULL DEFAULT ''
        CHECK (catchup IN ('', 'default', 'append', 'shift', 'flussonic', 'xc')),
    ADD COLUMN catchup_days integer NOT NULL DEFAULT 0 CHECK (catchup_days BETWEEN 0 AND 365),
    ADD COLUMN catchup_source text NOT NULL DEFAULT '' CHECK (length(catchup_source) <= 4096);

-- The time zone an Xtream Codes server gives its timeshift addresses in, as
-- its login says (server_info.timezone); empty when it does not say.
ALTER TABLE iptv_sources
    ADD COLUMN timezone text NOT NULL DEFAULT '' CHECK (length(timezone) <= 64);
