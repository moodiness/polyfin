-- The streaming services TMDB's details of titles name, by TMDB
-- identifier: their name and the path of their logo on TMDB's image
-- server, '' when TMDB has none, as the details read last (seen_at) gave
-- them. A service is a name, whatever its letter case.
CREATE TABLE tmdb_services (
    tmdb_id integer PRIMARY KEY CHECK (tmdb_id > 0),
    name text NOT NULL CHECK (name <> ''),
    logo_path text NOT NULL DEFAULT '',
    seen_at timestamptz NOT NULL
);

CREATE INDEX tmdb_services_name ON tmdb_services (lower(name));

-- The streaming services that offer the title by subscription, free or
-- free with ads, in the country they were read for (service_country, the
-- server's rating country then): their names and TMDB identifiers, in
-- TMDB's order. NULL until they are read, which makes the row due: the
-- rows kept before are read again once.
ALTER TABLE title_credits
    ADD COLUMN service_country text,
    ADD COLUMN services text[],
    ADD COLUMN service_ids integer[];
