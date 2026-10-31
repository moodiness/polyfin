package smartlists

import (
	"errors"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func group(match string, rules ...Rule) Rule {
	return Rule{Field: FieldGroup, Match: match, Rules: rules}
}

// A rule on what a title's description gives is unknown, negated or not,
// until the description is read; read, what it does not give the title
// lacks, and a rule on it matches neither negated nor not. A group is
// unknown while a rule that would decide it is.
func TestRulesAreUnknownUntilTheTitleIsDescribed(t *testing.T) {
	t.Parallel()
	french := Rule{Field: FieldLanguage, Value: "fr"}
	notFrench := Rule{Field: FieldLanguage, Value: "fr", Not: true}
	listed := Title{Genres: []string{"Comedy"}, GenresKnown: true, Year: 1995}
	described := listed
	described.Described, described.Languages = true, []string{"fre"}
	for _, c := range []struct {
		name  string
		rule  Rule
		title Title
		want  Verdict
	}{
		{"language not read", french, listed, Unknown},
		{"negated language not read", notFrench, listed, Unknown},
		{"language read", french, described, Yes},
		{"negated language read", notFrench, described, No},
		{"no language given", french, Title{Described: true}, Absent},
		{"negated, no language given", notFrench, Title{Described: true}, Absent},
		{"all with one unknown", group(MatchAll, Rule{Field: FieldGenre, Value: "comedy"}, french), listed, Unknown},
		{"all with one false", group(MatchAll, Rule{Field: FieldGenre, Value: "Drama"}, french), listed, No},
		{"any with one true", group(MatchAny, Rule{Field: FieldGenre, Value: "Comedy"}, french), listed, Yes},
		{"negated group", Rule{Field: FieldGroup, Match: MatchAny, Not: true, Rules: []Rule{{Field: FieldGenre, Value: "Comedy"}}}, listed, No},
		{"rating not given", Rule{Field: FieldRating, Min: new(7.0)}, listed, Unknown},
		{"rating described as none", Rule{Field: FieldRating, Min: new(7.0)}, Title{Described: true}, Absent},
		{"genre of a narrowed catalog", Rule{Field: FieldGenre, Value: "Comedy"}, Title{Genres: []string{"Comedy"}}, Yes},
		{"other genre of a narrowed catalog", Rule{Field: FieldGenre, Value: "Drama"}, Title{Genres: []string{"Comedy"}}, Unknown},
		{"years", Rule{Field: FieldYear, Min: new(1990.0), Max: new(1999.0)}, listed, Yes},
		{"rank", Rule{Field: FieldRank, Max: new(10.0)}, Title{Rank: 11}, No},
		{"started", Rule{Field: FieldState, Value: StateStarted}, Title{State: State{Started: true}}, Yes},
		{"unplayed", Rule{Field: FieldState, Value: StateUnplayed}, Title{State: State{Played: true}}, No},
		{"empty group", group(MatchAny), listed, Yes},
	} {
		if got := c.rule.Judge(c.title); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRulesAreValidated(t *testing.T) {
	t.Parallel()
	valid := []Rule{
		group(MatchAll),
		group(MatchAny, Rule{Field: FieldYear, Min: new(1990.0)}, group(MatchAll, Rule{Field: FieldLanguage, Value: "fre"})),
		group(MatchAll, Rule{Field: FieldRank, Max: new(100.0)}, Rule{Field: FieldState, Value: StateFavorite, Not: true}),
		group(MatchAll,
			Rule{Field: FieldRuntime, Min: new(60.0), Max: new(120.0)},
			Rule{Field: FieldContentRating, Value: "PG-13"},
			Rule{Field: FieldVotes, Min: new(1000.0)},
			Rule{Field: FieldPerson, Value: "Some Actor"},
			Rule{Field: FieldStudio, Value: "Some Studio", Not: true},
			Rule{Field: FieldReleased, Min: new(-30.0), Max: new(0.0)},
		),
	}
	for _, rules := range valid {
		if err := rules.Validate(); err != nil {
			t.Errorf("%+v: %v", rules, err)
		}
	}
	invalid := []Rule{
		group(MatchAll, Rule{Field: FieldRuntime, Min: new(0.0)}),
		group(MatchAll, Rule{Field: FieldRuntime, Min: new(90.5)}),
		group(MatchAll, Rule{Field: FieldContentRating, Value: "Unrated"}),
		group(MatchAll, Rule{Field: FieldContentRating, Value: "PG-13", Max: new(1.0)}),
		group(MatchAll, Rule{Field: FieldVotes, Min: new(-1.0)}),
		group(MatchAll, Rule{Field: FieldPerson, Value: "  "}),
		group(MatchAll, Rule{Field: FieldStudio}),
		group(MatchAll, Rule{Field: FieldReleased}),
		group(MatchAll, Rule{Field: FieldReleased, Min: new(-40000.0)}),
		{Field: FieldGenre, Value: "Comedy"},
		group("some"),
		group(MatchAll, group(MatchAny)),
		group(MatchAll, Rule{Field: FieldYear}),
		group(MatchAll, Rule{Field: FieldYear, Min: new(1999.5)}),
		group(MatchAll, Rule{Field: FieldYear, Min: new(2000.0), Max: new(1990.0)}),
		group(MatchAll, Rule{Field: FieldRating, Max: new(11.0)}),
		group(MatchAll, Rule{Field: FieldLanguage, Value: "elvish"}),
		group(MatchAll, Rule{Field: FieldType, Value: "episode"}),
		group(MatchAll, Rule{Field: FieldRank, Min: new(1.0), Max: new(10.0)}),
		group(MatchAll, Rule{Field: FieldState, Value: "liked"}),
		group(MatchAll, group(MatchAll, group(MatchAll, group(MatchAll, Rule{Field: FieldType, Value: "movie"})))),
	}
	for _, rules := range invalid {
		if err := rules.Validate(); !errors.Is(err, ErrInvalidRules) {
			t.Errorf("%+v: %v", rules, err)
		}
	}
}

// What a title is known to lack matches no rule, negated or not: a title
// without a runtime is neither long nor not long. A rule on what is not
// known yet stays unknown either way.
func TestLackingDataMatchesNoRuleNegatedOrNot(t *testing.T) {
	t.Parallel()
	described := Title{Described: true, VotesKnown: true, StudiosKnown: true, Today: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)}
	listed := Title{Today: described.Today}
	for _, rule := range []Rule{
		{Field: FieldGenre, Value: "Drama"},
		{Field: FieldYear, Min: new(1990.0)},
		{Field: FieldRating, Min: new(7.0)},
		{Field: FieldLanguage, Value: "fr"},
		{Field: FieldRuntime, Min: new(120.0)},
		{Field: FieldContentRating, Value: "PG-13"},
		{Field: FieldPerson, Value: "Some Actor"},
		{Field: FieldReleased, Min: new(-30.0), Max: new(0.0)},
		{Field: FieldVotes, Min: new(1000.0)},
		{Field: FieldStudio, Value: "Some Studio"},
	} {
		negated := rule
		negated.Not = true
		for _, r := range []Rule{rule, negated} {
			if got := r.Judge(described); got != Absent {
				t.Errorf("%s (not %v) lacking: %v, want absent", r.Field, r.Not, got)
			}
			if got := r.Judge(listed); got != Unknown {
				t.Errorf("%s (not %v) not known yet: %v, want unknown", r.Field, r.Not, got)
			}
			// Within a list, neither matches: lacking is left out, not
			// known yet waits.
			if got := Keeps(group(MatchAll, r), NoExclusions(), described); got == Yes {
				t.Errorf("%s (not %v) lacking is kept", r.Field, r.Not)
			}
		}
	}
	long := Rule{Field: FieldRuntime, Min: new(120.0)}
	drama := Rule{Field: FieldGenre, Value: "Drama"}
	comedy := Title{Described: true, Genres: []string{"Comedy"}, GenresKnown: true}
	for _, c := range []struct {
		name string
		rule Rule
		want Verdict
	}{
		// Not a drama, whatever its runtime: "not (drama and long)" holds.
		{"negated all, one false", Rule{Field: FieldGroup, Match: MatchAll, Not: true, Rules: []Rule{drama, long}}, Yes},
		// Not a drama, runtime lacking: "not (drama or long)" is not known
		// to hold.
		{"negated any, one false", Rule{Field: FieldGroup, Match: MatchAny, Not: true, Rules: []Rule{drama, long}}, Absent},
		{"any, one false", group(MatchAny, drama, long), Absent},
	} {
		if got := c.rule.Judge(comedy); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNewRulesJudgeWhatTitlesAre(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	title := Title{Described: true, Runtime: 95, ContentRating: "PG", People: []string{"Ann Actor", "Dee Director"},
		Released: today.AddDate(0, 0, -10), Votes: 5400, VotesKnown: true, Studios: []string{"North Studio"}, StudiosKnown: true, Today: today}
	for _, c := range []struct {
		name string
		rule Rule
		want Verdict
	}{
		{"runtime within", Rule{Field: FieldRuntime, Min: new(90.0), Max: new(120.0)}, Yes},
		{"runtime too short", Rule{Field: FieldRuntime, Min: new(100.0)}, No},
		{"rated at most PG-13", Rule{Field: FieldContentRating, Value: "PG-13"}, Yes},
		{"rated at most G", Rule{Field: FieldContentRating, Value: "G"}, No},
		{"rated above G", Rule{Field: FieldContentRating, Value: "G", Not: true}, Yes},
		{"rated at most its own", Rule{Field: FieldContentRating, Value: "pg"}, Yes},
		{"enough votes", Rule{Field: FieldVotes, Min: new(5000.0)}, Yes},
		{"too few votes", Rule{Field: FieldVotes, Min: new(10000.0)}, No},
		{"actor in any case", Rule{Field: FieldPerson, Value: "ann actor"}, Yes},
		{"director", Rule{Field: FieldPerson, Value: "Dee Director"}, Yes},
		{"someone else", Rule{Field: FieldPerson, Value: "Ed Else"}, No},
		{"studio", Rule{Field: FieldStudio, Value: "north studio"}, Yes},
		{"other studio", Rule{Field: FieldStudio, Value: "South Studio"}, No},
		{"released in the last 30 days", Rule{Field: FieldReleased, Min: new(-30.0), Max: new(0.0)}, Yes},
		{"released in the last week", Rule{Field: FieldReleased, Min: new(-7.0), Max: new(0.0)}, No},
		{"released in the next 60 days", Rule{Field: FieldReleased, Min: new(0.0), Max: new(60.0)}, No},
	} {
		if got := c.rule.Judge(title); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	upcoming := title
	upcoming.Released = today.Add(59*24*time.Hour + 23*time.Hour)
	if got := (Rule{Field: FieldReleased, Min: new(0.0), Max: new(60.0)}).Judge(upcoming); got != Yes {
		t.Errorf("released in 60 days: %v", got)
	}
}

// Exclusions leave out the titles they match: not those lacking what they
// look at, and those not known yet until they are.
func TestExclusionsLeaveOutWhatTheyMatch(t *testing.T) {
	t.Parallel()
	rules := group(MatchAll, Rule{Field: FieldType, Value: "series"})
	exclude := group(MatchAny, Rule{Field: FieldGenre, Value: "Animation"}, Rule{Field: FieldRuntime, Max: new(15.0)})
	for _, c := range []struct {
		name  string
		title Title
		want  Verdict
	}{
		{"matched by an exclusion", Title{Series: true, Described: true, Genres: []string{"Animation"}, GenresKnown: true}, No},
		{"matched by none", Title{Series: true, Described: true, Genres: []string{"Drama"}, GenresKnown: true, Runtime: 50}, Yes},
		{"lacking what they look at", Title{Series: true, Described: true, Genres: []string{"Drama"}, GenresKnown: true}, Yes},
		{"not known yet", Title{Series: true}, Unknown},
		{"not matched by the rules", Title{Described: true, Genres: []string{"Animation"}, GenresKnown: true}, No},
	} {
		if got := Keeps(rules, exclude, c.title); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if got := Keeps(rules, NoExclusions(), Title{Series: true}); got != Yes {
		t.Errorf("no exclusions: %v", got)
	}
}

// A user's own lists are reached in their scope alone: another scope does
// not find, change or delete them.
func TestListsStayInTheirScope(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	member, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	store := New(pool)
	addon := accounts.ID{1}
	own, err := store.Create(t.Context(), List{Owner: &member.ID, Name: " Mine ", ShownAs: ShownAsCollection,
		Catalogs: []Catalog{{AddonID: addon, Type: "movie", ID: "top"}}, Rules: group(MatchAll), Sort: SortName, MaxItems: 50})
	if err != nil || own.Name != "Mine" {
		t.Fatalf("created: %+v %v", own, err)
	}
	if _, err := store.Get(t.Context(), nil, own.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the server's scope finds it: %v", err)
	}
	moved := own
	moved.Owner = nil
	if _, err := store.Update(t.Context(), moved); !errors.Is(err, ErrNotFound) {
		t.Errorf("the server's scope changes it: %v", err)
	}
	if err := store.Delete(t.Context(), nil, own.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the server's scope deletes it: %v", err)
	}
	got, err := store.Get(t.Context(), &member.ID, own.ID)
	if err != nil || got.MaxItems != 50 || len(got.Catalogs) != 1 || got.Catalogs[0].AddonID != addon || got.ShownAs != ShownAsCollection {
		t.Errorf("read back: %+v %v", got, err)
	}
	seen, err := store.Seen(t.Context(), member.ID, true, false)
	if err != nil || len(seen) != 0 {
		t.Errorf("seen without their own: %v %v", seen, err)
	}
	if _, err := store.Create(t.Context(), List{Name: "No catalog", ShownAs: ShownAsLibrary, Rules: group(MatchAll), Sort: SortName}); !errors.Is(err, ErrInvalidCatalogs) {
		t.Errorf("a list without catalogs: %v", err)
	}
}

// A list keeps its exclusions and a sort by episodes; one saved without
// exclusions reads back with none.
func TestListsKeepTheirExclusions(t *testing.T) {
	t.Parallel()
	store := New(migrated.New(t))
	catalogs := []Catalog{{AddonID: accounts.ID{1}, Type: "series", ID: "top"}}
	exclude := group(MatchAny, Rule{Field: FieldStudio, Value: "Some Studio"})
	saved, err := store.Create(t.Context(), List{Name: "Shows", ShownAs: ShownAsLibrary, Catalogs: catalogs, Rules: group(MatchAll),
		Exclude: exclude, Sort: SortLatestEpisode})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), nil, saved.ID)
	if err != nil || got.Sort != SortLatestEpisode || len(got.Exclude.Rules) != 1 || got.Exclude.Rules[0].Value != "Some Studio" {
		t.Fatalf("read back: %+v %v", got, err)
	}
	got.Exclude, got.Sort = Rule{}, SortSeasonPremiere
	if _, err := store.Update(t.Context(), got); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(t.Context(), nil, saved.ID)
	if err != nil || got.Sort != SortSeasonPremiere || got.Exclude.Field != FieldGroup || len(got.Exclude.Rules) != 0 {
		t.Fatalf("without exclusions: %+v %v", got, err)
	}
	tooMany := List{Name: "Too many", ShownAs: ShownAsLibrary, Catalogs: catalogs, Sort: SortName}
	tooMany.Rules, tooMany.Exclude = group(MatchAll), group(MatchAny)
	for range 60 {
		tooMany.Rules.Rules = append(tooMany.Rules.Rules, Rule{Field: FieldType, Value: "movie"})
		tooMany.Exclude.Rules = append(tooMany.Exclude.Rules, Rule{Field: FieldType, Value: "series"})
	}
	if err := tooMany.Validate(); !errors.Is(err, ErrInvalidRules) {
		t.Errorf("rules and exclusions past the bound together: %v", err)
	}
}
