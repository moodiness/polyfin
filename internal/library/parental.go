package library

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title's rating is the certification the server's addons give in its
// complete description. Users' own addons are not asked: a user could
// install one that rates titles as they please. Catalog pages rarely carry
// ratings, so listings for a user whose parental control hides titles must
// look them up: once known, from any user's visit, a rating is kept with
// the item and listings read it from there. A title's genres, which the
// genres a user blocks hide, are learned the same way, from the same
// descriptions.
const (
	// ratingWait bounds how long one request waits for the ratings of the
	// titles it lists. Titles still unknown then are hidden from the
	// restricted user for that answer, while their lookup goes on.
	ratingWait = 3 * time.Second
	// ratingLookupTime bounds a lookup that goes on in the background.
	ratingLookupTime = 30 * time.Second
	// ratingFetches bounds the descriptions looked up at once, across
	// requests.
	ratingFetches = 8
	// maxRatingLookups bounds the lookups one request starts.
	maxRatingLookups = 100
	// hiddenReach bounds how far a restricted listing reads a catalog, as
	// a multiple of what an unrestricted one reads: a catalog whose titles
	// are mostly hidden ends there rather than being read whole.
	hiddenReach = 4
	// The ratings kept are asked again once they are this old, while
	// listings go on using them: a title rated after it was first seen is
	// often a new release, so a title without a rating is asked again
	// sooner.
	unratedTTL = metaTTL
	ratedTTL   = 7 * 24 * time.Hour
)

// certification is the rating a title's description gives: the addon's
// certification, else its local one.
func certification(meta stremio.Meta) string {
	if meta.Extras == nil {
		return ""
	}
	if meta.Extras.Certification != "" {
		return meta.Extras.Certification
	}
	return meta.Extras.CertificationLocal
}

// unratedKind names the kind of an item as parental control does (see
// accounts.UnratedKinds): seasons and episodes follow their series. Other
// items, folders and people, are never hidden.
func unratedKind(kind Kind) (string, bool) {
	switch kind {
	case KindMovie:
		return "Movie", true
	case KindSeries, KindSeason, KindEpisode:
		return "Series", true
	}
	return "", false
}

// judges reports whether the user's parental control or blocked genres
// may hide an item of kind depending on its rating or genres; the others
// need neither.
func (v view) judges(kind Kind) bool {
	name, ok := unratedKind(kind)
	return ok && (v.parental.Judges(name) || v.blocksGenres())
}

// restricted reports whether the user's parental control or blocked
// genres hide titles (see accounts.User.Restricted).
func (v view) restricted() bool {
	return v.parental.Restricted() || v.blocksGenres()
}

func (v view) blocksGenres() bool {
	return len(v.genres) > 0
}

// blocked reports whether one of genres is one the user blocks, compared
// without regard to case.
func (v view) blocked(genres []string) bool {
	return slices.ContainsFunc(genres, func(genre string) bool {
		genre = strings.TrimSpace(genre)
		return slices.ContainsFunc(v.genres, func(b string) bool { return strings.EqualFold(genre, b) })
	})
}

// traits are what hides a title from a restricted user: its rating and
// its genres, each with whether Polyfin knows it.
type traits struct {
	rating      string
	genres      []string
	ratingKnown bool
	genresKnown bool
}

// describedTraits are the traits a complete description gives.
func describedTraits(meta stremio.Meta) traits {
	return traits{rating: certification(meta), genres: meta.Genres, ratingKnown: true, genresKnown: true}
}

// verdict decides whether the user may reach an item of kind with traits
// t. A title is hidden as soon as a trait known hides it, and allowed once
// every trait that matters is known; until then, decided is false: it may
// be one the user's restrictions hide.
func (v view) verdict(kind Kind, t traits) (allowed, decided bool) {
	if !v.judges(kind) {
		return true, true
	}
	name, _ := unratedKind(kind)
	byRating := v.parental.Judges(name)
	switch {
	case t.genresKnown && v.blocked(t.genres):
		return false, true
	case byRating && t.ratingKnown && !v.parental.Allows(name, t.rating):
		return false, true
	case byRating && !t.ratingKnown, v.blocksGenres() && !t.genresKnown:
		return false, false
	}
	return true, true
}

// allows reports whether the user may reach an item of kind with traits t;
// a title whose verdict is not decided is hidden.
func (v view) allows(kind Kind, t traits) bool {
	allowed, decided := v.verdict(kind, t)
	return allowed && decided
}

// sharedAddon reports whether the addon identified by id is one of the
// server's that the user reaches.
func (v view) sharedAddon(id *accounts.ID) bool {
	if id == nil {
		return false
	}
	entry, ok := v.addon(*id)
	return ok && entry.shared
}

// rates reports whether one of the server's addons the user reaches can
// describe a title, which gives its rating and genres.
func (v view) rates(r record) bool {
	if r.Meta == nil {
		return false
	}
	for _, entry := range v.addons {
		if entry.shared && entry.addon.Manifest.Serves("meta", r.Meta.Type, r.Meta.ID) {
			return true
		}
	}
	return false
}

