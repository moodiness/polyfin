package plainsearch

// words are a language's words (see phrases for how a list is written):
// those that only link the others; those of movies, of series, of short
// titles and of highly rated ones; those before a runtime, and the units
// of runtimes, with the words of small numbers; the patterns of decades,
// where {d} stands for its digits ("80" or "1980") and {w} for its words;
// and the names of languages, by ISO 639-1 code, and of genres, by
// concept. In some languages one word names a genre's movies, or series.
type words struct {
	links, movies, series, short, rated string
	under, hours, minutes               string
	numbers                             map[string]float64
	decades                             string
	decadeWords                         map[int]string
	languages                           map[string]string
	genres, genreMovies, genreSeries    map[string]string
}

// concepts are the genres the genre words stand for, as catalogs commonly
// name them: a word means those of them the user's libraries have.
var concepts = map[string][]string{
	"action":      {"Action", "Action & Adventure"},
	"adventure":   {"Adventure", "Action & Adventure"},
	"animation":   {"Animation"},
	"comedy":      {"Comedy"},
	"crime":       {"Crime"},
	"documentary": {"Documentary"},
	"drama":       {"Drama"},
	"family":      {"Family", "Kids"},
	"fantasy":     {"Fantasy", "Sci-Fi & Fantasy"},
	"history":     {"History"},
	"horror":      {"Horror"},
	"music":       {"Music", "Musical"},
	"mystery":     {"Mystery"},
	"romance":     {"Romance"},
	"scifi":       {"Science Fiction", "Sci-Fi", "Sci-Fi & Fantasy"},
	"thriller":    {"Thriller"},
	"war":         {"War", "War & Politics"},
	"western":     {"Western"},
}

// languages are the tables, in the order a search tries them.
var languages = []words{english, french, german, spanish, italian, portuguese}

