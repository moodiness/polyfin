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
	// reasonNoSearch: no addon of the server describes and searches
	// titles of its kind (see library.Service.RemoteSearches), so none
	// could be asked; a user's own addons do not count.
	reasonNoSearch = "no_search"
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
// A search keeps a title by a looser rule when none matches exactly (see
// chooseItem): byYear for a title of the year before or after the name's,
// byNumerals for one whose numerals read alike, byYearNumerals for both.
const (
	byName         = "name"
	byNFO          = "nfo"
	bySearch       = "search"
	byYear         = "year"
	byNumerals     = "numerals"
	byYearNumerals = "year_numerals"
	byLink         = "link"
	byOwn          = "own"
)

// looserMethods are the matched_by of the titles a looser rule kept.
var looserMethods = []string{byYear, byNumerals, byYearNumerals}

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
// that failed it and why, as library.SearchFailure tells it. Undescribed
// tells a title found by its identifier that no metadata addon of the
// server describes as the folder's kind (see described).
type match struct {
	stremioID, imdb, tmdb      string
	name, poster               string
	year                       int
	by                         string
	reason                     string
	searchAddon, searchFailure string
	undescribed                bool
}

// searches keeps a scan's searches, so that files of the same name ask
// once, and what failed (see fail). A search an addon answered "too many
// requests" waits, with every search of the scan, as long as the addon
// asks, at most maxRateWait, or rateWait doubled at each try when it does
// not say, and is tried again, rateTries times at most. A search an addon
// failed (HTTP 5xx) is tried again after each of serverWaits, as such
// failures are often brief; another refusal (HTTP 4xx) is not.
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
	// searchable tells, by kind, whether a server's addon searches
	// titles of the kind, asked once per scan.
	searchable map[library.Kind]bool
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

// serverWaits are the waits before a search an addon failed is tried
// again, within the scan (see searches).
var serverWaits = [...]time.Duration{time.Second, 3 * time.Second}

func newSearches() *searches {
	return &searches{results: map[string]*search{}, searchable: map[library.Kind]bool{}}
}

// canSearch reports whether a server's addon searches titles of kind, once
// per scan. When that cannot be told, the titles are searched, and the
// search tells what fails.
func (c *searches) canSearch(ctx context.Context, titles Titles, kind library.Kind) bool {
	c.mu.Lock()
	known, ok := c.searchable[kind]
	c.mu.Unlock()
	if ok {
		return known
	}
	known, err := titles.RemoteSearches(ctx, kind)
	if err != nil {
		return true
	}
	c.mu.Lock()
	c.searchable[kind] = known
	c.mu.Unlock()
	return known
}

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

