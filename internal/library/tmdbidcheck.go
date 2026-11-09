package library

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Checks of the TMDB identifiers addons give as moviedb_id.
//
// A metadata addon may give a title's TMDB identifier as moviedb_id, which
// Polyfin takes when no _tmdbId or "tmdb:" identifier gives one (see
// stremio.Meta.TmdbFromMoviedb), and which was seen wrong: another title's,
// or one TMDB does not know. While the settings hold the server's TMDB key,
// each such identifier is checked, once and for good, in the queue of
// TMDB's background lookups (see tmdb.Queue):
//   - a title of an IMDb identifier is found on TMDB by it (find): the same
//     identifier of its kind is right; another one is wrong, and replaced
//     by TMDB's. When find has no title of its kind, the external
//     identifiers of the addon's give its IMDb identifier: another one, or
//     TMDB not knowing the title, makes it wrong, and dropped; none keeps
//     it;
//   - a title without one is judged by its year: TMDB not knowing the
//     identifier, or dating its title more than moviedbYearSlack years
//     apart, makes it wrong, and dropped.
//
// A verdict is kept by the kind, identifier and IMDb identifier (or year)
// it judged, so that another identifier an addon gives later is checked
// on its own; one TMDB gave as a replacement is known right. Verdicts are
// applied, with or without a key, where descriptions enter Polyfin, from
// addons and from the database (see checkMetas), and where catalogs make
// titles (see titleItem): every reader of a title's identifiers sees the
// corrected one. An administrator's identifier wins over them (see
// Overrides.identifiers), and a title whose TMDB identifier was set is
// never checked. Identifiers addons give as _tmdbId are taken as they are.
//
// A check is queued when a description of an identifier not checked
// enters, after a request's lookups; a title's page asks for its own ahead
// of the passes, and waits for it with its credits (see
// AwaitTitleCredits). A pass, once a day while a key is saved, queues
// those of the titles stored, and corrects those a verdict found wrong.
// Once a verdict finds one wrong, the titles showing it are corrected:
// their stored previews are rewritten, their kept descriptions forgotten,
// and what was learnt by the former identifier dropped (see
// titleTMDBChanged).
//
// What it costs: one request to TMDB a title of an IMDb identifier, two
// when find has no title of its kind, one a title without; a request's
// answers serve the other lookups asking the same (see tmdb.Client). The
// answers of find are kept as TMDB's IMDb identifiers of the titles found
// (see lookUpIMDb), which the IMDb check of imports reads (see
// ContradictedIMDb).

const (
	// moviedbYearSlack is how many years apart the year of a title without
	// an IMDb identifier and TMDB's of its moviedb_id may be.
	moviedbYearSlack = 1
	// moviedbChecksLoad bounds the reading of the verdicts.
	moviedbChecksLoad = 10 * time.Second
)

// moviedbCheck is a TMDB identifier (id) of kind a description read from
// moviedb_id, as it is checked: with the title's IMDb identifier, else
// with its year, 0 when it has none.
type moviedbCheck struct {
	kind tmdb.Kind
	id   int
	imdb string
	year int
}

// lookup is the key of the check's lookup in the queue.
func (c moviedbCheck) lookup() string {
	return "moviedb|" + string(c.kind) + "|" + strconv.Itoa(c.id) + "|" + c.imdb + "|" + strconv.Itoa(c.year)
}

// moviedbVerdict is what a check found: whether the identifier is wrong,
// and TMDB's identifier of the title that replaces it, 0 when it is
// dropped.
type moviedbVerdict struct {
	wrong bool
	now   int
}

// apply corrects meta's TMDB identifier by the verdict. A replacement
// keeps its mark of moviedb_id, which tells TMDB's numbering of its
// episodes is a guess (see TMDBNumbering).
func (v moviedbVerdict) apply(meta *stremio.Meta) {
	switch {
	case !v.wrong:
	case v.now > 0:
		meta.TmdbID = stremio.Text(strconv.Itoa(v.now))
	default:
		meta.TmdbID, meta.TmdbFromMoviedb = "", false
	}
}

