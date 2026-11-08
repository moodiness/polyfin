-- How TMDB numbers the episodes of a series against how its addon numbers
-- them, by the series' Stremio identifier, as aligned by their air days and
-- names (see the library package's episodealign.go): places holds the
-- episodes paired, [TMDB season, TMDB number, addon season, addon number]
-- each, those not paired having no episode on the other side, and is the
-- JSON null when both number them alike. tmdb_id is the series' TMDB
-- identifier, and episodes how many episodes the addon gave then: a row is
-- renewed after a lifetime, or once the addon gives another count. A series
-- no source numbers as TMDB does has no row.
CREATE TABLE episode_alignments (
    series text PRIMARY KEY,
    tmdb_id integer NOT NULL CHECK (tmdb_id > 0),
    episodes integer NOT NULL CHECK (episodes >= 0),
    places jsonb NOT NULL,
    asked_at timestamptz NOT NULL
);
