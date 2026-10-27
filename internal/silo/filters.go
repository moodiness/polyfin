package silo

import (
	"cmp"
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Filters.
//
// Silo's apps filter a library's titles, and the catalog's, by rule
// groups (see filter): those of GET /api/v2/catalog's groups parameter,
// which the Apple apps send, or of the body of POST /api/v2/catalog/query,
// which the Android apps send. Their filter sheets offer the values GET
// /api/v2/catalog/filters lists (genres and content ratings), and values
// of their own: decades, what the user did with a title, and its dynamic
// range, which Polyfin refuses as it knows no title's.
//
// Polyfin's titles come from addon catalogs, which narrow their titles by
// their genre filter alone (see package jellyfin's pages.go). A genre one
// must have is therefore asked of the Jellyfin API, which lists a
// library's titles, or the catalog's, of that genre as Jellyfin apps get
// them; the other rules keep, among the titles listed, those that hold to
// them, read a chunk at a time up to maxScanned titles. The genres a
// library offers are those of its catalog's filter, as the Jellyfin API's
// /Items/Filters gives them. A library of collections lists the titles of
// its collections, whose own genre filter, if any, chooses collections:
// it is filtered by its titles' genres, and offers those.

func (s *Server) filterRoutes() {
	s.mux.HandleFunc("GET /api/v2/catalog/filters", s.profiled(s.catalogFilters))
	s.mux.HandleFunc("POST /api/v2/catalog/query", s.profiled(s.catalogQuery))
}

const (
	// maxScanned bounds the titles a filtered page, or a filter sheet's
	// options, read: as many as a movie or series catalog lists by
	// default, and as the Jellyfin API's listings that leave titles out
	// read (scanLimit, in package jellyfin's userlists.go).
	maxScanned = 2000
	// scanChunk is how many titles each read of a listing asks.
	scanChunk = 100
	// maxGroupsLength bounds the groups parameter, as the contract does:
	// larger filters go in the body of POST /api/v2/catalog/query.
	maxGroupsLength = 32768
)

// filter is what a page is narrowed to: groups, all or any of which must
// hold, each of all or any of its rules.
type filter struct {
	any    bool
	groups []ruleGroup
}

type ruleGroup struct {
	any   bool
	rules []rule
}

// rule is a condition on a title's field: its type (text: movie, series
// or episode), a genre or its content rating (text), its year (between
// low and high), or what the user did with it (flag: watched, favorited,
// in_progress, in_watchlist). A negated rule holds when the condition
// does not.
type rule struct {
	field     string
	negate    bool
	text      string
	low, high int
	flag      bool
}

// narrows reports whether the filter leaves any title out.
func (f filter) narrows() bool {
	return slices.ContainsFunc(f.groups, func(g ruleGroup) bool { return len(g.rules) > 0 })
}

// wireGroup and wireRule are the contract's CatalogQueryGroup and
// CatalogQueryRule.
type wireGroup struct {
	Match string     `json:"match"`
	Rules []wireRule `json:"rules"`
}

type wireRule struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// filterAsked reads the filter of a catalog request q, sent in the
// request's query or body (in), answering the request when it cannot.
func filterAsked(w http.ResponseWriter, q url.Values, in string) (filter, bool) {
	var f filter
	switch q.Get("match") {
	case "", "all":
	case "any":
		f.any = true
	default:
		validationProblem(w, in+".match", "invalid_enum", "match is all or any.")
		return filter{}, false
	}
	text := q.Get("groups")
	if text == "" {
		return f, true
	}
	if in == "query" && len(text) > maxGroupsLength {
		validationProblem(w, "query.groups", "out_of_range", "groups is at most 32768 characters; send larger filters in a body.")
		return filter{}, false
	}
	var groups []wireGroup
	if err := json.Unmarshal([]byte(text), &groups); err != nil {
		validationProblem(w, in+".groups", "invalid", "groups is not a JSON array of rule groups.")
		return filter{}, false
	}
	for _, g := range groups {
		var group ruleGroup
		switch g.Match {
		case "", "all":
		case "any":
			group.any = true
		default:
			validationProblem(w, in+".groups", "invalid_enum", "A group's match is all or any.")
			return filter{}, false
		}
		for _, wire := range g.Rules {
			r, code, detail := ruleOf(wire)
			if code != "" {
				validationProblem(w, in+".groups", code, detail)
				return filter{}, false
			}
			group.rules = append(group.rules, r)
		}
		f.groups = append(f.groups, group)
	}
	return f, true
}

// ruleOf reads a rule, or the code and detail of the problem it has.
// Fields Polyfin cannot tell of its titles are refused as unsupported.
func ruleOf(w wireRule) (rule, string, string) {
	r := rule{field: w.Field}
	invalidOp := "The operator " + strconv.Quote(w.Op) + " does not apply to " + w.Field + "."
	switch w.Field {
	case "type", "genre", "content_rating":
		switch {
		case w.Op == "is", w.Field == "genre" && w.Op == "contains":
		case w.Op == "is_not", w.Field == "genre" && w.Op == "not_contains":
			r.negate = true
		default:
			return rule{}, "invalid", invalidOp
		}
		var text string
		if json.Unmarshal(w.Value, &text) != nil || strings.TrimSpace(text) == "" {
			return rule{}, "invalid_type", w.Field + " is compared with a text."
		}
		r.text = strings.TrimSpace(text)
	case "year":
		if w.Op == "between" {
			var bounds []json.RawMessage
			if json.Unmarshal(w.Value, &bounds) != nil || len(bounds) != 2 {
				return rule{}, "invalid_type", "year between takes two years."
			}
			low, lowOK := yearOf(bounds[0])
			high, highOK := yearOf(bounds[1])
			if !lowOK || !highOK {
				return rule{}, "invalid_type", "year between takes two years."
			}
			r.low, r.high = low, high
			break
		}
		year, ok := yearOf(w.Value)
		if !ok {
			return rule{}, "invalid_type", "year is compared with a year."
		}
		switch w.Op {
		case "is":
			r.low, r.high = year, year
		case "is_not":
			r.low, r.high, r.negate = year, year, true
		case "gt":
			r.low, r.high = year+1, math.MaxInt
		case "gte":
			r.low, r.high = year, math.MaxInt
		case "lt":
			r.low, r.high = math.MinInt, year-1
		case "lte":
			r.low, r.high = math.MinInt, year
		default:
			return rule{}, "invalid", invalidOp
		}
	case "watched", "favorited", "in_progress", "in_watchlist":
		if w.Op != "is" {
			return rule{}, "invalid", invalidOp
		}
		// The Apple apps send a boolean, the Android apps its text.
		var flag bool
		var text string
		switch {
		case json.Unmarshal(w.Value, &flag) == nil:
		case json.Unmarshal(w.Value, &text) == nil && (text == "true" || text == "false"):
			flag = text == "true"
		default:
			return rule{}, "invalid_type", w.Field + " is compared with true or false."
		}
		r.flag = flag
	default:
		return rule{}, "unsupported", "This server cannot filter by " + strconv.Quote(truncate(w.Field)) + "."
	}
	return r, "", ""
}

// yearOf reads a year, a number as the Apple apps send it or its text as
// the Android apps do.
func yearOf(raw json.RawMessage) (int, bool) {
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return int(number), number == math.Trunc(number) && math.Abs(number) < 1e6
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	year, err := strconv.Atoi(strings.TrimSpace(text))
	return year, err == nil
}

// required are the rules every title kept holds to: those of the groups
// that hold only when all their rules do, or of a group of one, when every
// group must hold or there is only one.
func (f filter) required() []rule {
	if f.any && len(f.groups) > 1 {
		return nil
	}
	var rules []rule
	for _, g := range f.groups {
		if !g.any || len(g.rules) == 1 {
			rules = append(rules, g.rules...)
		}
	}
	return rules
}

// genres are genres one of which every title kept has, if any: a genre
// required, or those of a group of genres alone, any of which must hold.
func (f filter) genres() []string {
	if f.any && len(f.groups) > 1 {
		return nil
	}
	for _, r := range f.required() {
		if r.field == "genre" && !r.negate {
			return []string{r.text}
		}
	}
	for _, g := range f.groups {
		var names []string
		for _, r := range g.rules {
			if r.field != "genre" || r.negate {
				names = nil
				break
			}
			names = append(names, r.text)
		}
		if len(names) > 0 {
			return names
		}
	}
	return nil
}

// state is the Jellyfin API's filter of what the user did that every
// title kept passes, if any: played, favorite or resumable.
func (f filter) state() string {
	for _, r := range f.required() {
		switch {
		case !r.flag || r.negate:
		case r.field == "watched":
			return "IsPlayed"
		case r.field == "favorited":
			return "IsFavorite"
		case r.field == "in_progress":
			return "IsResumable"
		}
	}
	return ""
}

// apiTypes are the Jellyfin API's types of the apps' types.
var apiTypes = map[string]string{"movie": "Movie", "series": "Series", "episode": "Episode"}

// types narrows the Jellyfin API's types listed to those the required
// rules on the type keep.
func (f filter) types(listed string) string {
	kept := strings.Split(listed, ",")
	for _, r := range f.required() {
		if r.field != "type" {
			continue
		}
		kept = slices.DeleteFunc(kept, func(t string) bool { return (t == apiTypes[r.text]) == r.negate })
	}
	return strings.Join(kept, ",")
}

// judge tells whether titles hold to a filter for a user.
type judge struct {
	r       *http.Request
	blocked []string
	// resumable are the items the user can resume, read when a rule asks.
	resumable map[string]bool
}

// judgeOf is the judge of a filter for v.
func (s *Server) judgeOf(r *http.Request, v viewer, f filter) (judge, answer, error) {
	j := judge{r: r, blocked: v.user.BlockedGenres}
	asks := slices.ContainsFunc(f.groups, func(g ruleGroup) bool {
		return slices.ContainsFunc(g.rules, func(r rule) bool { return r.field == "in_progress" })
	})
	if !asks {
		return j, answer{status: http.StatusOK}, nil
	}
	var result queryResult
	// The user's data lists them all at once, without reading a catalog.
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"Filters": {"IsResumable"}, "Recursive": {"true"},
		"Limit": {strconv.Itoa(math.MaxInt32)}}.Encode(), nil, &result)
	j.resumable = map[string]bool{}
	for _, i := range result.Items {
		j.resumable[i.Id] = true
	}
	return j, a, err
}

