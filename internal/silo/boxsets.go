package silo

import (
	"cmp"
	"context"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/boxset"
)

// Box sets.
//
// A Silo box set is a TMDB collection: the movies of a library that TMDB
// lists in one collection, when the user sees at least minBoxSetMovies of
// them there. Polyfin knows the collection of a movie once TMDB told it
// (see the boxset package), which it learns ahead for the libraries'
// first movies, and asks first for the movies a page or a search reads,
// while the server's TMDB key is saved: box sets are on then, in the
// libraries of movies, and the search capabilities say so (box_sets).
// They show on a movie's page, in a search, and open on their movies
// (getCatalogBoxSet), with their artwork relayed by Polyfin; a library's
// grid lists each movie, as Silo's does since it stopped grouping them
// (group=box_set, which the apps at the commits followed still send, is
// answered as without it). Without the key, the apps are told none.
//
// A library's box sets are made of its first maxScanned movies, as the
// Jellyfin API lists them to the user, under the user's limits; the
// passes learn as many (boxset.PassMost).

// minBoxSetMovies is how many of a collection's movies a library shows
// the user for it to be a box set there.
const minBoxSetMovies = 2

// maxBoxSetSearch bounds the box sets a search answers, and
// defaultBoxSetSearch is how many it answers by default, as the contract's.
const (
	maxBoxSetSearch     = 20
	defaultBoxSetSearch = 5
	maxBoxSetSearchText = 200
)

func (s *Server) boxSetRoutes() {
	s.mux.HandleFunc("GET /api/v2/catalog/box-sets", s.profiled(s.searchBoxSets))
	s.mux.HandleFunc("GET /api/v2/catalog/box-sets/{id}", s.profiled(s.boxSet))
	s.mux.HandleFunc("GET /api/v2/catalog/items/{id}/box-set", s.profiled(s.itemBoxSet))
	// Apps load artwork without credentials.
	s.mux.HandleFunc("GET /api/v2/artwork/box-sets/{id}/{kind}", s.boxSetArtwork)
}

// movieLookupWait bounds how long a movie's page waits for the lookup of
// its collection.
const movieLookupWait = time.Second

// boxSetArtworkURL is the address of a collection's poster ("Primary") or
// backdrop ("Backdrop") on Polyfin, tagged with its TMDB path; empty
// without one.
func boxSetArtworkURL(id int64, kind, path string, width int) string {
	if path == "" {
		return ""
	}
	query := url.Values{"tag": {strings.TrimPrefix(path, "/")}}
	if width > 0 {
		query.Set("width", strconv.Itoa(width))
	}
	return "/api/v2/artwork/box-sets/" + strconv.FormatInt(id, 10) + "/" + kind + "?" + query.Encode()
}

// boxSetArtwork serves a collection's artwork as the Jellyfin API serves
// it (GET /Polyfin/BoxSets/{collectionId}/Images/{imageType}): downloaded
// from TMDB once, kept and resized by Polyfin, as titles' artwork is (see
// artwork), so that the apps never reach TMDB.
func (s *Server) boxSetArtwork(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 || kind != "Primary" && kind != "Backdrop" {
		problem(w, http.StatusNotFound, "not_found", "No such artwork.")
		return
	}
	query := url.Values{}
	if width, err := strconv.Atoi(r.URL.Query().Get("width")); err == nil && width > 0 && width <= maxArtworkWidth {
		query.Set("maxWidth", strconv.Itoa(width))
	}
	s.relay(w, r, "/Polyfin/BoxSets/"+strconv.FormatInt(id, 10)+"/Images/"+kind+"?"+query.Encode(), "")
}

// boxSetsOn reports whether box sets are known: while the server's TMDB
// key is saved.
func (s *Server) boxSetsOn() bool { return s.options.BoxSets.Enabled() }

// boxSetLibrary reports whether a library groups its movies into box
// sets: a library of movies, while box sets are on.
func (s *Server) boxSetLibrary(view item) bool {
	return view.CollectionType == "movies" && s.boxSetsOn()
}

// boxSet is a box set of a library: its collection, the library's number,
// and the movies the user sees of it there, in release order.
type boxSet struct {
	collection boxset.Collection
	library    string
	members    []item
}

// libraryBoxSets are the box sets of a library, by their movies.
type libraryBoxSets map[string]*boxSet

