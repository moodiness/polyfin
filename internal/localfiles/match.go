package localfiles

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Why a file is not matched, its reason.
const (
	// reasonUnreadableName: its name gives neither a title nor an
	// identifier, or, for an episode, not its numbers.
	reasonUnreadableName = "unreadable_name"
	// reasonNotFound: the metadata addons know no title of its name.
	reasonNotFound = "not_found"
	// reasonAmbiguous: they know several, and its name tells no year that
	// leaves one.
	reasonAmbiguous = "ambiguous"
	// reasonOtherYear: they know titles of its name, of other years.
	reasonOtherYear = "other_year"
	// reasonSearchFailed: the search failed.
	reasonSearchFailed = "search_failed"
)

// How a file was matched, its matched_by.
const (
	byName   = "name"
	bySearch = "search"
	byLink   = "link"
)

// searchLimit bounds the titles a search reads.
const searchLimit = 20

// imdbPattern is an IMDb identifier.
var imdbPattern = regexp.MustCompile(`^tt\d{1,12}$`)

// match is the title a unit was matched to: its Stremio identifier, its
// IMDb and TMDB identifiers, name, poster and year, and how it was found;
// or, without identifier, why none was.
type match struct {
	stremioID, imdb, tmdb string
	name, poster          string
	year                  int
	by                    string
	reason                string
}

// searches keeps a scan's searches, so that files of the same name ask
// once.
type searches struct {
	mu      sync.Mutex
	results map[string]*search
}

type search struct {
	done    chan struct{}
	results []library.RemoteResult
	err     error
}

func newSearches() *searches { return &searches{results: map[string]*search{}} }

