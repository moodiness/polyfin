package library

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// People's details from TMDB.
//
// Metadata addons give people's names, and some their photos; none gives a
// biography. While the settings hold the server's TMDB key, Polyfin looks
// each person up on TMDB once, as Jellyfin's TmdbPersonProvider does: their
// profile photo, biography, birth and death dates and place of birth, kept
// on the person's record for every user. A person is looked up the first
// time their page is opened, which waits for it a moment, or a title's
// cast shows them without a photo, which does not wait.
//
// The person is TMDB's that the addon names by its identifier, else the
// one TMDB finds by that very name. Of several people of the name, the one
// whose photo the addon gives, when it is TMDB's, else the one credited in
// the titles Polyfin knows the person in, by TMDB identifier or by name and
// year; none when no title tells them apart, rather than someone else of
// the same name.
//
// The addon's photo wins over TMDB's, and an image an administrator chose
// over both (see overridden). A photo found dead (see PhotoFailed) gives
// way to the next one for a day, and the person shows none when it was
// the last, so that apps draw their placeholder. Each biography is asked
// in the language of the user who opens the page, their metadata language
// else the server's, with TMDB's English one when TMDB has none in it.
// Refresh metadata has a person looked up again (see refreshPeople).

const (
	// personLookupWait bounds how long a person's page waits for their
	// lookup: one not done then shows the next time.
	personLookupWait = 3 * time.Second
	// personLookupsAtOnce bounds the people looked up at once, and
	// maxPersonLookups those waiting: past it, the people shown are looked
	// up when they show again.
	personLookupsAtOnce = 2
	maxPersonLookups    = 500
	// personLookupTimeout bounds one person's lookup.
	personLookupTimeout = time.Minute
	// personMissingFor is how long a person TMDB did not find is believed
	// unknown; personRetry the wait after a failed lookup, doubled with each
	// failure up to personRetryMost.
	personMissingFor = 30 * 24 * time.Hour
	personRetry      = 10 * time.Minute
	personRetryMost  = 24 * time.Hour
	// maxPersonCandidates bounds the people of the same name whose credits
	// are read to tell them apart.
	maxPersonCandidates = 5
	// englishBiography is the language of the biography shown when TMDB
	// has none in the user's.
	englishBiography = "en"
	// deadPhotoFor is how long a photo found dead is not shown.
	deadPhotoFor = 24 * time.Hour
	// deadPhotoMarked is how long a photo marked dead is not marked again,
	// which would only move its deadPhotoFor that much later.
	deadPhotoMarked = time.Hour
)

// personDetails is what TMDB told of a person, kept on their record: their
// TMDB identifier, 0 when TMDB found no one (FoundAt telling when), their
// photo's address, birth and death dates ("2006-01-02"), place of birth,
// IMDb identifier, and biographies by the language they were asked in, an
// empty one when TMDB had none. FailedAt and Failures tell the lookups
// that failed since, and Due a lookup asked for by Refresh metadata.
type personDetails struct {
	ID          int               `json:"id,omitempty"`
	FoundAt     *time.Time        `json:"foundAt,omitempty"`
	Photo       string            `json:"photo,omitempty"`
	Birth       string            `json:"birth,omitempty"`
	Death       string            `json:"death,omitempty"`
	Place       string            `json:"place,omitempty"`
	IMDb        string            `json:"imdb,omitempty"`
	Biographies map[string]string `json:"biographies,omitempty"`
	FailedAt    *time.Time        `json:"failedAt,omitempty"`
	Failures    int               `json:"failures,omitempty"`
	Due         bool              `json:"due,omitempty"`
}

// UseTMDB sets what looks people up; it is called before the service is
// used. Without it, or without a key, people show what the addons give.
func (s *Service) UseTMDB(c *tmdb.Client) { s.tmdb = c }

// person loads the record of a person: the one a name's record points to
// once the person is known by their TMDB identifier (see people.go).
func (s *Service) person(ctx context.Context, id accounts.ID) (record, error) {
	r, err := s.load(ctx, id)
	if same, ok := r.sameAs(); err == nil && r.Kind == KindPerson && ok {
		return s.load(ctx, same)
	}
	return r, err
}