// search searches titles, waiting out the addons' rate limits, and trying
// again a search an addon failed (see searches).
func (c *searches) search(ctx context.Context, titles Titles, kind library.Kind, title string) ([]library.RemoteResult, error) {
	failures := 0
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
		if !limited && serverFailed(err) && failures < len(serverWaits) {
			timer := time.NewTimer(serverWaits[failures])
			failures++
			select {
			case <-timer.C:
				// A failure is no rate limit's try.
				try--
				continue
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
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

// serverFailed reports whether an addon failed a search (HTTP 5xx).
func serverFailed(err error) bool {
	failed, ok := errors.AsType[*library.SearchError](err)
	return ok && slices.ContainsFunc(failed.Failures, func(f library.SearchFailure) bool { return f.Failure == stremio.FailureServerError })
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
// its name (and year, when given), by the exact rule only when exact is
// true, as for a unit unlinked from a looser match (see Unlink).
func (s *Service) unitMatch(ctx context.Context, kind string, e entry, link string, cache *searches, exact bool) (match, error) {
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
	if !cache.canSearch(ctx, s.titles, titleKind(kind)) {
		return match{reason: reasonNoSearch}, nil
	}
	results, err := cache.find(ctx, s.titles, titleKind(kind), n.title)
	if err != nil && ctx.Err() != nil {
		return match{}, ctx.Err()
	}
	// A title none of the addons that answered knows may be one an addon
	// that failed knows; so may the title of the very name and year, which
	// would win over one a looser rule kept.
	m := chooseResult(kind, n, results, exact)
	if err != nil && (m.reason == reasonNotFound || m.reason == reasonOtherYear || slices.Contains(looserMethods, m.by)) {
		return s.searchFallback(ctx, kind, n, cache, err)
	}
	// A search that found no title of the name, even with its numerals
	// read alike, is made once more with them written the other way:
	// addons that search names as they are find "Harbor Lights II" only
	// when asked for "Harbor Lights II", not "Harbor Lights 2". What it
	// finds is chosen by the same rules. A unit left to the exact rule
	// needs no such search.
	other := otherNumerals(n.title)
	if exact || err != nil || m.reason != reasonNotFound || other == "" {
		return m, nil
	}
	results, err = cache.find(ctx, s.titles, titleKind(kind), other)
	if err != nil && ctx.Err() != nil {
		return match{}, ctx.Err()
	}
	m = chooseResult(kind, n, results, exact)
	if err != nil && (m.reason == reasonNotFound || m.reason == reasonOtherYear || slices.Contains(looserMethods, m.by)) {
		return cache.fail(err), nil
	}
	return m, nil
}

// chooseResult keeps the one title of results of the name and kind of n,
// and of its year when it gives one, or by a looser rule unless exact (see
// chooseItem).
func chooseResult(kind string, n name, results []library.RemoteResult, exact bool) match {
	item, by, reason := chooseItem(titleKind(kind), n.title, n.year, results, exact)
	if reason != "" {
		return match{reason: reason}
	}
	return match{stremioID: stremioID(item), imdb: item.ProviderIDs["Imdb"], tmdb: item.ProviderIDs["Tmdb"], name: item.Name,
		poster: item.Images.Primary, year: item.ProductionYear, by: by}
}

// chooseItem keeps the one title of results of kind named title, and of
// year when it is not zero, found bySearch. When there is none and exact
// is false, it keeps the one title a looser rule leaves: of the year
// before or after (byYear), or named alike once numerals are read
// (byNumerals, see numerals), and only then the one both rules leave
// (byYearNumerals). Several titles at the first of those steps that finds
// any leave none, as several of the very name and year do. Else it
// answers why there is none: reasonNotFound, reasonOtherYear or
// reasonAmbiguous.
//
// Jellyfin's TMDb ranking scores a year one off as the very year
// (MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs, ScoreYear, lines
// 246-255 at v12.4) and keeps the provider's first title of the best
// score; here a title of the very year always wins, and a title is kept
// only when it is the only one left.
func chooseItem(kind library.Kind, title string, year int, results []library.RemoteResult, exact bool) (item library.Item, by, reason string) {
	want := fold(title)
	wantNumerals := numerals(want)
	// The titles each step finds: the very name and year, one looser
	// rule, both.
	var steps [3][]library.Item
	var stepBy [3]string
	named := false
	seen := map[string]bool{}
	for _, r := range results {
		item := r.Item
		if item.Kind != kind || item.StremioID == "" {
			continue
		}
		folded := fold(item.Name)
		sameName := folded == want
		alike := !exact && !sameName && numerals(folded) == wantNumerals
		if !sameName && !alike {
			continue
		}
		id := stremioID(item)
		if seen[id] {
			continue
		}
		seen[id] = true
		named = true
		sameYear := year == 0 || item.ProductionYear == year
		nearYear := !exact && year != 0 && item.ProductionYear != 0 && (item.ProductionYear == year-1 || item.ProductionYear == year+1)
		step, how := -1, ""
		switch {
		case sameName && sameYear:
			step, how = 0, bySearch
		case sameName && nearYear:
			step, how = 1, byYear
		case alike && sameYear:
			step, how = 1, byNumerals
		case alike && nearYear:
			step, how = 2, byYearNumerals
		}
		if step >= 0 {
			steps[step] = append(steps[step], item)
			stepBy[step] = how
		}
	}
	if !named {
		return library.Item{}, "", reasonNotFound
	}
	for i, found := range steps {
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], stepBy[i], ""
		}
		return library.Item{}, "", reasonAmbiguous
	}
	return library.Item{}, "", reasonOtherYear
}

// romanNumerals reads the Roman numerals of titles, from I to XXXIX: those
// written with I, V and X only, so that words such as "mix", "civil" or
// "liv" stay words.
var romanNumerals = func() map[string]int {
	read := map[string]int{}
	for n := 1; n < 40; n++ {
		read[roman(n)] = n
	}
	return read
}()

// roman is n, from 1 to 39, in lower-case Roman numerals.
func roman(n int) string {
	ones := []string{"", "i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix"}
	return strings.Repeat("x", n/10) + ones[n%10]
}

// partNumbers are the numbers written in words read after "part", and
// partWords the same numbers, by their figures.
var partNumbers = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
	"eighteen": 18, "nineteen": 19, "twenty": 20}

var partWords = func() map[int]string {
	words := make(map[int]string, len(partNumbers))
	for word, n := range partNumbers {
		words[n] = word
	}
	return words
}()

// otherNumerals is a file's title with its numerals written the other
// way, for a second search (see unitMatch): figures from 1 to 39 in Roman
// numerals, or in words after "Part" up to twenty, and Roman numerals and
// numbers written in words after "Part" in figures, so that "Harbor Lights
// 2" is searched as "Harbor Lights II", "Harbor Lights II" as "Harbor
// Lights 2" and "Tidewater Part 1" as "Tidewater Part One". It is empty
// when the title holds no numeral that numerals reads alike (see
// numerals): for a title of one word, a single letter that is neither the
// last word nor after "Part", or a number written in words elsewhere.
func otherNumerals(title string) string {
	words := strings.Fields(title)
	if len(words) < 2 {
		return ""
	}
	// Each word's letters and digits, without the punctuation around them.
	starts, ends, cores := make([]int, len(words)), make([]int, len(words)), make([]string, len(words))
	for i, word := range words {
		starts[i] = strings.IndexFunc(word, isAlphanumeric)
		if starts[i] >= 0 {
			ends[i] = strings.LastIndexFunc(word, isAlphanumeric) + 1
			cores[i] = strings.ToLower(word[starts[i]:ends[i]])
		}
	}
	other := slices.Clone(words)
	for i, core := range cores {
		if core == "" {
			continue
		}
		afterPart := i > 0 && cores[i-1] == "part"
		written := ""
		if n, err := strconv.Atoi(core); err == nil && core[0] != '0' && n < 40 {
			if word, ok := partWords[n]; ok && afterPart {
				written = strings.ToUpper(word[:1]) + word[1:]
			} else if r := roman(n); len(r) > 1 || afterPart || i == len(words)-1 {
				written = strings.ToUpper(r)
			}
		} else if n, ok := romanNumerals[core]; ok && (len(core) > 1 || afterPart || i == len(words)-1) {
			written = strconv.Itoa(n)
		} else if n, ok := partNumbers[core]; ok && afterPart {
			written = strconv.Itoa(n)
		}
		if written != "" {
			other[i] = words[i][:starts[i]] + written + words[i][ends[i]:]
		}
	}
	searched := strings.Join(other, " ")
	if fold(searched) == fold(title) || numerals(fold(searched)) != numerals(fold(title)) {
		return ""
	}
	return searched
}

// isAlphanumeric reports whether r is an ASCII letter or digit.
func isAlphanumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// numerals is a folded title (see fold) with its numbers written in
// figures, so that "Harbor Lights II" reads as "Harbor Lights 2" and
// "Part One" as "Part 1": its Roman numerals (see romanNumerals), and the
// numbers written in words after "part". A title of one word is left as it
// is, so that "I", "X" or "XXX" is no number, nor is a number written in
// words elsewhere ("One Day"). A single letter, "i", "v" or "x", is a
// numeral only as the title's last word or after "part": "I Robot" and
// "V for Vendetta" keep their words.
func numerals(folded string) string {
	words := strings.Split(folded, " ")
	if len(words) < 2 {
		return folded
	}
	for i, word := range words {
		afterPart := i > 0 && words[i-1] == "part"
		if n, ok := romanNumerals[word]; ok && (len(word) > 1 || afterPart || i == len(words)-1) {
			words[i] = strconv.Itoa(n)
		} else if n, ok := partNumbers[word]; ok && afterPart {
			words[i] = strconv.Itoa(n)
		}
	}
	return strings.Join(words, " ")
}

// SearchLimit is how many titles a search reads, as a scan's do.
const SearchLimit = searchLimit

// ChooseTitle keeps, of the results of a search for title, the one title
// of kind of that name, and of year when it is not zero, by the exact rule
// a scan matches files by first (see local-folders.md, Matching): found
// is false when there is none, and ambiguous true when there are several.
func ChooseTitle(kind library.Kind, title string, year int, results []library.RemoteResult) (item library.Item, found, ambiguous bool) {
	item, _, reason := chooseItem(kind, title, year, results, true)
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
		m.undescribed = true
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
// an extra again, or a lone one: the folder is scanned. A unit a looser
// rule matched, without a link, is unlinked from that title for good: it
// is matched again by the exact rule only, through every scan, and left
// unmatched when that finds no title (see exactUnits).
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
		tag, err = s.db.Exec(ctx, `INSERT INTO local_exact_units (addon_id, unit)
			SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM local_files WHERE addon_id = $1 AND unit = $2 AND matched_by = ANY($3))
			ON CONFLICT DO NOTHING`, id, unit, looserMethods)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return addons.ErrNotFound
		}
		return s.rematch(ctx, folder, unit, "")
	}
	if _, how := readExtra(unit); folder.ReadExtras && how != readAsTitle {
		return s.Scan(ctx, id)
	}
	return s.rematch(ctx, folder, unit, "")
}

