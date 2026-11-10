package library

import (
	"context"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
)

// The rating filters of a listing.
//
// Jellyfin's /Items narrows a listing by its items' ratings when asked
// (see RatingFilter), on top of the user's parental control. Polyfin
// judges a listing's titles by these where it judges them by parental
// control (see visible), with the same ratings and scores: a title by its
// rating for the server's rating country, the one an administrator set
// first, and seasons and episodes by their series, as Jellyfin's inherit
// their series' rating. So a listing narrowed this way pages and counts
// the titles it keeps, as a restricted user's does.

// RatingFilter is what a listing asks of its items' ratings, as sent:
//   - MinOfficialRating and MaxOfficialRating keep the items rated at least,
//     or at most, that rating's score under the server's rating country, and
//     those without a rating. A rating Polyfin cannot score filters nothing.
//   - HasParentalRating false keeps only the items without a rating, unless
//     a lowest or highest score applies: one asked, or the user's parental
//     control's, or the user's parental control hides some unrated items.
//     True filters nothing.
//   - HasOfficialRating keeps the items with, or without, an official
//     rating.
//   - OfficialRatings keeps the items whose official rating is one of them,
//     as written.
//
// The official rating is the one a title shows, the edit's first; the
// custom rating an administrator sets does not count. Seasons and episodes
// have none of their own, unless an administrator rated them or their
// series.
type RatingFilter struct {
	MinOfficialRating string
	MaxOfficialRating string
	HasParentalRating *bool
	HasOfficialRating *bool
	OfficialRatings   []string
}

// ratingFilterKey marks the context of a listing narrowed by its items'
// ratings (see WithRatingFilter).
type ratingFilterKey struct{}

// WithRatingFilter marks a listing narrowed by f. The listings, searches
// and items the service answers under the context keep only the items f
// keeps.
func WithRatingFilter(ctx context.Context, f RatingFilter) context.Context {
	if f.MinOfficialRating == "" && f.MaxOfficialRating == "" && f.HasParentalRating == nil &&
		f.HasOfficialRating == nil && len(f.OfficialRatings) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ratingFilterKey{}, f)
}

// ratingQuery is the rating filter of a request as its view applies it.
// active is set when it narrows anything.
type ratingQuery struct {
	active      bool
	min, max    *localization.Score
	unratedOnly bool
	hasOfficial *bool
	officials   []string
}

// withRatingFilter returns v narrowed by the rating filter of ctx, if any.
// A rating asked as a limit is scored under the server's rating country,
// as Jellyfin scores it under its own.
func (v view) withRatingFilter(ctx context.Context) view {
	v.query = ratingQuery{}
	f, ok := ctx.Value(ratingFilterKey{}).(RatingFilter)
	if !ok {
		return v
	}
	q := ratingQuery{hasOfficial: f.HasOfficialRating, officials: f.OfficialRatings}
	q.min = limitScore(f.MinOfficialRating, v.country)
	q.max = limitScore(f.MaxOfficialRating, v.country)
	// A highest rating asked replaces the user's in Jellyfin's query, which
	// only lists unrated items when none applies; Polyfin applies both.
	userMax := strings.TrimSpace(f.MaxOfficialRating) == "" && v.parental.MaxRating != nil
	q.unratedOnly = f.HasParentalRating != nil && !*f.HasParentalRating && q.min == nil && q.max == nil && !userMax &&
		!slices.ContainsFunc(v.parental.BlockUnrated, func(kind string) bool { return kind != "Other" })
	q.active = q.min != nil || q.max != nil || q.unratedOnly || q.hasOfficial != nil || len(q.officials) > 0
	v.query = q
	return v
}

// limitScore scores a rating asked as a limit; nil when it is blank or
// Polyfin cannot score it.
func limitScore(rating, country string) *localization.Score {
	if strings.TrimSpace(rating) == "" {
		return nil
	}
	if score, ok := localization.RatingScore(rating, country); ok {
		return &score
	}
	return nil
}

// keeps reports whether the filter keeps an item rated rating, the rating
// it is judged by, whose official rating is official. An item without a
// rating Polyfin can score is within any limit.
func (q ratingQuery) keeps(rating, official, country string) bool {
	score, rated := localization.RatingScore(rating, country)
	switch {
	case rated && q.min != nil && compareScores(score, *q.min) < 0,
		rated && q.max != nil && compareScores(score, *q.max) > 0,
		rated && q.unratedOnly,
		q.hasOfficial != nil && (official != "") != *q.hasOfficial,
		len(q.officials) > 0 && !slices.Contains(q.officials, official):
		return false
	}
	return true
}

// compareScores orders two scores by score, then subscore, a missing
// subscore counting as 0.
func compareScores(a, b localization.Score) int {
	if a.Score != b.Score {
		return a.Score - b.Score
	}
	return subScore(a) - subScore(b)
}

func subScore(s localization.Score) int {
	if s.SubScore == nil {
		return 0
	}
	return *s.SubScore
}

// screens reports whether a listing judges an item of kind by its rating
// or genres: the user's restrictions, or the request's rating filter.
func (v view) screens(kind Kind) bool {
	if v.judges(kind) {
		return true
	}
	_, ok := unratedKind(kind)
	return ok && v.query.active
}

// narrows reports whether a listing leaves titles out by their rating or
// genres: the user's restrictions, or the request's rating filter.
func (v view) narrows() bool {
	return v.restricted() || v.query.active
}