// peopleLanguage is the language a user is told people's biographies in:
// their metadata language, else the server's.
func peopleLanguage(user accounts.User, settings accounts.Settings) string {
	return cmp.Or(user.MetadataLanguage, settings.Language, englishBiography)
}

// personPhoto is the photo a person shows at now: the addon's, else
// TMDB's, but for those found dead.
func (r record) personPhoto(now time.Time) string {
	if r.Person == nil {
		return ""
	}
	photos := []string{r.Person.Image}
	if r.TMDB != nil {
		photos = append(photos, r.TMDB.Photo)
	}
	for _, photo := range photos {
		if photo != "" && !r.photoDead(photo, now) {
			return photo
		}
	}
	return ""
}

// photoDead reports whether a photo of the person was found dead less than
// deadPhotoFor before now.
func (r record) photoDead(photo string, now time.Time) bool {
	at, dead := r.DeadPhotos[photo]
	return dead && now.Before(at.Add(deadPhotoFor))
}

// PhotoFailed marks a person's photo dead, as the image relay found it
// (jellyfin's artworkDead): its server does not have it, or failed it
// several times running. For deadPhotoFor, the person shows TMDB's in its
// place, looking it up as for a person without one, else none, unless the
// addon gives a new one. A photo marked dead less than deadPhotoMarked ago
// is left as it is: apps ask for it again and again meanwhile.
func (s *Service) PhotoFailed(ctx context.Context, id accounts.ID, photo string) error {
	now := s.now()
	_, err := s.db.Exec(ctx, `UPDATE items SET data = jsonb_set(data, '{deadPhotos}',
			COALESCE(data->'deadPhotos', '{}'::jsonb) || jsonb_build_object($2::text, $3::timestamptz)), updated_at = now()
		WHERE id = $1 AND kind = 'person' AND COALESCE((data->'deadPhotos'->>$2)::timestamptz <= $4, true)`,
		id, photo, now, now.Add(-deadPhotoMarked))
	return err
}

// personItem describes a person at now, with what TMDB told of them in
// language.
func personItem(id accounts.ID, r record, language string, now time.Time) Item {
	item := Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.personPhoto(now)}}
	d := r.TMDB
	if d == nil || d.ID == 0 {
		return item
	}
	item.ProviderIDs = map[string]string{"Tmdb": strconv.Itoa(d.ID)}
	if d.IMDb != "" {
		item.ProviderIDs["Imdb"] = d.IMDb
	}
	item.Overview = cmp.Or(d.Biographies[language], d.Biographies[englishBiography])
	item.PremiereDate, item.EndDate = personDate(d.Birth), personDate(d.Death)
	if item.PremiereDate != nil {
		item.ProductionYear = item.PremiereDate.Year()
	}
	if d.Place != "" {
		item.ProductionLocations = []string{d.Place}
	}
	return item
}

// personDate reads a date TMDB gives, nil when it gives none.
func personDate(date string) *time.Time {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return nil
	}
	return &parsed
}

// lookupDue reports whether a person is to be looked up for a biography
// in language: a lookup is due anyway (see identityDue), or the person
// TMDB found has no biography asked in language yet.
func (r record) lookupDue(language string, now time.Time) bool {
	if r.identityDue(now) {
		return true
	}
	d := r.TMDB
	if d.FailedAt != nil || d.ID == 0 {
		return false
	}
	_, asked := d.Biographies[language]
	return !asked
}

// identityDue reports whether a person is to be looked up whatever the
// language: never looked up, a failed lookup due again, a lookup Refresh
// metadata asked for, an identifier the addon gives that is not the one
// found, or someone TMDB did not find a while ago.
func (r record) identityDue(now time.Time) bool {
	d := r.TMDB
	switch {
	case d == nil:
		return true
	case d.FailedAt != nil:
		wait := personRetry << min(max(d.Failures-1, 0), 8)
		return !now.Before(d.FailedAt.Add(min(wait, personRetryMost)))
	case d.Due:
		return true
	case d.FoundAt == nil:
		return true
	case r.TMDBID != "" && r.TMDBID != strconv.Itoa(d.ID):
		return true
	case d.ID == 0:
		return !now.Before(d.FoundAt.Add(personMissingFor))
	}
	return false
}

