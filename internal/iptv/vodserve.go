package iptv

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// vodPage is how many titles a catalog page lists.
	vodPage = 100
	// detailTTL is how long details are kept before they are asked again,
	// and maxDetailSize bounds a details answer.
	detailTTL     = 7 * 24 * time.Hour
	maxDetailSize = 16 << 20
	// DetailGap is the least time between two details requests to one
	// provider host: providers ban accounts that send bursts.
	DetailGap = time.Second
	// otherCategory names the catalog of titles without category.
	otherCategory = "Other"
)

type openingKey struct{}

// Opening marks ctx as a request opening a title, such as an app's item
// details: only such requests ask an Xtream server for a title's details
// (and enrichment asks metadata addons), listings never do.
func Opening(ctx context.Context) context.Context { return context.WithValue(ctx, openingKey{}, true) }

// IsOpening reports whether ctx opens a title (see Opening).
func IsOpening(ctx context.Context) bool {
	opening, _ := ctx.Value(openingKey{}).(bool)
	return opening
}

// typeCatalogID is the identifier of a source's catalog of every title of
// a type, and categoryCatalogID that of one category's.
func typeCatalogID(typ string) string {
	if typ == typeMovie {
		return "movies"
	}
	return "series"
}

func categoryCatalogID(typ, category string) string { return typ + "." + shortHash(category) }

// searchCatalogID is the identifier of the catalog searches use when a
// source's libraries are by category.
func searchCatalogID(typ string) string { return typeCatalogID(typ) + ".search" }

// titleID is the Stremio identifier of a source's title, and episodeID
// that of an episode of one of its series.
func titleID(source accounts.ID, typ, key string) string {
	if typ == typeMovie {
		return prefix(source) + "vod:" + key
	}
	return prefix(source) + "series:" + key
}

func episodeID(source accounts.ID, series, key string) string {
	return prefix(source) + "ep:" + series + ":" + key
}

// vodCategory is a category of a source's titles.
type vodCategory struct{ typ, name string }

// vodCategories lists the categories of a source's titles, by type, in
// the provider's order.
func vodCategories(ctx context.Context, db queryer, source accounts.ID) ([]vodCategory, error) {
	rows, err := db.Query(ctx, `SELECT type, category FROM iptv_titles WHERE addon_id = $1 GROUP BY type, category ORDER BY type, min(position)`, source)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (vodCategory, error) {
		var c vodCategory
		err := row.Scan(&c.typ, &c.name)
		return c, err
	})
}

// manifest describes a source as an addon: its live TV catalog, and its
// movie and series catalogs, one per type or one per category, as its
// options say; it answers the metas and streams of its identifiers.
func manifest(source accounts.ID, kind, name string, o Options, categories []vodCategory) stremio.Manifest {
	description := "M3U playlist"
	if kind == addons.KindXtream {
		description = "Xtream Codes account"
	}
	var types []string
	var catalogs []stremio.Catalog
	if o.LiveTv {
		types = append(types, "tv")
		catalogs = append(catalogs, stremio.Catalog{Type: "tv", ID: catalogID, Name: name})
	}
	for _, part := range []struct {
		on  bool
		typ string
	}{{o.Movies, typeMovie}, {o.Series, typeSeries}} {
		if !part.on {
			continue
		}
		types = append(types, part.typ)
		var names []string
		for _, c := range categories {
			if c.typ == part.typ && !slices.Contains(o.VODExcluded, part.typ+":"+c.name) {
				names = append(names, c.name)
			}
		}
		if o.VODLibraries == LibrariesByType {
			extra := []stremio.Extra{{Name: "search"}}
			if genres := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == "" }); len(genres) > 0 {
				extra = append(extra, stremio.Extra{Name: "genre", Options: genres})
			}
			catalogs = append(catalogs, stremio.Catalog{Type: part.typ, ID: typeCatalogID(part.typ), Name: name, Extra: append(extra, stremio.Extra{Name: "skip"})})
			continue
		}
		for _, category := range names {
			catalogs = append(catalogs, stremio.Catalog{Type: part.typ, ID: categoryCatalogID(part.typ, category), Name: cmp.Or(category, otherCategory),
				Extra: []stremio.Extra{{Name: "skip"}}})
		}
		// A search catalog, which libraries do not list, searches every
		// category at once.
		catalogs = append(catalogs, stremio.Catalog{Type: part.typ, ID: searchCatalogID(part.typ), Name: name,
			Extra: []stremio.Extra{{Name: "search", IsRequired: true}, {Name: "skip"}}})
	}
	only := []string{prefix(source)}
	return stremio.Manifest{
		ID: "polyfin.iptv." + source.String(), Version: "1", Name: name, Description: description, Types: types,
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: types, IDPrefixes: only},
			{Name: "stream", Types: types, IDPrefixes: only}},
		Catalogs:   catalogs,
		IDPrefixes: only,
	}
}

