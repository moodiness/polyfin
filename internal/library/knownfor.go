package library

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Titles a person is known for.
//
// While the settings hold the server's TMDB key, a person of a known TMDB
// identifier also lists the movies and series TMDB knows them best for,
// beyond the server's libraries, from their combined credits (see
// personTMDBTitles). Jellyfin lists only the items of its libraries
// crediting a person (Person.cs lines 67-70); Polyfin goes beyond, so that
// someone credited in few titles of the libraries still shows what they
// are known for.
//
// The rule, close to the "Known For" of TMDB's pages:
//   - the titles most voted for on TMDB come first, a lasting measure of how
//     well a title is known; among equal votes, the most popular now
//     (TMDB's popularity), then movies before series and the lower TMDB
//     identifier, so that the order is stable;
//   - talk shows, the news and the person's appearances as themselves (see
//     tmdb.Credit.Appearance), titles for adults, and titles not out yet or
//     of no date are left out;
//   - of the first knownForRanked titles left, at most knownForMost are
//     listed: those the user may open and see. A title the page already
//     lists, of the libraries, counts among them and is not repeated, so
//     that the libraries' titles come first. The titles no catalog listed
//     that Polyfin knows the person in, such as one of these whose
//     description was read, fill in after them, within knownForMost, so
//     that what other users opened does not lengthen the list.
//
// A title is an item of the user's metadata addons: the one Polyfin knows
// under one of its identifiers when the user's view reaches it, else the
// title "tmdb:<id>" when one of the user's addons describes titles of that
// prefix and type, else the title of its IMDb identifier when one
// describes those. A title none of them describes is left out. The new
// ones are recorded as Resolve records titles no catalog listed (see
// unlistedTitle), with TMDB's name, date and poster until the addons
// describe them, which opening them does.
//
// The IMDb identifier of a title is the one credits or box sets learnt for
// it, else the one TMDB's external identifiers give: one request a title,
// in the queue of TMDB's background lookups (see tmdb.Queue), ahead of the
// passes', kept in the database, for good once found and titleIMDbFresh
// when TMDB gives none. A page waits for them knownForWait at most; the
// titles whose identifier comes later are listed the next time.
//
// Parental control, blocked genres, the user's hidden titles and the
// movies waiting for their digital release apply as on every listing: a
// rating not known yet is looked up, and a title whose rating is still
// unknown then is left out for a restricted user until it is known (see
// visible).

const (
	// knownForMost bounds the titles a person is known for, and
	// knownForRanked the titles considered for them, hence the requests
	// for their IMDb identifiers.
	knownForMost   = 15
	knownForRanked = 2 * knownForMost
	// knownForWait bounds how long a person's page waits for the IMDb
	// identifiers of the titles they are known for.
	knownForWait = 3 * time.Second
	// titleIMDbFresh is how long TMDB giving no IMDb identifier for a
	// title is kept before it is asked again.
	titleIMDbFresh = 30 * 24 * time.Hour
	// knownForPosterSize is the size of TMDB's posters of the titles a
	// person is known for, until their addons describe them.
	knownForPosterSize = "original"
)

// tmdbTitle is a movie ("movie") or series ("tv") by its TMDB identifier.
type tmdbTitle struct {
	kind tmdb.Kind
	id   int
}

func (c personCredit) title() tmdbTitle { return tmdbTitle{tmdb.Kind(c.Kind), c.ID} }

// libraryKind is the kind of the items of titles of kind.
func (t tmdbTitle) libraryKind() Kind {
	if t.kind == tmdb.TV {
		return KindSeries
	}
	return KindMovie
}

