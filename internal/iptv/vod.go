package iptv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Movies and series (VOD). A source imports the provider's movies and
// series when its options say so: an Xtream account's VOD and series
// lists, or the entries of an M3U playlist its addresses or names tell
// are movies or episodes. They are stored as titles, listed by the
// source's movie and series catalogs from list data only; an Xtream
// title's details are asked when it is opened or played (see details).

// Title types, and the kinds of M3U entries that are not live channels.
const (
	typeMovie   = "movie"
	typeSeries  = "series"
	KindMovie   = "movie"
	KindEpisode = "episode"
	kindLive    = "live"
)

// Title is a movie or a series of a provider's list: its key, the
// provider's identifier, which its Stremio and Jellyfin identifiers are
// made of; its name without the list's dressing, and the year that came
// with it; its category; the quality its name gives; an Xtream movie's
// container; an M3U movie's stream; the external identifiers the list
// gives; what an Xtream series' listing says beyond that; and what tells
// its details changed.
type Title struct {
	Type, Key, Name, Category, Poster string
	Year                              int
	Rating                            string
	Added                             *time.Time
	Extension, Quality                string
	TMDB, IMDb                        string
	URL                               string
	Headers                           map[string]string
	Listing                           listing
	Version                           string
}

// listing is what an Xtream series list says of a series besides its
// name and poster.
type listing struct {
	Plot       string   `json:"plot,omitempty"`
	Cast       []string `json:"cast,omitempty"`
	Director   []string `json:"director,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	Released   string   `json:"released,omitempty"`
	Background string   `json:"background,omitempty"`
	Trailer    string   `json:"trailer,omitempty"`
}

// episodeEntry is an episode of an M3U series.
type episodeEntry struct {
	series, key     string
	season, episode int
	name, quality   string
	url             string
	headers         map[string]string
}

var (
	// languagePrefix is the language or country code lists write before
	// titles, as in "EN - Name", "|EN| Name" or "[MULTI] Name".
	languagePrefix = regexp.MustCompile(`^\s*(?:\|\|?\s*[A-Z]{2,5}\s*\|\|?|\[\s*[A-Z]{2,5}\s*\]|[A-Z]{2,3}\s*(?:-|–|—|:|\|))\s*`)
	trailingYear   = regexp.MustCompile(`\s*(?:[\(\[]\s*((?:18|19|20)\d{2})\s*[\)\]]|[-–—]\s*((?:18|19|20)\d{2}))\s*$`)
	// episodePatterns find an episode's season and episode numbers in its
	// name, and the series name before them.
	episodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)[\s._\-\[(]*\bS(\d{1,3})\s*[._\-]?\s*E[Pp]?\s*(\d{1,4})\b`),
		regexp.MustCompile(`(?i)[\s._\-\[(]*\b(\d{1,2})x(\d{1,4})\b`),
		regexp.MustCompile(`(?i)[\s._\-\[(]*\b(?:Season|Saison|Staffel|Temporada|Stagione)\s*(\d{1,3})\s*[,._\-]*\s*(?:Episode|[ÉE]pisode|Folge|Episodio|Ep\.?)\s*(\d{1,4})\b`),
	}
	// providerID is the identifier Xtream-style addresses end with.
	providerID    = regexp.MustCompile(`/(?:movie|movies|series)/[^/]+/[^/]+/([0-9A-Za-z_-]+)\.[0-9A-Za-z]{2,5}$`)
	vodExtensions = map[string]bool{"mp4": true, "mkv": true, "avi": true, "mov": true, "m4v": true, "wmv": true, "flv": true, "webm": true,
		"mpg": true, "mpeg": true, "divx": true, "3gp": true, "ogv": true, "vob": true}
)

// classify tells what an M3U entry is: a movie or an episode by its
// address (a /movie/ or /series/ path) or file extension, an episode when
// its name or path says so, a live channel otherwise. An episode's name
// gives its series, season and episode.
func (e *Entry) classify() {
	address := e.URL
	if parsed, err := url.Parse(address); err == nil {
		address = parsed.Path
	}
	lower := strings.ToLower(address)
	extension := strings.TrimPrefix(path.Ext(lower), ".")
	byPath := ""
	switch {
	case strings.Contains(lower, "/series/"):
		byPath = KindEpisode
	case strings.Contains(lower, "/movie/"), strings.Contains(lower, "/movies/"):
		byPath = KindMovie
	}
	if byPath == "" && !vodExtensions[extension] {
		return
	}
	for _, pattern := range episodePatterns {
		if match := pattern.FindStringSubmatchIndex(e.Name); match != nil {
			series := strings.Trim(e.Name[:match[0]], " ._-–—:|([")
			if series == "" {
				continue
			}
			e.Kind, e.Series = KindEpisode, series
			e.Season, _ = strconv.Atoi(e.Name[match[2]:match[3]])
			e.Episode, _ = strconv.Atoi(e.Name[match[4]:match[5]])
			return
		}
	}
	if byPath == KindEpisode {
		// An episode whose name gives no numbers is numbered by its place.
		e.Kind, e.Series = KindEpisode, e.Name
		return
	}
	e.Kind = KindMovie
}