// blockedGenre reports whether the user may not see a genre's titles.
func (j judge) blockedGenre(genre string) bool {
	return slices.ContainsFunc(j.blocked, func(b string) bool { return strings.EqualFold(b, genre) })
}

// keeps reports whether a title holds to a filter. listedAs is the genre
// the Jellyfin API listed it for, if any, which it has whatever its
// description says.
func (j judge) keeps(f filter, i item, listedAs string) bool {
	groupHolds := func(g ruleGroup) bool {
		if len(g.rules) == 0 {
			return true
		}
		for _, r := range g.rules {
			if j.holds(r, i, listedAs) == g.any {
				return g.any
			}
		}
		return !g.any
	}
	if len(f.groups) == 0 {
		return true
	}
	for _, g := range f.groups {
		if groupHolds(g) == f.any {
			return f.any
		}
	}
	return !f.any
}

// holds reports whether a title holds to a rule. A year or rating the
// title does not have holds to none, negated or not; a genre the user may
// not see is no title's.
func (j judge) holds(r rule, i item, listedAs string) bool {
	var held bool
	switch r.field {
	case "type":
		held = kinds[i.Type] == r.text
	case "genre":
		held = !j.blockedGenre(r.text) && (strings.EqualFold(listedAs, r.text) ||
			slices.ContainsFunc(i.Genres, func(g string) bool { return strings.EqualFold(strings.TrimSpace(g), r.text) }))
	case "content_rating":
		if i.OfficialRating == "" {
			return false
		}
		held = strings.EqualFold(i.OfficialRating, r.text)
	case "year":
		if i.ProductionYear == nil {
			return false
		}
		held = r.low <= *i.ProductionYear && *i.ProductionYear <= r.high
	case "watched":
		held = (i.UserData != nil && i.UserData.Played) == r.flag
	case "favorited":
		held = (i.UserData != nil && i.UserData.IsFavorite) == r.flag
	case "in_progress":
		held = j.resumable[i.Id] == r.flag
	case "in_watchlist":
		held = inWatchlist(j.r, i) == r.flag
	}
	return held != r.negate
}