// exactUnits reads the units of a folder unlinked from a looser match (see
// Unlink), which only the exact rule matches.
func (s *Service) exactUnits(ctx context.Context, id accounts.ID) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, "SELECT unit FROM local_exact_units WHERE addon_id = $1", id)
	if err != nil {
		return nil, err
	}
	exact := map[string]bool{}
	var unit string
	_, err = pgx.ForEachRow(rows, []any{&unit}, func() error {
		exact[unit] = true
		return nil
	})
	return exact, err
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
// artwork is then its new title's, and what users did with the plain
// video it was follows its files (see follow).
func (s *Service) rematch(ctx context.Context, folder Folder, unit, link string) error {
	return s.matchUnit(ctx, folder, unit, link, false)
}

// matchUnit is rematch; with keep set, a unit whose files are matched
// keeps its title unless a title is found again, as a scan keeps it: an
// addon failing, or finding no title, leaves it as it was.
func (s *Service) matchUnit(ctx context.Context, folder Folder, unit, link string, keep bool) error {
	entries, err := s.unitFiles(ctx, folder, unit)
	if err != nil || len(entries) == 0 {
		return err
	}
	var exact bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM local_exact_units WHERE addon_id = $1 AND unit = $2)", folder.Addon.ID,
		unit).Scan(&exact); err != nil {
		return err
	}
	m, err := s.unitMatch(ctx, folder.Kind, entries[0], link, newSearches(), exact)
	if err != nil {
		return err
	}
	if keep && m.stremioID == "" {
		return nil
	}
	if err := s.noteLink(ctx, folder.Addon.ID, unit, link, m); err != nil {
		return err
	}
	defer s.artworkChanged()
	if err := s.store(ctx, folder.Addon.ID, folder.Kind, entries, m); err != nil {
		return err
	}
	if err := s.retryLater(ctx, folder.Addon.ID, m); err != nil {
		return err
	}
	// The files not found are left to the scans: one may be under way.
	return s.follow(ctx, folder.Addon.ID, []string{"."})
}

