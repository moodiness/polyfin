package library

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Titles' credits from TMDB.
//
// Metadata addons credit what they credit: some only a title's first
// actors, by name, and no crew but its directors and writers. While the
// settings hold the server's TMDB key, a movie or series TMDB knows, by
// its TMDB identifier, else its IMDb one through TMDB's find, takes its
// people from TMDB's credits, as Jellyfin 12.4's TMDB providers do (see
// tmdb.Client.TitleCredits): its first 15 actors in billing order, one
// credit per role of a series' actors, and its first 15 directors,
// writers and producers, each with their TMDB identifier, which keeps the
// person (see creditedID), their role, in the server's metadata language,
// and their photo. The addon's photos fill in the people TMDB has none of.
// The addon's people stay without a key, for a title of no TMDB or IMDb
// identifier, one TMDB does not know or credits no one in, and until its
// credits were read.
//
// The credits are kept in the database, for every user, by the identifier
// the title was looked up with, and read again after titleCreditsFresh;
// saveCredits records them on the people, so that a person lists the
// titles TMDB credits them in. They are read in the background, in the
// queue of TMDB's background lookups (see tmdb.Queue), which respects
// TMDB's rate with the box sets' lookups: a title's page asks for its own,
// waiting titleCreditsWait at most, ahead of the passes, and a pass, once
// a day while a key is saved, asks for those of the first CreditsPassMost
// movies and series of each of the server's libraries, those not read
// yet or read too long ago.
//
// What it costs: one request to TMDB a title, two for a title known by
// its IMDb identifier only, whose TMDB identifier box sets did not learn,
// at most four titles a second with the box sets' lookups. A pass over
// 2,000 titles takes 8 minutes or more; over every title of a server of
// 43,000, 26,000 of them movies, about 3 hours of lookups and up to 84,000
// requests, nearly 5 hours sharing the queue with the movies' box sets.
// Hence the pass's bound, as the box sets' (see boxset.PassMost): a title
// beyond it is read when its page opens.
//
// A person's page also lists the titles of the user's libraries TMDB
// credits them in (see tmdbCredited), from TMDB's combined credits of the
// person, kept personCreditsFresh, matched to the titles Polyfin knows by
// their TMDB identifier, or by the TMDB identifier credits and box sets
// learnt for their IMDb one.

const (
	// titleCreditsFresh is how long a title's credits, or TMDB not knowing
	// it, are kept before TMDB is asked again.
	titleCreditsFresh = 30 * 24 * time.Hour
	// titleCreditsWait bounds how long a title's page waits for its
	// credits: ones not read then show the next time.
	titleCreditsWait = time.Second
	// personCreditsFresh is how long a person's combined credits are kept,
	// and personCreditsWait bounds how long their page waits for them.
	personCreditsFresh = 30 * 24 * time.Hour
	personCreditsWait  = 3 * time.Second
	// creditsPassTime bounds the listing of a pass.
	creditsPassTime = 30 * time.Minute
	// tmdbPhotoSize is the size of TMDB's photos of people, the one of
	// their lookups (see lookUpPerson).
	tmdbPhotoSize = "original"
)

// CreditsPassMost is how many of each library's first titles a pass reads
// the credits of, as many as box sets learn (see boxset.PassMost).
const CreditsPassMost = 2000

// UseTitleCredits sets the queue titles' credits are read in, and titles'
// artwork (see tmdbartwork.go), and starts the credits' passes; it is
// called before the service is used, after UseTMDB. Without it, titles
// show the addons' people and artwork.
func (s *Service) UseTitleCredits(q *tmdb.Queue) {
	s.lookups = q
	q.Pass("title credits", s.creditsPass)
}

// creditsKey identifies a title as TMDB is asked about its credits, and as
// they are kept: by its kind, and the identifier it is asked by, "tmdb" or
// "imdb".
type creditsKey struct {
	kind     tmdb.Kind
	provider string
	id       string
}

