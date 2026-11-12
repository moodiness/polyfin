-- The networks and companies TMDB's details of titles name, by kind and
-- TMDB identifier: their name and the path of their logo on TMDB's image
-- server, '' when TMDB has none, as the details read last (seen_at) gave
-- them. A studio is a name, whatever its letter case: the networks and
-- companies sharing one are one studio, whose page lists the titles of
-- them all and shows the logo of the first network of a logo, else of the
-- first company.
CREATE TABLE tmdb_studios (
    kind text NOT NULL CHECK (kind IN ('network', 'company')),
    tmdb_id integer NOT NULL CHECK (tmdb_id > 0),
    name text NOT NULL CHECK (name <> ''),
    logo_path text NOT NULL DEFAULT '',
    seen_at timestamptz NOT NULL,
    PRIMARY KEY (kind, tmdb_id)
);

CREATE INDEX tmdb_studios_name ON tmdb_studios (lower(name));

-- The TMDB identifiers of the networks a series aired on and of the
-- companies that made the title, each once (see tmdb_studios). NULL until
-- they are read, which makes the row due: the rows kept before are read
-- again once.
ALTER TABLE title_credits
    ADD COLUMN network_ids integer[],
    ADD COLUMN company_ids integer[];
