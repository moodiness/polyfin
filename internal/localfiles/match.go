package localfiles

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// reasonSearchFailed: the search failed, at least that of the addon
	// the file tells (see match), and found no title of its name.
	reasonSearchFailed = "search_failed"
	// reasonDiscTitle: it is a disc's title other than its main one, which
	// the disc folder's name does not tell (see titleUnit).
	reasonDiscTitle = "disc_title"
	// reasonDiscEpisode: it is a title of a show's disc whose episodes
	// neither an administrator nor the disc's name gives (see
	// numberDiscs).
	reasonDiscEpisode = "disc_episode"
	// reasonLoneExtra: its name is an extra's, as "Movie-trailer.mkv"
	// is, but its movie is not beside it (see extraOwners); a link makes
	// it a title (see splitExtras).
	reasonLoneExtra = "lone_extra"
)

// How a file was matched, its matched_by; a home video is its own title.
const (
	byName   = "name"
	byNFO    = "nfo"
	bySearch = "search"
	byLink   = "link"
	byOwn    = "own"
)

// searchLimit bounds the titles a search reads, and maxCandidates those a
// search for a link answers (see Search).
const (
	searchLimit   = 20
	maxCandidates = 10
)

// imdbPattern is an IMDb identifier.
var imdbPattern = regexp.MustCompile(`^tt\d{1,12}$`)

// match is the title a unit was matched to: its Stremio identifier, its
// IMDb and TMDB identifiers, name, poster and year, and how it was found;
// or, without identifier, why none was, and for a failed search the addon
// that failed it and why, as library.SearchFailure tells it.
type match struct {
	stremioID, imdb, tmdb      string
	name, poster               string
	year                       int
	by                         string
	reason                     string
	searchAddon, searchFailure string
}

// searches keeps a scan's searches, so that files of the same name ask
// once, and what failed (see fail). A search an addon answered "too many
// requests" waits, with every search of the scan, as long as the addon
// asks, at most maxRateWait, or rateWait doubled at each try when it does
// not say, and is tried again, rateTries times at most.
type searches struct {
	mu      sync.Mutex
	results map[string]*search
	// resume is when the scan's searches may ask again.
	resume time.Time
	// failed counts the units whose search failed, first tells the first
	// failure, and wait is the longest wait an addon asked for, which the
	// next try of the folder waits at least (see searchRetry).
	failed int
	first  library.SearchFailure
	wait   time.Duration
}

type search struct {
	done    chan struct{}
	results []library.RemoteResult
	err     error
}

// How a scan's searches wait out the addons' rate limits (see searches).
const (
	rateTries   = 3
	rateWait    = time.Second
	maxRateWait = time.Minute
)

func newSearches() *searches { return &searches{results: map[string]*search{}} }