// stream is a listing of the Jellyfin API that titles are read from, and
// the genre it lists the titles of, if any.
type stream struct {
	query url.Values
	genre string
}

// scope is where the titles of a listing are read from, narrowed as a
// filter allows: the Jellyfin API's listings, or a library's collections
// one by one (gathered, see libraryPage). collections is set for a library
// of collections.
type scope struct {
	streams     []stream
	gathered    bool
	collections bool
}

// scopeOf is where the titles l lists that f may keep are read from. The
// Jellyfin API narrows a library's titles, and the catalog's, to a type,
// to what the user did, or to its catalogs' genre: the first of these
// that every title kept has.
func (s *Server) scopeOf(r *http.Request, v viewer, l listing, f filter) (scope, answer, error) {
	answered := answer{status: http.StatusOK}
	base := maps.Clone(l.query)
	var sc scope
	if l.libraryTitles() {
		base.Set("ParentId", l.view.Id)
		base.Set("Recursive", "true")
		gathered, a, err := s.gathers(r, v, l.view)
		if err != nil || !a.ok() {
			return scope{}, a, err
		}
		if gathered {
			return scope{gathered: true, collections: true}, a, nil
		}
		if sc.collections, a, err = s.collectionLibrary(r, v, l.view); err != nil || !a.ok() {
			return scope{}, a, err
		}
	}
	// What the Jellyfin API narrows: a library's titles and the catalog's,
	// but not a search, nor a list of what the user did.
	if l.source != "query" || l.whole || l.term != "" {
		sc.streams = []stream{{query: base}}
		return sc, answered, nil
	}
	types := f.types(base.Get("IncludeItemTypes"))
	if types == "" {
		return sc, answered, nil
	}
	base.Set("IncludeItemTypes", types)
	if state := f.state(); state != "" {
		// The Jellyfin API lists what the user did whatever the genre.
		base.Set("Filters", state)
		sc.streams = []stream{{query: base}}
		return sc, answered, nil
	}
	if genres := f.genres(); len(genres) > 0 && !sc.collections {
		j := judge{blocked: v.user.BlockedGenres}
		for _, genre := range genres {
			if j.blockedGenre(genre) {
				continue
			}
			listed := maps.Clone(base)
			listed.Set("Genres", genre)
			sc.streams = append(sc.streams, stream{query: listed, genre: genre})
		}
		return sc, answered, nil
	}
	sc.streams = []stream{{query: base}}
	return sc, answered, nil
}