// personLookups are the people being looked up, each by the language of
// its biography; done is closed once the lookup ends. Its zero value is
// ready, but for slots (see New). counts are what the lookups found, as
// PeopleStatus tells it.
type personLookups struct {
	mu      sync.Mutex
	running map[personLookupKey]chan struct{}
	slots   chan struct{}
	wg      sync.WaitGroup
	counts  peopleCounts
}

// peopleCounts are the people TMDB found, did not find, and whose last
// lookup failed, kept between the writes that change them, so that
// Health's poll reads none. Valid tells whether they still hold; changes
// counts those writes (see peopleChanged), so that counts read while one
// ran are not kept.
type peopleCounts struct {
	mu                       sync.Mutex
	valid                    bool
	changes                  uint64
	found, notFound, failing int
}

// peopleChanged tells that a write to people's TMDB details ended, which
// the people counts may no longer match: the next PeopleStatus counts
// them again. It is called once the write is committed, whether it
// succeeded or not.
func (s *Service) peopleChanged() {
	c := &s.personLookups.counts
	c.mu.Lock()
	c.valid = false
	c.changes++
	c.mu.Unlock()
}

type personLookupKey struct {
	person   accounts.ID
	language string
}

// lookUp has a person looked up in the background for a biography in
// language, unless they already are, and returns what is closed once it
// ends; nil when too many people wait.
func (s *Service) lookUp(person accounts.ID, language string) <-chan struct{} {
	l := &s.personLookups
	key := personLookupKey{person: person, language: language}
	l.mu.Lock()
	defer l.mu.Unlock()
	if done, ok := l.running[key]; ok {
		return done
	}
	if len(l.running) >= maxPersonLookups {
		return nil
	}
	if l.running == nil {
		l.running = map[personLookupKey]chan struct{}{}
	}
	done := make(chan struct{})
	l.running[key] = done
	l.wg.Go(func() {
		defer func() {
			l.mu.Lock()
			delete(l.running, key)
			l.mu.Unlock()
			close(done)
		}()
		l.slots <- struct{}{}
		defer func() { <-l.slots }()
		ctx, cancel := context.WithTimeout(context.Background(), personLookupTimeout)
		defer cancel()
		if err := s.lookUpPerson(ctx, person, language); err != nil {
			s.logger.Warn("A person could not be looked up on TMDB", "person", person.String(), "error", err)
		}
	})
	return done
}

// LookUpPerson looks a person up on TMDB for a page of theirs the user
// opens, unless they were already, waiting personLookupWait at most. It
// reports whether a lookup ended meanwhile, which may have changed what
// the person shows.
func (s *Service) LookUpPerson(ctx context.Context, user accounts.User, id accounts.ID) bool {
	if !s.tmdb.Enabled() {
		return false
	}
	r, err := s.person(ctx, id)
	language := peopleLanguage(user, s.settings())
	if err != nil || r.Kind != KindPerson || r.Person == nil || !r.lookupDue(language, s.now()) {
		return false
	}
	done := s.lookUp(r.ID, language)
	if done == nil {
		return false
	}
	timer := time.NewTimer(personLookupWait)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
	case <-ctx.Done():
	}
	return false
}

// CastPhotos returns the people of a title's cast with the photos Polyfin
// keeps of those the title gives none of, or one found dead of (see
// PhotoFailed): another title's addon gave it, or TMDB. Those left without
// one, and not looked up yet, are looked up in the background, for the
// next time the cast shows; those left without any show none.
func (s *Service) CastPhotos(ctx context.Context, user accounts.User, people []Person) []Person {
	if len(people) == 0 {
		return people
	}
	ids := make([]accounts.ID, len(people))
	for i, p := range people {
		ids[i] = p.ID
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return people
	}
	byID := make(map[accounts.ID]record, len(records))
	for _, r := range records {
		if r.Kind == KindPerson && r.Person != nil {
			byID[r.ID] = r
		}
	}
	lookups := s.tmdb.Enabled()
	language := peopleLanguage(user, s.settings())
	now := s.now()
	people = slices.Clone(people)
	for i, p := range people {
		r, ok := byID[p.ID]
		if !ok || p.Image != "" && !r.photoDead(p.Image, now) {
			continue
		}
		people[i].Image = r.personPhoto(now)
		// The photo is what the cast lacks: the biography waits for the
		// person's page.
		if people[i].Image == "" && lookups && r.identityDue(now) {
			s.lookUp(p.ID, language)
		}
	}
	return people
}