// find searches the metadata addons for titles of kind named title, once
// per scan. Like library.RemoteSearch, it may answer the titles some
// addons found with the *library.SearchError of the others.
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
		found.results, found.err = c.search(ctx, titles, kind, title)
		close(found.done)
	}
	select {
	case <-found.done:
		return found.results, found.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// search searches titles, waiting out the addons' rate limits.
func (c *searches) search(ctx context.Context, titles Titles, kind library.Kind, title string) ([]library.RemoteResult, error) {
	for try := 0; ; try++ {
		c.mu.Lock()
		resume := c.resume
		c.mu.Unlock()
		if wait := time.Until(resume); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
		results, err := titles.RemoteSearch(ctx, kind, title, searchLimit)
		wait, limited := rateLimited(err)
		c.mu.Lock()
		c.wait = max(c.wait, wait)
		c.mu.Unlock()
		if !limited || try == rateTries || wait > maxRateWait {
			return results, err
		}
		if wait == 0 {
			wait = rateWait << try
		}
		c.mu.Lock()
		if until := time.Now().Add(wait); until.After(c.resume) {
			c.resume = until
		}
		c.mu.Unlock()
	}
}

// rateLimited reports whether an addon answered a search "too many
// requests", with the longest wait those that did asked for, 0 when none
// said.
func rateLimited(err error) (wait time.Duration, limited bool) {
	failed, ok := errors.AsType[*library.SearchError](err)
	if !ok {
		return 0, false
	}
	for _, f := range failed.Failures {
		if f.Failure == stremio.FailureRateLimited {
			limited = true
			if asked, ok := stremio.RetryAfter(f.Err); ok {
				wait = max(wait, asked)
			}
		}
	}
	return wait, limited
}

// fail counts a unit whose search failed with err, and returns its match:
// unmatched, telling the first addon that failed and why.
func (c *searches) fail(err error) match {
	f := library.SearchFailure{Failure: library.SearchFailed, Err: err}
	if failed, ok := errors.AsType[*library.SearchError](err); ok && len(failed.Failures) > 0 {
		f = failed.Failures[0]
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failed++
	if c.failed == 1 {
		c.first = f
	}
	return match{reason: reasonSearchFailed, searchAddon: f.Addon, searchFailure: f.Failure}
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
// one its NFO file gives, else the one title the metadata addons find of
// its name (and year, when given).
func (s *Service) unitMatch(ctx context.Context, kind string, e entry, link string, cache *searches) (match, error) {
	n := e.title
	if link != "" {
		return s.described(ctx, kind, link, n, byLink), nil
	}
	if n.imdb != "" {
		return s.described(ctx, kind, n.imdb, n, byName), nil
	}
	if n.tmdb != "" {
		return s.tmdbMatch(ctx, kind, n, cache, byName)
	}
	if tmdb, ok := strings.CutPrefix(e.nfo, "tmdb:"); ok {
		n.tmdb = tmdb
		return s.tmdbMatch(ctx, kind, n, cache, byNFO)
	}
	if e.nfo != "" {
		return s.described(ctx, kind, e.nfo, n, byNFO), nil
	}
	if e.file.extra {
		return match{reason: reasonDiscTitle}, nil
	}
	if n.title == "" {
		return match{reason: reasonUnreadableName}, nil
	}
	results, err := cache.find(ctx, s.titles, titleKind(kind), n.title)
	if err != nil && ctx.Err() != nil {
		return match{}, ctx.Err()
	}
	// A title none of the addons that answered knows may be one an addon
	// that failed knows.
	m := chooseResult(kind, n, results)
	if err != nil && (m.reason == reasonNotFound || m.reason == reasonOtherYear) {
		return cache.fail(err), nil
	}
	return m, nil
}

// chooseResult keeps the one title of results of the name and kind of n,
// and of its year when it gives one.
func chooseResult(kind string, n name, results []library.RemoteResult) match {
	item, reason := chooseItem(titleKind(kind), n.title, n.year, results)
	if reason != "" {
		return match{reason: reason}
	}
	return match{stremioID: stremioID(item), imdb: item.ProviderIDs["Imdb"], tmdb: item.ProviderIDs["Tmdb"], name: item.Name,
		poster: item.Images.Primary, year: item.ProductionYear, by: bySearch}
}

// chooseItem keeps the one title of results of kind named title, and of
// year when it is not zero, or answers why there is none: reasonNotFound,
// reasonOtherYear or reasonAmbiguous.
func chooseItem(kind library.Kind, title string, year int, results []library.RemoteResult) (library.Item, string) {
	want := fold(title)
	var named, dated []library.Item
	seen := map[string]bool{}
	for _, r := range results {
		item := r.Item
		if item.Kind != kind || item.StremioID == "" || fold(item.Name) != want {
			continue
		}
		id := stremioID(item)
		if seen[id] {
			continue
		}
		seen[id] = true
		named = append(named, item)
		if year == 0 || item.ProductionYear == year {
			dated = append(dated, item)
		}
	}
	switch {
	case len(named) == 0:
		return library.Item{}, reasonNotFound
	case len(dated) == 0:
		return library.Item{}, reasonOtherYear
	case len(dated) > 1:
		return library.Item{}, reasonAmbiguous
	}
	return dated[0], ""
}

// SearchLimit is how many titles a search reads, as a scan's do.
const SearchLimit = searchLimit

// ChooseTitle keeps, of the results of a search for title, the one title
// of kind of that name, and of year when it is not zero, by the rule a
// scan matches files by (see local-folders.md, Matching): found is false
// when there is none, and ambiguous true when there are several.
func ChooseTitle(kind library.Kind, title string, year int, results []library.RemoteResult) (item library.Item, found, ambiguous bool) {
	item, reason := chooseItem(kind, title, year, results)
	return item, reason == "", reason == reasonAmbiguous
}

// stremioID is the identifier a title found is kept under: its IMDb
// identifier, which most addons answer streams for, else the addon's.
func stremioID(item library.Item) string {
	if imdb := item.ProviderIDs["Imdb"]; imdbPattern.MatchString(imdb) {
		return imdb
	}
	return item.StremioID
}

// tmdbMatch finds the title of a TMDB identifier, found as by tells: the
// title of its name the metadata addons know under it, else the one they
// describe under "tmdb:…", by its IMDb identifier when they give one.
func (s *Service) tmdbMatch(ctx context.Context, kind string, n name, cache *searches, by string) (match, error) {
	if n.title != "" {
		results, err := cache.find(ctx, s.titles, titleKind(kind), n.title)
		if err != nil && ctx.Err() != nil {
			return match{}, ctx.Err()
		}
		for _, r := range results {
			if item := r.Item; item.Kind == titleKind(kind) && item.ProviderIDs["Tmdb"] == n.tmdb {
				return match{stremioID: stremioID(item), imdb: item.ProviderIDs["Imdb"], tmdb: n.tmdb, name: item.Name,
					poster: item.Images.Primary, year: item.ProductionYear, by: by}, nil
			}
		}
	}
	m := s.described(ctx, kind, "tmdb:"+n.tmdb, n, by)
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
// them, with the identifier their NFO file gave, the titles of a show's
// discs numbered as a scan numbers them (see numberDiscs).
func (s *Service) unitFiles(ctx context.Context, folder Folder, unit string) ([]entry, error) {
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, nfo_id, coalesce(length, 0) FROM local_files WHERE addon_id = $1 AND unit = $2 ORDER BY path",
		folder.Addon.ID, unit)
	if err != nil {
		return nil, err
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (entry, error) {
		var rel, nfo string
		var f found
		var seconds int
		if err := row.Scan(&rel, &f.size, &f.modified, &nfo, &seconds); err != nil {
			return entry{}, err
		}
		f.extra = folder.Kind == KindMovies && extraTitle(rel, unit)
		f.length = time.Duration(seconds) * time.Second
		e := readEntry(folder.Kind, locationName(folder.Addon.ManifestURL), rel, f)
		e.nfo = nfo
		return e, nil
	})
	if err != nil || folder.Kind != KindShows {
		return entries, err
	}
	given, err := s.givenEpisodes(ctx, folder.Addon.ID)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]entry, len(entries))
	for _, e := range entries {
		byPath[e.path] = e
	}
	numberDiscs(byPath, given)
	for i, e := range entries {
		entries[i] = byPath[e.path]
	}
	return entries, nil
}

// Search looks query up in the metadata addons for titles of the folder's
// kind, as a scan looks a file's name up, for an administrator to link one
// of its units to (see Link). It answers the titles a link can name, those
// with an IMDb identifier, each once, those of year first when year is
// given, at most maxCandidates: those the addons that answered found when
// others failed, ErrSearchFailed when none found any. A home videos
// folder, whose titles are its own, asks no metadata addon: it answers
// addons.ErrNotFound.
func (s *Service) Search(ctx context.Context, id accounts.ID, query string, year int) ([]library.Item, error) {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return nil, err
	}
	if folder.Kind == KindHomeVideos {
		return nil, addons.ErrNotFound
	}
	kind := titleKind(folder.Kind)
	results, err := s.titles.RemoteSearch(ctx, kind, query, searchLimit)
	if err != nil && len(results) == 0 {
		return nil, fmt.Errorf("%w: %w", ErrSearchFailed, err)
	}
	items := []library.Item{}
	seen := map[string]bool{}
	for _, r := range results {
		imdb := r.Item.ProviderIDs["Imdb"]
		if r.Item.Kind != kind || !imdbPattern.MatchString(imdb) || seen[imdb] {
			continue
		}
		seen[imdb] = true
		items = append(items, r.Item)
	}
	if year > 0 {
		// The titles of that year first, each list in the addons' order.
		dated := slices.DeleteFunc(slices.Clone(items), func(item library.Item) bool { return item.ProductionYear != year })
		items = append(dated, slices.DeleteFunc(items, func(item library.Item) bool { return item.ProductionYear == year })...)
	}
	return items[:min(len(items), maxCandidates)], nil
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
// matched again by their name or their NFO file. A file a scan reading
// extras reads as one, which the link kept a title (see splitExtras), is
// an extra again, or a lone one: the folder is scanned.
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
	if _, how := readExtra(unit); folder.ReadExtras && how != readAsTitle {
		return s.Scan(ctx, id)
	}
	return s.rematch(ctx, folder, unit, "")
}

