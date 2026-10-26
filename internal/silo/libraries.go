package silo

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Libraries of collections.
//
// A library of collections holds collections rather than titles: an
// addon's catalog of collections, and Polyfin's Collections view. Silo's
// apps list it as a library of movies and series together (mixed): it
// opens on the titles of its collections, each once, in its order, as the
// Jellyfin API lists them to Jellyfin apps it tells nothing of its type;
// its landing has a row of each of its first collections, its Collections
// tab lists them all (see libraryCollections), and its row on Home lists
// its first titles. The Jellyfin API describes an addon's catalog of
// collections as boxsets to the apps an administrator chose (see
// accounts.Settings.CollectionLibraryApps), with no type to the others:
// it is known then by its listing, which holds collections.

func (s *Server) libraryRoutes() {
	s.mux.HandleFunc("GET /api/v2/library/{id}/sections", s.profiled(s.librarySections))
	s.mux.HandleFunc("GET /api/v2/library/{id}/sections/{section}/items", s.profiled(s.librarySectionItems))
}

// The sections of a library's landing, besides its row of latest titles:
// what the user is watching in it, and the rows of its collections.
const (
	resumePrefix       = "resume-"
	collectionPrefix   = "collection-"
	collectionsSection = "collection"
)

// boxsets is the Jellyfin API's type of a library of collections.
const boxsets = "boxsets"

// titleTypes are the Jellyfin API's types of the titles libraries list.
const titleTypes = "Movie,Series"

// sectionTypes are the Jellyfin API's types of the cards Silo's apps show
// in a section: the Apple app leaves out the others
// (SiloMediaType.isSupportedSectionItem).
var sectionTypes = []string{"Movie", "Series", "Episode", "Video"}

// sectionCards describes items as cards of a section.
func sectionCards(r *http.Request, items []item) []catalogItem {
	return cards(r, slices.DeleteFunc(slices.Clone(items), func(i item) bool { return !slices.Contains(sectionTypes, i.Type) }))
}

// allCollections reports whether items are collections, at least one.
func allCollections(items []item) bool {
	return len(items) > 0 && !slices.ContainsFunc(items, func(i item) bool { return i.Type != "BoxSet" })
}

// libraryPage is a page of a library's titles from start, count of them,
// narrowed by query (their types, a search) and keep: those within its
// folders, as Jellyfin apps ask for a library's movies and series, which
// for a library of collections are the titles of its collections. The
// Jellyfin API lists none for Polyfin's Collections view: those of a
// library it describes as boxsets that it lists none of are gathered
// collection by collection.
func (s *Server) libraryPage(r *http.Request, v viewer, view item, query url.Values, start, count int, keep func(item) bool) (catalogPage, answer, error) {
	listed := maps.Clone(query)
	listed.Set("ParentId", view.Id)
	listed.Set("Recursive", "true")
	listed.Set("StartIndex", strconv.Itoa(start))
	listed.Set("Limit", strconv.Itoa(count))
	listed.Set("EnableTotalRecordCount", "true")
	page, a, err := s.listPage(r, v, listed, start)
	if err != nil || !a.ok() || view.CollectionType != boxsets || page.Total > 0 {
		return page, a, err
	}
	return s.gatheredPage(r, v, view, query, start, count, keep)
}

// gatheredPage is a page of the titles of a library's collections, in
// their order, each once: the collections are read in turn until the page
// is full and one more title tells that more follow, the total counting
// those read.
func (s *Server) gatheredPage(r *http.Request, v viewer, view item, query url.Values, start, count int, keep func(item) bool) (catalogPage, answer, error) {
	var collections queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"ParentId": {view.Id}}.Encode(), nil, &collections)
	if err != nil || !a.ok() {
		return catalogPage{}, a, err
	}
	seen := map[string]bool{}
	var titles []item
	for _, collection := range collections.Items {
		if collection.Type != "BoxSet" {
			continue
		}
		listed := maps.Clone(query)
		listed.Set("ParentId", collection.Id)
		var result queryResult
		// A collection the user may no longer open lists nothing.
		if _, err := s.call(r, v, http.MethodGet, "/Items?"+listed.Encode(), nil, &result); err != nil {
			return catalogPage{}, a, err
		}
		for _, title := range result.Items {
			if !seen[title.Id] && slices.Contains(browsable, title.Type) && keep(title) {
				seen[title.Id] = true
				titles = append(titles, title)
			}
		}
		if len(titles) > start+count {
			break
		}
	}
	more := len(titles) > start+count
	page := catalogPage{Items: []catalogItem{}, Total: len(titles), TotalExact: !more, WindowCursor: cursor(0)}
	if start < len(titles) {
		page.Items = cards(r, titles[start:min(start+count, len(titles))])
	}
	if more {
		page.Page = pageInfo{HasMore: true, NextCursor: cursor(start + count)}
	}
	return page, a, nil
}

// keepAll keeps every title.
func keepAll(item) bool { return true }