// knownForRanking returns a person's credits in the order of the titles
// they are known for, those left out removed, the first knownForRanked
// of them.
func knownForRanking(credits []personCredit, now time.Time) []personCredit {
	today := now.UTC().Format(time.DateOnly)
	ranked := slices.DeleteFunc(slices.Clone(credits), func(c personCredit) bool {
		return c.Aside || c.ID <= 0 || c.Kind != string(tmdb.Movie) && c.Kind != string(tmdb.TV) || c.Date == "" || c.Date > today
	})
	slices.SortFunc(ranked, func(a, b personCredit) int {
		return cmp.Or(cmp.Compare(b.Votes, a.Votes), cmp.Compare(b.Popularity, a.Popularity), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	return ranked[:min(len(ranked), knownForRanked)]
}

// knownFor returns the records of the titles a person is known for that
// the user may open and see (see above), best known first, but those of
// listed, the titles their page lists already, which count among them.
// credits are the person's, as personTMDBCredits gives them. unlisted
// are titles no catalog listed that Polyfin knows the person in, as a
// title they are known for whose description was read: they fill in
// after the others, within knownForMost, or all of them without TMDB's
// ranking.
func (s *Service) knownFor(ctx context.Context, v view, credits []personCredit, listed, unlisted []record) []record {
	ranked := knownForRanking(credits, s.now())
	var picked []record
	counted := 0
	if len(ranked) > 0 {
		shown := s.listedTMDB(ctx, v, listed)
		deadline := time.Now().Add(knownForWait)
		for start := 0; start < len(ranked) && counted < knownForMost; {
			var window []personCredit
			for ; start < len(ranked) && counted+len(window) < knownForMost; start++ {
				if shown[ranked[start].title()] {
					counted++
				} else {
					window = append(window, ranked[start])
				}
			}
			var found []record
			for _, r := range s.knownForTitles(ctx, v, window, deadline) {
				switch {
				case r == nil || v.hidden[r.ID] || slices.ContainsFunc(picked, func(p record) bool { return p.ID == r.ID }):
				case slices.ContainsFunc(listed, func(l record) bool { return l.ID == r.ID }):
					counted++
				default:
					found = append(found, *r)
				}
			}
			found, _ = s.visible(ctx, v, found)
			found, _ = s.released(ctx, v, found)
			picked, counted = append(picked, found...), counted+len(found)
		}
	}
	for _, r := range unlisted {
		if len(ranked) > 0 && counted >= knownForMost {
			break
		}
		if !v.hidden[r.ID] && !slices.ContainsFunc(picked, func(p record) bool { return p.ID == r.ID }) {
			picked, counted = append(picked, r), counted+1
		}
	}
	return picked
}

// listedTMDB returns the TMDB titles of the records the user did not
// hide: by the TMDB identifier their listing gives, the one their credits
// were read by, or the one credits, box sets or the titles people are
// known for learnt for their IMDb identifier.
func (s *Service) listedTMDB(ctx context.Context, v view, listed []record) map[tmdbTitle]bool {
	shown := map[tmdbTitle]bool{}
	var ids []accounts.ID
	var imdbs []string
	for _, r := range listed {
		if r.Meta == nil || v.hidden[r.ID] || r.Kind != KindMovie && r.Kind != KindSeries {
			continue
		}
		kind := tmdb.Movie
		if r.Kind == KindSeries {
			kind = tmdb.TV
		}
		ids = append(ids, r.ID)
		provided := providerIDs(*r.Meta)
		if id, err := strconv.Atoi(provided["Tmdb"]); err == nil && id > 0 {
			shown[tmdbTitle{kind, id}] = true
		}
		if imdb := provided["Imdb"]; imdb != "" {
			imdbs = append(imdbs, imdb)
		}
	}
	if len(ids) == 0 {
		return shown
	}
	rows, err := s.db.Query(ctx, `SELECT kind, tmdb_id FROM title_credits WHERE tmdb_id IS NOT NULL AND titles && $1::uuid[]
		UNION SELECT kind, tmdb_id FROM title_imdb_ids WHERE imdb_id = ANY($2::text[])
		UNION SELECT 'movie', tmdb_id FROM movie_collections WHERE provider = 'imdb' AND id = ANY($2::text[]) AND tmdb_id IS NOT NULL
		UNION SELECT kind, tmdb_id FROM title_credits WHERE provider = 'imdb' AND id = ANY($2::text[]) AND tmdb_id IS NOT NULL`, ids, imdbs)
	if err != nil {
		return shown
	}
	learnt, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tmdbTitle, error) {
		var t tmdbTitle
		err := row.Scan(&t.kind, &t.id)
		return t, err
	})
	if err != nil {
		return shown
	}
	for _, t := range learnt {
		shown[t] = true
	}
	return shown
}

// knownForTitles returns the record of the item of each credit (see
// above), nil for a title none of the user's addons describes, or whose
// IMDb identifier, the one it needs, is not known by deadline.
func (s *Service) knownForTitles(ctx context.Context, v view, credits []personCredit, deadline time.Time) []*record {
	result := make([]*record, len(credits))
	if len(credits) == 0 {
		return result
	}
	imdb, err := s.learntIMDb(ctx, credits)
	if err != nil {
		return result
	}
	refs := make([]TitleRef, len(credits))
	for i, c := range credits {
		refs[i] = TitleRef{Series: c.title().kind == tmdb.TV, TMDB: c.ID, IMDb: imdb[c.title()], Name: c.Name}
	}
	reached := s.reachedTitles(ctx, v, refs)
	var asked []int
	for i, c := range credits {
		kind := c.title().libraryKind()
		switch tmdbID := "tmdb:" + strconv.Itoa(c.ID); {
		case reached[i] != nil:
			result[i] = reached[i]
		case describable(v, kind, tmdbID):
			result[i] = new(s.knownForTitle(kind, tmdbID, c, refs[i]))
		case refs[i].IMDb != "" && describable(v, kind, refs[i].IMDb):
			result[i] = new(s.knownForTitle(kind, refs[i].IMDb, c, refs[i]))
		case !hasKey(imdb, c.title()) && describable(v, kind, "tt"):
			asked = append(asked, i)
		}
	}
	if len(asked) == 0 || !s.lookups.Enabled() {
		return result
	}
	lookups := make([]tmdb.Lookup, 0, len(asked))
	keys := make([]string, 0, len(asked))
	waiting := make([]personCredit, 0, len(asked))
	for _, i := range asked {
		t := credits[i].title()
		lookup := tmdb.Lookup{Key: "imdb|" + string(t.kind) + "|" + strconv.Itoa(t.id), Run: func(ctx context.Context) error { return s.lookUpIMDb(ctx, t) }}
		lookups, keys, waiting = append(lookups, lookup), append(keys, lookup.Key), append(waiting, credits[i])
	}
	s.lookups.Add(lookups, true)
	s.lookups.Wait(ctx, keys, time.Until(deadline))
	if imdb, err = s.learntIMDb(ctx, waiting); err != nil {
		return result
	}
	for _, i := range asked {
		c := credits[i]
		kind := c.title().libraryKind()
		if id := imdb[c.title()]; id != "" && describable(v, kind, id) {
			ref := refs[i]
			ref.IMDb = id
			result[i] = new(s.knownForTitle(kind, id, c, ref))
		}
	}
	return result
}

func hasKey[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
}

// describable reports whether one of the user's metadata addons describes
// the movie or series (kind) of the Stremio identifier id, Polyfin's own
// sources aside, which describe their own titles only.
func describable(v view, kind Kind, id string) bool {
	return slices.ContainsFunc(describers(v, record{Meta: &stremio.Meta{Type: string(kind), ID: id}}, false),
		func(entry installed) bool { return !own(entry.addon) })
}

// reachedTitles returns, for each ref, the record of the title Polyfin
// knows under one of its identifiers that the user's view reaches, nil
// when there is none.
func (s *Service) reachedTitles(ctx context.Context, v view, refs []TitleRef) []*record {
	result := make([]*record, len(refs))
	titles, err := s.knownTitles(ctx, refs)
	if err != nil || len(titles) == 0 {
		return result
	}
	ids := make([]accounts.ID, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, itemID(titleKey(t.kind, t.stremioID)))
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return result
	}
	for i, ref := range refs {
		for _, t := range titles {
			if t.kind != ref.kind() || !t.matches(ref) {
				continue
			}
			at := slices.IndexFunc(records, func(r record) bool { return r.ID == itemID(titleKey(t.kind, t.stremioID)) })
			if at >= 0 && records[at].Meta != nil && v.reaches(records[at]) {
				result[i] = &records[at]
				break
			}
		}
	}
	return result
}

