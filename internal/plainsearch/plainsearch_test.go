package plainsearch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// libraryGenres are the genres of the libraries the searches are read
// with, as catalogs commonly name them: a movie catalog's and a series
// catalog's.
var libraryGenres = []string{"Action", "Animation", "Comedy", "Crime", "Documentary", "Drama", "Horror", "Romance",
	"Science Fiction", "Thriller", "Sci-Fi & Fantasy", "Action & Adventure"}

// describe tells rules in short: "type=movie genre=Horror year=1980-1989
// runtime<=119 rating>=7.5 language=fr", a genre of a group of any of
// them as "genre=A|B".
func describe(rules smartlists.Rule) string {
	var parts []string
	for _, r := range rules.Rules {
		switch r.Field {
		case smartlists.FieldType, smartlists.FieldGenre, smartlists.FieldLanguage:
			parts = append(parts, r.Field+"="+r.Value)
		case smartlists.FieldGroup:
			var names []string
			for _, g := range r.Rules {
				names = append(names, g.Value)
			}
			parts = append(parts, "genre="+strings.Join(names, "|"))
		case smartlists.FieldYear:
			parts = append(parts, fmt.Sprintf("year=%v-%v", *r.Min, *r.Max))
		case smartlists.FieldRuntime:
			parts = append(parts, fmt.Sprintf("runtime<=%v", *r.Max))
		case smartlists.FieldRating:
			parts = append(parts, fmt.Sprintf("rating>=%v", *r.Min))
		default:
			parts = append(parts, "?"+r.Field)
		}
	}
	return strings.Join(parts, " ")
}

// Searches in plain words, in each language, stand for the rules of what
// they name; "" for a search that is not understood, which stays a search
// of titles.
func TestParseUnderstandsPlainWords(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ search, want string }{
		// English.
		{"80s sci-fi under 2 hours", "genre=Science Fiction|Sci-Fi & Fantasy year=1980-1989 runtime<=119"},
		{"Highly rated French movies", "type=movie rating>=7.5 language=fr"},
		{"short comedies", "genre=Comedy runtime<=89"},
		{"horror movies from the eighties", "type=movie genre=Horror year=1980-1989"},
		{"1999 thrillers", "genre=Thriller year=1999-1999"},
		{"korean series under 45 min", "type=series language=ko runtime<=44"},
		{"comedy drama movies of the 1990s", "type=movie genre=Comedy genre=Drama year=1990-1999"},
		{"action movies of the '80s", "type=movie genre=Action|Action & Adventure year=1980-1989"},
		{"movies under an hour", "type=movie runtime<=59"},
		{"Kids", ""},
		{"1917", ""},
		{"Horror", ""},
		{"The Thing", ""},
		{"2001 a space odyssey", ""},
		{"80s westerns", ""}, // The libraries have no western.
		{"80s 90s movies", ""},
		{"short movies under 2 hours", ""},
		// French.
		{"films des années 80 de moins de 2 heures", "type=movie year=1980-1989 runtime<=119"},
		{"comédies romantiques italiennes", "genre=Comedy genre=Romance language=it"},
		{"séries policières des années 90", "type=series genre=Crime year=1990-1999"},
		{"films d’horreur des années quatre-vingt", "type=movie genre=Horror year=1980-1989"},
		{"documentaires courts bien notés", "genre=Documentary runtime<=89 rating>=7.5"},
		{"films de moins d’une heure", "type=movie runtime<=59"},
		{"Les Misérables", ""},
		{"La Haine", ""},
		// German.
		{"Horrorfilme der 80er", "type=movie genre=Horror year=1980-1989"},
		{"kurze deutsche Komödien", "genre=Comedy runtime<=89 language=de"},
		{"gut bewertete Serien aus den 90ern", "type=series rating>=7.5 year=1990-1999"},
		{"Filme der achtziger Jahre unter 90 Minuten", "type=movie year=1980-1989 runtime<=89"},
		{"Krimiserien kürzer als eine Stunde", "type=series genre=Crime runtime<=59"},
		{"Das Boot", ""},
		// Spanish.
		{"películas de terror de los ochenta", "type=movie genre=Horror year=1980-1989"},
		{"series españolas cortas", "type=series language=es runtime<=89"},
		{"comedias de menos de 90 minutos", "genre=Comedy runtime<=89"},
		{"películas de ciencia ficción de los años 70", "type=movie genre=Science Fiction|Sci-Fi & Fantasy year=1970-1979"},
		{"El laberinto del fauno", ""},
		// Italian.
		{"film horror anni 80", "type=movie genre=Horror year=1980-1989"},
		{"serie tv giapponesi ben recensite", "type=series language=ja rating>=7.5"},
		{"commedie sotto le 2 ore", "genre=Comedy runtime<=119"},
		{"film di fantascienza degli anni '50", "type=movie genre=Science Fiction|Sci-Fi & Fantasy year=1950-1959"},
		{"La vita è bella", ""},
		// Portuguese.
		{"filmes de terror dos anos 80", "type=movie genre=Horror year=1980-1989"},
		{"documentários curtos em inglês", "genre=Documentary runtime<=89 language=en"},
		{"séries bem avaliadas de 2010", "type=series rating>=7.5 year=2010-2010"},
		{"comédias com menos de uma hora e meia", ""},
		{"Cidade de Deus", ""},
	} {
		q, ok := Parse(test.search, libraryGenres)
		got := ""
		if ok {
			got = describe(q.Rules)
			if want := ""; strings.Contains(got, "type=") {
				want = strings.TrimPrefix(strings.Fields(got)[0], "type=")
				if q.Type != want {
					t.Errorf("%q: type %q, rules %s", test.search, q.Type, got)
				}
			}
		}
		if got != test.want {
			t.Errorf("%q: %q, want %q", test.search, got, test.want)
		}
	}
}

// A genre the libraries name is understood as they spell it, whatever the
// language of the other words, and in the plural.
func TestParseNamesTheLibrariesGenres(t *testing.T) {
	t.Parallel()
	genres := []string{"Policier", "Anime", "Sci-Fi"}
	for _, test := range []struct{ search, want string }{
		{"anime series", "type=series genre=Anime"},
		{"policiers des années 70", "genre=Policier year=1970-1979"},
		{"sci fi movies", "type=movie genre=Sci-Fi"},
		{"80s comedies", ""},
	} {
		q, ok := Parse(test.search, genres)
		got := ""
		if ok {
			got = describe(q.Rules)
		}
		if got != test.want {
			t.Errorf("%q: %q, want %q", test.search, got, test.want)
		}
	}
}

// The tables name the languages of smart lists' rules only, and the
// genre concepts their words stand for.
func TestWordsNameKnownLanguagesAndConcepts(t *testing.T) {
	t.Parallel()
	for _, w := range languages {
		for code := range w.languages {
			if !localization.KnownLanguage(code) {
				t.Errorf("unknown language %q", code)
			}
		}
		for _, table := range []map[string]string{w.genres, w.genreMovies, w.genreSeries} {
			for concept := range table {
				if _, ok := concepts[concept]; !ok {
					t.Errorf("unknown genre concept %q", concept)
				}
			}
		}
	}
}