// applied returns ids, a title's provider identifiers, corrected by the
// verdict.
func (v moviedbVerdict) applied(ids map[string]string) map[string]string {
	if !v.wrong {
		return ids
	}
	ids = maps.Clone(ids)
	if v.now > 0 {
		ids["Tmdb"] = strconv.Itoa(v.now)
	} else {
		delete(ids, "Tmdb")
	}
	return ids
}

// moviedbChecks are the verdicts kept, read on first use (see verdicts);
// mu orders their reading and changes.
type moviedbChecks struct {
	mu       sync.Mutex
	verdicts map[moviedbCheck]moviedbVerdict
}

// moviedbCheckOf is the check of the TMDB identifier meta read from
// moviedb_id, metaType being its type when it gives none; false when it
// gives none, or meta is no movie or series.
func moviedbCheckOf(meta *stremio.Meta, metaType string) (moviedbCheck, bool) {
	if !meta.TmdbFromMoviedb {
		return moviedbCheck{}, false
	}
	if meta.Type != "" {
		metaType = meta.Type
	}
	kind, ok := titleKind(metaType)
	if !ok {
		return moviedbCheck{}, false
	}
	year, _ := parseYears(string(meta.ReleaseInfo))
	if year == 0 {
		year, _ = parseYears(string(meta.Year))
	}
	imdb := meta.ImdbID
	if strings.HasPrefix(meta.ID, "tt") {
		imdb, _, _ = strings.Cut(meta.ID, ":")
	}
	return newMoviedbCheck(kind, meta.TmdbID.ID(), imdb, year)
}

// itemMoviedbCheck is the check of the TMDB identifier of a movie or
// series item read from its moviedb_id, as fromMeta filled it; false for
// none.
func itemMoviedbCheck(item Item) (moviedbCheck, bool) {
	if !item.TMDBFromMoviedb {
		return moviedbCheck{}, false
	}
	id, _ := strconv.Atoi(item.ProviderIDs["Tmdb"])
	return newMoviedbCheck(item.Kind, id, item.ProviderIDs["Imdb"], item.ProductionYear)
}

func newMoviedbCheck(kind Kind, id int, imdb string, year int) (moviedbCheck, bool) {
	c := moviedbCheck{id: id}
	switch kind {
	case KindMovie:
		c.kind = tmdb.Movie
	case KindSeries:
		c.kind = tmdb.TV
	default:
		return c, false
	}
	if imdbID.MatchString(imdb) {
		c.imdb = imdb
	} else {
		c.year = max(year, 0)
	}
	return c, id > 0
}

// libraryKind is the kind of the items of the check's titles.
func (c moviedbCheck) libraryKind() Kind {
	return tmdbTitle{kind: c.kind}.libraryKind()
}

// verdictsLocked returns the verdicts kept, read unless they were; nil
// when they cannot be read, until they can. s.moviedbChecks.mu is held.
func (s *Service) verdictsLocked() map[moviedbCheck]moviedbVerdict {
	if s.moviedbChecks.verdicts != nil {
		return s.moviedbChecks.verdicts
	}
	ctx, cancel := context.WithTimeout(context.Background(), moviedbChecksLoad)
	defer cancel()
	rows, err := s.db.Query(ctx, `SELECT kind, tmdb_id, imdb_id, year, wrong, COALESCE(replaced_by, 0) FROM moviedb_id_checks`)
	if err != nil {
		s.logger.Error("The checks of addons' TMDB identifiers could not be read", "error", err)
		return nil
	}
	verdicts := map[moviedbCheck]moviedbVerdict{}
	var c moviedbCheck
	var v moviedbVerdict
	if _, err := pgx.ForEachRow(rows, []any{&c.kind, &c.id, &c.imdb, &c.year, &v.wrong, &v.now}, func() error {
		verdicts[c] = v
		return nil
	}); err != nil {
		s.logger.Error("The checks of addons' TMDB identifiers could not be read", "error", err)
		return nil
	}
	s.moviedbChecks.verdicts = verdicts
	return verdicts
}