// titlesOf reads the titles of a scope keep keeps, until more than want of
// them are kept; done reports whether all were read.
func (s *Server) titlesOf(r *http.Request, v viewer, l listing, sc scope, want int, keep func(item, string) bool) ([]item, bool, answer, error) {
	if sc.gathered {
		titles, more, a, err := s.gatheredTitles(r, v, l.view, l.query, want, func(i item) bool { return keep(i, "") })
		return titles, !more, a, err
	}
	return s.scan(r, v, sc.streams, want, keep)
}

// scan reads the titles of streams, merged one of each in turn, each
// once, a chunk at a time, until more than want of them are kept or
// maxScanned are read; done reports whether all were read.
func (s *Server) scan(r *http.Request, v viewer, streams []stream, want int, keep func(item, string) bool) (kept []item, done bool, a answer, err error) {
	type reader struct {
		stream
		next    int
		pending []item
		ended   bool
	}
	readers := make([]*reader, len(streams))
	for n, st := range streams {
		readers[n] = &reader{stream: st}
	}
	a = answer{status: http.StatusOK}
	seen := map[string]bool{}
	scanned := 0
	for len(kept) <= want && scanned < maxScanned {
		took := false
		for _, rd := range readers {
			if len(rd.pending) == 0 && !rd.ended {
				query := maps.Clone(rd.query)
				query.Set("StartIndex", strconv.Itoa(rd.next))
				query.Set("Limit", strconv.Itoa(scanChunk))
				query.Set("EnableTotalRecordCount", "true")
				var result queryResult
				if a, err = s.call(r, v, http.MethodGet, "/Items?"+query.Encode(), nil, &result); err != nil || !a.ok() {
					return nil, false, a, err
				}
				rd.pending, rd.next = result.Items, rd.next+len(result.Items)
				rd.ended = len(result.Items) == 0 || rd.next >= result.TotalRecordCount
			}
			if len(rd.pending) == 0 {
				continue
			}
			i := rd.pending[0]
			rd.pending = rd.pending[1:]
			took = true
			if seen[i.Id] || !slices.Contains(browsable, i.Type) {
				continue
			}
			seen[i.Id] = true
			scanned++
			if keep(i, rd.genre) {
				kept = append(kept, i)
			}
			if len(kept) > want || scanned >= maxScanned {
				break
			}
		}
		if !took {
			return kept, true, a, nil
		}
	}
	return kept, false, a, nil
}