// knownForTitle is the record of the title a person is known for that
// none was kept of: the movie or series (kind) of the Stremio identifier
// id, named, dated and with a poster as TMDB credits it (c) until its
// addons describe it.
func (s *Service) knownForTitle(kind Kind, id string, c personCredit, ref TitleRef) record {
	r := unlistedTitle(kind, id, ref)
	r.Meta.Released = c.Date
	if len(c.Date) >= 4 {
		r.Meta.ReleaseInfo = stremio.Text(c.Date[:4])
	}
	if c.Poster != "" {
		r.Meta.Poster = s.tmdb.ImageURL(c.Poster, knownForPosterSize)
	}
	return r
}

// learntIMDb returns the IMDb identifiers known for the titles of
// credits: those credits or box sets learnt, else those TMDB's external
// identifiers gave. A title TMDB gave none for, not long ago, maps to an
// empty one; a title not asked yet, or long ago, is missing.
func (s *Service) learntIMDb(ctx context.Context, credits []personCredit) (map[tmdbTitle]string, error) {
	var movies, series []int
	for _, c := range credits {
		if c.title().kind == tmdb.TV {
			series = append(series, c.ID)
		} else {
			movies = append(movies, c.ID)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT kind, tmdb_id, COALESCE(imdb_id, '') FROM title_imdb_ids
			WHERE (kind = 'movie' AND tmdb_id = ANY($1::int[]) OR kind = 'tv' AND tmdb_id = ANY($2::int[]))
				AND (imdb_id IS NOT NULL OR asked_at > $3)
		UNION ALL SELECT kind, tmdb_id, id FROM title_credits
			WHERE provider = 'imdb' AND (kind = 'movie' AND tmdb_id = ANY($1::int[]) OR kind = 'tv' AND tmdb_id = ANY($2::int[]))
		UNION ALL SELECT 'movie', tmdb_id, id FROM movie_collections WHERE provider = 'imdb' AND tmdb_id = ANY($1::int[])`,
		movies, series, s.now().Add(-titleIMDbFresh))
	if err != nil {
		return nil, err
	}
	// An identifier found wins over TMDB giving none.
	known := map[tmdbTitle]string{}
	var t tmdbTitle
	var imdb string
	_, err = pgx.ForEachRow(rows, []any{&t.kind, &t.id, &imdb}, func() error {
		if imdb != "" || !hasKey(known, t) {
			known[t] = imdb
		}
		return nil
	})
	return known, err
}

// lookUpIMDb asks TMDB for the IMDb identifier of a title, unless it is
// known (see learntIMDb), and keeps the answer. A lookup that fails keeps
// nothing, and fails with tmdb.RateLimited when TMDB refused it for its
// rate.
func (s *Service) lookUpIMDb(ctx context.Context, t tmdbTitle) error {
	if !s.tmdb.Enabled() {
		return nil
	}
	known, err := s.learntIMDb(ctx, []personCredit{{Kind: string(t.kind), ID: t.id}})
	if err != nil || hasKey(known, t) {
		return err
	}
	imdb, err := s.tmdb.IMDbID(ctx, t.kind, t.id)
	var id *string
	switch {
	case err == nil && imdbID.MatchString(imdb):
		id = &imdb
	case err != nil && !errors.Is(err, tmdb.ErrNotFound):
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO title_imdb_ids (kind, tmdb_id, imdb_id, asked_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (kind, tmdb_id) DO UPDATE SET imdb_id = excluded.imdb_id, asked_at = excluded.asked_at`,
		string(t.kind), t.id, id, s.now())
	return err
}
