package silo

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Collections, read-only.
//
// Polyfin's collections are the Jellyfin API's: those users make, which
// every user sees, each counting and listing only the titles they may see.
// Silo's apps list them on a library's Collections tab, those with a
// title in the library, as the Jellyfin API lists a library's
// collections, and page through a collection's titles. Home shows them
// through the home rows (see homeSections). The apps' personal
// collections, of which Polyfin keeps none, list empty, and the
// operations that make or change one answer that this server does not.

func (s *Server) collectionRoutes() {
	s.mux.HandleFunc("GET /api/v2/library/{id}/collections", s.profiled(s.libraryCollections))
	s.mux.HandleFunc("GET /api/v2/library/{id}/collections/{collection}/items", s.profiled(s.libraryCollectionItems))
	s.mux.HandleFunc("GET /api/v2/collections", s.profiled(s.personalCollections))
	s.mux.HandleFunc("GET /api/v2/collections/capabilities", s.profiled(s.collectionCapabilities))
	// The operations whose refusal the contract gives as not implemented.
	for _, pattern := range []string{"POST /api/v2/collections", "PATCH /api/v2/collections/{id}", "POST /api/v2/collections/groups",
		"PATCH /api/v2/collections/groups/{id}", "DELETE /api/v2/collections/groups/{id}", "PUT /api/v2/collections/groups/order"} {
		s.mux.HandleFunc(pattern, s.profiled(func(w http.ResponseWriter, _ *http.Request, _ viewer) {
			problem(w, http.StatusNotImplemented, "capability_unsupported", "This server's collections are read-only in Silo's apps.")
		}))
	}
}

