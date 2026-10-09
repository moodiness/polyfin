package anime

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Source names a database that identifies anime entries or titles.
type Source string

// The sources the mapping knows. Stremio identifiers name anime by the
// first three: kitsu:<id>, mal:<id> and anidb:<id>.
const (
	Kitsu       Source = "kitsu"
	MyAnimeList Source = "mal"
	AniDB       Source = "anidb"
	AniList     Source = "anilist"
	TVDB        Source = "tvdb"
	IMDb        Source = "imdb"
	// TMDB numbers its shows and its movies apart: the same number may be
	// a show and a movie.
	TMDBShow  Source = "tmdb_tv"
	TMDBMovie Source = "tmdb_movie"
)

// IDs are the identifiers of one AniDB entry, as the identifier list
// gives them; zero or empty where it gives none.
type IDs struct {
	AniDB, Kitsu, MyAnimeList, AniList int
	// TVDB is the series the entry is part of on TVDB, TMDBShow the show
	// on TMDB.
	TVDB, TMDBShow int
	// TMDBMovies are the movies on TMDB the entry is, IMDb its titles on
	// IMDb: a movie's, or the series the entry is part of.
	TMDBMovies []int
	IMDb       []string
	// Movie is set for an entry AniDB counts as a movie.
	Movie bool
}

// idList is the identifier list, with an index by each identifier.
type idList struct {
	entries []IDs
	// first holds, for the anime sources, the entry of each identifier;
	// many holds, for TVDB and TMDB's, every entry of each identifier.
	first map[Source]map[int]int32
	many  map[Source]map[int][]int32
	imdb  map[string][]int32
}

// lookup lists the entries source identifies by id.
func (l *idList) lookup(source Source, id string) []IDs {
	if l == nil {
		return nil
	}
	var found []int32
	switch source {
	case IMDb:
		found = l.imdb[strings.TrimSpace(id)]
	case TVDB, TMDBShow, TMDBMovie:
		n, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil {
			return nil
		}
		found = l.many[source][n]
	default:
		n, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil {
			return nil
		}
		if i, ok := l.first[source][n]; ok {
			found = []int32{i}
		}
	}
	result := make([]IDs, 0, len(found))
	for _, i := range found {
		result = append(result, l.entries[i])
	}
	return result
}

// flexInt decodes an identifier given as a number, a numeric string or
// null; anything else reads as zero.
type flexInt int

func (n *flexInt) UnmarshalJSON(data []byte) error {
	value, err := strconv.Atoi(strings.Trim(string(bytes.TrimSpace(data)), `"`))
	if err != nil || value < 0 {
		value = 0
	}
	*n = flexInt(value)
	return nil
}

// imdbIDs decodes IMDb identifiers given as an array, or as one string
// that may list several separated by commas, as older lists did.
type imdbIDs []string

func (ids *imdbIDs) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		var one string
		if json.Unmarshal(data, &one) != nil {
			*ids = nil
			return nil
		}
		list = strings.Split(one, ",")
	}
	*ids = nil
	for _, id := range list {
		if id = strings.TrimSpace(id); strings.HasPrefix(id, "tt") && !slices.Contains(*ids, id) {
			*ids = append(*ids, id)
		}
	}
	return nil
}

// tmdbIDs decodes the TMDB identifiers of an entry: {"tv": n, "movie":
// [n…]}, or a lone number as older lists gave, whose kind the entry's
// type tells.
type tmdbIDs struct {
	TV     int
	Movies []int
	lone   int
}

func (t *tmdbIDs) UnmarshalJSON(data []byte) error {
	*t = tmdbIDs{}
	var lone flexInt
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] != '{' {
		_ = lone.UnmarshalJSON(trimmed)
		t.lone = int(lone)
		return nil
	}
	var both struct {
		TV    flexInt         `json:"tv"`
		Movie json.RawMessage `json:"movie"`
	}
	if json.Unmarshal(data, &both) != nil {
		return nil
	}
	t.TV = int(both.TV)
	var movies []flexInt
	if json.Unmarshal(both.Movie, &movies) != nil {
		var one flexInt
		_ = json.Unmarshal(both.Movie, &one)
		movies = []flexInt{one}
	}
	for _, id := range movies {
		if id > 0 {
			t.Movies = append(t.Movies, int(id))
		}
	}
	return nil
}

// idEntry is an entry of the identifier list as it reads.
type idEntry struct {
	Type    string  `json:"type"`
	AniDB   flexInt `json:"anidb_id"`
	Kitsu   flexInt `json:"kitsu_id"`
	MAL     flexInt `json:"mal_id"`
	AniList flexInt `json:"anilist_id"`
	TVDB    flexInt `json:"tvdb_id"`
	// TheTVDB is how older lists named tvdb_id.
	TheTVDB flexInt `json:"thetvdb_id"`
	IMDb    imdbIDs `json:"imdb_id"`
	TMDB    tmdbIDs `json:"themoviedb_id"`
}

// errNoEntries reports a list read whole that holds no entry: an answer
// that is not the list.
var errNoEntries = errors.New("the list holds no entry")