// renamed is a source's manifest under a new name: the source's own
// catalogs, named after it, follow.
func renamed(m stremio.Manifest, name string) stremio.Manifest {
	m.Name = name
	m.Catalogs = slices.Clone(m.Catalogs)
	for i, c := range m.Catalogs {
		if c.ID == catalogID || c.ID == typeCatalogID(c.Type) || c.ID == searchCatalogID(c.Type) {
			m.Catalogs[i].Name = name
		}
	}
	return m
}

// syncManifest makes a source's manifest, in tx, follow its options and
// titles; the libraries of its catalogs follow (see addons.SyncCatalogs).
func syncManifest(ctx context.Context, tx pgx.Tx, source accounts.ID, kind, name string, o Options) error {
	categories, err := vodCategories(ctx, tx, source)
	if err != nil {
		return err
	}
	return addons.SyncCatalogs(ctx, tx, source, manifest(source, kind, name, o, categories))
}

// shownTitle selects, for t joined with its source as i, the titles a
// source shows: of a type turned on, in a category not excluded.
const shownTitle = `(t.type = 'movie' AND i.movies OR t.type = 'series' AND i.series) AND NOT (t.type || ':' || t.category = ANY(i.vod_excluded))`

const titleColumns = `t.type, t.key, t.name, t.category, t.poster, coalesce(t.year, 0), t.rating, t.tmdb, t.imdb, t.listing, t.version, t.extension,
	t.quality, t.url, t.headers`

// storedTitle is a title as stored, with its source's address.
type storedTitle struct {
	Title
	address *string
	// kind, manifestAddress and manifest are the source's kind, address
	// (holding its credentials) and name; confined keeps its requests on
	// public addresses.
	kind, manifestAddress, manifest string
	confined                        bool
}

// scanTitle reads a title of titleColumns, then extra columns.
func scanTitle(row pgx.Row, t *storedTitle, extra ...any) error {
	err := row.Scan(append([]any{&t.Type, &t.Key, &t.Name, &t.Category, &t.Poster, &t.Year, &t.Rating, &t.TMDB, &t.IMDb, &t.Listing, &t.Version,
		&t.Extension, &t.Quality, &t.address, &t.Headers}, extra...)...)
	if t.address != nil {
		t.URL = *t.address
	}
	return err
}

// listingMeta describes a title from list data.
func listingMeta(source accounts.ID, t Title) stremio.Meta {
	meta := stremio.Meta{ID: titleID(source, t.Type, t.Key), Type: t.Type, Name: t.Name, Poster: t.Poster, ImdbRating: stremio.Text(t.Rating),
		ImdbID: t.IMDb, TmdbID: stremio.Text(t.TMDB), Description: t.Listing.Plot, Background: t.Listing.Background,
		Cast: t.Listing.Cast, Director: t.Listing.Director}
	if t.Year > 0 {
		meta.ReleaseInfo, meta.Year = stremio.Text(strconv.Itoa(t.Year)), stremio.Text(strconv.Itoa(t.Year))
	}
	if t.Category != "" {
		meta.Genres = stremio.Names{t.Category}
	}
	for _, genre := range t.Listing.Genres {
		if !slices.Contains(meta.Genres, genre) {
			meta.Genres = append(meta.Genres, genre)
		}
	}
	meta.Released = releaseDate(t.Listing.Released)
	if trailer := youTubeID(t.Listing.Trailer); trailer != "" {
		meta.Trailers = []stremio.Trailer{{Source: trailer, Type: "Trailer"}}
	}
	return meta
}

// releaseDate writes a provider date as Stremio does, "" when it is not
// one.
func releaseDate(date string) string {
	date = strings.TrimSpace(date)
	if len(date) >= 10 {
		if at, err := time.Parse("2006-01-02", date[:10]); err == nil {
			return at.Format("2006-01-02T15:04:05.000Z")
		}
	}
	return ""
}