// listVerdict decides whether a listing keeps an item of kind with traits
// t (see verdict), whose official rating is official: the user's
// restrictions, then the request's rating filter, which needs its rating.
func (v view) listVerdict(kind Kind, t traits, official string) (allowed, decided bool) {
	allowed, decided = v.verdict(kind, t)
	if _, ok := unratedKind(kind); !ok || !v.query.active || decided && !allowed {
		return allowed, decided
	}
	if !t.ratingKnown {
		return false, false
	}
	if !v.query.keeps(t.rating, official, v.country) {
		return false, true
	}
	return allowed, decided
}

// officialRating is the official rating of the item of r whose
// description rates it described: the one an administrator set, else that
// one. A season or an episode has the one set of it or of its series, as
// Jellyfin's edits of a series rate its seasons and episodes, and none
// otherwise.
func (s *Service) officialRating(r record, described string) string {
	set := s.overridesNow()
	if o, ok := set.fields[r.ID]; ok && o.OfficialRating != nil {
		return *o.OfficialRating
	}
	if r.Kind != KindSeason && r.Kind != KindEpisode {
		return described
	}
	if o, ok := set.fields[r.seriesItemID()]; ok && o.OfficialRating != nil {
		return *o.OfficialRating
	}
	return ""
}

// RatingFiltered keeps the items the rating filter of ctx keeps (see
// WithRatingFilter), in order, for listings made of items already known.
func (s *Service) RatingFiltered(ctx context.Context, user accounts.User, items []Item) ([]Item, error) {
	if _, ok := ctx.Value(ratingFilterKey{}).(RatingFilter); !ok || len(items) == 0 {
		return items, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	return s.ratingFiltered(ctx, v, items)
}

// ratingFiltered keeps the items v's rating filter keeps, judged as
// visible judges their records. Items Polyfin keeps no record of are not
// titles and are kept.
func (s *Service) ratingFiltered(ctx context.Context, v view, items []Item) ([]Item, error) {
	if !v.query.active || len(items) == 0 {
		return items, nil
	}
	ids := make([]accounts.ID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	loaded, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[accounts.ID]record, len(loaded))
	for _, r := range loaded {
		byID[r.ID] = r
	}
	judged := make([]record, 0, len(loaded))
	for _, item := range items {
		if r, ok := byID[item.ID]; ok {
			judged = append(judged, r)
		}
	}
	visible, _ := s.visible(ctx, v, judged)
	kept := make(map[accounts.ID]bool, len(visible))
	for _, r := range visible {
		kept[r.ID] = true
	}
	return slices.DeleteFunc(slices.Clone(items), func(item Item) bool {
		_, known := byID[item.ID]
		return known && !kept[item.ID]
	}), nil
}

// OfficialRatings lists the official ratings of items, each once, as the
// rating filter judges them (see officialRating): the one an
// administrator set first, else the one known for the server's rating
// country. No rating is looked up: a title whose rating is not known yet
// has none here. They are sorted by their score under the server's
// rating country, then by name, those Polyfin cannot score last.
func (s *Service) OfficialRatings(ctx context.Context, user accounts.User, items []Item) ([]string, error) {
	result := []string{}
	if len(items) == 0 {
		return result, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	ids := make([]accounts.ID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	titles := make([]record, len(records))
	for i, r := range records {
		titles[i] = r
		var holder accounts.ID
		switch {
		case r.Kind == KindSeason || r.Kind == KindEpisode:
			holder = r.seriesItemID()
		case r.folderVideo():
			holder = r.folderItemID()
		}
		if holder != (accounts.ID{}) {
			if title, err := s.load(ctx, holder); err == nil {
				titles[i] = title
			}
		}
	}
	s.storedTraits(ctx, titles)
	seen := map[string]bool{}
	for i, r := range titles {
		if _, ok := unratedKind(r.Kind); !ok {
			continue
		}
		t, _ := s.knownTraits(ctx, v, r)
		official := strings.TrimSpace(s.officialRating(records[i], t.rating))
		if official != "" && !seen[official] {
			seen[official] = true
			result = append(result, official)
		}
	}
	slices.SortFunc(result, func(a, b string) int {
		sa, oka := localization.RatingScore(a, v.country)
		sb, okb := localization.RatingScore(b, v.country)
		switch {
		case oka && okb:
			if c := compareScores(sa, sb); c != 0 {
				return c
			}
		case oka:
			return -1
		case okb:
			return 1
		}
		return strings.Compare(a, b)
	})
	return result, nil
}

// WithMembersFilter returns ctx with its rating filter narrowed to what
// a container's members are judged by when deciding whether the
// container shows: the highest rating asked only, as Jellyfin hides a
// collection or a playlist none of whose members is within it.
func WithMembersFilter(ctx context.Context) context.Context {
	f, ok := ctx.Value(ratingFilterKey{}).(RatingFilter)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, ratingFilterKey{}, RatingFilter{MaxOfficialRating: f.MaxOfficialRating})
}

// Narrows reports whether the listings user is answered under ctx leave
// titles out by their rating or genres: the user's parental control or
// blocked genres, or the rating filter of ctx (see WithRatingFilter).
func (s *Service) Narrows(ctx context.Context, user accounts.User) (bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	return v.narrows(), nil
}
