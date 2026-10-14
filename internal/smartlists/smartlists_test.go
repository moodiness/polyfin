package smartlists

import (
	"errors"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func group(match string, rules ...Rule) Rule {
	return Rule{Field: FieldGroup, Match: match, Rules: rules}
}

// A rule on what a title's description gives is unknown, negated or not,
// until the description is read; read, what it does not give the title
// does not have. A group is unknown while a rule that would decide it is.
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
		{"no language given", french, Title{Described: true}, No},
		{"all with one unknown", group(MatchAll, Rule{Field: FieldGenre, Value: "comedy"}, french), listed, Unknown},
		{"all with one false", group(MatchAll, Rule{Field: FieldGenre, Value: "Drama"}, french), listed, No},
		{"any with one true", group(MatchAny, Rule{Field: FieldGenre, Value: "Comedy"}, french), listed, Yes},
		{"negated group", Rule{Field: FieldGroup, Match: MatchAny, Not: true, Rules: []Rule{{Field: FieldGenre, Value: "Comedy"}}}, listed, No},
		{"rating not given", Rule{Field: FieldRating, Min: new(7.0)}, listed, Unknown},
		{"rating described as none", Rule{Field: FieldRating, Min: new(7.0)}, Title{Described: true}, No},
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
	}
	for _, rules := range valid {
		if err := rules.Validate(); err != nil {
			t.Errorf("%+v: %v", rules, err)
		}
	}
	invalid := []Rule{
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
