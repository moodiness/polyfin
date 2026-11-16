package silo

import (
	"cmp"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/library"
)

// Title order, letters and seek.
//
// Silo's apps sort a library by title, show an alphabetical bar of the
// letters its titles file under, and jump to a letter: POST
// /api/v2/catalog/letters gives each letter's count and the position of
// its first title, and the catalog's seek reads the list from a position
// (Apple BrowseViewModel.loadLetterIndex, jump and its CatalogPositionPager,
// Screens/Browse/BrowseViewModel.swift; Android CatalogLetterJump,
// androidApp ui/screens/browse/CatalogLetterJump.kt). Both ask for letters
// and send seek only while the list is in title order, and read the list's
// length from the letters, so the letters, the pages and seek must follow
// one order.
//
// A library whose titles Polyfin stores, an IPTV source's movies or
// series or a local folder, is in title order the Jellyfin API's order of
// the library by name (GET /Polyfin/NameOrder, see nameorder.go in
// package jellyfin): every title, kept 2 minutes. Filtered, it is read
// from that order as a filtered page is read, at most maxScanned titles,
// and sorted here by the same rule (see library.NameKey). Any other list,
// an addon's catalog and a library of collections among them, keeps its
// order in title order, as on Jellyfin apps; its letters are refused as
// not supported, which both apps take as no index, and seek is a
// position in its order, as in the other sorts.

// maxSeek is the largest seek the contract takes.
const maxSeek = 10_000_000

// seekAsked reads the seek of a catalog request q, sent in the request's
// query or body (in): the position its page starts at, in place of its
// cursor. Android sends the first page's window_cursor beside it, as the
// cursor, which only names the list here.
func seekAsked(w http.ResponseWriter, q url.Values, in string) (start int, seeking, ok bool) {
	text := q.Get("seek")
	if text == "" {
		return 0, false, true
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 0 || value > maxSeek {
		validationProblem(w, in+".seek", "out_of_range", "seek is a position from 0 to 10000000.")
		return 0, false, false
	}
	return value, true, true
}

// titleOrdered reports whether a catalog request q asks for its titles in
// title order.
func titleOrdered(q url.Values) bool {
	return strings.TrimPrefix(q.Get("sort"), "-") == "title"
}

// byName reports whether l lists a library's titles that the Jellyfin
// API's order by name pages through as they are: neither a search nor
// favorites.
func (l listing) byName() bool {
	return l.libraryTitles() && l.source == "query" && l.term == ""
}

// nameOrderQuery is the query of a library's order by name, for the
// listing l asks in title order, from l's query.
func nameOrderQuery(l listing, query url.Values) url.Values {
	listed := maps.Clone(query)
	listed.Set("ParentId", l.view.Id)
	listed.Set("Recursive", "true")
	listed.Set("SortOrder", cmp.Or(l.query.Get("SortOrder"), "Ascending"))
	listed.Del("SortBy")
	return listed
}

// notByName is the answer of a library's order by name when Polyfin does
// not store the library's titles.
const notByName = http.StatusNotImplemented

// storedByName reports whether l is a library whose titles Polyfin stores
// that its order by name pages through: neither a search nor favorites.
func (s *Server) storedByName(r *http.Request, v viewer, l listing) (bool, answer, error) {
	if !l.byName() {
		return false, answer{status: http.StatusOK}, nil
	}
	_, a, err := s.namePage(r, v, l, 0, 0)
	if a.status == notByName {
		return false, answer{status: http.StatusOK}, err
	}
	return err == nil && a.ok(), a, err
}

// titlePage is a page from start of what l lists that f keeps, in title
// order, descending when asked, when l is a library whose titles Polyfin
// stores; handled is false for another list, which keeps its order.
func (s *Server) titlePage(r *http.Request, v viewer, l listing, f filter, descending bool, start, count int) (page catalogPage, handled bool,
	a answer, err error) {
	if !f.narrows() && l.byName() {
		page, a, err = s.namePage(r, v, l, start, count)
		if a.status == notByName {
			return catalogPage{}, false, answer{status: http.StatusOK}, nil
		}
		return page, true, a, err
	}
	stored, a, err := s.storedByName(r, v, l)
	if err != nil || !a.ok() || !stored {
		return catalogPage{}, err != nil || !a.ok(), a, err
	}
	titles, done, a, err := s.sortedTitles(r, v, l, f, descending)
	if err != nil || !a.ok() {
		return catalogPage{}, true, a, err
	}
	return sortedPage(r, titles, done, start, count), true, a, nil
}

// namePage is a page from start of a library's titles in title order.
func (s *Server) namePage(r *http.Request, v viewer, l listing, start, count int) (catalogPage, answer, error) {
	listed := nameOrderQuery(l, l.query)
	listed.Set("StartIndex", strconv.Itoa(start))
	listed.Set("Limit", strconv.Itoa(count))
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Polyfin/NameOrder?"+listed.Encode(), nil, &result)
	if err != nil || !a.ok() {
		return catalogPage{}, a, err
	}
	page := catalogPage{Items: cards(r, result.Items), Total: result.TotalRecordCount, TotalExact: true, WindowCursor: cursor(0)}
	if next := start + len(result.Items); len(result.Items) > 0 && next < result.TotalRecordCount {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(next)}
	}
	return page, a, nil
}