// unitOfTitle is a unit of a folder, the IMDb identifier it is linked to,
// empty for none, and whether files of it are matched.
type unitOfTitle struct {
	folder  accounts.ID
	unit    string
	link    string
	matched bool
}

// titleUnits lists, in the order of their keys, the units of the movies or
// shows folders (kind) matched to the title of a Stremio identifier, or
// the unit of the plain video it is.
func (s *Service) titleUnits(ctx context.Context, kind library.Kind, stremioID string) ([]unitOfTitle, error) {
	folderKind := KindMovies
	switch kind {
	case library.KindSeries:
		folderKind = KindShows
	case library.KindMovie:
	default:
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT f.addon_id, f.unit, coalesce(min(k.imdb_id), ''), bool_or(f.stremio_id IS NOT NULL)
		FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		LEFT JOIN local_links k ON k.addon_id = f.addon_id AND k.unit = f.unit
		WHERE l.kind = $2 AND (f.stremio_id = $1 OR f.plain_id = $1)
		GROUP BY f.addon_id, f.unit ORDER BY f.addon_id, f.unit`, stremioID, folderKind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (unitOfTitle, error) {
		var u unitOfTitle
		err := row.Scan(&u.folder, &u.unit, &u.link, &u.matched)
		return u, err
	})
}

// LinkMatched links every unit of the movies or shows folders (kind)
// matched to the title of a Stremio identifier, by a scan or by hand, to
// an IMDb identifier, as Link does from the folder's page: each of the
// movie's files or discs, or each of the show's folders, in every folder
// of kind, as they hold the same title. The title each unit leaves is
// remembered (see Relinked), and what users did with it follows its files
// (see rememberMatched). It answers addons.ErrNotFound when no unit
// is matched to that title, ErrInvalidIMDb for an identifier that is not
// IMDb's.
func (s *Service) LinkMatched(ctx context.Context, kind library.Kind, stremioID, imdb string) error {
	imdb = strings.ToLower(strings.TrimSpace(imdb))
	if !imdbPattern.MatchString(imdb) {
		return ErrInvalidIMDb
	}
	units, err := s.titleUnits(ctx, kind, stremioID)
	if err != nil {
		return err
	}
	var folders []accounts.ID
	var paths []string
	for _, u := range units {
		if u.matched {
			folders, paths = append(folders, u.folder), append(paths, u.unit)
		}
	}
	if len(folders) == 0 {
		return addons.ErrNotFound
	}
	if stremioID != imdb {
		// In the order of their keys, as titleUnits lists them.
		if _, err := s.db.Exec(ctx, `INSERT INTO local_relinked_units (addon_id, unit, former)
			SELECT folder, unit, $3 FROM unnest($1::uuid[], $2::text[]) AS u (folder, unit)
			ON CONFLICT (addon_id, unit) DO UPDATE SET former = excluded.former`, folders, paths, stremioID); err != nil {
			return err
		}
		if err := s.rememberMatched(ctx, kind, stremioID); err != nil {
			return err
		}
	}
	for i, folder := range folders {
		if err := s.Link(ctx, folder, paths[i], imdb); err != nil {
			return err
		}
	}
	return nil
}

// rememberMatched keeps the items the files of the movies or shows folders
// (kind) matched to the title of stremioID are the titles of, as their
// former titles: what users did with them follows the files to the title
// they are matched to next (see follow), as Jellyfin keeps an item's user
// data when it is identified again.
func (s *Service) rememberMatched(ctx context.Context, kind library.Kind, stremioID string) error {
	folderKind := KindMovies
	if kind == library.KindSeries {
		folderKind = KindShows
	}
	rows, err := s.db.Query(ctx, `SELECT f.addon_id, f.path, f.season, f.episode, f.last_episode
		FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		WHERE f.stremio_id = $1 AND l.kind = $2 ORDER BY f.addon_id, f.path`, stremioID, folderKind)
	if err != nil {
		return err
	}
	var folders, items []accounts.ID
	var paths []string
	var folder accounts.ID
	var rel string
	var season, episode, last *int
	if _, err := pgx.ForEachRow(rows, []any{&folder, &rel, &season, &episode, &last}, func() error {
		for _, item := range titleItems(folderKind, stremioID, season, episode, last) {
			folders, paths, items = append(folders, folder), append(paths, rel), append(items, item.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO local_former_titles (addon_id, path, item_id)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::uuid[]) ON CONFLICT DO NOTHING`, folders, paths, items)
	return err
}