// titleName is a provider title name shown: without its language or
// country prefix, quality markers and tags, and the year written after it,
// which it returns.
func titleName(raw string) (string, int) {
	base := extraTags.ReplaceAllString(raw, " ")
	for _, q := range qualities {
		base = q.pattern.ReplaceAllString(base, " ")
	}
	base = strings.Join(strings.Fields(base), " ")
	year := 0
	if match := trailingYear.FindStringSubmatchIndex(base); match != nil && match[0] > 0 {
		group := 2
		if match[2] < 0 {
			group = 4
		}
		year, _ = strconv.Atoi(base[match[group]:match[group+1]])
		base = base[:match[0]]
	}
	if stripped := languagePrefix.ReplaceAllString(base, ""); stripped != "" {
		base = stripped
	}
	return cleanName(base), year
}

// ratingText keeps a provider rating out of ten, "" for none.
func ratingText(rating string) string {
	value, err := strconv.ParseFloat(strings.TrimSpace(rating), 64)
	if err != nil || value <= 0 || value > 10 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// numeric keeps an identifier made of digits, "" otherwise.
func numeric(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.Trim(id, "0123456789") != "" || strings.Trim(id, "0") == "" {
		return ""
	}
	return id
}

// imdbID keeps an IMDb identifier, "" otherwise.
func imdbID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 2 && strings.HasPrefix(id, "tt") && numeric(id[2:]) != "" {
		return id
	}
	return ""
}

// yearOf is the year a date starts with, 0 for none.
func yearOf(date string) int {
	date = strings.TrimSpace(date)
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil || year < 1800 || year > 3000 {
		return 0
	}
	return year
}

// splitNames splits a provider's list of names, separated by commas or
// slashes.
func splitNames(text string) []string {
	var names []string
	for _, name := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '/' || r == '|' }) {
		if name = strings.TrimSpace(name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// firstImage reads an image address the provider sends alone or in an
// array.
func firstImage(raw json.RawMessage) string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return webImage(one)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, image := range many {
			if image = webImage(image); image != "" {
				return image
			}
		}
	}
	return ""
}

func webImage(address string) string {
	address = strings.TrimSpace(address)
	if !webAddress(address) {
		return ""
	}
	return address
}

// shortHash is the first 16 hex digits of text's SHA-256.
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// vodKey identifies an M3U movie or episode: the identifier its address
// ends with, else its entry key.
func vodKey(address, fallback string) string {
	if parsed, err := url.Parse(address); err == nil {
		if match := providerID.FindStringSubmatch(parsed.Path); match != nil {
			return match[1]
		}
	}
	return fallback
}

// vodEntry is an M3U entry stored as a movie or an episode.
type vodEntry struct {
	key, name, logo, group, url string
	kind, series                string
	season, episode             int
	headers                     map[string]string
}

// loadVODEntries reads the stored entries of an M3U source of kind.
func loadVODEntries(ctx context.Context, db queryer, source accounts.ID, kind string) ([]vodEntry, error) {
	rows, err := db.Query(ctx, `SELECT key, name, logo, group_title, url, kind, series_name, coalesce(season, 0), coalesce(episode, 0), headers
		FROM iptv_entries WHERE addon_id = $1 AND kind = $2 ORDER BY position`, source, kind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (vodEntry, error) {
		var e vodEntry
		err := row.Scan(&e.key, &e.name, &e.logo, &e.group, &e.url, &e.kind, &e.series, &e.season, &e.episode, &e.headers)
		return e, err
	})
}

// m3uMovies makes movies of an M3U source's movie entries.
func m3uMovies(entries []vodEntry) []Title {
	titles := make([]Title, 0, len(entries))
	seen := map[string]bool{}
	for _, e := range entries {
		key := vodKey(e.url, e.key)
		for n := 2; seen[key]; n++ {
			key = vodKey(e.url, e.key) + "-" + strconv.Itoa(n)
		}
		seen[key] = true
		name, year := titleName(e.name)
		t := Title{Type: typeMovie, Key: key, Name: name, Year: year, Category: e.group, Poster: e.logo, URL: e.url, Headers: e.headers}
		t.Quality, _ = streamQuality(e.name)
		titles = append(titles, t)
	}
	return titles
}

