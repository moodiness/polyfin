package playback

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// language is an entry of Jellyfin 12.1's language list, in the order its
// /Localization/Cultures endpoint returned it on the oracle. Jellyfin finds
// a stream's LocalizedLanguage in it.
type language struct {
	// code is the ISO 639-2/T code ("fra").
	code string
	// bibliographic is the ISO 639-2/B code when it differs ("fre").
	bibliographic string
	// short is the ISO 639-1 code, sometimes with a region ("pt-br").
	short string
	// name is the English name as listed ("Spanish; Castilian").
	name string
	// french is the French name from the Unicode CLDR. Jellyfin ignores
	// it; Polyfin accepts it from addons.
	french string
}

// specialLanguages are the ISO 639-2 codes that name no language. Jellyfin
// leaves them out of audio display titles (observed for und and zxx).
var specialLanguages = [...]string{"und", "zxx", "mis", "mul"}

// languageIndex maps lowercase keys to positions in languages.
type languageIndex struct {
	// jellyfin holds what Jellyfin looks a language up by: both ISO 639-2
	// codes, the ISO 639-1 code and the full English name.
	jellyfin map[string]int
	// bibliographic maps ISO 639-2/B codes to ISO 639-2/T codes.
	bibliographic map[string]string
	// addon also holds each part of the English names ("Spanish",
	// "Castilian") and the French names.
	addon map[string]int
}

var languageLookup = sync.OnceValue(func() languageIndex {
	index := languageIndex{
		jellyfin:      make(map[string]int, 3*len(languages)),
		bibliographic: make(map[string]string, 32),
		addon:         make(map[string]int, 5*len(languages)),
	}
	add := func(m map[string]int, key string, i int) {
		key = strings.ToLower(strings.TrimSpace(key))
		if _, taken := m[key]; key != "" && !taken {
			m[key] = i
		}
	}
	for i, l := range languages {
		for _, key := range [...]string{l.code, l.bibliographic, l.short, l.name} {
			add(index.jellyfin, key, i)
			add(index.addon, key, i)
		}
		if l.bibliographic != "" {
			index.bibliographic[l.bibliographic] = l.code
		}
	}
	// Names come after codes, so that no name hides a code.
	for i, l := range languages {
		for part := range strings.FieldsFuncSeq(l.name, func(r rune) bool { return r == ';' || r == ',' }) {
			add(index.addon, part, i)
		}
		add(index.addon, l.french, i)
	}
	return index
})

// streamLanguage is the language Jellyfin reports for a track tagged
// language: an ISO 639-2/B code becomes the /T code ("fre" gives "fra",
// "ger" gives "deu"), and anything else is kept as tagged ("fr", "French",
// "pt-br", "und"), as observed.
func streamLanguage(language string) string {
	if len(language) == 3 {
		if code, ok := languageLookup().bibliographic[strings.ToLower(language)]; ok {
			return code
		}
	}
	return language
}

// addonLanguage is the ISO 639-2/T code of a language as an addon gives
// it: an ISO 639-2/B or /T code, an ISO 639-1 code with or without a
// region, or an English or French name, in any letter case ("fre", "fr",
// "fr-FR", "French", "Français" all give "fra"). Jellyfin reads sidecar
// file names the same way ("x.fr.srt", "x.French.srt" and "x.fre.srt" all
// give "fra", observed); French names are Polyfin's addition. Anything else
// is kept as given.
func addonLanguage(language string) string {
	language = strings.TrimSpace(language)
	index := languageLookup().addon
	key := strings.ToLower(language)
	i, ok := index[key]
	if !ok {
		if base, _, cut := strings.Cut(strings.ReplaceAll(key, "_", "-"), "-"); cut {
			i, ok = index[base]
		}
	}
	if !ok {
		return language
	}
	return languages[i].code
}

// languageName is the LocalizedLanguage Jellyfin reports for a track's
// language: the English name of the first entry of its list matching the
// language by code or full name in any letter case, up to the first
// semicolon or comma ("Spanish; Castilian" gives "Spanish", "Greek, Modern
// (1453-)" gives "Greek"), or "" when none matches ("Spanish" alone, or
// "Français"). Jellyfin keeps these names in English whatever its UI
// culture (observed with UICulture=fr).
func languageName(language string) string {
	if language == "" {
		return ""
	}
	i, ok := languageLookup().jellyfin[strings.ToLower(language)]
	if !ok {
		return ""
	}
	name := languages[i].name
	if end := strings.IndexAny(name, ";,"); end >= 0 {
		name = name[:end]
	}
	return name
}

// languageLabel is how a display title shows a language: its English name,
// else the language as tagged with its first letter capitalized ("Pob",
// "Fr-FR"), as observed.
func languageLabel(language, name string) string {
	if name != "" {
		return name
	}
	return capitalized(language)
}