// knownTraits returns what Polyfin knows of a title's rating and genres:
// kept from a complete description, from one still cached, or carried by
// its listing, from the server's addons. A listing's genres count only
// when they include one the user blocks, as a description may give more
// genres. A title none of the server's addons describes has no rating,
// and the genres of its listing. stale is set when the traits kept are old
// enough to be asked again.
func (s *Service) knownTraits(v view, r record) (t traits, stale bool) {
	if r.Rating != nil && (r.Genres != nil || !v.blocksGenres()) {
		kept := traits{rating: *r.Rating, ratingKnown: true, genresKnown: r.Genres != nil}
		if r.Genres != nil {
			kept.genres = *r.Genres
		}
		if r.RatedAt != nil && s.now().Sub(*r.RatedAt) < ratingTTL(*r.Rating) {
			return kept, false
		}
		if v.sharedAddon(r.Addon) {
			if full, ok := s.cachedMeta(r); ok {
				return describedTraits(full), false
			}
		}
		return kept, true
	}
	if v.sharedAddon(r.Addon) {
		if full, ok := s.cachedMeta(r); ok {
			return describedTraits(full), false
		}
	}
	describable := v.rates(r)
	if v.sharedAddon(r.Addon) && r.Meta != nil {
		if rating := certification(*r.Meta); rating != "" {
			t.rating, t.ratingKnown = rating, true
		}
		if !describable || v.blocked(r.Meta.Genres) {
			t.genres, t.genresKnown = r.Meta.Genres, true
		}
	}
	if !describable {
		t.ratingKnown, t.genresKnown = true, true
	}
	return t, false
}

func ratingTTL(rating string) time.Duration {
	if rating == "" {
		return unratedTTL
	}
	return ratedTTL
}

// learnTraits keeps the rating and genres a description by one of the
// server's addons gives, none included, so that listings know them
// without asking again.
func (s *Service) learnTraits(ctx context.Context, r record, meta stremio.Meta) {
	rating, genres := certification(meta), []string(meta.Genres)
	if genres == nil {
		genres = []string{}
	}
	now := s.now()
	// Traits confirmed recently are not written again.
	if r.Rating != nil && *r.Rating == rating && r.Genres != nil && slices.Equal(*r.Genres, genres) &&
		r.RatedAt != nil && now.Sub(*r.RatedAt) < unratedTTL {
		return
	}
	r.Rating, r.Genres, r.RatedAt = &rating, &genres, &now
	if err := s.save(ctx, []record{r}); err != nil && ctx.Err() == nil {
		s.logger.Warn("The rating and genres of a title could not be kept", "item", r.ID, "error", err)
	}
}

// visible keeps the records of the items the user's parental control and
// blocked genres let them reach, in order. Seasons and episodes are judged
// by their series. The ratings and genres of titles not known yet are
// looked up, until the request's deadline; those still unknown then are
// left out. held reports that some were left out only because their lookup
// did not answer in time or could not start, the request's lookups being
// spent.
func (s *Service) visible(ctx context.Context, v view, records []record) (kept []record, held bool) {
	if !v.restricted() || len(records) == 0 {
		return records, false
	}
	titles := make([]record, len(records))
	for i, r := range records {
		titles[i] = r
		if (r.Kind == KindSeason || r.Kind == KindEpisode) && v.judges(r.Kind) {
			if series, err := s.load(ctx, r.seriesItemID()); err == nil {
				titles[i] = series
			}
		}
	}
	s.storedTraits(ctx, titles)
	allowed := make([]bool, len(records))
	var unknown, stale []int
	for i, r := range titles {
		if !v.judges(r.Kind) {
			allowed[i] = true
			continue
		}
		t, old := s.knownTraits(v, r)
		if ok, decided := v.verdict(r.Kind, t); decided {
			allowed[i] = ok
			if old {
				stale = append(stale, i)
			}
		} else {
			unknown = append(unknown, i)
		}
	}
	found, pending := s.lookUpTraits(ctx, v, titles, unknown)
	for i, t := range found {
		if t != nil {
			allowed[unknown[i]] = v.allows(titles[unknown[i]].Kind, *t)
		}
		held = held || pending[i]
	}
	// Traits kept long ago are asked again for the next listings; this one
	// uses them.
	s.lookUpTraits(context.WithoutCancel(ctx), v.withoutWait(), titles, stale)
	kept = records[:0:0]
	for i, r := range records {
		if allowed[i] {
			kept = append(kept, r)
		}
	}
	return kept, held
}

// withoutWait returns the view with its deadline passed, for lookups no
// one waits for.
func (v view) withoutWait() view {
	v.deadline = time.Time{}
	return v
}