// readIDs reads the identifier list, a JSON array of entries, one entry at
// a time.
func readIDs(r io.Reader) (*idList, error) {
	decoder := json.NewDecoder(r)
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return nil, errors.New("the identifier list is not a JSON array")
	}
	l := &idList{
		first: map[Source]map[int]int32{AniDB: {}, Kitsu: {}, MyAnimeList: {}, AniList: {}},
		many:  map[Source]map[int][]int32{TVDB: {}, TMDBShow: {}, TMDBMovie: {}},
		imdb:  map[string][]int32{},
	}
	for decoder.More() {
		var e idEntry
		if err := decoder.Decode(&e); err != nil {
			return nil, err
		}
		ids := IDs{AniDB: int(e.AniDB), Kitsu: int(e.Kitsu), MyAnimeList: int(e.MAL), AniList: int(e.AniList),
			TVDB: int(cmpOr(e.TVDB, e.TheTVDB)), TMDBShow: e.TMDB.TV, TMDBMovies: e.TMDB.Movies, IMDb: e.IMDb,
			Movie: strings.EqualFold(e.Type, "movie")}
		if e.TMDB.lone > 0 {
			if ids.Movie {
				ids.TMDBMovies = []int{e.TMDB.lone}
			} else {
				ids.TMDBShow = e.TMDB.lone
			}
		}
		if ids.AniDB == 0 && ids.Kitsu == 0 && ids.MyAnimeList == 0 && ids.AniList == 0 {
			continue
		}
		i := int32(len(l.entries))
		l.entries = append(l.entries, ids)
		for _, source := range [...]struct {
			source Source
			id     int
		}{{AniDB, ids.AniDB}, {Kitsu, ids.Kitsu}, {MyAnimeList, ids.MyAnimeList}, {AniList, ids.AniList}} {
			if _, taken := l.first[source.source][source.id]; source.id > 0 && !taken {
				l.first[source.source][source.id] = i
			}
		}
		if ids.TVDB > 0 {
			l.many[TVDB][ids.TVDB] = append(l.many[TVDB][ids.TVDB], i)
		}
		if ids.TMDBShow > 0 {
			l.many[TMDBShow][ids.TMDBShow] = append(l.many[TMDBShow][ids.TMDBShow], i)
		}
		for _, id := range ids.TMDBMovies {
			if !slices.Contains(l.many[TMDBMovie][id], i) {
				l.many[TMDBMovie][id] = append(l.many[TMDBMovie][id], i)
			}
		}
		for _, id := range ids.IMDb {
			l.imdb[id] = append(l.imdb[id], i)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if len(l.entries) == 0 {
		return nil, errNoEntries
	}
	l.entries = slices.Clip(l.entries)
	return l, nil
}

func cmpOr[T comparable](a, b T) T {
	var zero T
	if a != zero {
		return a
	}
	return b
}

// Place is where an AniDB episode is on TVDB: episode Number of season
// Season of series TVDB, or, when Absolute, the episode numbered Number
// in TVDB's absolute numbering of the series, which counts its regular
// episodes from its first, across its seasons.
type Place struct {
	TVDB           int
	Season, Number int
	Absolute       bool
}

// episodeList is the episode list: how the episodes of each AniDB entry
// are numbered on TVDB.
type episodeList struct {
	entries map[int32]*seriesMap
}

// seriesMap is how the episodes of an AniDB entry are numbered on TVDB:
// in season, absolute when it is absoluteSeason, after adding offset,
// unless a mapping says otherwise.
type seriesMap struct {
	tvdb     int32
	season   int16
	offset   int32
	mappings []mapping
}

// absoluteSeason marks an entry numbered as TVDB's absolute numbering.
const absoluteSeason = -1

// mapping is a mapping of the mapping-list: the AniDB episodes of
// anidbSeason (1 regular, 0 specials) from start to end are those of
// tvdbSeason after adding offset (when end is set), and pairs name
// episodes one by one.
type mapping struct {
	anidbSeason, tvdbSeason int16
	start, end, offset      int32
	pairs                   []pair
}

// pair maps an AniDB episode to a TVDB episode, none when tvdb is 0.
type pair struct {
	anidb, tvdb int32
}

// place finds where the episode number of anidb is on TVDB: the regular
// episodes, or the specials when special.
func (l *episodeList) place(anidb int, special bool, number int) (Place, bool) {
	if l == nil || number <= 0 || anidb <= 0 || anidb > maxID {
		return Place{}, false
	}
	m := l.entries[int32(anidb)]
	if m == nil {
		return Place{}, false
	}
	season := int16(1)
	if special {
		season = 0
	}
	// An episode named one by one wins over ranges and the default.
	for _, mp := range m.mappings {
		if mp.anidbSeason != season {
			continue
		}
		for _, p := range mp.pairs {
			if int(p.anidb) == number {
				if p.tvdb <= 0 {
					return Place{}, false
				}
				return Place{TVDB: int(m.tvdb), Season: int(mp.tvdbSeason), Number: int(p.tvdb)}, true
			}
		}
	}
	for _, mp := range m.mappings {
		if mp.anidbSeason == season && mp.end > 0 && int(mp.start) <= number && number <= int(mp.end) {
			if tvdb := number + int(mp.offset); tvdb > 0 {
				return Place{TVDB: int(m.tvdb), Season: int(mp.tvdbSeason), Number: tvdb}, true
			}
			return Place{}, false
		}
	}
	// Specials are where the mapping-list puts them, nowhere else.
	if special {
		return Place{}, false
	}
	tvdb := number + int(m.offset)
	if tvdb <= 0 {
		return Place{}, false
	}
	if m.season == absoluteSeason {
		return Place{TVDB: int(m.tvdb), Number: tvdb, Absolute: true}, true
	}
	return Place{TVDB: int(m.tvdb), Season: int(m.season), Number: tvdb}, true
}

// maxID bounds the identifiers and numbers kept, which fit in 32 bits.
const maxID = 1<<31 - 1

// episodeEntry is an anime element of the episode list as it reads.
type episodeEntry struct {
	AniDB         string `xml:"anidbid,attr"`
	TVDB          string `xml:"tvdbid,attr"`
	DefaultSeason string `xml:"defaulttvdbseason,attr"`
	Offset        string `xml:"episodeoffset,attr"`
	Mappings      []struct {
		AniDBSeason string `xml:"anidbseason,attr"`
		TVDBSeason  string `xml:"tvdbseason,attr"`
		Start       string `xml:"start,attr"`
		End         string `xml:"end,attr"`
		Offset      string `xml:"offset,attr"`
		Pairs       string `xml:",chardata"`
	} `xml:"mapping-list>mapping"`
}

// number reads a number of the list, ok false when text is not one.
func number(text string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return n, err == nil && n >= -maxID && n <= maxID
}

// readEpisodes reads the episode list, an XML document of anime elements,
// one element at a time. Entries that are not part of a TVDB series (a
// movie, or one TVDB does not list) are left out.
func readEpisodes(r io.Reader) (*episodeList, error) {
	decoder := xml.NewDecoder(r)
	l := &episodeList{entries: map[int32]*seriesMap{}}
	sawList := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "anime-list":
			sawList = true
			continue
		case "anime":
		default:
			continue
		}
		var e episodeEntry
		if err := decoder.DecodeElement(&e, &start); err != nil {
			return nil, err
		}
		anidb, ok := number(e.AniDB)
		tvdb, isSeries := number(e.TVDB)
		if !ok || anidb <= 0 || !isSeries || tvdb <= 0 {
			continue
		}
		m := &seriesMap{tvdb: int32(tvdb)}
		switch season, ok := number(e.DefaultSeason); {
		case strings.TrimSpace(e.DefaultSeason) == "a":
			m.season = absoluteSeason
		case ok && season >= 0 && season < 1<<15:
			m.season = int16(season)
		default:
			m.season = 1
		}
		if offset, ok := number(e.Offset); ok {
			m.offset = int32(offset)
		}
		for _, raw := range e.Mappings {
			anidbSeason, ok1 := number(raw.AniDBSeason)
			tvdbSeason, ok2 := number(raw.TVDBSeason)
			// Mappings to TMDB seasons only are not TVDB's.
			if !ok1 || !ok2 || anidbSeason < 0 || anidbSeason > 1 || tvdbSeason < 0 || tvdbSeason >= 1<<15 {
				continue
			}
			mp := mapping{anidbSeason: int16(anidbSeason), tvdbSeason: int16(tvdbSeason)}
			start, okStart := number(raw.Start)
			end, okEnd := number(raw.End)
			if okStart && okEnd && start > 0 && end >= start {
				offset, _ := number(raw.Offset)
				mp.start, mp.end, mp.offset = int32(start), int32(end), int32(offset)
			}
			mp.pairs = pairs(raw.Pairs)
			if mp.end > 0 || len(mp.pairs) > 0 {
				m.mappings = append(m.mappings, mp)
			}
		}
		l.entries[int32(anidb)] = m
	}
	if !sawList || len(l.entries) == 0 {
		return nil, errNoEntries
	}
	return l, nil
}

// pairs reads the episodes a mapping names one by one: ";1-5;2-6;", the
// AniDB episode then the TVDB one, 0 for none. Of an AniDB episode that
// spans several TVDB episodes ("1-1+2"), the first counts.
func pairs(text string) []pair {
	var result []pair
	for _, item := range strings.Split(text, ";") {
		anidbText, tvdbText, ok := strings.Cut(strings.TrimSpace(item), "-")
		if !ok {
			continue
		}
		tvdbText, _, _ = strings.Cut(tvdbText, "+")
		anidb, ok1 := number(anidbText)
		tvdb, ok2 := number(tvdbText)
		if ok1 && ok2 && anidb > 0 && tvdb >= 0 {
			result = append(result, pair{anidb: int32(anidb), tvdb: int32(tvdb)})
		}
	}
	return result
}