// unit finds the unit path names in a folder: a unit, or a file of one. A
// home video is no unit a link links: its title is its own.
func (s *Service) unit(ctx context.Context, id accounts.ID, path string) (Folder, string, error) {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return Folder{}, "", err
	}
	if folder.Kind == KindHomeVideos {
		return Folder{}, "", addons.ErrNotFound
	}
	var unit string
	err = s.db.QueryRow(ctx, "SELECT unit FROM local_files WHERE addon_id = $1 AND (unit = $2 OR path = $2) LIMIT 1", id, path).Scan(&unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, "", addons.ErrNotFound
	}
	return folder, unit, err
}

// rematch matches a unit's files again, to link when given; the unit's
// artwork is then its new title's.
func (s *Service) rematch(ctx context.Context, folder Folder, unit, link string) error {
	entries, err := s.unitFiles(ctx, folder, unit)
	if err != nil || len(entries) == 0 {
		return err
	}
	m, err := s.unitMatch(ctx, folder.Kind, entries[0], link, newSearches())
	if err != nil {
		return err
	}
	defer s.artworkChanged()
	if err := s.store(ctx, folder.Addon.ID, folder.Kind, entries, m); err != nil {
		return err
	}
	return s.retryLater(ctx, folder.Addon.ID, m)
}