// moviedbVerdictOf returns the verdict kept on c, false for none.
func (s *Service) moviedbVerdictOf(c moviedbCheck) (moviedbVerdict, bool) {
	s.moviedbChecks.mu.Lock()
	defer s.moviedbChecks.mu.Unlock()
	v, ok := s.verdictsLocked()[c]
	return v, ok
}

// checkMetas corrects the TMDB identifiers metas read from moviedb_id that
// a check found wrong, metaType being their type when they give none, and,
// when queue is true, queues the checks of those not checked yet (see
// checkedMeta).
func (s *Service) checkMetas(metas []stremio.Meta, metaType string, queue bool) {
	var lookups []tmdb.Lookup
	for i := range metas {
		if lookup, ok := s.checkedMeta(&metas[i], metaType, queue); ok {
			lookups = append(lookups, lookup)
		}
	}
	s.lookups.Add(lookups, false)
}

// checkMeta is checkMetas of one description.
func (s *Service) checkMeta(meta *stremio.Meta, metaType string, queue bool) {
	if lookup, ok := s.checkedMeta(meta, metaType, queue); ok {
		s.lookups.Add([]tmdb.Lookup{lookup}, false)
	}
}

// checkedMeta corrects the TMDB identifier meta read from moviedb_id when
// a check found it wrong, and returns the lookup of its check when it was
// not checked yet and queue is true, while a key is saved, but for a
// title whose TMDB identifier an administrator set.
func (s *Service) checkedMeta(meta *stremio.Meta, metaType string, queue bool) (tmdb.Lookup, bool) {
	c, ok := moviedbCheckOf(meta, metaType)
	if !ok {
		return tmdb.Lookup{}, false
	}
	if v, known := s.moviedbVerdictOf(c); known {
		v.apply(meta)
		return tmdb.Lookup{}, false
	}
	if !queue || !s.lookups.Enabled() || s.overridesNow().fields[itemID(titleKey(c.libraryKind(), meta.ID))].TmdbID != nil {
		return tmdb.Lookup{}, false
	}
	return s.moviedbLookup(c), true
}

// moviedbLookup is the lookup of a check.
func (s *Service) moviedbLookup(c moviedbCheck) tmdb.Lookup {
	return tmdb.Lookup{Key: c.lookup(), Run: func(ctx context.Context) error { return s.checkMoviedb(ctx, c) }}
}

// awaitMoviedbCheck has the TMDB identifier of a movie or series a user
// opens, when it is its moviedb_id, checked ahead of the passes, unless it
// was, waiting d at most. It returns the title's identifiers, corrected by
// the verdict, and whether the verdict changed them.
func (s *Service) awaitMoviedbCheck(ctx context.Context, item Item, d time.Duration) (map[string]string, bool) {
	c, ok := itemMoviedbCheck(item)
	if !ok {
		return item.ProviderIDs, false
	}
	v, known := s.moviedbVerdictOf(c)
	if !known {
		lookup := s.moviedbLookup(c)
		s.lookups.Add([]tmdb.Lookup{lookup}, true)
		s.lookups.Wait(ctx, []string{lookup.Key}, d)
		if v, known = s.moviedbVerdictOf(c); !known {
			return item.ProviderIDs, false
		}
	}
	return v.applied(item.ProviderIDs), v.wrong
}