// titleCreditsKey is the key of a movie's or series' credits, by its
// identifiers under Jellyfin's names: its TMDB identifier, else its IMDb
// one; false for another kind, or neither identifier.
func titleCreditsKey(kind Kind, ids map[string]string) (creditsKey, bool) {
	var key creditsKey
	switch kind {
	case KindMovie:
		key.kind = tmdb.Movie
	case KindSeries:
		key.kind = tmdb.TV
	default:
		return key, false
	}
	if id := strings.TrimSpace(ids["Tmdb"]); id != "" {
		if n, err := strconv.Atoi(id); err == nil && n > 0 {
			key.provider, key.id = "tmdb", strconv.Itoa(n)
			return key, true
		}
	}
	if id := strings.TrimSpace(ids["Imdb"]); strings.HasPrefix(id, "tt") {
		key.provider, key.id = "imdb", id
		return key, true
	}
	return key, false
}

// lookup is the key of the lookup of a title's credits in the queue.
func (k creditsKey) lookup(title accounts.ID) string {
	return "credits|" + string(k.kind) + "|" + k.provider + "|" + k.id + "|" + title.String()
}

// keptCredits is what is kept of a title's credits: found is false for a
// title TMDB does not know; titles are the items whose people were saved
// from them.
type keptCredits struct {
	found    bool
	language string
	credits  []tmdb.TitleCredit
	titles   []accounts.ID
	asked    time.Time
}

// due reports whether TMDB is asked again about the title for title: its
// credits are old, of another language, or not saved on title's people.
func (k keptCredits) due(now time.Time, language string, title accounts.ID) bool {
	return now.Sub(k.asked) >= titleCreditsFresh || k.found && (k.language != language || !slices.Contains(k.titles, title))
}

// shows reports whether title shows the credits: TMDB credits someone,
// and they were saved on its people.
func (k keptCredits) shows(title accounts.ID) bool {
	return k.found && len(k.credits) > 0 && slices.Contains(k.titles, title)
}

// creditsLanguage is the language roles are asked in: the server's
// metadata language.
func (s *Service) creditsLanguage() string { return s.settings().Language }

// keptTitleCredits reads what is kept of titles' credits, by key.
func (s *Service) keptTitleCredits(ctx context.Context, keys []creditsKey) (map[creditsKey]keptCredits, error) {
	kept := map[creditsKey]keptCredits{}
	if len(keys) == 0 {
		return kept, nil
	}
	kinds, providers, ids := make([]string, len(keys)), make([]string, len(keys)), make([]string, len(keys))
	for i, key := range keys {
		kinds[i], providers[i], ids[i] = string(key.kind), key.provider, key.id
	}
	rows, err := s.db.Query(ctx, `SELECT kind, provider, id, tmdb_id IS NOT NULL, language, credits, titles, asked_at
		FROM title_credits JOIN unnest($1::text[], $2::text[], $3::text[]) AS asked (kind, provider, id) USING (kind, provider, id)`,
		kinds, providers, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key creditsKey
		var k keptCredits
		var kind string
		var credits []byte
		if err := rows.Scan(&kind, &key.provider, &key.id, &k.found, &k.language, &credits, &k.titles, &k.asked); err != nil {
			return nil, err
		}
		key.kind = tmdb.Kind(kind)
		if err := json.Unmarshal(credits, &k.credits); err != nil {
			return nil, err
		}
		kept[key] = k
	}
	return kept, rows.Err()
}

// keptOne reads what is kept of a title's credits.
func (s *Service) keptOne(ctx context.Context, key creditsKey) (keptCredits, bool, error) {
	kept, err := s.keptTitleCredits(ctx, []creditsKey{key})
	k, ok := kept[key]
	return k, ok, err
}

// titlePeople returns the people a movie or series (kind) shows, of its
// identifiers ids: its TMDB credits while a key is saved and they are
// saved on its people, else the addon's people, each under the record
// they are kept as (see resolvedPeople).
func (s *Service) titlePeople(ctx context.Context, title accounts.ID, kind Kind, ids map[string]string, addon []Person) []Person {
	if s.lookups.Enabled() {
		if key, ok := titleCreditsKey(kind, ids); ok {
			if k, ok, err := s.keptOne(ctx, key); err == nil && ok && k.shows(title) {
				return s.tmdbPeople(k.credits, addon)
			}
		}
	}
	return s.resolvedPeople(ctx, addon)
}