// Unmatched is a file of a folder no title was matched to: its path, the
// unit a link would link (see Link), what its name tells, and why, with,
// for a failed search, the addon that failed it and why (see
// library.SearchFailure). Disc is the kind of the disc whose entry the
// file is (DiscBluRay, DiscDVD), its unit the disc's folder, or one of its
// titles' (see titleUnit), or of the disc image it is (DiscDVDImage,
// DiscBluRayImage); empty for any other file. A title of a show's disc has
// the kind of its disc, its unit its show's folder, and DiscTitle telling
// which it is. In a music folder, it is a track whose tags and names give
// no title ("no_title") or no album ("no_album"), which no link links.
type Unmatched struct {
	Path          string
	Unit          string
	Title         string
	Year          int
	Size          int64
	Reason        string
	SearchAddon   string
	SearchFailure string
	Disc          string
	DiscTitle     *DiscTitle
}

// maxUnmatched bounds the unmatched files listed at once.
const maxUnmatched = 1000

// Unmatched lists the files of a folder no title was matched to, by path,
// at most maxUnmatched, with how many there are; the titles of a show's
// disc follow one another, in the order of their numbers.
func (s *Service) Unmatched(ctx context.Context, id accounts.ID) ([]Unmatched, int, error) {
	folder, err := s.Folder(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	query := `SELECT f.path, f.unit, f.title, coalesce(f.year, 0), f.size, f.reason, f.search_addon, f.search_failure,
			coalesce(i.kind, '') AS image FROM local_files f
		LEFT JOIN local_disc_images i ON i.addon_id = f.addon_id AND i.path = regexp_replace(f.path, '#[0-9]+$', '')
		WHERE f.addon_id = $1 AND f.stremio_id IS NULL`
	if folder.Kind == KindMusic {
		query = `SELECT path, path, title, year, size, reason, '', '', '' AS image FROM local_tracks WHERE addon_id = $1 AND reason <> ''`
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM ("+query+") u", id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, query+" ORDER BY path LIMIT $2", id, maxUnmatched)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Unmatched, error) {
		var u Unmatched
		var image string
		err := row.Scan(&u.Path, &u.Unit, &u.Title, &u.Year, &u.Size, &u.Reason, &u.SearchAddon, &u.SearchFailure, &image)
		u.Disc = imageKind(image)
		if kind, disc, ok := discEntry(u.Path); ok && folder.Kind == KindMovies {
			_, number, playlist := discPlaylist(u.Path)
			if _, title, ok := titleOf(u.Path); ok {
				number, playlist = title, true
			}
			if u.Unit == discUnit(disc) || playlist && u.Unit == titleUnit(disc, number) {
				u.Disc = kind
			}
		}
		if kind, _, ok := discEntry(u.Path); ok && folder.Kind == KindShows {
			u.Disc = kind
		}
		return u, err
	})
	if err != nil || folder.Kind != KindShows {
		return list, total, err
	}
	titles, err := s.showDiscTitles(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	for i, u := range list {
		if title, ok := titles[u.Path]; ok {
			list[i].DiscTitle = &title
		}
	}
	// A disc's titles in the order of their numbers, where its first is.
	slices.SortStableFunc(list, func(a, b Unmatched) int {
		key := func(u Unmatched) (string, int) {
			if u.DiscTitle == nil {
				return u.Path, 0
			}
			return u.DiscTitle.Disc, u.DiscTitle.Number
		}
		pathA, numberA := key(a)
		pathB, numberB := key(b)
		return cmp.Or(strings.Compare(pathA, pathB), cmp.Compare(numberA, numberB))
	})
	return list, total, nil
}