// youTubeID reads a trailer the provider gives as a YouTube identifier or
// address.
func youTubeID(trailer string) string {
	trailer = strings.TrimSpace(trailer)
	if parsed, err := url.Parse(trailer); err == nil && parsed.Host != "" {
		if id := parsed.Query().Get("v"); id != "" {
			trailer = id
		} else {
			trailer = strings.Trim(parsed.Path, "/")
		}
	}
	if len(trailer) < 6 || len(trailer) > 20 || strings.ContainsFunc(trailer, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) {
		return ""
	}
	return trailer
}

// Catalog lists a page of one of a source's movie or series catalogs, from
// list data only: from skip, the titles of the catalog's category, or of
// genre (a category) in a catalog of a type, or those whose name holds
// search, newest first, then in the provider's order.
func (s *Service) Catalog(ctx context.Context, source accounts.ID, catalogType, catalog string, skip int, genre, search string) ([]stremio.Meta, error) {
	metas, _, err := s.catalogWindow(ctx, source, catalogType, catalog, genre, search, skip, vodPage, false)
	return metas, err
}

// CatalogWindow lists the titles [start, start+count) of one of a source's
// movie or series catalogs, as Catalog orders them, and how many there
// are: a stored list is paged and counted whole, unlike an addon's
// catalog.
func (s *Service) CatalogWindow(ctx context.Context, source accounts.ID, catalogType, catalog, genre, search string, start, count int) ([]stremio.Meta, int, error) {
	return s.catalogWindow(ctx, source, catalogType, catalog, genre, search, start, count, true)
}

func (s *Service) catalogWindow(ctx context.Context, source accounts.ID, catalogType, catalog, genre, search string, skip, limit int,
	counted bool) ([]stremio.Meta, int, error) {
	if catalogType != typeMovie && catalogType != typeSeries {
		return nil, 0, nil
	}
	filter, args := "", []any{source, catalogType}
	switch catalog {
	case typeCatalogID(catalogType), searchCatalogID(catalogType):
		if genre != "" {
			args = append(args, genre)
			filter += " AND t.category = $" + strconv.Itoa(len(args))
		}
	default:
		hash, ok := strings.CutPrefix(catalog, catalogType+".")
		if !ok {
			return nil, 0, nil
		}
		args = append(args, hash)
		filter += " AND left(encode(sha256(convert_to(t.category, 'UTF8')), 'hex'), 16) = $" + strconv.Itoa(len(args))
	}
	order := "t.added DESC NULLS LAST, t.position"
	if search = strings.TrimSpace(search); search != "" {
		args = append(args, likePattern(search), strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(Fold(search))+"%")
		filter += " AND t.search LIKE $" + strconv.Itoa(len(args)-1)
		order = "t.search LIKE $" + strconv.Itoa(len(args)) + " DESC, " + order
	} else if catalog == searchCatalogID(catalogType) {
		return nil, 0, nil
	}
	from := ` FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id WHERE t.addon_id = $1 AND t.type = $2 AND ` + shownTitle + filter
	total := 0
	if counted {
		countArgs := args
		if search != "" {
			countArgs = args[:len(args)-1]
		}
		if err := s.db.QueryRow(ctx, "SELECT count(*)"+from, countArgs...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	rows, err := s.db.Query(ctx, `SELECT `+titleColumns+from+` ORDER BY `+order+fmt.Sprintf(" LIMIT %d OFFSET %d", max(limit, 0), max(skip, 0)), args...)
	if err != nil {
		return nil, 0, err
	}
	metas, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Meta, error) {
		var t storedTitle
		err := scanTitle(row, &t)
		return listingMeta(source, t.Title), err
	})
	if metas == nil {
		metas = []stremio.Meta{}
	}
	return metas, total, err
}

// title finds a title a source shows, with its source's address and name.
func (s *Service) title(ctx context.Context, source accounts.ID, typ, key string) (storedTitle, error) {
	var t storedTitle
	err := scanTitle(s.db.QueryRow(ctx, `SELECT `+titleColumns+`, a.kind, a.manifest_url, a.manifest->>'name',
		a.owner_id IS NOT NULL AND NOT coalesce(u.is_administrator, false)
		FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id JOIN addons a ON a.id = t.addon_id LEFT JOIN users u ON u.id = a.owner_id
		WHERE t.addon_id = $1 AND t.type = $2 AND t.key = $3 AND `+shownTitle, source, typ, key), &t, &t.kind, &t.manifestAddress, &t.manifest, &t.confined)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedTitle{}, stremio.ErrNotFound
	}
	return t, err
}