// creditedByTMDB reports whether a title's people are its TMDB credits
// (see titlePeople), which its addon's description then does not replace.
func (s *Service) creditedByTMDB(ctx context.Context, title accounts.ID, kind Kind, ids map[string]string) bool {
	if !s.lookups.Enabled() {
		return false
	}
	key, ok := titleCreditsKey(kind, ids)
	if !ok {
		return false
	}
	k, ok, err := s.keptOne(ctx, key)
	return err == nil && ok && k.shows(title)
}

// tmdbPeople are the people of TMDB's credits, in order, each with their
// TMDB photo, else the photo addon gives them, by TMDB identifier or by
// name.
func (s *Service) tmdbPeople(credits []tmdb.TitleCredit, addon []Person) []Person {
	photos := map[string]string{}
	for _, p := range addon {
		if p.Image == "" {
			continue
		}
		if p.TMDB != "" {
			photos["tmdb:"+p.TMDB] = p.Image
		}
		if name := "name:" + strings.ToLower(strings.TrimSpace(p.Name)); photos[name] == "" {
			photos[name] = p.Image
		}
	}
	people := make([]Person, 0, len(credits))
	for _, c := range credits {
		id := ""
		if c.ID > 0 {
			id = strconv.Itoa(c.ID)
		}
		photo := ""
		switch {
		case c.ProfilePath != "":
			photo = s.tmdb.ImageURL(c.ProfilePath, tmdbPhotoSize)
		case id != "" && photos["tmdb:"+id] != "":
			photo = photos["tmdb:"+id]
		default:
			photo = photos["name:"+strings.ToLower(c.Name)]
		}
		people = append(people, Person{ID: creditedID(c.Name, id), Name: c.Name, Role: c.Role, Type: c.Kind, Image: photo, TMDB: id})
	}
	return people
}

// resolvedPeople returns an addon's people under the records their
// credits are kept as (see creditedAs): those the addon gives a TMDB
// identifier under its record while the server's TMDB key is saved, the
// others under the record their name's became, once they were known by
// their TMDB identifier.
func (s *Service) resolvedPeople(ctx context.Context, people []Person) []Person {
	byTMDB := s.tmdb.Enabled()
	people = slices.Clone(people)
	var names []accounts.ID
	for i, p := range people {
		switch {
		case p.TMDB != "" && byTMDB:
			people[i].ID = creditedID(p.Name, p.TMDB)
		default:
			names = append(names, p.ID)
		}
	}
	if len(names) == 0 {
		return people
	}
	records, err := s.loadWhere(ctx, "id = ANY($1::uuid[]) AND kind = 'person' AND data ? 'sameAs'", names)
	if err != nil || len(records) == 0 {
		return people
	}
	moved := make(map[accounts.ID]record, len(records))
	for _, r := range records {
		moved[r.ID] = r
	}
	for i, p := range people {
		if same, ok := moved[p.ID].sameAs(); ok && (p.TMDB == "" || !byTMDB) {
			people[i].ID = same
		}
	}
	return people
}

// AwaitTitleCredits has the credits of a movie or series a user opens read
// on TMDB, ahead of the passes, unless they are kept, waiting
// titleCreditsWait at most. It reports whether the title shows them now,
// which changes its people.
func (s *Service) AwaitTitleCredits(ctx context.Context, item Item) bool {
	if !s.lookups.Enabled() {
		return false
	}
	key, ok := titleCreditsKey(item.Kind, item.ProviderIDs)
	if !ok {
		return false
	}
	k, kept, err := s.keptOne(ctx, key)
	if err != nil || kept && !k.due(s.now(), s.creditsLanguage(), item.ID) {
		return false
	}
	lookup := s.creditsLookup(key, item.ID)
	s.lookups.Add([]tmdb.Lookup{lookup}, true)
	s.lookups.Wait(ctx, []string{lookup.Key}, titleCreditsWait)
	after, ok, err := s.keptOne(ctx, key)
	return err == nil && ok && after.shows(item.ID) && !(kept && after.asked.Equal(k.asked) && k.shows(item.ID))
}