// boxSetsOf groups titles, the first of a library's, into the library's
// box sets, as far as their collections are known (see boxset.Store.Of).
func (s *Server) boxSetsOf(ctx context.Context, titles []item, library string) (libraryBoxSets, error) {
	var movies []item
	var keys [][]boxset.Key
	seen := map[string]bool{}
	for _, t := range titles {
		if t.Type == "Movie" && !seen[t.Id] {
			seen[t.Id] = true
			movies, keys = append(movies, t), append(keys, boxset.Keys(t.ProviderIds))
		}
	}
	collections, err := s.options.BoxSets.Of(ctx, keys)
	if err != nil {
		return nil, err
	}
	sets := map[int64]*boxSet{}
	for n, c := range collections {
		if c == nil {
			continue
		}
		set := sets[c.ID]
		if set == nil {
			set = &boxSet{collection: *c, library: library}
			sets[c.ID] = set
		}
		set.members = append(set.members, movies[n])
	}
	grouped := libraryBoxSets{}
	for _, set := range sets {
		if len(set.members) < minBoxSetMovies {
			continue
		}
		slices.SortStableFunc(set.members, releaseOrder)
		for _, m := range set.members {
			grouped[m.Id] = set
		}
	}
	return grouped, nil
}

// releaseOrder orders movies by their release: their premiere date, else
// their year, then their name.
func releaseOrder(a, b item) int {
	day := func(i item) time.Time {
		switch {
		case i.PremiereDate != nil:
			return *i.PremiereDate
		case i.ProductionYear != nil:
			return time.Date(*i.ProductionYear, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return cmp.Or(day(a).Compare(day(b)), strings.Compare(a.Name, b.Name), strings.Compare(a.Id, b.Id))
}

// sets are the box sets, each once, by their collection's identifier.
func (b libraryBoxSets) sets() []*boxSet {
	var sets []*boxSet
	for _, set := range b {
		if !slices.Contains(sets, set) {
			sets = append(sets, set)
		}
	}
	slices.SortFunc(sets, func(x, y *boxSet) int { return cmp.Compare(x.collection.ID, y.collection.ID) })
	return sets
}

// boxSetCard is a box set as the apps show it, the contract's
// CatalogBoxSet.
type boxSetCard struct {
	ID          string `json:"id"`
	LibraryID   string `json:"library_id"`
	Title       string `json:"title"`
	PosterURL   string `json:"poster_url,omitempty"`
	BackdropURL string `json:"backdrop_url,omitempty"`
	MovieCount  int    `json:"movie_count"`
	PlayedCount int    `json:"played_count"`
	FirstYear   int    `json:"first_year,omitempty"`
	LastYear    int    `json:"last_year,omitempty"`
}

// boxSetTitle names a box set after its TMDB collection, whose names end
// in "Collection": that word becomes "Box Set", as the contract says.
func boxSetTitle(name string) string {
	name = strings.TrimSpace(name)
	const suffix = " collection"
	if len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix) {
		return name[:len(name)-len(suffix)] + " Box Set"
	}
	return name
}

// cardOf describes a box set. Its poster and backdrop are its
// collection's, which Polyfin relays from TMDB as it relays titles'
// artwork (see boxSetArtwork), else those of its first movie.
func (s *Server) cardOf(r *http.Request, set *boxSet) boxSetCard {
	c := boxSetCard{ID: strconv.FormatInt(set.collection.ID, 10), LibraryID: set.library, Title: boxSetTitle(set.collection.Name),
		MovieCount:  len(set.members),
		PosterURL:   boxSetArtworkURL(set.collection.ID, "Primary", set.collection.PosterPath, artworkWidth(r, "poster")),
		BackdropURL: boxSetArtworkURL(set.collection.ID, "Backdrop", set.collection.BackdropPath, artworkWidth(r, "backdrop"))}
	for _, m := range set.members {
		if m.UserData != nil && m.UserData.Played {
			c.PlayedCount++
		}
		if m.ProductionYear != nil && *m.ProductionYear > 0 {
			if c.FirstYear == 0 || *m.ProductionYear < c.FirstYear {
				c.FirstYear = *m.ProductionYear
			}
			c.LastYear = max(c.LastYear, *m.ProductionYear)
		}
	}
	if len(set.members) > 0 && (c.PosterURL == "" || c.BackdropURL == "") {
		poster, backdrop, _ := artworkOf(r, set.members[0])
		c.PosterURL, c.BackdropURL = cmp.Or(c.PosterURL, poster), cmp.Or(c.BackdropURL, backdrop)
	}
	return c
}

// window reads the first maxScanned titles of the Jellyfin API's listing
// query, a chunk at a time.
func (s *Server) window(r *http.Request, v viewer, query url.Values) (titles []item, a answer, err error) {
	a = answer{status: http.StatusOK}
	for start := 0; start < maxScanned; {
		listed := maps.Clone(query)
		listed.Set("StartIndex", strconv.Itoa(start))
		listed.Set("Limit", strconv.Itoa(min(scanChunk, maxScanned-start)))
		listed.Set("EnableTotalRecordCount", "true")
		var result queryResult
		if a, err = s.call(r, v, http.MethodGet, "/Items?"+listed.Encode(), nil, &result); err != nil || !a.ok() {
			return nil, a, err
		}
		titles = append(titles, result.Items...)
		start += len(result.Items)
		if len(result.Items) == 0 || start >= result.TotalRecordCount {
			break
		}
	}
	return titles, a, nil
}

// moviesQuery lists a library's movies.
func moviesQuery(view item) url.Values {
	return url.Values{"ParentId": {view.Id}, "Recursive": {"true"}, "IncludeItemTypes": {"Movie"}, "Fields": {listFields}}
}

// libraryBoxSetsOf reads the box sets of a library.
func (s *Server) libraryBoxSetsOf(r *http.Request, v viewer, view item, library string) (libraryBoxSets, answer, error) {
	titles, a, err := s.window(r, v, moviesQuery(view))
	if err != nil || !a.ok() {
		return nil, a, err
	}
	sets, err := s.boxSetsOf(r.Context(), titles, library)
	return sets, a, err
}

// boxSetLibraries are the user's libraries of box sets, with their
// numbers: the one numbered library when not empty, else all of them, in
// the user's order.
func (s *Server) boxSetLibraries(r *http.Request, v viewer, library string) ([]item, []string, answer, error) {
	views, numbers, a, err := s.views(r, v)
	if err != nil || !a.ok() {
		return nil, nil, a, err
	}
	var kept []item
	var keptNumbers []string
	for _, view := range views {
		id, _ := accounts.ParseID(view.Id)
		number := strconv.Itoa(int(numbers[id]))
		if s.boxSetLibrary(view) && (library == "" || library == number) {
			kept, keptNumbers = append(kept, view), append(keptNumbers, number)
		}
	}
	return kept, keptNumbers, a, nil
}

// noBoxSets answers a box set read while box sets are off, as a server
// without them does.
func noBoxSets(w http.ResponseWriter) {
	problem(w, http.StatusNotFound, "not_found", "This server has no box sets.")
}

// boxSetDetail is a box set with its movies in release order, the
// contract's CatalogBoxSetDetail.
type boxSetDetail struct {
	boxSetCard
	Items []catalogItem `json:"items"`
}

func (s *Server) detailOf(r *http.Request, set *boxSet) boxSetDetail {
	return boxSetDetail{boxSetCard: s.cardOf(r, set), Items: cards(r, set.members)}
}

// boxSet answers one box set of a library with its movies the user sees
// there (getCatalogBoxSet); 404 when the library has box sets off or the
// user sees too few of its movies.
func (s *Server) boxSet(w http.ResponseWriter, r *http.Request, v viewer) {
	if !s.boxSetsOn() {
		noBoxSets(w)
		return
	}
	library := r.URL.Query().Get("library_id")
	if library == "" {
		validationProblem(w, "query.library_id", "required", "library_id is required.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, http.StatusNotFound, "not_found", "Box set not found.")
		return
	}
	views, numbers, a, err := s.boxSetLibraries(r, v, library)
	if err != nil || !a.ok() {
		s.writePage(w, r, catalogPage{}, a, err)
		return
	}
	for n, view := range views {
		sets, a, err := s.libraryBoxSetsOf(r, v, view, numbers[n])
		if err != nil || !a.ok() {
			s.writePage(w, r, catalogPage{}, a, err)
			return
		}
		for _, set := range sets.sets() {
			if set.collection.ID == id {
				writeJSON(w, http.StatusOK, s.detailOf(r, set))
				return
			}
		}
	}
	problem(w, http.StatusNotFound, "not_found", "Box set not found.")
}

// itemBoxSet answers the box set a movie's page shows
// (getCatalogItemBoxSet): the movie's, in the library asked, else the
// first of the user's that shows it, with its movies; none for a title in
// no box set the user sees. 404 for a title the user may not see, and for
// a season or an episode, which are not catalog titles. The collection of
// a movie not known yet is looked up then.
func (s *Server) itemBoxSet(w http.ResponseWriter, r *http.Request, v viewer) {
	if !s.boxSetsOn() {
		noBoxSets(w)
		return
	}
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	if i.Type == "Season" || i.Type == "Episode" {
		problem(w, http.StatusNotFound, "not_found", "Item not found.")
		return
	}
	var answer struct {
		BoxSet *boxSetDetail `json:"box_set,omitempty"`
	}
	if i.Type != "Movie" {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	// A movie's first page waits a moment for its own lookup, ahead of
	// the passes, so that it may show its box set already.
	movie := [][]boxset.Key{boxset.Keys(i.ProviderIds)}
	collections, err := s.options.BoxSets.Of(r.Context(), movie)
	if err == nil && collections[0] == nil {
		s.options.BoxSets.Wait(r.Context(), movie, movieLookupWait)
		collections, err = s.options.BoxSets.Of(r.Context(), movie)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if collections[0] == nil {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	views, numbers, a, err := s.boxSetLibraries(r, v, r.URL.Query().Get("library_id"))
	if err != nil || !a.ok() {
		s.writePage(w, r, catalogPage{}, a, err)
		return
	}
	for n, view := range views {
		sets, a, err := s.libraryBoxSetsOf(r, v, view, numbers[n])
		if err != nil || !a.ok() {
			s.writePage(w, r, catalogPage{}, a, err)
			return
		}
		if set := sets[i.Id]; set != nil {
			detail := s.detailOf(r, set)
			answer.BoxSet = &detail
			break
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// boxSetSuffixWords are the words of a search a collection's name ending
// in "Collection" answers, shown as "Box Set": they count beside another
// word of the name, so that "box" alone finds no box set.
var boxSetSuffixWords = []string{"box", "set", "collection"}

// boxSetMatch tells whether a box set matches the words of a search: by
// its name, holding each, or by its movies, two or more of whose titles
// hold each. stem is its name without "Collection", lowercased.
func boxSetMatch(set *boxSet, words []string) (matched, byName bool, stem string) {
	stem = strings.ToLower(strings.TrimSpace(set.collection.Name))
	suffixed := false
	if trimmed, ok := strings.CutSuffix(stem, " collection"); ok && strings.TrimSpace(trimmed) != "" {
		stem, suffixed = strings.TrimSpace(trimmed), true
	}
	byName = slices.ContainsFunc(words, func(word string) bool { return !slices.Contains(boxSetSuffixWords, word) })
	for _, word := range words {
		if !strings.Contains(stem, word) && !(suffixed && slices.Contains(boxSetSuffixWords, word)) {
			byName = false
		}
	}
	if byName {
		return true, true, stem
	}
	titles := 0
	for _, m := range set.members {
		name := strings.ToLower(m.Name)
		if !slices.ContainsFunc(words, func(word string) bool { return !strings.Contains(name, word) }) {
			titles++
		}
	}
	return titles >= minBoxSetMovies, false, stem
}

// searchBoxSets answers the box sets of the user's libraries, or of the
// one asked, that match a search (searchCatalogBoxSets): those whose name
// holds each of its words, the shortest names first, then those two or
// more of whose movies' titles do.
func (s *Server) searchBoxSets(w http.ResponseWriter, r *http.Request, v viewer) {
	if !s.boxSetsOn() {
		noBoxSets(w)
		return
	}
	query := r.URL.Query()
	text := strings.TrimSpace(query.Get("q"))
	if text == "" || utf8.RuneCountInString(text) > maxBoxSetSearchText {
		validationProblem(w, "query.q", "out_of_range", "q is 1 to 200 characters.")
		return
	}
	limit := defaultBoxSetSearch
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxBoxSetSearch {
			validationProblem(w, "query.limit", "out_of_range", "limit is between 1 and 20.")
			return
		}
		limit = value
	}
	views, numbers, a, err := s.boxSetLibraries(r, v, query.Get("library_id"))
	if err != nil || !a.ok() {
		s.writePage(w, r, catalogPage{}, a, err)
		return
	}
	words := strings.Fields(strings.ToLower(text))
	type found struct {
		set    *boxSet
		byName bool
		stem   string
	}
	var matches []found
	for n, view := range views {
		sets, a, err := s.libraryBoxSetsOf(r, v, view, numbers[n])
		if err != nil || !a.ok() {
			s.writePage(w, r, catalogPage{}, a, err)
			return
		}
		for _, set := range sets.sets() {
			if matched, byName, stem := boxSetMatch(set, words); matched {
				matches = append(matches, found{set, byName, stem})
			}
		}
	}
	slices.SortStableFunc(matches, func(x, y found) int {
		switch {
		case x.byName && !y.byName:
			return -1
		case !x.byName && y.byName:
			return 1
		case x.byName:
			return cmp.Compare(utf8.RuneCountInString(x.stem), utf8.RuneCountInString(y.stem))
		}
		return 0
	})
	result := struct {
		Items []boxSetCard `json:"items"`
	}{Items: []boxSetCard{}}
	for _, m := range matches[:min(len(matches), limit)] {
		result.Items = append(result.Items, s.cardOf(r, m.set))
	}
	writeJSON(w, http.StatusOK, result)
}