// m3uSeries groups an M3U source's episode entries into series, by their
// series' name once dressing is left out, and numbers the episodes whose
// names gave no number by their place in their season.
func m3uSeries(entries []vodEntry) ([]Title, []episodeEntry) {
	var titles []Title
	var episodes []episodeEntry
	bySeries := map[string]int{}
	seen := map[string]bool{}
	next := map[string]int{}
	for _, e := range entries {
		name, year := titleName(e.series)
		folded := Fold(strings.Join(strings.Fields(name), " "))
		key := shortHash(folded)
		if _, ok := bySeries[key]; !ok {
			bySeries[key] = len(titles)
			titles = append(titles, Title{Type: typeSeries, Key: key, Name: name, Year: year, Category: e.group, Poster: e.logo})
		}
		episodeKey := vodKey(e.url, e.key)
		for n := 2; seen[key+":"+episodeKey]; n++ {
			episodeKey = vodKey(e.url, e.key) + "-" + strconv.Itoa(n)
		}
		seen[key+":"+episodeKey] = true
		season, number := e.season, e.episode
		if season == 0 && number == 0 {
			season = 1
		}
		place := key + ":" + strconv.Itoa(season)
		if number == 0 {
			number = next[place] + 1
		}
		next[place] = max(next[place], number)
		quality, _ := streamQuality(e.name)
		episodes = append(episodes, episodeEntry{series: key, key: episodeKey, season: season, episode: number, name: e.name, quality: quality,
			url: e.url, headers: e.headers})
	}
	return titles, episodes
}

// storeTitles replaces a source's titles of a type, and an M3U source's
// episodes with its series; the details of titles gone are forgotten.
func storeTitles(ctx context.Context, tx pgx.Tx, source accounts.ID, typ string, titles []Title, episodes []episodeEntry) error {
	if _, err := tx.Exec(ctx, "DELETE FROM iptv_titles WHERE addon_id = $1 AND type = $2", source, typ); err != nil {
		return err
	}
	rows := make([][]any, 0, len(titles))
	seen := map[string]bool{}
	for _, t := range titles {
		if seen[t.Key] {
			continue
		}
		seen[t.Key] = true
		var year *int
		if t.Year > 0 {
			year = &t.Year
		}
		var address *string
		if t.URL != "" {
			address = &t.URL
		}
		headers := t.Headers
		if headers == nil {
			headers = map[string]string{}
		}
		rows = append(rows, []any{source, typ, t.Key, len(rows) + 1, t.Name, t.Category, t.Poster, year, t.Rating, t.Added, t.Extension, t.Quality,
			t.TMDB, t.IMDb, address, headers, t.Listing, t.Version, Fold(t.Name)})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"iptv_titles"}, []string{"addon_id", "type", "key", "position", "name", "category", "poster",
		"year", "rating", "added", "extension", "quality", "tmdb", "imdb", "url", "headers", "listing", "version", "search"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	if typ == typeSeries {
		if _, err := tx.Exec(ctx, "DELETE FROM iptv_episodes WHERE addon_id = $1", source); err != nil {
			return err
		}
		episodeRows := make([][]any, 0, len(episodes))
		for _, e := range episodes {
			headers := e.headers
			if headers == nil {
				headers = map[string]string{}
			}
			episodeRows = append(episodeRows, []any{source, e.series, e.key, len(episodeRows) + 1, e.season, e.episode, e.name, e.quality, e.url, headers})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"iptv_episodes"}, []string{"addon_id", "series_key", "key", "position", "season", "episode",
			"name", "quality", "url", "headers"}, pgx.CopyFromRows(episodeRows)); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `DELETE FROM iptv_details d WHERE addon_id = $1 AND type = $2
		AND NOT EXISTS (SELECT 1 FROM iptv_titles t WHERE t.addon_id = d.addon_id AND t.type = d.type AND t.key = d.key)`, source, typ)
	return err
}

// reconcileVOD makes a source's titles follow its options: an M3U
// source's from its stored entries; an Xtream source's of a type turned
// off are dropped, those of a type on having been stored with its list.
func reconcileVOD(ctx context.Context, tx pgx.Tx, source accounts.ID, kind string, o Options) error {
	for _, part := range []struct {
		on        bool
		typ, kind string
	}{{o.Movies, typeMovie, KindMovie}, {o.Series, typeSeries, KindEpisode}} {
		switch {
		case !part.on:
			if err := storeTitles(ctx, tx, source, part.typ, nil, nil); err != nil {
				return err
			}
		case kind != addons.KindXtream:
			entries, err := loadVODEntries(ctx, tx, source, part.kind)
			if err != nil {
				return err
			}
			var titles []Title
			var episodes []episodeEntry
			if part.typ == typeMovie {
				titles = m3uMovies(entries)
			} else {
				titles, episodes = m3uSeries(entries)
			}
			if err := storeTitles(ctx, tx, source, part.typ, titles, episodes); err != nil {
				return err
			}
		}
	}
	return nil
}