// visibleMetas keeps the entries of a catalog page the user's parental
// control and blocked genres let them see. Entries that are not titles,
// collections among them, are kept. held reports titles left out only
// because their rating or genres are not known yet (see visible).
func (s *Service) visibleMetas(ctx context.Context, v view, src source, metas []stremio.Meta) ([]stremio.Meta, bool) {
	if !v.restricted() || len(metas) == 0 {
		return metas, false
	}
	var titles []record
	judged := make(map[accounts.ID]int, len(metas))
	for i, meta := range metas {
		// The listing saves its records with their folder afterwards.
		if _, r, err := titleItem(src.addon.addon.ID, src.catalog, meta, accounts.ID{}, src.addon.confined); err == nil {
			r.Parent = nil
			titles = append(titles, r)
			judged[r.ID] = i
		}
	}
	allowed := make([]bool, len(metas))
	for i := range metas {
		allowed[i] = true
	}
	for _, r := range titles {
		allowed[judged[r.ID]] = false
	}
	visible, held := s.visible(ctx, v, titles)
	for _, r := range visible {
		allowed[judged[r.ID]] = true
	}
	kept := metas[:0:0]
	for i, meta := range metas {
		if allowed[i] {
			kept = append(kept, meta)
		}
	}
	return kept, held
}

// storedTraits fills in the ratings and genres kept for records made from
// a listing, which carry none of their own.
func (s *Service) storedTraits(ctx context.Context, records []record) {
	ids := make([]accounts.ID, 0, len(records))
	for _, r := range records {
		if r.Rating == nil {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id, data->>'rating', (data->>'ratedAt')::timestamptz, data->'genres' FROM items
		WHERE id = ANY($1) AND data ? 'rating'`, ids)
	if err != nil {
		s.logger.Debug("Stored ratings could not be read", "error", err)
		return
	}
	defer rows.Close()
	type kept struct {
		rating string
		at     *time.Time
		genres *[]string
	}
	stored := map[accounts.ID]kept{}
	for rows.Next() {
		var id accounts.ID
		var k kept
		if rows.Scan(&id, &k.rating, &k.at, &k.genres) == nil {
			stored[id] = k
		}
	}
	for i := range records {
		if k, ok := stored[records[i].ID]; ok && records[i].Rating == nil {
			records[i].Rating, records[i].RatedAt, records[i].Genres = &k.rating, k.at, k.genres
		}
	}
}

// lookUpTraits looks up the ratings and genres of the titles at indexes
// among the server's addons, a few at a time, and returns those known by
// the request's deadline, nil for the others. pending marks the titles
// whose lookup did not answer in time, or did not start because the
// request started its share. Lookups outlive the request, so that a later
// one knows them.
func (s *Service) lookUpTraits(ctx context.Context, v view, titles []record, indexes []int) (found []*traits, pending []bool) {
	found, pending = make([]*traits, len(indexes)), make([]bool, len(indexes))
	if len(indexes) == 0 {
		return found, pending
	}
	type result struct {
		at     int
		traits *traits
	}
	done := make(chan result, len(indexes))
	started := 0
	for at, i := range indexes {
		title := titles[i]
		if title.Meta == nil {
			continue
		}
		pending[at] = true
		if v.lookups.Add(1) > maxRatingLookups {
			continue
		}
		started++
		go func() {
			r := result{at: at}
			defer func() { done <- r }()
			lookup, cancel := context.WithTimeout(context.WithoutCancel(ctx), ratingLookupTime)
			defer cancel()
			select {
			case s.ratingLookups <- struct{}{}:
				defer func() { <-s.ratingLookups }()
			case <-lookup.Done():
				return
			}
			// describe keeps the rating and genres it finds.
			if meta, ok := s.describe(lookup, v, title, true); ok {
				r.traits = new(describedTraits(meta))
			}
		}()
	}
	if started == 0 || v.deadline.IsZero() {
		return found, pending
	}
	wait := time.NewTimer(time.Until(v.deadline))
	defer wait.Stop()
	for range started {
		select {
		case r := <-done:
			found[r.at], pending[r.at] = r.traits, false
		case <-wait.C:
			return found, pending
		case <-ctx.Done():
			return found, pending
		}
	}
	return found, pending
}

// refreshBatch bounds the titles one RefreshRatings asks about.
const refreshBatch = 500

// RefreshRatings asks the server's addons again for the ratings and genres
// kept for titles once they are old enough to be asked again, the oldest
// first and at most refreshBatch of them, one at a time within the lookups
// listings share, and returns how many were asked. Listings do the same
// for the titles they list; this does it ahead, when an administrator
// starts it.
func (s *Service) RefreshRatings(ctx context.Context) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM items
		WHERE data ? 'rating' AND data ? 'ratedAt' AND (data->>'ratedAt')::timestamptz <
			CASE WHEN data->>'rating' = '' THEN $1::timestamptz ELSE $2::timestamptz END
		ORDER BY (data->>'ratedAt')::timestamptz LIMIT $3`,
		s.now().Add(-unratedTTL), s.now().Add(-ratedTTL), refreshBatch)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return 0, err
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return 0, err
	}
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return 0, err
	}
	asked := 0
	for _, r := range records {
		if !v.rates(r) {
			continue
		}
		select {
		case s.ratingLookups <- struct{}{}:
		case <-ctx.Done():
			return asked, ctx.Err()
		}
		lookup, cancel := context.WithTimeout(ctx, ratingLookupTime)
		// describe keeps the rating and genres it finds.
		s.describe(lookup, v, r, true)
		cancel()
		<-s.ratingLookups
		asked++
	}
	return asked, ctx.Err()
}
