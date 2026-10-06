package localization

import (
	"strconv"
	"strings"
)

// Score is where a rating sits on the scale parental control compares
// ratings on. Score is the youngest age the rating suits; ratings for adults
// only sit at 1000 and above, past any age. SubScore, given by the US
// systems only, sets apart the ratings of one age that warn of more: TV-Y7-FV
// after TV-Y7, the TV ratings with content descriptors after their plain
// rating, NC-17 and TV-MA after R. A missing SubScore counts as 0.
type Score struct {
	Score    int
	SubScore *int
}

// Rating is a rating apps offer as a user's limit. Score is nil for the
// one that stands for titles without a rating.
type Rating struct {
	Name  string
	Score *Score
}

// Ratings returns the ratings apps offer as a user's limit, as Jellyfin
// lists those of its default metadata country: the US movie ratings of the
// Motion Picture Association and the TV Parental Guidelines, which are the
// ratings addons give (AIOMetadata's certification is always the US one),
// then the ages and grades every system shares past them. The unrated
// entry comes first, then by score and subscore; ratings of one score and
// subscore keep the order of their system. Ratings of the other systems
// (see RatingScore) map onto these scores, so any limit covers them. The
// list is shared: callers must not change it.
func Ratings() []Rating {
	return ratings
}

// RatingScore returns the score of a title's rating. It reads the US
// ratings above, those of France (TP, -10 to -18, also written with an en
// dash or as "Interdit aux moins de 12 ans"), Germany (FSK 0 to FSK 18,
// "ab 12"), the United Kingdom (U, 12A, R18), P for adult works, and plain
// ages ("12", "16+"), written in any case and possibly preceded by "Rated"
// or by the country ("US:PG-13", "FR-12", "DE-FSK-16"). Where systems share
// a name, as PG, the US meaning wins, as the addons' ratings are mostly the
// US ones. As in Jellyfin 12.2, a list of ratings separated by "/" ("PG-13
// / 12") has the score of the first one known, when the whole is not one.
// ok is false for a title without a rating, which includes the ratings
// that say so (NR, Unrated) and those Polyfin does not know.
func RatingScore(rating string) (score Score, ok bool) {
	value := strings.ToLower(strings.TrimSpace(rating))
	value = strings.TrimSpace(strings.TrimPrefix(value, "rated "))
	if score, ok := scores[value]; ok {
		return score, true
	}
	if age, ok := plainAge(value); ok {
		return Score{Score: age}, true
	}
	for _, prefix := range []string{"fsk", "ab ", "interdit aux moins de "} {
		if rest, ok := strings.CutPrefix(value, prefix); ok {
			if age, ok := plainAge(strings.TrimSuffix(strings.TrimLeft(rest, " -"), " ans")); ok {
				return Score{Score: age}, true
			}
		}
	}
	if strings.Contains(value, "/") {
		for part := range strings.SplitSeq(value, "/") {
			if part = strings.TrimSpace(part); part != "" {
				if score, ok := RatingScore(part); ok {
					return score, true
				}
			}
		}
		return Score{}, false
	}
	for _, country := range []string{"us", "fr", "de", "gb", "uk"} {
		if rest, ok := strings.CutPrefix(value, country); ok && rest != "" && strings.ContainsRune(":- ", rune(rest[0])) {
			return RatingScore(rest[1:])
		}
	}
	return Score{}, false
}

// plainAge reads an age written alone, as Germany and the United Kingdom
// do, with the "+" some systems add or the "-" of French television,
// which is also written with an en dash.
func plainAge(value string) (int, bool) {
	value = strings.TrimPrefix(value, "-")
	value = strings.TrimSuffix(strings.TrimPrefix(value, "–"), "+")
	age, err := strconv.Atoi(value)
	if err != nil || age < 0 || age > 99 || value[0] == '+' {
		return 0, false
	}
	return age, true
}

// usRatings are the US ratings by score, in the order apps list them. The
// TV ratings with content descriptors (dialogue, language, sex, violence)
// are written out, as apps list each.
var usRatings = []struct {
	score, subScore int
	names           []string
}{
	{0, 0, []string{"Approved", "G", "TV-G", "TV-Y"}},
	{7, 0, []string{"TV-Y7"}},
	{7, 1, []string{"TV-Y7-FV"}},
	{10, 0, []string{"PG", "TV-PG"}},
	{10, 1, withDescriptors("TV-PG", "DLSV")},
	{13, 0, []string{"PG-13"}},
	{14, 0, []string{"TV-14"}},
	{14, 1, withDescriptors("TV-14", "DLSV")},
	{17, 0, []string{"R"}},
	{17, 1, append([]string{"NC-17", "TV-MA"}, withDescriptors("TV-MA", "LSV")...)},
	{18, 0, []string{"TV-X", "TV-AO"}},
}

// withDescriptors names a TV rating with each combination of its content
// descriptors, fewer first, letters in the order given.
func withDescriptors(rating, letters string) []string {
	var names []string
	for size := 1; size <= len(letters); size++ {
		var combine func(prefix string, from int)
		combine = func(prefix string, from int) {
			if len(prefix) == size {
				names = append(names, rating+"-"+prefix)
				return
			}
			for i := from; i < len(letters); i++ {
				combine(prefix+letters[i:i+1], i+1)
			}
		}
		combine("", 0)
	}
	return names
}

// beyondUS are the limits apps offer past the US ratings: an age above
// any rating's, then adult only and banned titles.
var beyondUS = []struct {
	name  string
	score int
}{{"21", 21}, {"XXX", 1000}, {"Banned", 1001}}

var ratings = func() []Rating {
	list := []Rating{{Name: "Unrated"}}
	for _, group := range usRatings {
		for _, name := range group.names {
			list = append(list, Rating{Name: name, Score: &Score{Score: group.score, SubScore: new(group.subScore)}})
		}
	}
	for _, entry := range beyondUS {
		list = append(list, Rating{Name: entry.name, Score: &Score{Score: entry.score}})
	}
	return list
}()

// scores finds a rating's score by its lower-case name: the listed ratings,
// then the names of other systems that are not plain ages.
var scores = func() map[string]Score {
	result := map[string]Score{
		// France: movies and television for all; the CNC also writes "tous
		// publics" out.
		"tp":           {Score: 0},
		"tous publics": {Score: 0},
		// United Kingdom (BBFC): U for all, 12A for 12 and over unless with
		// an adult, R18 for licensed adult works.
		"u":   {Score: 0},
		"12a": {Score: 12},
		"r18": {Score: 1000},
		// P, for adult works, which Jellyfin 12.2 reads in every country.
		"p": {Score: 1000},
	}
	for _, rating := range ratings {
		if rating.Score != nil {
			result[strings.ToLower(rating.Name)] = *rating.Score
		}
	}
	return result
}()