// titleMeta describes a title a source shows: its list data, completed by
// its details when they are known or the request opens it (see Opening),
// and, for a series, its episodes.
func (s *Service) titleMeta(ctx context.Context, source accounts.ID, typ, key string) (stremio.Meta, error) {
	t, err := s.title(ctx, source, typ, key)
	if err != nil {
		return stremio.Meta{}, err
	}
	meta := listingMeta(source, t.Title)
	d, err := s.details(ctx, source, t, IsOpening(ctx))
	if err != nil {
		return stremio.Meta{}, err
	}
	if d != nil {
		d.apply(source, t.Title, &meta)
	}
	if typ == typeSeries && t.kind != addons.KindXtream {
		if meta.Videos, err = s.m3uEpisodes(ctx, source, key); err != nil {
			return stremio.Meta{}, err
		}
	}
	return meta, nil
}

// m3uEpisodes lists an M3U series' episodes.
func (s *Service) m3uEpisodes(ctx context.Context, source accounts.ID, series string) ([]stremio.Video, error) {
	rows, err := s.db.Query(ctx, `SELECT key, season, episode, name FROM iptv_episodes WHERE addon_id = $1 AND series_key = $2
		ORDER BY season, episode, position`, source, series)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Video, error) {
		var key, name string
		var season, episode int
		if err := row.Scan(&key, &season, &episode, &name); err != nil {
			return stremio.Video{}, err
		}
		return stremio.Video{ID: episodeID(source, series, key), Title: episodeTitle(name), Season: stremio.Number(season),
			Episode: stremio.Number(episode)}, nil
	})
}

// episodeTitle is what an episode's name says after its numbers, without
// quality markers; "" when nothing is left.
func episodeTitle(name string) string {
	for _, pattern := range episodePatterns {
		if match := pattern.FindStringIndex(name); match != nil {
			name = name[match[1]:]
			break
		}
	}
	for _, q := range qualities {
		name = q.pattern.ReplaceAllString(name, " ")
	}
	return strings.Trim(strings.Join(strings.Fields(name), " "), " ._-–—:|()[]")
}

// details are what an Xtream server tells of a title when asked for it:
// a movie's or a series' description, and a series' seasons and episodes.
type details struct {
	Plot       string            `json:"plot,omitempty"`
	Cast       []string          `json:"cast,omitempty"`
	Director   []string          `json:"director,omitempty"`
	Genres     []string          `json:"genres,omitempty"`
	Released   string            `json:"released,omitempty"`
	Runtime    int               `json:"runtime,omitempty"`
	Poster     string            `json:"poster,omitempty"`
	Background string            `json:"background,omitempty"`
	Trailer    string            `json:"trailer,omitempty"`
	TMDB       string            `json:"tmdb,omitempty"`
	IMDb       string            `json:"imdb,omitempty"`
	Rating     string            `json:"rating,omitempty"`
	Extension  string            `json:"extension,omitempty"`
	Seasons    map[string]string `json:"seasons,omitempty"`
	Episodes   []episodeDetails  `json:"episodes,omitempty"`
}

type episodeDetails struct {
	Key       string `json:"key"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Title     string `json:"title,omitempty"`
	Plot      string `json:"plot,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Released  string `json:"released,omitempty"`
	Runtime   int    `json:"runtime,omitempty"`
	Extension string `json:"extension,omitempty"`
	Quality   string `json:"quality,omitempty"`
}