// checkMoviedb checks a TMDB identifier read from moviedb_id on TMDB,
// unless it was, keeps the verdict, and corrects the titles showing a
// wrong one. A check that fails keeps nothing, and fails with
// tmdb.RateLimited when TMDB refused it for its rate.
func (s *Service) checkMoviedb(ctx context.Context, c moviedbCheck) error {
	if _, known := s.moviedbVerdictOf(c); known || !s.tmdb.Enabled() {
		return nil
	}
	v, err := s.judgeMoviedb(ctx, c)
	if err != nil {
		return err
	}
	return s.keepMoviedbVerdict(ctx, c, v)
}

// judgeMoviedb asks TMDB whether a TMDB identifier read from moviedb_id is
// right (see above).
func (s *Service) judgeMoviedb(ctx context.Context, c moviedbCheck) (moviedbVerdict, error) {
	wrong := moviedbVerdict{wrong: true}
	if c.imdb == "" {
		year, err := s.tmdb.TitleYear(ctx, c.kind, c.id)
		switch {
		case errors.Is(err, tmdb.ErrNotFound):
			return wrong, nil
		case err != nil:
			return moviedbVerdict{}, err
		case c.year > 0 && year > 0 && max(year-c.year, c.year-year) > moviedbYearSlack:
			return wrong, nil
		}
		return moviedbVerdict{}, nil
	}
	found, err := s.tmdb.FoundByIMDb(ctx, c.imdb)
	if err != nil {
		return moviedbVerdict{}, err
	}
	for _, kind := range []tmdb.Kind{tmdb.Movie, tmdb.TV} {
		if id := found.Of(kind); id > 0 {
			if err := s.keepIMDb(ctx, tmdbTitle{kind, id}, c.imdb, true); err != nil {
				return moviedbVerdict{}, err
			}
		}
	}
	switch id := found.Of(c.kind); {
	case id == c.id:
		return moviedbVerdict{}, nil
	case id > 0:
		return moviedbVerdict{wrong: true, now: id}, nil
	}
	imdb, known, err := s.titleIMDb(ctx, tmdbTitle{c.kind, c.id})
	switch {
	case err != nil:
		return moviedbVerdict{}, err
	case !known || imdb != "" && imdb != c.imdb:
		return wrong, nil
	}
	return moviedbVerdict{}, nil
}

// titleIMDb returns TMDB's IMDb identifier of a title, empty for none, as
// kept (see learntIMDb), else asked and kept; known is false when TMDB
// does not know the title.
func (s *Service) titleIMDb(ctx context.Context, t tmdbTitle) (imdb string, known bool, err error) {
	err = s.db.QueryRow(ctx, `SELECT COALESCE(imdb_id, ''), found FROM title_imdb_ids
		WHERE kind = $1 AND tmdb_id = $2 AND (imdb_id IS NOT NULL OR asked_at > $3)`,
		string(t.kind), t.id, s.now().Add(-titleIMDbFresh)).Scan(&imdb, &known)
	if !errors.Is(err, pgx.ErrNoRows) {
		return imdb, known, err
	}
	imdb, err = s.tmdb.IMDbID(ctx, t.kind, t.id)
	known = true
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		imdb, known = "", false
	case err != nil:
		return "", false, err
	case !imdbID.MatchString(imdb):
		imdb = ""
	}
	return imdb, known, s.keepIMDb(ctx, t, imdb, known)
}