// Relinked tells the Stremio identifier of the title the files of a
// folder relinked by LinkMatched from the title of stremioID are matched
// to now, while none of the folder's files is matched to stremioID; empty
// otherwise, as once a unit's link is removed and its files are matched
// to that title again.
func (s *Service) Relinked(ctx context.Context, folder accounts.ID, stremioID string) (string, error) {
	var now string
	err := s.db.QueryRow(ctx, `SELECT f.stremio_id FROM local_relinked_units r
		JOIN local_files f ON f.addon_id = r.addon_id AND f.unit = r.unit AND f.stremio_id IS NOT NULL
		WHERE r.addon_id = $1 AND r.former = $2
			AND NOT EXISTS (SELECT 1 FROM local_files m WHERE m.addon_id = $1 AND m.stremio_id = $2)
		ORDER BY f.path LIMIT 1`, folder, stremioID).Scan(&now)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return now, err
}

// Rematch matches again, as a scan matches a unit, every unit of the
// movies or shows folders (kind) matched to the title of a Stremio
// identifier, or the unit of the plain video it is: by its link, else by
// its name, NFO file or a search. A unit matched keeps its title when no
// title is found again.
func (s *Service) Rematch(ctx context.Context, kind library.Kind, stremioID string) error {
	units, err := s.titleUnits(ctx, kind, stremioID)
	if err != nil {
		return err
	}
	for _, u := range units {
		folder, err := s.Folder(ctx, u.folder)
		if errors.Is(err, addons.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.matchUnit(ctx, folder, u.unit, u.link, u.matched); err != nil {
			return err
		}
	}
	return nil
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

// Looser is a unit of a folder a looser rule matched (see chooseItem): the
// unit Unlink names, its first file and how many it has, what its name
// tells, the title it was matched to, named and dated as the metadata
// addons give it, and how: byYear, byNumerals or byYearNumerals.
type Looser struct {
	Unit      string
	Path      string
	Files     int
	Title     string
	Year      int
	StremioID string
	Name      string
	TitleYear int
	By        string
}

// Looser lists the units of a folder a looser rule matched, by unit, at
// most maxUnmatched, with how many there are.
func (s *Service) Looser(ctx context.Context, id accounts.ID) ([]Looser, int, error) {
	if _, err := s.Folder(ctx, id); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(DISTINCT unit) FROM local_files WHERE addon_id = $1 AND matched_by = ANY($2)", id,
		looserMethods).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (unit) unit, path, count(*) OVER (PARTITION BY unit), title, coalesce(year, 0), stremio_id,
			name, coalesce(title_year, 0), matched_by
		FROM local_files WHERE addon_id = $1 AND matched_by = ANY($2) ORDER BY unit, path LIMIT $3`, id, looserMethods, maxUnmatched)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Looser, error) {
		var l Looser
		err := row.Scan(&l.Unit, &l.Path, &l.Files, &l.Title, &l.Year, &l.StremioID, &l.Name, &l.TitleYear, &l.By)
		return l, err
	})
	return list, total, err
}

// noteLink records, for a unit linked to an identifier, whether a metadata
// addon of the server described its title as the folder's kind when its
// files were matched to it, m: a link to an identifier none describes,
// such as an episode's typed for its series', is flagged on the folder's
// page. A unit without a link has nothing to record.
func (s *Service) noteLink(ctx context.Context, folder accounts.ID, unit, link string, m match) error {
	if link == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, "UPDATE local_links SET described = $3 WHERE addon_id = $1 AND unit = $2 AND described <> $3",
		folder, unit, !m.undescribed)
	return err
}