// lookUpPerson looks a person up on TMDB, for their biography in language,
// and keeps what TMDB tells on their record: who they are, the first time
// or when the addon names someone else, then their details. A person TMDB
// does not find, or cannot tell apart from others of the same name, is
// kept as not found; a lookup that fails is tried again later.
func (s *Service) lookUpPerson(ctx context.Context, id accounts.ID, language string) error {
	if !s.tmdb.Enabled() {
		return nil
	}
	r, err := s.load(ctx, id)
	if err != nil || r.Kind != KindPerson || r.Person == nil {
		return nil
	}
	now := s.now()
	if !r.lookupDue(language, now) {
		return nil
	}
	known := 0
	if d := r.TMDB; d != nil && d.ID > 0 && (r.TMDBID == "" || r.TMDBID == strconv.Itoa(d.ID)) {
		known = d.ID
	}
	found := known
	if found == 0 {
		found, err = s.findPerson(ctx, r)
	}
	var person tmdb.Person
	biographies := map[string]string{}
	if err == nil {
		person, err = s.tmdb.PersonDetails(ctx, found, language)
		biographies[language] = strings.TrimSpace(person.Biography)
	}
	if err == nil && biographies[language] == "" && baseLanguage(language) != englishBiography {
		var english tmdb.Person
		if english, err = s.tmdb.PersonDetails(ctx, found, englishBiography); err == nil {
			biographies[englishBiography] = strings.TrimSpace(english.Biography)
		}
	}
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		return s.keepPersonDetails(ctx, id, map[string]any{"id": 0, "foundAt": now, "failedAt": nil, "failures": 0, "due": nil,
			"photo": nil, "birth": nil, "death": nil, "place": nil, "imdb": nil}, nil, 0)
	case errors.Is(err, tmdb.ErrNoKey):
		return nil
	case err != nil:
		failures := 1
		if r.TMDB != nil {
			failures = r.TMDB.Failures + 1
		}
		if keepErr := s.keepPersonDetails(ctx, id, map[string]any{"failedAt": now, "failures": failures}, nil, -1); keepErr != nil {
			return keepErr
		}
		return err
	}
	fields := map[string]any{"id": found, "foundAt": now, "failedAt": nil, "failures": 0, "due": nil,
		"photo": nil, "birth": nil, "death": nil, "place": nil, "imdb": nil}
	if person.ProfilePath != "" {
		fields["photo"] = s.tmdb.ImageURL(person.ProfilePath, "original")
	}
	for name, value := range map[string]string{"birth": person.Birthday, "death": person.Deathday,
		"place": strings.TrimSpace(person.PlaceOfBirth), "imdb": person.IMDbID} {
		if value != "" {
			fields[name] = value
		}
	}
	return s.keepPersonDetails(ctx, id, fields, biographies, found)
}