// capitalized returns s with its first letter in upper case.
func capitalized(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// isSpecialLanguage reports whether language names no language.
func isSpecialLanguage(language string) bool {
	for _, code := range specialLanguages {
		if strings.EqualFold(language, code) {
			return true
		}
	}
	return false
}

// languages is Jellyfin 12.1's language list, observed through
// /Localization/Cultures, with French names from the Unicode CLDR.
var languages = [...]language{
	{"abk", "", "ab", "Abkhazian", "abkhaze"},
	{"ace", "", "", "Achinese", "aceh"},
	{"ach", "", "", "Acoli", "acoli"},
	{"ada", "", "", "Adangme", "adangme"},
	{"ady", "", "", "Adyghe; Adygei", "adyghéen"},
	{"aar", "", "aa", "Afar", "afar"},
	{"afh", "", "", "Afrihili", "afrihili"},
	{"afr", "", "af", "Afrikaans", "afrikaans"},
	{"afa", "", "", "Afro-Asiatic languages", ""},
	{"ain", "", "", "Ainu", "aïnou"},
	{"aka", "", "ak", "Akan", "akan"},
	{"akk", "", "", "Akkadian", "akkadien"},
	{"sqi", "alb", "sq", "Albanian", "albanais"},
	{"ale", "", "", "Aleut", "aléoute"},
	{"alg", "", "", "Algonquian languages", ""},
	{"tut", "", "", "Altaic languages", ""},
	{"amh", "", "am", "Amharic", "amharique"},
	{"anp", "", "", "Angika", "angika"},
	{"apa", "", "", "Apache languages", ""},
	{"ara", "", "ar", "Arabic", "arabe"},
	{"arg", "", "an", "Aragonese", "aragonais"},
	{"arp", "", "", "Arapaho", "arapaho"},
	{"arw", "", "", "Arawak", "arawak"},
	{"hye", "arm", "hy", "Armenian", "arménien"},
	{"rup", "", "", "Aromanian; Arumanian; Macedo-Romanian", "valaque"},
	{"art", "", "", "Artificial languages", ""},
	{"asm", "", "as", "Assamese", "assamais"},
	{"ast", "", "", "Asturian; Bable; Leonese; Asturleonese", "asturien"},
	{"ath", "", "", "Athapascan languages", ""},
	{"aus", "", "", "Australian languages", ""},
	{"map", "", "", "Austronesian languages", ""},
	{"ava", "", "av", "Avaric", "avar"},
	{"ave", "", "ae", "Avestan", "avestique"},
	{"awa", "", "", "Awadhi", "awadhi"},
	{"aym", "", "ay", "Aymara", "aymara"},
	{"aze", "", "az", "Azerbaijani", "azéri"},
	{"ban", "", "", "Balinese", "balinais"},
	{"bat", "", "", "Baltic languages", ""},
	{"bal", "", "", "Baluchi", "baloutchi"},
	{"bam", "", "bm", "Bambara", "bambara"},
	{"bai", "", "", "Bamileke languages", ""},
	{"bad", "", "", "Banda languages", ""},
	{"bnt", "", "", "Bantu (Other)", ""},
	{"bas", "", "", "Basa", "bassa"},
	{"bak", "", "ba", "Bashkir", "bachkir"},
	{"eus", "baq", "eu", "Basque", "basque"},
	{"btk", "", "", "Batak languages", ""},
	{"bej", "", "", "Beja; Bedawiyet", "bedja"},
	{"bel", "", "be", "Belarusian", "biélorusse"},
	{"bem", "", "", "Bemba", "bemba"},
	{"ben", "", "bn", "Bengali", "bengali"},
	{"ber", "", "", "Berber languages", ""},
	{"bho", "", "", "Bhojpuri", "bhojpuri"},
	{"bih", "", "bh", "Bihari languages", "bhojpuri"},
	{"bik", "", "", "Bikol", "bikol"},
	{"bin", "", "", "Bini; Edo", "bini"},
	{"bis", "", "bi", "Bislama", "bichelamar"},
	{"byn", "", "", "Blin; Bilin", "blin"},
	{"zbl", "", "", "Blissymbols; Blissymbolics; Bliss", "symboles Bliss"},
	{"bos", "", "bs", "Bosnian", "bosniaque"},
	{"bra", "", "", "Braj", "braj"},
	{"bre", "", "br", "Breton", "breton"},
	{"bug", "", "", "Buginese", "bugi"},
	{"bul", "", "bg", "Bulgarian", "bulgare"},
	{"bua", "", "", "Buriat", "bouriate"},
	{"mya", "bur", "my", "Burmese", "birman"},
	{"cad", "", "", "Caddo", "caddo"},
	{"cat", "", "ca", "Catalan; Valencian", "catalan"},
	{"cau", "", "", "Caucasian languages", ""},
	{"ceb", "", "", "Cebuano", "cebuano"},
	{"cel", "", "", "Celtic languages", ""},
	{"cai", "", "", "Central American Indian languages", ""},
	{"khm", "", "km", "Central Khmer", "khmer"},
	{"chg", "", "", "Chagatai", "tchaghataï"},
	{"cmc", "", "", "Chamic languages", ""},
	{"cha", "", "ch", "Chamorro", "chamorro"},
	{"che", "", "ce", "Chechen", "tchétchène"},
	{"chr", "", "", "Cherokee", "cherokee"},
	{"chy", "", "", "Cheyenne", "cheyenne"},
	{"chb", "", "", "Chibcha", "chibcha"},
	{"nya", "", "ny", "Chichewa; Chewa; Nyanja", "nyanja"},
	{"zho", "chi", "zh", "Chinese", "chinois"},
	{"zho", "chi", "ze", "Chinese (Bilingual)", ""},
	{"zho", "chi", "zh-hk", "Chinese (Hong Kong)", ""},
	{"zho", "chi", "zh-cn", "Chinese (Simplified)", ""},
	{"zho", "chi", "zh-tw", "Chinese (Traditional)", ""},
	{"chn", "", "", "Chinook jargon", "jargon chinook"},
	{"chp", "", "", "Chipewyan; Dene Suline", "chipewyan"},
	{"cho", "", "", "Choctaw", "choctaw"},
	{"chu", "", "cu", "Church Slavic; Old Slavonic; Church Slavonic; Old Bulgarian; Old Church Slavonic", "slavon d’église"},
	{"chk", "", "", "Chuukese", "chuuk"},
	{"chv", "", "cv", "Chuvash", "tchouvache"},
	{"nwc", "", "", "Classical Newari; Old Newari; Classical Nepal Bhasa", "newarî classique"},
	{"syc", "", "", "Classical Syriac", "syriaque classique"},
	{"cop", "", "", "Coptic", "copte"},
	{"cor", "", "kw", "Cornish", "cornique"},
	{"cos", "", "co", "Corsican", "corse"},
	{"cre", "", "cr", "Cree", "cree"},
	{"mus", "", "", "Creek", "creek"},
	{"crp", "", "", "Creoles and pidgins ", ""},
	{"cpe", "", "", "Creoles and pidgins, English based", ""},
	{"cpf", "", "", "Creoles and pidgins, French-based ", ""},
	{"cpp", "", "", "Creoles and pidgins, Portuguese-based ", ""},
	{"crh", "", "", "Crimean Tatar; Crimean Turkish", "turc de Crimée"},
	{"hrv", "", "hr", "Croatian", "croate"},
	{"cus", "", "", "Cushitic languages", ""},
	{"ces", "cze", "cs", "Czech", "tchèque"},
	{"dak", "", "", "Dakota", "dakota"},
	{"dan", "", "da", "Danish", "danois"},
	{"dar", "", "", "Dargwa", "dargwa"},
	{"del", "", "", "Delaware", "delaware"},
	{"din", "", "", "Dinka", "dinka"},
	{"div", "", "dv", "Divehi; Dhivehi; Maldivian", "maldivien"},
	{"doi", "", "", "Dogri", "dogri"},
	{"dgr", "", "", "Dogrib", "dogrib"},
	{"dra", "", "", "Dravidian languages", ""},
	{"dua", "", "", "Duala", "douala"},
	{"dum", "", "", "Dutch, Middle (ca.1050-1350)", "moyen néerlandais"},
	{"nld", "dut", "nl", "Dutch; Flemish", "néerlandais"},
	{"dyu", "", "", "Dyula", "dioula"},
	{"dzo", "", "dz", "Dzongkha", "dzongkha"},
	{"frs", "", "", "Eastern Frisian", "frison oriental"},
	{"efi", "", "", "Efik", "éfik"},
	{"egy", "", "", "Egyptian (Ancient)", "égyptien ancien"},
	{"eka", "", "", "Ekajuk", "ékadjouk"},
	{"elx", "", "", "Elamite", "élamite"},
	{"eng", "", "en", "English", "anglais"},
	{"enm", "", "", "English, Middle (1100-1500)", "moyen anglais"},
	{"ang", "", "", "English, Old (ca.450-1100)", "ancien anglais"},
	{"myv", "", "", "Erzya", "erzya"},
	{"epo", "", "eo", "Esperanto", "espéranto"},
	{"est", "", "et", "Estonian", "estonien"},
	{"ewe", "", "ee", "Ewe", "éwé"},
	{"ewo", "", "", "Ewondo", "éwondo"},
	{"fan", "", "", "Fang", "fang"},
	{"fat", "", "", "Fanti", "akan"},
	{"fao", "", "fo", "Faroese", "féroïen"},
	{"fij", "", "fj", "Fijian", "fidjien"},
	{"fil", "", "", "Filipino; Pilipino", "filipino"},
	{"fin", "", "fi", "Finnish", "finnois"},
	{"fiu", "", "", "Finno-Ugrian languages", ""},
	{"fon", "", "", "Fon", "fon"},
	{"fra", "fre", "fr", "French", "français"},
	{"frc", "", "fr-ca", "French (Canada)", ""},
	{"frm", "", "", "French, Middle (ca.1400-1600)", "moyen français"},
	{"fro", "", "", "French, Old (842-ca.1400)", "ancien français"},
	{"fur", "", "", "Friulian", "frioulan"},
	{"ful", "", "ff", "Fulah", "peul"},
	{"gaa", "", "", "Ga", "ga"},
	{"gla", "", "gd", "Gaelic; Scottish Gaelic", "gaélique écossais"},
	{"car", "", "", "Galibi Carib", "caribe"},
	{"glg", "", "gl", "Galician", "galicien"},
	{"lug", "", "lg", "Ganda", "ganda"},
	{"gay", "", "", "Gayo", "gayo"},
	{"gba", "", "", "Gbaya", "gbaya"},
	{"gez", "", "", "Geez", "guèze"},
	{"kat", "geo", "ka", "Georgian", "géorgien"},
	{"deu", "ger", "de", "German", "allemand"},
	{"gmh", "", "", "German, Middle High (ca.1050-1500)", "moyen haut-allemand"},
	{"goh", "", "", "German, Old High (ca.750-1050)", "ancien haut allemand"},
	{"gem", "", "", "Germanic languages", ""},
	{"gil", "", "", "Gilbertese", "gilbertin"},
	{"gon", "", "", "Gondi", "gondi"},
	{"gor", "", "", "Gorontalo", "gorontalo"},
	{"got", "", "", "Gothic", "gothique"},
	{"grb", "", "", "Grebo", "grebo"},
	{"grc", "", "", "Greek, Ancient (to 1453)", "grec ancien"},
	{"ell", "gre", "el", "Greek, Modern (1453-)", "grec"},
	{"grn", "", "gn", "Guarani", "guarani"},
	{"guj", "", "gu", "Gujarati", "goudjerati"},
	{"gwi", "", "", "Gwich'in", "gwichʼin"},
	{"hai", "", "", "Haida", "haida"},
	{"hat", "", "ht", "Haitian; Haitian Creole", "créole haïtien"},
	{"hau", "", "ha", "Hausa", "haoussa"},
	{"haw", "", "", "Hawaiian", "hawaïen"},
	{"heb", "", "he", "Hebrew", "hébreu"},
	{"her", "", "hz", "Herero", "héréro"},
	{"hil", "", "", "Hiligaynon", "hiligaynon"},
	{"him", "", "", "Himachali languages; Western Pahari languages", ""},
	{"hin", "", "hi", "Hindi", "hindi"},
	{"hmo", "", "ho", "Hiri Motu", "hiri motu"},
	{"hit", "", "", "Hittite", "hittite"},
	{"hmn", "", "", "Hmong; Mong", "hmong"},
	{"hun", "", "hu", "Hungarian", "hongrois"},
	{"hup", "", "", "Hupa", "hupa"},
	{"iba", "", "", "Iban", "iban"},
	{"isl", "ice", "is", "Icelandic", "islandais"},
	{"ido", "", "io", "Ido", "ido"},
	{"ibo", "", "ig", "Igbo", "igbo"},
	{"ijo", "", "", "Ijo languages", ""},
	{"ilo", "", "", "Iloko", "ilokano"},
	{"inc", "", "", "Indic languages", ""},
	{"ine", "", "", "Indo-European languages", ""},
	{"ind", "", "id", "Indonesian", "indonésien"},
	{"inh", "", "", "Ingush", "ingouche"},
	{"ina", "", "ia", "Interlingua (International Auxiliary Language Association)", "interlingua"},
	{"ile", "", "ie", "Interlingue; Occidental", "interlingue"},
	{"iku", "", "iu", "Inuktitut", "inuktitut"},
	{"ipk", "", "ik", "Inupiaq", "inupiaq"},
	{"ira", "", "", "Iranian languages", ""},
	{"gle", "", "ga", "Irish", "irlandais"},
	{"mga", "", "", "Irish, Middle (900-1200)", "moyen irlandais"},
	{"sga", "", "", "Irish, Old (to 900)", "ancien irlandais"},
	{"iro", "", "", "Iroquoian languages", ""},
	{"ita", "", "it", "Italian", "italien"},
	{"jpn", "", "ja", "Japanese", "japonais"},
	{"jav", "", "jv", "Javanese", "javanais"},
	{"jrb", "", "", "Judeo-Arabic", "judéo-arabe"},
	{"jpr", "", "", "Judeo-Persian", "judéo-persan"},
	{"kbd", "", "", "Kabardian", "kabardin"},
	{"kab", "", "", "Kabyle", "kabyle"},
	{"kac", "", "", "Kachin; Jingpho", "kachin"},
	{"kal", "", "kl", "Kalaallisut; Greenlandic", "groenlandais"},
	{"xal", "", "", "Kalmyk; Oirat", "kalmouk"},
	{"kam", "", "", "Kamba", "kamba"},
	{"kan", "", "kn", "Kannada", "kannada"},
	{"kau", "", "kr", "Kanuri", "kanouri"},
	{"kaa", "", "", "Kara-Kalpak", "karakalpak"},
	{"krc", "", "", "Karachay-Balkar", "karatchaï balkar"},
	{"krl", "", "", "Karelian", "carélien"},
	{"kar", "", "", "Karen languages", ""},
	{"kas", "", "ks", "Kashmiri", "kashmiri"},
	{"csb", "", "", "Kashubian", "kachoube"},
	{"kaw", "", "", "Kawi", "kawi"},
	{"kaz", "", "kk", "Kazakh", "kazakh"},
	{"kha", "", "", "Khasi", "khasi"},
	{"khi", "", "", "Khoisan languages", ""},
	{"kho", "", "", "Khotanese; Sakan", "khotanais"},
	{"kik", "", "ki", "Kikuyu; Gikuyu", "kikuyu"},
	{"kmb", "", "", "Kimbundu", "kimboundou"},
	{"kin", "", "rw", "Kinyarwanda", "rwanda"},
	{"kir", "", "ky", "Kirghiz; Kyrgyz", "kirghize"},
	{"tlh", "", "", "Klingon; tlhIngan-Hol", "klingon"},
	{"kom", "", "kv", "Komi", "komi"},
	{"kon", "", "kg", "Kongo", "kongo"},
	{"kok", "", "", "Konkani", "konkani"},
	{"kor", "", "ko", "Korean", "coréen"},
	{"kos", "", "", "Kosraean", "kosraéen"},
	{"kpe", "", "", "Kpelle", "kpellé"},
	{"kro", "", "", "Kru languages", ""},
	{"kua", "", "kj", "Kuanyama; Kwanyama", "kouanyama"},
	{"kum", "", "", "Kumyk", "koumyk"},
	{"kur", "", "ku", "Kurdish", "kurde"},
	{"kru", "", "", "Kurukh", "kouroukh"},
	{"kut", "", "", "Kutenai", "kutenai"},
	{"lad", "", "", "Ladino", "ladino"},
	{"lah", "", "", "Lahnda", "lahnda"},
	{"lam", "", "", "Lamba", "lamba"},
	{"day", "", "", "Land Dayak languages", ""},
	{"lao", "", "lo", "Lao", "lao"},
	{"lat", "", "la", "Latin", "latin"},
	{"lav", "", "lv", "Latvian", "letton"},
	{"lez", "", "", "Lezghian", "lezghien"},
	{"lim", "", "li", "Limburgan; Limburger; Limburgish", "limbourgeois"},
	{"lin", "", "ln", "Lingala", "lingala"},
	{"lit", "", "lt", "Lithuanian", "lituanien"},
	{"jbo", "", "", "Lojban", "lojban"},
	{"nds", "", "", "Low German; Low Saxon; German, Low; Saxon, Low", "bas-allemand"},
	{"dsb", "", "", "Lower Sorbian", "bas-sorabe"},
	{"loz", "", "", "Lozi", "lozi"},
	{"lub", "", "lu", "Luba-Katanga", "luba-katanga"},
	{"lua", "", "", "Luba-Lulua", "luba-lulua"},
	{"lui", "", "", "Luiseno", "luiseño"},
	{"lun", "", "", "Lunda", "lunda"},
	{"luo", "", "", "Luo (Kenya and Tanzania)", "luo"},
	{"lus", "", "", "Lushai", "lushaï"},
	{"ltz", "", "lb", "Luxembourgish; Letzeburgesch", "luxembourgeois"},
	{"mkd", "mac", "mk", "Macedonian", "macédonien"},
	{"mad", "", "", "Madurese", "madourais"},
	{"mag", "", "", "Magahi", "magahi"},
	{"mai", "", "", "Maithili", "maithili"},
	{"mak", "", "", "Makasar", "makassar"},
	{"mlg", "", "mg", "Malagasy", "malgache"},
	{"msa", "may", "ms", "Malay", "malais"},
	{"mal", "", "ml", "Malayalam", "malayalam"},
	{"mlt", "", "mt", "Maltese", "maltais"},
	{"mnc", "", "", "Manchu", "mandchou"},
	{"mdr", "", "", "Mandar", "mandar"},
	{"man", "", "", "Mandingo", "mandingue"},
	{"mni", "", "", "Manipuri", "manipuri"},
	{"mno", "", "", "Manobo languages", ""},
	{"glv", "", "gv", "Manx", "mannois"},
	{"mri", "mao", "mi", "Maori", "maori"},
	{"arn", "", "", "Mapudungun; Mapuche", "mapuche"},
	{"mar", "", "mr", "Marathi", "marathe"},
	{"chm", "", "", "Mari", "mari"},
	{"mah", "", "mh", "Marshallese", "marshallais"},
	{"mwr", "", "", "Marwari", "marwarî"},
	{"mas", "", "", "Masai", "massaï"},
	{"myn", "", "", "Mayan languages", ""},
	{"men", "", "", "Mende", "mendé"},
	{"mic", "", "", "Mi'kmaq; Micmac", "micmac"},
	{"min", "", "", "Minangkabau", "minangkabau"},
	{"mwl", "", "", "Mirandese", "mirandais"},
	{"moh", "", "", "Mohawk", "mohawk"},
	{"mdf", "", "", "Moksha", "moksa"},
	{"mkh", "", "", "Mon-Khmer languages", ""},
	{"lol", "", "", "Mongo", "mongo"},
	{"mon", "", "mn", "Mongolian", "mongol"},
	{"mos", "", "", "Mossi", "moré"},
	{"mul", "", "", "Multiple languages", ""},
	{"mun", "", "", "Munda languages", ""},
	{"nqo", "", "", "N'Ko", "n’ko"},
	{"nah", "", "", "Nahuatl languages", ""},
	{"nau", "", "na", "Nauru", "nauruan"},
	{"nav", "", "nv", "Navajo; Navaho", "navaho"},
	{"nde", "", "nd", "Ndebele, North; North Ndebele", "ndébélé du Nord"},
	{"nbl", "", "nr", "Ndebele, South; South Ndebele", "ndébélé du Sud"},
	{"ndo", "", "ng", "Ndonga", "ndonga"},
	{"nap", "", "", "Neapolitan", "napolitain"},
	{"new", "", "", "Nepal Bhasa; Newari", "newari"},
	{"nep", "", "ne", "Nepali", "népalais"},
	{"nia", "", "", "Nias", "nias"},
	{"nic", "", "", "Niger-Kordofanian languages", ""},
	{"ssa", "", "", "Nilo-Saharan languages", ""},
	{"niu", "", "", "Niuean", "niuéen"},
	{"zxx", "", "", "No linguistic content; Not applicable", ""},
	{"nog", "", "", "Nogai", "nogaï"},
	{"non", "", "", "Norse, Old", "vieux norrois"},
	{"nai", "", "", "North American Indian languages", ""},
	{"frr", "", "", "Northern Frisian", "frison du Nord"},
	{"sme", "", "se", "Northern Sami", "sami du Nord"},
	{"nor", "", "no", "Norwegian", "norvégien bokmål"},
	{"nob", "", "nb", "Norwegian (Bokmal)", "norvégien bokmål"},
	{"nno", "", "nn", "Norwegian (Nynorsk)", "norvégien nynorsk"},
	{"nub", "", "", "Nubian languages", ""},
	{"nym", "", "", "Nyamwezi", "nyamwezi"},
	{"nyn", "", "", "Nyankole", "nyankolé"},
	{"nyo", "", "", "Nyoro", "nyoro"},
	{"nzi", "", "", "Nzima", "nzema"},
	{"oci", "", "oc", "Occitan (post 1500); Provençal", "occitan"},
	{"arc", "", "", "Official Aramaic (700-300 BCE); Imperial Aramaic (700-300 BCE)", "araméen"},
	{"oji", "", "oj", "Ojibwa", "ojibwa"},
	{"ori", "", "or", "Oriya", "oriya"},
	{"orm", "", "om", "Oromo", "oromo"},
	{"osa", "", "", "Osage", "osage"},
	{"oss", "", "os", "Ossetian; Ossetic", "ossète"},
	{"oto", "", "", "Otomian languages", ""},
	{"pal", "", "", "Pahlavi", "pahlavi"},
	{"pau", "", "", "Palauan", "palau"},
	{"pli", "", "pi", "Pali", "pali"},
	{"pam", "", "", "Pampanga; Kapampangan", "pampangan"},
	{"pag", "", "", "Pangasinan", "pangasinan"},
	{"pan", "", "pa", "Panjabi; Punjabi", "pendjabi"},
	{"pap", "", "", "Papiamento", "papiamento"},
	{"paa", "", "", "Papuan languages", ""},
	{"nso", "", "", "Pedi; Sepedi; Northern Sotho", "sotho du Nord"},
	{"fas", "per", "fa", "Persian", "persan"},
	{"peo", "", "", "Persian, Old (ca.600-400 B.C.)", "persan ancien"},
	{"phi", "", "", "Philippine languages", ""},
	{"phn", "", "", "Phoenician", "phénicien"},
	{"pon", "", "", "Pohnpeian", "pohnpei"},
	{"pol", "", "pl", "Polish", "polonais"},
	{"por", "", "pt", "Portuguese", "portugais"},
	{"por", "", "pt-br", "Portuguese (Brazil)", ""},
	{"por", "", "pt-pt", "Portuguese (Portugal)", ""},
	{"pra", "", "", "Prakrit languages", ""},
	{"pro", "", "", "Provençal, Old (to 1500)", "provençal ancien"},
	{"pus", "", "ps", "Pushto; Pashto", "pachto"},
	{"que", "", "qu", "Quechua", "quechua"},
	{"raj", "", "", "Rajasthani", "rajasthani"},
	{"rap", "", "", "Rapanui", "rapanui"},
	{"rar", "", "", "Rarotongan; Cook Islands Maori", "rarotongien"},
	{"qaa-qtz", "", "", "Reserved for local use", ""},
	{"roa", "", "", "Romance languages", ""},
	{"ron", "rum", "ro", "Romanian; Moldavian; Moldovan", "roumain"},
	{"roh", "", "rm", "Romansh", "romanche"},
	{"rom", "", "", "Romany", "romani"},
	{"run", "", "rn", "Rundi", "roundi"},
	{"rus", "", "ru", "Russian", "russe"},
	{"sal", "", "", "Salishan languages", ""},
	{"sam", "", "", "Samaritan Aramaic", "araméen samaritain"},
	{"smn", "", "", "Sami (Inari)", "sami d’Inari"},
	{"smj", "", "", "Sami (Lule)", "sami de Lule"},
	{"sms", "", "", "Sami (Skolt)", "sami skolt"},
	{"smi", "", "", "Sami languages", ""},
	{"smo", "", "sm", "Samoan", "samoan"},
	{"sad", "", "", "Sandawe", "sandawe"},
	{"sag", "", "sg", "Sango", "sangho"},
	{"san", "", "sa", "Sanskrit", "sanskrit"},
	{"sat", "", "", "Santali", "santal"},
	{"srd", "", "sc", "Sardinian", "sarde"},
	{"sas", "", "", "Sasak", "sasak"},
	{"sco", "", "", "Scots", "écossais"},
	{"sel", "", "", "Selkup", "selkoupe"},
	{"sem", "", "", "Semitic languages", ""},
	{"srp", "", "sr", "Serbian", "serbe"},
	{"srr", "", "", "Serer", "sérère"},
	{"shn", "", "", "Shan", "shan"},
	{"sna", "", "sn", "Shona", "shona"},
	{"iii", "", "ii", "Sichuan Yi; Nuosu", "yi du Sichuan"},
	{"scn", "", "", "Sicilian", "sicilien"},
	{"sid", "", "", "Sidamo", "sidamo"},
	{"sgn", "", "", "Sign Languages", ""},
	{"bla", "", "", "Siksika", "siksika"},
	{"snd", "", "sd", "Sindhi", "sindhi"},
	{"sin", "", "si", "Sinhala; Sinhalese", "cinghalais"},
	{"sit", "", "", "Sino-Tibetan languages", ""},
	{"sio", "", "", "Siouan languages", ""},
	{"den", "", "", "Slave (Athapascan)", "esclave"},
	{"sla", "", "", "Slavic languages", ""},
	{"slk", "slo", "sk", "Slovak", "slovaque"},
	{"slv", "", "sl", "Slovenian", "slovène"},
	{"sog", "", "", "Sogdian", "sogdien"},
	{"som", "", "so", "Somali", "somali"},
	{"son", "", "", "Songhai languages", ""},
	{"snk", "", "", "Soninke", "soninké"},
	{"wen", "", "", "Sorbian languages", ""},
	{"sot", "", "st", "Sotho, Southern", "sotho du Sud"},
	{"sai", "", "", "South American Indian (Other)", ""},
	{"alt", "", "", "Southern Altai", "altaï du Sud"},
	{"sma", "", "", "Southern Sami", "sami du Sud"},
	{"spa", "", "es", "Spanish; Castilian", "espagnol"},
	{"spa", "", "es-419", "Spanish; Latin", ""},
	{"srn", "", "", "Sranan Tongo", "sranan tongo"},
	{"zgh", "", "", "Standard Moroccan Tamazight", "amazighe standard marocain"},
	{"suk", "", "", "Sukuma", "soukouma"},
	{"sux", "", "", "Sumerian", "sumérien"},
	{"sun", "", "su", "Sundanese", "soundanais"},
	{"sus", "", "", "Susu", "soussou"},
	{"swa", "", "sw", "Swahili", "swahili"},
	{"ssw", "", "ss", "Swati", "swati"},
	{"swe", "", "sv", "Swedish", "suédois"},
	{"gsw", "", "", "Swiss German; Alemannic; Alsatian", "suisse allemand"},
	{"syr", "", "", "Syriac", "syriaque"},
	{"tgl", "", "tl", "Tagalog", "filipino"},
	{"tah", "", "ty", "Tahitian", "tahitien"},
	{"tai", "", "", "Tai languages", ""},
	{"tgk", "", "tg", "Tajik", "tadjik"},
	{"tmh", "", "", "Tamashek", "tamacheq"},
	{"tam", "", "ta", "Tamil", "tamoul"},
	{"tat", "", "tt", "Tatar", "tatar"},
	{"tel", "", "te", "Telugu", "télougou"},
	{"ter", "", "", "Tereno", "tereno"},
	{"tet", "", "", "Tetum", "tetum"},
	{"tha", "", "th", "Thai", "thaï"},
	{"bod", "tib", "bo", "Tibetan", "tibétain"},
	{"tig", "", "", "Tigre", "tigré"},
	{"tir", "", "ti", "Tigrinya", "tigrigna"},
	{"tem", "", "", "Timne", "temne"},
	{"tiv", "", "", "Tiv", "tiv"},
	{"tli", "", "", "Tlingit", "tlingit"},
	{"tpi", "", "", "Tok Pisin", "tok pisin"},
	{"tkl", "", "", "Tokelau", "tokelau"},
	{"tog", "", "", "Tonga (Nyasa)", "tonga nyasa"},
	{"ton", "", "to", "Tonga (Tonga Islands)", "tonguien"},
	{"tsi", "", "", "Tsimshian", "tsimshian"},
	{"tso", "", "ts", "Tsonga", "tsonga"},
	{"tsn", "", "tn", "Tswana", "tswana"},
	{"tum", "", "", "Tumbuka", "toumbouka"},
	{"tup", "", "", "Tupi languages", ""},
	{"tur", "", "tr", "Turkish", "turc"},
	{"ota", "", "", "Turkish, Ottoman (1500-1928)", "turc ottoman"},
	{"tuk", "", "tk", "Turkmen", "turkmène"},
	{"tvl", "", "", "Tuvalu", "tuvalu"},
	{"tyv", "", "", "Tuvinian", "touvain"},
	{"twi", "", "tw", "Twi", "akan"},
	{"udm", "", "", "Udmurt", "oudmourte"},
	{"uga", "", "", "Ugaritic", "ougaritique"},
	{"uig", "", "ug", "Uighur; Uyghur", "ouïghour"},
	{"ukr", "", "uk", "Ukrainian", "ukrainien"},
	{"umb", "", "", "Umbundu", "oumboundou"},
	{"mis", "", "", "Uncoded languages", ""},
	{"und", "", "", "Undetermined", ""},
	{"hsb", "", "", "Upper Sorbian", "haut-sorabe"},
	{"urd", "", "ur", "Urdu", "ourdou"},
	{"uzb", "", "uz", "Uzbek", "ouzbek"},
	{"vai", "", "", "Vai", "vaï"},
	{"ven", "", "ve", "Venda", "venda"},
	{"vie", "", "vi", "Vietnamese", "vietnamien"},
	{"vol", "", "vo", "Volapük", "volapuk"},
	{"vot", "", "", "Votic", "vote"},
	{"wak", "", "", "Wakashan languages", ""},
	{"wal", "", "", "Walamo", "walamo"},
	{"wln", "", "wa", "Walloon", "wallon"},
	{"war", "", "", "Waray", "waray"},
	{"was", "", "", "Washo", "washo"},
	{"cym", "wel", "cy", "Welsh", "gallois"},
	{"fry", "", "fy", "Western Frisian", "frison occidental"},
	{"wol", "", "wo", "Wolof", "wolof"},
	{"xho", "", "xh", "Xhosa", "xhosa"},
	{"sah", "", "", "Yakut", "iakoute"},
	{"yao", "", "", "Yao", "yao"},
	{"yap", "", "", "Yapese", "yapois"},
	{"yid", "", "yi", "Yiddish", "yiddish"},
	{"yor", "", "yo", "Yoruba", "yoruba"},
	{"ypk", "", "", "Yupik languages", ""},
	{"znd", "", "", "Zande languages", ""},
	{"zap", "", "", "Zapotec", "zapotèque"},
	{"zza", "", "", "Zaza; Dimili; Dimli; Kirdki; Kirmanjki; Zazaki", "zazaki"},
	{"zen", "", "", "Zenaga", "zenaga"},
	{"zha", "", "za", "Zhuang; Chuang", "zhuang"},
	{"zul", "", "zu", "Zulu", "zoulou"},
	{"zun", "", "", "Zuni", "zuñi"},
}
