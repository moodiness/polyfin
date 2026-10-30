package smartlists

import (
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/localization"
)

// What a rule looks at.
const (
	// FieldGroup is a group of rules, all or any of which a title must
	// match (see Match).
	FieldGroup = "group"
	// FieldGenre matches a title with the genre Value, in any letter case.
	FieldGenre = "genre"
	// FieldYear matches a title that came out from Min to Max, either
	// left out for no bound.
	FieldYear = "year"
	// FieldRating matches a title whose IMDb rating is from Min to Max.
	FieldRating = "rating"
	// FieldLanguage matches a title whose description gives the language
	// Value, an ISO 639 code.
	FieldLanguage = "language"
	// FieldType matches a movie ("movie") or a series ("series").
	FieldType = "type"
	// FieldRank matches a title among the first Max of its catalog, as
	// the catalog orders them: by popularity for a popular catalog.
	FieldRank = "rank"
	// FieldState matches what the user who opens the list did with a
	// title: Value is one of the states (see StatePlayed).
	FieldState = "state"
	// FieldRuntime matches a title whose runtime is from Min to Max
	// minutes.
	FieldRuntime = "runtime"
	// FieldContentRating matches a title rated at most Value, a rating
	// localization.RatingScore knows, on the scale parental control
	// compares ratings on: "PG-13" matches PG-13, PG and G titles.
	FieldContentRating = "contentRating"
	// FieldVotes matches a title whose IMDb rating counts from Min to Max
	// votes, as MDBList tells them.
	FieldVotes = "votes"
	// FieldPerson matches a title that credits Value, in any letter case,
	// as an actor or a director.
	FieldPerson = "person"
	// FieldStudio matches a title made by the studio or aired on the
	// network Value, in any letter case.
	FieldStudio = "studio"
	// FieldReleased matches a title released from Min to Max days after
	// today: Min -30 and Max 0 for the last 30 days, Min 0 and Max 60 for
	// the next 60.
	FieldReleased = "released"
)

// What a state rule matches: a title the user played (a series once every
// episode released is), one they did not, one they started without
// finishing it (a movie with a resume point, a series with an episode
// played or started, not every one played), or one they marked favorite.
const (
	StatePlayed   = "played"
	StateUnplayed = "unplayed"
	StateStarted  = "started"
	StateFavorite = "favorite"
)

// How a group combines its rules: a title matches all of them, or any.
const (
	MatchAll = "all"
	MatchAny = "any"
)

const (
	// maxDepth bounds how deep groups go: a list's group, groups within
	// it, and groups within those.
	maxDepth = 3
	// maxRules bounds the rules and groups of a list, all levels counted,
	// its exclusions included.
	maxRules = 100
	// maxGenreLength bounds a genre rule's genre, and a person's or a
	// studio's name, in characters.
	maxGenreLength = 100
	// maxRuntime bounds a runtime rule, in minutes, maxVotes a votes
	// rule, and maxDays a released rule, in days either way.
	maxRuntime = 1000
	maxVotes   = 100_000_000
	maxDays    = 36500
)

