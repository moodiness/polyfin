package iptv

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database/unsynced"
)

// Names learned from the metadata addons (Options.MetadataNames). A
// provider names its titles in its own way, with prefixes and branding
// the list's cleaning cannot tell from a name (see titleName). A source
// that enriches its titles (Options.Enrichment) and whose option is on
// names a movie or a series after the metadata addon that describes it,
// once it is described with that addon's description: the library learns
// the addon's name (see LearnName) and the source keeps it
// (iptv_title_names), under the title's key, a group's under its
// leader's, with the identifiers the provider's list gave the title. The
// catalogs and their counts, searches and the descriptions then show it,
// a group the name its leader learned, else its face; a search matches it
// as well as the provider's. A name learned forgets the descriptions that
// show it (see descriptions.go), not the source's version: the source's
// other descriptions are kept, and the listings by name sort it again
// within a few minutes (see jellyfin's nameorder.go).
//
// Only the name shows: the titles keep their identifiers, groups, their
// users' data and their entries' names, which label the versions of a
// group (see labelled) and Content › Versions lists. A title not yet
// described with an addon's description keeps the provider's name, so
// lists mix both until their titles are opened. Turned off, or with
// enrichment off, the provider's names show again; the names learned are
// kept for when it is turned back on, and go with their titles, or once
// their titles have other identifiers (see forgetGoneNames).

// namesShown selects, for its source i, the titles that show the names
// they learned: those of a source that enriches its titles and names them
// so.
const namesShown = `i.enrichment AND i.metadata_names`

// learnedNames joins, for t of source i, the group g it is the face of
// (see groupOfFace) and the names learned while i shows them: as ln, the
// name g's leader learned; as fn, t's own.
const learnedNames = ` LEFT JOIN iptv_title_names ln ON ` + namesShown + ` AND ln.addon_id = t.addon_id AND ln.type = t.type AND ln.key = g.leader
	LEFT JOIN iptv_title_names fn ON ` + namesShown + ` AND fn.addon_id = t.addon_id AND fn.type = t.type AND fn.key = t.key`

// learnedName is the name a title of learnedNames shows instead of the
// provider's, empty for none.
const learnedName = `coalesce(ln.name, fn.name, '')`

// learnedMatch selects, for t of source $1 and type $2, the titles whose
// name shown matches the pattern of parameter number pattern, while the
// source shows the names learned: a group's face by its leader's name,
// else a title by its own, as learnedNames shows them. The keys are read
// once, before the titles, so that a search filters them as it reads
// them, joining nothing.
func learnedMatch(pattern string) string {
	return `t.key = ANY(ARRAY(SELECT n.key FROM iptv_title_names n JOIN iptv_sources i ON i.addon_id = n.addon_id AND ` + namesShown + `
			WHERE n.addon_id = $1 AND n.type = $2 AND n.search LIKE $` + pattern + `
			AND NOT EXISTS (SELECT 1 FROM iptv_title_groups g JOIN iptv_title_names l ON l.addon_id = g.addon_id AND l.type = g.type AND l.key = g.leader
				WHERE g.addon_id = n.addon_id AND g.type = n.type AND g.face = n.key AND g.leader <> n.key)
		UNION SELECT g.face FROM iptv_title_groups g JOIN iptv_sources i ON i.addon_id = g.addon_id AND ` + namesShown + `
			JOIN iptv_title_names n ON n.addon_id = g.addon_id AND n.type = g.type AND n.key = g.leader
			WHERE g.addon_id = $1 AND g.type = $2 AND n.search LIKE $` + pattern + `))`
}

// LearnName keeps name as the name of a source's movie or series of
// Stremio identifier id, a group's under its leader's, the metadata
// addon's that described it, while the source names its titles so, with
// the identifiers the provider's list gives the title, or the group's
// TMDB id: its listings and descriptions show it from then on. A name it
// already has writes nothing. What it keeps may be learned again, so its
// write does not wait for the disk (see unsynced.Exec).
func (s *Service) LearnName(ctx context.Context, source accounts.ID, id, name string) error {
	of, typ, key, ok := titleOfID(id)
	name = strings.TrimSpace(name)
	if !ok || of != source || name == "" {
		return nil
	}
	var kept string
	err := s.db.QueryRow(ctx, "SELECT name FROM iptv_title_names WHERE addon_id = $1 AND type = $2 AND key = $3", source, typ, key).Scan(&kept)
	if err == nil && kept == name {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tag, err := unsynced.Exec(ctx, s.db, `INSERT INTO iptv_title_names (addon_id, type, key, name, search, tmdb, imdb)
		SELECT i.addon_id, $2::text, $3::text, $4::text, $5::text, coalesce(t.tmdb, g.tmdb), coalesce(t.imdb, '')
		FROM iptv_sources i
		LEFT JOIN iptv_titles t ON t.addon_id = i.addon_id AND t.type = $2 AND t.key = $3
		LEFT JOIN iptv_title_groups g ON g.addon_id = i.addon_id AND g.type = $2 AND g.leader = $3
		WHERE i.addon_id = $1 AND `+namesShown+` AND (t.key IS NOT NULL OR g.leader IS NOT NULL)
		ON CONFLICT (addon_id, type, key) DO UPDATE SET name = excluded.name, search = excluded.search, tmdb = excluded.tmdb, imdb = excluded.imdb
		WHERE iptv_title_names.name <> excluded.name`, source, typ, key, name, Fold(name))
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	// The title's description shows it, and so does its group's face's.
	s.descriptions.forget(source, typ, key)
	group, err := s.titleGroup(ctx, source, typ, key)
	if err != nil || group == nil {
		return err
	}
	for _, member := range group.listed {
		s.descriptions.forget(source, typ, member)
	}
	return nil
}

// forgetGoneNames forgets, in tx, the names of a source's titles its
// lists no longer give, but for the leaders of its groups, which keep the
// group's, and those of titles whose list, or whose group, gives other
// identifiers than when they learned them.
func forgetGoneNames(ctx context.Context, tx pgx.Tx, source accounts.ID) error {
	_, err := tx.Exec(ctx, `DELETE FROM iptv_title_names n WHERE n.addon_id = $1
		AND NOT EXISTS (SELECT 1 FROM iptv_titles t WHERE t.addon_id = n.addon_id AND t.type = n.type AND t.key = n.key
			AND t.tmdb = n.tmdb AND t.imdb = n.imdb)
		AND NOT EXISTS (SELECT 1 FROM iptv_title_groups g WHERE g.addon_id = n.addon_id AND g.type = n.type AND g.leader = n.key
			AND g.tmdb = n.tmdb)`, source)
	return err
}