// sortedTitles are the titles l lists that f keeps, in title order, read
// as a filtered page reads them (see filteredPage), at most maxScanned:
// a library's from its order by name, then sorted by name. done reports
// whether every title was read.
func (s *Server) sortedTitles(r *http.Request, v viewer, l listing, f filter, descending bool) ([]item, bool, answer, error) {
	j, a, err := s.judgeOf(r, v, f)
	if err != nil || !a.ok() {
		return nil, false, a, err
	}
	sc, a, err := s.scopeOf(r, v, l, f)
	if err != nil || !a.ok() {
		return nil, false, a, err
	}
	for n, st := range sc.streams {
		// What the user did is narrowed by /Items alone.
		if l.libraryTitles() && st.query.Get("Filters") == "" {
			st.path, st.query = "/Polyfin/NameOrder", nameOrderQuery(l, st.query)
		}
		sc.streams[n] = st
	}
	keep := func(i item, listedAs string) bool {
		return (!l.libraryTitles() || l.keep(i)) && j.keeps(f, i, listedAs)
	}
	titles, done, a, err := s.titlesOf(r, v, l, sc, maxScanned, keep)
	if err != nil || !a.ok() {
		return nil, false, a, err
	}
	sortTitles(titles, descending)
	return titles, done, a, nil
}

// nameKey is what a title sorts by in title order (see library.NameKey).
func nameKey(i item) string {
	return library.FoldName(cmp.Or(i.SortName, i.Name))
}

// sortTitles sorts titles by name, then by ID, descending when asked.
func sortTitles(titles []item, descending bool) {
	slices.SortStableFunc(titles, func(a, b item) int {
		result := cmp.Or(strings.Compare(nameKey(a), nameKey(b)), strings.Compare(a.Id, b.Id))
		if descending {
			return -result
		}
		return result
	})
}

// sortedPage is a page from start of titles in title order.
func sortedPage(r *http.Request, titles []item, done bool, start, count int) catalogPage {
	page := catalogPage{Items: []catalogItem{}, Total: len(titles), TotalExact: done, WindowCursor: cursor(0)}
	if start < len(titles) {
		page.Items = cards(r, titles[start:min(start+count, len(titles))])
	}
	if start+count < len(titles) {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(start + count)}
	}
	return page
}

// catalogLetters is the contract's CatalogLetters.
type catalogLetters struct {
	Total   int             `json:"total"`
	Letters []catalogLetter `json:"letters"`
	Runs    []letterRun     `json:"runs"`
}

type catalogLetter struct {
	Letter   string `json:"letter"`
	Count    int    `json:"count"`
	Position int    `json:"position"`
}

type letterRun struct {
	Letter   string `json:"letter"`
	Position int    `json:"position"`
}

func lettersOf(total int, letters []library.Letter, runs []library.Run) catalogLetters {
	answer := catalogLetters{Total: total, Letters: make([]catalogLetter, len(letters)), Runs: make([]letterRun, len(runs))}
	for n, l := range letters {
		answer.Letters[n] = catalogLetter{l.Letter, l.Count, l.Position}
	}
	for n, run := range runs {
		answer.Runs[n] = letterRun{run.Letter, run.Position}
	}
	return answer
}

// letters answers POST /api/v2/catalog/letters: the letters of the list
// the catalog pages through in title order for the same library, type and
// filters, in the order asked (see the top of this file).
func (s *Server) letters(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		LibraryID string          `json:"library_id"`
		Type      string          `json:"type"`
		Match     string          `json:"match"`
		Groups    json.RawMessage `json:"groups"`
		Order     string          `json:"order"`
	}
	if !decode(w, r, &body) {
		return
	}
	q := url.Values{"source": {"query"}, "sort": {"title"}}
	switch body.Order {
	case "", "asc":
	case "desc":
		q.Set("sort", "-title")
	default:
		validationProblem(w, "body.order", "invalid_enum", "order is asc or desc.")
		return
	}
	for key, value := range map[string]string{"library_id": body.LibraryID, "type": body.Type, "match": body.Match} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if groups := strings.TrimSpace(string(body.Groups)); groups != "" && groups != "null" {
		q.Set("groups", groups)
	}
	f, ok := filterAsked(w, q, "body")
	if !ok {
		return
	}
	l, ok := s.listingAsked(w, r, v, q)
	if !ok {
		return
	}
	if l.none {
		writeJSON(w, http.StatusOK, lettersOf(0, nil, nil))
		return
	}
	refuse := func() {
		problem(w, http.StatusNotImplemented, "capability_unsupported",
			"This server gives the letters of a library whose titles it stores, an IPTV source's or a local folder's.")
	}
	if !f.narrows() && l.byName() {
		listed := nameOrderQuery(l, l.query)
		var result struct {
			Total   int
			Letters []library.Letter
			Runs    []library.Run
		}
		a, err := s.call(r, v, http.MethodGet, "/Polyfin/NameOrder/Letters?"+listed.Encode(), nil, &result)
		switch {
		case err != nil:
			s.internalError(w, r, err)
		case a.status == notByName:
			refuse()
		case !a.ok():
			s.jellyfinRefusal(w, r, a)
		default:
			writeJSON(w, http.StatusOK, lettersOf(result.Total, result.Letters, result.Runs))
		}
		return
	}
	stored, a, err := s.storedByName(r, v, l)
	if err != nil || !a.ok() {
		s.writePage(w, r, catalogPage{}, a, err)
		return
	}
	if !stored {
		refuse()
		return
	}
	titles, _, a, err := s.sortedTitles(r, v, l, f, strings.HasPrefix(q.Get("sort"), "-"))
	switch {
	case err != nil:
		s.internalError(w, r, err)
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
	default:
		keys := make([]string, len(titles))
		for n, title := range titles {
			keys[n] = nameKey(title)
		}
		letters, runs := library.Letters(keys)
		writeJSON(w, http.StatusOK, lettersOf(len(titles), letters, runs))
	}
}