// curatedCollection is a collection on a library's Collections tab.
type curatedCollection struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Slug             string   `json:"slug"`
	Description      string   `json:"description"`
	CollectionType   string   `json:"collection_type"`
	LibraryID        string   `json:"library_id"`
	LibraryIDs       []string `json:"library_ids"`
	ItemCount        int      `json:"item_count"`
	SortOrder        int      `json:"sort_order"`
	Featured         bool     `json:"featured"`
	GroupID          *string  `json:"group_id"`
	PosterURL        string   `json:"poster_url"`
	PosterIsCollage  bool     `json:"poster_is_collage"`
	BackdropURL      string   `json:"backdrop_url"`
	Visibility       string   `json:"visibility"`
	ManagementKey    string   `json:"management_key"`
	ManagementMode   string   `json:"management_mode"`
	ManagementSource string   `json:"management_source"`
	SourceURL        string   `json:"source_url"`
	LastSyncStatus   string   `json:"last_sync_status"`
	LastSyncMessage  string   `json:"last_sync_message"`
	QueryDefinition  any      `json:"query_definition"`
	SortConfig       any      `json:"sort_config"`
	SourceConfig     any      `json:"source_config"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

// libraryCollections is a library's Collections tab: the collections with
// a title in it, by name, without groups.
func (s *Server) libraryCollections(w http.ResponseWriter, r *http.Request, v viewer) {
	number := r.PathValue("id")
	view, ok := s.library(w, r, v, number)
	if !ok {
		return
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"ParentId": {view.Id}, "IncludeItemTypes": {"BoxSet"},
		"SortBy": {"SortName"}, "Fields": {listFields}}.Encode(), nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	tab := struct {
		LibraryID   string              `json:"library_id"`
		Collections []curatedCollection `json:"collections"`
		Groups      []struct{}          `json:"groups"`
	}{number, []curatedCollection{}, []struct{}{}}
	for _, i := range result.Items {
		if i.Type != "BoxSet" {
			continue
		}
		poster, backdrop, _ := artworkOf(r, i)
		c := curatedCollection{ID: i.Id, Title: i.Name, Slug: i.Id, CollectionType: "manual", LibraryID: number, LibraryIDs: []string{number},
			ItemCount: intOf(i.ChildCount), SortOrder: len(tab.Collections), PosterURL: poster, BackdropURL: backdrop}
		if i.Overview != nil {
			c.Description = *i.Overview
		}
		created := time.Unix(0, 0)
		if i.DateCreated != nil {
			created = *i.DateCreated
		}
		c.CreatedAt, c.UpdatedAt = instant(created), instant(created)
		tab.Collections = append(tab.Collections, c)
	}
	writeJSON(w, http.StatusOK, tab)
}

// libraryCollectionItems pages through the titles of a collection of a
// library's Collections tab.
func (s *Server) libraryCollectionItems(w http.ResponseWriter, r *http.Request, v viewer) {
	q := r.URL.Query()
	start, limit, ok := pageAsked(w, q)
	if !ok {
		return
	}
	if _, ok := s.library(w, r, v, r.PathValue("id")); !ok {
		return
	}
	collection := r.PathValue("collection")
	if _, err := accounts.ParseID(collection); err != nil {
		problem(w, http.StatusNotFound, "not_found", "Collection not found.")
		return
	}
	page, ok := s.listing(w, r, v, url.Values{"ParentId": {collection}, "Fields": {listFields}, "StartIndex": {strconv.Itoa(start)},
		"Limit": {strconv.Itoa(limit)}, "EnableTotalRecordCount": {"true"}}, start)
	if ok {
		writeJSON(w, http.StatusOK, struct {
			Items []catalogItem `json:"items"`
			Page  pageInfo      `json:"page"`
		}{page.Items, page.Page})
	}
}

// personalCollections lists the apps' personal collections: Polyfin keeps
// none.
func (s *Server) personalCollections(w http.ResponseWriter, _ *http.Request, _ viewer) {
	writeJSON(w, http.StatusOK, struct {
		Items  []struct{} `json:"items"`
		Groups []struct{} `json:"groups"`
	}{[]struct{}{}, []struct{}{}})
}

// collectionCapabilities tells the apps personal collections are not
// supported.
func (s *Server) collectionCapabilities(w http.ResponseWriter, _ *http.Request, _ viewer) {
	type timeZone struct {
		Abbreviation string `json:"abbreviation"`
		UTCOffset    string `json:"utc_offset"`
	}
	type presets struct {
		Media   []string `json:"media"`
		Watched []string `json:"watched"`
	}
	writeJSON(w, http.StatusOK, struct {
		Revision                  string   `json:"revision"`
		State                     string   `json:"state"`
		Allowed                   bool     `json:"allowed"`
		Artwork                   bool     `json:"artwork"`
		CollectionDefaultSort     bool     `json:"collection_default_sort"`
		CollectionSortPreferences bool     `json:"collection_sort_preferences"`
		ContainsItem              bool     `json:"contains_item"`
		CreateDescription         bool     `json:"create_description"`
		DisplayFilterFields       []string `json:"display_filter_fields"`
		DisplayFilterPresets      presets  `json:"display_filter_presets"`
		EffectiveCollectionSort   bool     `json:"effective_collection_sort"`
		Groups                    bool     `json:"groups"`
		ImportSources             []string `json:"import_sources"`
		Imports                   bool     `json:"imports"`
		ItemReorder               bool     `json:"item_reorder"`
		LoginSharing              bool     `json:"login_sharing"`
		ListSearch                bool     `json:"mdblist_search"`
		PosterCollages            bool     `json:"poster_collages"`
		PreviewPosters            bool     `json:"preview_posters"`
		ScheduleTimeZone          timeZone `json:"schedule_time_zone"`
		SortPreferenceKinds       []string `json:"sort_preference_kinds"`
		SyncScheduleEditable      bool     `json:"sync_schedule_editable"`
	}{Revision: "polyfin-1", State: "unsupported", DisplayFilterFields: []string{}, DisplayFilterPresets: presets{[]string{}, []string{}},
		ImportSources: []string{}, ScheduleTimeZone: timeZone{"UTC", "+00:00"}, SortPreferenceKinds: []string{}})
}
