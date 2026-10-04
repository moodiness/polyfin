package library

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title's rating is the certification its addon gives in its complete
// description. Catalog pages rarely carry it, so listings for a user whose
// parental control hides titles must look it up: once known, from any
// user's visit, it is kept with the item and listings read it from there.
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
	// maxRatingLookups bounds the lookups one request starts: a listing
	// that hides most of a catalog may read far into it.
	maxRatingLookups = 100
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

// allows reports whether the user's parental control lets them reach an
// item of kind rated rating. A title whose rating is not known is hidden
// from a restricted user: it may be one the control hides.
func (v view) allows(kind Kind, rating string, known bool) bool {
	if !v.parental.Restricted() {
		return true
	}
	name, ok := unratedKind(kind)
	return !ok || known && v.parental.Allows(name, rating)
}

// knownRating returns a title's rating when Polyfin knows it: kept from a
// complete description, from one still cached, or carried by its listing.
func (s *Service) knownRating(r record) (string, bool) {
	if r.Rating != nil {
		return *r.Rating, true
	}
	if full, ok := s.cachedMeta(r); ok {
		return certification(full), true
	}
	if r.Meta != nil {
		if rating := certification(*r.Meta); rating != "" {
			return rating, true
		}
	}
	return "", false
}

// learnRating keeps the rating a title's complete description gives, none
// included, so that listings know it without asking again.
func (s *Service) learnRating(ctx context.Context, r record, meta stremio.Meta) {
	rating := certification(meta)
	if r.Rating != nil && *r.Rating == rating {
		return
	}
	r.Rating = &rating
	if err := s.save(ctx, []record{r}); err != nil && ctx.Err() == nil {
		s.logger.Warn("The rating of a title could not be kept", "item", r.ID, "error", err)
	}
}

// visible keeps the records of the items the user's parental control lets
// them reach, in order. Seasons and episodes are judged by their series.
// The ratings of titles not known yet are looked up, until the request's
// deadline; those still unknown then are left out.
func (s *Service) visible(ctx context.Context, v view, records []record) []record {
	if !v.parental.Restricted() || len(records) == 0 {
		return records
	}
	titles := make([]record, len(records))
	for i, r := range records {
		titles[i] = r
		if r.Kind == KindSeason || r.Kind == KindEpisode {
			if series, err := s.load(ctx, r.seriesItemID()); err == nil {
				titles[i] = series
			}
		}
	}
	s.storedRatings(ctx, titles)
	allowed := make([]bool, len(records))
	var unknown []int
	for i, r := range titles {
		if _, judged := unratedKind(r.Kind); !judged {
			allowed[i] = true
		} else if rating, known := s.knownRating(r); known {
			allowed[i] = v.allows(r.Kind, rating, true)
		} else {
			unknown = append(unknown, i)
		}
	}
	for i, rating := range s.lookUpRatings(ctx, v, titles, unknown) {
		if rating != nil {
			allowed[unknown[i]] = v.allows(titles[unknown[i]].Kind, *rating, true)
		}
	}
	kept := records[:0:0]
	for i, r := range records {
		if allowed[i] {
			kept = append(kept, r)
		}
	}
	return kept
}

// visibleMetas keeps the entries of a catalog page the user's parental
// control lets them see. Entries that are not titles, collections among
// them, are kept.
func (s *Service) visibleMetas(ctx context.Context, v view, src source, metas []stremio.Meta) []stremio.Meta {
	if !v.parental.Restricted() || len(metas) == 0 {
		return metas
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
	for _, r := range s.visible(ctx, v, titles) {
		allowed[judged[r.ID]] = true
	}
	kept := metas[:0:0]
	for i, meta := range metas {
		if allowed[i] {
			kept = append(kept, meta)
		}
	}
	return kept
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
	rows, err := s.db.Query(ctx, "SELECT id, data->>'rating' FROM items WHERE id = ANY($1) AND data ? 'rating'", ids)
	if err != nil {
		s.logger.Debug("Stored ratings could not be read", "error", err)
		return
	}
	defer rows.Close()
	stored := map[accounts.ID]string{}
	for rows.Next() {
		var id accounts.ID
		var rating string
		if rows.Scan(&id, &rating) == nil {
			stored[id] = rating
		}
	}
	for i := range records {
		if rating, ok := stored[records[i].ID]; ok && records[i].Rating == nil {
			records[i].Rating = &rating
		}
	}
}

// lookUpRatings looks up the ratings of the titles at indexes, a few at a
// time, and returns those known by the request's deadline, nil for the
// others. Lookups outlive the request, so that a later one knows them.
func (s *Service) lookUpRatings(ctx context.Context, v view, titles []record, indexes []int) []*string {
	results := make([]*string, len(indexes))
	if len(indexes) == 0 {
		return results
	}
	type found struct {
		at     int
		rating *string
	}
	done := make(chan found, len(indexes))
	started := 0
	for at, i := range indexes {
		r := titles[i]
		if r.Meta == nil || v.lookups.Add(1) > maxRatingLookups {
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
			// titleMeta keeps the rating it finds.
			if meta, ok := s.titleMeta(lookup, v, r); ok {
				result.rating = new(certification(meta))
			}
		}()
	}
	if started == 0 {
		return results
	}
	wait := time.NewTimer(time.Until(v.deadline))
	defer wait.Stop()
	for range started {
		select {
		case f := <-done:
			results[f.at] = f.rating
		case <-wait.C:
			return results
		case <-ctx.Done():
			return results
		}
	}
	return results
}
