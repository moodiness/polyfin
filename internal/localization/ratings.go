package localization

import (
	"slices"
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

// DefaultRatingCountry is the rating country of a server that chose none:
// the US, whose ratings addons give as their certification.
const DefaultRatingCountry = "US"

// RatingCountries returns the countries whose ratings Polyfin knows, by
// their ISO 3166-1 alpha-2 code, in alphabetical order. The list is
// shared: callers must not change it.
func RatingCountries() []string {
	return ratingCountries
}

// ValidRatingCountry reports whether code, an ISO 3166-1 alpha-2 code in
// upper case, is one of RatingCountries.
func ValidRatingCountry(code string) bool {
	_, ok := systems[code]
	return ok
}

// Ratings returns the ratings apps offer as a user's limit under country
// (see RatingCountries; another country, or none, is the US):
//   - for the US, as Jellyfin lists them, the movie ratings of the Motion
//     Picture Association and the TV Parental Guidelines with their
//     subscores, then the age and grades past them (21, XXX, Banned);
//   - for another country, one entry per level its official systems give,
//     named by the ratings titles show at that level joined by " / "
//     ("VM14 / 14+"), so that a parent reads the choice as they read the
//     ratings; then XXX and Banned, unless a rating of the country already
//     stands at that level.
//
// The unrated entry comes first, then by score and subscore; ratings of
// one score and subscore keep the order of their system. A user's limit is
// a score, so it keeps its meaning whatever the country, even where that
// country's list has no entry at that score. The list is shared: callers
// must not change it.
func Ratings(country string) []Rating {
	return systemOf(country).list
}

// RatingScore returns the score of a title's rating under country (see
// RatingCountries; another country, or none, is the US), as Jellyfin's
// GetRatingScore finds it, written in any case and possibly preceded by
// "Rated":
//  1. among the ratings of country, and the names of its limits in Ratings
//     ("VM14 / 14+"), which smart lists keep as rule values;
//  2. among the US ratings, those other systems share (U, TP or "tous
//     publics" for all, 12A, R18 and P for adult works), the ages written
//     alone ("12", "16+", "-12"), and the German and French forms "FSK 12",
//     "ab 12" and "Interdit aux moins de 12 ans";
//  3. preceded by a country Polyfin knows ("IT-VM14", "de:FSK 16",
//     "GB 15"), among that country's ratings, then as above;
//  4. as a list separated by "/" ("PG-13 / 12"), the first of its ratings
//     known by the above, when the whole is not one, as in Jellyfin 12.2;
//  5. among the ratings of every other country: the highest score of
//     those that name it, so that a rating several systems give different
//     meanings is judged by the strictest.
//
// ok is false for a title without a rating, which includes the ratings
// that say so (NR, Unrated) and those Polyfin does not know.
func RatingScore(rating, country string) (score Score, ok bool) {
	value := strings.ToLower(strings.TrimSpace(rating))
	value = strings.TrimSpace(strings.TrimPrefix(value, "rated "))
	if _, unrated := unratedNames[key(value)]; unrated || value == "" {
		return Score{}, false
	}
	if score, ok := scoreNear(value, systemOf(country)); ok {
		return score, true
	}
	if strings.Contains(value, "/") {
		for part := range strings.SplitSeq(value, "/") {
			if part = strings.TrimSpace(part); part != "" {
				if score, ok := scoreNear(part, systemOf(country)); ok {
					return score, true
				}
			}
		}
	}
	found := false
	for _, code := range ratingCountries {
		if s, ok := systems[code].scores[key(value)]; ok && (!found || s.Score > score.Score) {
			score, found = s, true
		}
	}
	return score, found
}

// CountryScore returns the score of rating among the ratings of country
// alone (see RatingCountries; another country, or none, is the US), also
// preceded by that country ("IT-VM14"): it tells whether a rating is one
// of that country's. ok is false otherwise.
func CountryScore(rating, country string) (score Score, ok bool) {
	value := strings.ToLower(strings.TrimSpace(rating))
	value = strings.TrimSpace(strings.TrimPrefix(value, "rated "))
	system := systemOf(country)
	if score, ok := system.scores[key(value)]; ok {
		return score, true
	}
	if code, rest, ok := countryPrefix(value); ok && systems[code] == system {
		score, ok := system.scores[key(rest)]
		return score, ok
	}
	return Score{}, false
}

// scoreNear scores value, in lower case, by steps 1 to 3 of RatingScore.
func scoreNear(value string, system *ratingSystem) (Score, bool) {
	if score, ok := system.scores[key(value)]; ok {
		return score, true
	}
	if score, ok := shared[key(value)]; ok {
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
	if code, rest, ok := countryPrefix(value); ok {
		return scoreNear(strings.TrimSpace(rest), systems[code])
	}
	return Score{}, false
}

// countryPrefix splits a rating preceded by a country Polyfin knows, in
// lower case, and ":", "-" or a space: "it-vm14" gives IT and "vm14". The
// United Kingdom is also written UK.
func countryPrefix(value string) (code, rest string, ok bool) {
	if len(value) < 4 || !strings.ContainsRune(":- ", rune(value[2])) {
		return "", "", false
	}
	code = strings.ToUpper(value[:2])
	if code == "UK" {
		code = "GB"
	}
	if _, known := systems[code]; !known {
		return "", "", false
	}
	return code, value[3:], true
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

// key is how ratings are looked up: in lower case, without the spaces,
// hyphens, en dashes and dots their writings differ by ("FSK 12",
// "FSK-12", "fsk12"; "B-15", "B15"; "-12", "12").
func key(value string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(" -–.", r) {
			return -1
		}
		return r
	}, strings.ToLower(value))
}

// unratedNames are the ratings that say a title has none, by key.
var unratedNames = func() map[string]struct{} {
	names := map[string]struct{}{}
	for _, name := range []string{"NR", "Not Rated", "Unrated", "UR", "N/A", "NA", "None", "Exempt"} {
		names[key(name)] = struct{}{}
	}
	return names
}()

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

// beyondUS are the limits the US list offers past its ratings: an age
// above any rating's, then adult only and banned titles. Their names are
// read in every country; other countries list the last two alone, at
// levels their own ratings leave empty.
var beyondUS = []struct {
	name  string
	score int
}{{"21", 21}, {"XXX", 1000}, {"Banned", 1001}}

// shared are the ratings read in every country (step 2 of RatingScore)
// that are not ages written alone, by key: the US ones, those past them,
// and those other systems share.
var shared = func() map[string]Score {
	result := map[string]Score{
		// France: movies and television for all; the CNC also writes "tous
		// publics" out.
		"tp":          {Score: 0},
		"touspublics": {Score: 0},
		// United Kingdom (BBFC): U for all, 12A for 12 and over unless with
		// an adult, R18 for licensed adult works.
		"u":   {Score: 0},
		"12a": {Score: 12},
		"r18": {Score: 1000},
		// P, for adult works, which Jellyfin 12.2 reads in every country.
		"p": {Score: 1000},
	}
	for _, group := range usRatings {
		for _, name := range group.names {
			result[key(name)] = Score{Score: group.score, SubScore: new(group.subScore)}
		}
	}
	for _, entry := range beyondUS {
		result[key(entry.name)] = Score{Score: entry.score}
	}
	return result
}()

// countryRating is a rating of a country's system: listed among the
// limits apps offer, or another writing of a listed one.
type countryRating struct {
	name  string
	score int
	alias bool
}

func listed(name string, score int) countryRating { return countryRating{name: name, score: score} }

func alias(name string, score int) countryRating {
	return countryRating{name: name, score: score, alias: true}
}

// ages lists plain ages, as the systems that name their ratings by the
// youngest age they suit write them.
func ages(list ...int) []countryRating {
	result := make([]countryRating, len(list))
	for i, age := range list {
		result[i] = listed(strconv.Itoa(age), age)
	}
	return result
}

// countrySystems are the ratings of each country but the US (see
// usRatings), written for Polyfin from the categories each country's
// classification body publishes, in the order apps list them. Each is
// scored by the youngest age it suits; ratings for adult works only score
// 1000, and refused ones 1001, as Banned. The listed ratings of one score
// make one entry of Ratings; an alias is another writing titles show,
// scored but not named there.
var countrySystems = map[string][]countryRating{
	// Argentina: the Instituto Nacional de Cine y Artes Audiovisuales
	// (INCAA): apta para todo público, then solo apta para mayores de 13, 16
	// and 18, and C, for 18 with restrictions.
	"AR": {listed("ATP", 0), listed("+13", 13), alias("SAM 13", 13), listed("+16", 16), alias("SAM 16", 16),
		listed("+18", 18), alias("SAM 18", 18), listed("C", 18)},
	// Austria: the Jugendmedienkommission (JMK) of the federal ministry of
	// education, by age up to 16.
	"AT": ages(0, 6, 8, 10, 12, 14, 16),
	// Australia: the Australian Classification Board: G, PG, M (mature,
	// advised from 15), MA15+, R18+, X18+ for adult works, RC for refused.
	"AU": {listed("G", 0), listed("PG", 8), listed("M", 15), listed("MA15+", 15), alias("AV15+", 15),
		listed("R18+", 18), listed("X18+", 1000), listed("RC", 1001)},
	// Belgium: Cinecheck, the Belgian ratings by age, and the former KT
	// (kinderen toegelaten, enfants admis) and KNT (kinderen niet
	// toegelaten, enfants non admis, 16 and over).
	"BE": append([]countryRating{listed("AL", 0), alias("KT", 0), alias("EA", 0)},
		append(ages(6, 9, 12, 14, 16, 18), alias("KNT", 16), alias("ENA", 16))...),
	// Brazil: the Classificação Indicativa (ClassInd) of the Ministério da
	// Justiça: L (livre), then by age; "A" before an age marks a rating
	// its producer gave.
	"BR": append([]countryRating{listed("L", 0), alias("AL", 0), alias("Livre", 0)},
		append(ages(10, 12, 14, 16, 18), alias("A10", 10), alias("A12", 12), alias("A14", 14), alias("A16", 16),
			alias("A18", 18))...),
	// Canada: the provincial film boards' Canadian Home Video Rating
	// System (G, PG, 14A, 18A, R), Québec's Régie du cinéma (G, 13+, 16+,
	// 18+), and the Canadian TV classification (C, C8, G, PG, 14+, 18+).
	"CA": {listed("G", 0), listed("C", 0), listed("PG", 8), listed("C8", 8), listed("13+", 13), listed("14A", 14),
		listed("14+", 14), listed("16+", 16), listed("18A", 18), listed("18+", 18), listed("R", 18)},
	// Switzerland: the Schweizerische Kommission Jugendschutz im Film, by
	// age.
	"CH": ages(0, 6, 8, 10, 12, 14, 16, 18),
	// Czechia: the ratings of the audiovisual act (U, přístupný bez
	// omezení, for all; then by age).
	"CZ": append([]countryRating{listed("U", 0)}, ages(12, 15, 18)...),
	// Germany: the Freiwillige Selbstkontrolle der Filmwirtschaft (FSK),
	// by age, also written "FSK 12".
	"DE": append(ages(0, 6, 12, 16, 18), alias("FSK 0", 0), alias("FSK 6", 6), alias("FSK 12", 12),
		alias("FSK 16", 16), alias("FSK 18", 18)),
	// Denmark: Medierådet for Børn og Unge: A (alle), then by age.
	"DK": append([]countryRating{listed("A", 0)}, ages(7, 11, 15)...),
	// Spain: the Instituto de la Cinematografía y de las Artes
	// Audiovisuales (ICAA): A (apta para todos los públicos), Ai (also
	// recommended for children), 7 and 7i, then by age, X for adult works;
	// and TP (todos los públicos) of the CNMC's television code.
	"ES": append([]countryRating{listed("A", 0), listed("Ai", 0), listed("TP", 0), listed("7", 7), listed("7i", 7)},
		append(ages(12, 16, 18), listed("X", 1000))...),
	// Finland: the Kansallinen audiovisuaalinen instituutti (KAVI): S
	// (sallittu, T in Swedish) for all, then K-7 to K-18.
	"FI": {listed("S", 0), alias("T", 0), listed("K-7", 7), alias("7", 7), listed("K-12", 12), alias("12", 12),
		listed("K-16", 16), alias("16", 16), listed("K-18", 18), alias("18", 18)},
	// France: the visas of the Centre national du cinéma (CNC): tous
	// publics (TP, also U), then forbidden under 10, 12, 16 and 18, also
	// written "-12" by the Arcom on television; X for adult works.
	"FR": {listed("TP", 0), listed("U", 0), alias("Tous publics", 0), listed("10", 10), listed("12", 12),
		listed("16", 16), listed("18", 18), listed("X", 1000)},
	// United Kingdom: the British Board of Film Classification (BBFC): U,
	// PG, 12A and 12, 15, 18, R18 for licensed adult works.
	"GB": {listed("U", 0), alias("Uc", 0), listed("PG", 8), listed("12A", 12), listed("12", 12), listed("15", 15),
		listed("18", 18), listed("R18", 1000)},
	// Ireland: the Irish Film Classification Office (IFCO): G, PG, 12A and
	// 12, 15A and 15, 16, 18.
	"IE": {listed("G", 0), listed("PG", 8), listed("12A", 12), listed("12", 12), listed("15A", 15),
		listed("15", 15), listed("16", 16), listed("18", 18)},
	// Italy: the Commissione per la classificazione delle opere
	// cinematografiche of the Ministero della Cultura (T, 6+, 14+, 18+
	// since 2021; VM14 and VM18 before, which titles still show), and the
	// AGCOM's television signs (T, BA for children with an adult, VM12,
	// VM14, VM18).
	"IT": {listed("T", 0), alias("Per tutti", 0), listed("6+", 6), alias("VM6", 6), listed("BA", 10),
		listed("VM12", 12), listed("VM14", 14), listed("14+", 14), listed("VM18", 18), listed("18+", 18)},
	// Japan: the Film Classification and Rating Organization (Eirin): G,
	// PG12, R15+, R18+.
	"JP": {listed("G", 0), listed("PG12", 12), listed("R15+", 15), listed("R18+", 18)},
	// South Korea: the Korea Media Rating Board (KMRB): all, 12, 15,
	// 18 (청소년관람불가), restricted screening; and 7 and 19 of television.
	"KR": {listed("ALL", 0), alias("전체관람가", 0), listed("7", 7), listed("12", 12), alias("12세이상관람가", 12),
		listed("15", 15), alias("15세이상관람가", 15), listed("18", 18), alias("청소년관람불가", 18), listed("19", 19),
		listed("Restricted Screening", 1000), alias("제한상영가", 1000)},
	// Mexico: the Dirección General de Radio, Televisión y Cinematografía
	// (RTC): AA and A for all, B (12), B-15, C (18), D for adult works.
	"MX": {listed("AA", 0), listed("A", 0), listed("B", 12), listed("B-15", 15), listed("C", 18), listed("D", 1000)},
	// Netherlands: the Kijkwijzer of the NICAM: AL (alle leeftijden), then
	// by age.
	"NL": append([]countryRating{listed("AL", 0)}, ages(6, 9, 12, 14, 16, 18)...),
	// Norway: Medietilsynet: A (alle), then by age.
	"NO": append([]countryRating{listed("A", 0)}, ages(6, 9, 12, 15, 18)...),
	// New Zealand: the Classification Office (Te Mana Whakaatu): G, PG, M
	// (16 and over), R13 to R18, RP13 to RP18 (with a parent), R for the
	// restricted ones.
	"NZ": {listed("G", 0), listed("PG", 8), listed("R13", 13), listed("RP13", 13), listed("R15", 15),
		listed("M", 16), listed("R16", 16), listed("RP16", 16), listed("R18", 18), listed("RP18", 18), listed("R", 18)},
	// Poland: the signs of the Krajowa Rada Radiofonii i Telewizji
	// (KRRiT): without restriction (bez ograniczeń), then by age.
	"PL": append([]countryRating{listed("BO", 0), alias("Bez ograniczeń", 0), alias("AL", 0)}, ages(7, 12, 16, 18)...),
	// Portugal: the Comissão de Classificação de Espetáculos (IGAC): M/3
	// to M/18, P for adult works.
	"PT": {listed("M/3", 3), listed("M/6", 6), listed("M/12", 12), listed("M/14", 14), listed("M/16", 16),
		listed("M/18", 18), listed("P", 1000)},
	// Sweden: Statens medieråd: Btl (barntillåten), then from 7, 11 and 15
	// years.
	"SE": {listed("Btl", 0), alias("Barntillåten", 0), listed("7", 7), alias("Från 7 år", 7), listed("11", 11),
		alias("Från 11 år", 11), listed("15", 15), alias("Från 15 år", 15)},
}

// ratingSystem is a country's ratings: scores by key, and the list apps
// offer as limits.
type ratingSystem struct {
	scores map[string]Score
	list   []Rating
}

// systems are the countries' rating systems, by ISO 3166-1 alpha-2 code.
var systems = func() map[string]*ratingSystem {
	result := map[string]*ratingSystem{}
	us := &ratingSystem{scores: map[string]Score{}, list: []Rating{{Name: "Unrated"}}}
	for _, group := range usRatings {
		for _, name := range group.names {
			score := Score{Score: group.score, SubScore: new(group.subScore)}
			us.scores[key(name)] = score
			us.list = append(us.list, Rating{Name: name, Score: &score})
		}
	}
	for _, entry := range beyondUS {
		us.list = append(us.list, Rating{Name: entry.name, Score: &Score{Score: entry.score}})
	}
	result[DefaultRatingCountry] = us
	for code, ratings := range countrySystems {
		system := &ratingSystem{scores: map[string]Score{}, list: []Rating{{Name: "Unrated"}}}
		var levels []int
		labels := map[int][]string{}
		for _, rating := range ratings {
			system.scores[key(rating.name)] = Score{Score: rating.score}
			if rating.alias {
				continue
			}
			if _, ok := labels[rating.score]; !ok {
				levels = append(levels, rating.score)
			}
			labels[rating.score] = append(labels[rating.score], rating.name)
		}
		// Adult only and banned titles stay a choice everywhere, but not
		// 21, an age no other system rates at.
		for _, entry := range beyondUS {
			if _, ok := labels[entry.score]; !ok && entry.score >= 1000 {
				levels = append(levels, entry.score)
				labels[entry.score] = []string{entry.name}
			}
		}
		slices.Sort(levels)
		for _, level := range levels {
			name := strings.Join(labels[level], " / ")
			if len(labels[level]) > 1 {
				system.scores[key(name)] = Score{Score: level}
			}
			system.list = append(system.list, Rating{Name: name, Score: &Score{Score: level}})
		}
		result[code] = system
	}
	return result
}()

// systemOf is the rating system of country, the US one for a country
// Polyfin has none of.
func systemOf(country string) *ratingSystem {
	if system, ok := systems[country]; ok {
		return system
	}
	return systems[DefaultRatingCountry]
}

var ratingCountries = func() []string {
	codes := make([]string, 0, len(systems))
	for code := range systems {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}()