// find searches the metadata addons for titles of kind named title, once
// per scan.
func (c *searches) find(ctx context.Context, titles Titles, kind library.Kind, title string) ([]library.RemoteResult, error) {
	key := string(kind) + "|" + fold(title)
	c.mu.Lock()
	found, ok := c.results[key]
	if !ok {
		found = &search{done: make(chan struct{})}
		c.results[key] = found
	}
	c.mu.Unlock()
	if !ok {
		found.results, found.err = titles.RemoteSearch(ctx, kind, title, searchLimit)
		close(found.done)
	}
	select {
	case <-found.done:
		return found.results, found.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// titleKind is the library kind of a folder's titles.
func titleKind(kind string) library.Kind {
	if kind == KindShows {
		return library.KindSeries
	}
	return library.KindMovie
}

// unitMatch finds the title of a unit, e being one of its files: the IMDb
// identifier it is linked to, else the identifier its name gives, else the
// one title the metadata addons find of its name (and year, when given).
func (s *Service) unitMatch(ctx context.Context, kind string, e entry, link string, cache *searches) (match, error) {
	n := e.title
	if link != "" {
		return s.described(ctx, kind, link, n, byLink), nil
	}
	if n.imdb != "" {
		return s.described(ctx, kind, n.imdb, n, byName), nil
	}
	if n.tmdb != "" {
		return s.tmdbMatch(ctx, kind, n, cache)
	}
	if n.title == "" {
		return match{reason: reasonUnreadableName}, nil
	}
	results, err := cache.find(ctx, s.titles, titleKind(kind), n.title)
	if err != nil {
		if ctx.Err() != nil {
			return match{}, ctx.Err()
		}
		s.logger.Debug("The metadata addons could not search a local file's title", "title", n.title, "error", err)
		return match{reason: reasonSearchFailed}, nil
	}
	return chooseResult(kind, n, results), nil
}

// chooseResult keeps the one title of results of the name and kind of n,
// and of its year when it gives one.
func chooseResult(kind string, n name, results []library.RemoteResult) match {
	want := fold(n.title)
	var named, dated []library.Item
	seen := map[string]bool{}
	for _, r := range results {
		item := r.Item
		if item.Kind != titleKind(kind) || item.StremioID == "" || fold(item.Name) != want {
			continue
		}
		id := stremioID(item)
		if seen[id] {
			continue
		}
		seen[id] = true
		named = append(named, item)
		if n.year == 0 || item.ProductionYear == n.year {
			dated = append(dated, item)
		}
	}
	switch {
	case len(named) == 0:
		return match{reason: reasonNotFound}
	case len(dated) == 0:
		return match{reason: reasonOtherYear}
	case len(dated) > 1:
		return match{reason: reasonAmbiguous}
	}
	item := dated[0]
	return match{stremioID: stremioID(item), imdb: item.ProviderIDs["Imdb"], tmdb: item.ProviderIDs["Tmdb"], name: item.Name,
		poster: item.Images.Primary, year: item.ProductionYear, by: bySearch}
}

// stremioID is the identifier a title found is kept under: its IMDb
// identifier, which most addons answer streams for, else the addon's.
func stremioID(item library.Item) string {
	if imdb := item.ProviderIDs["Imdb"]; imdbPattern.MatchString(imdb) {
		return imdb
	}
	return item.StremioID
}

// tmdbMatch finds the title of a TMDB identifier: the title of its name
// the metadata addons know under it, else the one they describe under
// "tmdb:…", by its IMDb identifier when they give one.
func (s *Service) tmdbMatch(ctx context.Context, kind string, n name, cache *searches) (match, error) {
	if n.title != "" {
		results, err := cache.find(ctx, s.titles, titleKind(kind), n.title)
		if err != nil && ctx.Err() != nil {
			return match{}, ctx.Err()
		}
		for _, r := range results {
			if item := r.Item; item.Kind == titleKind(kind) && item.ProviderIDs["Tmdb"] == n.tmdb {
				return match{stremioID: stremioID(item), imdb: item.ProviderIDs["Imdb"], tmdb: n.tmdb, name: item.Name,
					poster: item.Images.Primary, year: item.ProductionYear, by: byName}, nil
			}
		}
	}
	m := s.described(ctx, kind, "tmdb:"+n.tmdb, n, byName)
	m.tmdb = n.tmdb
	if imdbPattern.MatchString(m.imdb) {
		m.stremioID = m.imdb
	}
	return m, nil
}

// described is the match of a title by its identifier, named, pictured and
// dated as the metadata addons describe it, else as n tells.
func (s *Service) described(ctx context.Context, kind, id string, n name, by string) match {
	m := match{stremioID: id, name: n.title, year: n.year, by: by}
	if imdbPattern.MatchString(id) {
		m.imdb = id
	}
	meta, ok := s.titles.TitleMeta(ctx, metaType(kind), id)
	if !ok {
		if m.name == "" {
			m.name = id
		}
		return m
	}
	if name := strings.TrimSpace(meta.Name); name != "" {
		m.name = name
	}
	m.poster = meta.Poster
	if year := metaYear(meta); year > 0 {
		m.year = year
	}
	if m.imdb == "" && imdbPattern.MatchString(meta.ImdbID) {
		m.imdb = meta.ImdbID
	}
	if m.tmdb == "" {
		m.tmdb = string(meta.TmdbID)
	}
	return m
}

// metaYear is the first year of a description's release.
func metaYear(meta stremio.Meta) int {
	for _, text := range []stremio.Text{meta.ReleaseInfo, meta.Year} {
		if len(text) >= 4 {
			if year, err := strconv.Atoi(string(text[:4])); err == nil {
				return year
			}
		}
	}
	return 0
}

// matchedUnits reads the title each matched unit of a folder has.
func (s *Service) matchedUnits(ctx context.Context, id accounts.ID) (map[string]match, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (unit) unit, stremio_id, imdb_id, tmdb_id, name, poster, coalesce(title_year, 0), matched_by
		FROM local_files WHERE addon_id = $1 AND stremio_id IS NOT NULL ORDER BY unit, path`, id)
	if err != nil {
		return nil, err
	}
	matched := map[string]match{}
	for rows.Next() {
		var unit string
		var m match
		if err := rows.Scan(&unit, &m.stremioID, &m.imdb, &m.tmdb, &m.name, &m.poster, &m.year, &m.by); err != nil {
			rows.Close()
			return nil, err
		}
		matched[unit] = m
	}
	return matched, rows.Err()
}

// unitFiles reads the files of one of a folder's units, as a scan read
// them.
func (s *Service) unitFiles(ctx context.Context, id accounts.ID, kind, unit string) ([]entry, error) {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified FROM local_files WHERE addon_id = $1 AND unit = $2 ORDER BY path", id, unit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entry, error) {
		var rel string
		var f found
		if err := row.Scan(&rel, &f.size, &f.modified); err != nil {
			return entry{}, err
		}
		return readEntry(kind, rel, f), nil
	})
}

// Link links a unit of a folder to an IMDb identifier, for good: path is
// one of its files, or a show's folder, which links every file of the
// show. Its files are that title's at once, and stay so through scans.
func (s *Service) Link(ctx context.Context, id accounts.ID, path, imdb string) error {
	imdb = strings.ToLower(strings.TrimSpace(imdb))
	if !imdbPattern.MatchString(imdb) {
		return ErrInvalidIMDb
	}
	folder, unit, err := s.unit(ctx, id, path)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO local_links (addon_id, unit, imdb_id) VALUES ($1, $2, $3)
		ON CONFLICT (addon_id, unit) DO UPDATE SET imdb_id = $3`, id, unit, imdb); err != nil {
		return err
	}
	return s.rematch(ctx, folder, unit, imdb)
}

// Unlink removes the link of a unit of a folder (see Link): its files are
// matched again by their name.
func (s *Service) Unlink(ctx context.Context, id accounts.ID, path string) error {
	folder, unit, err := s.unit(ctx, id, path)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, "DELETE FROM local_links WHERE addon_id = $1 AND unit = $2", id, unit)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return addons.ErrNotFound
	}
	return s.rematch(ctx, folder, unit, "")
}