// Rule is a condition on titles, or a group of them. Not negates it: a
// negated rule matches the titles the rule does not.
type Rule struct {
	Field string   `json:"field"`
	Not   bool     `json:"not,omitempty"`
	Value string   `json:"value,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	// Match and Rules make a group (FieldGroup).
	Match string `json:"match,omitempty"`
	Rules []Rule `json:"rules,omitempty"`
}

// Validate checks a list's rules: a group, which may hold no rule, when
// the list keeps every title of its catalogs.
func (r Rule) Validate() error {
	count := 0
	return r.validate(&count)
}

// validate is Validate, counting the rules into count, which the rules
// and the exclusions of a list share.
func (r Rule) validate(count *int) error {
	if r.Field != FieldGroup {
		return ErrInvalidRules
	}
	if !r.valid(1, count) || *count > maxRules {
		return ErrInvalidRules
	}
	return nil
}

func (r Rule) valid(depth int, count *int) bool {
	*count++
	whole := func(f *float64) bool { return f == nil || *f == math.Trunc(*f) }
	bounded := func(low, high float64) bool {
		return (r.Min != nil || r.Max != nil) && (r.Min == nil || *r.Min >= low && *r.Min <= high) &&
			(r.Max == nil || *r.Max >= low && *r.Max <= high) && (r.Min == nil || r.Max == nil || *r.Min <= *r.Max)
	}
	plain := r.Match == "" && len(r.Rules) == 0
	switch r.Field {
	case FieldGroup:
		if r.Match != MatchAll && r.Match != MatchAny || r.Value != "" || r.Min != nil || r.Max != nil || depth > maxDepth ||
			depth > 1 && len(r.Rules) == 0 {
			return false
		}
		for _, rule := range r.Rules {
			if !rule.valid(depth+1, count) {
				return false
			}
		}
		return true
	case FieldGenre, FieldPerson, FieldStudio:
		value := strings.TrimSpace(r.Value)
		return plain && value != "" && utf8.RuneCountInString(value) <= maxGenreLength && r.Min == nil && r.Max == nil
	case FieldYear:
		return plain && r.Value == "" && whole(r.Min) && whole(r.Max) && bounded(1800, 2200)
	case FieldRating:
		return plain && r.Value == "" && bounded(0, 10)
	case FieldLanguage:
		return plain && localization.KnownLanguage(r.Value) && r.Min == nil && r.Max == nil
	case FieldType:
		return plain && (r.Value == "movie" || r.Value == "series") && r.Min == nil && r.Max == nil
	case FieldRank:
		return plain && r.Value == "" && r.Min == nil && r.Max != nil && whole(r.Max) && *r.Max >= 1 && *r.Max <= MaxItems
	case FieldState:
		return plain && r.Min == nil && r.Max == nil &&
			(r.Value == StatePlayed || r.Value == StateUnplayed || r.Value == StateStarted || r.Value == StateFavorite)
	case FieldRuntime:
		return plain && r.Value == "" && whole(r.Min) && whole(r.Max) && bounded(1, maxRuntime)
	case FieldVotes:
		return plain && r.Value == "" && whole(r.Min) && whole(r.Max) && bounded(0, maxVotes)
	case FieldReleased:
		return plain && r.Value == "" && whole(r.Min) && whole(r.Max) && bounded(-maxDays, maxDays)
	case FieldContentRating:
		_, rated := localization.RatingScore(r.Value)
		return plain && rated && r.Min == nil && r.Max == nil
	}
	return false
}

// Uses reports whether the rules look at field, at any level.
func (r Rule) Uses(field string) bool {
	if r.Field == field {
		return true
	}
	for _, rule := range r.Rules {
		if rule.Uses(field) {
			return true
		}
	}
	return false
}

// Genres lists the genres every title the rules match has: those of the
// genre rules, not negated, of a group that matches all of its rules. The
// library asks a catalog that offers one of them in its genre filter for
// that genre's titles alone, rather than reading it whole.
func (r Rule) Genres() []string {
	if r.Field != FieldGroup || r.Not || r.Match != MatchAll && len(r.Rules) > 1 {
		return nil
	}
	var genres []string
	for _, rule := range r.Rules {
		switch {
		case rule.Field == FieldGenre && !rule.Not:
			genres = append(genres, strings.TrimSpace(rule.Value))
		case rule.Field == FieldGroup:
			genres = append(genres, rule.Genres()...)
		}
	}
	return genres
}

// Languages lists the ISO 639-1 codes of the languages a language rule
// may name, as the admin app offers them.
func Languages() []string {
	var codes []string
	for _, l := range localization.Languages() {
		if l.Alpha2 != "" {
			codes = append(codes, l.Alpha2)
		}
	}
	return codes
}

// Title is what is known of a title a list judges. Described is set once
// its description was read: what neither its listing nor its description
// gives, it does not have. Until then, what its listing does not give is
// unknown, and the language always is.
type Title struct {
	Series bool
	// Genres are the genres known; GenresKnown is set once its listing
	// gave them: a title read from a catalog narrowed to a genre has that
	// one, and may have others.
	Genres      []string
	GenresKnown bool
	// Year is when it came out, 0 when not known.
	Year int
	// Rating is its IMDb rating, when Rated.
	Rating float64
	Rated  bool
	// Languages are the ISO 639-2/B codes of the languages its
	// description gives.
	Languages []string
	Described bool
	// Runtime is its runtime in minutes, 0 when not known.
	Runtime int
	// ContentRating is its age rating, empty when not known.
	ContentRating string
	// People are the names of the actors and directors it credits.
	People []string
	// Released is the day it was released, zero when not known to the
	// day: a year alone does not tell it.
	Released time.Time
	// Votes is how many votes its IMDb rating counts, once VotesKnown:
	// MDBList answered for it, 0 when MDBList has none.
	Votes      int
	VotesKnown bool
	// Studios are the studios that made it and the networks it aired on,
	// once StudiosKnown.
	Studios      []string
	StudiosKnown bool
	// Rank is its place in its catalog from 1, the best of its catalogs.
	Rank  int
	State State
	// Today is the day released rules count from.
	Today time.Time
}

// State is what the user who opens a list did with a title.
type State struct {
	Played, Started, Favorite bool
}

// Verdict is whether a title matches rules: Unknown while what they look
// at is not known yet, such as the language of a title whose description
// was not read, and Absent when it is known to be missing, such as the
// language of a title whose description gives none. A list leaves out
// the titles not known to match: what a title lacks matches no rule,
// negated or not.
type Verdict int8

const (
	No Verdict = iota
	Yes
	Unknown
	Absent
)

func verdictOf(matches bool) Verdict {
	if matches {
		return Yes
	}
	return No
}

// Judge tells whether a title matches the rule. A negated rule matches
// what the rule does not, and is as unknown, or as absent, as it.
func (r Rule) Judge(t Title) Verdict {
	v := r.judge(t)
	if r.Not && (v == Yes || v == No) {
		v = verdictOf(v == No)
	}
	return v
}

// Excludes tells whether exclusions, a group, leave a title out of a list:
// Yes when the title matches them, Unknown while that is not known. A
// group without rules excludes nothing, and a title lacking what they
// look at is not excluded for it.
func (r Rule) Excludes(t Title) Verdict {
	if len(r.Rules) == 0 {
		return No
	}
	switch v := r.Judge(t); v {
	case Yes, Unknown:
		return v
	}
	return No
}

// Keeps tells whether a list with rules and exclusions keeps a title: it
// matches the rules and not the exclusions.
func Keeps(rules, exclusions Rule, t Title) Verdict {
	v := rules.Judge(t)
	if v != Yes {
		return v
	}
	switch exclusions.Excludes(t) {
	case Yes:
		return No
	case Unknown:
		return Unknown
	}
	return Yes
}

func (r Rule) judge(t Title) Verdict {
	within := func(value float64) bool {
		return (r.Min == nil || value >= *r.Min) && (r.Max == nil || value <= *r.Max)
	}
	// What a title's description does not give, the title does not have.
	missing := Unknown
	if t.Described {
		missing = Absent
	}
	named := func(names []string) bool {
		return slices.ContainsFunc(names, func(name string) bool {
			return strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(r.Value))
		})
	}
	switch r.Field {
	case FieldGroup:
		return r.judgeGroup(t)
	case FieldGenre:
		switch {
		case named(t.Genres):
			return Yes
		case t.GenresKnown:
			return No
		}
		return missing
	case FieldYear:
		if t.Year == 0 {
			return missing
		}
		return verdictOf(within(float64(t.Year)))
	case FieldRating:
		if !t.Rated {
			return missing
		}
		return verdictOf(within(t.Rating))
	case FieldLanguage:
		for _, code := range t.Languages {
			if localization.SameLanguage(code, r.Value) {
				return Yes
			}
		}
		if t.Described && len(t.Languages) > 0 {
			return No
		}
		return missing
	case FieldType:
		return verdictOf(t.Series == (r.Value == "series"))
	case FieldRank:
		return verdictOf(t.Rank > 0 && within(float64(t.Rank)))
	case FieldState:
		switch r.Value {
		case StatePlayed:
			return verdictOf(t.State.Played)
		case StateUnplayed:
			return verdictOf(!t.State.Played)
		case StateStarted:
			return verdictOf(t.State.Started)
		case StateFavorite:
			return verdictOf(t.State.Favorite)
		}
	case FieldRuntime:
		if t.Runtime <= 0 {
			return missing
		}
		return verdictOf(within(float64(t.Runtime)))
	case FieldContentRating:
		return judgeContentRating(t.ContentRating, r.Value, missing)
	case FieldPerson:
		switch {
		case named(t.People):
			return Yes
		case len(t.People) > 0:
			return No
		}
		return missing
	case FieldReleased:
		if t.Released.IsZero() {
			return missing
		}
		day := func(at time.Time) time.Time { return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC) }
		days := math.Round(day(t.Released).Sub(day(t.Today)).Hours() / 24)
		return verdictOf(within(days))
	case FieldVotes:
		switch {
		case !t.VotesKnown:
			return Unknown
		case t.Votes <= 0:
			return Absent
		}
		return verdictOf(within(float64(t.Votes)))
	case FieldStudio:
		switch {
		case !t.StudiosKnown:
			return Unknown
		case named(t.Studios):
			return Yes
		case len(t.Studios) > 0:
			return No
		}
		return Absent
	}
	return No
}

// judgeGroup judges a group: one of its rules not matched decides a group
// of all, one matched a group of any, else one unknown leaves it unknown,
// else one whose data is absent leaves it absent. An empty group matches
// every title.
func (r Rule) judgeGroup(t Title) Verdict {
	if len(r.Rules) == 0 {
		return Yes
	}
	decides, otherwise := No, Yes
	if r.Match == MatchAny {
		decides, otherwise = Yes, No
	}
	unknown, absent := false, false
	for _, rule := range r.Rules {
		switch rule.Judge(t) {
		case decides:
			return decides
		case Unknown:
			unknown = true
		case Absent:
			absent = true
		}
	}
	switch {
	case unknown:
		return Unknown
	case absent:
		return Absent
	}
	return otherwise
}

// judgeContentRating judges a rating against the highest one a rule
// allows, as parental control does: below its score, or at its score up
// to its subscore. A title without a rating known, or rated with one that
// says it has none, is missing it.
func judgeContentRating(rating, most string, missing Verdict) Verdict {
	score, rated := localization.RatingScore(rating)
	if !rated {
		return missing
	}
	limit, _ := localization.RatingScore(most)
	if score.Score != limit.Score {
		return verdictOf(score.Score < limit.Score)
	}
	sub := func(s localization.Score) int {
		if s.SubScore == nil {
			return 0
		}
		return *s.SubScore
	}
	return verdictOf(sub(score) <= sub(limit))
}