// libraryRow is a library's row on Home and on its landing: its latest
// titles, as Jellyfin apps show them, titled after it; for a library of
// collections, whose latest are collections, which Silo's apps do not show
// in a row, the first of its titles, titled with its name.
func (s *Server) libraryRow(r *http.Request, v viewer, view item, number string) (section, answer, error) {
	sec := section{ID: latestPrefix + number, Type: "recently_added", Title: fmt.Sprintf(s.titles()[2], view.Name), ItemLimit: sectionLimit,
		Items: []catalogItem{}}
	if view.CollectionType != boxsets {
		var items []item
		a, err := s.call(r, v, http.MethodGet, "/Items/Latest?"+url.Values{"ParentId": {view.Id}, "Limit": {strconv.Itoa(sectionLimit)},
			"Fields": {listFields}}.Encode(), nil, &items)
		if err != nil || !a.ok() || !allCollections(items) {
			sec.Items = sectionCards(r, items)
			sec.TotalCount = len(sec.Items)
			return sec, a, err
		}
	}
	page, a, err := s.libraryPage(r, v, view, url.Values{"IncludeItemTypes": {titleTypes}, "Fields": {listFields}}, 0, sectionLimit, keepAll)
	sec.Type, sec.Title, sec.Items, sec.TotalCount = collectionsSection, view.Name, page.Items, max(page.Total, len(page.Items))
	return sec, a, err
}

// maxCollectionRows bounds the rows of collections a library's landing
// shows; its Collections tab lists them all.
const maxCollectionRows = 24

// collectionReads bounds the collections read at once.
const collectionReads = 4

// librarySections answers a library's landing: for a library of
// collections, a row of each of its first collections, with its first
// titles; for the others, what the user is watching in it, and its
// latest titles.
func (s *Server) librarySections(w http.ResponseWriter, r *http.Request, v viewer) {
	number := r.PathValue("id")
	view, ok := s.library(w, r, v, number)
	if !ok {
		return
	}
	var children queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"ParentId": {view.Id}, "Limit": {strconv.Itoa(maxCollectionRows)},
		"Fields": {listFields}}.Encode(), nil, &children)
	var sections []section
	switch {
	case err != nil || !a.ok():
	case view.CollectionType == boxsets || allCollections(children.Items):
		sections, err = s.collectionRows(r, v, children.Items)
	default:
		sections, a, err = s.titleRows(r, v, view, number)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Sections []section `json:"sections"`
	}{sections})
}

// titleRows are the rows of a library of titles' landing: what the user
// is watching in it, then its latest titles, those with titles.
func (s *Server) titleRows(r *http.Request, v viewer, view item, number string) ([]section, answer, error) {
	resume, a, err := s.sectionOf(r, v, resumePrefix+number, s.titles()[0], view.Id)
	if err != nil || !a.ok() {
		return nil, a, err
	}
	latest, a, err := s.libraryRow(r, v, view, number)
	if err != nil || !a.ok() {
		return nil, a, err
	}
	sections := []section{}
	for _, sec := range []section{resume, latest} {
		if len(sec.Items) > 0 {
			sections = append(sections, sec)
		}
	}
	return sections, a, nil
}

// collectionRows are the rows of collections, in their order, each with
// its first titles; those without any are left out, with those the user
// may no longer open.
func (s *Server) collectionRows(r *http.Request, v viewer, collections []item) ([]section, error) {
	collections = slices.DeleteFunc(slices.Clone(collections), func(i item) bool { return i.Type != "BoxSet" })
	rows := make([]section, len(collections))
	var group errgroup.Group
	group.SetLimit(collectionReads)
	for i, collection := range collections {
		group.Go(func() error {
			row, a, err := s.collectionRow(r, v, collection)
			if a.ok() {
				rows[i] = row
			}
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	sections := []section{}
	for _, row := range rows {
		if len(row.Items) > 0 {
			sections = append(sections, row)
		}
	}
	return sections, nil
}

// collectionRow is the row of a collection: its first titles.
func (s *Server) collectionRow(r *http.Request, v viewer, collection item) (section, answer, error) {
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"ParentId": {collection.Id}, "IncludeItemTypes": {titleTypes},
		"Limit": {strconv.Itoa(sectionLimit)}, "Fields": {listFields}, "EnableTotalRecordCount": {"true"}}.Encode(), nil, &result)
	sec := section{ID: collectionPrefix + collection.Id, Title: collection.Name, Type: collectionsSection, ItemLimit: sectionLimit,
		Items: sectionCards(r, result.Items)}
	sec.TotalCount = max(result.TotalRecordCount, len(sec.Items))
	return sec, a, err
}

// librarySectionItems answers one section of a library's landing.
func (s *Server) librarySectionItems(w http.ResponseWriter, r *http.Request, v viewer) {
	number, id := r.PathValue("id"), r.PathValue("section")
	if _, ok := s.library(w, r, v, number); !ok {
		return
	}
	// The sections of the library's own titles name it.
	for _, prefix := range []string{resumePrefix, latestPrefix} {
		if rest, ok := strings.CutPrefix(id, prefix); ok && rest != number {
			problem(w, http.StatusNotFound, "not_found", "Section not found.")
			return
		}
	}
	if sec, ok := s.namedSection(w, r, v, id); ok {
		writeJSON(w, http.StatusOK, sec)
	}
}
