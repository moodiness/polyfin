package library

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title's rating is the certification the server's addons give in its
// complete description. Users' own addons are not asked: a user could
// install one that rates titles as they please. Catalog pages rarely carry
// ratings, so listings for a user whose parental control hides titles must
// look them up: once known, from any user's visit, a rating is kept with
// the item and listings read it from there.
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

// judges reports whether the user's parental control may hide an item of
// kind depending on its rating; the others need no rating.
func (v view) judges(kind Kind) bool {
	name, ok := unratedKind(kind)
	return ok && v.parental.Judges(name)
}

// allows reports whether the user's parental control lets them reach an
// item of kind rated rating. A title whose rating is not known is hidden
// when its rating matters: it may be one the control hides.
func (v view) allows(kind Kind, rating string, known bool) bool {
	if !v.judges(kind) {
		return true
	}
	name, _ := unratedKind(kind)
	return known && v.parental.Allows(name, rating)
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
// describe a title, which gives its rating.
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

// knownRating returns a title's rating when Polyfin knows it: kept from a
// complete description, from one still cached, or carried by its listing,
// from the server's addons. A title none of them describes has none. stale
// is set when the rating kept is old enough to be asked again.
func (s *Service) knownRating(v view, r record) (rating string, known, stale bool) {
	fresh := r.Rating != nil && r.RatedAt != nil && s.now().Sub(*r.RatedAt) < ratingTTL(*r.Rating)
	if fresh {
		return *r.Rating, true, false
	}
	if v.sharedAddon(r.Addon) {
		if full, ok := s.cachedMeta(r); ok {
			return certification(full), true, false
		}
	}
	if r.Rating != nil {
		return *r.Rating, true, true
	}
	if v.sharedAddon(r.Addon) && r.Meta != nil {
		if rating := certification(*r.Meta); rating != "" {
			return rating, true, false
		}
	}
	if !v.rates(r) {
		return "", true, false
	}
	return "", false, false
}

func ratingTTL(rating string) time.Duration {
	if rating == "" {
		return unratedTTL
	}
	return ratedTTL
}

// learnRating keeps the rating a description by one of the server's
// addons gives, none included, so that listings know it without asking
// again.
func (s *Service) learnRating(ctx context.Context, r record, meta stremio.Meta) {
	rating := certification(meta)
	now := s.now()
	// A rating confirmed recently is not written again.
	if r.Rating != nil && *r.Rating == rating && r.RatedAt != nil && now.Sub(*r.RatedAt) < unratedTTL {
		return
	}
	r.Rating, r.RatedAt = &rating, &now
	if err := s.save(ctx, []record{r}); err != nil && ctx.Err() == nil {
		s.logger.Warn("The rating of a title could not be kept", "item", r.ID, "error", err)
	}
}

// visible keeps the records of the items the user's parental control lets
// them reach, in order. Seasons and episodes are judged by their series.
// The ratings of titles not known yet are looked up, until the request's
// deadline; those still unknown then are left out. held reports that some
// were left out only because their lookup did not answer in time or could
// not start, the request's lookups being spent.
func (s *Service) visible(ctx context.Context, v view, records []record) (kept []record, held bool) {
	if !v.parental.Restricted() || len(records) == 0 {
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
	s.storedRatings(ctx, titles)
	allowed := make([]bool, len(records))
	var unknown, stale []int
	for i, r := range titles {
		if !v.judges(r.Kind) {
			allowed[i] = true
			continue
		}
		rating, known, old := s.knownRating(v, r)
		switch {
		case known:
			allowed[i] = v.allows(r.Kind, rating, true)
			if old {
				stale = append(stale, i)
			}
		default:
			unknown = append(unknown, i)
		}
	}
	ratings, pending := s.lookUpRatings(ctx, v, titles, unknown)
	for i, rating := range ratings {
		if rating != nil {
			allowed[unknown[i]] = v.allows(titles[unknown[i]].Kind, *rating, true)
		}
		held = held || pending[i]
	}
	// Ratings kept long ago are asked again for the next listings; this one
	// uses them.
	s.lookUpRatings(context.WithoutCancel(ctx), v.withoutWait(), titles, stale)
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
// control lets them see. Entries that are not titles, collections among
// them, are kept. held reports titles left out only because their rating
// is not known yet (see visible).
func (s *Service) visibleMetas(ctx context.Context, v view, src source, metas []stremio.Meta) ([]stremio.Meta, bool) {
	if !v.parental.Restricted() || len(metas) == 0 {
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

// storedRatings fills in the ratings kept for records made from a listing,
// which carry none of their own.
func (s *Service) storedRatings(ctx context.Context, records []record) {
	ids := make([]accounts.ID, 0, len(records))
	for _, r := range records {
		if r.Rating == nil {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id, data->>'rating', (data->>'ratedAt')::timestamptz FROM items
		WHERE id = ANY($1) AND data ? 'rating'`, ids)
	if err != nil {
		s.logger.Debug("Stored ratings could not be read", "error", err)
		return
	}
	defer rows.Close()
	type kept struct {
		rating string
		at     *time.Time
	}
	stored := map[accounts.ID]kept{}
	for rows.Next() {
		var id accounts.ID
		var k kept
		if rows.Scan(&id, &k.rating, &k.at) == nil {
			stored[id] = k
		}
	}
	for i := range records {
		if k, ok := stored[records[i].ID]; ok && records[i].Rating == nil {
			records[i].Rating, records[i].RatedAt = &k.rating, k.at
		}
	}
}

// lookUpRatings looks up the ratings of the titles at indexes among the
// server's addons, a few at a time, and returns those known by the
// request's deadline, nil for the others. pending marks the titles whose
// lookup did not answer in time, or did not start because the request
// started its share. Lookups outlive the request, so that a later one
// knows them.
func (s *Service) lookUpRatings(ctx context.Context, v view, titles []record, indexes []int) (ratings []*string, pending []bool) {
	ratings, pending = make([]*string, len(indexes)), make([]bool, len(indexes))
	if len(indexes) == 0 {
		return ratings, pending
	}
	type found struct {
		at     int
		rating *string
	}
	done := make(chan found, len(indexes))
	started := 0
	for at, i := range indexes {
		r := titles[i]
		if r.Meta == nil {
			continue
		}
		pending[at] = true
		if v.lookups.Add(1) > maxRatingLookups {
			continue
		}
		started++
		go func() {
			result := found{at: at}
			defer func() { done <- result }()
			lookup, cancel := context.WithTimeout(context.WithoutCancel(ctx), ratingLookupTime)
			defer cancel()
			select {
			case s.ratingLookups <- struct{}{}:
				defer func() { <-s.ratingLookups }()
			case <-lookup.Done():
				return
			}
			// describe keeps the rating it finds.
			if meta, ok := s.describe(lookup, v, r, true); ok {
				result.rating = new(certification(meta))
			}
		}()
	}
	if started == 0 || v.deadline.IsZero() {
		return ratings, pending
	}
	wait := time.NewTimer(time.Until(v.deadline))
	defer wait.Stop()
	for range started {
		select {
		case f := <-done:
			ratings[f.at], pending[f.at] = f.rating, false
		case <-wait.C:
			return ratings, pending
		case <-ctx.Done():
			return ratings, pending
		}
	}
	return ratings, pending
}