// filteredPage is a page from start of the titles l lists that f keeps;
// its total counts those read when not all were.
func (s *Server) filteredPage(r *http.Request, v viewer, l listing, f filter, start, count int) (catalogPage, answer, error) {
	j, a, err := s.judgeOf(r, v, f)
	if err != nil || !a.ok() {
		return catalogPage{}, a, err
	}
	sc, a, err := s.scopeOf(r, v, l, f)
	if err != nil || !a.ok() {
		return catalogPage{}, a, err
	}
	keep := func(i item, listedAs string) bool {
		return (!l.libraryTitles() || l.keep(i)) && j.keeps(f, i, listedAs)
	}
	titles, done, a, err := s.titlesOf(r, v, l, sc, start+count, keep)
	if err != nil || !a.ok() {
		return catalogPage{}, a, err
	}
	page := catalogPage{Items: []catalogItem{}, Total: len(titles), TotalExact: done, WindowCursor: cursor(0)}
	if start < len(titles) {
		page.Items = cards(r, titles[start:min(start+count, len(titles))])
	}
	if len(titles) > start+count {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(start + count)}
	}
	return page, a, nil
}

// catalogQuery answers POST /api/v2/catalog/query, the catalog asked in a
// body, as the Android apps ask a filtered page, as the catalog's query
// would ask it.
func (s *Server) catalogQuery(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		Source       string          `json:"source"`
		Scope        string          `json:"scope"`
		SectionID    string          `json:"section_id"`
		LibraryID    string          `json:"library_id"`
		CollectionID string          `json:"collection_id"`
		PersonID     string          `json:"person_id"`
		Q            string          `json:"q"`
		NamePrefix   string          `json:"name_prefix"`
		Type         string          `json:"type"`
		Match        string          `json:"match"`
		Groups       json.RawMessage `json:"groups"`
		Sort         string          `json:"sort"`
		Order        string          `json:"order"`
		Limit        *int            `json:"limit"`
		Cursor       string          `json:"cursor"`
	}
	if !decode(w, r, &body) {
		return
	}
	// The artwork's widths come in the query.
	q := r.URL.Query()
	for key, value := range map[string]string{"source": body.Source, "scope": body.Scope, "section_id": body.SectionID,
		"library_id": body.LibraryID, "collection_id": body.CollectionID, "person_id": body.PersonID, "q": body.Q,
		"name_prefix": body.NamePrefix, "type": body.Type, "match": body.Match, "cursor": body.Cursor} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if body.Limit != nil {
		q.Set("limit", strconv.Itoa(*body.Limit))
	}
	switch body.Order {
	case "", "asc":
	case "desc":
		body.Sort = "-" + body.Sort
	default:
		validationProblem(w, "body.order", "invalid_enum", "order is asc or desc.")
		return
	}
	if strings.TrimPrefix(body.Sort, "-") != "" {
		q.Set("sort", body.Sort)
	}
	if groups := strings.TrimSpace(string(body.Groups)); groups != "" && groups != "null" {
		q.Set("groups", groups)
	}
	s.catalogOf(w, r, v, q, "body")
}

// catalogFilters are the values a filter sheet offers, the contract's
// CatalogFilters: genres and content ratings, the others empty, without
// the facets of files, which Polyfin does not know before playing.
type catalogFilters struct {
	Genres            []string `json:"genres"`
	Studios           []string `json:"studios"`
	Networks          []string `json:"networks"`
	Countries         []string `json:"countries"`
	OriginalLanguages []string `json:"original_languages"`
	ContentRatings    []string `json:"content_ratings"`
	Authors           []string `json:"authors"`
	Narrators         []string `json:"narrators"`
	Series            []string `json:"series"`
}