// apply completes a title's meta with its details.
func (d *details) apply(source accounts.ID, t Title, meta *stremio.Meta) {
	meta.Description = cmp.Or(d.Plot, meta.Description)
	if len(d.Cast) > 0 {
		meta.Cast = d.Cast
	}
	if len(d.Director) > 0 {
		meta.Director = d.Director
	}
	for _, genre := range d.Genres {
		if !slices.Contains(meta.Genres, genre) {
			meta.Genres = append(meta.Genres, genre)
		}
	}
	if released := releaseDate(d.Released); released != "" {
		meta.Released = released
		if meta.ReleaseInfo == "" {
			meta.ReleaseInfo = stremio.Text(d.Released[:4])
		}
	}
	if d.Runtime > 0 {
		meta.Runtime = stremio.Text(strconv.Itoa(d.Runtime) + " min")
	}
	meta.Poster, meta.Background = cmp.Or(d.Poster, meta.Poster), cmp.Or(d.Background, meta.Background)
	if trailer := youTubeID(d.Trailer); trailer != "" {
		meta.Trailers = []stremio.Trailer{{Source: trailer, Type: "Trailer"}}
	}
	meta.TmdbID, meta.ImdbID = cmp.Or(stremio.Text(d.TMDB), meta.TmdbID), cmp.Or(d.IMDb, meta.ImdbID)
	meta.ImdbRating = cmp.Or(stremio.Text(d.Rating), meta.ImdbRating)
	if len(d.Seasons) > 0 {
		meta.Extras = &stremio.Extras{SeasonPosterByNumber: d.Seasons}
	}
	if t.Type == typeSeries {
		meta.Videos = make([]stremio.Video, 0, len(d.Episodes))
		for _, e := range d.Episodes {
			video := stremio.Video{ID: episodeID(source, t.Key, e.Key), Title: e.Title, Season: stremio.Number(e.Season),
				Episode: stremio.Number(e.Episode), Thumbnail: e.Thumbnail, Overview: e.Plot, Released: releaseDate(e.Released)}
			if e.Runtime > 0 {
				video.Runtime = stremio.Text(strconv.Itoa(e.Runtime) + " min")
			}
			meta.Videos = append(meta.Videos, video)
		}
	}
}