// refreshTitleCredits has the TMDB credits of a movie or series refreshed
// on its own (see RefreshItem) read again, ahead of the passes, as Jellyfin's
// refresh of an item asks its providers again: the kept ones are due, and
// TMDB's answer kept by the client forgotten. They show until the new ones
// come. A library's refresh leaves titles' credits to the passes, which
// read them once 30 days old, rather than queue thousands of lookups ahead
// of the users'.
func (s *Service) refreshTitleCredits(ctx context.Context, title record) error {
	if !s.lookups.Enabled() || title.Meta == nil {
		return nil
	}
	key, ok := titleCreditsKey(title.Kind, providerIDs(*title.Meta))
	if !ok {
		return nil
	}
	var tmdbID *int
	err := s.db.QueryRow(ctx, `UPDATE title_credits SET asked_at = 'epoch' WHERE kind = $1 AND provider = $2 AND id = $3
		RETURNING tmdb_id`, string(key.kind), key.provider, key.id).Scan(&tmdbID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if tmdbID != nil {
		s.tmdb.ForgetTitleCredits(key.kind, strconv.Itoa(*tmdbID))
	}
	s.lookups.Add([]tmdb.Lookup{s.creditsLookup(key, title.ID)}, true)
	return nil
}

// creditsLookup is the lookup of a title's credits for the item title.
func (s *Service) creditsLookup(key creditsKey, title accounts.ID) tmdb.Lookup {
	return tmdb.Lookup{Key: key.lookup(title), Run: func(ctx context.Context) error { return s.lookUpCredits(ctx, key, title) }}
}

// lookUpCredits reads a title's credits from TMDB, unless they are kept
// and fresh, and saves them on the people of the item title. A title
// TMDB does not know is kept as such; a lookup that fails keeps nothing,
// and fails with tmdb.RateLimited when TMDB refused it for its rate.
func (s *Service) lookUpCredits(ctx context.Context, key creditsKey, title accounts.ID) error {
	if !s.tmdb.Enabled() {
		return nil
	}
	language := s.creditsLanguage()
	k, kept, err := s.keptOne(ctx, key)
	if err != nil {
		return err
	}
	if !kept || s.now().Sub(k.asked) >= titleCreditsFresh || k.found && k.language != language {
		if k, err = s.askCredits(ctx, key, language); err != nil {
			return err
		}
	}
	if !k.found {
		return nil
	}
	r, err := s.load(ctx, title)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(k.credits) > 0 {
		addon, err := s.creditedIn(ctx, title)
		if err != nil {
			return err
		}
		if err := s.saveCredits(ctx, title, s.tmdbPeople(k.credits, addon), r.Confined); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE title_credits SET titles = array_append(array_remove(titles, $4::uuid), $4::uuid)
		WHERE kind = $1 AND provider = $2 AND id = $3`, string(key.kind), key.provider, key.id, title)
	return err
}

// askCredits asks TMDB for a title's credits, through its TMDB identifier,
// found by its IMDb one when that is the key, and keeps the answer.
func (s *Service) askCredits(ctx context.Context, key creditsKey, language string) (keptCredits, error) {
	id := key.id
	var err error
	if key.provider == "imdb" {
		if id = s.learntTMDB(ctx, key); id == "" {
			id, err = s.tmdb.FindByIMDb(ctx, key.kind, key.id)
		}
	}
	var credits []tmdb.TitleCredit
	if err == nil {
		credits, err = s.tmdb.TitleCredits(ctx, key.kind, id, language)
	}
	found := err == nil
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return keptCredits{}, err
	}
	var tmdbID *int
	if n, convErr := strconv.Atoi(id); found && convErr == nil && n > 0 {
		tmdbID = &n
	}
	if credits == nil {
		credits = []tmdb.TitleCredit{}
	}
	encoded, err := json.Marshal(credits)
	if err != nil {
		return keptCredits{}, err
	}
	now := s.now()
	var titles []accounts.ID
	if err := s.db.QueryRow(ctx, `INSERT INTO title_credits (kind, provider, id, tmdb_id, language, credits, asked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (kind, provider, id) DO UPDATE SET tmdb_id = excluded.tmdb_id, language = excluded.language,
			credits = excluded.credits, asked_at = excluded.asked_at,
			titles = CASE WHEN title_credits.tmdb_id IS NOT DISTINCT FROM excluded.tmdb_id THEN title_credits.titles ELSE '{}' END
		RETURNING titles`,
		string(key.kind), key.provider, key.id, tmdbID, language, encoded, now).Scan(&titles); err != nil {
		return keptCredits{}, err
	}
	return keptCredits{found: found, language: language, credits: credits, titles: titles, asked: now}, nil
}

// learntTMDB is the TMDB identifier box sets or credits learnt for a title
// known by its IMDb identifier, empty when none did.
func (s *Service) learntTMDB(ctx context.Context, key creditsKey) string {
	var id int
	err := s.db.QueryRow(ctx, `SELECT tmdb_id FROM title_credits WHERE kind = $1 AND provider = 'imdb' AND id = $2 AND tmdb_id IS NOT NULL
		UNION ALL SELECT tmdb_id FROM movie_collections WHERE $1 = 'movie' AND provider = 'imdb' AND id = $2 AND tmdb_id IS NOT NULL
		LIMIT 1`, string(key.kind), key.id).Scan(&id)
	if err != nil {
		return ""
	}
	return strconv.Itoa(id)
}

// creditedIn returns the people credited in a title now, with their
// photos: the addon's, which fill in those TMDB has none of.
func (s *Service) creditedIn(ctx context.Context, title accounts.ID) ([]Person, error) {
	records, err := s.loadWhere(ctx, "kind = 'person' AND data->'credits' ? $1", title.String())
	if err != nil {
		return nil, err
	}
	people := make([]Person, 0, len(records))
	for _, r := range records {
		if r.Person != nil {
			people = append(people, Person{Name: r.Person.Name, Image: r.Person.Image, TMDB: r.TMDBID})
		}
	}
	return people, nil
}

// creditsPass queues the lookups of the credits of the first
// CreditsPassMost movies and series of each of the server's libraries,
// those not kept, kept too long, or not saved on the title's people.
func (s *Service) creditsPass(ctx context.Context) error {
	listing, cancel := context.WithTimeout(ctx, creditsPassTime)
	defer cancel()
	titles, libraries, read, err := s.libraryTitles(listing, CreditsPassMost, func(collection string) bool {
		return collection == "movies" || collection == "tvshows"
	}, func(kind Kind) bool { return kind == KindMovie || kind == KindSeries })
	if err != nil {
		return err
	}
	if libraries == 0 {
		s.logger.Info("No library of movies or series: the pass of titles' credits reads none; a title's are read when its page opens")
		return nil
	}
	language, now := s.creditsLanguage(), s.now()
	queued, known := 0, 0
	// Kept credits are read a thousand titles at a time.
	for start := 0; start < len(titles); start += 1000 {
		batch := titles[start:min(start+1000, len(titles))]
		keys := make([]creditsKey, len(batch))
		valid := make([]bool, len(batch))
		for i, title := range batch {
			keys[i], valid[i] = titleCreditsKey(title.Kind, title.ProviderIDs)
		}
		kept, err := s.keptTitleCredits(ctx, keys)
		if err != nil {
			return err
		}
		var lookups []tmdb.Lookup
		for i, title := range batch {
			if !valid[i] {
				continue
			}
			if k, ok := kept[keys[i]]; !ok || k.due(now, language, title.ID) {
				lookups = append(lookups, s.creditsLookup(keys[i], title.ID))
			} else {
				known++
			}
		}
		queued += len(lookups)
		s.lookups.Add(lookups, false)
	}
	s.logger.Info("The pass of titles' credits listed the libraries", "libraries", libraries, "read", read,
		"titles", len(titles), "queued", queued, "known", known)
	return nil
}

// Filmographies from TMDB.

// tmdbCredited lists the movies and series TMDB credits a person in that
// Polyfin knows, described or not, the user's view reaches and their
// parental control lets them see, but the movies waiting for their
// digital release, as credited does: while a key is saved, for a person
// of a known TMDB identifier. A title matches a credit by its TMDB
// identifier, or by the one credits or box sets learnt for its IMDb one.
// Titles of no library are not listed, as Jellyfin lists only the items
// of its libraries crediting a person (Person.cs lines 67-70).
func (s *Service) tmdbCredited(ctx context.Context, v view, person record) []record {
	if !s.tmdb.Enabled() {
		return nil
	}
	id, err := strconv.Atoi(person.identity())
	if err != nil || id <= 0 {
		return nil
	}
	credits, err := s.personTMDBTitles(ctx, id)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("A person's titles could not be read from TMDB", "person", person.ID.String(), "error", err)
		}
		return nil
	}
	var ids []accounts.ID
	var movies, series []int
	for _, c := range credits {
		kind := KindMovie
		if c.MediaType == "tv" {
			kind = KindSeries
			series = append(series, c.ID)
		} else {
			movies = append(movies, c.ID)
		}
		ids = append(ids, itemID(titleKey(kind, "tmdb:"+strconv.Itoa(c.ID))))
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT unnest(titles) FROM title_credits
		WHERE kind = 'movie' AND tmdb_id = ANY($1::int[]) OR kind = 'tv' AND tmdb_id = ANY($2::int[])`, movies, series)
	if err != nil {
		return nil
	}
	mapped, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil
	}
	ids = append(ids, mapped...)
	rows, err = s.db.Query(ctx, `SELECT id FROM movie_collections WHERE provider = 'imdb' AND tmdb_id = ANY($1::int[])`, movies)
	if err != nil {
		return nil
	}
	imdb, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil
	}
	for _, id := range imdb {
		ids = append(ids, itemID(titleKey(KindMovie, id)))
	}
	slices.SortFunc(ids, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
	titles, err := s.loadAll(ctx, slices.Compact(ids))
	if err != nil {
		return nil
	}
	titles = slices.DeleteFunc(titles, func(title record) bool {
		return title.Meta == nil || title.Kind != KindMovie && title.Kind != KindSeries || !v.reaches(title)
	})
	titles, _ = s.visible(ctx, v, titles)
	titles, _ = s.released(ctx, v, titles)
	return titles
}

// personTMDBTitles returns the movies and series TMDB credits the person
// it identifies as id in: those kept, else TMDB's, waiting
// personCreditsWait at most, then kept.
func (s *Service) personTMDBTitles(ctx context.Context, id int) ([]tmdb.Credit, error) {
	var found bool
	var data []byte
	var asked time.Time
	err := s.db.QueryRow(ctx, `SELECT found, titles, asked_at FROM person_credits WHERE tmdb_id = $1`, id).Scan(&found, &data, &asked)
	switch {
	case err == nil && s.now().Sub(asked) < personCreditsFresh:
		var titles []personCredit
		if err := json.Unmarshal(data, &titles); err != nil {
			return nil, err
		}
		credits := make([]tmdb.Credit, 0, len(titles))
		for _, t := range titles {
			credits = append(credits, tmdb.Credit{ID: t.ID, MediaType: t.Kind})
		}
		return credits, nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	asking, cancel := context.WithTimeout(ctx, personCreditsWait)
	defer cancel()
	credits, err := s.tmdb.PersonTitles(asking, id)
	found = err == nil
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return nil, err
	}
	titles := make([]personCredit, 0, len(credits))
	for _, c := range credits {
		titles = append(titles, personCredit{Kind: c.MediaType, ID: c.ID})
	}
	encoded, err := json.Marshal(titles)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO person_credits (tmdb_id, found, titles, asked_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tmdb_id) DO UPDATE SET found = excluded.found, titles = excluded.titles, asked_at = excluded.asked_at`,
		id, found, encoded, s.now())
	return credits, err
}

// personCredit is a title a person's combined credits name, as kept: a
// movie or a series ("tv"), by its TMDB identifier.
type personCredit struct {
	Kind string `json:"kind"`
	ID   int    `json:"id"`
}