// catalogFilters answers the values of a scope's filter sheet, among the
// titles the user sees there, as the catalog applies them: the genres of
// a library's catalog, or of the libraries of titles for the whole
// catalog, and elsewhere those of its titles; and the content ratings of
// its titles. The sections of Home and of a library are not filtered:
// they offer none.
func (s *Server) catalogFilters(w http.ResponseWriter, r *http.Request, v viewer) {
	options := catalogFilters{Genres: []string{}, Studios: []string{}, Networks: []string{}, Countries: []string{},
		OriginalLanguages: []string{}, ContentRatings: []string{}, Authors: []string{}, Narrators: []string{}, Series: []string{}}
	q := r.URL.Query()
	if q.Get("source") == "section" {
		writeJSON(w, http.StatusOK, options)
		return
	}
	if q.Get("type") == "video_with_episodes" {
		q.Set("type", "video")
	}
	for _, key := range []string{"q", "name_prefix", "sort"} {
		q.Del(key)
	}
	l, ok := s.listingAsked(w, r, v, q)
	if !ok {
		return
	}
	sc, a, err := s.scopeOf(r, v, l, filter{})
	if err == nil && a.ok() {
		var titles []item
		if titles, _, a, err = s.titlesOf(r, v, l, sc, maxScanned, func(item, string) bool { return true }); err == nil && a.ok() {
			options.ContentRatings = distinct(titles, func(i item) []string { return []string{i.OfficialRating} })
			switch {
			case l.libraryTitles() && l.source == "query" && !sc.collections:
				options.Genres, a, err = s.catalogGenres(r, v, []item{l.view})
			case l.source == "query" && !l.whole && l.view.Id == "":
				options.Genres, a, err = s.titleLibraryGenres(r, v)
			default:
				options.Genres = distinct(titles, func(i item) []string { return i.Genres })
			}
		}
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	j := judge{blocked: v.user.BlockedGenres}
	options.Genres = slices.DeleteFunc(options.Genres, j.blockedGenre)
	writeJSON(w, http.StatusOK, options)
}

// distinct are the values of titles, each once whatever its letter case,
// sorted.
func distinct(titles []item, values func(item) []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, i := range titles {
		for _, value := range values(i) {
			value = strings.TrimSpace(value)
			if key := strings.ToLower(value); value != "" && !seen[key] {
				seen[key] = true
				result = append(result, value)
			}
		}
	}
	slices.SortFunc(result, func(a, b string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
	})
	return result
}

// catalogGenres are the genres the catalogs of libraries offer, as the
// Jellyfin API's /Items/Filters gives them, each once, in their order.
func (s *Server) catalogGenres(r *http.Request, v viewer, views []item) ([]string, answer, error) {
	seen := map[string]bool{}
	genres := []string{}
	a := answer{status: http.StatusOK}
	for _, view := range views {
		var filters struct{ Genres []string }
		var err error
		if a, err = s.call(r, v, http.MethodGet, "/Items/Filters?"+url.Values{"ParentId": {view.Id}}.Encode(), nil, &filters); err != nil || !a.ok() {
			return nil, a, err
		}
		for _, genre := range filters.Genres {
			genre = strings.TrimSpace(genre)
			if key := strings.ToLower(genre); genre != "" && !seen[key] {
				seen[key] = true
				genres = append(genres, genre)
			}
		}
	}
	return genres, a, nil
}

// titleLibraryGenres are the genres of the catalog's filter: those the
// catalogs of the user's libraries of titles offer, which the Jellyfin
// API lists the titles of across them. Those of a library of collections
// choose collections.
func (s *Server) titleLibraryGenres(r *http.Request, v viewer) ([]string, answer, error) {
	views, _, a, err := s.views(r, v)
	if err != nil || !a.ok() {
		return nil, a, err
	}
	var titled []item
	for _, view := range views {
		collections, a, err := s.collectionLibrary(r, v, view)
		if err != nil || !a.ok() {
			return nil, a, err
		}
		if !collections {
			titled = append(titled, view)
		}
	}
	return s.catalogGenres(r, v, titled)
}