// keepMoviedbVerdict keeps the verdict on c, the replacement it gives as
// right, and corrects the titles showing a wrong identifier.
func (s *Service) keepMoviedbVerdict(ctx context.Context, c moviedbCheck, v moviedbVerdict) error {
	var replacedBy *int
	if v.now > 0 {
		replacedBy = &v.now
	}
	now := s.now()
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO moviedb_id_checks (kind, tmdb_id, imdb_id, year, wrong, replaced_by, checked_at) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (kind, tmdb_id, imdb_id, year) DO UPDATE SET wrong = excluded.wrong, replaced_by = excluded.replaced_by, checked_at = excluded.checked_at`,
		string(c.kind), c.id, c.imdb, c.year, v.wrong, replacedBy, now)
	replacement := moviedbCheck{kind: c.kind, id: v.now, imdb: c.imdb}
	if v.now > 0 {
		batch.Queue(`INSERT INTO moviedb_id_checks (kind, tmdb_id, imdb_id, year, wrong, checked_at) VALUES ($1, $2, $3, 0, false, $4)
			ON CONFLICT DO NOTHING`, string(c.kind), v.now, c.imdb, now)
	}
	if err := s.db.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	s.moviedbChecks.mu.Lock()
	if verdicts := s.moviedbChecks.verdicts; verdicts != nil {
		verdicts[c] = v
		if _, known := verdicts[replacement]; v.now > 0 && !known {
			verdicts[replacement] = moviedbVerdict{}
		}
	}
	s.moviedbChecks.mu.Unlock()
	if !v.wrong {
		return nil
	}
	corrected, err := s.correctMoviedb(ctx, c, v)
	s.logger.Debug("A TMDB identifier an addon gave as moviedb_id was wrong", "kind", c.kind, "imdb", c.imdb, "year", c.year,
		"moviedb_id", c.id, "tmdb", v.now, "titles", corrected)
	return err
}

// correctMoviedb corrects the titles showing the identifier c checked,
// which v found wrong: the movies or series of its IMDb identifier, or
// listed with it, whose stored preview or kept description shows it. Their
// previews are rewritten, their descriptions forgotten, to be described
// again (see checkMetas), and what was learnt by the former identifier is
// dropped (see titleTMDBChanged). It returns how many it corrected.
func (s *Service) correctMoviedb(ctx context.Context, c moviedbCheck, v moviedbVerdict) (int, error) {
	kind := c.libraryKind()
	titles, err := s.loadWhere(ctx, `kind IN ('movie', 'series') AND kind = $1
		AND (data->'meta'->>'_tmdbId' = $2 OR key = $3 OR data->'meta'->>'imdb_id' = $4)`,
		string(kind), strconv.Itoa(c.id), titleKey(kind, c.imdb), c.imdb)
	if err != nil || len(titles) == 0 {
		return 0, err
	}
	installed, err := s.addons.All(ctx)
	if err != nil {
		return 0, err
	}
	addonIDs := make([]accounts.ID, len(installed))
	for i, a := range installed {
		addonIDs[i] = a.ID
	}
	corrected := 0
	var errs []error
	for _, r := range titles {
		if r.Meta == nil {
			continue
		}
		got, previewed := moviedbCheckOf(r.Meta, "")
		previewed = previewed && got == c
		// Only the title of the IMDb identifier checked may have a kept
		// description showing it.
		if !previewed && (c.imdb == "" || sourceIDs(*r.Meta)["Imdb"] != c.imdb) {
			continue
		}
		shown, err := s.forgetMoviedb(ctx, r, c, addonIDs)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if previewed {
			shown = r.Meta
		}
		if shown == nil {
			continue
		}
		if previewed {
			// A replacement keeps the mark of moviedb_id (see apply).
			if _, err := s.db.Exec(ctx, `UPDATE items SET updated_at = now(), data = CASE WHEN $3::int > 0
					THEN jsonb_set(data, '{meta,_tmdbId}', to_jsonb($3::int::text))
					ELSE data #- '{meta,_tmdbId}' #- '{meta,_tmdbIdFromMoviedb}' END
				WHERE id = $1 AND data->'meta'->>'_tmdbId' = $2 AND data->'meta'->>'_tmdbIdFromMoviedb' = 'true'`,
				r.ID, strconv.Itoa(c.id), v.now); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		fixed := *shown
		v.apply(&fixed)
		old, now := s.providerIDs(r.ID, *shown), s.providerIDs(r.ID, fixed)
		corrected++
		if old["Tmdb"] != now["Tmdb"] {
			errs = append(errs, s.titleTMDBChanged(ctx, Item{ID: r.ID, Kind: r.Kind, StremioID: r.Meta.ID}, old, now))
		}
	}
	return corrected, errors.Join(errs...)
}

// forgetMoviedb forgets the descriptions of a title that the addons of
// addonIDs gave, kept in memory and in the database (see forgetMeta), and
// returns the first that showed the identifier c checked, nil for none.
func (s *Service) forgetMoviedb(ctx context.Context, title record, c moviedbCheck, addonIDs []accounts.ID) (*stremio.Meta, error) {
	var shown *stremio.Meta
	for _, addon := range addonIDs {
		key := metaKey{addon, title.Meta.Type, title.Meta.ID}
		if kept, ok := s.metas.Get(key); ok && shown == nil {
			if got, ok := moviedbCheckOf(&kept.value, key.metaType); ok && got == c {
				shown = &kept.value
			}
		}
		s.metas.Delete(key)
	}
	rows, err := s.db.Query(ctx, `DELETE FROM metas WHERE addon_id = ANY($1::uuid[]) AND meta_type = $2 AND meta_id = $3 RETURNING meta`,
		addonIDs, title.Meta.Type, title.Meta.ID)
	if err != nil {
		return nil, err
	}
	var data []byte
	_, err = pgx.ForEachRow(rows, []any{&data}, func() error {
		var kept stremio.Meta
		if shown == nil && json.Unmarshal(data, &kept) == nil {
			if got, ok := moviedbCheckOf(&kept, title.Meta.Type); ok && got == c {
				shown = &kept
			}
		}
		return nil
	})
	return shown, err
}

// moviedbChecksPass queues the checks of the TMDB identifiers the titles
// stored took from moviedb_id, those not checked yet, and corrects those
// a verdict found wrong.
func (s *Service) moviedbChecksPass(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT id, jsonb_strip_nulls(jsonb_build_object('id', m->'id', 'type', m->'type',
			'imdb_id', m->'imdb_id', '_tmdbId', m->'_tmdbId', '_tmdbIdFromMoviedb', m->'_tmdbIdFromMoviedb',
			'releaseInfo', m->'releaseInfo', 'year', m->'year'))
		FROM items, LATERAL (SELECT data->'meta' AS m) meta
		WHERE kind IN ('movie', 'series') AND m->>'_tmdbIdFromMoviedb' = 'true'`)
	if err != nil {
		return err
	}
	checks := map[moviedbCheck]bool{}
	var id accounts.ID
	var meta stremio.Meta
	edits := s.overridesNow().fields
	if _, err := pgx.ForEachRow(rows, []any{&id, &meta}, func() error {
		if c, ok := moviedbCheckOf(&meta, ""); ok && edits[id].TmdbID == nil {
			checks[c] = true
		}
		meta = stremio.Meta{}
		return nil
	}); err != nil {
		return err
	}
	var lookups []tmdb.Lookup
	wrong := map[moviedbCheck]moviedbVerdict{}
	s.moviedbChecks.mu.Lock()
	verdicts := s.verdictsLocked()
	for c := range checks {
		switch v, known := verdicts[c]; {
		case !known && verdicts != nil:
			lookups = append(lookups, s.moviedbLookup(c))
		case v.wrong:
			wrong[c] = v
		}
	}
	s.moviedbChecks.mu.Unlock()
	if verdicts == nil {
		return errors.New("the checks of addons' TMDB identifiers could not be read")
	}
	s.lookups.Add(lookups, false)
	corrected := 0
	for c, v := range wrong {
		n, err := s.correctMoviedb(ctx, c, v)
		if err != nil {
			return err
		}
		corrected += n
	}
	s.logger.Info("The pass of the checks of addons' TMDB identifiers listed the titles", "identifiers", len(checks),
		"queued", len(lookups), "corrected", corrected)
	return nil
}