// keepPersonDetails sets fields of a person's TMDB details, and adds
// biographies to theirs: in place of theirs when the TMDB identifier kept
// is not found, as for someone else than before, or no one; found is -1
// for a lookup that identified no one, keeping theirs. Lookups of the same
// person in two languages at once thus keep both biographies.
func (s *Service) keepPersonDetails(ctx context.Context, id accounts.ID, fields map[string]any, biographies map[string]string, found int) error {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if biographies == nil {
		biographies = map[string]string{}
	}
	bios, err := json.Marshal(biographies)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE items SET data = jsonb_set(data, '{tmdb}', jsonb_strip_nulls(COALESCE(data->'tmdb', '{}'::jsonb) || $2::jsonb
			|| jsonb_build_object('biographies', CASE WHEN $4::int >= 0 AND COALESCE((data->'tmdb'->>'id')::int, 0) <> $4::int THEN $3::jsonb
				ELSE COALESCE(data->'tmdb'->'biographies', '{}'::jsonb) || $3::jsonb END))),
			updated_at = now()
		WHERE id = $1 AND kind = 'person'`, id, encoded, bios, found)
	s.peopleChanged()
	return err
}

// baseLanguage is a language without its region: "pt" for "pt-BR".
func baseLanguage(language string) string {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	return base
}

// findPerson finds a person on TMDB: by the identifier the addon gave,
// else among the people of that very name, the one whose photo the addon
// gives when it is TMDB's, the only one, or the one credited in the
// titles Polyfin knows the person in. It answers tmdb.ErrNotFound when no
// one is, or when no title tells several apart.
func (s *Service) findPerson(ctx context.Context, r record) (int, error) {
	if id, err := strconv.Atoi(r.TMDBID); err == nil && id > 0 {
		return id, nil
	}
	found, err := s.tmdb.SearchPeople(ctx, r.Person.Name)
	if err != nil {
		return 0, err
	}
	if photo := tmdbPhotoFile(r.Person.Image); photo != "" {
		for _, p := range found {
			if path.Base(p.ProfilePath) == photo {
				return p.ID, nil
			}
		}
	}
	switch len(found) {
	case 0:
		return 0, tmdb.ErrNotFound
	case 1:
		return found[0].ID, nil
	}
	slices.SortStableFunc(found, func(a, b tmdb.FoundPerson) int { return cmp.Compare(b.Popularity, a.Popularity) })
	titles, err := s.titlesKnownIn(ctx, r)
	if err != nil {
		return 0, err
	}
	// The titles each is known for first, which the search gives; then
	// all their credits, a request each.
	if id := bestCredited(found, titles, func(p tmdb.FoundPerson) ([]tmdb.Credit, error) { return p.KnownFor, nil }); id != 0 {
		return id, nil
	}
	var failed error
	id := bestCredited(found[:min(len(found), maxPersonCandidates)], titles, func(p tmdb.FoundPerson) ([]tmdb.Credit, error) {
		credits, err := s.tmdb.PersonCredits(ctx, p.ID)
		if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
			failed = err
		}
		return credits, nil
	})
	switch {
	case id != 0:
		return id, nil
	case failed != nil:
		return 0, failed
	}
	return 0, tmdb.ErrNotFound
}

// bestCredited returns the person, of people, credited in the most titles
// of titles, the first of those credited in as many; 0 when none is in any.
func bestCredited(people []tmdb.FoundPerson, titles []personTitle, credits func(tmdb.FoundPerson) ([]tmdb.Credit, error)) int {
	best, most := 0, 0
	for _, p := range people {
		list, _ := credits(p)
		matched := 0
		for _, t := range titles {
			if slices.ContainsFunc(list, t.matches) {
				matched++
			}
		}
		if matched > most {
			best, most = p.ID, matched
		}
	}
	return best
}

// tmdbPhotoFile is the file name of a photo on TMDB's image server, empty
// for another address.
func tmdbPhotoFile(photo string) string {
	parsed, err := url.Parse(photo)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), "tmdb.org") || path.Base(parsed.Path) == "/" {
		return ""
	}
	return path.Base(parsed.Path)
}

// personTitle is a movie or series Polyfin knows a person in: its TMDB
// identifier when its addon gives one, its name, and its year, 0 unknown.
type personTitle struct {
	series bool
	tmdb   string
	name   string
	year   int
}

// titlesKnownIn lists the movies and series a person is credited in, as
// their addons listed them.
func (s *Service) titlesKnownIn(ctx context.Context, r record) ([]personTitle, error) {
	ids := make([]accounts.ID, 0, len(r.Credits))
	for raw := range r.Credits {
		if id, err := accounts.ParseID(raw); err == nil {
			ids = append(ids, id)
		}
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	var titles []personTitle
	for _, title := range records {
		if title.Meta == nil || title.Kind != KindMovie && title.Kind != KindSeries {
			continue
		}
		titles = append(titles, personTitle{series: title.Kind == KindSeries, tmdb: metaTMDB(*title.Meta),
			name: strings.TrimSpace(title.Meta.Name), year: metaYear(*title.Meta)})
	}
	return titles, nil
}

// metaTMDB is the TMDB identifier a description gives a title, empty for
// none.
func metaTMDB(meta stremio.Meta) string {
	if id, ok := strings.CutPrefix(meta.ID, "tmdb:"); ok {
		return id
	}
	return strings.TrimSpace(string(meta.TmdbID))
}

// metaYear is the year a description gives a title, 0 when it gives none.
func metaYear(meta stremio.Meta) int {
	for _, text := range []string{string(meta.Year), string(meta.ReleaseInfo), meta.Released} {
		if len(text) >= 4 {
			if year, err := strconv.Atoi(text[:4]); err == nil {
				return year
			}
		}
	}
	return 0
}

// matches reports whether a credit on TMDB is the title: of the same kind,
// by TMDB identifier, else by name and year, a year apart at most, as
// release dates differ between countries.
func (t personTitle) matches(c tmdb.Credit) bool {
	if (c.MediaType == "tv") != t.series || c.MediaType != "tv" && c.MediaType != "movie" {
		return false
	}
	if t.tmdb != "" {
		return t.tmdb == strconv.Itoa(c.ID)
	}
	if !slices.ContainsFunc(c.Names(), func(name string) bool { return strings.EqualFold(strings.TrimSpace(name), t.name) }) {
		return false
	}
	year := c.Year()
	return t.year == 0 || year == 0 || max(year, t.year)-min(year, t.year) <= 1
}

// PeopleStatus is how people's lookups on TMDB stand: Enabled while the
// settings hold a TMDB key; of the people looked up, Found those TMDB
// found and NotFound those it did not, Failing those whose last lookup
// failed, and Waiting the lookups waiting or under way. LimitedAt is when
// TMDB last refused a request for its rate since the start, zero for
// never, with the wait it asked then, LimitWait.
type PeopleStatus struct {
	Enabled   bool
	Found     int
	NotFound  int
	Failing   int
	Waiting   int
	LimitedAt time.Time
	LimitWait time.Duration
}

// peopleCountsQuery counts the people TMDB found, did not find, and whose
// last lookup failed, through items_person_tmdb.
const peopleCountsQuery = `SELECT
		count(*) FILTER (WHERE data->'tmdb' ? 'foundAt' AND COALESCE((data->'tmdb'->>'id')::int, 0) > 0),
		count(*) FILTER (WHERE data->'tmdb' ? 'foundAt' AND COALESCE((data->'tmdb'->>'id')::int, 0) = 0),
		count(*) FILTER (WHERE data->'tmdb' ? 'failedAt')
	FROM items WHERE kind = 'person' AND data ? 'tmdb'`

// PeopleStatus tells how people's lookups on TMDB stand. The counts are
// read once, then again only after a write changed people's TMDB details
// (see peopleChanged).
func (s *Service) PeopleStatus(ctx context.Context) (PeopleStatus, error) {
	status := PeopleStatus{Enabled: s.tmdb.Enabled()}
	status.LimitedAt, status.LimitWait = s.tmdb.LastRateLimit()
	c := &s.personLookups.counts
	c.mu.Lock()
	valid, changes := c.valid, c.changes
	status.Found, status.NotFound, status.Failing = c.found, c.notFound, c.failing
	c.mu.Unlock()
	if !valid {
		err := s.db.QueryRow(ctx, peopleCountsQuery).Scan(&status.Found, &status.NotFound, &status.Failing)
		if err != nil {
			return PeopleStatus{}, err
		}
		c.mu.Lock()
		if c.changes == changes {
			c.valid = true
			c.found, c.notFound, c.failing = status.Found, status.NotFound, status.Failing
		}
		c.mu.Unlock()
	}
	l := &s.personLookups
	l.mu.Lock()
	status.Waiting = len(l.running)
	l.mu.Unlock()
	return status, nil
}