// details returns a title's details: those kept while fresh; else, when
// fetch is set and its source is an Xtream account, asked again, paced
// per host (see DetailGap); else those kept, even old, or nil. A failed
// request is logged and answers what was kept.
func (s *Service) details(ctx context.Context, source accounts.ID, t storedTitle, fetch bool) (*details, error) {
	var kept *details
	var version string
	var at time.Time
	err := s.db.QueryRow(ctx, "SELECT details, version, fetched_at FROM iptv_details WHERE addon_id = $1 AND type = $2 AND key = $3",
		source, t.Type, t.Key).Scan(&kept, &version, &at)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	fresh := kept != nil && version == t.Version && s.now().Sub(at) < detailTTL
	if fresh || !fetch || t.kind != addons.KindXtream {
		return kept, nil
	}
	result, err, _ := s.flight.Do("details "+source.String()+" "+t.Type+" "+t.Key, func() (any, error) {
		account := accountOf(t.kind, t.manifestAddress)
		action := "get_vod_info&vod_id=" + url.QueryEscape(t.Key)
		if t.Type == typeSeries {
			action = "get_series_info&series_id=" + url.QueryEscape(t.Key)
		}
		if err := s.pace(ctx, account.Server); err != nil {
			return nil, err
		}
		s.detailRequests.Add(1)
		var fetched *details
		err := download(ctx, s.client, account.api(action), t.confined, maxDetailSize, func(body io.Reader) error {
			var err error
			if t.Type == typeSeries {
				fetched, err = parseSeriesInfo(body)
			} else {
				fetched, err = parseVODInfo(body)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		_, err = s.db.Exec(ctx, `INSERT INTO iptv_details (addon_id, type, key, version, fetched_at, details) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (addon_id, type, key) DO UPDATE SET version = excluded.version, fetched_at = excluded.fetched_at, details = excluded.details`,
			source, t.Type, t.Key, t.Version, s.now(), fetched)
		return fetched, err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) {
			return nil, err
		}
		s.logger.Warn("An IPTV title's details could not be fetched", "source", t.manifest, "type", t.Type, "error", err)
		return kept, nil
	}
	return result.(*details), nil
}

// pace waits for host's turn to answer a details request, at most one
// every detailGap, and takes it.
func (s *Service) pace(ctx context.Context, host string) error {
	s.paceMu.Lock()
	now := time.Now()
	turn := now
	if next := s.nextDetail[host]; next.After(now) {
		turn = next
	}
	s.nextDetail[host] = turn.Add(s.detailGap)
	s.paceMu.Unlock()
	if wait := turn.Sub(now); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// xtreamInfo is the "info" of a get_vod_info or get_series_info answer.
type xtreamInfo struct {
	TMDB         text            `json:"tmdb_id"`
	TMDB2        text            `json:"tmdb"`
	IMDB         text            `json:"imdb_id"`
	CoverBig     string          `json:"cover_big"`
	MovieImage   string          `json:"movie_image"`
	Cover        string          `json:"cover"`
	Released     string          `json:"releasedate"`
	Released2    string          `json:"releaseDate"`
	Released3    string          `json:"release_date"`
	Trailer      string          `json:"youtube_trailer"`
	Director     string          `json:"director"`
	Actors       string          `json:"actors"`
	Cast         string          `json:"cast"`
	Description  string          `json:"description"`
	Plot         string          `json:"plot"`
	Genre        string          `json:"genre"`
	Backdrop     json.RawMessage `json:"backdrop_path"`
	DurationSecs text            `json:"duration_secs"`
	Duration     string          `json:"duration"`
	Rating       text            `json:"rating"`
}

// minutes reads a duration the provider gives in seconds or as hh:mm:ss.
func minutes(seconds text, clock string) int {
	if n, err := strconv.Atoi(string(seconds)); err == nil && n > 0 {
		return (n + 30) / 60
	}
	parts := strings.Split(strings.TrimSpace(clock), ":")
	if len(parts) == 3 {
		h, e1 := strconv.Atoi(parts[0])
		m, e2 := strconv.Atoi(parts[1])
		if e1 == nil && e2 == nil && h >= 0 && m >= 0 && h*60+m > 0 {
			return h*60 + m
		}
	}
	return 0
}

func (info xtreamInfo) details() *details {
	return &details{Plot: strings.TrimSpace(cmp.Or(info.Plot, info.Description)), Cast: splitNames(cmp.Or(info.Cast, info.Actors)),
		Director: splitNames(info.Director), Genres: splitNames(info.Genre), Released: cmp.Or(info.Released, info.Released2, info.Released3),
		Runtime: minutes(info.DurationSecs, info.Duration), Poster: webImage(cmp.Or(info.CoverBig, info.MovieImage, info.Cover)),
		Background: firstImage(info.Backdrop), Trailer: strings.TrimSpace(info.Trailer), TMDB: numeric(cmp.Or(string(info.TMDB), string(info.TMDB2))),
		IMDb: imdbID(string(info.IMDB)), Rating: ratingText(string(info.Rating))}
}

// object decodes raw into into when it is a JSON object: servers send an
// empty array for what they do not know.
func object(raw json.RawMessage, into any) {
	if trimmed := strings.TrimSpace(string(raw)); strings.HasPrefix(trimmed, "{") {
		_ = json.Unmarshal(raw, into)
	}
}

// parseVODInfo reads a get_vod_info answer.
func parseVODInfo(body io.Reader) (*details, error) {
	var answer struct {
		Info  json.RawMessage `json:"info"`
		Movie json.RawMessage `json:"movie_data"`
	}
	if err := json.NewDecoder(body).Decode(&answer); err != nil {
		return nil, jsonError(err)
	}
	var info xtreamInfo
	object(answer.Info, &info)
	var movie struct {
		Extension string `json:"container_extension"`
	}
	object(answer.Movie, &movie)
	d := info.details()
	d.Extension = strings.Trim(strings.TrimSpace(movie.Extension), ".")
	return d, nil
}

// parseSeriesInfo reads a get_series_info answer: the series' info, its
// seasons' posters and its episodes, which servers group by season in an
// object or an array.
func parseSeriesInfo(body io.Reader) (*details, error) {
	var answer struct {
		Info     json.RawMessage `json:"info"`
		Seasons  json.RawMessage `json:"seasons"`
		Episodes json.RawMessage `json:"episodes"`
	}
	if err := json.NewDecoder(body).Decode(&answer); err != nil {
		return nil, jsonError(err)
	}
	var info xtreamInfo
	object(answer.Info, &info)
	d := info.details()
	var seasons []struct {
		Number   text   `json:"season_number"`
		Cover    string `json:"cover"`
		CoverBig string `json:"cover_big"`
	}
	if json.Unmarshal(answer.Seasons, &seasons) == nil {
		for _, season := range seasons {
			if poster := webImage(cmp.Or(season.CoverBig, season.Cover)); poster != "" && numeric(string(season.Number)) != "" {
				if d.Seasons == nil {
					d.Seasons = map[string]string{}
				}
				d.Seasons[string(season.Number)] = poster
			}
		}
	}
	type episode struct {
		ID        text            `json:"id"`
		Number    text            `json:"episode_num"`
		Title     string          `json:"title"`
		Extension string          `json:"container_extension"`
		Season    text            `json:"season"`
		Info      json.RawMessage `json:"info"`
	}
	var groups [][]episode
	var bySeason map[string][]episode
	if json.Unmarshal(answer.Episodes, &bySeason) == nil {
		keys := make([]string, 0, len(bySeason))
		for key := range bySeason {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b string) int {
			x, _ := strconv.Atoi(a)
			y, _ := strconv.Atoi(b)
			return cmp.Or(cmp.Compare(x, y), strings.Compare(a, b))
		})
		for _, key := range keys {
			list := bySeason[key]
			for i := range list {
				if list[i].Season == "" {
					list[i].Season = text(key)
				}
			}
			groups = append(groups, list)
		}
	} else {
		_ = json.Unmarshal(answer.Episodes, &groups)
	}
	for _, list := range groups {
		for _, e := range list {
			if e.ID == "" {
				continue
			}
			var ei struct {
				MovieImage   string `json:"movie_image"`
				Plot         string `json:"plot"`
				DurationSecs text   `json:"duration_secs"`
				Duration     string `json:"duration"`
				Released     string `json:"releasedate"`
				AirDate      string `json:"air_date"`
			}
			object(e.Info, &ei)
			season, _ := strconv.Atoi(string(e.Season))
			number, _ := strconv.Atoi(string(e.Number))
			quality, _ := streamQuality(e.Title)
			d.Episodes = append(d.Episodes, episodeDetails{Key: string(e.ID), Season: max(season, 0), Episode: max(number, 0), Title: episodeTitle(e.Title),
				Plot: strings.TrimSpace(ei.Plot), Thumbnail: webImage(ei.MovieImage), Released: cmp.Or(ei.Released, ei.AirDate),
				Runtime: minutes(ei.DurationSecs, ei.Duration), Extension: strings.Trim(strings.TrimSpace(e.Extension), "."), Quality: quality})
		}
	}
	return d, nil
}

// vodURL is the address of an Xtream movie's or episode's file.
func (a Account) vodURL(kind, id, extension string) string {
	return a.Server + "/" + kind + "/" + url.PathEscape(a.Username) + "/" + url.PathEscape(a.Password) + "/" + url.PathEscape(id) + "." +
		url.PathEscape(cmp.Or(extension, "mp4"))
}

// vodStream is a movie's or episode's one stream, named after its source
// and the quality its name gives.
func vodStream(sourceName, quality, address string, headers map[string]string) stremio.Stream {
	stream := stremio.Stream{Name: sourceName, Description: quality, URL: address}
	if len(headers) > 0 {
		stream.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: headers}
	}
	return stream
}

// vodStreams lists the stream of a movie or an episode a source shows: an
// Xtream account's file, whose episode needs its series' details (asked
// when unknown: playing opens it), or an M3U entry's address.
func (s *Service) vodStreams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error) {
	if key, ok := strings.CutPrefix(id, "vod:"); ok {
		t, err := s.title(ctx, source, typeMovie, key)
		if errors.Is(err, stremio.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if t.kind != addons.KindXtream {
			return []stremio.Stream{vodStream(t.manifest, t.Quality, t.URL, t.Headers)}, nil
		}
		extension := t.Extension
		if extension == "" {
			if d, err := s.details(ctx, source, t, true); err == nil && d != nil {
				extension = d.Extension
			}
		}
		return []stremio.Stream{vodStream(t.manifest, t.Quality, accountOf(t.kind, t.manifestAddress).vodURL("movie", key, extension), nil)}, nil
	}
	rest, ok := strings.CutPrefix(id, "ep:")
	series, key, found := strings.Cut(rest, ":")
	if !ok || !found {
		return nil, nil
	}
	t, err := s.title(ctx, source, typeSeries, series)
	if errors.Is(err, stremio.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.kind != addons.KindXtream {
		var address, quality string
		var headers map[string]string
		err := s.db.QueryRow(ctx, "SELECT url, quality, headers FROM iptv_episodes WHERE addon_id = $1 AND series_key = $2 AND key = $3",
			source, series, key).Scan(&address, &quality, &headers)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []stremio.Stream{vodStream(t.manifest, quality, address, headers)}, nil
	}
	d, err := s.details(ctx, source, t, true)
	if err != nil || d == nil {
		return nil, err
	}
	index := slices.IndexFunc(d.Episodes, func(e episodeDetails) bool { return e.Key == key })
	if index < 0 {
		return nil, nil
	}
	e := d.Episodes[index]
	return []stremio.Stream{vodStream(t.manifest, e.Quality, accountOf(t.kind, t.manifestAddress).vodURL("series", key, e.Extension), nil)}, nil
}

// Enrichment reports whether a source describes its titles through the
// server's metadata addons.
func (s *Service) Enrichment(ctx context.Context, source accounts.ID) bool {
	var on bool
	_ = s.db.QueryRow(ctx, "SELECT enrichment FROM iptv_sources WHERE addon_id = $1", source).Scan(&on)
	return on
}

// VODCounts count a source's titles: movies and series in its lists,
// episodes known, those it shows, and the categories of each type.
type VODCounts struct {
	Movies, Series, Episodes          int
	ShownMovies, ShownSeries          int
	MovieCategories, SeriesCategories int
}

func (s *Service) vodCounts(ctx context.Context, source accounts.ID) (VODCounts, error) {
	var c VODCounts
	err := s.db.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE t.type = 'movie'), count(*) FILTER (WHERE t.type = 'series'),
		count(*) FILTER (WHERE t.type = 'movie' AND `+shownTitle+`), count(*) FILTER (WHERE t.type = 'series' AND `+shownTitle+`),
		count(DISTINCT t.category) FILTER (WHERE t.type = 'movie'), count(DISTINCT t.category) FILTER (WHERE t.type = 'series'),
		(SELECT count(*) FROM iptv_episodes WHERE addon_id = $1) +
			(SELECT coalesce(sum(jsonb_array_length(coalesce(details->'episodes', '[]'))), 0) FROM iptv_details WHERE addon_id = $1 AND type = 'series')
		FROM iptv_sources i LEFT JOIN iptv_titles t ON t.addon_id = i.addon_id WHERE i.addon_id = $1`, source).
		Scan(&c.Movies, &c.Series, &c.ShownMovies, &c.ShownSeries, &c.MovieCategories, &c.SeriesCategories, &c.Episodes)
	return c, err
}

// previewTitles groups titles of a type by category, those whose name or
// key holds q, in the provider's order.
func previewTitles(titles []Title, typ, q string, excluded []string) []PreviewCategory {
	var keys []string
	categories := map[string]*PreviewCategory{}
	for _, t := range titles {
		key := typ + ":" + t.Category
		c := categories[key]
		if c == nil {
			c = &PreviewCategory{Key: key, Name: t.Category, Excluded: slices.Contains(excluded, key)}
			categories[key] = c
			keys = append(keys, key)
		}
		c.Channels++
	}
	needle := Fold(strings.TrimSpace(q))
	result := []PreviewCategory{}
	for _, key := range keys {
		c := categories[key]
		if needle == "" || strings.Contains(Fold(c.Name), needle) || strings.Contains(Fold(c.Key), needle) {
			result = append(result, *c)
		}
	}
	return result
}

// titlesOf are the titles of a type a download brought: an Xtream
// account's list, or an M3U playlist's entries of that kind.
func titlesOf(d downloaded, typ string) []Title {
	if typ == typeMovie && d.movies != nil || typ == typeSeries && d.series != nil {
		if typ == typeMovie {
			return d.movies
		}
		return d.series
	}
	kind := KindMovie
	if typ == typeSeries {
		kind = KindEpisode
	}
	var entries []vodEntry
	for i, e := range d.entries {
		if e.Kind == kind {
			entries = append(entries, vodEntry{key: entryKey(e) + "#" + strconv.Itoa(i), name: e.Name, logo: e.Logo, group: e.Group, url: e.URL,
				kind: e.Kind, series: e.Series, season: e.Season, episode: e.Episode})
		}
	}
	if typ == typeMovie {
		return m3uMovies(entries)
	}
	titles, _ := m3uSeries(entries)
	return titles
}

// storedTitles reads a source's titles of a type, for previews.
func storedTitles(ctx context.Context, db queryer, source accounts.ID, typ string) ([]Title, error) {
	rows, err := db.Query(ctx, "SELECT key, category FROM iptv_titles WHERE addon_id = $1 AND type = $2 ORDER BY position", source, typ)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Title, error) {
		t := Title{Type: typ}
		err := row.Scan(&t.Key, &t.Category)
		return t, err
	})
}