var english = words{
	links:   "the|a|an|of|from|in|with|and|that|are|on|for|some|released|made",
	movies:  "movie(/s)",
	series:  "series|show(/s)|tv show(/s)|tv series",
	short:   "short",
	rated:   "highly rated|high rated|top rated|well rated|best rated",
	under:   "under|below|less than|shorter than",
	hours:   "hour(/s)|h|hr(/s)",
	minutes: "minute(/s)|min(/s)",
	numbers: map[string]float64{"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4},
	decades: "{d} s|{w}",
	decadeWords: map[int]string{1920: "twenties", 1930: "thirties", 1940: "forties", 1950: "fifties", 1960: "sixties",
		1970: "seventies", 1980: "eighties", 1990: "nineties"},
	languages: map[string]string{
		"en": "english", "fr": "french", "es": "spanish", "de": "german", "it": "italian", "pt": "portuguese",
		"ja": "japanese", "ko": "korean", "zh": "chinese|mandarin", "hi": "hindi", "ru": "russian", "sv": "swedish",
		"da": "danish", "nl": "dutch", "no": "norwegian", "pl": "polish", "tr": "turkish",
	},
	genres: map[string]string{
		"action": "action", "adventure": "adventure(/s)", "animation": "animation(/s)|animated|cartoon(/s)",
		"comedy": "comedy|comedies", "crime": "crime", "documentary": "documentary|documentaries", "drama": "drama(/s)",
		"family": "family", "fantasy": "fantasy", "history": "history|historical", "horror": "horror",
		"music": "music|musical(/s)", "mystery": "mystery|mysteries", "romance": "romance(/s)|romantic",
		"scifi": "science fiction|sci-fi|scifi", "thriller": "thriller(/s)", "war": "war", "western": "western(/s)",
	},
}

var french = words{
	links:   "le|la|les|l|de|des|du|d|en|et|avec|qui|sorti(/s/e/es)",
	movies:  "film(/s)",
	series:  "série(/s)|série(/s) télé|série(/s) tv|feuilleton(/s)",
	short:   "court(/s/e/es)",
	rated:   "bien noté(/s/e/es)|très bien noté(/s/e/es)|mieux noté(/s/e/es)",
	under:   "moins de|moins d",
	hours:   "heure(/s)|h",
	minutes: "minute(/s)|min(/s)|mn",
	numbers: map[string]float64{"un": 1, "une": 1, "deux": 2, "trois": 3, "quatre": 4},
	decades: "années {d}|années {w}",
	decadeWords: map[int]string{1920: "vingt", 1930: "trente", 1940: "quarante", 1950: "cinquante", 1960: "soixante",
		1970: "soixante-dix", 1980: "quatre-vingt(/s)", 1990: "quatre-vingt-dix"},
	languages: map[string]string{
		"en": "anglais(/e/es)", "fr": "français(/e/es)", "es": "espagnol(/e/s/es)", "de": "allemand(/e/s/es)",
		"it": "italien(/ne/s/nes)", "pt": "portugais(/e/es)", "ja": "japonais(/e/es)", "ko": "coréen(/ne/s/nes)",
		"zh": "chinois(/e/es)|mandarin", "hi": "hindi", "ru": "russe(/s)", "sv": "suédois(/e/es)", "da": "danois(/e/es)",
		"nl": "néerlandais(/e/es)|hollandais(/e/es)", "no": "norvégien(/ne/s/nes)", "pl": "polonais(/e/es)", "tr": "turc(/s)|turque(/s)",
	},
	genres: map[string]string{
		"action": "action", "adventure": "aventure(/s)", "animation": "animation|animé(/s/e/es)|dessin(/s) animé(/s)",
		"comedy": "comédie(/s)", "crime": "policier(/s/e/es)|policière(/s)|criminel(/s/le/les)|crime(/s)",
		"documentary": "documentaire(/s)", "drama": "drame(/s)|dramatique(/s)", "family": "famille|familial(/e/es)|familiaux",
		"fantasy": "fantastique(/s)|fantasy", "history": "histoire|historique(/s)", "horror": "horreur|épouvante",
		"music": "musique|musical(/e/es)|musicaux", "mystery": "mystère(/s)", "romance": "romance(/s)|romantique(/s)",
		"scifi": "science-fiction|sf", "thriller": "thriller(/s)|suspense", "war": "guerre", "western": "western(/s)",
	},
}

var german = words{
	links:   "der|die|das|den|dem|des|ein|eine|einen|einer|eines|aus|von|vom|mit|und|in|im|auf",
	movies:  "film(/e/en)|spielfilm(/e/en)|kinofilm(/e/en)",
	series:  "serie(/n)|fernsehserie(/n)|tv-serie(/n)",
	short:   "kurz(/e/en/er/es)",
	rated:   "gut bewertet(/e/en/er/es)|hoch bewertet(/e/en/er/es)|bestbewertet(/e/en/er/es)|best bewertet(/e/en/er/es)",
	under:   "unter|weniger als|kürzer als",
	hours:   "stunde(/n)|std|h",
	minutes: "minute(/n)|min",
	numbers: map[string]float64{"ein": 1, "eine": 1, "einer": 1, "zwei": 2, "drei": 3, "vier": 4},
	decades: "{d} er|{d} ern|{d} er jahre(/n)|{w}|{w} jahre(/n)",
	decadeWords: map[int]string{1920: "zwanziger(/n)", 1930: "dreißiger(/n)|dreissiger(/n)", 1940: "vierziger(/n)",
		1950: "fünfziger(/n)", 1960: "sechziger(/n)", 1970: "siebziger(/n)", 1980: "achtziger(/n)", 1990: "neunziger(/n)"},
	languages: map[string]string{
		"en": "englisch(/e/en/er/es)", "fr": "französisch(/e/en/er/es)", "es": "spanisch(/e/en/er/es)",
		"de": "deutsch(/e/en/er/es)", "it": "italienisch(/e/en/er/es)", "pt": "portugiesisch(/e/en/er/es)",
		"ja": "japanisch(/e/en/er/es)", "ko": "koreanisch(/e/en/er/es)", "zh": "chinesisch(/e/en/er/es)|mandarin",
		"hi": "hindi", "ru": "russisch(/e/en/er/es)", "sv": "schwedisch(/e/en/er/es)", "da": "dänisch(/e/en/er/es)",
		"nl": "niederländisch(/e/en/er/es)|holländisch(/e/en/er/es)", "no": "norwegisch(/e/en/er/es)",
		"pl": "polnisch(/e/en/er/es)", "tr": "türkisch(/e/en/er/es)",
	},
	genres: map[string]string{
		"action": "action", "adventure": "abenteuer", "animation": "animation|zeichentrick|animiert(/e/en)",
		"comedy": "komödie(/n)", "crime": "krimi(/s)|kriminal", "documentary": "dokumentation(/en)|doku(/s)",
		"drama": "drama|dramen", "family": "familie", "fantasy": "fantasy", "history": "geschichte|historie|historisch(/e/en/er/es)",
		"horror": "horror", "music": "musik|musical(/s)", "mystery": "mystery", "romance": "romantik|romanze(/n)|romantisch(/e/en/er/es)",
		"scifi": "science-fiction|sci-fi|scifi", "thriller": "thriller", "war": "krieg", "western": "western",
	},
	genreMovies: map[string]string{
		"action": "actionfilm(/e/en)", "adventure": "abenteuerfilm(/e/en)", "animation": "animationsfilm(/e/en)|zeichentrickfilm(/e/en)",
		"crime": "kriminalfilm(/e/en)|krimifilm(/e/en)", "documentary": "dokumentarfilm(/e/en)", "family": "familienfilm(/e/en)",
		"fantasy": "fantasyfilm(/e/en)", "history": "historienfilm(/e/en)", "horror": "horrorfilm(/e/en)", "music": "musikfilm(/e/en)",
		"romance": "liebesfilm(/e/en)", "scifi": "science-fiction-film(/e/en)|sci-fi-film(/e/en)", "war": "kriegsfilm(/e/en)",
	},
	genreSeries: map[string]string{
		"animation": "zeichentrickserie(/n)|animationsserie(/n)", "comedy": "comedyserie(/n)", "crime": "krimiserie(/n)",
		"drama": "dramaserie(/n)", "horror": "horrorserie(/n)", "scifi": "science-fiction-serie(/n)|sci-fi-serie(/n)",
	},
}

var spanish = words{
	links:   "el|la|los|las|de|del|en|y|con|que|un|una|unos|unas|estrenad(o/os/a/as)",
	movies:  "película(/s)|peli(/s)",
	series:  "serie(/s)|serie(/s) de televisión|serie(/s) de tv|serie(/s) tv",
	short:   "cort(o/os/a/as)",
	rated:   "bien valorad(o/os/a/as)|mejor valorad(o/os/a/as)|bien calificad(o/os/a/as)|mejor calificad(o/os/a/as)",
	under:   "menos de",
	hours:   "hora(/s)|h",
	minutes: "minuto(/s)|min(/s)",
	numbers: map[string]float64{"un": 1, "una": 1, "dos": 2, "tres": 3, "cuatro": 4},
	decades: "años {d}|años {w}|década de {d}|década de los {d}|década de los {w}|los {d}|los {w}",
	decadeWords: map[int]string{1920: "veinte", 1930: "treinta", 1940: "cuarenta", 1950: "cincuenta", 1960: "sesenta",
		1970: "setenta", 1980: "ochenta", 1990: "noventa"},
	languages: map[string]string{
		"en": "ingl(és/esa/esas/eses)", "fr": "franc(és/esa/esas/eses)", "es": "español(/a/as/es)", "de": "alem(án/ana/anas/anes)",
		"it": "italian(o/os/a/as)", "pt": "portugu(és/esa/esas/eses)", "ja": "japon(és/esa/esas/eses)", "ko": "corean(o/os/a/as)",
		"zh": "chin(o/os/a/as)|mandarín", "hi": "hindi", "ru": "rus(o/os/a/as)", "sv": "suec(o/os/a/as)", "da": "dan(és/esa/esas/eses)",
		"nl": "neerland(és/esa/esas/eses)|holand(és/esa/esas/eses)", "no": "norueg(o/os/a/as)", "pl": "polac(o/os/a/as)",
		"tr": "turc(o/os/a/as)",
	},
	genres: map[string]string{
		"action": "acción", "adventure": "aventura(/s)", "animation": "animación|animad(o/os/a/as)|dibujos animados",
		"comedy": "comedia(/s)", "crime": "crimen|policiac(o/os/a/as)|policíac(o/os/a/as)", "documentary": "documental(/es)",
		"drama": "drama(/s)|dramátic(o/os/a/as)", "family": "familia|familiar(/es)", "fantasy": "fantasía|fantástic(o/os/a/as)",
		"history": "historia|históric(o/os/a/as)", "horror": "terror|horror|miedo", "music": "música|musical(/es)",
		"mystery": "misterio(/s)", "romance": "romance(/s)|romántic(o/os/a/as)", "scifi": "ciencia ficción|sci-fi",
		"thriller": "thriller(/s)|suspense|suspenso", "war": "guerra|bélic(o/os/a/as)", "western": "western(/s)|oeste",
	},
}

var italian = words{
	links: "il|lo|la|i|gli|le|l|un|una|uno|di|del|dello|della|dei|degli|delle|dell|da|dal|dalla|dai|dagli|in|nel|nella|" +
		"nei|negli|nelle|e|con|che|uscit(o/i/a/e)",
	movies:  "film",
	series:  "serie|serie tv|serie televisiv(a/e)",
	short:   "brev(e/i)|cort(o/i/a/e)",
	rated:   "ben recensit(o/i/a/e)|ben valutat(o/i/a/e)|più votat(o/i/a/e)|molto apprezzat(o/i/a/e)",
	under:   "meno di|meno d|sotto le|sotto i|sotto",
	hours:   "ora|ore|h",
	minutes: "minut(o/i)|min",
	numbers: map[string]float64{"un": 1, "una": 1, "uno": 1, "due": 2, "tre": 3, "quattro": 4},
	decades: "anni {d}|anni {w}",
	decadeWords: map[int]string{1920: "venti", 1930: "trenta", 1940: "quaranta", 1950: "cinquanta", 1960: "sessanta",
		1970: "settanta", 1980: "ottanta", 1990: "novanta"},
	languages: map[string]string{
		"en": "ingles(e/i)", "fr": "frances(e/i)", "es": "spagnol(o/i/a/e)", "de": "tedesc(o/hi/a/he)", "it": "italian(o/i/a/e)",
		"pt": "portoghes(e/i)", "ja": "giappones(e/i)", "ko": "corean(o/i/a/e)", "zh": "cines(e/i)|mandarino", "hi": "hindi",
		"ru": "russ(o/i/a/e)", "sv": "svedes(e/i)", "da": "danes(e/i)", "nl": "olandes(e/i)", "no": "norveges(e/i)",
		"pl": "polacc(o/hi/a/he)", "tr": "turc(o/hi/a/he)",
	},
	genres: map[string]string{
		"action": "azione", "adventure": "avventura|avventure", "animation": "animazione|animat(o/i/a/e)|cartoni animati|cartone animato",
		"comedy": "commedia|commedie|comic(o/i/a/he)", "crime": "crimine|poliziesc(o/hi/a/he)", "documentary": "documentario|documentari",
		"drama": "dramma|drammi|drammatic(o/i/a/he)", "family": "famiglia", "fantasy": "fantasy|fantastic(o/i/a/he)",
		"history": "storia|storic(o/i/a/he)", "horror": "horror|orrore", "music": "musica|musical|musical(e/i)",
		"mystery": "mistero|misteri", "romance": "romantic(o/i/a/he)|sentimental(e/i)", "scifi": "fantascienza|sci-fi",
		"thriller": "thriller", "war": "guerra", "western": "western",
	},
}

var portuguese = words{
	links:   "o|a|os|as|de|do|da|dos|das|em|no|na|nos|nas|e|com|que|um|uma|lançad(o/os/a/as)",
	movies:  "filme(/s)",
	series:  "série(/s)|série(/s) de tv|série(/s) de televisão|seriado(/s)",
	short:   "curt(o/os/a/as)",
	rated:   "bem avaliad(o/os/a/as)|mais bem avaliad(o/os/a/as)|bem votad(o/os/a/as)|bem cotad(o/os/a/as)",
	under:   "menos de",
	hours:   "hora(/s)|h",
	minutes: "minuto(/s)|min(/s)",
	numbers: map[string]float64{"um": 1, "uma": 1, "dois": 2, "duas": 2, "três": 3, "quatro": 4},
	decades: "anos {d}|anos {w}|década de {d}|década de {w}",
	decadeWords: map[int]string{1920: "vinte", 1930: "trinta", 1940: "quarenta", 1950: "cinquenta", 1960: "sessenta",
		1970: "setenta", 1980: "oitenta", 1990: "noventa"},
	languages: map[string]string{
		"en": "ingl(ês/esa/esas/eses)", "fr": "franc(ês/esa/esas/eses)", "es": "espanhol(/a/as)|espanhóis",
		"de": "alem(ão/ã/ãs/ães)", "it": "italian(o/os/a/as)", "pt": "portugu(ês/esa/esas/eses)", "ja": "japon(ês/esa/esas/eses)",
		"ko": "corean(o/os/a/as)", "zh": "chin(ês/esa/esas/eses)|mandarim", "hi": "hindi", "ru": "russ(o/os/a/as)",
		"sv": "suec(o/os/a/as)", "da": "dinamarqu(ês/esa/esas/eses)", "nl": "holand(ês/esa/esas/eses)|neerland(ês/esa/esas/eses)",
		"no": "noruegu(ês/esa/esas/eses)", "pl": "polon(ês/esa/esas/eses)|polac(o/os/a/as)", "tr": "turc(o/os/a/as)",
	},
	genres: map[string]string{
		"action": "ação", "adventure": "aventura(/s)", "animation": "animação|animad(o/os/a/as)|desenho(/s) animado(/s)",
		"comedy": "comédia(/s)", "crime": "crime(/s)|policial|policiais", "documentary": "documentário(/s)",
		"drama": "drama(/s)|dramátic(o/os/a/as)", "family": "família|familiar(/es)", "fantasy": "fantasia|fantástic(o/os/a/as)",
		"history": "história|históric(o/os/a/as)", "horror": "terror|horror", "music": "música|musical|musicais",
		"mystery": "mistério(/s)", "romance": "romance(/s)|romântic(o/os/a/as)", "scifi": "ficção científica|sci-fi",
		"thriller": "thriller(/s)|suspense", "war": "guerra", "western": "faroeste(/s)|western(/s)",
	},
}