// unit finds the unit path names in a folder: a unit, or a file of one.
func (s *Service) unit(ctx context.Context, id accounts.ID, path string) (Folder, string, error) {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return Folder{}, "", err
	}
	var unit string
	err = s.db.QueryRow(ctx, "SELECT unit FROM local_files WHERE addon_id = $1 AND (unit = $2 OR path = $2) LIMIT 1", id, path).Scan(&unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, "", addons.ErrNotFound
	}
	return folder, unit, err
}

// rematch matches a unit's files again, to link when given.
func (s *Service) rematch(ctx context.Context, folder Folder, unit, link string) error {
	entries, err := s.unitFiles(ctx, folder.Addon.ID, folder.Kind, unit)
	if err != nil || len(entries) == 0 {
		return err
	}
	m, err := s.unitMatch(ctx, folder.Kind, entries[0], link, newSearches())
	if err != nil {
		return err
	}
	return s.store(ctx, folder.Addon.ID, folder.Kind, entries, m)
}

// Unmatched is a file of a folder no title was matched to: its path, the
// unit a link would link (see Link), what its name tells, and why.
type Unmatched struct {
	Path   string
	Unit   string
	Title  string
	Year   int
	Size   int64
	Reason string
}

// maxUnmatched bounds the unmatched files listed at once.
const maxUnmatched = 1000

// Unmatched lists the files of a folder no title was matched to, by path,
// at most maxUnmatched, with how many there are.
func (s *Service) Unmatched(ctx context.Context, id accounts.ID) ([]Unmatched, int, error) {
	if _, err := s.Folder(ctx, id); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM local_files WHERE addon_id = $1 AND stremio_id IS NULL", id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT path, unit, title, coalesce(year, 0), size, reason FROM local_files
		WHERE addon_id = $1 AND stremio_id IS NULL ORDER BY path LIMIT $2`, id, maxUnmatched)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Unmatched, error) {
		var u Unmatched
		err := row.Scan(&u.Path, &u.Unit, &u.Title, &u.Year, &u.Size, &u.Reason)
		return u, err
	})
	return list, total, err
}
